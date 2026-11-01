package jackprivacy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestRedactedCloseReasonRetainsWireStatusAndValidUTF8(t *testing.T) {
	p := New(true, false, "application/json", "public-model", nil)
	policyCtx := WithPolicy(context.Background(), p)
	reason := "effort low: " + strings.Repeat("中", 36)
	require.Greater(t, len(p.Text(reason)), 123, "fixture must exceed the wire limit after masking")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_ = conn.Close(websocket.StatusMessageTooBig, CloseReason(policyCtx, reason))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	_, _, err = conn.Read(ctx)
	require.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err))
	var closed websocket.CloseError
	require.True(t, errors.As(err, &closed))
	require.Contains(t, closed.Reason, "effort [redacted]")
	require.True(t, utf8.ValidString(closed.Reason))
	require.LessOrEqual(t, len(closed.Reason), 120)
}
