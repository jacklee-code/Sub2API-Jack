package jackchat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Emit sends one event to the browser stream.
type Emit func(event map[string]any)

// SendInput is a new user turn, or a regenerate of the last one.
type SendInput struct {
	Text          string  `json:"text"`
	AttachmentIDs []int64 `json:"attachment_ids"`
	Regenerate    bool    `json:"regenerate"`
	WebSearch     bool    `json:"web_search"`
}

const maxTitleRunes = 40

func titleFrom(text string, atts []Attachment) string {
	t := strings.Join(strings.Fields(text), " ")
	if t == "" && len(atts) > 0 {
		t = atts[0].Filename
	}
	if utf8.RuneCountInString(t) > maxTitleRunes {
		r := []rune(t)
		t = string(r[:maxTitleRunes]) + "…"
	}
	return t
}

// prepareTurn stores the user turn (or drops the last reply on regenerate)
// and returns the conversation history to send.
func (s *Service) prepareTurn(ctx context.Context, userID int64, conv *Conversation, in SendInput, settings Settings) ([]Message, Message, error) {
	var user Message
	if in.Regenerate {
		keys, err := s.Store.DeleteTrailingAssistant(ctx, userID, conv.ID)
		if err != nil {
			return nil, user, err
		}
		s.DeleteObjects(keys)
	} else {
		text := strings.TrimSpace(in.Text)
		if text == "" && len(in.AttachmentIDs) == 0 {
			return nil, user, ErrEmpty
		}
		if len(in.AttachmentIDs) > settings.MaxAttachments {
			return nil, user, ErrBadAttachment
		}
		var err error
		user, err = s.Store.InsertMessage(ctx, Message{ConversationID: conv.ID, Role: RoleUser, Content: text, Status: StatusComplete})
		if err != nil {
			return nil, user, err
		}
		atts, err := s.Store.BindAttachments(ctx, userID, user.ID, in.AttachmentIDs)
		if err != nil {
			return nil, user, err
		}
		user.Attachments = atts
		if conv.Title == "" {
			conv.Title = titleFrom(text, atts)
			if updated, err := s.Store.UpdateConversation(ctx, userID, *conv); err == nil {
				*conv = updated
			}
		}
	}
	history, err := s.Store.Messages(ctx, conv.ID)
	if err != nil {
		return nil, user, err
	}
	if len(history) == 0 || history[len(history)-1].Role != RoleUser {
		return nil, user, ErrEmpty
	}
	if in.Regenerate {
		user = history[len(history)-1]
	}
	return history, user, nil
}

// BuildResponsesBody converts the history into an OpenAI Responses request.
func (s *Service) BuildResponsesBody(ctx context.Context, conv Conversation, history []Message, settings Settings, webSearch bool) ([]byte, error) {
	input := make([]map[string]any, 0, len(history))
	for _, m := range history {
		switch m.Role {
		case RoleUser:
			parts := []map[string]any{}
			if strings.TrimSpace(m.Content) != "" {
				parts = append(parts, map[string]any{"type": "input_text", "text": m.Content})
			}
			for _, a := range m.Attachments {
				part, err := s.attachmentPart(ctx, a, settings)
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
			}
			if len(parts) == 0 {
				continue
			}
			input = append(input, map[string]any{"role": "user", "content": parts})
		case RoleAssistant:
			if strings.TrimSpace(m.Content) == "" || m.Status == StatusError {
				continue
			}
			input = append(input, map[string]any{"role": "assistant", "content": []map[string]any{{"type": "output_text", "text": m.Content}}})
		}
	}
	body := map[string]any{
		"model":  conv.Model,
		"input":  input,
		"stream": true,
		"store":  false,
	}
	if p := strings.TrimSpace(settings.SystemPrompt); p != "" {
		body["instructions"] = p
	}
	if conv.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": conv.ReasoningEffort, "summary": "auto"}
	}
	if webSearch {
		body["tools"] = []map[string]any{{"type": "web_search"}}
	}
	return json.Marshal(body)
}

