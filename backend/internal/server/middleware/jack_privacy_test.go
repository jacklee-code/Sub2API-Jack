package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const privateResponse = `{"id":"resp_public","model":"private-snapshot","account_id":"provider-account","chatgpt_account_id":"provider-uuid","reasoning":{"effort":"low","summary":"auto"},"usage":{"input_tokens":9007199254740993},"output":[{"type":"function_call","arguments":"{\"model\":\"keep-user-data\"}"}]}`
const clientRequest = `{"model":"public-model","reasoning":{"effort":"xhigh"},"input":"hi"}`

func privacyRouter(role string, output gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JackRoutingPrivacy())
	r.Use(func(c *gin.Context) { c.Set(string(ContextKeyUserRole), role); c.Next() })
	r.POST("/v1/responses", output)
	return r
}

func TestJackPrivacyJSONHeadersAndAdminEvidence(t *testing.T) {
	for _, role := range []string{service.RoleUser, service.RoleAdmin} {
		t.Run(role, func(t *testing.T) {
			var observed []byte
			r := privacyRouter(role, func(c *gin.Context) {
				_, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				observed = []byte(privateResponse) // Simulate the independent billing/ops observer.
				c.Header("OpenAI-Model", "private-snapshot")
				c.Header("X-Account-Id", "provider-account")
				c.Header("X-Reasoning-Effort", "low")
				c.Header("X-Codex-Primary-Used-Percent", "42")
				c.Header("X-Codex-Turn-State", "opaque-provider-state")
				c.Header("X-Private-Route", "private-snapshot")
				c.Header("X-Reasoning-Included", "true")
				c.Data(200, "application/json", []byte(privateResponse))
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(clientRequest))
			req.Header.Set("Content-Type", "application/json")
			// Neither of these untrusted headers can grant admin visibility.
			req.Header.Set("X-Admin-UI-Request", "true")
			req.Header.Set("X-User-Role", "admin")
			r.ServeHTTP(rec, req)
			require.Equal(t, 200, rec.Code)
			require.JSONEq(t, privateResponse, string(observed))
			if role == service.RoleAdmin {
				require.Equal(t, privateResponse, rec.Body.String())
				require.Equal(t, "private-snapshot", rec.Header().Get("OpenAI-Model"))
				return
			}
			out := rec.Body.String()
			for _, secret := range []string{"private-snapshot", "provider-account", "provider-uuid", "opaque-provider-state"} {
				require.NotContains(t, out, secret)
			}
			require.Equal(t, "public-model", gjson.Get(out, "model").String())
			require.Equal(t, "xhigh", gjson.Get(out, "reasoning.effort").String())
			require.Equal(t, "9007199254740993", gjson.Get(out, "usage.input_tokens").Raw)
			require.Equal(t, `{"model":"keep-user-data"}`, gjson.Get(out, "output.0.arguments").String())
			for _, h := range []string{"OpenAI-Model", "X-Account-Id", "X-Reasoning-Effort", "X-Codex-Primary-Used-Percent", "X-Codex-Turn-State", "X-Private-Route"} {
				require.Empty(t, rec.Header().Get(h), h)
			}
			require.Equal(t, "true", rec.Header().Get("X-Reasoning-Included"))
		})
	}
}

func TestJackPrivacySSEChunksAndFlush(t *testing.T) {
	for _, role := range []string{service.RoleUser, service.RoleAdmin} {
		t.Run(role, func(t *testing.T) {
			stream := "event: response.completed\r\nid: provider-account\r\ndata: {\"type\":\"response.completed\",\"response\":" + privateResponse + "}\r\n\r\n: provider-account private-snapshot\n\ndata: [DONE]\n\n"
			r := privacyRouter(role, func(c *gin.Context) {
				_, _ = io.ReadAll(c.Request.Body)
				c.Header("Content-Type", "text/event-stream")
				for _, b := range []byte(stream) {
					_, err := c.Writer.Write([]byte{b})
					require.NoError(t, err)
					c.Writer.Flush()
				}
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(clientRequest))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(rec, req)
			require.True(t, rec.Flushed)
			if role == service.RoleAdmin {
				require.Equal(t, stream, rec.Body.String())
				return
			}
			out := rec.Body.String()
			require.NotContains(t, out, "private-snapshot")
			require.NotContains(t, out, "provider-account")
			require.NotContains(t, out, `"effort":"low"`)
			require.Contains(t, out, `"model":"public-model"`)
			require.Contains(t, out, `"effort":"xhigh"`)
			require.Contains(t, out, "data: [DONE]\n\n")
		})
	}
}

func TestJackPrivacyErrorsMissingRequestAndEncoding(t *testing.T) {
	for _, tc := range []struct {
		name, body, encoding string
		status               int
	}{
		{"error", `{"error":{"type":"invalid_request_error","message":"private-snapshot rejected effort low for provider-account","param":"private-snapshot","account_id":"provider-account","code":"unsupported_parameter"}}`, "", 400},
		{"malformed", `{"model":"private-snapshot"`, "", 200},
		{"provider-detail", `{"detail":"private-snapshot rejected low effort for provider-account"}`, "", 400},
		{"gzip", privateResponse, "gzip", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := privacyRouter(service.RoleUser, func(c *gin.Context) {
				_, _ = io.ReadAll(c.Request.Body)
				b := []byte(tc.body)
				if tc.encoding != "" {
					var zipped bytes.Buffer
					gz := gzip.NewWriter(&zipped)
					_, _ = gz.Write(b)
					require.NoError(t, gz.Close())
					b = zipped.Bytes()
					c.Header("Content-Encoding", tc.encoding)
				}
				c.Data(tc.status, "application/json", b)
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"input":"no explicit settings"}`))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(rec, req)
			out := rec.Body.String()
			require.True(t, json.Valid(rec.Body.Bytes()))
			require.Empty(t, rec.Header().Get("Content-Encoding"))
			for _, secret := range []string{"private-snapshot", "provider-account", "provider-uuid", `"effort":"low"`} {
				require.NotContains(t, out, secret)
			}
			require.False(t, gjson.Get(out, "model").Exists())
			require.False(t, gjson.Get(out, "reasoning.effort").Exists())
		})
	}
}

func TestJackPrivacyWebSocketMultiTurnAndAdmin(t *testing.T) {
	for _, role := range []string{service.RoleUser, service.RoleAdmin} {
		t.Run(role, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(JackRoutingPrivacy())
			r.GET("/v1/responses", func(c *gin.Context) {
				c.Set(string(ContextKeyUserRole), role)
				conn, err := coderws.Accept(c.Writer, c.Request, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.CloseNow() }()
				for turn := 0; turn < 2; turn++ {
					kind, b, err := conn.Read(c.Request.Context())
					if err != nil {
						return
					}
					jackprivacy.CaptureClientFrame(c.Request.Context(), b)
					if err = conn.Write(c.Request.Context(), kind, jackprivacy.ClientFrame(c.Request.Context(), []byte(privateResponse))); err != nil {
						return
					}
				}
			})
			srv := httptest.NewServer(r)
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			for i, frame := range []string{`{"type":"response.create","model":"first-public","reasoning":{"effort":"xhigh"}}`, `{"type":"response.create","model":"second-public"}`} {
				// Binary JSON frames must receive the same protection as text frames.
				require.NoError(t, conn.Write(ctx, coderws.MessageBinary, []byte(frame)))
				_, body, err := conn.Read(ctx)
				require.NoError(t, err)
				if role == service.RoleAdmin {
					require.Equal(t, privateResponse, string(body))
					continue
				}
				require.NotContains(t, string(body), "private-snapshot")
				require.NotContains(t, string(body), "provider-account")
				if i == 0 {
					require.Equal(t, "first-public", gjson.GetBytes(body, "model").String())
					require.Equal(t, "xhigh", gjson.GetBytes(body, "reasoning.effort").String())
				} else {
					require.Equal(t, "second-public", gjson.GetBytes(body, "model").String())
					require.False(t, gjson.GetBytes(body, "reasoning.effort").Exists())
				}
			}
		})
	}
}

func TestJackPrivacyPanelNestedPoliciesAndErrorHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JackRoutingPrivacy())
	r.GET("/api/v1/usage/errors/1", func(c *gin.Context) {
		c.JSON(200, gin.H{"model": "public-model", "message": "private-snapshot for provider-account", "error_body": privateResponse, "account_id": 73, "group": gin.H{"name": "Public group", "reasoning_effort_mappings": []any{gin.H{"from": "xhigh", "to": "low"}}}})
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/usage/errors/1", nil))
	out := rec.Body.String()
	for _, s := range []string{"private-snapshot", "provider-account", "account_id", "reasoning_effort_mappings"} {
		require.NotContains(t, out, s)
	}
	require.Equal(t, "public-model", gjson.Get(out, "model").String())
	require.Equal(t, "Public group", gjson.Get(out, "group.name").String())
}

func TestJackPrivacyCoversNewGatewayAliasesWithoutPathListChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JackRoutingPrivacy())
	r.POST("/future-upstream-alias", func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{})
		_, _ = io.ReadAll(c.Request.Body)
		c.Data(200, "application/json", []byte(privateResponse))
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/future-upstream-alias", strings.NewReader(clientRequest))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, "public-model", gjson.Get(rec.Body.String(), "model").String())
	require.NotContains(t, rec.Body.String(), "private-snapshot")
	require.NotContains(t, rec.Body.String(), "provider-account")
}

func TestJackPrivacyUsageStatsRetainPublicModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/v1/usage", "/antigravity/v1/usage", "/v1/images/batches/job", "/v1beta/cachedContents/cache"} {
		r := gin.New()
		r.Use(JackRoutingPrivacy())
		r.GET(path, func(c *gin.Context) {
			c.Set(string(ContextKeyAPIKey), &service.APIKey{})
			c.JSON(200, gin.H{"model_stats": []any{gin.H{"model": "public-model", "requests": 2}}, "account_id": 73, "error": gin.H{"message": "private-snapshot for provider-account"}})
		})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		require.Equal(t, "public-model", gjson.Get(rec.Body.String(), "model_stats.0.model").String())
		require.NotContains(t, rec.Body.String(), "account_id")
		require.NotContains(t, rec.Body.String(), "private-snapshot")
		require.NotContains(t, rec.Body.String(), "provider-account")
	}
}

func TestJackPrivacyOldChatErrorsDoNotExposeProviderDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JackRoutingPrivacy())
	r.GET("/api/v1/chat/conversations/1", func(c *gin.Context) {
		c.JSON(200, gin.H{"messages": []any{
			gin.H{"model": "public-model", "content": "Keep model-generated text", "error": "private-model for provider-account"},
			gin.H{"error": "context_length_exceeded"},
		}})
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/chat/conversations/1", nil))
	out := rec.Body.String()
	require.NotContains(t, out, "private-model")
	require.NotContains(t, out, "provider-account")
	require.Contains(t, out, "Keep model-generated text")
	require.Equal(t, "context_length_exceeded", gjson.Get(out, "messages.1.error").String())
}

func TestJackPrivacySSEFlushBeforeFirstEvent(t *testing.T) {
	r := privacyRouter(service.RoleUser, func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Content-Encoding", "gzip") // A stale upstream header after decoding.
		c.Header("X-Account-Id", "provider-account")
		c.Writer.Flush()
		_, err := c.Writer.Write([]byte("data: " + privateResponse + "\n\n"))
		require.NoError(t, err)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(clientRequest))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Empty(t, rec.Result().Header.Get("X-Account-Id"))
	require.Empty(t, rec.Result().Header.Get("Content-Encoding"))
	require.Contains(t, rec.Body.String(), `"model":"public-model"`)
	require.NotContains(t, rec.Body.String(), "private-snapshot")
}

func TestJackPrivacyRoleAvailableBeforeArchiveCompression(t *testing.T) {
	for _, role := range []string{service.RoleUser, service.RoleAdmin} {
		r := privacyRouter(role, func(c *gin.Context) {
			c.Header("Content-Type", "application/zip")
			require.Equal(t, role == service.RoleAdmin, jackprivacy.FromContext(c.Request.Context()).IsAdmin())
			_, err := c.Writer.Write([]byte{0x50, 0x4b, 0x03, 0x04})
			require.NoError(t, err)
		})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/responses", nil))
		require.Equal(t, []byte{0x50, 0x4b, 0x03, 0x04}, rec.Body.Bytes())
	}
}

func TestJackPrivacySSECarriageReturnLines(t *testing.T) {
	r := privacyRouter(service.RoleUser, func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Header("Content-Type", "text/event-stream")
		stream := "event: response.completed\rdata: " + privateResponse + "\r\rdata: [DONE]\r\r"
		for _, b := range []byte(stream) {
			_, err := c.Writer.Write([]byte{b})
			require.NoError(t, err)
			c.Writer.Flush()
		}
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(clientRequest))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.NotContains(t, rec.Body.String(), "private-snapshot")
	require.NotContains(t, rec.Body.String(), "provider-account")
	require.Contains(t, rec.Body.String(), `"model":"public-model"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]\n\n")
}
