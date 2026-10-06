package jackchat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"  // register decoders for dimension detection
	_ "image/jpeg" //
	_ "image/png"  //
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	_ "golang.org/x/image/webp" // register webp decoder
)

const (
	MaxImageCount       = 4
	maxGeneratedBytes   = 64 << 20
	imageDownloadClient = 60 * time.Second
)

// ImageInput is one image-mode turn.
type ImageInput struct {
	Prompt        string  `json:"prompt"`
	AttachmentIDs []int64 `json:"attachment_ids"`
	Regenerate    bool    `json:"regenerate"`
}

type imageResult struct {
	index int
	att   *Attachment
	err   string
}

// SendImages generates conv.ImageCount images, one gateway request each, so
// every image is billed and logged like a separate API call.
func (s *Service) SendImages(ctx context.Context, client Client, userID int64, conv Conversation, in ImageInput, emit Emit) error {
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return ErrDisabled
	}
	if conv.Mode != ModeImage || !IsImageModel(conv.Model) {
		return ErrBadModel
	}
	if !IsImageAspect(conv.ImageAspect) {
		conv.ImageAspect = "1:1"
	}
	count := conv.ImageCount
	if count < 1 || count > MaxImageCount {
		count = 1
	}
	store, prefix, ok := s.ObjectStore()
	if !ok {
		return ErrStorageRequired
	}
	if _, err := s.Group(ctx, userID, conv.GroupID); err != nil {
		return err
	}
	key, err := s.EnsureKey(ctx, userID, *conv.GroupID)
	if err != nil {
		return err
	}
	for _, id := range in.AttachmentIDs {
		a, err := s.Store.Attachment(ctx, userID, id)
		if err != nil || (a.Kind != KindImage && a.Kind != KindGenerated) {
			return ErrBadAttachment
		}
	}
	_, user, err := s.prepareTurn(ctx, userID, &conv, SendInput{Text: in.Prompt, AttachmentIDs: in.AttachmentIDs, Regenerate: in.Regenerate}, settings)
	if err != nil {
		return err
	}
	prompt := strings.TrimSpace(user.Content)
	if prompt == "" {
		return ErrEmpty
	}
	refs := make([]refImage, 0, len(user.Attachments))
	for _, a := range user.Attachments {
		if a.Kind != KindImage && a.Kind != KindGenerated {
			continue
		}
		data, err := s.readObject(ctx, a, maxGeneratedBytes)
		if err != nil {
			return err
		}
		refs = append(refs, refImage{name: a.Filename, mime: a.Mime, data: data})
	}

	reply, err := s.Store.InsertMessage(ctx, Message{ConversationID: conv.ID, Role: RoleAssistant, Model: conv.Model, Status: StatusStreaming})
	if err != nil {
		return err
	}
	emit(map[string]any{"type": "start", "conversation": conv, "user_message": user, "assistant_message": reply, "regenerate": in.Regenerate, "count": count, "aspect": conv.ImageAspect})

	parallel := count
	if u, err := s.users.GetByID(ctx, userID); err == nil && u.Concurrency > 0 && u.Concurrency < parallel {
		parallel = u.Concurrency
	}
	sem := make(chan struct{}, parallel)
	results := make(chan imageResult, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- imageResult{index: i, err: "cancelled"}
				return
			}
			defer func() { <-sem }()
			data, err := s.generateOne(ctx, client, key, conv.Model, ImagePrompt(conv.ImageAspect, prompt), refs)
			if err != nil {
				results <- imageResult{index: i, err: err.Error()}
				return
			}
			att, err := s.saveGenerated(context.WithoutCancel(ctx), store, prefix, userID, reply.ID, i, data)
			if err != nil {
				results <- imageResult{index: i, err: err.Error()}
				return
			}
			results <- imageResult{index: i, att: att}
		}(i)
	}
	go func() { wg.Wait(); close(results) }()

	errs := []string{}
	for r := range results {
		if r.att != nil {
			reply.Attachments = append(reply.Attachments, *r.att)
			emit(map[string]any{"type": "image", "index": r.index, "attachment": r.att})
		} else {
			errs = append(errs, r.err)
			emit(map[string]any{"type": "image_error", "index": r.index, "error": r.err})
		}
	}
	switch {
	case len(reply.Attachments) == count:
		reply.Status = StatusComplete
	case ctx.Err() != nil:
		reply.Status, reply.Error = runEndStatus(ctx)
	case len(reply.Attachments) > 0:
		reply.Status, reply.Error = StatusComplete, strings.Join(dedupe(errs), "; ")
	default:
		reply.Status, reply.Error = StatusError, strings.Join(dedupe(errs), "; ")
	}
	sortAttachments(reply.Attachments)
	return s.finishReply(conv.ID, reply, emit)
}

