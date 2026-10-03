package service

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Guard only explicit admin/purchase/redeem changes. Natural window maintenance
// and usage charging continue through the unmodified upstream billing code.
func (s *SubscriptionService) guardFleetSubscription(ctx context.Context, id, userID, groupID int64) error {
	if !s.fleetManagementEnabled || s.entClient == nil {
		return nil
	}
	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	rows, err := client.QueryContext(ctx, `SELECT 1 FROM jack_fleet_members m JOIN user_subscriptions s ON s.id=m.subscription_id WHERE ($1>0 AND s.id=$1) OR ($1=0 AND s.user_id=$2 AND s.group_id=$3) LIMIT 1`, id, userID, groupID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return apperrors.Conflict("FLEET_MANAGED_SUBSCRIPTION", "This subscription is managed by a fleet; use Fleet Management")
	}
	return rows.Err()
}

// InvalidateFleetSubscription is called after the fleet transaction commits.
func (s *SubscriptionService) InvalidateFleetSubscription(ctx context.Context, userID, groupID int64) error {
	return s.invalidateSubscriptionCaches(userID, groupID)
}

func (s *SubscriptionService) EnableFleetManagement() { s.fleetManagementEnabled = true }

// Unlike the generic best-effort invalidator, fleet operations persist retry
// state until Redis and local authentication caches have both been invalidated.
func (s *APIKeyService) InvalidateFleetAuthCache(ctx context.Context, userID int64) error {
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		cacheKey := s.authCacheKey(key)
		if s.authCacheL1 != nil {
			s.authCacheL1.Del(cacheKey)
			s.authCacheL1.Wait()
		}
		if s.authNegativeCacheL1 != nil {
			s.authNegativeCacheL1.Del(cacheKey)
			s.authNegativeCacheL1.Wait()
		}
		if s.cache != nil {
			if err = s.cache.DeleteAuthCache(ctx, cacheKey); err != nil {
				return err
			}
			if err = s.cache.PublishAuthCacheInvalidation(ctx, cacheKey); err != nil {
				return err
			}
		}
	}
	return nil
}
