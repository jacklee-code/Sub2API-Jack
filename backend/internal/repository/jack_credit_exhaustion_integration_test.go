//go:build integration

package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

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

func (s *AccountRepoSuite) TestJackCreditRestoreOutboxFailureRollsBack() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "exhausted-credit-rollback", Status: service.StatusError,
		ErrorMessage: service.JackOpenAICreditExhaustedMessage, Schedulable: false,
	})
	s.repo.sql = &failAtomicSchedulerOutboxSQLExecutor{sqlExecutor: s.repo.sql}
	restored, err := s.repo.RestoreJackOpenAICreditExhausted(s.ctx, account.ID)
	s.Require().Error(err)
	s.Require().False(restored)
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().False(got.Schedulable)
	s.Require().True(service.IsJackOpenAICreditExhausted(got))
}
