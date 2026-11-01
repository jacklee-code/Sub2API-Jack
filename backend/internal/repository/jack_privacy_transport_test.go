package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	"github.com/stretchr/testify/require"
)

func TestJackPrivacyTransportObservesIdentityWithoutChangingRequest(t *testing.T) {
	const body = `{"model":"private-model","thinking":{"budget_tokens":4096},"reasoning":{"effort":"low"}}`
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received = string(b)
		if r.Header.Get("Chatgpt-Account-Id") != "acct-real-upstream" {
			t.Error("upstream account header changed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"error":{"message":"acct-real-upstream cannot use private-model; budget_tokens must be at least 1024","code":"custom_validation","param":"budget_tokens"}}`)
	}))
	defer server.Close()
	p := jackprivacy.New(true, false, "application/json", "public-model", nil)
	p.Remember("model", "private-model")
	req, err := http.NewRequestWithContext(jackprivacy.WithPolicy(context.Background(), p), "POST", server.URL, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Chatgpt-Account-Id", "acct-real-upstream")
	resp, err := doUpstreamRequest(server.Client(), req)
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, body, received)
	require.Equal(t, 400, resp.StatusCode)
	sanitized := string(p.ErrorJSON(raw))
	require.NotContains(t, sanitized, "acct-real-upstream")
	require.NotContains(t, sanitized, "private-model")
	require.Contains(t, sanitized, "budget_tokens must be at least 1024")
	require.Contains(t, sanitized, `"code":"custom_validation"`)
	require.Contains(t, sanitized, `"param":"budget_tokens"`)
	p.SetAdmin(true)
	require.Equal(t, raw, p.ErrorJSON(raw))
}
