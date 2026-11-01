// Package jackprivacy owns the client-visible routing metadata boundary. It must
// never modify the upstream payload used for observation, billing or admin logs.
package jackprivacy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

const MaxRequestPrefix = 1 << 20

type contextKey struct{}

// Requested is deliberately independent of every forwarded/effective value.
type Requested struct {
	Model  string
	Effort *string
}

type EffortParser func([]byte, ...string) *string

type Policy struct {
	gateway        atomic.Bool
	ErrorHistory   bool
	RequestHistory bool // Model values already come from verified requested-model queries.
	ChatHistory    bool
	admin          atomic.Bool
	enabled        atomic.Bool
	capture        atomic.Bool
	mu             sync.Mutex
	prefix         []byte
	httpCaptured   bool
	contentType    string
	requested      Requested
	parseEffort    EffortParser
	secrets        map[string]struct{}
}

func New(gateway, errorHistory bool, contentType, pathModel string, parser EffortParser) *Policy {
	p := &Policy{ErrorHistory: errorHistory, contentType: contentType, requested: Requested{Model: pathModel}, parseEffort: parser}
	p.gateway.Store(gateway)
	p.enabled.Store(true)
	p.capture.Store(true)
	return p
}

func WithPolicy(ctx context.Context, p *Policy) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
func FromContext(ctx context.Context) *Policy {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(contextKey{}).(*Policy)
	return p
}
func (p *Policy) IsGateway() bool     { return p != nil && p.gateway.Load() }
func (p *Policy) SetGateway()         { p.gateway.Store(true) }
func (p *Policy) SetAdmin(admin bool) { p.admin.Store(admin) }
func (p *Policy) IsAdmin() bool       { return p != nil && p.admin.Load() }
func (p *Policy) Enabled() bool       { return p != nil && p.enabled.Load() }
func (p *Policy) CanCapture() bool    { return p != nil && p.capture.Load() }
func (p *Policy) SetProvider(platform string) {
	p.enabled.Store(platform == "openai")
	// Composite routing must retain the ingress before it selects a provider.
	p.capture.Store(platform == "openai" || platform == "composite")
}
func SetProvider(ctx context.Context, platform string) {
	if p := FromContext(ctx); p != nil {
		p.SetProvider(platform)
	}
}

// Write tees only a bounded prefix of the original request as the existing body
// limiter/handler reads it. It does not pre-read uploads or bypass body limits.
func (p *Policy) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.httpCaptured {
		return len(b), nil
	}
	if left := MaxRequestPrefix - len(p.prefix); left > 0 {
		part := b[:min(left, len(b))]
		if needed := len(p.prefix) + len(part); needed > cap(p.prefix) {
			// Grow only for bytes that actually arrived, never Content-Length.
			capacity := min(MaxRequestPrefix, max(needed, max(512, cap(p.prefix)*2)))
			grown := make([]byte, len(p.prefix), capacity)
			copy(grown, p.prefix)
			p.prefix = grown
		}
		p.prefix = append(p.prefix, part...)
	}
	return len(b), nil
}

func (p *Policy) Requested() Requested {
	if p == nil {
		return Requested{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.prefix) > 0 {
		p.captureHTTP(p.prefix)
		p.prefix = nil
	}
	out := p.requested
	if out.Effort != nil {
		v := *out.Effort
		out.Effort = &v
	}
	return out
}

func (p *Policy) captureHTTP(body []byte) {
	media, params, _ := mime.ParseMediaType(p.contentType)
	switch media {
	case "multipart/form-data":
		r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := r.NextPart()
			if err != nil {
				break
			}
			if part.FormName() == "model" {
				value, _ := io.ReadAll(io.LimitReader(part, 256))
				p.requested.Model = strings.TrimSpace(string(value))
				break
			}
			if part.FormName() == "session" {
				value, _ := io.ReadAll(io.LimitReader(part, 65536))
				p.captureJSON(value, false)
				continue
			}
			_, _ = io.Copy(io.Discard, part)
		}
	case "application/x-www-form-urlencoded":
		values, _ := url.ParseQuery(string(body))
		p.requested.Model = strings.TrimSpace(values.Get("model"))
	default:
		p.captureJSON(body, false)
	}
}

// CaptureHTTPRequest receives the first fully decoded ingress body from the
// shared reader, before preread buffers can be replaced by policy/mapped bodies.
// It keeps only metadata, without copying large inputs or retaining uploads.
func CaptureHTTPRequest(ctx context.Context, body []byte) {
	if p := FromContext(ctx); p.CanCapture() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.httpCaptured {
			p.captureHTTP(body)
			p.httpCaptured = true
			p.prefix = nil
		}
	}
}

