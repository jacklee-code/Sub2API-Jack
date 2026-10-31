package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/jackreserve"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var jackReserveDiscoveryVersion atomic.Uint64

func invalidateJackReserveDiscovery(ctx context.Context) {
	AfterAccountCommit(ctx, func(context.Context) { jackReserveDiscoveryVersion.Add(1) })
}

// Spendable Codex points are not reset cards. Keep the last upstream credit
// snapshot as a preference only: it never clears an actual upstream 429, a
// disabled account, a model restriction or a local administrator-set key quota.
func jackReserveHasSpendableCredits(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() || account.IsShadow() {
		return false
	}
	plan, err := jackreserve.Parse(account.Extra)
	if err != nil || !plan.Enabled {
		return false
	}
	raw, exists := account.Extra[openaiQuotaCreditsKey]
	if !exists {
		return false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var snapshot openAICreditsSnapshot
	if json.Unmarshal(data, &snapshot) != nil || snapshot.Credits == nil {
		return false
	}
	credits := snapshot.Credits
	if credits.Unlimited {
		return true
	}
	if !credits.HasCredits {
		return false
	}
	if credits.Balance == nil {
		return true
	}
	balance := strings.TrimSpace(*credits.Balance)
	// Decimal text is intentionally not rounded through float64.
	hasDigit, positive, dot := false, false, false
	for _, ch := range balance {
		if ch == '.' && !dot {
			dot = true
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
		hasDigit = true
		positive = positive || ch != '0'
	}
	return hasDigit && positive
}

func jackReserveShouldTryPrimaryCredits(account *Account) bool {
	if jackReserveHasSpendableCredits(account) {
		return true
	}
	if account == nil || !account.IsOpenAIOAuth() || account.IsShadow() {
		return false
	}
	plan, err := jackreserve.Parse(account.Extra)
	if err != nil || !plan.Enabled {
		return false
	}
	// A 100% subscription window does not prove that paid credits are exhausted.
	// Let the actual upstream decide once, even if the admin balance snapshot
	// is absent or stale. Its 429 sets the ordinary cooldown before failover;
	// this never clears that cooldown or probes once per incoming request.
	for _, window := range []string{"5h", "7d"} {
		if usage, known := resolveOpenAIQuotaUtilization(account.Extra, window, time.Now()); known && usage >= 1 {
			return true
		}
	}
	return false
}

// Optional repository capability keeps this Jack-specific lookup out of the
// upstream repository interface. Disabled plans cost no account hydration.
type jackReserveAccountRepository interface {
	ListJackReserveAccounts(context.Context) ([]Account, error)
}

func (s *adminServiceImpl) validateJackReserveExtra(ctx context.Context, account *Account, extra map[string]any) error {
	if !jackreserve.HasConfig(extra) {
		return nil
	}
	c, err := jackreserve.Parse(extra)
	bad := func(message string) error { return infraerrors.BadRequest("JACK_RESERVE_INVALID", message) }
	if err != nil {
		return bad(err.Error())
	}
	if !c.Enabled {
		return nil
	}
	if account.Platform != PlatformOpenAI || account.IsShadow() {
		return bad("reserve plans require a non-shadow OpenAI account")
	}
	for _, id := range c.AccountIDs {
		if id == account.ID {
			return bad("an account cannot be its own reserve")
		}
		target, err := s.accountRepo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if target == nil || target.Platform != PlatformOpenAI || target.IsShadow() {
			return bad("reserve accounts must be existing non-shadow OpenAI accounts")
		}
	}
	return nil
}

func (s *adminServiceImpl) validateJackReserveExtraPatch(ctx context.Context, id int64, updates map[string]any) error {
	if !jackreserve.HasConfig(updates) {
		return nil
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	merged := make(map[string]any, len(account.Extra)+len(updates))
	for key, value := range account.Extra {
		merged[key] = value
	}
	for key, value := range updates {
		merged[key] = value
	}
	return s.validateJackReserveExtra(ctx, account, merged)
}

// Quota recovery is evaluated for every new request. Administrative disablement,
// credential failures and proxy failures do not activate a primary's reserve.
func jackReserveQuotaBlocked(ctx context.Context, account *Account) bool {
	if account == nil || !account.IsActive() || !account.Schedulable || account.IsShadow() {
		return false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !time.Now().Before(*account.ExpiresAt) {
		return false
	}
	if account.IsRateLimited() || (account.SupportsAccountQuota() && account.IsQuotaExceeded()) {
		return true
	}
	if account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) && IsAccountSchedulingThresholdReason(account.TempUnschedulableReason) {
		return true
	}
	paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, account)
	return paused
}

// selectJackReserveEnergy returns the real destination account, never a primary
// with substituted credentials. The forwarder, proxy, concurrency and billing
// paths therefore all use the same destination identity.
func (s *OpenAIGatewayService) selectJackReserveEnergy(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, map[int64]struct{}, error) {
	excluded := cloneExcludedAccountIDs(req.ExcludedIDs)
	repo, supported := s.accountRepo.(jackReserveAccountRepository)
	if !supported || req.Platform != PlatformOpenAI {
		return nil, excluded, nil
	}
	version := jackReserveDiscoveryVersion.Load()
	if time.Now().UnixNano() < s.jackReserveEmptyUntil.Load() && s.jackReserveEmptyVersion.Load() == version {
		return nil, excluded, nil
	}
	plans, err := repo.ListJackReserveAccounts(ctx)
	if err != nil {
		return nil, excluded, err
	}
	if len(plans) == 0 {
		// Cache only absence. Active plans and quota state remain database-fresh.
		// Local admin writes invalidate after commit; other instances discover
		// activation within one second without a shared-cache dependency.
		s.jackReserveEmptyVersion.Store(version)
		s.jackReserveEmptyUntil.Store(time.Now().Add(time.Second).UnixNano())
		return nil, excluded, nil
	}
	sort.Slice(plans, func(i, j int) bool {
		if plans[i].Priority != plans[j].Priority {
			return plans[i].Priority < plans[j].Priority
		}
		return plans[i].ID < plans[j].ID
	})
	type plan struct {
		parent *Account
		config jackreserve.Config
	}
	eligible := make([]plan, 0, len(plans))
	for i := range plans {
		parent := &plans[i]
		if parent.Platform != PlatformOpenAI || parent.IsShadow() || !s.openAIAccountMatchesSchedulingGroup(parent, req.GroupID) {
			continue
		}
		config, err := jackreserve.Parse(parent.Extra)
		if err != nil || !config.Enabled {
			continue
		}
		eligible = append(eligible, plan{parent, config})
		// Within this primary's groups reserves must never enter weighted/sticky
		// scheduling, even after the primary has recovered.
		if excluded == nil {
			excluded = make(map[int64]struct{})
		}
		for _, id := range config.AccountIDs {
			excluded[id] = struct{}{}
		}
	}
	if len(eligible) == 0 {
		return nil, excluded, nil
	}
	if s.checkChannelPricingRestriction(ctx, req.GroupID, req.RequestedModel) {
		return nil, excluded, ErrNoAvailableAccounts
	}
	if req.PreviousResponseID != "" && !req.PreviousResponseCanMove {
		return nil, excluded, nil
	}
	scheduler := &defaultOpenAIAccountScheduler{service: s}
	canUse := func(account *Account) bool {
		return account != nil && account.Platform == PlatformOpenAI && !account.IsShadow() &&
			isOpenAICompatibleAccountEligibleForRequestBeforeProfit(ctx, account, req.Platform, req.RequestedModel, req.RequireCompact, req.RequiredCapability) &&
			!s.isOpenAIAccountBlockedBySchedulingThreshold(ctx, account) &&
			scheduler.isAccountRequestCompatible(ctx, account, req) &&
			scheduler.isAccountTransportCompatible(account, req.RequiredTransport)
	}
	for _, plan := range eligible {
		parent := plan.parent
		quotaBlocked := jackReserveQuotaBlocked(ctx, parent) || s.isOpenAIAccountBlockedBySchedulingThreshold(ctx, parent)
		if !quotaBlocked || !supportsOpenAIRequestModel(parent, req.RequestedModel, req.RequireCompact) {
			continue
		}
		// No recursive plans: the administrator's listed destinations are the
		// complete chain, so cycles cannot cause repeated upstream requests.
		for index, id := range plan.config.AccountIDs {
			if _, failed := req.ExcludedIDs[id]; failed {
				continue
			}
			account, err := s.accountRepo.GetByID(ctx, id)
			if err != nil || account == nil {
				continue
			}
			if !canUse(account) {
				continue
			}
			result, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
			if err != nil {
				return nil, excluded, err
			}
			if result == nil || !result.Acquired {
				// Fill first also applies under load: queue for the first healthy
				// reserve instead of spreading traffic across the later accounts.
				cfg := s.schedulingConfig()
				return attachSelectionProfitGate(ctx, &AccountSelectionResult{
					Account: account, JackReserveRemaining: len(plan.config.AccountIDs) - index,
					WaitPlan: &AccountWaitPlan{
						AccountID: account.ID, MaxConcurrency: account.Concurrency,
						Timeout: cfg.StickySessionWaitTimeout, MaxWaiting: cfg.StickySessionMaxWaiting,
					},
				}), excluded, nil
			}
			// Close the acquire/configuration race without bypassing group model,
			// privacy, capability, quota, runtime or profit gates on the target.
			fresh, readErr := s.accountRepo.GetByID(ctx, id)
			if readErr != nil || !canUse(fresh) || fresh.Concurrency != account.Concurrency {
				if result.ReleaseFunc != nil {
					result.ReleaseFunc()
				}
				continue
			}
			return attachSelectionProfitGate(ctx, &AccountSelectionResult{
				Account: fresh, JackReserveRemaining: len(plan.config.AccountIDs) - index,
				Acquired: true, ReleaseFunc: result.ReleaseFunc,
			}), excluded, nil
		}
	}
	return nil, excluded, nil
}
