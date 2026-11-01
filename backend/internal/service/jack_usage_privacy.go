package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/jackprivacy"
)

// Use the original authenticated ingress metadata, not a protocol adapter's
// effective values. Old/background callers without ingress retain their inputs.
func jackRequestedUsageFields(ctx context.Context, model string, effort *string) (string, *string) {
	if policy := jackprivacy.FromContext(ctx); policy != nil {
		requested := policy.Requested()
		return requested.Model, requested.Effort
	}
	return model, effort
}
