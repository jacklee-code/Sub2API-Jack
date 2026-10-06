package service

import (
	"context"
	"io"
	"strings"
)

// JackChatObjectStore is the object storage Jack chat mode needs for
// attachments and generated images. The S3 image storage implements it.
type JackChatObjectStore interface {
	ImageStorage
	Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
	Delete(ctx context.Context, key string) error
}

// JackChatStore returns the configured image object storage and its key
// prefix. ok is false when image storage is disabled or incomplete.
func (s *ImageStorageSettingService) JackChatStore() (store JackChatObjectStore, prefix string, ok bool) {
	uploader, enabled := s.resolve()
	if !enabled || uploader == nil || uploader.storage == nil {
		return nil, "", false
	}
	store, ok = uploader.storage.(JackChatObjectStore)
	if !ok {
		return nil, "", false
	}
	prefix = strings.Trim(uploader.prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return store, prefix, true
}
