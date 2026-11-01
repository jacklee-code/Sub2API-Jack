package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Stored in the existing error field so older binaries also exclude the account.
// Keep this stable: only an explicit administrator recovery may remove it.
const JackOpenAICreditExhaustedMessage = "[jack_openai_credit_exhausted] OpenAI API credit balance exhausted; use Restore status after replenishing credits."

func IsJackOpenAICreditExhausted(account *Account) bool {
	return account != nil && account.ErrorMessage == JackOpenAICreditExhaustedMessage
}

func jackOpenAICreditExhaustedCode(body []byte) bool {
	if !gjson.ValidBytes(body) {
		return false
	}
	for _, path := range []string{"error.code", "response.error.code", "code"} {
		if gjson.GetBytes(body, path).String() == "credit_balance_exhausted" {
			return true
		}
	}
	return false
}

func isJackOfficialOpenAICreditExhaustion(account *Account, status int, body []byte) bool {
	if account == nil || !account.IsOpenAIApiKey() || (status != http.StatusTooManyRequests && status != http.StatusPaymentRequired) {
		return false
	}
	base, err := url.Parse(account.GetOpenAIBaseURL())
	return err == nil && base.Scheme == "https" && strings.EqualFold(base.Hostname(), "api.openai.com") &&
		(base.Port() == "" || base.Port() == "443") && base.User == nil && jackOpenAICreditExhaustedCode(body)
}

// This precedes pool/custom temporary rules: a confirmed empty balance cannot
// recover after a short timer. The ordinary error write updates scheduler caches.
func (s *RateLimitService) handleJackOpenAICreditExhaustion(ctx context.Context, account *Account, status int, body []byte) bool {
	if s == nil || !isJackOfficialOpenAICreditExhaustion(account, status, body) {
		return false
	}
	opCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	s.notifyAccountSchedulingBlocked(account, time.Time{}, "jack_openai_credit_exhausted")
	if s.accountRepo != nil {
		if err := s.accountRepo.SetError(opCtx, account.ID, JackOpenAICreditExhaustedMessage); err != nil {
			slog.Error("jack_openai_credit_exhaustion_persist_failed", "account_id", account.ID, "error", err)
		}
	}
	return true
}

type jackCreditExhaustionRepository interface {
	RestoreJackOpenAICreditExhausted(context.Context, int64) (bool, error)
}

func restoreJackOpenAICreditExhausted(ctx context.Context, repo AccountRepository, id int64) error {
	r, ok := repo.(jackCreditExhaustionRepository)
	if !ok {
		return fmt.Errorf("account repository does not support credit exhaustion recovery")
	}
	restored, err := r.RestoreJackOpenAICreditExhausted(ctx, id)
	if err != nil {
		return err
	}
	if !restored {
		return fmt.Errorf("account state changed; reload before restoring status")
	}
	return nil
}
