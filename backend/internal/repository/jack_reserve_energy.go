package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/jackreserve"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) ListJackReserveAccounts(ctx context.Context) ([]service.Account, error) {
	return r.FindByExtraField(ctx, jackreserve.EnabledKey, true)
}
