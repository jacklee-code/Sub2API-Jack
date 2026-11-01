//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackreserve"
	"github.com/stretchr/testify/require"
)

const jackEmptyCreditBody = `{"error":{"code":"credit_balance_exhausted","type":"insufficient_quota","message":"No credits remain"}}`

type jackCreditRepoStub struct {
	*jackReserveRepoStub
	restoreErr error
	setCalls   int
	clearCalls int
}

func (r *jackCreditRepoStub) SetError(ctx context.Context, id int64, message string) error {
	requireContext := ctx.Err()
	if requireContext != nil {
		return requireContext
	}
	r.setCalls++
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			r.accounts[i].Status = StatusError
			r.accounts[i].ErrorMessage = message
			r.accounts[i].Schedulable = false
			return nil
		}
	}
	return ErrAccountNotFound
}

func (r *jackCreditRepoStub) RestoreJackOpenAICreditExhausted(_ context.Context, id int64) (bool, error) {
	if r.restoreErr != nil {
		return false, r.restoreErr
	}
	for i := range r.accounts {
		if r.accounts[i].ID == id && IsJackOpenAICreditExhausted(&r.accounts[i]) && r.accounts[i].Status == StatusError {
			r.accounts[i].Status = StatusActive
			r.accounts[i].ErrorMessage = ""
			r.accounts[i].Schedulable = true
			return true, nil
		}
	}
	return false, nil
}

func (r *jackCreditRepoStub) ClearError(context.Context, int64) error { r.clearCalls++; return nil }

func TestJackCreditExhaustionStructuredOfficialEvidenceOnly(t *testing.T) {
	base := Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, tc := range []struct {
		name, baseURL, body, platform, kind string
		status                              int
		want                                bool
	}{
		{name: "default official", body: jackEmptyCreditBody, status: 429, want: true},
		{name: "official v1 proxy independent", baseURL: "https://api.openai.com/v1", body: jackEmptyCreditBody, status: 429, want: true},
		{name: "payment", body: jackEmptyCreditBody, status: 402, want: true},
		{name: "failed stream", body: `{"type":"response.failed","response":{"error":{"code":"credit_balance_exhausted"}}}`, status: 429, want: true},
		{name: "error event", body: `{"type":"error","code":"credit_balance_exhausted"}`, status: 429, want: true},
		{name: "legacy quota ambiguous", body: `{"error":{"code":"insufficient_quota","type":"insufficient_quota"}}`, status: 429},
		{name: "project spend", body: `{"error":{"code":"project_spend_limit_exceeded","type":"insufficient_quota"}}`, status: 429},
		{name: "rate limit", body: `{"error":{"code":"rate_limit_exceeded"}}`, status: 429},
		{name: "echoed message", body: `{"error":{"message":"credit_balance_exhausted"}}`, status: 429},
		{name: "echoed input", body: `{"request":{"error":{"code":"credit_balance_exhausted"}}}`, status: 429},
		{name: "plain text", body: "credit_balance_exhausted", status: 429},
		{name: "bad request", body: jackEmptyCreditBody, status: 400},
		{name: "compatible provider", baseURL: "https://provider.example/v1", body: jackEmptyCreditBody, status: 429},
		{name: "lookalike", baseURL: "https://api.openai.com.example/v1", body: jackEmptyCreditBody, status: 429},
		{name: "userinfo", baseURL: "https://user@api.openai.com", body: jackEmptyCreditBody, status: 429},
		{name: "OAuth", kind: AccountTypeOAuth, body: jackEmptyCreditBody, status: 429},
		{name: "other platform", platform: PlatformAnthropic, body: jackEmptyCreditBody, status: 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := base
			a.Credentials = map[string]any{"base_url": tc.baseURL}
			if tc.platform != "" {
				a.Platform = tc.platform
			}
			if tc.kind != "" {
				a.Type = tc.kind
			}
			require.Equal(t, tc.want, isJackOfficialOpenAICreditExhaustion(&a, tc.status, []byte(tc.body)))
		})
	}
}

