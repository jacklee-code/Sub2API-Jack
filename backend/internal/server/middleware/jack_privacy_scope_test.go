package middleware

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestJackPrivacyNonOpenAIResponsesUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{"anthropic", "gemini", "antigravity", "grok", "kiro", "commandcode", "ollama", "opencode-go"} {
		for _, stream := range []bool{false, true} {
			t.Run(platform+map[bool]string{false: "/json", true: "/sse"}[stream], func(t *testing.T) {
				raw, typ := privateResponse, "application/json"
				if stream {
					raw = "event: message_start\r\nid: provider-account\r\ndata: " + privateResponse + "\r\n\r\n"
					typ = "text/event-stream"
				}
				r := gin.New()
				r.Use(JackRoutingPrivacy())
				r.POST("/v1/responses", func(c *gin.Context) {
					c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{Platform: platform}})
					_, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
					require.NoError(t, err)
					require.False(t, jackprivacy.FromContext(c.Request.Context()).CanCapture())
					c.Header("OpenAI-Model", "private-snapshot")
					c.Header("X-Account-Id", "provider-account")
					c.Header("X-Custom-Trace", "trace")
					c.Data(200, typ, []byte(raw))
					c.Writer.Flush()
				})
				req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(clientRequest))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)
				require.Equal(t, raw, rec.Body.String())
				require.Equal(t, "private-snapshot", rec.Header().Get("OpenAI-Model"))
				require.Equal(t, "provider-account", rec.Header().Get("X-Account-Id"))
				require.Equal(t, "trace", rec.Header().Get("X-Custom-Trace"))
			})
		}
	}
}

func TestJackPrivacyCompositeSelectedProvider(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformAnthropic, service.PlatformGrok} {
		r := gin.New()
		r.Use(JackRoutingPrivacy())
		r.POST("/v1/responses", func(c *gin.Context) {
			c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}})
			_, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
			require.NoError(t, err)
			// Trusted account selection, after ingress and composite model rewriting.
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Platform, platform))
			jackprivacy.SetProvider(c.Request.Context(), platform)
			c.Data(200, "application/json", []byte(privateResponse))
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(clientRequest))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if platform == service.PlatformOpenAI {
			require.Contains(t, rec.Body.String(), `"model":"public-model"`)
			require.NotContains(t, rec.Body.String(), "provider-account")
		} else {
			require.Equal(t, privateResponse, rec.Body.String())
		}
	}
}

func TestJackPrivacyTurnStateRoundtripAndOperationalHeaders(t *testing.T) {
	var received []string
	r := privacyRouter(service.RoleUser, func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		received = append(received, c.GetHeader("X-Codex-Turn-State"))
		c.Header("X-Codex-Turn-State", "opaque-continuation")
		c.Header("X-Codex-Primary-Used-Percent", "42")
		c.Header("X-Custom-Correlation", "safe-trace")
		c.Header("X-Account-Quota", "100")
		c.Header("X-Model-Latency", "20")
		c.Header("X-Account-Id", "provider-account")
		c.Data(200, "application/json", []byte(privateResponse))
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	state := ""
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest("POST", srv.URL+"/v1/responses", strings.NewReader(clientRequest))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Codex-Turn-State", state)
		resp, err := srv.Client().Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		state = resp.Header.Get("X-Codex-Turn-State")
		require.Equal(t, "opaque-continuation", state)
		require.Equal(t, "42", resp.Header.Get("X-Codex-Primary-Used-Percent"))
		require.Equal(t, "safe-trace", resp.Header.Get("X-Custom-Correlation"))
		require.Equal(t, "100", resp.Header.Get("X-Account-Quota"))
		require.Equal(t, "20", resp.Header.Get("X-Model-Latency"))
		require.Empty(t, resp.Header.Get("X-Account-Id"))
		require.NotContains(t, string(body), "private-snapshot")
	}
	require.Equal(t, []string{"", "opaque-continuation"}, received)
}

func TestJackPrivacyNonOpenAIWebSocketUnchanged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := gin.New()
	r.Use(JackRoutingPrivacy())
	r.GET("/v1/responses", func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{Platform: service.PlatformGrok}})
		c.Header("X-Codex-Turn-State", "opaque-handshake")
		conn, err := coderws.Accept(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_, frame, err := conn.Read(c.Request.Context())
		if err != nil {
			return
		}
		jackprivacy.CaptureClientFrame(c.Request.Context(), frame)
		_ = conn.Write(c.Request.Context(), coderws.MessageBinary, jackprivacy.ClientFrame(c.Request.Context(), []byte(privateResponse)))
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	conn, resp, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	require.Equal(t, "opaque-handshake", resp.Header.Get("X-Codex-Turn-State"))
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(clientRequest)))
	_, body, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, privateResponse, string(body))
}

func TestJackPrivacyFirstSSEEventArrivesBeforeAnswerCompletes(t *testing.T) {
	release := make(chan struct{})
	r := privacyRouter(service.RoleUser, func(c *gin.Context) {
		_, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		require.NoError(t, err)
		c.Header("Content-Type", "text/event-stream")
		_, err = c.Writer.Write([]byte("data: " + privateResponse + "\n\n"))
		require.NoError(t, err)
		c.Writer.Flush()
		<-release
		_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
	})
	srv := httptest.NewServer(r)
	defer func() { close(release); srv.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", srv.URL+"/v1/responses", strings.NewReader(clientRequest))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	first, err := bufio.NewReader(resp.Body).ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, first, `"model":"public-model"`)
	require.NotContains(t, first, "provider-account")
}
