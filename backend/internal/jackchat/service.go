package jackchat

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/jackchatkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
)

var (
	ErrGroupUnavailable  = apperrors.Forbidden("CHAT_GROUP_UNAVAILABLE", "this group is not available for chat")
	ErrNoGroup           = apperrors.BadRequest("CHAT_NO_GROUP", "no OpenAI group is available to this account")
	ErrBadModel          = apperrors.BadRequest("CHAT_MODEL_INVALID", "choose a model for this mode")
	ErrStorageRequired   = apperrors.BadRequest("CHAT_STORAGE_UNAVAILABLE", "file storage is not configured; ask the administrator to enable image storage")
	ErrDispatcherMissing = apperrors.InternalServer("CHAT_NOT_READY", "chat dispatcher is not ready")
)

// ReasoningEfforts lists the efforts offered in chat mode. Empty keeps the
// model default; the group's own effort policy still applies upstream.
var ReasoningEfforts = []string{"", "low", "medium", "high", "xhigh"}

// imageSizes maps aspect ratios to the standard gpt-image sizes. Sizes below
// the upstream minimum (about 655k pixels, e.g. 1024x576) are ignored there
// and replaced by "auto", so only sizes the upstream accepts are offered.
// "auto" sends no size and lets the model choose.
var imageSizes = map[string]string{
	"1:1":  "1024x1024",
	"3:2":  "1536x1024",
	"2:3":  "1024x1536",
	"auto": "",
}

// ImageAspects lists the aspect ratios in display order.
var ImageAspects = []string{"1:1", "3:2", "2:3", "auto"}

// DefaultContextTokens is the default estimated token budget for the history
// sent with each chat turn.
const DefaultContextTokens = 200000

// ImageSize returns the request size for an aspect ratio ("" for auto).
func ImageSize(aspect string) (string, bool) {
	size, ok := imageSizes[aspect]
	return size, ok
}

// IsImageModel reports whether model is an image-generation model.
func IsImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image")
}

// Engine serves in-process HTTP requests (the gin engine).
type Engine interface {
	ServeHTTP(w http.ResponseWriter, req *http.Request)
}

type Service struct {
	Store   *Store
	Runs    *Runs
	apiKeys *service.APIKeyService
	keyRepo service.APIKeyRepository
	users   service.UserRepository
	storage *service.ImageStorageSettingService
	engine  atomic.Pointer[engineHolder]
	keyMu   sync.Mutex
}

type engineHolder struct{ Engine }

func NewService(store *Store, apiKeys *service.APIKeyService, keyRepo service.APIKeyRepository, users service.UserRepository, storage *service.ImageStorageSettingService) *Service {
	return &Service{Store: store, Runs: NewRuns(), apiKeys: apiKeys, keyRepo: keyRepo, users: users, storage: storage}
}

// SetEngine wires the router that serves dispatched gateway requests.
func (s *Service) SetEngine(e Engine) { s.engine.Store(&engineHolder{e}) }

// ObjectStore returns the attachment storage, if configured.
func (s *Service) ObjectStore() (service.JackChatObjectStore, string, bool) {
	if s.storage == nil {
		return nil, "", false
	}
	return s.storage.JackChatStore()
}

