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
	resetSync     *service.FleetResetSyncService
	subscriptions *service.SubscriptionService
	keys          *service.APIKeyService
	cancel        context.CancelFunc
	done          chan struct{}
	mu            sync.Mutex
}

func NewFleetHandler(db *sql.DB, subs *service.SubscriptionService, keys *service.APIKeyService, quota *service.OpenAIQuotaService) *FleetHandler {
	h := &FleetHandler{store: jackfleet.NewStore(db), subscriptions: subs, keys: keys, done: make(chan struct{})}
	subs.EnableFleetManagement()
	h.resetSync = service.NewFleetResetSyncService(h.store, quota, h.drain)
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
func (h *FleetHandler) Stop() { h.resetSync.Stop(); h.cancel(); <-h.done }
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
	result, err := h.store.Replay(c.Request.Context(), actor, key, input)
	if err == nil && result == nil {
		if input.Action == "reset_sync" && input.SyncEnabled {
			if h.resetSync == nil {
				response.BadRequest(c, "Fleet reset synchronization is unavailable")
				return
			}
			input.SyncBaseline, err = h.resetSync.Baseline(c.Request.Context(), input.SyncAccountID)
		}
		if err == nil {
			result, err = h.store.Mutate(c.Request.Context(), actor, key, input)
		}
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 8*time.Second)
	defer cancel()
	if err = h.sync(ctx, actor, key, result); err != nil {
		log.Printf("[Fleet] committed operation awaiting cache synchronization: %v", err)
	}
	if h.resetSync != nil {
		h.resetSync.RefreshTargets(ctx)
	}
	response.Success(c, result)
}

func normalizeFleetMembers(members []jackfleet.Member) {
	for i := range members {
		m := &members[i]
		normalized := service.NormalizeFleetSubscriptionView(service.UserSubscription{FleetWeeklyManaged: m.WeeklyManaged, FleetNextWeeklyReset: m.NextWeeklyReset, StartsAt: m.StartsAt, ExpiresAt: m.ExpiresAt, Status: m.Status, DailyWindowStart: m.DailyStart, WeeklyWindowStart: m.WeeklyStart, MonthlyWindowStart: m.MonthlyStart, DailyUsageUSD: m.Daily, WeeklyUsageUSD: m.Weekly, MonthlyUsageUSD: m.Monthly})
		m.Daily, m.Weekly, m.Monthly, m.Status = normalized.DailyUsageUSD, normalized.WeeklyUsageUSD, normalized.MonthlyUsageUSD, normalized.Status
	}
}

func (h *FleetHandler) ResetEvents(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid fleet ID")
		return
	}
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	events, err := h.store.ResetEvents(c.Request.Context(), id, before, 20)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, events)
}
func (h *FleetHandler) ResetSources(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid fleet ID")
		return
	}
	rows, err := h.store.DB.QueryContext(c.Request.Context(), `SELECT a.id,a.name FROM accounts a JOIN account_groups ag ON ag.account_id=a.id JOIN jack_fleets f ON f.group_id=ag.group_id WHERE f.id=$1 AND f.archived_at IS NULL AND a.platform='openai' AND a.type='oauth' AND a.parent_account_id IS NULL AND a.deleted_at IS NULL AND a.status='active' ORDER BY a.id`, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer func() { _ = rows.Close() }()
	type source struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	values := []source{}
	for rows.Next() {
		var v source
		if err = rows.Scan(&v.ID, &v.Name); err != nil {
			response.ErrorFrom(c, err)
			return
		}
		values = append(values, v)
	}
	if err = rows.Err(); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, values)
}
