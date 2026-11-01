package middleware

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const jackPrivacyMaxResponse = 64 << 20

type privacyReadCloser struct {
	io.Reader
	io.Closer
}

// JackRoutingPrivacy runs outside OpsErrorLogger: ops/billing observe the raw
// response first, while only the final downstream copy is sanitized. Role comes
// exclusively from verified authentication middleware, never request headers.
func JackRoutingPrivacy() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		gateway := jackPrivacyGatewayPath(path)
		model := strings.TrimSpace(c.Query("model"))
		if i := strings.Index(path, "/models/"); gateway && i >= 0 {
			model = strings.SplitN(path[i+len("/models/"):], ":", 2)[0]
			model, _ = url.PathUnescape(model)
		}
		p := jackprivacy.New(gateway, strings.HasPrefix(path, "/api/v1/usage/errors"), c.GetHeader("Content-Type"), model, service.CanonicalRequestedReasoningEffort)
		p.SetProvider("")
		p.RequestHistory = path == "/v1/usage" || path == "/antigravity/v1/usage"
		// These public resource DTOs use the model saved from the original create
		// request, including GET/list/PATCH calls with no model in this request.
		for _, prefix := range []string{"/v1/images/batches", "/v1beta/cachedContents"} {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				p.RequestHistory = true
			}
		}
		p.ChatHistory = strings.HasPrefix(path, "/api/v1/chat/")
		c.Request = c.Request.WithContext(jackprivacy.WithPolicy(c.Request.Context(), p))
		original := c.Writer
		w := &jackPrivacyWriter{ResponseWriter: original, c: c, policy: p, status: http.StatusOK, size: -1}
		if c.Request.Body != nil {
			c.Request.Body = &privacyReadCloser{Reader: io.TeeReader(c.Request.Body, privacyCaptureWriter{w}), Closer: c.Request.Body}
		}
		c.Writer = w
		defer func() { w.finish(); c.Writer = original }()
		c.Next()
	}
}

type privacyCaptureWriter struct{ owner *jackPrivacyWriter }

func (w privacyCaptureWriter) Write(body []byte) (int, error) {
	w.owner.refreshRole()
	if w.owner.policy.CanCapture() {
		return w.owner.policy.Write(body)
	}
	return len(body), nil
}

