package jackfleet

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (s *Store) locked(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7241060302)`); err == nil {
		_, err = tx.ExecContext(ctx, `SELECT set_config('jack.fleet_mutation','on',true)`)
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}
func jsonValue(v any) string { b, _ := json.Marshal(v); return string(b) }

const syncColumns = `enabled,account_id,generation,configured_at,baseline,candidate,next_reset_at,observed_at,status,error_code,rebaseline`

type rowScanner interface{ Scan(...any) error }

func scanSync(row rowScanner) (*ResetSync, error) {
	var v ResetSync
	var baseline, candidate []byte
	err := row.Scan(&v.Enabled, &v.AccountID, &v.Generation, &v.ConfiguredAt, &baseline, &candidate, &v.NextResetAt, &v.ObservedAt, &v.Status, &v.ErrorCode, &v.Rebaseline)
	if err != nil {
		return nil, err
	}
	if len(baseline) > 0 {
		if err = json.Unmarshal(baseline, &v.Baseline); err != nil {
			return nil, err
		}
	}
	if len(candidate) > 0 {
		if err = json.Unmarshal(candidate, &v.Candidate); err != nil {
			return nil, err
		}
	}
	return &v, nil
}
func (s *Store) ResetSync(ctx context.Context, id int64) (*ResetSync, error) {
	v, err := scanSync(s.DB.QueryRowContext(ctx, `SELECT `+syncColumns+` FROM jack_fleet_reset_sync WHERE fleet_id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return v, err
}
func (s *Store) configureResetSync(ctx context.Context, tx *sql.Tx, f Fleet, in Input, out *Result, now time.Time) error {
	var oldAccount int64
	var enabled bool
	err := tx.QueryRowContext(ctx, `SELECT account_id,enabled FROM jack_fleet_reset_sync WHERE fleet_id=$1`, f.ID).Scan(&oldAccount, &enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !in.SyncEnabled {
		if !enabled {
			return nil
		}
		if _, err = tx.ExecContext(ctx, `UPDATE jack_fleet_reset_sync SET enabled=false,generation=generation+1,candidate=NULL,status='disabled',configured_at=$2 WHERE fleet_id=$1`, f.ID, now); err != nil {
			return err
		}
		rows, e := tx.QueryContext(ctx, `UPDATE user_subscriptions s SET weekly_window_start=$2,updated_at=now() FROM jack_fleet_members m WHERE m.fleet_id=$1 AND m.subscription_id=s.id RETURNING s.user_id,s.group_id`, f.ID, now)
		if e != nil {
			return e
		}
		return collectPairs(rows, out)
	}
	if enabled && oldAccount == in.SyncAccountID {
		return nil
	}
	if in.SyncBaseline == nil || !in.SyncBaseline.Valid(now) {
		return bad("A fresh upstream seven-day baseline is required")
	}
	var eligible bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN account_groups ag ON ag.account_id=a.id WHERE a.id=$1 AND ag.group_id=$2 AND a.platform='openai' AND a.type='oauth' AND a.parent_account_id IS NULL AND a.deleted_at IS NULL AND a.status='active')`, in.SyncAccountID, f.GroupID).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return bad("Choose a ChatGPT OAuth parent account bound to this fleet's group")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO jack_fleet_reset_sync(fleet_id,account_id,enabled,baseline,next_reset_at,observed_at,configured_at) VALUES($1,$2,true,$3,$4,$5,$6) ON CONFLICT(fleet_id) DO UPDATE SET account_id=EXCLUDED.account_id,enabled=true,generation=jack_fleet_reset_sync.generation+1,baseline=EXCLUDED.baseline,candidate=NULL,next_reset_at=EXCLUDED.next_reset_at,observed_at=EXCLUDED.observed_at,configured_at=EXCLUDED.configured_at,status='watching',error_code='',rebaseline=false`, f.ID, in.SyncAccountID, jsonValue(in.SyncBaseline), in.SyncBaseline.ResetAt, in.SyncBaseline.At, now)
	if err != nil {
		return err
	}
	return collectFleetPairs(ctx, tx, f.ID, out)
}
func collectFleetPairs(ctx context.Context, tx *sql.Tx, id int64, out *Result) error {
	rows, err := tx.QueryContext(ctx, `SELECT s.user_id,s.group_id FROM jack_fleet_members m JOIN user_subscriptions s ON s.id=m.subscription_id WHERE m.fleet_id=$1 AND m.active`, id)
	if err != nil {
		return err
	}
	return collectPairs(rows, out)
}
func (s *Store) resetEvent(ctx context.Context, tx *sql.Tx, id, account int64, key, source string, before, after *Observation, now time.Time) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jack_fleet_reset_events WHERE fleet_id=$1 AND operation_key=$2)`, id, key).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	out := &Result{FleetID: id, Changed: []Pair{}, CachePending: true}
	if err := s.reset(ctx, tx, id, Input{Daily: true, Weekly: true, Monthly: true, upstreamReset: true}, out, now); err != nil {
		return err
	}
	if err := touch(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO jack_fleet_reset_events(fleet_id,account_id,source,operation_key,before_snapshot,after_snapshot,members) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, account, source, key, jsonValue(before), jsonValue(after), len(out.Changed)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO jack_fleet_operations(actor_id,operation_key,request_hash,result) VALUES(0,$1,$2,$3)`, key, key, jsonValue(out))
	return err
}

