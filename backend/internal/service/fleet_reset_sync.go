package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackfleet"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type fleetQuotaReader interface {
	QueryUsageSnapshot(context.Context, int64) (*OpenAIQuotaUsage, error)
	ResetCreditStable(context.Context, int64, string, ...string) (*OpenAIQuotaResetResult, error)
}
type fleetObservationInput struct {
	blockedSubscription int64
	account             int64
	snapshot            *OpenAICodexUsageSnapshot
	quota               *OpenAIQuotaUsage
	at                  time.Time
}

// FleetResetSyncService consumes observations and durable, event-created jobs.
// Its job ticker never creates monitoring traffic for idle accounts.
type FleetResetSyncService struct {
	store      *jackfleet.Store
	quota      fleetQuotaReader
	accounts   AccountRepository
	flush      func(context.Context)
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	queue      chan fleetObservationInput
	mu         sync.Mutex
	interested map[int64]bool
	sampled    map[int64]jackfleet.Observation
	blocked    map[int64]time.Time
}

var fleetResetRegistry struct {
	sync.RWMutex
	service *FleetResetSyncService
}

func NewFleetResetSyncService(store *jackfleet.Store, quota *OpenAIQuotaService, flush func(context.Context)) *FleetResetSyncService {
	if quota == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &FleetResetSyncService{store: store, quota: quota, accounts: quota.accountRepo, flush: flush, ctx: ctx, cancel: cancel, queue: make(chan fleetObservationInput, 256), interested: map[int64]bool{}, sampled: map[int64]jackfleet.Observation{}, blocked: map[int64]time.Time{}}
	s.RefreshTargets(ctx)
	fleetResetRegistry.Lock()
	fleetResetRegistry.service = s
	fleetResetRegistry.Unlock()
	s.wg.Add(2)
	go s.runObservations()
	go s.runJobs()
	return s
}
func (s *FleetResetSyncService) Stop() {
	if s == nil {
		return
	}
	fleetResetRegistry.Lock()
	if fleetResetRegistry.service == s {
		fleetResetRegistry.service = nil
	}
	fleetResetRegistry.Unlock()
	s.cancel()
	s.wg.Wait()
}
func currentFleetResetService() *FleetResetSyncService {
	fleetResetRegistry.RLock()
	defer fleetResetRegistry.RUnlock()
	return fleetResetRegistry.service
}
func (s *FleetResetSyncService) RefreshTargets(ctx context.Context) {
	_, validationErr := s.store.DB.ExecContext(ctx, `UPDATE jack_fleet_reset_sync r SET status='unavailable',error_code='SOURCE_UNAVAILABLE',candidate=NULL WHERE r.enabled AND r.error_code<>'SOURCE_UNAVAILABLE' AND NOT EXISTS(SELECT 1 FROM accounts a JOIN account_groups ag ON ag.account_id=a.id JOIN jack_fleets f ON f.group_id=ag.group_id WHERE a.id=r.account_id AND f.id=r.fleet_id AND f.archived_at IS NULL AND a.deleted_at IS NULL AND a.platform='openai' AND a.type='oauth' AND a.parent_account_id IS NULL AND a.status='active')`)
	if validationErr != nil {
		slog.Warn("fleet_reset_source_validation_failed")
		return
	}
	rows, err := s.store.DB.QueryContext(ctx, `SELECT DISTINCT r.account_id FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id WHERE r.enabled AND f.archived_at IS NULL`)
	if err != nil {
		slog.Warn("fleet_reset_targets_unavailable")
		return
	}
	next := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return
		}
		next[id] = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return
	}
	s.mu.Lock()
	s.interested = next
	for id := range s.sampled {
		if !next[id] {
			delete(s.sampled, id)
		}
	}
	s.mu.Unlock()
}
func NotifyFleetResetSnapshot(account int64, snapshot *OpenAICodexUsageSnapshot) {
	if s := currentFleetResetService(); s != nil && snapshot != nil {
		s.enqueue(fleetObservationInput{account: account, snapshot: snapshot})
	}
}
func notifyFleetQuotaObservation(account int64, quota *OpenAIQuotaUsage, at time.Time) {
	if s := currentFleetResetService(); s != nil && quota != nil {
		s.enqueue(fleetObservationInput{account: account, quota: quota, at: at})
	}
}
func (s *FleetResetSyncService) enqueue(in fleetObservationInput) {
	s.mu.Lock()
	wanted := s.interested[in.account]
	s.mu.Unlock()
	if !wanted {
		return
	}
	select {
	case s.queue <- in:
	case <-s.ctx.Done():
	default:
		slog.Warn("fleet_reset_observation_queue_full", "account_id", in.account)
	}
}
func NotifyFleetQuotaBlocked(sub *UserSubscription) {
	if sub == nil || !sub.FleetWeeklyManaged {
		return
	}
	s := currentFleetResetService()
	if s == nil {
		return
	}
	now := time.Now()
	s.mu.Lock()
	last := s.blocked[sub.ID]
	if now.Sub(last) < time.Minute {
		s.mu.Unlock()
		return
	}
	s.blocked[sub.ID] = now
	// Bound this admission cache; database cooldown remains authoritative.
	if len(s.blocked) > 10000 {
		for id, at := range s.blocked {
			if now.Sub(at) > time.Minute {
				delete(s.blocked, id)
			}
		}
	}
	s.mu.Unlock()
	select {
	case s.queue <- fleetObservationInput{blockedSubscription: sub.ID}:
	case <-s.ctx.Done():
	default:
		slog.Warn("fleet_reset_check_queue_full", "subscription_id", sub.ID)
	}

}
func fleetAccountIdentity(a *Account) string {
	return jackfleet.SourceIdentity(a.ID, a.GetCredential("chatgpt_account_id"), a.GetCredential("organization_id"), a.GetCredential("email"), a.GetCredential("plan_type"))
}
func fleetObservation(in fleetObservationInput, a *Account) *jackfleet.Observation {
	if a == nil || a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth || a.IsShadow() || a.Status != StatusActive {
		return nil
	}
	o := &jackfleet.Observation{Identity: fleetAccountIdentity(a), Plan: strings.ToLower(a.GetCredential("plan_type")), Source: "request"}
	if in.quota != nil {
		if in.quota.RateLimit == nil {
			return nil
		}
		var window *OpenAIRateLimitWindow
		for _, w := range []*OpenAIRateLimitWindow{in.quota.RateLimit.PrimaryWindow, in.quota.RateLimit.SecondaryWindow} {
			if w != nil && w.LimitWindowSeconds == 604800 {
				window = w
			}
		}
		if window == nil || !window.usedPercentPresent {
			return nil
		}
		// A mismatched returned identity must never be accepted as this account's data.
		if got, want := in.quota.AccountID, a.GetCredential("chatgpt_account_id"); got != "" && want != "" && got != want {
			return nil
		}
		o.Used = window.UsedPercent
		o.At = in.at
		o.ResetAt = time.Unix(window.ResetAt, 0)
		o.Source = "query"
		if in.quota.PlanType != "" {
			o.Plan = strings.ToLower(in.quota.PlanType)
		}
	} else if in.snapshot != nil {
		n := in.snapshot.Normalize()
		if n == nil || n.Used7dPercent == nil || n.Reset7dSeconds == nil || n.Window7dMinutes == nil || *n.Window7dMinutes != 10080 {
			return nil
		}
		o.Used = *n.Used7dPercent
		o.At = codexSnapshotBaseTime(in.snapshot, time.Now())
		if at, err := time.Parse(time.RFC3339, in.snapshot.ServerObservedAt); err == nil {
			o.At = at
		}
		o.ResetAt = o.At.Add(time.Duration(*n.Reset7dSeconds) * time.Second)
	} else {
		return nil
	}
	if !o.Valid(time.Now()) {
		return nil
	}
	return o
}
func (s *FleetResetSyncService) Baseline(ctx context.Context, account int64) (*jackfleet.Observation, error) {
	a, err := s.accounts.GetByID(ctx, account)
	if err != nil {
		return nil, err
	}
	at := time.Now()
	quota, err := s.quota.QueryUsageSnapshot(ctx, account)
	if err != nil {
		return nil, err
	}
	o := fleetObservation(fleetObservationInput{account: account, quota: quota, at: at}, a)
	if o == nil {
		return nil, infraerrors.BadRequest("FLEET_RESET_BASELINE_UNAVAILABLE", "A fresh ChatGPT seven-day usage window is required")
	}
	return o, nil
}
func (s *FleetResetSyncService) runObservations() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case in := <-s.queue:
			s.observe(in)
		}
	}
}
func (s *FleetResetSyncService) observe(in fleetObservationInput) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if in.blockedSubscription > 0 {
		if err := s.store.QueueQuotaCheck(ctx, in.blockedSubscription); err != nil {
			slog.Warn("fleet_reset_check_enqueue_failed", "subscription_id", in.blockedSubscription)
		}
		return
	}
	a, err := s.accounts.GetByID(ctx, in.account)
	if err != nil {
		return
	}
	o := fleetObservation(in, a)
	if o == nil {
		return
	}
	s.mu.Lock()
	last, known := s.sampled[in.account]
	// Keep abrupt falls/window transitions despite the regular-sample throttle.
	suppress := in.quota == nil && known && o.Identity == last.Identity && o.Plan == last.Plan && o.At.Sub(last.At) < time.Minute && !(last.Used >= 5 && o.Used <= 1) && !(!o.At.Before(last.ResetAt) && o.ResetAt.After(last.ResetAt.Add(time.Minute)))
	if !suppress {
		s.sampled[in.account] = *o
	}
	s.mu.Unlock()
	if suppress {
		return
	}
	if err = s.store.Observe(ctx, in.account, *o); err != nil {
		slog.Warn("fleet_reset_observation_failed", "account_id", in.account)
		return
	}
	s.flush(ctx)
}
func (s *FleetResetSyncService) runJobs() {
	defer s.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	refresh := time.NewTicker(10 * time.Second)
	defer refresh.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-refresh.C:
			s.RefreshTargets(s.ctx)
		case <-ticker.C:
			s.runPending(s.ctx)
		}
	}
}

