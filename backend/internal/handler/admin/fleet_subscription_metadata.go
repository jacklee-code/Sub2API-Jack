package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/lib/pq"
)

func (h *FleetHandler) AttachSubscriptions(subscriptions *SubscriptionHandler) {
	subscriptions.fleet = h
}
func (h *SubscriptionHandler) annotateFleet(ctx context.Context, subscriptions []dto.AdminUserSubscription) error {
	if h.fleet == nil || len(subscriptions) == 0 {
		return nil
	}
	ids := make([]int64, len(subscriptions))
	for i := range subscriptions {
		ids[i] = subscriptions[i].ID
	}
	rows, err := h.fleet.store.DB.QueryContext(ctx, `SELECT m.subscription_id,m.fleet_id,m.independent_expiry FROM jack_fleet_members m WHERE m.subscription_id=ANY($1)`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	byID := map[int64]int{}
	for i := range subscriptions {
		byID[subscriptions[i].ID] = i
	}
	for rows.Next() {
		var subID, fleetID int64
		var independent bool
		if err = rows.Scan(&subID, &fleetID, &independent); err != nil {
			return err
		}
		if i, ok := byID[subID]; ok {
			subscriptions[i].FleetID = &fleetID
			subscriptions[i].FleetIndependentExpiry = independent
		}
	}
	return rows.Err()
}
