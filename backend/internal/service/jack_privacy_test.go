package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestJackPrivacyActualWSClientFrameWriter(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(map[bool]string{false: "user", true: "admin"}[admin], func(t *testing.T) {
			raw := []byte(`{"type":"response.completed","response":{"model":"private-snapshot","reasoning":{"effort":"low"},"account_id":73,"output":[]}}`)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.CloseNow() }()
				p := jackprivacy.New(true, false, "application/json", "", CanonicalRequestedReasoningEffort)
				p.SetAdmin(admin)
				ctx := jackprivacy.WithPolicy(r.Context(), p)
				jackprivacy.CaptureClientFrame(ctx, []byte(`{"type":"response.create","model":"client-model","reasoning":{"effort":"xhigh"}}`))
				writer := &openAIWSClientFrameConn{conn: conn, controlCtx: ctx}
				// The relay deliberately uses a background write context. The writer
				// must still use the authenticated connection's privacy context.
				_ = writer.WriteFrame(context.Background(), coderws.MessageBinary, raw)
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			_, body, err := conn.Read(ctx)
			require.NoError(t, err)
			if admin {
				require.Equal(t, raw, body)
				return
			}
			require.Equal(t, "client-model", gjson.GetBytes(body, "response.model").String())
			require.Equal(t, "xhigh", gjson.GetBytes(body, "response.reasoning.effort").String())
			require.NotContains(t, string(body), "private-snapshot")
			require.False(t, gjson.GetBytes(body, "response.account_id").Exists())
		})
	}
}

func TestJackPrivacyUsageKeepsOriginalIngressEvenAfterMapping(t *testing.T) {
	p := jackprivacy.New(true, false, "application/json", "", CanonicalRequestedReasoningEffort)
	_, _ = p.Write([]byte(`{"model":"client-model","reasoning":{"effort":"xhigh"}}`))
	ctx := jackprivacy.WithPolicy(context.Background(), p)
	actual := "low"
	model, effort := jackRequestedUsageFields(ctx, "mapped-model", &actual)
	require.Equal(t, "client-model", model)
	require.Equal(t, "xhigh", *effort)
	p = jackprivacy.New(true, false, "application/json", "", CanonicalRequestedReasoningEffort)
	_, _ = p.Write([]byte(`{"model":"client-model"}`))
	ctx = jackprivacy.WithPolicy(context.Background(), p)
	_, effort = jackRequestedUsageFields(ctx, "mapped-model", &actual)
	require.Nil(t, effort)
}
