//go:build integration

package jackfleet_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/jackmigrations"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestFleetResetWorkerImmediatelyConfirmsExternalDrop(t *testing.T) {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("fleet_worker"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("test-only"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	require.NoError(t, jackmigrations.Apply(ctx, db))
	store := jackfleet.NewStore(db)
	var user, group, account, sub int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status) VALUES('worker@example.test','test','admin','active') RETURNING id`).Scan(&user))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform,subscription_type,status) VALUES('Worker','openai','subscription','active') RETURNING id`).Scan(&group))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type,credentials,status) VALUES('mock-source','openai','oauth','{"chatgpt_account_id":"test-only-account","access_token":"fake-test-token","plan_type":"pro"}','active') RETURNING id`).Scan(&account))
	_, err = db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)`, account, group)
	require.NoError(t, err)
	expiry := time.Now().Add(30 * 24 * time.Hour)
	created, err := store.Mutate(ctx, user, "create-worker", jackfleet.Input{Action: "create", Name: "Worker", GroupID: group, ExpiresAt: &expiry, Members: []jackfleet.MemberInput{{UserID: user}}})
	require.NoError(t, err)
	fleets, err := store.List(ctx)
	require.NoError(t, err)
	require.Len(t, fleets, 1)
	resetAt := time.Now().Add(3 * 24 * time.Hour).UTC().Truncate(time.Second)
	base := jackfleet.Observation{Identity: jackfleet.SourceIdentity(account, "test-only-account", "", ""), Plan: "pro", Used: 80, At: time.Now(), ResetAt: resetAt, Source: "query"}
	_, err = store.Mutate(ctx, user, "enable-worker", jackfleet.Input{Action: "reset_sync", FleetID: created.FleetID, Versions: map[int64]int64{created.FleetID: fleets[0].Version}, SyncEnabled: true, SyncAccountID: account, SyncBaseline: &base})
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT subscription_id FROM jack_fleet_members WHERE fleet_id=$1`, created.FleetID).Scan(&sub))
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=50,weekly_usage_usd=50,monthly_usage_usd=50 WHERE id=$1`, sub)
	require.NoError(t, err)
	// A recent ordinary quota check must not impose a minute's delay on a reset.
	_, err = db.ExecContext(ctx, `INSERT INTO jack_fleet_reset_jobs(account_id,due_at,last_attempt_at) VALUES($1,'infinity',now())`, account)
	require.NoError(t, err)

	started, release := make(chan struct{}, 1), make(chan struct{})
	var unblock sync.Once
	var queries, flushes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			t.Errorf("unexpected upstream request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		queries.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"account_id":"test-only-account","plan_type":"pro","rate_limit":{"secondary_window":{"used_percent":2,"limit_window_seconds":604800,"reset_at":%d}}}`, resetAt.Unix())
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	target, err := url.Parse(srv.URL)
	require.NoError(t, err)
	// Every upstream request, including unexpected paths, goes only to httptest.
	factory := func(_ string) (*req.Client, error) {
		return req.C().WrapRoundTripFunc(func(rt req.RoundTripper) req.RoundTripFunc {
			return func(r *req.Request) (*req.Response, error) {
				r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
				return rt.RoundTrip(r)
			}
		}), nil
	}
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	accounts := repository.NewAccountRepository(client, db, nil)
	quota := service.NewOpenAIQuotaService(accounts, nil, service.NewOpenAITokenProvider(accounts, nil, nil), factory, nil)
	worker := service.NewFleetResetSyncService(store, quota, func(context.Context) { flushes.Add(1) })
	t.Cleanup(func() {
		unblock.Do(func() { close(release) })
		worker.Stop()
	})
	notify := func(used float64) {
		minutes, seconds := 10080, int(time.Until(resetAt).Seconds())
		service.NotifyFleetResetSnapshot(account, &service.OpenAICodexUsageSnapshot{SecondaryUsedPercent: &used, SecondaryWindowMinutes: &minutes, SecondaryResetAfterSeconds: &seconds, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	notify(0)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("reset confirmation was not queried immediately")
	}
	var daily, weekly, monthly float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, sub).Scan(&daily, &weekly, &monthly))
	require.Equal(t, 50., weekly, "the request snapshot alone must not reset usage")
	for range 10 {
		notify(0)
	}
	unblock.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		events, e := store.ResetEvents(ctx, created.FleetID, 0, 10)
		return e == nil && len(events) == 1 && flushes.Load() >= 2
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, sub).Scan(&daily, &weekly, &monthly))
	require.Zero(t, daily)
	require.Zero(t, weekly)
	require.Zero(t, monthly)
	// A second genuine reset in the same minute/window must survive the
	// ordinary request-sample throttle as well as the query cooldown.
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=7,weekly_usage_usd=7,monthly_usage_usd=7 WHERE id=$1`, sub)
	require.NoError(t, err)
	notify(80)
	notify(0)
	require.Eventually(t, func() bool {
		events, e := store.ResetEvents(ctx, created.FleetID, 0, 10)
		return e == nil && len(events) == 2
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, sub).Scan(&daily, &weekly, &monthly))
	require.Zero(t, daily)
	require.Zero(t, weekly)
	require.Zero(t, monthly)
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=7,weekly_usage_usd=7,monthly_usage_usd=7 WHERE id=$1`, sub)
	require.NoError(t, err)
	notify(0)
	require.Never(t, func() bool { return queries.Load() != 2 }, 100*time.Millisecond, 10*time.Millisecond)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, sub).Scan(&daily, &weekly, &monthly))
	require.Equal(t, 7., daily)
	require.Equal(t, 7., weekly)
	require.Equal(t, 7., monthly)
}
