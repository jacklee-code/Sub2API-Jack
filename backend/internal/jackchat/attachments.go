package jackchat

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var textExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true, ".json": true, ".jsonl": true,
	".xml": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".log": true, ".html": true, ".htm": true,
	".css": true, ".js": true, ".mjs": true, ".ts": true, ".tsx": true, ".jsx": true, ".vue": true, ".py": true,
	".go": true, ".java": true, ".kt": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cs": true,
	".rs": true, ".rb": true, ".php": true, ".swift": true, ".sh": true, ".bash": true, ".ps1": true, ".sql": true,
	".r": true, ".m": true, ".scala": true, ".lua": true, ".dart": true, ".tex": true, ".srt": true, ".vtt": true,
}

var imageMimes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}

// ErrTooLarge reports a file over the configured limit.
func ErrTooLarge(limit int64) error {
	return apperrors.BadRequest("CHAT_ATTACHMENT_TOO_LARGE", fmt.Sprintf("file is larger than %d MB", limit>>20))
}

var ErrUnsupportedFile = apperrors.BadRequest("CHAT_ATTACHMENT_UNSUPPORTED", "only images, PDF and text files are supported")

// MaxUploadBytes is the largest file any kind may have.
func MaxUploadBytes(s Settings) int64 {
	limit := s.MaxImageBytes
	if s.MaxPDFBytes > limit {
		limit = s.MaxPDFBytes
	}
	if s.MaxTextBytes > limit {
		limit = s.MaxTextBytes
	}
	return limit
}

// Classify returns the attachment kind and MIME type of an upload.
func Classify(filename string, data []byte) (kind, mime string, err error) {
	sniffed := http.DetectContentType(data)
	if i := strings.Index(sniffed, ";"); i > 0 {
		sniffed = sniffed[:i]
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case imageMimes[sniffed]:
		return KindImage, sniffed, nil
	case sniffed == "application/pdf":
		return KindPDF, sniffed, nil
	case textExtensions[ext] || strings.HasPrefix(sniffed, "text/"):
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return "", "", ErrUnsupportedFile
		}
		return KindText, "text/plain", nil
	}
	return "", "", ErrUnsupportedFile
}

// Upload stores a pending attachment for the user's next message.
func (s *Service) Upload(ctx context.Context, userID int64, filename string, data []byte) (Attachment, error) {
	settings, err := s.Store.Settings(ctx)
	if err != nil {
		return Attachment{}, err
	}
	if !settings.Enabled {
		return Attachment{}, ErrDisabled
	}
	filename = sanitizeFilename(filename)
	kind, mime, err := Classify(filename, data)
	if err != nil {
		return Attachment{}, err
	}
	limit := map[string]int64{KindImage: settings.MaxImageBytes, KindPDF: settings.MaxPDFBytes, KindText: settings.MaxTextBytes}[kind]
	if int64(len(data)) > limit {
		return Attachment{}, ErrTooLarge(limit)
	}
	a := Attachment{Kind: kind, Filename: filename, Mime: mime, Size: int64(len(data))}
	if kind == KindText {
		a.ExtractedText = string(data)
		return s.Store.InsertAttachment(ctx, userID, a)
	}
	store, prefix, ok := s.ObjectStore()
	if !ok {
		return Attachment{}, ErrStorageRequired
	}
	if kind == KindImage {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return Attachment{}, ErrUnsupportedFile
		}
		a.Width, a.Height = cfg.Width, cfg.Height
	}
	a.StorageKey = fmt.Sprintf("%sjack-chat/%d/%s%s", prefix, userID, randomName(), extensionFor(mime))
	if _, err := store.Save(ctx, a.StorageKey, mime, data); err != nil {
		return Attachment{}, err
	}
	out, err := s.Store.InsertAttachment(ctx, userID, a)
	if err != nil {
		s.DeleteObjects([]string{a.StorageKey})
	}
	return out, err
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '"' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	if utf8.RuneCountInString(name) > 200 {
		r := []rune(name)
		name = string(r[len(r)-200:])
	}
	return name
}