// Groups returns the user's available OpenAI groups, newest first.
func (s *Service) Groups(ctx context.Context, userID int64) ([]service.Group, error) {
	all, err := s.apiKeys.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]service.Group, 0, len(all))
	for _, g := range all {
		if g.Platform == service.PlatformOpenAI && g.IsActive() {
			out = append(out, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// DefaultGroup keeps the preferred group while it is available, otherwise the
// newest available group.
func DefaultGroup(groups []service.Group, preferred *int64) *int64 {
	if len(groups) == 0 {
		return nil
	}
	if preferred != nil {
		for _, g := range groups {
			if g.ID == *preferred {
				id := g.ID
				return &id
			}
		}
	}
	id := groups[0].ID
	return &id
}

// Group returns the available group with id, or ErrGroupUnavailable.
func (s *Service) Group(ctx context.Context, userID int64, id *int64) (service.Group, error) {
	if id == nil {
		return service.Group{}, ErrNoGroup
	}
	groups, err := s.Groups(ctx, userID)
	if err != nil {
		return service.Group{}, err
	}
	for _, g := range groups {
		if g.ID == *id {
			return g, nil
		}
	}
	return service.Group{}, ErrGroupUnavailable
}

// EnsureKey returns the hidden chat key for (user, group), creating it once.
func (s *Service) EnsureKey(ctx context.Context, userID, groupID int64) (string, error) {
	if key, err := s.findKey(ctx, userID, groupID); err != nil || key != "" {
		return key, err
	}
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if key, err := s.findKey(ctx, userID, groupID); err != nil || key != "" {
		return key, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	gid := groupID
	key := &service.APIKey{
		UserID:  userID,
		Key:     jackchatkey.Prefix + hex.EncodeToString(raw),
		Name:    jackchatkey.Name,
		GroupID: &gid,
		Status:  service.StatusActive,
	}
	if err := s.keyRepo.Create(ctx, key); err != nil {
		return "", fmt.Errorf("create chat key: %w", err)
	}
	s.apiKeys.InvalidateAuthCacheByKey(ctx, key.Key)
	return key.Key, nil
}

func (s *Service) findKey(ctx context.Context, userID, groupID int64) (string, error) {
	var key string
	err := s.Store.DB.QueryRowContext(ctx, `SELECT key FROM api_keys WHERE user_id=$1 AND group_id=$2 AND key LIKE $3 AND deleted_at IS NULL ORDER BY id LIMIT 1`,
		userID, groupID, jackchatkey.Prefix+"%").Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return key, err
}

var forwardedHeaders = []string{"User-Agent", "Accept-Language", "X-Forwarded-For", "X-Real-Ip", "Cf-Connecting-Ip", "True-Client-Ip", "X-Forwarded-Proto"}

// dispatch runs a gateway request in-process with the hidden key, exactly as
// an API client using that key would. Cancelling ctx cancels the request.
func (s *Service) dispatch(ctx context.Context, client Client, key, method, path, contentType string, body []byte, w http.ResponseWriter) error {
	holder := s.engine.Load()
	if holder == nil || holder.Engine == nil {
		return ErrDispatcherMissing
	}
	// A fresh context keeps panel-session values out of the gateway request
	// while still following ctx's cancellation.
	inner, cancel := context.WithCancel(context.Background())
	stop := context.AfterFunc(ctx, cancel)
	defer func() { stop(); cancel() }()
	var reader io.Reader = http.NoBody
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(jackchatkey.WithInternal(inner), method, path, reader)
	if err != nil {
		return err
	}
	req.RemoteAddr = client.RemoteAddr
	req.Host = client.Host
	for h, v := range client.Header {
		req.Header[h] = append([]string(nil), v...)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	holder.ServeHTTP(w, req)
	return nil
}

// Models lists the group's models for mode, as /v1/models returns them to an
// API client with a key in that group.
func (s *Service) Models(ctx context.Context, client Client, userID, groupID int64, mode string) ([]string, error) {
	key, err := s.EnsureKey(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	w := newCaptureWriter(8<<20, nil)
	if err := s.dispatch(ctx, client, key, http.MethodGet, "/v1/models", "", nil, w); err != nil {
		return nil, err
	}
	if w.statusCode() >= 300 {
		return nil, apperrors.New(w.statusCode(), "CHAT_MODELS_FAILED", gatewayError(w.statusCode(), w.body.Bytes()))
	}
	out := []string{}
	seen := map[string]bool{}
	gjson.GetBytes(w.body.Bytes(), "data").ForEach(func(_, item gjson.Result) bool {
		id := strings.TrimSpace(item.Get("id").String())
		if id == "" || seen[id] || IsImageModel(id) != (mode == ModeImage) {
			return true
		}
		seen[id] = true
		out = append(out, id)
		return true
	})
	return out, nil
}

// readObject loads an attachment's stored bytes.
func (s *Service) readObject(ctx context.Context, a Attachment, limit int64) ([]byte, error) {
	if a.StorageKey == "" {
		return nil, ErrBadAttachment
	}
	store, _, ok := s.ObjectStore()
	if !ok {
		return nil, ErrStorageRequired
	}
	body, _, _, err := store.Open(ctx, a.StorageKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrBadAttachment
	}
	return data, nil
}

// DeleteObjects removes stored objects in the background (best effort).
func (s *Service) DeleteObjects(keys []string) {
	if len(keys) == 0 {
		return
	}
	store, _, ok := s.ObjectStore()
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, k := range keys {
			if err := store.Delete(ctx, k); err != nil {
				log.Printf("jackchat: delete object %s: %v", k, err)
			}
		}
	}()
}

// CleanupPending deletes uploads that were never sent and marks replies whose
// run can no longer be running as interrupted.
func (s *Service) CleanupPending(ctx context.Context) {
	if err := s.Store.ExpireStreaming(ctx, s.Store.Now().Add(-RunTimeout-5*time.Minute)); err != nil {
		log.Printf("jackchat: expire streaming replies: %v", err)
	}
	keys, err := s.Store.StalePending(ctx, s.Store.Now().Add(-24*time.Hour))
	if err != nil {
		log.Printf("jackchat: cleanup pending uploads: %v", err)
		return
	}
	s.DeleteObjects(keys)
}

func randomName() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

// ImageSizes returns a copy of the ratio-to-size table for display.
func ImageSizes() map[string]string {
	out := make(map[string]string, len(imageSizes))
	for k, v := range imageSizes {
		out[k] = v
	}
	return out
}