// TryFleetManualCredit retains the old handler seam for mocks and deployments
// without fleet management; production always has this service installed.
func TryFleetManualCredit(ctx context.Context, actor, account int64, key string) (*OpenAIQuotaResetResult, bool, error) {
	s := currentFleetResetService()
	if s == nil {
		return nil, false, nil
	}
	source, err := s.accounts.GetByID(ctx, account)
	if err != nil {
		return nil, true, err
	}
	if source == nil {
		return nil, true, infraerrors.BadRequest("FLEET_CREDIT_REJECTED", "Account not found")
	}
	op, err := s.store.BeginCredit(ctx, actor, account, key, fleetAccountIdentity(source))
	if err != nil {
		return nil, true, err
	}
	result, err := s.executeCredit(ctx, op)
	return result, true, err
}
func (s *FleetResetSyncService) executeCredit(ctx context.Context, op *jackfleet.CreditOperation) (*OpenAIQuotaResetResult, error) {
	var result *OpenAIQuotaResetResult
	if op.State == "failed" {
		return nil, infraerrors.BadRequest("FLEET_CREDIT_REJECTED", "The upstream rejected this redemption; refresh quota before starting a new operation")
	}
	if op.State == "complete" {
		result = &OpenAIQuotaResetResult{}
		if err := json.Unmarshal(op.Result, result); err != nil {
			return nil, err
		}
	} else {
		source, loadErr := s.accounts.GetByID(ctx, op.AccountID)
		if loadErr != nil {
			return nil, loadErr
		}
		if source == nil || (op.Identity != "" && fleetAccountIdentity(source) != op.Identity) {
			if err := s.store.RejectCredit(ctx, op.ID, "SOURCE_IDENTITY_CHANGED"); err != nil {
				return nil, err
			}
			return nil, infraerrors.BadRequest("FLEET_CREDIT_REJECTED", "Account identity changed; the previous redemption will not be retried")
		}
		var err error
		expectedAccount := source.GetCredential("chatgpt_account_id")
		if expectedAccount == "" {
			expectedAccount = source.GetCredential("organization_id")
		}
		result, err = s.quota.ResetCreditStable(ctx, op.AccountID, op.RedeemID, expectedAccount)
		persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer stop()
		if err != nil || result == nil {
			if err != nil && definitiveFleetCreditRejection(err) {
				if saveErr := s.store.RejectCredit(persist, op.ID, infraerrors.Reason(err)); saveErr != nil {
					return nil, saveErr
				}
				return nil, infraerrors.BadRequest("FLEET_CREDIT_REJECTED", "The upstream rejected this redemption; refresh quota before starting a new operation")
			}
			_ = s.store.RetryCredit(persist, op.ID, "REDEMPTION_UNCONFIRMED")
			if err != nil {
				return nil, err
			}
			return nil, infraerrors.InternalServer("FLEET_CREDIT_EMPTY_RESULT", "Upstream redemption outcome is unknown; retry the same operation")
		}
		// Persist only the outcome, never upstream credit/account identifiers.
		body, _ := json.Marshal(OpenAIQuotaResetResult{Code: result.Code, WindowsReset: result.WindowsReset})
		if err = s.store.CompleteCredit(persist, op, result.Code, result.WindowsReset, body); err != nil {
			_ = s.store.RetryCredit(persist, op.ID, "LOCAL_SYNC_PENDING")
			result.FleetSyncStatus = "pending"
			return result, nil
		}
	}
	post, stop := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer stop()
	s.flush(post)
	status, count, err := s.store.CreditSyncStatus(post, op.ID)
	if err != nil {
		status = "pending"
	}
	result.FleetSyncStatus = status
	result.FleetSyncCount = count
	return result, nil
}

