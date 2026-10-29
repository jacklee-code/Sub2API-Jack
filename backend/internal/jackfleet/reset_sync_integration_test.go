//go:build integration

package jackfleet_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/jackmigrations"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestFleetResetSyncTransactions(t *testing.T) {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("fleet_reset"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("test-only"), tcpostgres.BasicWaitStrategies())
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
	now := time.Now().UTC().Truncate(time.Second)
	store := jackfleet.NewStore(db)
	store.Now = func() time.Time { return now }
	var user, group, account, otherAccount int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,role,status) VALUES('sync@example.test','test','admin','active') RETURNING id`).Scan(&user))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform,subscription_type,status,daily_limit_usd,weekly_limit_usd,monthly_limit_usd) VALUES('Sync','openai','subscription','active',100,100,100) RETURNING id`).Scan(&group))
	for i, dst := range []*int64{&account, &otherAccount} {
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type,credentials,status) VALUES($1,'openai','oauth','{}','active') RETURNING id`, fmt.Sprintf("source-%d", i)).Scan(dst))
		_, err = db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)`, *dst, group)
		require.NoError(t, err)
	}
	expiry := now.Add(60 * 24 * time.Hour)
	created, err := store.Mutate(ctx, user, "create-test", jackfleet.Input{Action: "create", Name: "Sync", GroupID: group, ExpiresAt: &expiry, Members: []jackfleet.MemberInput{{UserID: user, Independent: true, ExpiresAt: &expiry}}})
	require.NoError(t, err)
	id := created.FleetID
	versions := func() map[int64]int64 {
		rows, e := store.List(ctx)
		require.NoError(t, e)
		v := map[int64]int64{}
		for _, f := range rows {
			v[f.ID] = f.Version
		}
		return v
	}
	var sub int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT subscription_id FROM jack_fleet_members WHERE fleet_id=$1`, id).Scan(&sub))
	setUsage := func(v float64) {
		_, e := db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=$2,weekly_usage_usd=$2,monthly_usage_usd=$2,daily_window_start=$3,weekly_window_start=$3,monthly_window_start=$3 WHERE id=$1`, sub, v, now.Add(-8*24*time.Hour))
		require.NoError(t, e)
	}
	checkUsage := func(want float64) {
		var d, w, m float64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, sub).Scan(&d, &w, &m))
		require.Equal(t, want, d)
		require.Equal(t, want, w)
		require.Equal(t, want, m)
	}
	base := jackfleet.Observation{Identity: "same-account", Plan: "pro", Used: 80, At: now, ResetAt: now.Add(3 * 24 * time.Hour), Source: "query"}
	setUsage(90)
	configInput := jackfleet.Input{Action: "reset_sync", FleetID: id, SyncEnabled: true, SyncAccountID: account, SyncBaseline: &base, Versions: versions()}
	_, err = store.Mutate(ctx, user, "enable-sync", configInput)
	require.NoError(t, err)
	checkUsage(90)
	replay, err := store.Replay(ctx, user, "enable-sync", configInput)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := repository.NewUserSubscriptionRepository(client)
	subs := service.NewSubscriptionService(nil, repo, nil, client, nil)
	subs.EnableFleetManagement()
	t.Cleanup(subs.Stop)
	loaded, err := repo.GetByID(ctx, sub)
	require.NoError(t, err)
	require.True(t, loaded.FleetWeeklyManaged)
	require.False(t, loaded.NeedsWeeklyResetAt(now))
	require.Equal(t, base.ResetAt, *loaded.WeeklyResetTime())
	require.NoError(t, repo.ResetWeeklyUsage(ctx, sub, loaded.WeeklyWindowStart, now))
	checkUsage(90)
	// Exercise the real API-key middleware using a synthetic key and a local
	// mock model response. No real upstream account, key or reset credit is used.
	testKey := "sk-fleet-reset-integration-test-only"
	_, err = db.ExecContext(ctx, `INSERT INTO api_keys(user_id,key,name,group_id,status) VALUES($1,$2,'test',$3,'active')`, user, testKey, group)
	require.NoError(t, err)
	cfg := &config.Config{}
	keys := service.NewAPIKeyService(repository.NewAPIKeyRepository(client, db), repository.NewUserRepository(client, db), repository.NewGroupRepository(client, db), repo, nil, nil, cfg)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/responses", gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(keys, subs, cfg)), func(c *gin.Context) { c.JSON(200, gin.H{"output": "mock-model-ok"}) })
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test"}`))
		req.Header.Set("Authorization", "Bearer "+testKey)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	setUsage(101)
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_window_start=$2,weekly_window_start=$2,monthly_window_start=$2 WHERE id=$1`, sub, now)
	require.NoError(t, err)
	denied := request()
	require.Equal(t, http.StatusTooManyRequests, denied.Code, denied.Body.String())
	setUsage(90)
	loaded.DailyWindowStart = &now
	loaded.MonthlyWindowStart = &now
	loaded.WeeklyUsageUSD = 101
	limit := 100.
	_, err = subs.ValidateAndCheckLimits(loaded, &service.Group{WeeklyLimitUSD: &limit})
	require.ErrorIs(t, err, service.ErrWeeklyLimitExceeded)
	require.NoError(t, store.QueueQuotaCheck(ctx, sub))
	freshJob, freshErr := store.ClaimCheck(ctx)
	require.NoError(t, freshErr)
	require.Zero(t, freshJob, "fresh upstream observations do not need another query")
	now = now.Add(time.Minute)
	require.NoError(t, store.QueueQuotaCheck(ctx, sub))
	job, err := store.ClaimCheck(ctx)
	require.NoError(t, err)
	require.Equal(t, account, job)
	require.NoError(t, store.FinishCheck(ctx, account, ""))
	require.NoError(t, store.QueueQuotaCheck(ctx, sub))
	job, err = store.ClaimCheck(ctx)
	require.NoError(t, err)
	require.Zero(t, job)
	now = now.Add(2 * time.Minute)
	low := base
	low.At = now
	low.Used = .2
	require.NoError(t, store.Observe(ctx, account, low))
	checkUsage(90)
	state, err := store.ResetSync(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "confirming", state.Status)
	now = now.Add(time.Minute)
	low.At = now
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- store.Observe(ctx, account, low) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	checkUsage(0)
	events, err := store.ResetEvents(ctx, id, 0, 20)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.True(t, events[0].CachePending)
	require.Equal(t, "early", events[0].Source)
	require.NoError(t, subs.InvalidateFleetSubscription(ctx, user, group))
	allowed := request()
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
	require.Contains(t, allowed.Body.String(), "mock-model-ok")

	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=3,weekly_usage_usd=3,monthly_usage_usd=3 WHERE id=$1`, sub)
	require.NoError(t, err)
	now = now.Add(time.Minute)
	low.At = now
	require.NoError(t, store.Observe(ctx, account, low))
	checkUsage(3)
	old := base
	old.At = now.Add(-time.Minute)
	require.NoError(t, store.Observe(ctx, account, old))
	checkUsage(3)
	now = now.Add(time.Minute)
	high := base
	high.At = now
	high.Used = 20
	require.NoError(t, store.Observe(ctx, account, high))
	now = now.Add(time.Minute)
	low.At = now
	require.NoError(t, store.Observe(ctx, account, low))
	now = now.Add(time.Minute)
	low.At = now
	require.NoError(t, store.Observe(ctx, account, low))
	checkUsage(0)
	events, err = store.ResetEvents(ctx, id, 0, 20)
	require.NoError(t, err)
	require.Len(t, events, 2)
	setUsage(50)
	// The real consume endpoint returns code="reset". Manual redemption must
	// reset all counters even when the observed percentage is already below 1%.
	op, err := store.BeginCredit(ctx, user, account, "manual-credit-1", jackfleet.SourceIdentity(account, "", "", ""))
	require.NoError(t, err)
	_, err = store.BeginCredit(ctx, user, account, "manual-credit-1")
	require.Error(t, err)
	now = now.Add(31 * time.Second)
	retry, err := store.BeginCredit(ctx, user, account, "manual-credit-1")
	require.NoError(t, err)
	require.Equal(t, op.RedeemID, retry.RedeemID)
	require.NoError(t, store.CompleteCredit(ctx, retry, "reset", 1, []byte(`{"code":"reset","windows_reset":1}`)))
	checkUsage(0)
	setUsage(7)
	require.NoError(t, store.CompleteCredit(ctx, retry, "reset", 1, []byte(`{"code":"reset","windows_reset":1}`)))
	checkUsage(7)
	replayOp, err := store.BeginCredit(ctx, user, account, "manual-credit-1")
	require.NoError(t, err)
	require.Equal(t, "complete", replayOp.State)
	require.JSONEq(t, `{"code":"reset","windows_reset":1}`, string(replayOp.Result))
	now = now.Add(time.Minute)
	low.At = now
	require.NoError(t, store.Observe(ctx, account, low))
	checkUsage(7)
	events, err = store.ResetEvents(ctx, id, 0, 20)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, "manual_credit", events[0].Source)
	require.NoError(t, store.MarkSynced(ctx, 0, fmt.Sprintf("fleet-credit:%d:%d", op.ID, id)))
	status, count, err := store.CreditSyncStatus(ctx, op.ID)
	require.NoError(t, err)
	require.Equal(t, "synced", status)
	require.Equal(t, 1, count)
	noCredit, err := store.BeginCredit(ctx, user, account, "no-credit-test")
	require.NoError(t, err)
	require.NoError(t, store.CompleteCredit(ctx, noCredit, "no_credit", 0, []byte(`{"code":"no_credit","windows_reset":0}`)))
	checkUsage(7)
	noWindows, err := store.BeginCredit(ctx, user, account, "no-windows-test")
	require.NoError(t, err)
	require.NoError(t, store.CompleteCredit(ctx, noWindows, "reset", 0, []byte(`{"code":"reset","windows_reset":0}`)))
	checkUsage(7)
	unknown, err := store.BeginCredit(ctx, user, account, "unknown-code-test")
	require.NoError(t, err)
	require.NoError(t, store.CompleteCredit(ctx, unknown, "unknown", 1, []byte(`{"code":"unknown","windows_reset":1}`)))
	checkUsage(7)
	stale, err := store.BeginCredit(ctx, user, account, "stale-target")
	require.NoError(t, err)
	now = now.Add(time.Minute)
	base.At = now
	base.Identity = "second-account"
	_, err = store.Mutate(ctx, user, "change-source", jackfleet.Input{Action: "reset_sync", FleetID: id, SyncEnabled: true, SyncAccountID: otherAccount, SyncBaseline: &base, Versions: versions()})
	require.NoError(t, err)
	require.NoError(t, store.CompleteCredit(ctx, stale, "reset", 2, []byte(`{"code":"reset","windows_reset":2}`)))
	checkUsage(7)
	// Credential replacement during a mock redemption must not reset the new source.
	changed, e := store.BeginCredit(ctx, user, otherAccount, "changed-identity", jackfleet.SourceIdentity(otherAccount, "", "", ""))
	require.NoError(t, e)
	_, e = db.ExecContext(ctx, `UPDATE accounts SET credentials='{"chatgpt_account_id":"replacement"}' WHERE id=$1`, otherAccount)
	require.NoError(t, e)
	require.NoError(t, store.CompleteCredit(ctx, changed, "reset", 1, []byte(`{"code":"reset","windows_reset":1}`)))
	checkUsage(7)
	_, err = store.Mutate(ctx, user, "disable-sync", jackfleet.Input{Action: "reset_sync", FleetID: id, Versions: versions()})
	require.NoError(t, err)
	checkUsage(7)
	loaded, err = repo.GetByID(ctx, sub)
	require.NoError(t, err)
	require.False(t, loaded.FleetWeeklyManaged)
	require.WithinDuration(t, now, *loaded.WeeklyWindowStart, time.Millisecond)
	require.Equal(t, expiry.Unix(), loaded.ExpiresAt.Unix())
	require.Equal(t, sub, loaded.ID)
}
