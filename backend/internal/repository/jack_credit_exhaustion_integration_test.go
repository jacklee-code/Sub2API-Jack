//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func (s *AccountRepoSuite) TestJackCreditExhaustionRequiresExplicitRestore() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "exhausted-credit", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
	})
	cache := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cache
	s.Require().NoError(s.repo.SetError(s.ctx, account.ID, service.JackOpenAICreditExhaustedMessage))
	// In-flight errors and automatic recovery must neither replace nor clear
	// the hold, even when they have read an older active/error snapshot.
	s.Require().NoError(s.repo.SetError(s.ctx, account.ID, "late transient failure"))
	s.Require().NoError(s.repo.ClearError(s.ctx, account.ID))
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().False(got.Schedulable)
	s.Require().True(service.IsJackOpenAICreditExhausted(got))
	s.Require().Equal(service.StatusError, cache.accounts[account.ID].Status)

	restored, err := s.repo.RestoreJackOpenAICreditExhausted(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().True(restored)
	got, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, got.Status)
	s.Require().True(got.Schedulable)
	s.Require().Empty(got.ErrorMessage)
	s.Require().True(cache.accounts[account.ID].Schedulable)

	// A stale recovery must not erase a subsequent unrelated error.
	s.Require().NoError(s.repo.SetError(s.ctx, account.ID, "invalid key"))
	restored, err = s.repo.RestoreJackOpenAICreditExhausted(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().False(restored)
	got, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("invalid key", got.ErrorMessage)
}

func TestJackCreditRestoreOutboxFailureRollsBack(t *testing.T) {
	// Use autocommit, as in production: a deliberately failed statement aborts
	// a suite transaction, making readback impossible until rollback. Keeping
	// this independent also proves the CTE itself is atomic, without a test
	// savepoint rolling back an otherwise partially successful restore.
	ctx := context.Background()
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{
		Name: "exhausted-credit-rollback", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusError,
		ErrorMessage: service.JackOpenAICreditExhaustedMessage, Schedulable: false,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_ = client.Account.DeleteOneID(account.ID).Exec(context.Background())
	})
	// mustCreateAccount defaults scheduling to true, even when false is supplied.
	// Set the real held state explicitly before provoking the failed restore.
	require.NoError(t, client.Account.UpdateOneID(account.ID).SetSchedulable(false).Exec(ctx))
	repo := newAccountRepositoryWithSQL(client, &failAtomicSchedulerOutboxSQLExecutor{sqlExecutor: integrationDB}, nil)
	restored, err := repo.RestoreJackOpenAICreditExhausted(ctx, account.ID)
	require.Error(t, err)
	require.False(t, restored)
	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusError, got.Status)
	require.False(t, got.Schedulable)
	require.True(t, service.IsJackOpenAICreditExhausted(got))
	var events int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", account.ID).Scan(&events))
	require.Zero(t, events)
}
