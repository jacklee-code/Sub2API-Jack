//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	"github.com/stretchr/testify/require"
)

func TestFleetObservationRejectsFabricatedOrWrongWindows(t *testing.T) {
	now := time.Now()
	a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "test-account"}}
	parse := func(body string) *OpenAIQuotaUsage {
		var q OpenAIQuotaUsage
		require.NoError(t, json.Unmarshal([]byte(body), &q))
		return &q
	}
	for _, body := range []string{
		`{"rate_limit":{"primary_window":{"limit_window_seconds":604800,"reset_at":4102444800}}}`,
		`{"rate_limit":{"primary_window":null}}`,
		`{"rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":18000,"reset_at":4102444800}}}`,
		`{"account_id":"different-account","rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":4102444800}}}`,
	} {
		require.Nil(t, fleetObservation(fleetObservationInput{quota: parse(body), at: now}, a), body)
	}
	q := parse(`{"account_id":"test-account","rate_limit":{"secondary_window":{"used_percent":0.2,"limit_window_seconds":604800,"reset_at":4102444800}}}`)
	o := fleetObservation(fleetObservationInput{quota: q, at: now}, a)
	require.NotNil(t, o)
	require.Equal(t, .2, o.Used)
	require.Nil(t, fleetObservation(fleetObservationInput{quota: q, at: now.Add(-3 * time.Minute)}, a))
	five, seven, seconds, pct := 300, 10080, 600, 0.3
	snap := &OpenAICodexUsageSnapshot{PrimaryWindowMinutes: &five, SecondaryWindowMinutes: &seven, SecondaryResetAfterSeconds: &seconds, SecondaryUsedPercent: &pct, UpdatedAt: now.Format(time.RFC3339)}
	require.NotNil(t, fleetObservation(fleetObservationInput{snapshot: snap}, a))
	snap.ServerObservedAt = now.Add(-3 * time.Minute).Format(time.RFC3339)
	require.Nil(t, fleetObservation(fleetObservationInput{snapshot: snap}, a), "late streams and reused handshake headers are not fresh observations")
	snap.ServerObservedAt = ""
	snap.SecondaryUsedPercent = nil
	require.Nil(t, fleetObservation(fleetObservationInput{snapshot: snap}, a))
}
func TestFleetUsageOnlyQueryAndStableRedemptionUseMockUpstream(t *testing.T) {
	account := &Account{ID: 203, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "test-only-account"}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	cache := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "fake-test-token"}}
	var usageCalls int
	var redeemIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			usageCalls++
			_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":4102444800}}}`))
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			redeemIDs = append(redeemIDs, body["redeem_request_id"])
			require.Empty(t, body["credit_id"])
			_, _ = w.Write([]byte(`{"code":"ok","windows_reset":1}`))
		default:
			t.Errorf("unexpected mock upstream call: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	// All requests are redirected to the local httptest server. No real token or
	// real credit is used, including the explicit simulated consume response.
	quota := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, cache, nil), newQuotaRedirectingFactory(srv), nil)
	q, err := quota.QueryUsageSnapshot(context.Background(), account.ID)
	require.NoError(t, err)
	require.True(t, q.RateLimit.PrimaryWindow.usedPercentPresent)
	require.Equal(t, 1, usageCalls)
	for range 2 {
		_, err = quota.ResetCreditStable(context.Background(), account.ID, "test-operation-same-id")
		require.NoError(t, err)
	}
	require.Equal(t, []string{"test-operation-same-id", "test-operation-same-id"}, redeemIDs)
	_, err = quota.ResetCreditStable(context.Background(), account.ID, "test-operation-same-id", "a-different-account")
	require.Error(t, err)
	require.Len(t, redeemIDs, 2, "an identity mismatch must never reach even the mock consume endpoint")

}

type fleetNoNetworkQuota struct{ t *testing.T }

func (q fleetNoNetworkQuota) QueryUsageSnapshot(context.Context, int64) (*OpenAIQuotaUsage, error) {
	q.t.Fatal("idle fleet must not query upstream")
	return nil, nil
}
func (q fleetNoNetworkQuota) ResetCreditStable(context.Context, int64, string, ...string) (*OpenAIQuotaResetResult, error) {
	q.t.Fatal("idle worker must not spend a credit")
	return nil, nil
}
func TestFleetWorkerHasNoIdleUpstreamPolling(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery("UPDATE jack_fleet_credit_operations").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "redeem_id", "state"}))
	mock.ExpectQuery("UPDATE jack_fleet_reset_jobs").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
	s := &FleetResetSyncService{store: jackfleet.NewStore(db), quota: fleetNoNetworkQuota{t}}
	s.runPending(context.Background())
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestFleetBlockedSignalIsNonblockingAndCoalesced(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &FleetResetSyncService{ctx: ctx, queue: make(chan fleetObservationInput, 2), blocked: map[int64]time.Time{}}
	fleetResetRegistry.Lock()
	old := fleetResetRegistry.service
	fleetResetRegistry.service = s
	fleetResetRegistry.Unlock()
	t.Cleanup(func() { fleetResetRegistry.Lock(); fleetResetRegistry.service = old; fleetResetRegistry.Unlock() })
	NotifyFleetQuotaBlocked(&UserSubscription{ID: 1})
	require.Empty(t, s.queue)
	sub := &UserSubscription{ID: 2, FleetWeeklyManaged: true}
	NotifyFleetQuotaBlocked(sub)
	NotifyFleetQuotaBlocked(sub)
	require.Len(t, s.queue, 1)
	require.Equal(t, int64(2), (<-s.queue).blockedSubscription)
}

func TestFleetIdentitySurvivesTokenRefreshPlanMetadata(t *testing.T) {
	a := &Account{ID: 1, Credentials: map[string]any{"chatgpt_account_id": "test-account", "email": "test@example.test", "plan_type": "pro", "access_token": "old-test-token"}}
	before := fleetAccountIdentity(a)
	a.Credentials["access_token"] = "new-test-token"
	a.Credentials["plan_type"] = "plus"
	require.Equal(t, before, fleetAccountIdentity(a), "a successful manual redemption must survive token refresh metadata updates")
	a.Credentials["chatgpt_account_id"] = "different-account"
	require.NotEqual(t, before, fleetAccountIdentity(a))
}