func (p *Policy) captureJSON(body []byte, followup bool) {
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		model = strings.TrimSpace(gjson.GetBytes(body, "session.model").String())
	}
	if model != "" {
		p.requested.Model = model
	}
	var effort *string
	for _, path := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort", "session.reasoning.effort"} {
		if value := gjson.GetBytes(body, path); value.Exists() {
			v := strings.TrimSpace(value.String())
			effort = &v
			break
		}
	}
	if effort == nil && p.parseEffort != nil {
		effort = p.parseEffort(body, p.requested.Model)
	}
	if !followup {
		p.requested.Effort = effort
		return
	}
	// response.create is a new request, including when effort is omitted. Session
	// updates/continuation frames without settings inherit the selected settings.
	frameType := gjson.GetBytes(body, "type").String()
	if frameType == "response.create" || frameType == "" || effort != nil {
		p.requested.Effort = effort
	}
}

// CaptureClientFrame must run on the original frame, before any policy/model
// rewrite. The mutex protects the relay's concurrent upstream/downstream loops.
func CaptureClientFrame(ctx context.Context, body []byte) {
	p := FromContext(ctx)
	if !p.CanCapture() {
		return
	}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return
	}
	if p != nil {
		p.Requested()
		p.mu.Lock()
		defer p.mu.Unlock()
		p.captureJSON(body, true)
	}
}

func ClientFrame(ctx context.Context, body []byte) []byte {
	p := FromContext(ctx)
	if !p.Enabled() || p.IsAdmin() {
		return body
	}
	return p.JSON(body)
}

// Media sockets can carry raw audio as well as JSON control envelopes. Only
// callers that know the frame is media may use this variant.
func ClientMediaFrame(ctx context.Context, body []byte) []byte {
	p := FromContext(ctx)
	if !p.Enabled() || p.IsAdmin() {
		return body
	}
	if !gjson.ValidBytes(body) {
		return body
	}
	return p.JSON(body)
}

func CloseReason(ctx context.Context, reason string) string {
	if p := FromContext(ctx); p.Enabled() && !p.IsAdmin() && reason != "" {
		reason = strings.ToValidUTF8(p.Text(reason), "�")
		if len(reason) > 120 {
			end := 120
			for !utf8.ValidString(reason[:end]) {
				end--
			}
			reason = reason[:end]
		}
		return reason
	}
	return reason
}

const GenericError = `{"error":{"type":"upstream_error","message":"Request failed"}}`

func (p *Policy) JSON(body []byte) []byte {
	if !p.Enabled() || p.IsAdmin() {
		return body
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return []byte(GenericError)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return []byte(GenericError)
	}
	p.observe(value)
	p.scrub(value, p.Requested())
	out, err := json.Marshal(value)
	if err != nil {
		return []byte(GenericError)
	}
	return out
}

// ErrorJSON retains the upstream error envelope, including custom codes and
// details. Plain-text/HTML errors keep their useful text and content type.
func (p *Policy) ErrorJSON(body []byte) []byte {
	if !p.Enabled() || p.IsAdmin() {
		return body
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return []byte(p.Text(string(body)))
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return []byte(p.Text(string(body)))
	}
	p.observe(value)
	if text, ok := value.(string); ok {
		value = p.Text(text)
	} else {
		p.scrubError(value, p.Requested())
	}
	out, err := json.Marshal(value)
	if err != nil {
		return []byte(GenericError)
	}
	return out
}

func normalized(key string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.':
			return -1
		default:
			return unicode.ToLower(r)
		}
	}, key)
}

