package jackprivacy

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

const Redacted = "[redacted]"

// Labels identify a sensitive value even when an upstream error has no routing
// fields. Bare words such as "high load" and arbitrary UUIDs are not secrets.
var labelledValue = regexp.MustCompile("(?i)\\b((?:chatgpt[-_. ]?)?account(?:[-_. ]?(?:id|uuid))?|(?:upstream[-_. ]|actual[-_. ]|effective[-_. ])?model(?:[-_. ]?(?:id|name|version))?|(?:reasoning[-_. ]|upstream[-_. ]|actual[-_. ]|effective[-_. ])?effort|thinking[-_. ]?level)([\"'`]?\\s*[:=]\\s*[\"'`]?|\\s+[\"'`])([a-z0-9][a-z0-9_.:/@+\\-]*)")
var labelledEffort = regexp.MustCompile(`(?i)(\b(?:reasoning[-_. ]+)?effort\s+(?:is\s+)?)(none|minimal|low|medium|high|xhigh|max|auto)\b`)
var accountUUID = regexp.MustCompile(`(?i)(\baccount\s+)([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})\b`)

var reverseEffort = regexp.MustCompile(`(?i)\b(none|minimal|low|medium|high|xhigh|max|auto)(\s+(?:reasoning\s+)?effort)\b`)
var invalidParameterValue = regexp.MustCompile("(?i)((?:unsupported|invalid|unknown) value\\s*:?\\s*[\"'`])([^\"'`]+)")

func (p *Policy) parameterMessage(message, param string) string {
	name := normalized(param)
	if !modelField(name) && !effortField(name) && !accountField(name) {
		return message
	}
	requested := p.Requested()
	return invalidParameterValue.ReplaceAllStringFunc(message, func(match string) string {
		parts := invalidParameterValue.FindStringSubmatch(match)
		if modelField(name) && parts[2] == requested.Model {
			return match
		}
		if effortField(name) && requested.Effort != nil && parts[2] == *requested.Effort {
			return match
		}
		return parts[1] + Redacted
	})
}

// Remember only records routing metadata, never edits an outgoing request.
func Remember(ctx context.Context, field, value string) {
	if p := FromContext(ctx); p.Enabled() {
		p.Remember(field, value)
	}
}
func (p *Policy) Remember(field, value string) {
	name := normalized(field)
	value = strings.TrimSpace(value)
	if value == "" || value == Redacted || (!accountField(name) && !modelField(name)) {
		return
	}
	if modelField(name) && value == p.Requested().Model {
		return
	}
	// Numeric database account IDs are ambiguous in prose; labelled occurrences
	// are handled below without removing matching token counts or retry seconds.
	if accountField(name) && strings.Trim(value, "0123456789") == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.secrets == nil {
		p.secrets = make(map[string]struct{})
	}
	p.secrets[value] = struct{}{}
}
func ObserveUpstreamHeaders(ctx context.Context, h http.Header) {
	if p := FromContext(ctx); p.Enabled() {
		p.ObserveHeaders(h)
	}
}
func (p *Policy) ObserveHeaders(h http.Header) {
	for key, values := range h {
		name := normalized(key)
		for _, value := range values {
			if accountField(name) {
				p.Remember("account_id", value)
			}
			if strings.HasSuffix(name, "model") || strings.HasSuffix(name, "modelname") || strings.HasSuffix(name, "modelid") || strings.HasSuffix(name, "modelversion") {
				p.Remember("model", value)
			}
		}
	}
}
func (p *Policy) observe(value any) {
	switch node := value.(type) {
	case []any:
		for _, child := range node {
			p.observe(child)
		}
	case map[string]any:
		for key, child := range node {
			name := normalized(key)
			if contentField(name) {
				continue
			}
			if name == "account" {
				if obj, ok := child.(map[string]any); ok {
					if id, ok := obj["id"].(string); ok {
						p.Remember("account_id", id)
					}
				} else if id, ok := child.(string); ok {
					p.Remember("account_id", id)
				}
			}
			if str, ok := child.(string); ok {
				p.Remember(name, str)
			}
			p.observe(child)
		}
	}
}
func tokenByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("_.-", rune(b))
}
func replaceToken(text, secret string) string {
	var out strings.Builder
	for {
		i := strings.Index(text, secret)
		if i < 0 {
			_, _ = out.WriteString(text)
			return out.String()
		}
		end := i + len(secret)
		_, _ = out.WriteString(text[:i])
		if (i == 0 || !tokenByte(text[i-1])) && (end == len(text) || !tokenByte(text[end])) {
			_, _ = out.WriteString(Redacted)
		} else {
			_, _ = out.WriteString(secret)
		}
		text = text[end:]
	}
}
func (p *Policy) Text(text string) string {
	if !p.Enabled() || p.IsAdmin() {
		return text
	}
	p.mu.Lock()
	secrets := make([]string, 0, len(p.secrets))
	for value := range p.secrets {
		secrets = append(secrets, value)
	}
	p.mu.Unlock()
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		if strings.Contains(text, secret) {
			text = replaceToken(text, secret)
		}
	}
	requested := p.Requested()
	text = labelledValue.ReplaceAllStringFunc(text, func(match string) string {
		parts := labelledValue.FindStringSubmatch(match)
		if modelField(normalized(parts[1])) && parts[3] == requested.Model {
			return match
		}
		if effortField(normalized(parts[1])) && requested.Effort != nil && parts[3] == *requested.Effort {
			return match
		}
		return parts[1] + parts[2] + Redacted
	})
	text = labelledEffort.ReplaceAllStringFunc(text, func(match string) string {
		parts := labelledEffort.FindStringSubmatch(match)
		if requested.Effort != nil && parts[2] == *requested.Effort {
			return match
		}
		return parts[1] + Redacted
	})
	text = reverseEffort.ReplaceAllStringFunc(text, func(match string) string {
		parts := reverseEffort.FindStringSubmatch(match)
		if requested.Effort != nil && parts[1] == *requested.Effort {
			return match
		}
		return Redacted + parts[2]
	})
	return accountUUID.ReplaceAllString(text, "${1}"+Redacted)
}
