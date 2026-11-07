package httputil

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
	"github.com/klauspost/compress/zstd"
)

func TestJackPrivacyCapturesOriginalBodyWithMemoryBudget(t *testing.T) {
	body := []byte(`{"model":"client-model","reasoning":{"effort":"high"}}`)
	for _, encoding := range []string{"identity", "gzip", "zstd", "preread"} {
		t.Run(encoding, func(t *testing.T) {
			policy := jackprivacy.New(true, false, "application/json", "", nil)
			budget := NewBodyMemoryBudget(32 << 20)
			ctx, release := WithRequestBodyMemoryBudget(jackprivacy.WithPolicy(context.Background(), policy), budget)
			defer release()
			wireBody := body
			switch encoding {
			case "gzip":
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				if _, err := writer.Write(body); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				wireBody = compressed.Bytes()
			case "zstd":
				writer, err := zstd.NewWriter(nil)
				if err != nil {
					t.Fatal(err)
				}
				wireBody = writer.EncodeAll(body, nil)
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(wireBody)).WithContext(ctx)
			if encoding == "preread" {
				req.Body = NewPrereadBody(body)
			} else {
				req.Header.Set("Content-Encoding", encoding)
			}
			got, err := ReadRequestBodyWithPrealloc(req)
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("decoded body = %q, error = %v", got, err)
			}
			charged := budget.used
			if encoding == "preread" && charged != 0 {
				t.Fatalf("borrowed body was charged again: %d", charged)
			}
			req.Body = NewPrereadBody([]byte(`{"model":"mapped-model","reasoning":{"effort":"low"}}`))
			if _, err := ReadRequestBodyWithPrealloc(req); err != nil {
				t.Fatal(err)
			}
			requested := policy.Requested()
			if requested.Model != "client-model" || requested.Effort == nil || *requested.Effort != "high" {
				t.Fatalf("original metadata was lost: %+v", requested)
			}
			if budget.used != charged {
				t.Fatalf("preread capture changed the reservation: %d -> %d", charged, budget.used)
			}
			release()
			if budget.used != 0 {
				t.Fatalf("reservation leaked after request: %d", budget.used)
			}
		})
	}
}

func TestJackPrivacyDoesNotCaptureBodyRejectedByMemoryBudget(t *testing.T) {
	policy := jackprivacy.New(true, false, "application/json", "", nil)
	budget := NewBodyMemoryBudget(1)
	ctx, release := WithRequestBodyMemoryBudget(jackprivacy.WithPolicy(context.Background(), policy), budget)
	defer release()
	req := httptest.NewRequest("POST", "/v1/responses", bytes.NewBufferString(`{"model":"rejected-model"}`)).WithContext(ctx)
	if _, err := ReadRequestBodyWithPrealloc(req); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("expected memory limit error, got %v", err)
	}
	if got := policy.Requested(); got.Model != "" || got.Effort != nil {
		t.Fatalf("captured a rejected request: %+v", got)
	}
	if budget.used != 0 {
		t.Fatalf("failed read leaked reservation: %d", budget.used)
	}
}