func jackPrivacyGatewayPath(path string) bool {
	for _, prefix := range []string{"/v1", "/v1beta", "/backend-api/codex", "/antigravity", "/responses", "/chat/completions", "/models", "/embeddings", "/images", "/videos", "/live", "/realtime", "/tts", "/stt", "/custom-voices", "/web_search", "/x_search"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

type jackPrivacyWriter struct {
	gin.ResponseWriter
	c                *gin.Context
	policy           *jackprivacy.Policy
	status, size     int
	mode             string
	buffer           []byte
	scan, lineStart  int
	failed, hijacked bool
	err              error
}

func (w *jackPrivacyWriter) refreshRole() {
	role, ok := GetUserRoleFromContext(w.c)
	w.policy.SetAdmin(ok && role == service.RoleAdmin)
	// All authenticated gateway routes are protected, including aliases added by
	// future upstream syncs. The path list only classifies pre-auth failures.
	platform, _ := w.c.Request.Context().Value(ctxkey.Platform).(string)
	if platform == "" {
		platform, _ = GetForcePlatformFromContext(w.c)
	}
	if platform == "" {
		platform, _ = service.ResolvedTargetPlatformFromContext(w.c.Request.Context())
	}
	if key, ok := GetAPIKeyFromContext(w.c); ok && key != nil {
		w.policy.SetGateway()
		if platform == "" && key.Group != nil {
			platform = key.Group.Platform
		}
	}
	w.policy.SetProvider(platform)
}
func (w *jackPrivacyWriter) Header() http.Header {
	// Archive writers set headers before building their binary body; make the
	// verified role available to their pre-compression privacy hook as well.
	w.refreshRole()
	return w.ResponseWriter.Header()
}
func (w *jackPrivacyWriter) Status() int   { return w.status }
func (w *jackPrivacyWriter) Size() int     { return w.size }
func (w *jackPrivacyWriter) Written() bool { return w.size >= 0 }
func (w *jackPrivacyWriter) WriteHeader(code int) {
	if !w.Written() && code > 0 {
		w.status = code
	}
}
func (w *jackPrivacyWriter) WriteHeaderNow() {
	w.refreshRole()
	w.policy.Headers(w.Header())
	if !w.policy.Enabled() || w.policy.IsAdmin() || w.status == http.StatusSwitchingProtocols || w.status == http.StatusNoContent || w.status == http.StatusNotModified {
		w.ResponseWriter.WriteHeader(w.status)
		w.ResponseWriter.WriteHeaderNow()
	}
	if w.size < 0 {
		w.size = 0
	}
}
func (w *jackPrivacyWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *jackPrivacyWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.refreshRole()
	w.policy.Headers(w.Header())
	w.hijacked = true
	return w.ResponseWriter.Hijack()
}
func (w *jackPrivacyWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }
func (w *jackPrivacyWriter) Write(data []byte) (int, error) {
	w.refreshRole()
	if w.size < 0 {
		w.size = 0
	}
	w.size += len(data)
	if w.policy.IsAdmin() {
		w.ResponseWriter.WriteHeader(w.status)
		return w.ResponseWriter.Write(data)
	}
	if !w.policy.Enabled() {
		w.ResponseWriter.WriteHeader(w.status)
		return w.ResponseWriter.Write(data)
	}
	if w.err != nil {
		return 0, w.err
	}
	if w.mode == "" {
		typ := strings.ToLower(w.Header().Get("Content-Type"))
		switch {
		case strings.Contains(typ, "text/event-stream"):
			w.mode = "sse"
		case strings.Contains(typ, "json") || w.status >= 400:
			w.mode = "json"
		case bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) || bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")):
			w.mode = "json"
		case typ == "" || (w.policy.IsGateway() && strings.HasPrefix(typ, "text/")):
			w.mode = "json"
		default:
			w.mode = "binary"
		}
	}
	if w.mode == "binary" {
		w.policy.Headers(w.Header())
		w.ResponseWriter.WriteHeader(w.status)
		return w.ResponseWriter.Write(data)
	}
	if w.failed {
		return len(data), nil
	}
	if len(data) > jackPrivacyMaxResponse-len(w.buffer) {
		w.failed = true
		w.buffer = nil
		if w.mode == "sse" {
			w.err = w.emit([]byte("event: error\ndata: " + jackprivacy.GenericError + "\n\n"))
		}
		return len(data), w.err
	}
	w.buffer = append(w.buffer, data...)
	if w.mode == "sse" {
		w.consumeEvents()
	}
	return len(data), w.err
}

func (w *jackPrivacyWriter) consumeEvents() {
	for w.scan < len(w.buffer) {
		i := bytes.IndexAny(w.buffer[w.scan:], "\r\n")
		if i < 0 {
			w.scan = len(w.buffer)
			return
		}
		i += w.scan
		end := i + 1
		if w.buffer[i] == '\r' {
			// A CR at a write boundary may be the first byte of CRLF. Wait
			// for the next byte to avoid treating CRLF as two line endings.
			if end == len(w.buffer) {
				w.scan = i
				return
			}
			if w.buffer[end] == '\n' {
				end++
			}
		}
		line := w.buffer[w.lineStart:i]
		w.scan = end
		if len(line) == 0 {
			if w.err = w.emit(w.policy.SSE(w.buffer[:end])); w.err != nil {
				return
			}
			w.buffer = w.buffer[end:]
			w.scan = 0
			w.lineStart = 0
		} else {
			w.lineStart = end
		}
	}
}
func (w *jackPrivacyWriter) emit(data []byte) error {
	w.policy.Headers(w.Header())
	w.Header().Del("Content-Length")
	w.Header().Del("Content-Encoding")
	w.ResponseWriter.WriteHeader(w.status)
	_, err := w.ResponseWriter.Write(data)
	return err
}
func (w *jackPrivacyWriter) Flush() {
	w.refreshRole()
	if w.mode == "" && strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		w.mode = "sse"
	}
	if !w.policy.Enabled() || w.policy.IsAdmin() || w.mode == "binary" || w.mode == "sse" {
		w.policy.Headers(w.Header())
		if w.policy.Enabled() && !w.policy.IsAdmin() && w.mode == "sse" {
			w.Header().Del("Content-Encoding")
			w.Header().Del("Content-Length")
		}
		w.ResponseWriter.WriteHeader(w.status)
		w.ResponseWriter.Flush()
		return
	}
	// JSON keepalive writers emit whitespace while a long image/compact request
	// runs. Whitespace is safe to flush and retains their transport liveness.
	if len(w.buffer) > 0 && len(bytes.TrimSpace(w.buffer)) == 0 {
		_ = w.emit(w.buffer)
		w.buffer = nil
		w.ResponseWriter.Flush()
	}
}
func (w *jackPrivacyWriter) finish() {
	if w.hijacked {
		return
	}
	w.refreshRole()
	if !w.policy.Enabled() || w.policy.IsAdmin() {
		w.ResponseWriter.WriteHeader(w.status)
		return
	}
	w.policy.Headers(w.Header())
	if w.c.Request.Method == http.MethodHead {
		w.ResponseWriter.WriteHeader(w.status)
		return
	}
	if w.mode == "sse" {
		if !w.failed && len(bytes.TrimSpace(w.buffer)) > 0 {
			_ = w.emit(w.policy.SSE(w.buffer))
		}
		return
	}
	if w.mode == "binary" {
		return
	}
	if !w.Written() {
		w.ResponseWriter.WriteHeader(w.status)
		return
	}
	if w.status == http.StatusNoContent || w.status == http.StatusNotModified || w.status == http.StatusSwitchingProtocols {
		return
	}
	body := w.buffer
	encoding := strings.TrimSpace(w.Header().Get("Content-Encoding"))
	if encoding != "" {
		if strings.EqualFold(encoding, "gzip") {
			r, err := gzip.NewReader(bytes.NewReader(body))
			if err == nil {
				body, err = io.ReadAll(io.LimitReader(r, jackPrivacyMaxResponse+1))
				_ = r.Close()
			}
			if err != nil || len(body) > jackPrivacyMaxResponse {
				w.failed = true
			}
		} else {
			w.failed = true
		}
		w.Header().Del("Content-Encoding")
	}
	if w.failed {
		body = []byte(jackprivacy.GenericError)
	} else if w.policy.IsGateway() && w.status >= 400 {
		body = w.policy.ErrorJSON(body)
	} else {
		body = w.policy.JSON(body)
	}
	if json.Valid(body) && (w.status < 400 || w.failed || w.Header().Get("Content-Type") == "") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	_ = w.emit(body)
}
