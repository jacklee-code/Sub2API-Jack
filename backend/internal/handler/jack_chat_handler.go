package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackchat"
	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// JackChatHandler serves Jack's web chat mode. Model calls are dispatched
// in-process to the /v1 gateway with hidden per-(user, group) keys.
type JackChatHandler struct {
	svc    *jackchat.Service
	cancel context.CancelFunc
	done   chan struct{}
}

func NewJackChatHandler(db *sql.DB, apiKeys *service.APIKeyService, keyRepo service.APIKeyRepository, users service.UserRepository, storage *service.ImageStorageSettingService) *JackChatHandler {
	h := &JackChatHandler{svc: jackchat.NewService(jackchat.NewStore(db), apiKeys, keyRepo, users, storage), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		defer close(h.done)
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, stop := context.WithTimeout(ctx, time.Minute)
				h.svc.CleanupPending(attempt)
				stop()
			}
		}
	}()
	return h
}

// Stop ends the background cleanup.
func (h *JackChatHandler) Stop() { h.cancel(); <-h.done }

// SetEngine wires the router used for in-process gateway dispatch.
func (h *JackChatHandler) SetEngine(e jackchat.Engine) { h.svc.SetEngine(e) }

func chatUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

func chatParamID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid id")
		return 0, false
	}
	return id, true
}

type chatGroup struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	RateMultiplier   float64 `json:"rate_multiplier"`
	SubscriptionType string  `json:"subscription_type"`
}

// Config returns everything the chat page needs to start.
// GET /api/v1/chat/config
func (h *JackChatHandler) Config(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	settings, err := h.svc.Store.Settings(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	pref, err := h.svc.Store.Preference(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groups, err := h.svc.Groups(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]chatGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, chatGroup{ID: g.ID, Name: g.Name, Description: g.Description, RateMultiplier: g.RateMultiplier, SubscriptionType: g.SubscriptionType})
	}
	pref.GroupID = jackchat.DefaultGroup(groups, pref.GroupID)
	_, _, storageOK := h.svc.ObjectStore()
	response.Success(c, gin.H{
		"enabled":           settings.Enabled,
		"groups":            out,
		"preference":        pref,
		"reasoning_efforts": jackchat.ReasoningEfforts,
		"image_aspects":     jackchat.ImageAspects,
		"max_image_count":   jackchat.MaxImageCount,
		"storage_available": storageOK,
		"limits": gin.H{
			"max_image_bytes": settings.MaxImageBytes,
			"max_pdf_bytes":   settings.MaxPDFBytes,
			"max_text_bytes":  settings.MaxTextBytes,
			"max_attachments": settings.MaxAttachments,
		},
	})
}

