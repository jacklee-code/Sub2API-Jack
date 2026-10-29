package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Enabled by the fleet handler only after Jack migrations have run. Upstream-only
// fixtures and deployments keep their original repository behavior.
func (r *userSubscriptionRepository) EnableFleetResetPolicy() { r.fleetResetPolicy.Store(true) }
func (r *userSubscriptionRepository) withFleetResetPolicy(ctx context.Context, sub *service.UserSubscription) (*service.UserSubscription, error) {
	if sub == nil {
		return nil, nil
	}
	values := []service.UserSubscription{*sub}
	if err := r.attachFleetResetPolicy(ctx, values); err != nil {
		return nil, err
	}
	return &values[0], nil
}
func (r *userSubscriptionRepository) fleetResetPolicies(ctx context.Context, subs []service.UserSubscription) ([]service.UserSubscription, error) {
	err := r.attachFleetResetPolicy(ctx, subs)
	return subs, err
}
func (r *userSubscriptionRepository) attachFleetResetPolicy(ctx context.Context, subs []service.UserSubscription) error {
	if !r.fleetResetPolicy.Load() || len(subs) == 0 {
		return nil
	}
	ids := make([]int64, len(subs))
	index := map[int64]int{}
	for i := range subs {
		ids[i] = subs[i].ID
		index[subs[i].ID] = i
	}
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, `SELECT m.subscription_id,r.next_reset_at FROM jack_fleet_members m JOIN jack_fleets f ON f.id=m.fleet_id JOIN jack_fleet_reset_sync r ON r.fleet_id=f.id WHERE m.subscription_id=ANY($1) AND m.active AND r.enabled AND f.archived_at IS NULL`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var resetAt *time.Time
		if err = rows.Scan(&id, &resetAt); err != nil {
			return err
		}
		i := index[id]
		subs[i].FleetWeeklyManaged = true
		subs[i].FleetNextWeeklyReset = resetAt
	}
	return rows.Err()
}