func (s *Service) attachmentPart(ctx context.Context, a Attachment, settings Settings) (map[string]any, error) {
	switch a.Kind {
	case KindText:
		return map[string]any{"type": "input_text", "text": "File: " + a.Filename + "\n```\n" + a.ExtractedText + "\n```"}, nil
	case KindPDF:
		data, err := s.readObject(ctx, a, settings.MaxPDFBytes)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "input_file", "filename": a.Filename, "file_data": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(data)}, nil
	default:
		data, err := s.readObject(ctx, a, settings.MaxImageBytes*2)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "input_image", "image_url": "data:" + a.Mime + ";base64," + base64.StdEncoding.EncodeToString(data)}, nil
	}
}

// SendChat runs one chat turn and streams the reply through emit.
func (s *Service) SendChat(ctx context.Context, orig *http.Request, userID int64, conv Conversation, in SendInput, emit Emit) error {
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return ErrDisabled
	}
	if conv.Mode != ModeChat || strings.TrimSpace(conv.Model) == "" || IsImageModel(conv.Model) {
		return ErrBadModel
	}
	if _, err := s.Group(ctx, userID, conv.GroupID); err != nil {
		return err
	}
	key, err := s.EnsureKey(ctx, userID, *conv.GroupID)
	if err != nil {
		return err
	}
	history, user, err := s.prepareTurn(ctx, userID, &conv, in, settings)
	if err != nil {
		return err
	}
	body, err := s.BuildResponsesBody(ctx, conv, history, settings, in.WebSearch)
	if err != nil {
		return err
	}
	reply, err := s.Store.InsertMessage(ctx, Message{ConversationID: conv.ID, Role: RoleAssistant, Model: conv.Model, ReasoningEffort: conv.ReasoningEffort, Status: StatusStreaming, WebSearch: in.WebSearch})
	if err != nil {
		return err
	}
	emit(map[string]any{"type": "start", "conversation": conv, "user_message": user, "assistant_message": reply, "regenerate": in.Regenerate})

	var text, reasoning strings.Builder
	parser := &responsesParser{}
	done := false
	streamErr := ""
	w := newCaptureWriter(1<<20, func(line string) {
		ev, ok := parser.line(line)
		if !ok {
			return
		}
		if ev.Text != "" {
			_, _ = text.WriteString(ev.Text)
			emit(map[string]any{"type": "delta", "text": ev.Text})
		}
		if ev.Reasoning != "" {
			_, _ = reasoning.WriteString(ev.Reasoning)
			emit(map[string]any{"type": "reasoning", "text": ev.Reasoning})
		}
		if ev.Search != "" {
			emit(map[string]any{"type": "search", "status": ev.Search, "query": ev.Query})
		}
		if len(ev.Citations) > 0 {
			before := len(reply.Citations)
			reply.Citations = addCitations(reply.Citations, ev.Citations)
			if len(reply.Citations) > before {
				emit(map[string]any{"type": "citations", "citations": reply.Citations})
			}
		}
		if ev.Done {
			done = true
			reply.InputTokens, reply.OutputTokens = ev.InputTokens, ev.OutputTokens
			if ev.Error != "" {
				streamErr = ev.Error
			}
		}
	})
	dispatchErr := s.dispatch(ctx, orig, key, http.MethodPost, "/v1/responses", "application/json", body, w)
	w.finish()

	reply.Content = text.String()
	reply.Reasoning = strings.TrimSpace(reasoning.String())
	switch {
	case dispatchErr != nil:
		reply.Status, reply.Error = StatusError, dispatchErr.Error()
	case w.statusCode() >= 300:
		reply.Status, reply.Error = StatusError, gatewayError(w.statusCode(), w.body.Bytes())
	case ctx.Err() != nil:
		reply.Status = StatusAborted
	case streamErr != "":
		reply.Status, reply.Error = StatusError, streamErr
	case !done:
		reply.Status, reply.Error = StatusError, "the stream ended before the reply finished"
	default:
		reply.Status = StatusComplete
	}
	return s.finishReply(conv.ID, reply, emit)
}

func (s *Service) finishReply(conversationID int64, reply Message, emit Emit) error {
	// The browser may have gone away; save with a fresh context.
	saveCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Store.FinishMessage(saveCtx, reply); err != nil {
		return err
	}
	_ = s.Store.TouchConversation(saveCtx, conversationID)
	if reply.Attachments == nil {
		reply.Attachments = []Attachment{}
	}
	emit(map[string]any{"type": "done", "message": reply})
	return nil
}
