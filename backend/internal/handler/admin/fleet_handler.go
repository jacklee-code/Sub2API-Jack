package admin

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type FleetHandler struct {
	store         *jackfleet.Store
	subscriptions *service.SubscriptionService
	keys          *service.APIKeyService
	cancel        context.CancelFunc
	done          chan struct{}
	mu            sync.Mutex
}

func NewFleetHandler(db *sql.DB, subs *service.SubscriptionService, keys *service.APIKeyService) *FleetHandler {
	h := &FleetHandler{store: jackfleet.NewStore(db), subscriptions: subs, keys: keys, done: make(chan struct{})}
	subs.EnableFleetManagement()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		defer close(h.done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, stop := context.WithTimeout(ctx, 8*time.Second)
				h.drain(attempt)
				stop()
			}
		}
	}()
	return h
}
func (h *FleetHandler) Stop() { h.cancel(); <-h.done }
func (h *FleetHandler) sync(ctx context.Context, actor int64, key string, result *jackfleet.Result) error {
	if !result.CachePending {
		return nil
	}
	seen := map[jackfleet.Pair]bool{}
	for _, pair := range result.Changed {
		if seen[pair] {
			continue
		}
		seen[pair] = true
		if err := h.keys.InvalidateFleetAuthCache(ctx, pair.UserID); err != nil {
			return err
		}
		if err := h.subscriptions.InvalidateFleetSubscription(ctx, pair.UserID, pair.GroupID); err != nil {
			return err
		}
	}
	if err := h.store.MarkSynced(ctx, actor, key); err != nil {
		return err
	}
	result.CachePending = false
	return nil
}
func (h *FleetHandler) drain(ctx context.Context) {
	if !h.mu.TryLock() {
		return
	}
	defer h.mu.Unlock()
	pending, err := h.store.Pending(ctx)
	if err != nil {
		return
	}
	for _, op := range pending {
		if err = h.sync(ctx, op.Actor, op.Key, &op.Result); err != nil {
			log.Printf("[Fleet] cache synchronization pending: %v", err)
			return
		}
	}
}
func (h *FleetHandler) List(c *gin.Context) {
	fleets, err := h.store.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	for i := range fleets {
		normalizeFleetMembers(fleets[i].Members)
	}
	response.Success(c, fleets)
}
func (h *FleetHandler) Preview(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	members, err := h.store.Preview(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	normalizeFleetMembers(members)
	response.Success(c, members)
}
func (h *FleetHandler) Mutate(c *gin.Context) {
	var input jackfleet.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid fleet input")
		return
	}
	actor := getAdminIDFromContext(c)
	key := c.GetHeader("Idempotency-Key")
	result, err := h.store.Mutate(c.Request.Context(), actor, key, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 8*time.Second)
	defer cancel()
	if err = h.sync(ctx, actor, key, result); err != nil {
		log.Printf("[Fleet] committed operation awaiting cache synchronization: %v", err)
	}
	response.Success(c, result)
}

func normalizeFleetMembers(members []jackfleet.Member) {
	for i := range members {
		m := &members[i]
		normalized := service.NormalizeFleetSubscriptionView(service.UserSubscription{StartsAt: m.StartsAt, ExpiresAt: m.ExpiresAt, Status: m.Status, DailyWindowStart: m.DailyStart, WeeklyWindowStart: m.WeeklyStart, MonthlyWindowStart: m.MonthlyStart, DailyUsageUSD: m.Daily, WeeklyUsageUSD: m.Weekly, MonthlyUsageUSD: m.Monthly})
		m.Daily, m.Weekly, m.Monthly, m.Status = normalized.DailyUsageUSD, normalized.WeeklyUsageUSD, normalized.MonthlyUsageUSD, normalized.Status
	}
}