func accountField(name string) bool {
	return strings.Contains(name, "accountid") || strings.Contains(name, "accountuuid")
}
func modelField(name string) bool {
	switch name {
	case "model", "modelversion", "modelname", "modelid", "basemodel", "actualmodel", "resolvedmodel", "effectivemodel", "upstreammodel", "upstreamresponsemodel":
		return true
	}
	return false
}
func effortField(name string) bool {
	switch name {
	case "reasoningeffort", "effort", "actualeffort", "effectiveeffort", "upstreamreasoningeffort", "thinkinglevel":
		return true
	}
	return false
}
func privateField(name string) bool {
	return accountField(name) || name == "upstreammodel" || name == "upstreamresponsemodel" || name == "upstreamreasoningeffort" || name == "modelmappingchain" || name == "modelmapping" || name == "reasoningeffortmappings" || name == "maxreasoningeffort" || name == "maxreasoningeffortoverlimit"
}
func contentField(name string) bool {
	switch name {
	case "arguments", "parameters", "schema", "jsonschema", "content", "text", "delta", "input", "instructions", "tool", "embedding":
		return true
	}
	return false
}
func (p *Policy) scrub(value any, requested Requested)      { p.scrubValue(value, requested, false) }
func (p *Policy) scrubError(value any, requested Requested) { p.scrubValue(value, requested, true) }
func (p *Policy) scrubValue(value any, requested Requested, errorText bool) {
	switch node := value.(type) {
	case []any:
		for i, child := range node {
			if str, ok := child.(string); ok && errorText {
				node[i] = p.Text(str)
			} else {
				p.scrubValue(child, requested, errorText)
			}
		}
	case map[string]any:
		if typ, _ := node["type"].(string); typ == "error" {
			errorText = true
		}
		for key, child := range node {
			name := normalized(key)
			if name == "account" {
				if obj, ok := child.(map[string]any); ok {
					delete(obj, "id")
				} else {
					delete(node, key)
					continue
				}
			}
			if privateField(name) {
				delete(node, key)
				continue
			}
			if p.IsGateway() && modelField(name) && !(name == "model" && p.RequestHistory) {
				if requested.Model == "" {
					delete(node, key)
				} else {
					node[key] = requested.Model
				}
				continue
			}
			if p.IsGateway() && effortField(name) {
				if requested.Effort == nil || *requested.Effort == "" {
					delete(node, key)
				} else {
					node[key] = *requested.Effort
				}
				continue
			}
			nestedError := errorText || name == "error"
			if str, ok := child.(string); ok && nestedError {
				if param, ok := node["param"].(string); ok && name == "message" {
					str = p.parameterMessage(str, param)
				}
				node[key] = p.Text(str)
				continue
			}
			if contentField(name) && !nestedError {
				continue
			}
			p.scrubValue(child, requested, nestedError)
		}
	}
}

// Headers removes only account IDs and actual model/effort values. Other
// operational headers retain the existing gateway policy.
func (p *Policy) Headers(h http.Header) {
	if !p.Enabled() || p.IsAdmin() {
		return
	}
	p.ObserveHeaders(h)
	for key, values := range h {
		field := normalized(key)
		if accountField(field) || strings.HasSuffix(field, "model") || strings.HasSuffix(field, "modelname") || strings.HasSuffix(field, "modelid") || strings.HasSuffix(field, "modelversion") || strings.HasSuffix(field, "effort") {
			delete(h, key)
			continue
		}
		// The continuation blob is opaque protocol state, not diagnostic text.
		if strings.EqualFold(key, "x-codex-turn-state") {
			continue
		}
		for i, v := range values {
			values[i] = p.Text(v)
		}
	}
}

func (p *Policy) SSE(event []byte) []byte {
	if !p.Enabled() || p.IsAdmin() {
		return event
	}
	// Keep every non-data field (including id/retry/comments/custom events) and
	// its original line ending. Only reconstruct data when its payload changes.
	type line struct{ text, ending string }
	var lines []line
	var data []string
	eventType := ""
	for rest := string(event); rest != ""; {
		i := strings.IndexAny(rest, "\r\n")
		if i < 0 {
			lines = append(lines, line{rest, ""})
			break
		}
		end := i + 1
		if rest[i] == '\r' && end < len(rest) && rest[end] == '\n' {
			end++
		}
		lines = append(lines, line{rest[:i], rest[i:end]})
		rest = rest[end:]
	}
	for _, l := range lines {
		if l.text == "data" || strings.HasPrefix(l.text, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(l.text, "data:"), " "))
		}
		if strings.HasPrefix(l.text, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(l.text, "event:"))
		}
	}
	payload := strings.Join(data, "\n")
	filtered := payload
	if len(data) > 0 && payload != "[DONE]" {
		if eventType == "error" {
			filtered = string(p.ErrorJSON([]byte(payload)))
		} else if json.Valid([]byte(payload)) {
			filtered = string(p.JSON([]byte(payload)))
		} else {
			filtered = p.Text(payload)
		}
	}
	var out strings.Builder
	emitted := false
	for _, l := range lines {
		if l.text == "data" || strings.HasPrefix(l.text, "data:") {
			if filtered == payload {
				_, _ = out.WriteString(l.text)
				_, _ = out.WriteString(l.ending)
			} else if !emitted {
				_, _ = out.WriteString("data: ")
				_, _ = out.WriteString(strings.ReplaceAll(filtered, "\n", "\ndata: "))
				_, _ = out.WriteString(l.ending)
				emitted = true
			}
		} else {
			// Preserve SSE grammar and redact values, never field names.
			key, value, found := strings.Cut(l.text, ":")
			if found {
				_, _ = out.WriteString(key)
				_ = out.WriteByte(':')
				_, _ = out.WriteString(p.Text(value))
			} else {
				_, _ = out.WriteString(l.text)
			}
			_, _ = out.WriteString(l.ending)
		}
	}
	return []byte(out.String())
}
