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
		return "Request ended"
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
	p.scrub(value, p.Requested())
	out, err := json.Marshal(value)
	if err != nil {
		return []byte(GenericError)
	}
	return out
}

// HTTP failures may use non-OpenAI shapes (detail, plain text, HTML, etc.).
// Normalize the complete envelope so no provider-specific text escapes in them.
func (p *Policy) ErrorJSON(body []byte) []byte {
	if !p.Enabled() || p.IsAdmin() {
		return body
	}
	var value any
	_ = json.Unmarshal(body, &value)
	if node, ok := value.(map[string]any); ok {
		if nested, exists := node["error"]; exists {
			value = nested
		}
	}
	out, _ := json.Marshal(map[string]any{"error": safeError(value)})
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

func privateField(name string) bool {
	if strings.Contains(name, "accountid") {
		return true
	}
	switch name {
	case "account", "accountid", "accountids", "accountname", "chatgptaccountid", "parentaccountid", "parentchatgptaccountid",
		"organizationid", "upstreammodel", "upstreamresponsemodel", "upstreamreasoningeffort", "upstreammodelmismatch",
		"upstreamrequestid", "modelmappingchain", "modelmapping", "reasoningeffortmappings", "maxreasoningeffort",
		"maxreasoningeffortoverlimit", "systemfingerprint":
		return true
	}
	return false
}

func (p *Policy) scrub(value any, requested Requested) {
	switch node := value.(type) {
	case []any:
		for _, child := range node {
			p.scrub(child, requested)
		}
	case map[string]any:
		if typ, _ := node["type"].(string); p.IsGateway() && typ == "error" {
			value := any(node)
			if nested, ok := node["error"]; ok {
				value = nested
			}
			safe := safeError(value)
			for key := range node {
				delete(node, key)
			}
			node["type"] = "error"
			node["error"] = safe
			return
		}
		for key, child := range node {
			name := normalized(key)
			if p.ChatHistory && name == "error" {
				if message, ok := child.(string); ok && message != "" && message != "context_length_exceeded" {
					node[key] = "Request failed"
				}
				continue
			}
			if p.ChatHistory && name == "message" && node["type"] == "error" {
				node[key] = "Request failed"
				continue
			}
			if p.ChatHistory && name == "imageerrors" {
				delete(node, key)
				continue
			}
			if privateField(name) {
				delete(node, key)
				continue
			}
			if p.ErrorHistory && (name == "message" || name == "errorbody") {
				if name == "errorbody" {
					node[key] = GenericError
				} else {
					node[key] = "Request failed"
				}
				continue
			}
			if p.IsGateway() {
				switch name {
				case "model":
					if p.RequestHistory {
						continue
					}
					fallthrough
				case "modelversion", "modelname", "modelid", "basemodel", "actualmodel", "resolvedmodel", "effectivemodel":
					if requested.Model == "" {
						delete(node, key)
					} else {
						node[key] = requested.Model
					}
					continue
				case "reasoningeffort", "effort", "actualeffort", "effectiveeffort":
					if requested.Effort == nil || *requested.Effort == "" {
						delete(node, key)
					} else {
						node[key] = *requested.Effort
					}
					continue
				case "budgettokens", "thinkingbudget", "thinkinglevel":
					delete(node, key)
					continue
				case "reasoning", "outputconfig":
					if settings, ok := child.(map[string]any); ok {
						if _, exists := settings["effort"]; exists {
							if requested.Effort == nil || *requested.Effort == "" {
								delete(settings, "effort")
							} else {
								settings["effort"] = *requested.Effort
							}
						}
					}
				case "error":
					if child != nil {
						node[key] = safeError(child)
					}
					continue
				}
				// Responses error events also have top-level message/code fields.
				if typ, _ := node["type"].(string); typ == "error" {
					if name == "message" {
						node[key] = "Request failed"
						continue
					}
					if name == "code" || name == "param" {
						delete(node, key)
						continue
					}
				}
			}
			// These are user/model content, not routing envelopes. Never rewrite a
			// generated tool argument, schema or text that happens to use "model".
			switch name {
			case "arguments", "parameters", "schema", "jsonschema", "content", "text", "delta", "input", "instructions", "tool", "embedding":
				continue
			}
			p.scrub(child, requested)
		}
	}
}

func safeError(value any) map[string]any {
	out := map[string]any{"type": "upstream_error", "message": "Request failed"}
	if node, ok := value.(map[string]any); ok {
		if typ, _ := node["type"].(string); safeErrorType(typ) {
			out["type"] = typ
		}
		if code, _ := node["code"].(string); safeErrorCode(code) {
			out["code"] = code
		}
	}
	if out["code"] == "context_length_exceeded" || out["type"] == "context_length_exceeded" {
		out["message"] = "context_length_exceeded"
	}
	return out
}
func safeErrorType(v string) bool {
	switch v {
	case "invalid_request_error", "authentication_error", "permission_error", "rate_limit_error", "overloaded_error", "api_error", "upstream_error", "server_error", "not_found_error", "context_length_exceeded":
		return true
	}
	return false
}
func safeErrorCode(v string) bool {
	switch v {
	case "context_length_exceeded", "rate_limit_exceeded", "server_error", "invalid_request_error", "insufficient_quota", "content_policy_violation", "cyber_policy", "unsupported_parameter":
		return true
	}
	return false
}

// Headers is the final downstream boundary, after specialized header writers.
// AdditionalAllowed must not re-enable private headers for an ordinary user.
func (p *Policy) Headers(h http.Header) {
	if !p.Enabled() || p.IsAdmin() {
		return
	}
	for key := range h {
		name := strings.ToLower(key)
		field := normalized(name)
		identity := strings.Contains(field, "accountid") || strings.Contains(field, "accountuuid") || strings.Contains(field, "accountname") || strings.Contains(field, "organizationid") || strings.HasSuffix(field, "account") || strings.HasSuffix(field, "organization")
		model := strings.HasSuffix(field, "model") || strings.HasSuffix(field, "modelname") || strings.HasSuffix(field, "modelid") || strings.HasSuffix(field, "modelversion")
		if identity || model || strings.HasSuffix(field, "effort") || strings.HasSuffix(field, "systemfingerprint") || name == "x-private-route" || name == "x-upstream-route" {
			delete(h, key)
			continue
		}
		// Preserve the gateway's existing header policy. In particular, Codex
		// turn-state, quota windows and unrelated allowed headers remain intact.
	}
}

func (p *Policy) SSE(event []byte) []byte {
	if !p.Enabled() || p.IsAdmin() {
		return event
	}
	var data []string
	var eventType string
	lines := strings.ReplaceAll(strings.ReplaceAll(string(event), "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(lines, "\n") {
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
	}
	if len(data) == 0 {
		return []byte(": ping\n\n")
	}
	payload := strings.Join(data, "\n")
	if payload != "[DONE]" {
		if eventType == "error" {
			payload = string(p.ErrorJSON([]byte(payload)))
		} else {
			payload = string(p.JSON([]byte(payload)))
		}
	}
	var out strings.Builder
	// Do not relay arbitrary upstream comments, event IDs or custom event names.
	if strings.HasPrefix(eventType, "response.") || strings.HasPrefix(eventType, "message_") || strings.HasPrefix(eventType, "content_block_") || strings.HasPrefix(eventType, "image_generation.") || strings.HasPrefix(eventType, "image_edit.") || eventType == "error" || eventType == "ping" {
		_, _ = out.WriteString("event: ")
		_, _ = out.WriteString(eventType)
		_ = out.WriteByte('\n')
	}
	_, _ = out.WriteString("data: ")
	_, _ = out.WriteString(payload)
	_, _ = out.WriteString("\n\n")
	return []byte(out.String())
}
