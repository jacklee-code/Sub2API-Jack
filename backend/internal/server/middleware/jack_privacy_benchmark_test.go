package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var privacyBenchmarkSink []byte

func privacyBenchmarkPolicy() *jackprivacy.Policy {
	p := jackprivacy.New(true, false, "application/json", "", service.CanonicalRequestedReasoningEffort)
	jackprivacy.CaptureHTTPRequest(jackprivacy.WithPolicy(context.Background(), p), []byte(`{"model":"public-model","reasoning":{"effort":"xhigh"}}`))
	return p
}

func BenchmarkRoutingPrivacyJSON(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	for _, size := range []int{1024, 64 << 10, 1 << 20, 8 << 20} {
		payload := []byte(`{"model":"private-model","account_id":"provider-account","reasoning":{"effort":"low"},"output":[{"type":"message","content":[{"type":"output_text","text":"` + strings.Repeat("a", size) + `"}]}]}`)
		for _, enabled := range []bool{false, true} {
			b.Run(fmt.Sprintf("%dKiB/privacy=%t", size>>10, enabled), func(b *testing.B) {
				router := gin.New()
				if enabled {
					router.Use(JackRoutingPrivacy())
				}
				router.POST("/v1/responses", func(c *gin.Context) {
					c.Set(string(ContextKeyUserRole), service.RoleUser)
					c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI}})
					_, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
					if err != nil {
						panic(err)
					}
					c.Data(200, "application/json", payload)
				})
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"public-model","reasoning":{"effort":"xhigh"}}`))
					req.Header.Set("Content-Type", "application/json")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					privacyBenchmarkSink = rec.Body.Bytes()
				}
			})
		}
	}
}

func BenchmarkRoutingPrivacySSE(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	const events = 100
	event := []byte("data: " + `{"type":"response.output_text.delta","response_id":"resp_id","output_index":0,"content_index":0,"delta":"hello","model":"private-model","account_id":"provider-account"}` + "\n\n")
	for _, enabled := range []bool{false, true} {
		b.Run(fmt.Sprintf("100events/privacy=%t", enabled), func(b *testing.B) {
			router := gin.New()
			if enabled {
				router.Use(JackRoutingPrivacy())
			}
			router.POST("/v1/responses", func(c *gin.Context) {
				c.Set(string(ContextKeyUserRole), service.RoleUser)
				c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI}})
				_, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
				if err != nil {
					panic(err)
				}
				c.Header("Content-Type", "text/event-stream")
				for j := 0; j < events; j++ {
					if _, err := c.Writer.Write(event); err != nil {
						panic(err)
					}
					c.Writer.Flush()
				}
			})
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"public-model","reasoning":{"effort":"xhigh"}}`))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				privacyBenchmarkSink = rec.Body.Bytes()
			}
			b.StopTimer()
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*events), "ns/event")
		})
	}
}

func BenchmarkRoutingPrivacyWSFrame(b *testing.B) {
	for _, size := range []int{256, 64 << 10} {
		payload := []byte(`{"type":"response.completed","response":{"model":"private-model","reasoning":{"effort":"low"},"output":[{"content":[{"text":"` + strings.Repeat("a", size) + `"}]}]}}`)
		ctx := jackprivacy.WithPolicy(context.Background(), privacyBenchmarkPolicy())
		b.Run(fmt.Sprintf("%dbytes", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for i := 0; i < b.N; i++ {
				privacyBenchmarkSink = jackprivacy.ClientFrame(ctx, payload)
			}
		})
	}
}

func BenchmarkRoutingPrivacyIngress(b *testing.B) {
	for _, size := range []int{1024, 1 << 20, 8 << 20} {
		body := []byte(`{"input":"` + strings.Repeat("a", size) + `","model":"public-model","reasoning":{"effort":"xhigh"}}`)
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				p := jackprivacy.New(true, false, "application/json", "", service.CanonicalRequestedReasoningEffort)
				ctx := jackprivacy.WithPolicy(context.Background(), p)
				if _, err := io.Copy(io.Discard, io.TeeReader(bytes.NewReader(body), p)); err != nil {
					panic(err)
				}
				jackprivacy.CaptureHTTPRequest(ctx, body)
			}
		})
	}
}
