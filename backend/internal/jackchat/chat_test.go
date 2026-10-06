package jackchat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/jackchatkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestResponsesParserCollectsTextReasoningSearchAndUsage(t *testing.T) {
	p := &responsesParser{}
	var text, reasoning strings.Builder
	var searches []string
	var cites []Citation
	var final ResponsesEvent
	for _, line := range []string{
		"event: response.created",
		`data: {"type":"response.created"}`,
		`data: {"type":"response.web_search_call.searching"}`,
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","action":{"query":"jack weather"}}}`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"Thinking"}`,
		`data: {"type":"response.output_text.delta","delta":"Hel"}`,
		`data: {"type":"response.output_text.delta","delta":"lo"}`,
		`data: {"type":"response.output_text.annotation.added","annotation":{"type":"url_citation","url":"https://example.test/a","title":"A"}}`,
		`data: {"type":"response.output_text.done","text":"Hello"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":3}}}`,
		"data: [DONE]",
	} {
		ev, ok := p.line(line)
		if !ok {
			continue
		}
		_, _ = text.WriteString(ev.Text)
		_, _ = reasoning.WriteString(ev.Reasoning)
		if ev.Search != "" {
			searches = append(searches, ev.Search+":"+ev.Query)
		}
		cites = addCitations(cites, ev.Citations)
		if ev.Done {
			final = ev
		}
	}
	require.Equal(t, "Hello", text.String())
	require.Equal(t, "Thinking", reasoning.String())
	require.Equal(t, []string{"searching:", "done:jack weather"}, searches)
	require.Equal(t, []Citation{{URL: "https://example.test/a", Title: "A"}}, cites)
	require.True(t, final.Done)
	require.Empty(t, final.Error)
	require.Equal(t, 12, final.InputTokens)
	require.Equal(t, 3, final.OutputTokens)
}

func TestResponsesParserFallsBackToCompletedOutputAndErrors(t *testing.T) {
	p := &responsesParser{}
	ev, ok := p.line(`data: {"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"Only final","annotations":[{"type":"url_citation","url":"https://example.test/b"}]}]}]}}`)
	require.True(t, ok)
	require.Equal(t, "Only final", ev.Text)
	require.Equal(t, []Citation{{URL: "https://example.test/b"}}, ev.Citations)

	ev, ok = (&responsesParser{}).line(`data: {"type":"response.failed","response":{"error":{"message":"quota"}}}`)
	require.True(t, ok)
	require.True(t, ev.Done)
	require.Equal(t, "quota", ev.Error)

	ev, ok = (&responsesParser{}).line(`data: {"error":{"message":"upstream down"}}`)
	require.True(t, ok)
	require.Equal(t, "upstream down", ev.Error)
}

func TestCaptureWriterSplitsSSELinesAndBuffersJSON(t *testing.T) {
	var lines []string
	w := newCaptureWriter(1024, func(l string) { lines = append(lines, l) })
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: a\r\n\nda"))
	_, _ = w.Write([]byte("ta: b"))
	w.finish()
	require.Equal(t, []string{"data: a", "", "data: b"}, lines)

	j := newCaptureWriter(1024, func(string) { t.Fatal("JSON must not be parsed as SSE") })
	j.Header().Set("Content-Type", "application/json")
	j.WriteHeader(429)
	_, _ = j.Write([]byte(`{"error":{"message":"Too many"}}`))
	require.Equal(t, 429, j.statusCode())
	require.Equal(t, "Too many", gatewayError(j.statusCode(), j.body.Bytes()))
}

func TestBuildResponsesBodyUsesHistoryOptionsAndTools(t *testing.T) {
	s := &Service{}
	conv := Conversation{Model: "gpt-5", ReasoningEffort: "high"}
	history := []Message{
		{Role: RoleUser, Content: "hi", Attachments: []Attachment{{Kind: KindText, Filename: "a.txt", ExtractedText: "file body"}}},
		{Role: RoleAssistant, Content: "hello", Status: StatusComplete},
		{Role: RoleAssistant, Content: "broken", Status: StatusError},
		{Role: RoleUser, Content: "again"},
	}
	body, err := s.BuildResponsesBody(context.Background(), conv, history, Settings{SystemPrompt: "Be brief."}, true)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "gpt-5", got["model"])
	require.Equal(t, true, got["stream"])
	require.Equal(t, false, got["store"])
	require.Equal(t, "Be brief.", got["instructions"])
	require.Equal(t, map[string]any{"effort": "high", "summary": "auto"}, got["reasoning"])
	require.Equal(t, []any{map[string]any{"type": "web_search"}}, got["tools"])
	require.Equal(t, "jackchat:0", got["prompt_cache_key"], "one cache key per conversation")
	input, ok := got["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3, "error replies are not sent back to the model")
	turn, ok := input[0].(map[string]any)
	require.True(t, ok)
	first, ok := turn["content"].([]any)
	require.True(t, ok)
	part, ok := first[1].(map[string]any)
	require.True(t, ok)
	require.Contains(t, part["text"], "file body")

	body, err = s.BuildResponsesBody(context.Background(), Conversation{Model: "gpt-5"}, history[:1], Settings{}, false)
	require.NoError(t, err)
	require.NotContains(t, string(body), "reasoning")
	require.NotContains(t, string(body), "tools")
	require.NotContains(t, string(body), "instructions")
}