func TestJackCreditExhaustionSkipsFailedChainUntilManualRecovery(t *testing.T) {
	svc, original, req := reserveFixture()
	for i := 1; i < len(original.accounts); i++ {
		original.accounts[i].Credentials = map[string]any{}
	}
	third := original.accounts[2]
	third.ID = 4
	original.accounts = append(original.accounts, third)
	original.accounts[0].Extra[jackreserve.AccountsKey] = []int64{3, 2, 4}
	repo := &jackCreditRepoStub{jackReserveRepoStub: original}
	svc.accountRepo = repo
	rl := NewRateLimitService(repo, nil, svc.cfg, nil, nil)
	rl.SetAccountRuntimeBlocker(svc)
	svc.rateLimitService = rl
	ctx := context.Background()
	for _, id := range []int64{3, 2} {
		selected, _, err := svc.selectJackReserveEnergy(ctx, req)
		require.NoError(t, err)
		require.Equal(t, id, selected.Account.ID)
		selected.ReleaseFunc()
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		require.True(t, svc.handleOpenAIAccountUpstreamError(cancelled, selected.Account, 429, nil, []byte(jackEmptyCreditBody)))
		failover := svc.newOpenAIAccountFailoverError(selected.Account, 429, nil, []byte(jackEmptyCreditBody), "No credits remain", true, true)
		require.False(t, failover.RetryableOnSameAccount, "empty credits must not retry even in pool mode")
	}
	for range 3 {
		selected, _, err := svc.selectJackReserveEnergy(ctx, req)
		require.NoError(t, err)
		require.EqualValues(t, 4, selected.Account.ID, "new requests skip every exhausted predecessor")
		selected.ReleaseFunc()
	}
	require.Equal(t, 2, repo.setCalls)
	// Losing the process-local cooldown (restart or elapsed time) cannot remove
	// the durable hold. A successful manual/scheduled test is not recovery.
	svc.openaiAccountRuntimeBlockUntil.Store(int64(3), time.Now().Add(-time.Hour))
	for _, options := range []AccountRecoveryOptions{{}, {InvalidateToken: true}} {
		result, err := rl.RecoverAccountState(ctx, 3, options)
		require.NoError(t, err)
		require.False(t, result.ClearedError)
	}
	result, err := rl.RecoverAccountAfterSuccessfulTest(ctx, 3)
	require.NoError(t, err)
	require.False(t, result.ClearedError)
	require.Zero(t, repo.clearCalls)
	repo.restoreErr = errors.New("write unavailable")
	_, err = rl.RecoverAccountState(ctx, 3, AccountRecoveryOptions{RestoreExhaustedCredit: true})
	require.Error(t, err)
	require.True(t, IsJackOpenAICreditExhausted(&repo.accounts[1]))
	repo.restoreErr = nil
	result, err = rl.RecoverAccountState(ctx, 3, AccountRecoveryOptions{RestoreExhaustedCredit: true})
	require.NoError(t, err)
	require.True(t, result.ClearedError)
	require.True(t, repo.accounts[1].Schedulable)
	selected, _, err := svc.selectJackReserveEnergy(ctx, req)
	require.NoError(t, err)
	require.EqualValues(t, 3, selected.Account.ID, "manual recovery returns to the first eligible reserve")
	selected.ReleaseFunc()
}

func TestJackCreditExhaustionStreamAndPolicies(t *testing.T) {
	svc, original, _ := reserveFixture()
	repo := &jackCreditRepoStub{jackReserveRepoStub: original}
	rl := NewRateLimitService(repo, nil, svc.cfg, nil, nil)
	svc.rateLimitService = rl
	account := &original.accounts[1]
	account.Credentials = map[string]any{}
	account.Extra = map[string]any{"pool_mode": true, "custom_error_codes_enabled": true, "custom_error_codes": []any{401}}
	body := []byte(`{"type":"response.failed","response":{"error":{"code":"credit_balance_exhausted","type":"insufficient_quota"}}}`)
	require.Equal(t, http.StatusTooManyRequests, openAIStreamFailureStatus(body, ""))
	failover := svc.newOpenAIStreamFailoverError(nil, account, false, "", body, "")
	require.False(t, failover.RetryableOnSameAccount)
	require.Equal(t, StatusError, original.accounts[1].Status)
	require.False(t, original.accounts[1].Schedulable)
	require.Equal(t, ErrorPolicyMatched, rl.CheckErrorPolicy(context.Background(), account, 429, []byte(jackEmptyCreditBody)))
	// Raw error callers get the same durable policy, before custom rules.
	require.True(t, rl.HandleUpstreamError(context.Background(), account, 429, nil, []byte(jackEmptyCreditBody)))
}

func TestJackCreditExhaustionForwardThenDirectFallbackUsesEachProxy(t *testing.T) {
	svc, original, req := reserveFixture()
	repo := &jackCreditRepoStub{jackReserveRepoStub: original}
	svc.accountRepo = repo
	rl := NewRateLimitService(repo, nil, svc.cfg, nil, nil)
	rl.SetAccountRuntimeBlocker(svc)
	svc.rateLimitService = rl
	for i := range original.accounts {
		a := &original.accounts[i]
		a.Proxy = &Proxy{ID: a.ID, Protocol: "http", Host: "127.0.0.1", Port: 18080 + int(a.ID)}
		a.ProxyID = &a.Proxy.ID
		a.Credentials = map[string]any{"api_key": "sk-fixture-" + a.Name}
	}
	for attempt, wantID := range []int64{3, 2, 2} {
		selected, _, err := svc.selectJackReserveEnergy(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, wantID, selected.Account.ID)
		status, body := http.StatusOK, `{"id":"resp_mock","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`
		if attempt == 0 {
			status, body = http.StatusTooManyRequests, jackEmptyCreditBody
		}
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}}
		svc.httpUpstream = upstream
		c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
		result, err := svc.Forward(context.Background(), c, selected.Account, []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`))
		if attempt == 0 {
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.False(t, failover.RetryableOnSameAccount)
			require.Nil(t, result)
		} else {
			require.NoError(t, err)
			require.NotNil(t, result)
		}
		require.Equal(t, selected.Account.Proxy.URL(), upstream.lastProxyURL)
		require.NotEqual(t, original.accounts[0].Proxy.URL(), upstream.lastProxyURL)
		require.Equal(t, "Bearer "+selected.Account.GetCredential("api_key"), upstream.lastReq.Header.Get("Authorization"))
		selected.ReleaseFunc()
	}
	require.Equal(t, 1, repo.setCalls, "later requests never send to the exhausted first reserve")
}
