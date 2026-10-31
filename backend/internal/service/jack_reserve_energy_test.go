//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackreserve"
	"github.com/stretchr/testify/require"
)

type jackReserveRepoStub struct {
	schedulerGroupAwareOpenAIAccountRepo
	listErr   error
	listCalls int
}

func (r *jackReserveRepoStub) Update(_ context.Context, account *Account) error {
	for i := range r.accounts {
		if r.accounts[i].ID == account.ID {
			r.accounts[i] = *account
			return nil
		}
	}
	return ErrAccountNotFound
}

func (r *jackReserveRepoStub) UpdateExtra(_ context.Context, id int64, extra map[string]any) error {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			if r.accounts[i].Extra == nil {
				r.accounts[i].Extra = map[string]any{}
			}
			for key, value := range extra {
				r.accounts[i].Extra[key] = value
			}
			return nil
		}
	}
	return ErrAccountNotFound
}

func (r *jackReserveRepoStub) ListJackReserveAccounts(context.Context) ([]Account, error) {
	r.listCalls++
	var result []Account
	for _, a := range r.accounts {
		if a.Extra[jackreserve.EnabledKey] == true {
			result = append(result, a)
		}
	}
	return result, r.listErr
}

func TestJackReserveDisabledLookupIsCachedAndActivationInvalidates(t *testing.T) {
	svc, repo, req := reserveFixture()
	repo.accounts[0].Extra[jackreserve.EnabledKey] = false
	for range 3 {
		selection, _, err := svc.selectJackReserveEnergy(context.Background(), req)
		require.NoError(t, err)
		require.Nil(t, selection)
	}
	require.Equal(t, 1, repo.listCalls)
	admin := &adminServiceImpl{accountRepo: repo}
	require.NoError(t, admin.UpdateAccountExtra(context.Background(), 1, map[string]any{jackreserve.EnabledKey: true}))
	selection, _, err := svc.selectJackReserveEnergy(context.Background(), req)
	require.NoError(t, err)
	require.EqualValues(t, 3, selection.Account.ID)
	require.Equal(t, 2, repo.listCalls)
	selection.ReleaseFunc()
}

func reserveFixture() (*OpenAIGatewayService, *jackReserveRepoStub, OpenAIAccountScheduleRequest) {
	future := time.Now().Add(time.Hour)
	primary := *newOpenAIImageGenerationControlTestAccount()
	primary.ID = 1
	primary.GroupIDs = []int64{7}
	primary.Type = AccountTypeOAuth
	primary.RateLimitResetAt = &future
	primary.Extra = map[string]any{jackreserve.EnabledKey: true, jackreserve.AccountsKey: []int64{3, 2}}
	first := *newOpenAIImageGenerationControlTestAccount()
	first.ID = 3
	second := *newOpenAIImageGenerationControlTestAccount()
	second.ID = 2
	repo := &jackReserveRepoStub{schedulerGroupAwareOpenAIAccountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{primary, first, second}}}}
	svc := newOpenAIImageGenerationControlTestService(nil)
	svc.accountRepo = repo
	group := int64(7)
	return svc, repo, OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, RequestedModel: "gpt-5.6-sol", RequiredTransport: OpenAIUpstreamTransportHTTPSSE, PreviousResponseCanMove: true}
}

func TestJackReserveFillFirstFallbackAndRecovery(t *testing.T) {
	svc, repo, req := reserveFixture()
	ctx := context.Background()
	for range 3 {
		selected, excluded, err := svc.selectJackReserveEnergy(ctx, req)
		require.NoError(t, err)
		require.EqualValues(t, 3, selected.Account.ID)
		require.Equal(t, 2, selected.JackReserveRemaining)
		require.Contains(t, excluded, int64(2))
		require.Contains(t, excluded, int64(3))
		require.Nil(t, req.ExcludedIDs, "caller exclusions must not be mutated")
		selected.ReleaseFunc()
	}
	req.ExcludedIDs = map[int64]struct{}{3: {}}
	selected, _, err := svc.selectJackReserveEnergy(ctx, req)
	require.NoError(t, err)
	require.EqualValues(t, 2, selected.Account.ID)
	selected.ReleaseFunc()
	repo.accounts[1].Extra = map[string]any{"quota_limit": 1.0, "quota_used": 1.0}
	req.ExcludedIDs = nil
	selected, _, err = svc.selectJackReserveEnergy(ctx, req)
	require.NoError(t, err)
	require.EqualValues(t, 2, selected.Account.ID)
	selected.ReleaseFunc()
	repo.accounts[0].RateLimitResetAt = nil
	selected, excluded, err := svc.selectJackReserveEnergy(ctx, req)
	require.NoError(t, err)
	require.Nil(t, selected)
	// The regular scheduler sees the recovered primary, even with sticky state
	// pointing at a reserve; reserve IDs stay excluded from ordinary scheduling.
	regular, _, err := svc.SelectAccountWithScheduler(ctx, req.GroupID, "", "", req.RequestedModel, excluded, OpenAIUpstreamTransportHTTPSSE, false)
	require.NoError(t, err)
	require.EqualValues(t, 1, regular.Account.ID)
	regular.ReleaseFunc()
}