type refImage struct {
	name string
	mime string
	data []byte
}

// generateOne requests one image. No size is sent: the upstream ignores it, and
// the ratio is part of prompt (see ImagePrompt).
func (s *Service) generateOne(ctx context.Context, client Client, key, model, prompt string, refs []refImage) ([]byte, error) {
	var body []byte
	var contentType, path string
	fields := map[string]string{"model": model, "prompt": prompt, "n": "1"}
	if len(refs) == 0 {
		path, contentType = "/v1/images/generations", "application/json"
		body, _ = json.Marshal(map[string]any{"model": model, "prompt": prompt, "n": 1})
	} else {
		path = "/v1/images/edits"
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		for i, r := range refs {
			h := textproto.MIMEHeader{}
			name := r.name
			if name == "" {
				name = "reference-" + strconv.Itoa(i+1) + extensionFor(r.mime)
			}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename=%q`, name))
			h.Set("Content-Type", r.mime)
			part, err := mw.CreatePart(h)
			if err != nil {
				return nil, err
			}
			if _, err = part.Write(r.data); err != nil {
				return nil, err
			}
		}
		if err := mw.Close(); err != nil {
			return nil, err
		}
		body, contentType = buf.Bytes(), mw.FormDataContentType()
	}
	w := newCaptureWriter(maxGeneratedBytes, nil)
	if err := s.dispatch(ctx, client, key, http.MethodPost, path, contentType, body, w); err != nil {
		return nil, err
	}
	raw := bytes.TrimSpace(w.body.Bytes())
	if w.statusCode() >= 300 {
		return nil, fmt.Errorf("%s", gatewayError(w.statusCode(), raw))
	}
	if msg := gjson.GetBytes(raw, "error.message").String(); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	item := gjson.GetBytes(raw, "data.0")
	if b64 := item.Get("b64_json").String(); b64 != "" {
		return base64.StdEncoding.DecodeString(b64)
	}
	if u := item.Get("url").String(); u != "" {
		return download(ctx, u)
	}
	return nil, fmt.Errorf("the gateway returned no image")
}

func download(ctx context.Context, url string) ([]byte, error) {
	if strings.HasPrefix(url, "data:") {
		if i := strings.Index(url, ","); i > 0 {
			return base64.StdEncoding.DecodeString(url[i+1:])
		}
		return nil, fmt.Errorf("invalid image data")
	}
	ctx, cancel := context.WithTimeout(ctx, imageDownloadClient)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download image: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxGeneratedBytes))
}

func (s *Service) saveGenerated(ctx context.Context, store interface {
	Save(ctx context.Context, key, contentType string, data []byte) (string, error)
}, prefix string, userID, messageID int64, index int, data []byte) (*Attachment, error) {
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil, fmt.Errorf("the gateway returned an unsupported image")
	}
	width, height := 0, 0
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		width, height = cfg.Width, cfg.Height
	}
	ext := extensionFor(mime)
	key := fmt.Sprintf("%sjack-chat/%d/%s%s", prefix, userID, randomName(), ext)
	if _, err := store.Save(ctx, key, mime, data); err != nil {
		return nil, err
	}
	mid := messageID
	a, err := s.Store.InsertAttachment(ctx, userID, Attachment{
		MessageID: &mid, Kind: KindGenerated, StorageKey: key, Filename: fmt.Sprintf("image-%d%s", index+1, ext),
		Mime: mime, Size: int64(len(data)), Width: width, Height: height,
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func extensionFor(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "application/pdf":
		return ".pdf"
	}
	return ""
}

func sortAttachments(a []Attachment) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j].Filename < a[j-1].Filename; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
