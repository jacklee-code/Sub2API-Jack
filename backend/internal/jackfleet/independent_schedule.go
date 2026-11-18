package jackfleet

import (
	"context"
	"fmt"
	"time"
)

// ResetPreservedSchedules keeps the original weekly subscription cadence after
// an independent account reset. It needs no upstream read, and shares the fleet
// transaction/idempotency/cache path with other resets.
func (s *Store) ResetPreservedSchedules(ctx context.Context) (bool, error) {
	now := s.Now()
	var due bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id WHERE r.enabled AND f.archived_at IS NULL AND r.independent_reset_due_at<=$1)`, now).Scan(&due); err != nil || !due {
		return false, err
	}
	tx, err := s.locked(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT r.fleet_id FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id WHERE r.enabled AND f.archived_at IS NULL AND r.independent_reset_due_at<=$1 ORDER BY r.independent_reset_due_at,r.fleet_id LIMIT 100`, now)
	if err != nil {
		return false, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return false, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		v, e := scanSync(tx.QueryRowContext(ctx, `SELECT `+syncColumns+` FROM jack_fleet_reset_sync WHERE fleet_id=$1`, id))
		if e != nil {
			return false, e
		}
		key := fmt.Sprintf("fleet-independent-schedule:%d:%d:%d", id, v.Generation, v.IndependentResetDueAt.UnixNano())
		if e = s.resetEvent(ctx, tx, id, v.AccountID, key, "independent_schedule", nil, nil, now); e != nil {
			return false, e
		}
		// Advance from the promised boundary, never from worker execution time.
		// Downtime spanning several periods grants one current allowance. Leave
		// upstream observations/candidates intact: they run on a separate clock.
		const week = 7 * 24 * time.Hour
		next := v.IndependentResetDueAt.Add((now.Sub(*v.IndependentResetDueAt)/week + 1) * week)
		if _, e = tx.ExecContext(ctx, `UPDATE jack_fleet_reset_sync SET independent_reset_due_at=$2,next_reset_at=$2 WHERE fleet_id=$1`, id, next); e != nil {
			return false, e
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}