func TestJackReserveEligibilityBoundaries(t *testing.T) {
	for _, name := range []string{"other_group", "disabled_primary", "inactive_primary", "expired_primary", "disabled_reserves", "wrong_platform", "privacy", "model", "pinned_continuation", "all_failed"} {
		t.Run(name, func(t *testing.T) {
			svc, repo, req := reserveFixture()
			switch name {
			case "other_group":
				g := int64(88)
				req.GroupID = &g
			case "disabled_primary":
				repo.accounts[0].Schedulable = false
			case "inactive_primary":
				repo.accounts[0].Status = "inactive"
			case "expired_primary":
				past := time.Now().Add(-time.Hour)
				repo.accounts[0].AutoPauseOnExpired = true
				repo.accounts[0].ExpiresAt = &past
			case "disabled_reserves":
				repo.accounts[1].Schedulable = false
				repo.accounts[2].Status = "error"
			case "wrong_platform":
				repo.accounts[1].Platform = PlatformAnthropic
				repo.accounts[2].Platform = PlatformGemini
			case "privacy":
				req.RequirePrivacySet = true
			case "model":
				for i := 1; i < 3; i++ {
					repo.accounts[i].Credentials["model_mapping"] = map[string]any{"other": "other"}
				}
			case "pinned_continuation":
				req.PreviousResponseID = "resp_owner"
				req.PreviousResponseCanMove = false
			case "all_failed":
				req.ExcludedIDs = map[int64]struct{}{3: {}, 2: {}}
			}
			selected, _, err := svc.selectJackReserveEnergy(context.Background(), req)
			require.NoError(t, err)
			require.Nil(t, selected)
		})
	}
}

func TestJackReserveUsesDestinationCredentialsAndProxy(t *testing.T) {
	for _, proxy := range []*Proxy{nil, {ID: 22, Protocol: "http", Host: "127.0.0.1", Port: 18081}} {
		svc, repo, req := reserveFixture()
		primaryProxy := int64(11)
		repo.accounts[0].ProxyID = &primaryProxy
		repo.accounts[0].Proxy = &Proxy{ID: 11, Protocol: "http", Host: "127.0.0.1", Port: 18080}
		repo.accounts[1].Credentials = map[string]any{"api_key": "sk-reserve-fixture"}
		if proxy != nil {
			repo.accounts[1].ProxyID = &proxy.ID
			repo.accounts[1].Proxy = proxy
		}
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_mock","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`))}}
		svc.httpUpstream = upstream
		selection, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), req.GroupID, "", "", req.RequestedModel, nil, req.RequiredTransport, "", false, true, true)
		require.NoError(t, err)
		require.EqualValues(t, 3, selection.Account.ID)
		c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
		result, err := svc.Forward(context.Background(), c, selection.Account, []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`))
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "Bearer sk-reserve-fixture", upstream.lastReq.Header.Get("Authorization"))
		expected := ""
		if proxy != nil {
			expected = proxy.URL()
		}
		require.Equal(t, expected, upstream.lastProxyURL)
		selection.ReleaseFunc()
	}
}

func TestJackReserveFillFirstQueuesAndReleases(t *testing.T) {
	svc, _, req := reserveFixture()
	var acquired, released []int64
	cache := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{3: false, 2: true}, acquiredIDs: &acquired, releasedIDs: &released}
	svc.concurrencyService = NewConcurrencyService(cache)
	selection, _, err := svc.selectJackReserveEnergy(context.Background(), req)
	require.NoError(t, err)
	require.EqualValues(t, 3, selection.Account.ID)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, []int64{3}, acquired)
}