// Observe serializes detection with manual resets, transfers and source changes.
// Calls only carry real samples; every confirmation must be a newer observation.
func (s *Store) Observe(ctx context.Context, account int64, o Observation) error {
	now := s.Now()
	if !o.Valid(now) {
		return nil
	}
	tx, err := s.locked(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// A potentially successful credit redemption is a barrier for passive samples.
	var redeemPending bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jack_fleet_credit_operations WHERE account_id=$1 AND state='pending')`, account).Scan(&redeemPending); err != nil {
		return err
	}
	if redeemPending {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.fleet_id FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id WHERE r.account_id=$1 AND r.enabled AND f.archived_at IS NULL`, account)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		v, e := scanSync(tx.QueryRowContext(ctx, `SELECT `+syncColumns+` FROM jack_fleet_reset_sync WHERE fleet_id=$1`, id))
		if e != nil {
			return e
		}
		if !o.At.After(v.ConfiguredAt) || (v.ObservedAt != nil && !o.At.After(*v.ObservedAt)) {
			continue
		}
		var eligible bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN account_groups ag ON ag.account_id=a.id JOIN jack_fleets f ON f.group_id=ag.group_id WHERE f.id=$1 AND a.id=$2 AND a.deleted_at IS NULL AND a.parent_account_id IS NULL AND a.platform='openai' AND a.type='oauth' AND a.status='active')`, id, account).Scan(&eligible); e != nil {
			return e
		}
		if !eligible {
			if _, e = tx.ExecContext(ctx, `UPDATE jack_fleet_reset_sync SET status='unavailable',error_code='SOURCE_UNAVAILABLE',candidate=NULL WHERE fleet_id=$1`, id); e != nil {
				return e
			}
			continue
		}
		status := "watching"
		if v.Baseline == nil || v.Rebaseline || !sameIdentity(*v.Baseline, o) {
			v.Baseline = &o
			v.Candidate = nil
		} else if v.Candidate != nil {
			if v.Candidate.Kind == "early" && v.Status != "confirming" && o.Used >= 5 {
				v.Baseline = &o
				v.Candidate = nil
			} else if o.Source != "query" {
				// Keep the candidate and its observation watermark while its fresh
				// query is in flight. Busy request traffic must neither confirm the
				// reset nor make that query look out of order on arrival.
				continue
			} else if confirmsReset(*v.Candidate, o) {
				key := "fleet-sync:" + uuid.NewString()
				if e = s.resetEvent(ctx, tx, id, account, key, v.Candidate.Kind, &v.Candidate.Before, &o, now); e != nil {
					return e
				}
				v.Candidate = nil
				v.Baseline = &o
				status = "synced"
			} else {
				// Retain a rejected candidate rather than querying repeatedly for
				// every low request header. A later explicit query may confirm it,
				// or a new high observation can establish a new baseline.
				status = "needs_review"
			}
		} else if kind := detectReset(*v.Baseline, o); kind != "" {
			v.Candidate = &ResetCandidate{Before: *v.Baseline, First: o, Kind: kind}
			status = "confirming"
			if e = queueResetJob(ctx, tx, account, now); e != nil {
				return e
			}
		} else {
			v.Baseline = &o
		}
		if o.Plan == "" && v.Baseline != nil {
			o.Plan = v.Baseline.Plan
		}
		if _, e = tx.ExecContext(ctx, `UPDATE jack_fleet_reset_sync SET baseline=$2,candidate=$3,next_reset_at=$4,observed_at=$5,status=$6,error_code='',rebaseline=false WHERE fleet_id=$1`, id, jsonValue(v.Baseline), jsonValue(v.Candidate), o.ResetAt, o.At, status); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func queueResetJob(ctx context.Context, tx *sql.Tx, account int64, due time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO jack_fleet_reset_jobs(account_id,due_at) VALUES($1,$2) ON CONFLICT(account_id) DO UPDATE SET due_at=LEAST(jack_fleet_reset_jobs.due_at,EXCLUDED.due_at)`, account, due)
	return err
}
func (s *Store) QueueQuotaCheck(ctx context.Context, subscription int64) error {
	// No upstream work for invalid subscriptions, key quotas or unauthenticated traffic.
	_, err := s.DB.ExecContext(ctx, `INSERT INTO jack_fleet_reset_jobs(account_id,due_at) SELECT r.account_id,$2::timestamptz FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id JOIN jack_fleet_members m ON m.fleet_id=f.id JOIN user_subscriptions s ON s.id=m.subscription_id WHERE m.subscription_id=$1 AND m.active AND r.enabled AND f.archived_at IS NULL AND s.deleted_at IS NULL AND s.status='active' AND s.expires_at>$2 AND (r.observed_at IS NULL OR r.observed_at<=$2::timestamptz-interval '60 seconds' OR r.candidate IS NOT NULL AND r.candidate<>'null'::jsonb) ON CONFLICT(account_id) DO UPDATE SET due_at=LEAST(jack_fleet_reset_jobs.due_at,EXCLUDED.due_at)`, subscription, s.Now())
	return err
}

