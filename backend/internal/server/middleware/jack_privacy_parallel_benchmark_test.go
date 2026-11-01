package middleware

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// With -cpu=4 this runs 32 concurrent requests. It measures local processing,
// not model generation, network latency or production VPS capacity.
func BenchmarkRoutingPrivacyConcurrent(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	for _, stream := range []bool{false, true} {
		for _, mode := range []string{"baseline", "openai", "anthropic"} {
			b.Run(fmt.Sprintf("stream=%t/%s", stream, mode), func(b *testing.B) {
				payload := []byte(`{"model":"private-model","account_id":"provider-account","content":"` + strings.Repeat("a", 1024) + `"}`)
				event := []byte("data: " + `{"type":"response.output_text.delta","model":"private-model","account_id":"provider-account","delta":"hello"}` + "\n\n")
				router := gin.New()
				if mode != "baseline" {
					router.Use(JackRoutingPrivacy())
				}
				platform := service.PlatformOpenAI
				if mode == "anthropic" {
					platform = service.PlatformAnthropic
				}
				router.POST("/v1/responses", func(c *gin.Context) {
					c.Set(string(ContextKeyUserRole), service.RoleUser)
					c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{Platform: platform}})
					if _, err := httputil.ReadRequestBodyWithPrealloc(c.Request); err != nil {
						panic(err)
					}
					if stream {
						c.Header("Content-Type", "text/event-stream")
						for j := 0; j < 100; j++ {
							if _, err := c.Writer.Write(event); err != nil {
								panic(err)
							}
							c.Writer.Flush()
						}
					} else {
						c.Data(200, "application/json", payload)
					}
				})
				b.ReportAllocs()
				b.SetParallelism(8)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"public-model","reasoning":{"effort":"xhigh"}}`))
						req.Header.Set("Content-Type", "application/json")
						rec := httptest.NewRecorder()
						router.ServeHTTP(rec, req)
						if rec.Code != 200 || rec.Body.Len() == 0 {
							panic("invalid benchmark response")
						}
					}
				})
			})
		}
	}
}
