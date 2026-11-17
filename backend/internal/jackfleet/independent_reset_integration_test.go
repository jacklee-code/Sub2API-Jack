//go:build integration

package jackfleet_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/jackmigrations"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// This drives the real admin endpoint, credit service, SQL transaction, and
// passive detector. Every upstream request is redirected to a local HTTP server.
func TestIndependentResetPreservesFleetThroughQuotaRecovery(t *testing.T) {
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

	// Keep the background post-redemption query behind its normal cooldown;
	// this test drives delayed samples deterministically after stopping the worker.
	_, err = db.ExecContext(ctx, `INSERT INTO jack_fleet_reset_jobs(account_id,due_at,last_attempt_at) VALUES($1,'infinity',now())`, account)
	require.NoError(t, err)
	var redemptionIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			redemptionIDs = append(redemptionIDs, body["redeem_request_id"])
			_, _ = w.Write([]byte(`{"code":"reset","windows_reset":1}`))
		case "/backend-api/wham/usage":
			_, _ = fmt.Fprintf(w, `{"account_id":"test-only-account","plan_type":"pro","rate_limit":{"secondary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":%d}}}`, resetAt.Unix())
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	require.NoError(t, err)
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
	worker := service.NewFleetResetSyncService(store, quota, func(context.Context) {})
	t.Cleanup(worker.Stop)
	handler := admin.NewOpenAIOAuthHandler(nil, &independentResetAdmin{}, quota, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/accounts/:id/reset-quota", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: user})
		handler.ResetQuota(c)
	})
	reset := func(independent bool) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"independent_reset":%t}`, independent)
		request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/accounts/%d/reset-quota", account), strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "independent-test-credit")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		return w
	}
	checkUsage := func(want float64) {
		t.Helper()
		var daily, weekly, monthly float64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, sub).Scan(&daily, &weekly, &monthly))
		require.Equal(t, want, daily, "independent reset must preserve daily usage")
		require.Equal(t, want, weekly, "independent reset must preserve weekly usage")
		require.Equal(t, want, monthly, "independent reset must preserve monthly usage")
	}
	result := reset(true)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	checkUsage(50)
	require.Contains(t, result.Body.String(), `"fleet_sync_status":"independent"`)
	result = reset(true)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	require.Len(t, redemptionIDs, 1, "a replay must not spend another credit")
	checkUsage(50)
	result = reset(false)
	require.Equal(t, http.StatusConflict, result.Code, result.Body.String())
	require.Len(t, redemptionIDs, 1, "a retry cannot change reset mode")
	// Stop the worker and reload the store to test persisted suppression without
	// races from its one-shot check. A delayed pre-redemption observation is ignored.
	worker.Stop()
	store = jackfleet.NewStore(db)
	require.NoError(t, store.Observe(ctx, account, base))
	now := time.Now().Add(time.Second)
	store.Now = func() time.Time { return now }
	high := base
	high.At = now
	high.Source = "request"
	require.NoError(t, store.Observe(ctx, account, high))
	now = now.Add(time.Second)
	high.At = now
	high.Source = "query"
	require.NoError(t, store.Observe(ctx, account, high))
	// Even an initially stale high usage query must not remove the barrier.
	for _, source := range []string{"request", "query", "query"} {
		now = now.Add(time.Second)
		low := base
		low.At, low.Used, low.Source = now, 0, source
		if source == "query" {
			low.Used = 2
		} // new traffic while recovery is queried
		require.NoError(t, store.Observe(ctx, account, low))
		checkUsage(50)
	}
	events, err := store.ResetEvents(ctx, created.FleetID, 0, 10)
	require.NoError(t, err)
	require.Empty(t, events, "quota recovery must not be detected as a fleet reset")
	// A subsequent real reset must still work within the same seven-day window.
	now = now.Add(time.Second)
	high.At, high.Source = now, "query"
	require.NoError(t, store.Observe(ctx, account, high))
	for _, source := range []string{"request", "query"} {
		now = now.Add(time.Second)
		low := base
		low.At, low.Used, low.Source = now, 0, source
		require.NoError(t, store.Observe(ctx, account, low))
	}
	checkUsage(0)
	// Recovery after a process restart retains both the upstream id and the mode.
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=17,weekly_usage_usd=17,monthly_usage_usd=17 WHERE id=$1`, sub)
	require.NoError(t, err)
	now = now.Add(time.Second)
	high.At = now
	require.NoError(t, store.Observe(ctx, account, high))
	// A candidate that predates the redemption must be canceled as well.
	now = now.Add(time.Second)
	candidate := base
	candidate.At, candidate.Used, candidate.Source = now, 0, "request"
	require.NoError(t, store.Observe(ctx, account, candidate))
	pending, err := store.BeginIndependentCredit(ctx, user, account, "restart-independent", base.Identity)
	require.NoError(t, err)
	now = now.Add(time.Second)
	low := base
	low.At, low.Used, low.Source = now, 0, "query"
	require.NoError(t, store.Observe(ctx, account, low))
	checkUsage(17)
	// A fleet enabled during the unresolved redemption also needs protection.
	var secondGroup, secondSub int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform,subscription_type,status) VALUES('During reset','openai','subscription','active') RETURNING id`).Scan(&secondGroup))
	_, err = db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)`, account, secondGroup)
	require.NoError(t, err)
	second, err := store.Mutate(ctx, user, "create-during-reset", jackfleet.Input{Action: "create", Name: "During reset", GroupID: secondGroup, ExpiresAt: &expiry, Members: []jackfleet.MemberInput{{UserID: user}}})
	require.NoError(t, err)
	fleetRows, err := store.List(ctx)
	require.NoError(t, err)
	versions := map[int64]int64{}
	for _, f := range fleetRows {
		versions[f.ID] = f.Version
	}
	high.At = now
	_, err = store.Mutate(ctx, user, "enable-during-reset", jackfleet.Input{Action: "reset_sync", FleetID: second.FleetID, SyncEnabled: true, SyncAccountID: account, SyncBaseline: &high, Versions: versions})
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT subscription_id FROM jack_fleet_members WHERE fleet_id=$1`, second.FleetID).Scan(&secondSub))
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=23,weekly_usage_usd=23,monthly_usage_usd=23 WHERE id=$1`, secondSub)
	require.NoError(t, err)
	now = now.Add(31 * time.Second)
	restarted := jackfleet.NewStore(db)
	restarted.Now = store.Now
	recovered, err := restarted.ClaimCredit(ctx)
	require.NoError(t, err)
	require.NotNil(t, recovered)
	require.True(t, recovered.IndependentReset)
	require.Equal(t, pending.RedeemID, recovered.RedeemID)
	require.NoError(t, restarted.CompleteCredit(ctx, recovered, "reset", 1, []byte(`{"code":"reset","windows_reset":1}`)))
	for range 2 {
		now = now.Add(time.Second)
		low.At = now
		require.NoError(t, restarted.Observe(ctx, account, low))
		checkUsage(17)
	}
	var secondDaily, secondWeekly, secondMonthly float64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT daily_usage_usd,weekly_usage_usd,monthly_usage_usd FROM user_subscriptions WHERE id=$1`, secondSub).Scan(&secondDaily, &secondWeekly, &secondMonthly))
	require.Equal(t, 23., secondDaily)
	require.Equal(t, 23., secondWeekly)
	require.Equal(t, 23., secondMonthly)
	// Definitive no-credit results must not suppress the next genuine recovery.
	now = now.Add(time.Second)
	high.At = now
	require.NoError(t, restarted.Observe(ctx, account, high))
	rejected, err := restarted.BeginIndependentCredit(ctx, user, account, "no-independent-credit", base.Identity)
	require.NoError(t, err)
	require.NoError(t, restarted.CompleteCredit(ctx, rejected, "no_credit", 0, []byte(`{"code":"no_credit","windows_reset":0}`)))
	for range 2 {
		now = now.Add(time.Second)
		low.At = now
		require.NoError(t, restarted.Observe(ctx, account, low))
	}
	checkUsage(0)
	// A low baseline must not let an initially stale high query re-arm detection.
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=19,weekly_usage_usd=19,monthly_usage_usd=19 WHERE id=$1`, sub)
	require.NoError(t, err)
	lowBaseline, err := restarted.BeginIndependentCredit(ctx, user, account, "low-baseline-independent", base.Identity)
	require.NoError(t, err)
	require.NoError(t, restarted.CompleteCredit(ctx, lowBaseline, "reset", 1, []byte(`{"code":"reset","windows_reset":1}`)))
	now = now.Add(time.Second)
	high.At = now
	require.NoError(t, restarted.Observe(ctx, account, high))
	for range 2 {
		now = now.Add(time.Second)
		low.At = now
		require.NoError(t, restarted.Observe(ctx, account, low))
		checkUsage(19)
	}
	// Recovery spanning the previous weekly deadline is still independent.
	_, err = db.ExecContext(ctx, `UPDATE user_subscriptions SET daily_usage_usd=29,weekly_usage_usd=29,monthly_usage_usd=29 WHERE id=$1`, sub)
	require.NoError(t, err)
	now = now.Add(time.Second)
	high.At = now
	require.NoError(t, restarted.Observe(ctx, account, high))
	boundary, err := restarted.BeginIndependentCredit(ctx, user, account, "boundary-independent", base.Identity)
	require.NoError(t, err)
	require.NoError(t, restarted.CompleteCredit(ctx, boundary, "reset", 1, []byte(`{"code":"reset","windows_reset":1}`)))
	now = base.ResetAt.Add(time.Second)
	low.At, low.ResetAt = now, now.Add(7*24*time.Hour)
	require.NoError(t, restarted.Observe(ctx, account, low))
	checkUsage(29)
	// The following natural rollover still resets once a fresh query confirms it.
	now = low.ResetAt.Add(time.Second)
	low.At, low.ResetAt, low.Used, low.Source = now, now.Add(7*24*time.Hour), 10, "request"
	require.NoError(t, restarted.Observe(ctx, account, low))
	checkUsage(29)
	now = now.Add(time.Second)
	low.At, low.Source = now, "query"
	require.NoError(t, restarted.Observe(ctx, account, low))
	checkUsage(0)
	var preservedSub int64
	var preservedExpiry time.Time
	require.NoError(t, db.QueryRowContext(ctx, `SELECT subscription_id,s.expires_at FROM jack_fleet_members m JOIN user_subscriptions s ON s.id=m.subscription_id WHERE m.fleet_id=$1`, created.FleetID).Scan(&preservedSub, &preservedExpiry))
	require.Equal(t, sub, preservedSub)
	require.WithinDuration(t, expiry, preservedExpiry, time.Millisecond)
}

type independentResetAdmin struct{ service.AdminService }
