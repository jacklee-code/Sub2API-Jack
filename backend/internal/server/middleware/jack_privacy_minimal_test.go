package middleware

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestJackPrivacyPlainErrorPreservesStatusContentTypeAndDiagnostics(t *testing.T) {
	const raw = "private-model unavailable for acct-private; retry in 15 seconds"
	for _, role := range []string{service.RoleUser, service.RoleAdmin} {
		router := privacyRouter(role, func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			require.Equal(t, clientRequest, string(body))
			service.SetOpsUpstreamModel(c, "private-model")
			jackprivacy.ObserveUpstreamHeaders(c.Request.Context(), map[string][]string{"Chatgpt-Account-Id": {"acct-private"}})
			c.Data(503, "text/plain; charset=utf-8", []byte(raw))
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(clientRequest)))
		require.Equal(t, 503, rec.Code)
		require.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
		if role == service.RoleAdmin {
			require.Equal(t, raw, rec.Body.String())
		} else {
			require.Equal(t, "[redacted] unavailable for [redacted]; retry in 15 seconds", rec.Body.String())
		}
	}
}
