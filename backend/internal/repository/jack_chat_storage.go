package repository

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.JackChatObjectStore = (*S3ImageStorage)(nil)

// Open reads an object for Jack chat mode attachments.
func (s *S3ImageStorage) Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	finish := servertiming.ObserveDependency(ctx, "s3")
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	finish()
	if err != nil {
		return nil, "", 0, fmt.Errorf("S3 GetObject: %w", err)
	}
	contentType := ""
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, contentType, size, nil
}

// Delete removes an object for Jack chat mode attachments.
func (s *S3ImageStorage) Delete(ctx context.Context, key string) error {
	finish := servertiming.ObserveDependency(ctx, "s3")
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	finish()
	if err != nil {
		return fmt.Errorf("S3 DeleteObject: %w", err)
	}
	return nil
}
