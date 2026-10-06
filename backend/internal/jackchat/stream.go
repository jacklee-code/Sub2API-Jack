package jackchat

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// captureWriter receives the gateway's response for an in-process dispatch.
// SSE bodies are parsed line by line; other bodies are buffered up to limit.
type captureWriter struct {
	header http.Header
	status int
	limit  int
	body   bytes.Buffer
	line   bytes.Buffer
	onLine func(line string)
	sse    bool
	closed chan bool
}

func newCaptureWriter(limit int, onLine func(string)) *captureWriter {
	return &captureWriter{header: http.Header{}, limit: limit, onLine: onLine, closed: make(chan bool, 1)}
}

func (w *captureWriter) Header() http.Header { return w.header }

func (w *captureWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.sse = strings.HasPrefix(strings.ToLower(w.header.Get("Content-Type")), "text/event-stream")
	}
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.sse && w.onLine != nil && w.status < 300 {
		for _, b := range p {
			if b == '\n' {
				w.onLine(strings.TrimRight(w.line.String(), "\r"))
				w.line.Reset()
				continue
			}
			_ = w.line.WriteByte(b)
		}
		return len(p), nil
	}
	if w.body.Len()+len(p) <= w.limit {
		_, _ = w.body.Write(p)
	}
	return len(p), nil
}

func (w *captureWriter) Flush() {}

// CloseNotify satisfies gin's optional CloseNotifier probe; cancellation is
// carried by the request context instead.
func (w *captureWriter) CloseNotify() <-chan bool { return w.closed }

func (w *captureWriter) finish() {
	if w.sse && w.line.Len() > 0 && w.onLine != nil {
		w.onLine(strings.TrimRight(w.line.String(), "\r"))
		w.line.Reset()
	}
}

func (w *captureWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// gatewayError extracts a readable message from a gateway error body.
func gatewayError(status int, body []byte) string {
	trimmed := bytes.TrimSpace(body)
	for _, path := range []string{"error.message", "message", "error"} {
		if v := gjson.GetBytes(trimmed, path); v.Exists() && v.Type == gjson.String && strings.TrimSpace(v.String()) != "" {
			return strings.TrimSpace(v.String())
		}
	}
	if len(trimmed) > 0 && len(trimmed) < 500 && !bytes.HasPrefix(trimmed, []byte("{")) {
		return string(trimmed)
	}
	return http.StatusText(status)
}

// ResponsesEvent is one parsed event of an OpenAI Responses stream.
type ResponsesEvent struct {
	Text         string
	Reasoning    string
	Search       string // "searching" or "done" for web_search calls
	Query        string
	Citations    []Citation
	Done         bool
	Error        string
	InputTokens  int
	OutputTokens int
}

// responsesParser turns SSE lines into ResponsesEvents.
type responsesParser struct {
	gotText  bool
	gotCites bool
}

func (p *responsesParser) line(line string) (ResponsesEvent, bool) {
	if !strings.HasPrefix(line, "data:") {
		return ResponsesEvent{}, false
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" || data == "[DONE]" {
		return ResponsesEvent{}, false
	}
	switch gjson.Get(data, "type").String() {
	case "response.output_text.delta":
		p.gotText = true
		return ResponsesEvent{Text: gjson.Get(data, "delta").String()}, true
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		return ResponsesEvent{Reasoning: gjson.Get(data, "delta").String()}, true
	case "response.web_search_call.in_progress", "response.web_search_call.searching":
		return ResponsesEvent{Search: "searching"}, true
	case "response.output_item.done":
		if gjson.Get(data, "item.type").String() == "web_search_call" {
			return ResponsesEvent{Search: "done", Query: gjson.Get(data, "item.action.query").String()}, true
		}
	case "response.output_text.annotation.added":
		if c, ok := citation(gjson.Get(data, "annotation")); ok {
			p.gotCites = true
			return ResponsesEvent{Citations: []Citation{c}}, true
		}
	case "response.reasoning_summary_part.done":
		return ResponsesEvent{Reasoning: "\n\n"}, true
	case "response.output_text.done":
		if !p.gotText {
			p.gotText = true
			return ResponsesEvent{Text: gjson.Get(data, "text").String()}, true
		}
	case "response.completed", "response.incomplete":
		ev := ResponsesEvent{
			Done:         true,
			InputTokens:  int(gjson.Get(data, "response.usage.input_tokens").Int()),
			OutputTokens: int(gjson.Get(data, "response.usage.output_tokens").Int()),
		}
		if !p.gotText {
			ev.Text = completedText(gjson.Get(data, "response.output"))
		}
		if !p.gotCites {
			ev.Citations = completedCitations(gjson.Get(data, "response.output"))
		}
		if reason := gjson.Get(data, "response.incomplete_details.reason").String(); reason != "" && ev.Text == "" {
			ev.Error = "incomplete: " + reason
		}
		return ev, true
	case "response.failed":
		msg := gjson.Get(data, "response.error.message").String()
		if msg == "" {
			msg = "response failed"
		}
		return ResponsesEvent{Done: true, Error: msg}, true
	case "error":
		msg := gjson.Get(data, "message").String()
		if msg == "" {
			msg = gjson.Get(data, "error.message").String()
		}
		if msg == "" {
			msg = "upstream error"
		}
		return ResponsesEvent{Done: true, Error: msg}, true
	case "":
		if msg := gjson.Get(data, "error.message").String(); msg != "" {
			return ResponsesEvent{Done: true, Error: msg}, true
		}
	}
	return ResponsesEvent{}, false
}

func completedText(output gjson.Result) string {
	var b strings.Builder
	output.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() != "message" {
			return true
		}
		item.Get("content").ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "output_text" {
				_, _ = b.WriteString(part.Get("text").String())
			}
			return true
		})
		return true
	})
	return b.String()
}

func citation(a gjson.Result) (Citation, bool) {
	if a.Get("type").String() != "url_citation" {
		return Citation{}, false
	}
	url := strings.TrimSpace(a.Get("url").String())
	if url == "" {
		return Citation{}, false
	}
	return Citation{URL: url, Title: strings.TrimSpace(a.Get("title").String())}, true
}

func completedCitations(output gjson.Result) []Citation {
	var out []Citation
	output.ForEach(func(_, item gjson.Result) bool {
		item.Get("content").ForEach(func(_, part gjson.Result) bool {
			part.Get("annotations").ForEach(func(_, a gjson.Result) bool {
				if c, ok := citation(a); ok {
					out = append(out, c)
				}
				return true
			})
			return true
		})
		return true
	})
	return out
}

// addCitations appends new sources, keeping the first title seen per URL.
func addCitations(list []Citation, add []Citation) []Citation {
	for _, c := range add {
		dup := false
		for _, have := range list {
			if have.URL == c.URL {
				dup = true
				break
			}
		}
		if !dup {
			list = append(list, c)
		}
	}
	return list
}