func (s *FleetResetSyncService) runPending(ctx context.Context) {
	if op, err := s.store.ClaimCredit(ctx); err == nil && op != nil {
		_, _ = s.executeCredit(ctx, op)
	}
	account, err := s.store.ClaimCheck(ctx)
	if err != nil || account == 0 {
		return
	}
	o, err := s.Baseline(ctx, account)
	code := ""
	if err != nil {
		code = "UPSTREAM_QUERY_FAILED"
	} else if err = s.store.Observe(ctx, account, *o); err != nil {
		code = "OBSERVATION_SAVE_FAILED"
	}
	_ = s.store.FinishCheck(ctx, account, code)
	s.flush(ctx)
}

func definitiveFleetCreditRejection(err error) bool {
	switch infraerrors.Reason(err) {
	case "OPENAI_QUOTA_TOKEN_UNAVAILABLE", "OPENAI_QUOTA_NOT_CONFIGURED", "OPENAI_QUOTA_CLIENT_ERROR", "OPENAI_QUOTA_AUTH_FAILED", "OPENAI_QUOTA_SOURCE_CHANGED", "OPENAI_QUOTA_ACCOUNT_NOT_FOUND", "OPENAI_QUOTA_INVALID_PLATFORM", "OPENAI_QUOTA_INVALID_TYPE", "OPENAI_QUOTA_MISSING_ACCOUNT_ID", "SPARK_SHADOW_RESET_NOT_SUPPORTED":
		return true
	case "OPENAI_QUOTA_RESET_UPSTREAM_ERROR":
		status := infraerrors.Code(err)
		return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests
	default:
		return false
	}
}