// ClaimCheck claims only event-created jobs, never scans idle upstream accounts.
func (s *Store) ClaimCheck(ctx context.Context) (int64, error) {
	now := s.Now()
	var id int64
	// A detected reset bypasses the ordinary quota-check cooldown. The lease
	// still coalesces concurrent signals; failed/mismatched reads leave the
	// confirming state, so they cannot create a rapid retry loop.
	err := s.DB.QueryRowContext(ctx, `UPDATE jack_fleet_reset_jobs SET lease_until=$1::timestamptz+interval '30 seconds',last_attempt_at=$1,due_at='infinity' WHERE account_id=(SELECT j.account_id FROM jack_fleet_reset_jobs j WHERE (due_at<=$1 OR lease_until<=$1) AND (lease_until IS NULL OR lease_until<=$1) AND (last_attempt_at IS NULL OR last_attempt_at<=$1::timestamptz-interval '60 seconds' OR EXISTS(SELECT 1 FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id WHERE r.account_id=j.account_id AND r.enabled AND f.archived_at IS NULL AND r.status='confirming' AND r.candidate IS NOT NULL AND r.candidate<>'null'::jsonb)) AND EXISTS(SELECT 1 FROM jack_fleet_reset_sync r JOIN jack_fleets f ON f.id=r.fleet_id JOIN account_groups ag ON ag.group_id=f.group_id AND ag.account_id=r.account_id JOIN accounts a ON a.id=r.account_id WHERE r.account_id=j.account_id AND r.enabled AND f.archived_at IS NULL AND a.deleted_at IS NULL AND a.platform='openai' AND a.type='oauth' AND a.parent_account_id IS NULL AND a.status='active') ORDER BY due_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING account_id`, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}
func (s *Store) FinishCheck(ctx context.Context, account int64, code string) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE jack_fleet_reset_jobs SET lease_until=NULL WHERE account_id=$1`, account); err != nil {
		return err
	}
	if code != "" {
		_, err := s.DB.ExecContext(ctx, `UPDATE jack_fleet_reset_sync SET status='unavailable',error_code=$2 WHERE account_id=$1 AND enabled`, account, code)
		return err
	}
	return nil
}
func (s *Store) ResetEvents(ctx context.Context, id, before int64, limit int) ([]ResetEvent, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.account_id,e.source,e.before_snapshot,e.after_snapshot,e.members,e.created_at,COALESCE(o.cache_pending,false) FROM jack_fleet_reset_events e LEFT JOIN jack_fleet_operations o ON o.actor_id=0 AND o.operation_key=e.operation_key WHERE e.fleet_id=$1 AND ($2=0 OR e.id<$2) ORDER BY e.id DESC LIMIT $3`, id, before, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := []ResetEvent{}
	for rows.Next() {
		var e ResetEvent
		var b, a []byte
		if err = rows.Scan(&e.ID, &e.AccountID, &e.Source, &b, &a, &e.Members, &e.CreatedAt, &e.CachePending); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &e.Before); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(a, &e.After); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// Replay lets handlers avoid repeating upstream baseline reads on a lost response.
func (s *Store) Replay(ctx context.Context, actor int64, key string, in Input) (*Result, error) {
	var hash string
	var body []byte
	var pending bool
	err := s.DB.QueryRowContext(ctx, `SELECT request_hash,result,cache_pending FROM jack_fleet_operations WHERE actor_id=$1 AND operation_key=$2`, actor, key).Scan(&hash, &body, &pending)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if hash != inputHash(in) {
		return nil, conflict("Idempotency key was already used with different input")
	}
	var out Result
	if err = json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	out.Replayed = true
	out.CachePending = pending
	return &out, nil
}
func inputHash(in Input) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(jsonValue(in)))) }
