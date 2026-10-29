package jackfleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CreditOperation struct {
	Identity  string
	ID        int64
	AccountID int64
	RedeemID  string
	State     string
	Result    []byte
}

// BeginCredit durably fixes one upstream redemption id BEFORE spending a credit.
// Retries of a timed-out operation must use that same id, including after restart.
func (s *Store) BeginCredit(ctx context.Context, actor, account int64, key string, identity ...string) (*CreditOperation, error) {
	if actor <= 0 || len(key) < 8 || len(key) > 128 {
		return nil, bad("An Idempotency-Key (8-128 characters) is required")
	}
	tx, err := s.locked(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	now := s.Now()
	op := &CreditOperation{AccountID: account}
	if len(identity) > 0 {
		op.Identity = identity[0]
	}
	var lease *time.Time
	err = tx.QueryRowContext(ctx, `SELECT id,redeem_id,state,result,lease_until,account_identity FROM jack_fleet_credit_operations WHERE actor_id=$1 AND account_id=$2 AND operation_key=$3`, actor, account, key).Scan(&op.ID, &op.RedeemID, &op.State, &op.Result, &lease, &op.Identity)
	if err == nil {
		if op.State != "pending" {
			return op, nil
		}
		if lease != nil && lease.After(now) {
			return nil, conflict("Credit redemption is in progress; retry the same operation")
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		var busy bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jack_fleet_credit_operations WHERE account_id=$1 AND state='pending')`, account).Scan(&busy); err != nil {
			return nil, err
		}
		if busy {
			return nil, conflict("A previous credit redemption is unresolved; retry that operation")
		}
		rows, e := tx.QueryContext(ctx, `SELECT r.fleet_id,r.generation FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id WHERE r.account_id=$1 AND r.enabled AND f.archived_at IS NULL`, account)
		if e != nil {
			return nil, e
		}
		targets := map[int64]int64{}
		for rows.Next() {
			var id, g int64
			if e = rows.Scan(&id, &g); e != nil {
				_ = rows.Close()
				return nil, e
			}
			targets[id] = g
		}
		e = rows.Err()
		_ = rows.Close()
		if e != nil {
			return nil, e
		}
		op.RedeemID = uuid.NewString()
		op.State = "pending"
		if err = tx.QueryRowContext(ctx, `INSERT INTO jack_fleet_credit_operations(actor_id,account_id,operation_key,redeem_id,targets,account_identity) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, actor, account, key, op.RedeemID, jsonValue(targets), op.Identity).Scan(&op.ID); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jack_fleet_credit_operations SET lease_until=$2::timestamptz+interval '30 seconds' WHERE id=$1`, op.ID, now); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return op, nil
}
func (s *Store) ClaimCredit(ctx context.Context) (*CreditOperation, error) {
	now := s.Now()
	var op CreditOperation
	err := s.DB.QueryRowContext(ctx, `UPDATE jack_fleet_credit_operations SET lease_until=$1::timestamptz+interval '30 seconds' WHERE id=(SELECT id FROM jack_fleet_credit_operations WHERE state='pending' AND next_attempt_at<=$1 AND (lease_until IS NULL OR lease_until<=$1) ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id,account_id,redeem_id,state,account_identity`, now).Scan(&op.ID, &op.AccountID, &op.RedeemID, &op.State, &op.Identity)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &op, err
}
func (s *Store) RetryCredit(ctx context.Context, id int64, code string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jack_fleet_credit_operations SET lease_until=NULL,next_attempt_at=$2::timestamptz+interval '60 seconds',error_code=$3 WHERE id=$1 AND state='pending'`, id, s.Now(), code)
	return err
}
func CreditSucceeded(code string, windows int) bool {
	if windows <= 0 {
		return false
	}
	// The consume endpoint reports a completed redemption as "reset"; retain
	// the existing success aliases without accepting unknown or no-op outcomes.
	switch strings.ToLower(code) {
	case "reset", "ok", "success":
		return true
	default:
		return false
	}
}
func (s *Store) CompleteCredit(ctx context.Context, op *CreditOperation, code string, windows int, result []byte) error {
	tx, err := s.locked(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	var targetsJSON []byte
	if err = tx.QueryRowContext(ctx, `SELECT state,targets FROM jack_fleet_credit_operations WHERE id=$1 FOR UPDATE`, op.ID).Scan(&state, &targetsJSON); err != nil {
		return err
	}
	if state != "pending" {
		return nil
	}
	now := s.Now()
	identityMatches := true
	if op.Identity != "" {
		var account, organization, email string
		e := tx.QueryRowContext(ctx, `SELECT COALESCE(credentials->>'chatgpt_account_id',''),COALESCE(credentials->>'organization_id',''),COALESCE(credentials->>'email','') FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, op.AccountID).Scan(&account, &organization, &email)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		identityMatches = e == nil && SourceIdentity(op.AccountID, account, organization, email) == op.Identity
	}
	if CreditSucceeded(code, windows) && identityMatches {
		targets := map[int64]int64{}
		if err = json.Unmarshal(targetsJSON, &targets); err != nil {
			return err
		}
		for id, generation := range targets {
			v, e := scanSync(tx.QueryRowContext(ctx, `SELECT `+syncColumns+` FROM jack_fleet_reset_sync WHERE fleet_id=$1`, id))
			if errors.Is(e, sql.ErrNoRows) {
				continue
			}
			if e != nil {
				return e
			}
			if !v.Enabled || v.AccountID != op.AccountID || v.Generation != generation {
				continue
			}
			var bound bool
			if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jack_fleets f JOIN account_groups ag ON ag.group_id=f.group_id WHERE f.id=$1 AND ag.account_id=$2 AND f.archived_at IS NULL)`, id, op.AccountID).Scan(&bound); e != nil {
				return e
			}
			if !bound {
				continue
			}
			key := fmt.Sprintf("fleet-credit:%d:%d", op.ID, id)
			if e = s.resetEvent(ctx, tx, id, op.AccountID, key, "manual_credit", v.Baseline, nil, now); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE jack_fleet_reset_sync SET candidate=NULL,rebaseline=true,configured_at=$2,status='synced',error_code='' WHERE fleet_id=$1`, id, now); e != nil {
				return e
			}
		}
		// Only a one-shot post-redemption read, not periodic monitoring.
		if err = queueResetJob(ctx, tx, op.AccountID, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jack_fleet_credit_operations SET state='complete',result=$2,completed_at=$3,lease_until=NULL,error_code='' WHERE id=$1`, op.ID, string(result), now); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CreditSyncStatus(ctx context.Context, id int64) (string, int, error) {
	var count, pending int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE o.cache_pending) FROM jack_fleet_reset_events e JOIN jack_fleet_operations o ON o.actor_id=0 AND o.operation_key=e.operation_key WHERE e.operation_key LIKE $1`, fmt.Sprintf("fleet-credit:%d:%%", id)).Scan(&count, &pending)
	status := "not_enabled"
	if count > 0 {
		status = "synced"
	}
	if pending > 0 {
		status = "pending"
	}
	return status, count, err
}

// RejectCredit is only for a definitive rejection, never an ambiguous transport
// failure after the upstream may already have consumed the persisted request id.
func (s *Store) RejectCredit(ctx context.Context, id int64, code string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jack_fleet_credit_operations SET state='failed',lease_until=NULL,error_code=$2,completed_at=$3 WHERE id=$1 AND state='pending'`, id, code, s.Now())
	return err
}
