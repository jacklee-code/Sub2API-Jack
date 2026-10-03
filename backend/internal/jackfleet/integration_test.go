//go:build integration

package jackfleet_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/jackmigrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestFleetLifecycle(t *testing.T) {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("fleet_test"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("test-only"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	require.NoError(t, jackmigrations.Apply(ctx, db))
	require.NoError(t, jackmigrations.Apply(ctx, db))
	s := jackfleet.NewStore(db)
	var admin, user, other, g1, g2, g3 int64
	for i, dst := range []*int64{&admin, &user, &other} {
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status) VALUES($1,'test','user','active') RETURNING id`, fmt.Sprintf("fleet-%d@example.test", i)).Scan(dst))
	}
	for i, dst := range []*int64{&g1, &g2, &g3} {
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform,subscription_type,status) VALUES($1,'openai','subscription','active') RETURNING id`, fmt.Sprintf("fleet-group-%d", i)).Scan(dst))
	}
	now := time.Now().UTC()
	expiry := now.AddDate(0, 0, 30)
	adminExpiry := now.AddDate(0, 0, 40)
	var originalSub int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,daily_usage_usd,weekly_usage_usd,monthly_usage_usd) VALUES($1,$2,now()-interval '1 day',$3,'active',2,3,4) RETURNING id`, admin, g1, adminExpiry).Scan(&originalSub))
	versions := func() map[int64]int64 {
		fleets, e := s.List(ctx)
		require.NoError(t, e)
		v := map[int64]int64{}
		for _, f := range fleets {
			v[f.ID] = f.Version
		}
		return v
	}
	seq := 0
	mutate := func(in jackfleet.Input) *jackfleet.Result {
		seq++
		in.Versions = versions()
		out, e := s.Mutate(ctx, admin, fmt.Sprintf("operation-%08d", seq), in)
		require.NoError(t, e)
		return out
	}
	a := mutate(jackfleet.Input{Action: "create", Name: "A", GroupID: g1, ExpiresAt: &expiry, Members: []jackfleet.MemberInput{{UserID: admin, Independent: true, Adopt: true}, {UserID: user}}}).FleetID
	b := mutate(jackfleet.Input{Action: "create", Name: "B", GroupID: g2, ExpiresAt: &expiry}).FleetID
	var id int64
	var amount float64
	var date time.Time
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id,expires_at,daily_usage_usd FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`, admin, g1).Scan(&id, &date, &amount))
	require.Equal(t, originalSub, id)
	require.WithinDuration(t, adminExpiry, date, time.Millisecond)
	require.Equal(t, 2.0, amount)
	mutate(jackfleet.Input{Action: "expiry", FleetID: a, Days: 30})
	require.NoError(t, db.QueryRowContext(ctx, `SELECT expires_at FROM user_subscriptions WHERE id=$1`, originalSub).Scan(&date))
	require.WithinDuration(t, adminExpiry, date, time.Millisecond)
	// Normal admin paths cannot change a managed expiry, including independent members.
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET expires_at=expires_at+interval '1 day' WHERE id=$1`, originalSub)
	require.ErrorContains(t, err, "FLEET_MANAGED")
	_, err = db.ExecContext(ctx, `UPDATE groups SET deleted_at=now() WHERE id=$1`, g1)
	require.ErrorContains(t, err, "FLEET_MANAGED")
	// Adoption is explicit, and failures leave membership/expiry untouched.
	var personalID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES($1,$2,now(),$3,'active') RETURNING id`, other, g2, expiry).Scan(&personalID))
	_, err = s.Mutate(ctx, admin, "adopt-conflict", jackfleet.Input{Action: "add", FleetID: b, Versions: versions(), Members: []jackfleet.MemberInput{{UserID: other}}})
	require.Error(t, err)
	mutate(jackfleet.Input{Action: "add", FleetID: b, Members: []jackfleet.MemberInput{{UserID: other, Adopt: true}}})
	// Seed a key and usage. Moving keeps key material/limits and independent usage.
	_, err = db.ExecContext(ctx, `INSERT INTO api_keys(user_id,key,name,group_id,status,quota,quota_used) VALUES($1,'fleet-test-key','test',$2,'active',100,7)`, user, g1)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=5,weekly_usage_usd=6,monthly_usage_usd=7,daily_window_start=now(),weekly_window_start=now(),monthly_window_start=now() WHERE user_id=$1 AND group_id=$2`, user, g1)
	require.NoError(t, err)
	mutate(jackfleet.Input{Action: "transfer", FleetID: a, TargetID: b, UserID: user})
	var key string
	var bound int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT key,group_id,quota_used FROM api_keys WHERE user_id=$1`, user).Scan(&key, &bound, &amount))
	require.Equal(t, "fleet-test-key", key)
	require.Equal(t, g2, bound)
	require.Equal(t, 7.0, amount)
	var status string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`, user, g1).Scan(&status))
	require.Equal(t, "suspended", status)
	mutate(jackfleet.Input{Action: "transfer", FleetID: b, TargetID: a, UserID: user})
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`, user, g1).Scan(&amount))
	require.Equal(t, 5.0, amount)
	// Reset includes independent members but leaves personal subscriptions untouched.
	_, err = db.ExecContext(ctx, `INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,daily_usage_usd) VALUES($1,$2,now(),$3,'active',9)`, user, g3, expiry)
	require.NoError(t, err)
	mutate(jackfleet.Input{Action: "reset_all", Daily: true, Weekly: true, Monthly: true})
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,expires_at FROM user_subscriptions WHERE id=$1`, originalSub).Scan(&amount, &date))
	require.Zero(t, amount)
	require.WithinDuration(t, adminExpiry, date, time.Millisecond)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`, user, g3).Scan(&amount))
	require.Equal(t, 9.0, amount)
	// A replay with a stale fleet version must return the saved result without extending again.
	input := jackfleet.Input{Action: "expiry", FleetID: a, Days: 30, Versions: versions()}
	first, err := s.Mutate(ctx, admin, "replay-extension", input)
	require.NoError(t, err)
	second, err := s.Mutate(ctx, admin, "replay-extension", input)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.FleetID, second.FleetID)
	input.Days = 31
	_, err = s.Mutate(ctx, admin, "replay-extension", input)
	require.Error(t, err)
	// Concurrent changes from the same page snapshot: exactly one may commit.
	v := versions()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Mutate(ctx, admin, fmt.Sprintf("concurrent-%d", i), jackfleet.Input{Action: "expiry", FleetID: a, Days: 1, Versions: v})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
	// Removing an independent-expiry member still disables access; rejoining retains ID.
	mutate(jackfleet.Input{Action: "remove", FleetID: a, UserID: admin})
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status FROM user_subscriptions WHERE id=$1`, originalSub).Scan(&status))
	require.Equal(t, "suspended", status)
	mutate(jackfleet.Input{Action: "add", FleetID: a, Members: []jackfleet.MemberInput{{UserID: admin}}})
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id,expires_at FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`, admin, g1).Scan(&id, &date))
	require.Equal(t, originalSub, id)
	require.WithinDuration(t, adminExpiry, date, time.Millisecond)
	pending, err := s.Pending(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, pending)
	require.NoError(t, s.MarkSynced(ctx, pending[0].Actor, pending[0].Key))
	// The guard must not suppress deletion of an unrelated subscription.
	result, err := db.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE user_id=$1 AND group_id=$2`, user, g3)
	require.NoError(t, err)
	n, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
