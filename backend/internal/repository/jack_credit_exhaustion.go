package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Restore only this hold, atomically restoring scheduling with the status. A
// concurrent unrelated error must not be erased by a stale recovery request.
func (r *accountRepository) RestoreJackOpenAICreditExhausted(ctx context.Context, id int64) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.RestoreJackOpenAICreditExhausted(ctx, id)
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
			UPDATE accounts SET status = $1, error_message = '', schedulable = TRUE, updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL AND status = $3 AND error_message = $4
			RETURNING id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $5, id, NULL, NULL FROM updated
	`, service.StatusActive, id, service.StatusError, service.JackOpenAICreditExhaustedMessage, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}
