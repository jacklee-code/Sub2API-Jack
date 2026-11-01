package jackprivacy

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinimalErrorsKeepDiagnosticsAndOnlyMaskRouting(t *testing.T) {
	p := New(true, false, "application/json", "public-model", nil)
	p.Remember("model", "private-model")
	p.Remember("account_id", "acct-private")
	for _, tc := range []struct{ in, want string }{
		{"trace=prefix_acct-private_suffix; /accounts/acct-private/details", "trace=prefix_[redacted]_suffix; /accounts/[redacted]/details"},
		{"private-model unavailable for acct-private; retry in 30 seconds", "[redacted] unavailable for [redacted]; retry in 30 seconds"},
		{"account_id=unknown-id; model: unseen-model; reasoning.effort=low", "account_id=[redacted]; model: [redacted]; reasoning.effort=[redacted]"},
		{"model `unseen-model` does not support parameter 'temperature'", "model `[redacted]` does not support parameter 'temperature'"},
		{"Invalid budget_tokens: minimum is 1024; high load; request_id=req-42", "Invalid budget_tokens: minimum is 1024; high load; request_id=req-42"},
		{"rejected low effort; retry later", "rejected [redacted] effort; retry later"},
		{"model: public-model; unrelated-private-model-suffix", "model: public-model; unrelated-private-model-suffix"},
	} {
		require.Equal(t, tc.want, p.Text(tc.in), tc.in)
	}
	raw := `{"error":{"type":"custom_provider_error","code":"model_not_found","message":"private-model unavailable for acct-private; retry in 30 seconds","param":"temperature","details":{"budget_tokens":1024,"retry_after":30}},"request_id":"req-42"}`
	want := `{"error":{"type":"custom_provider_error","code":"model_not_found","message":"[redacted] unavailable for [redacted]; retry in 30 seconds","param":"temperature","details":{"budget_tokens":1024,"retry_after":30}},"request_id":"req-42"}`
	require.JSONEq(t, want, string(p.ErrorJSON([]byte(raw))))
	require.Equal(t, "Retry in 30 seconds for [redacted]", CloseReason(WithPolicy(context.Background(), p), "Retry in 30 seconds for private-model"))
	require.JSONEq(t, `{"error":{"param":"reasoning.effort","message":"Unsupported value: '[redacted]'; supported values: 'medium' and 'high'."}}`, string(p.ErrorJSON([]byte(`{"error":{"param":"reasoning.effort","message":"Unsupported value: 'low'; supported values: 'medium' and 'high'."}}`))))
	p.SetAdmin(true)
	require.Equal(t, raw, string(p.ErrorJSON([]byte(raw))))
}

func TestMinimalMetadataPreservesBudgetsAndOperationalFields(t *testing.T) {
	p := New(true, false, "application/json", "public-model", nil)
	CaptureHTTPRequest(WithPolicy(context.Background(), p), []byte(`{"model":"public-model","reasoning":{"effort":"high"},"budget_tokens":4096}`))
	raw := `{"account":{"id":"acct-private","name":"display-name"},"model":"private-model","reasoning":{"effort":"low","budget_tokens":4096},"thinking_budget":512,"thinking_level":"adaptive","system_fingerprint":"fp-1","organization_id":"org-1","upstream_request_id":"up-1","upstream_model_mismatch":true,"content":{"model":"private-model","budget_tokens":99}}`
	want := `{"account":{"name":"display-name"},"model":"public-model","reasoning":{"effort":"high","budget_tokens":4096},"thinking_budget":512,"thinking_level":"high","system_fingerprint":"fp-1","organization_id":"org-1","upstream_request_id":"up-1","upstream_model_mismatch":true,"content":{"model":"private-model","budget_tokens":99}}`
	require.JSONEq(t, want, string(p.JSON([]byte(raw))))
	h := http.Header{"Openai-Model": {"private-model"}, "Chatgpt-Account-Id": {"acct-private"}, "X-System-Fingerprint": {"fp-1"}, "Openai-Organization": {"org-1"}, "X-Codex-Turn-State": {"private-model-opaque-state"}}
	h.Set("X-Codex-Debug-Message", "private-model unavailable; retry later")
	p.Headers(h)
	require.Equal(t, "[redacted] unavailable; retry later", h.Get("X-Codex-Debug-Message"))
	require.Empty(t, h.Get("Openai-Model"))
	require.Empty(t, h.Get("Chatgpt-Account-Id"))
	require.Equal(t, "fp-1", h.Get("X-System-Fingerprint"))
	require.Equal(t, "org-1", h.Get("Openai-Organization"))
	require.Equal(t, "private-model-opaque-state", h.Get("X-Codex-Turn-State"))
	p.SetProvider("anthropic")
	require.Equal(t, raw, string(p.JSON([]byte(raw))))
}

func TestMinimalSSEPreservesFieldsAndRedactsOnlySensitiveValues(t *testing.T) {
	p := New(true, false, "application/json", "public-model", nil)
	safe := "id: evt-123\r\nretry: 2500\r\nevent: custom.progress\r\n: keepalive\r\nx-extension: value\r\ndata: {\"progress\":50}\r\n\r\n"
	require.Equal(t, safe, string(p.SSE([]byte(safe))))
	require.Equal(t, ": keepalive\n\n", string(p.SSE([]byte(": keepalive\n\n"))))
	p.Remember("model", "private-model")
	raw := "id: evt-123\nretry: 2500\nevent: error\n: private-model unavailable\ndata: {\"error\":{\"message\":\"private-model unavailable; retry in 2 seconds\",\"code\":\"custom_code\"}}\n\n"
	out := string(p.SSE([]byte(raw)))
	require.Contains(t, out, "id: evt-123\nretry: 2500\nevent: error\n: [redacted] unavailable\n")
	require.Contains(t, out, `"message":"[redacted] unavailable; retry in 2 seconds"`)
	require.Contains(t, out, `"code":"custom_code"`)
	p.SetAdmin(true)
	require.Equal(t, raw, string(p.SSE([]byte(raw))))
}
