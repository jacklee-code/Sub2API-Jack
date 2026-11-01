package jackprivacy

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaAndCloseFrames(t *testing.T) {
	p := New(true, false, "application/json", "public-model", nil)
	ctx := WithPolicy(context.Background(), p)
	audio := []byte{0, 1, 0xff, 3}
	require.Equal(t, audio, ClientMediaFrame(ctx, audio))
	require.NotContains(t, string(ClientMediaFrame(ctx, []byte(`{"model":"private-model","account_id":"provider-id"}`))), "private-model")
	require.Equal(t, "Request ended", CloseReason(ctx, "private-model for provider-id"))
	require.Empty(t, CloseReason(ctx, ""))
	p.SetAdmin(true)
	require.Equal(t, "private-model for provider-id", CloseReason(ctx, "private-model for provider-id"))
}

func TestNormalizedMatchesPreviousRules(t *testing.T) {
	for _, key := range []string{"account_id", "ChatGPT-Account.ID", "modelVersion", "effort", "Ö_Account-ID", "ΑΒC.DEＦ", "模型"} {
		want := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
		require.Equal(t, want, normalized(key))
	}
}

func TestContextLimitHintDoesNotExposeRouting(t *testing.T) {
	p := New(true, false, "application/json", "public-model", nil)
	body := p.ErrorJSON([]byte(`{"error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"private-model limit for provider-account"}}`))
	require.Contains(t, string(body), `"message":"context_length_exceeded"`)
	require.NotContains(t, string(body), "private-model")
	require.NotContains(t, string(body), "provider-account")
}

func TestOriginalPrefixBoundAndEffortPrecedence(t *testing.T) {
	canonical := "canonical-suffix"
	for _, tc := range []struct{ body, want string }{
		{`{"model":"public-model","reasoning":{"effort":"xhigh"}}`, "xhigh"},
		{`{"model":"public-model","reasoning_effort":"high"}`, "high"},
		{`{"session":{"model":"public-model","reasoning":{"effort":"medium"}}}`, "medium"},
		{`{"model":"public-model"}`, canonical},
	} {
		p := New(true, false, "application/json", "", func([]byte, ...string) *string { return &canonical })
		_, err := p.Write([]byte(tc.body))
		require.NoError(t, err)
		require.Equal(t, tc.want, *p.Requested().Effort)
	}
	p := New(true, false, "application/json", "", nil)
	chunk := []byte(strings.Repeat("x", 8192))
	for i := 0; i < 256; i++ {
		_, err := p.Write(chunk)
		require.NoError(t, err)
	}
	require.Len(t, p.prefix, MaxRequestPrefix)
	require.Equal(t, MaxRequestPrefix, cap(p.prefix))
}