func TestJackReserveConfigurationValidationAndReadFailure(t *testing.T) {
	svc, repo, req := reserveFixture()
	admin := &adminServiceImpl{accountRepo: repo}
	for _, ids := range [][]int64{{1}, {3, 3}, {77}} {
		err := admin.validateJackReserveExtra(context.Background(), &repo.accounts[0], map[string]any{jackreserve.EnabledKey: true, jackreserve.AccountsKey: ids})
		require.Error(t, err)
	}
	require.NoError(t, admin.validateJackReserveExtra(context.Background(), &repo.accounts[0], repo.accounts[0].Extra))
	repo.listErr = errors.New("database unavailable")
	_, _, err := svc.selectJackReserveEnergy(context.Background(), req)
	require.ErrorContains(t, err, "database unavailable")
}

func TestJackReserveCreditPriority(t *testing.T) {
	for _, tc := range []struct {
		name, credits string
		wantPrimary   bool
	}{
		{"positive", `{"has_credits":true,"unlimited":false,"balance":"1200.50"}`, true},
		{"tiny decimal", `{"has_credits":true,"balance":"0.000000000000000000000001"}`, true},
		{"unlimited", `{"has_credits":false,"unlimited":true,"balance":null}`, true},
		{"hidden balance", `{"has_credits":true,"balance":null}`, true},
		{"zero", `{"has_credits":true,"balance":"0.000"}`, false},
		{"unavailable", `{"has_credits":false,"balance":"10"}`, false},
		{"malformed", `{"has_credits":true,"balance":"NaN"}`, false},
		{"missing", `null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, req := reserveFixture()
			parent := &repo.accounts[0]
			parent.RateLimitResetAt = nil
			parent.Extra["auto_pause_5h_threshold"] = 0.95
			parent.Extra["codex_5h_used_percent"] = 95.0
			parent.Extra["codex_5h_reset_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
			parent.Extra["codex_usage_updated_at"] = time.Now().Format(time.RFC3339)
			var credits *OpenAICredits
			require.NoError(t, json.Unmarshal([]byte(tc.credits), &credits))
			parent.Extra[openaiQuotaCreditsKey] = openAICreditsSnapshot{Credits: credits, FetchedAt: time.Now().Unix()}
			// Reset cards must never be mistaken for spendable points.
			parent.Extra[openaiQuotaResetCreditsKey] = map[string]any{"available_count": 10}
			require.Equal(t, tc.wantPrimary, jackReserveHasSpendableCredits(parent))
			paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), parent)
			require.Equal(t, !tc.wantPrimary, paused)
			selection, _, err := svc.SelectAccountWithScheduler(context.Background(), req.GroupID, "", "", req.RequestedModel, nil, req.RequiredTransport, false)
			require.NoError(t, err)
			want := int64(3)
			if tc.wantPrimary {
				want = 1
			}
			require.Equal(t, want, selection.Account.ID)
			selection.ReleaseFunc()
			if tc.wantPrimary {
				// Actual upstream rejection wins over a stale positive snapshot.
				until := time.Now().Add(time.Minute)
				parent.RateLimitResetAt = &until
				selection, _, err = svc.selectJackReserveEnergy(context.Background(), req)
				require.NoError(t, err)
				require.EqualValues(t, 3, selection.Account.ID)
				selection.ReleaseFunc()
			}
		})
	}
}

func TestJackReserveUnknownCreditsGetUpstreamDecision(t *testing.T) {
	svc, repo, req := reserveFixture()
	parent := &repo.accounts[0]
	parent.RateLimitResetAt = nil
	parent.Extra["auto_pause_5h_threshold"] = 1.0
	parent.Extra["codex_5h_used_percent"] = 100.0
	parent.Extra["codex_5h_reset_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), req.GroupID, "", "", req.RequestedModel, nil, req.RequiredTransport, false)
	require.NoError(t, err)
	require.EqualValues(t, 1, selection.Account.ID, "100%% subscription usage does not rule out spendable Credit")
	selection.ReleaseFunc()
	// Once upstream rejects the primary, its persisted rate limit takes over.
	until := time.Now().Add(time.Hour)
	parent.RateLimitResetAt = &until
	selection, _, err = svc.SelectAccountWithScheduler(context.Background(), req.GroupID, "", "", req.RequestedModel, map[int64]struct{}{1: {}}, req.RequiredTransport, false)
	require.NoError(t, err)
	require.EqualValues(t, 3, selection.Account.ID)
	selection.ReleaseFunc()
}

func TestJackReserveAdminUpdatesPersistAndPreservePlan(t *testing.T) {
	_, repo, _ := reserveFixture()
	admin := &adminServiceImpl{accountRepo: repo}
	ctx := context.Background()
	updated, err := admin.UpdateAccount(ctx, 1, &UpdateAccountInput{Extra: map[string]any{"unrelated_setting": true}})
	require.NoError(t, err)
	config, err := jackreserve.Parse(updated.Extra)
	require.NoError(t, err)
	require.True(t, config.Enabled)
	require.Equal(t, []int64{3, 2}, config.AccountIDs)
	require.NoError(t, admin.UpdateAccountExtra(ctx, 1, map[string]any{jackreserve.AccountsKey: []int64{2, 3}}))
	config, err = jackreserve.Parse(repo.accounts[0].Extra)
	require.NoError(t, err)
	require.Equal(t, []int64{2, 3}, config.AccountIDs)
	err = admin.UpdateAccountExtra(ctx, 1, map[string]any{jackreserve.AccountsKey: []int64{1}})
	require.Error(t, err)
	config, err = jackreserve.Parse(repo.accounts[0].Extra)
	require.NoError(t, err)
	require.Equal(t, []int64{2, 3}, config.AccountIDs, "invalid update must not persist")
}

func TestJackReserveQuotaPauseVersusTransportFailure(t *testing.T) {
	for _, quotaPause := range []bool{false, true} {
		svc, repo, req := reserveFixture()
		parent := &repo.accounts[0]
		parent.RateLimitResetAt = nil
		until := time.Now().Add(time.Hour)
		parent.TempUnschedulableUntil = &until
		if quotaPause {
			parent.TempUnschedulableReason = BuildDetailedAccountSchedulingThresholdReason(AccountSchedulingThresholdReasonInput{Platform: PlatformOpenAI, Window: "7d", ThresholdPercent: 95, UsedPercent: 96, Until: until})
		} else {
			parent.TempUnschedulableReason = BuildTempUnschedReasonPayload("upstream_transport", "proxy unavailable")
		}
		selection, _, err := svc.selectJackReserveEnergy(context.Background(), req)
		require.NoError(t, err)
		if quotaPause {
			require.NotNil(t, selection)
			require.EqualValues(t, 3, selection.Account.ID)
			selection.ReleaseFunc()
		} else {
			require.Nil(t, selection)
		}
	}
}

func TestJackReservePinnedContinuationKeepsItsAuthorizedOwner(t *testing.T) {
	svc, repo, req := reserveFixture()
	req.PreviousResponseID = "resp_reserve_owner"
	req.PreviousResponseCanMove = false
	ctx := context.Background()
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, *req.GroupID, req.PreviousResponseID, 2, time.Hour))
	for _, recovered := range []bool{false, true} {
		if recovered {
			repo.accounts[0].RateLimitResetAt = nil
		}
		selection, _, err := svc.selectJackReserveEnergy(ctx, req)
		require.NoError(t, err)
		require.EqualValues(t, 2, selection.Account.ID, "a pinned continuation cannot move to primary or the first reserve")
		require.Equal(t, 1, selection.JackReserveRemaining)
		selection.ReleaseFunc()
	}
	req.ExcludedIDs = map[int64]struct{}{2: {}}
	selection, _, err := svc.selectJackReserveEnergy(ctx, req)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, selection, "never replay a pinned continuation through reserve 1")
	req.ExcludedIDs = nil
	repo.accounts[0].Schedulable = false
	selection, _, err = svc.selectJackReserveEnergy(ctx, req)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, selection, "disabling the primary revokes continuation delegation")
	otherGroup := int64(88)
	req.GroupID = &otherGroup
	selection, _, err = svc.selectJackReserveEnergy(ctx, req)
	require.NoError(t, err)
	require.Nil(t, selection, "response ownership is group scoped")
}