func TestDefaultGroupPrefersAvailableChoiceThenNewest(t *testing.T) {
	groups := []service.Group{{ID: 9, CreatedAt: time.Now()}, {ID: 3, CreatedAt: time.Now().Add(-time.Hour)}}
	pref := int64(3)
	require.Equal(t, int64(3), *DefaultGroup(groups, &pref))
	gone := int64(5)
	require.Equal(t, int64(9), *DefaultGroup(groups, &gone))
	require.Equal(t, int64(9), *DefaultGroup(groups, nil))
	require.Nil(t, DefaultGroup(nil, &pref))
}

func TestModelModesAndImageSizesAreAcceptedUpstream(t *testing.T) {
	require.True(t, IsImageModel("gpt-image-2"))
	require.True(t, IsImageModel(" GPT-Image-1 "))
	require.False(t, IsImageModel("gpt-5.2"))
	for _, aspect := range ImageAspects {
		size, ok := ImageSize(aspect)
		require.True(t, ok, aspect)
		if aspect == "auto" {
			require.Empty(t, size, "auto lets the model choose")
			continue
		}
		var w, h int
		_, err := fmt.Sscanf(size, "%dx%d", &w, &h)
		require.NoError(t, err)
		// gpt-image ignores sizes below about 655k pixels and falls back to auto.
		require.GreaterOrEqual(t, w*h, 655360, aspect)
		require.Zero(t, w%16, aspect)
		require.Zero(t, h%16, aspect)
	}
	_, ok := ImageSize("16:9")
	require.False(t, ok, "1024x576 was below the minimum and is no longer offered")
}

func TestSelectContextKeepsAStableStartAndTheLastTurn(t *testing.T) {
	long := strings.Repeat("a", 4000) // about 1000 tokens
	history := []Message{}
	for i := 1; i <= 20; i++ {
		role := RoleUser
		if i%2 == 0 {
			role = RoleAssistant
		}
		history = append(history, Message{ID: int64(i), Role: role, Content: long})
	}

	kept, start, omitted := selectContext(history, nil, 50000)
	require.Len(t, kept, 20, "fits: everything is sent")
	require.Nil(t, start)
	require.Zero(t, omitted)

	kept, start, omitted = selectContext(history, nil, 10000)
	require.NotNil(t, start, "overflow moves the start")
	require.Equal(t, RoleUser, kept[0].Role, "context starts on a user turn")
	require.Equal(t, int64(20), kept[len(kept)-1].ID)
	require.LessOrEqual(t, len(kept)*1004, 10000*7/10+1004)
	require.Equal(t, 20-len(kept), omitted)

	// The next turn reuses the saved start until it overflows again.
	next := append(history, Message{ID: 21, Role: RoleUser, Content: "short"})
	kept2, start2, omitted2 := selectContext(next, start, 10000)
	require.Nil(t, start2, "no move while it still fits")
	require.Equal(t, *start, kept2[0].ID)
	require.Equal(t, omitted, omitted2)

	// A single huge turn is still sent.
	huge := []Message{{ID: 1, Role: RoleUser, Content: strings.Repeat("b", 400000)}}
	kept, _, omitted = selectContext(huge, nil, 10000)
	require.Len(t, kept, 1)
	require.Zero(t, omitted)
}

func TestEstimateTokensAndFriendlyContextError(t *testing.T) {
	require.Equal(t, 4+25, estimateTextTokens(strings.Repeat("x", 100)))
	require.Equal(t, 4+10, estimateTextTokens("一二三四五六七八九十"))
	m := Message{Content: "hi", Attachments: []Attachment{{Kind: KindImage}, {Kind: KindPDF, Size: 500000}}}
	require.Equal(t, estimateTextTokens("hi")+1500+5000, estimateMessageTokens(m))

	require.Equal(t, ContextFullError, friendlyError("This model's maximum context length is 400000 tokens"))
	require.Equal(t, ContextFullError, friendlyError(`{"code":"context_length_exceeded"}`))
	require.Equal(t, "quota exceeded", friendlyError("quota exceeded"))
}

func TestClassifyAttachments(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	kind, mime, err := Classify("x.bin", png)
	require.NoError(t, err)
	require.Equal(t, KindImage, kind)
	require.Equal(t, "image/png", mime)

	kind, _, err = Classify("doc.pdf", []byte("%PDF-1.7\n"))
	require.NoError(t, err)
	require.Equal(t, KindPDF, kind)

	kind, mime, err = Classify("main.go", []byte("package main\n"))
	require.NoError(t, err)
	require.Equal(t, KindText, kind)
	require.Equal(t, "text/plain", mime)

	_, _, err = Classify("evil.txt", []byte{0xff, 0xfe, 0x00, 0x01})
	require.Error(t, err)
	_, _, err = Classify("a.zip", []byte("PK\x03\x04"))
	require.Error(t, err)
}

func TestTitleAndFilenameAreBounded(t *testing.T) {
	require.Equal(t, "hello world", titleFrom("  hello\n world ", nil))
	require.Equal(t, "a.pdf", titleFrom("", []Attachment{{Filename: "a.pdf"}}))
	long := titleFrom(strings.Repeat("字", 60), nil)
	require.Equal(t, maxTitleRunes+1, len([]rune(long)))
	require.Equal(t, "passwd", sanitizeFilename(`..\..\etc/passwd`))
	require.Equal(t, "ab.txt", sanitizeFilename("a\"b.txt"))
	require.Equal(t, "file", sanitizeFilename(""))
}

func TestChatKeyPrefixAndInternalMarker(t *testing.T) {
	require.True(t, jackchatkey.IsChatKey(jackchatkey.Prefix+"abc"))
	require.False(t, jackchatkey.IsChatKey("sk-normal"))
	require.False(t, jackchatkey.IsInternal(context.Background()))
	require.True(t, jackchatkey.IsInternal(jackchatkey.WithInternal(context.Background())))
}