// Models lists a group's models for one mode.
// GET /api/v1/chat/groups/:id/models?mode=chat|image
func (h *JackChatHandler) Models(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	groupID, ok := chatParamID(c, "id")
	if !ok {
		return
	}
	mode := c.DefaultQuery("mode", jackchat.ModeChat)
	if mode != jackchat.ModeChat && mode != jackchat.ModeImage {
		response.BadRequest(c, "Invalid mode")
		return
	}
	if _, err := h.svc.Group(c.Request.Context(), userID, &groupID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	models, err := h.svc.Models(c.Request.Context(), jackchat.ClientFrom(c.Request), userID, groupID, mode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, models)
}

// UpdatePreference merges changes into the user's chat defaults.
// PUT /api/v1/chat/preferences
func (h *JackChatHandler) UpdatePreference(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	var in map[string]json.RawMessage
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	pref, err := h.svc.Store.Preference(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if raw, ok := in["web_search"]; ok {
		_ = json.Unmarshal(raw, &pref.WebSearch)
	}
	if err := h.svc.Store.SavePreference(c.Request.Context(), userID, pref); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pref)
}

// ListConversations returns the user's conversations, newest first.
// GET /api/v1/chat/conversations
func (h *JackChatHandler) ListConversations(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	list, err := h.svc.Store.ListConversations(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, list)
}

type conversationPatch struct {
	Title           *string `json:"title"`
	Mode            *string `json:"mode"`
	GroupID         *int64  `json:"group_id"`
	Model           *string `json:"model"`
	ReasoningEffort *string `json:"reasoning_effort"`
	ImageAspect     *string `json:"image_aspect"`
	ImageCount      *int    `json:"image_count"`
}

// apply validates the patch onto conv and the user's preference.
func (p conversationPatch) apply(conv *jackchat.Conversation, pref *jackchat.Preference) error {
	if p.Title != nil {
		t := strings.TrimSpace(*p.Title)
		if len([]rune(t)) > 200 {
			t = string([]rune(t)[:200])
		}
		conv.Title = t
	}
	if p.Mode != nil {
		if *p.Mode != jackchat.ModeChat && *p.Mode != jackchat.ModeImage {
			return apperrors.BadRequest("CHAT_MODE_INVALID", "invalid mode")
		}
		conv.Mode = *p.Mode
		pref.Mode = *p.Mode
	}
	if p.GroupID != nil {
		conv.GroupID = p.GroupID
		pref.GroupID = p.GroupID
	}
	if p.Model != nil {
		conv.Model = strings.TrimSpace(*p.Model)
	}
	if p.ReasoningEffort != nil {
		valid := false
		for _, e := range jackchat.ReasoningEfforts {
			valid = valid || e == *p.ReasoningEffort
		}
		if !valid {
			return apperrors.BadRequest("CHAT_EFFORT_INVALID", "invalid reasoning effort")
		}
		conv.ReasoningEffort = *p.ReasoningEffort
		pref.ReasoningEffort = *p.ReasoningEffort
	}
	if p.ImageAspect != nil {
		if _, ok := jackchat.ImageSize(*p.ImageAspect); !ok {
			return apperrors.BadRequest("CHAT_ASPECT_INVALID", "invalid aspect ratio")
		}
		conv.ImageAspect = *p.ImageAspect
		pref.ImageAspect = *p.ImageAspect
	}
	if p.ImageCount != nil {
		if *p.ImageCount < 1 || *p.ImageCount > jackchat.MaxImageCount {
			return apperrors.BadRequest("CHAT_COUNT_INVALID", "image count must be 1-4")
		}
		conv.ImageCount = *p.ImageCount
		pref.ImageCount = *p.ImageCount
	}
	if conv.Model != "" && jackchat.IsImageModel(conv.Model) != (conv.Mode == jackchat.ModeImage) {
		// Switching mode keeps the other mode's model out of this conversation.
		conv.Model = ""
	}
	if conv.Model != "" {
		if conv.Mode == jackchat.ModeImage {
			pref.ImageModel = conv.Model
		} else {
			pref.ChatModel = conv.Model
		}
	}
	return nil
}

// CreateConversation starts a conversation from the user's preference.
// POST /api/v1/chat/conversations
func (h *JackChatHandler) CreateConversation(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	var patch conversationPatch
	if err := c.ShouldBindJSON(&patch); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request")
		return
	}
	ctx := c.Request.Context()
	settings, err := h.svc.Store.Settings(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !settings.Enabled {
		response.ErrorFrom(c, jackchat.ErrDisabled)
		return
	}
	pref, err := h.svc.Store.Preference(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groups, err := h.svc.Groups(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	conv := jackchat.Conversation{Mode: pref.Mode, GroupID: jackchat.DefaultGroup(groups, pref.GroupID), ReasoningEffort: pref.ReasoningEffort, ImageAspect: pref.ImageAspect, ImageCount: pref.ImageCount}
	if conv.Mode != jackchat.ModeImage {
		conv.Mode = jackchat.ModeChat
	}
	if err := patch.apply(&conv, &pref); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if patch.Model == nil {
		if conv.Mode == jackchat.ModeImage {
			conv.Model = pref.ImageModel
		} else {
			conv.Model = pref.ChatModel
		}
	}
	if conv.GroupID != nil {
		if _, err := h.svc.Group(ctx, userID, conv.GroupID); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	if conv.ImageCount < 1 || conv.ImageCount > jackchat.MaxImageCount {
		conv.ImageCount = 1
	}
	if _, ok := jackchat.ImageSize(conv.ImageAspect); !ok {
		conv.ImageAspect = "1:1"
	}
	out, err := h.svc.Store.CreateConversation(ctx, userID, conv, settings.MaxConversations)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	pref.GroupID = conv.GroupID
	_ = h.svc.Store.SavePreference(ctx, userID, pref)
	response.Created(c, out)
}

// UpdateConversation renames a conversation or changes its group, mode,
// model or options; the new choices become the user's defaults.
// PATCH /api/v1/chat/conversations/:id
func (h *JackChatHandler) UpdateConversation(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	id, ok := chatParamID(c, "id")
	if !ok {
		return
	}
	var patch conversationPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	ctx := c.Request.Context()
	conv, err := h.svc.Store.Conversation(ctx, userID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	pref, err := h.svc.Store.Preference(ctx, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := patch.apply(&conv, &pref); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if patch.GroupID != nil {
		if _, err := h.svc.Group(ctx, userID, conv.GroupID); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	out, err := h.svc.Store.UpdateConversation(ctx, userID, conv)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if patch.Title == nil || patch.Mode != nil || patch.GroupID != nil || patch.Model != nil || patch.ReasoningEffort != nil || patch.ImageAspect != nil || patch.ImageCount != nil {
		_ = h.svc.Store.SavePreference(ctx, userID, pref)
	}
	response.Success(c, out)
}

// DeleteConversation removes a conversation, its messages and files.
// Usage logs are kept.
// DELETE /api/v1/chat/conversations/:id
func (h *JackChatHandler) DeleteConversation(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	id, ok := chatParamID(c, "id")
	if !ok {
		return
	}
	keys, err := h.svc.Store.DeleteConversation(c.Request.Context(), userID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.svc.DeleteObjects(keys)
	response.Success(c, gin.H{"deleted": true})
}

// Messages returns a conversation's messages.
// GET /api/v1/chat/conversations/:id/messages
func (h *JackChatHandler) Messages(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	id, ok := chatParamID(c, "id")
	if !ok {
		return
	}
	if _, err := h.svc.Store.Conversation(c.Request.Context(), userID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	msgs, err := h.svc.Store.Messages(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, msgs)
}

// sseStream writes chat events; before the first event an error can still
// be returned as an ordinary JSON response.
type sseStream struct {
	c       *gin.Context
	mu      sync.Mutex
	started bool
}

func (s *sseStream) emit(event map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		s.started = true
		h := s.c.Writer.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no")
		s.c.Status(http.StatusOK)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.c.Writer, "data: %s\n\n", data)
	s.c.Writer.Flush()
}

func (s *sseStream) ping() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return
	}
	_, _ = fmt.Fprint(s.c.Writer, ": ping\n\n")
	s.c.Writer.Flush()
}

func (s *sseStream) fail(err error) {
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		response.ErrorFrom(s.c, err)
		return
	}
	s.emit(map[string]any{"type": "error", "message": apperrors.Message(err)})
}

func (h *JackChatHandler) conversationFor(c *gin.Context) (int64, jackchat.Conversation, bool) {
	userID, ok := chatUserID(c)
	if !ok {
		return 0, jackchat.Conversation{}, false
	}
	id, ok := chatParamID(c, "id")
	if !ok {
		return 0, jackchat.Conversation{}, false
	}
	conv, err := h.svc.Store.Conversation(c.Request.Context(), userID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, jackchat.Conversation{}, false
	}
	return userID, conv, true
}

// SendMessage streams one chat reply.
// POST /api/v1/chat/conversations/:id/messages
func (h *JackChatHandler) SendMessage(c *gin.Context) {
	userID, conv, ok := h.conversationFor(c)
	if !ok {
		return
	}
	var in jackchat.SendInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	if pref, err := h.svc.Store.Preference(c.Request.Context(), userID); err == nil && pref.WebSearch != in.WebSearch {
		pref.WebSearch = in.WebSearch
		_ = h.svc.Store.SavePreference(c.Request.Context(), userID, pref)
	}
	client := jackchat.ClientFrom(c.Request)
	run, err := h.svc.Runs.Start(userID, conv.ID, func(ctx context.Context, emit jackchat.Emit) error {
		return h.svc.SendChat(ctx, client, userID, conv, in, emit)
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.follow(c, run, false)
}

// SendImages streams one image-generation turn.
// POST /api/v1/chat/conversations/:id/images
func (h *JackChatHandler) SendImages(c *gin.Context) {
	userID, conv, ok := h.conversationFor(c)
	if !ok {
		return
	}
	var in jackchat.ImageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	client := jackchat.ClientFrom(c.Request)
	run, err := h.svc.Runs.Start(userID, conv.ID, func(ctx context.Context, emit jackchat.Emit) error {
		return h.svc.SendImages(ctx, client, userID, conv, in, emit)
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.follow(c, run, false)
}

// StreamRun reattaches to a conversation's running reply: a snapshot of the
// reply so far, then live events. Without a run it answers {"active": false}.
// GET /api/v1/chat/conversations/:id/stream
func (h *JackChatHandler) StreamRun(c *gin.Context) {
	userID, conv, ok := h.conversationFor(c)
	if !ok {
		return
	}
	run := h.svc.Runs.Get(userID, conv.ID)
	if run == nil {
		response.Success(c, gin.H{"active": false})
		return
	}
	h.follow(c, run, true)
}

// StopRun stops a conversation's running reply; it is saved as stopped.
// POST /api/v1/chat/conversations/:id/stop
func (h *JackChatHandler) StopRun(c *gin.Context) {
	userID, conv, ok := h.conversationFor(c)
	if !ok {
		return
	}
	run := h.svc.Runs.Get(userID, conv.ID)
	if run != nil {
		run.Cancel()
	}
	response.Success(c, gin.H{"stopped": run != nil})
}

// chatKeepalive keeps proxies from closing a quiet stream, for example while
// an image is generating.
const chatKeepalive = 15 * time.Second

// follow streams a run until it ends or the browser leaves. Leaving only
// detaches; the run keeps going and can be reattached with StreamRun.
func (h *JackChatHandler) follow(c *gin.Context, run *jackchat.Run, snapshot bool) {
	stream := &sseStream{c: c}
	snap, events, detach := run.Subscribe()
	defer detach()
	if snapshot && snap != nil {
		stream.emit(map[string]any{
			"type":           "snapshot",
			"message":        snap.Message,
			"searching":      snap.Searching,
			"search_queries": snap.SearchQueries,
			"pending_images": snap.PendingImages,
			"image_errors":   snap.ImageErrors,
			"aspect":         snap.Aspect,
		})
	}
	ticker := time.NewTicker(chatKeepalive)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			stream.ping()
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev["type"] == "fail" {
				err, _ := ev["error"].(error)
				stream.fail(err)
				return
			}
			stream.emit(ev)
			if ev["type"] == "done" || ev["type"] == "error" {
				return
			}
		}
	}
}

// UploadAttachment stores a file for the next message.
// POST /api/v1/chat/attachments (multipart field "file")
func (h *JackChatHandler) UploadAttachment(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	settings, err := h.svc.Store.Settings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "File is required")
		return
	}
	defer func() { _ = file.Close() }()
	limit := jackchat.MaxUploadBytes(settings)
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		response.BadRequest(c, "Failed to read file")
		return
	}
	if int64(len(data)) > limit {
		response.ErrorFrom(c, jackchat.ErrTooLarge(limit))
		return
	}
	att, err := h.svc.Upload(c.Request.Context(), userID, header.Filename, data)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, att)
}

// DeleteAttachment removes an upload that has not been sent.
// DELETE /api/v1/chat/attachments/:id
func (h *JackChatHandler) DeleteAttachment(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	id, ok := chatParamID(c, "id")
	if !ok {
		return
	}
	keys, err := h.svc.Store.DeletePendingAttachment(c.Request.Context(), userID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.svc.DeleteObjects(keys)
	response.Success(c, gin.H{"deleted": true})
}

// AttachmentContent streams a file the user owns. ?download=1 forces a
// download with the original file name.
// GET /api/v1/chat/attachments/:id/content
func (h *JackChatHandler) AttachmentContent(c *gin.Context) {
	userID, ok := chatUserID(c)
	if !ok {
		return
	}
	id, ok := chatParamID(c, "id")
	if !ok {
		return
	}
	att, err := h.svc.Store.Attachment(c.Request.Context(), userID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	disposition := "inline"
	if c.Query("download") == "1" {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, url.PathEscape(att.Filename)))
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("X-Content-Type-Options", "nosniff")
	if att.Kind == jackchat.KindText {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(att.ExtractedText))
		return
	}
	store, _, ok := h.svc.ObjectStore()
	if !ok {
		response.ErrorFrom(c, jackchat.ErrStorageRequired)
		return
	}
	body, _, size, err := store.Open(c.Request.Context(), att.StorageKey)
	if err != nil {
		response.ErrorFrom(c, apperrors.NotFound("CHAT_FILE_MISSING", "file is no longer available"))
		return
	}
	defer func() { _ = body.Close() }()
	if size <= 0 {
		size = att.Size
	}
	// Serve the recorded type, never the stored object's, so uploads cannot
	// become HTML on this origin.
	c.DataFromReader(http.StatusOK, size, att.Mime, body, nil)
}

// AdminSettings returns chat-mode settings.
// GET /api/v1/admin/chat-settings
func (h *JackChatHandler) AdminSettings(c *gin.Context) {
	s, err := h.svc.Store.Settings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	_, _, storageOK := h.svc.ObjectStore()
	response.Success(c, gin.H{"settings": s, "storage_available": storageOK})
}

// UpdateAdminSettings saves chat-mode settings.
// PUT /api/v1/admin/chat-settings
func (h *JackChatHandler) UpdateAdminSettings(c *gin.Context) {
	var in jackchat.Settings
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	s, err := h.svc.Store.SaveSettings(c.Request.Context(), in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	_, _, storageOK := h.svc.ObjectStore()
	response.Success(c, gin.H{"settings": s, "storage_available": storageOK})
}
