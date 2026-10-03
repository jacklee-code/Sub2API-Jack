// Package jackfleet manages fleet membership while preserving upstream billing rows.
package jackfleet

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type Store struct {
	DB  *sql.DB
	Now func() time.Time
}

func NewStore(db *sql.DB) *Store { return &Store{DB: db, Now: time.Now} }

type Member struct {
	UserID         int64      `json:"user_id"`
	Email          string     `json:"email"`
	Username       string     `json:"username"`
	Role           string     `json:"role"`
	SubscriptionID int64      `json:"subscription_id"`
	Independent    bool       `json:"independent_expiry"`
	Active         bool       `json:"active"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Status         string     `json:"status"`
	Daily          float64    `json:"daily_usage_usd"`
	Weekly         float64    `json:"weekly_usage_usd"`
	Monthly        float64    `json:"monthly_usage_usd"`
	DailyStart     *time.Time `json:"daily_window_start"`
	WeeklyStart    *time.Time `json:"weekly_window_start"`
	MonthlyStart   *time.Time `json:"monthly_window_start"`
	KeyCount       int        `json:"key_count"`
	OwnerFleetID   *int64     `json:"owner_fleet_id,omitempty"`
}
type Fleet struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	GroupID      int64     `json:"group_id"`
	GroupName    string    `json:"group_name"`
	Platform     string    `json:"platform"`
	ExpiresAt    time.Time `json:"expires_at"`
	Version      int64     `json:"version"`
	DailyLimit   *float64  `json:"daily_limit_usd"`
	WeeklyLimit  *float64  `json:"weekly_limit_usd"`
	MonthlyLimit *float64  `json:"monthly_limit_usd"`
	Members      []Member  `json:"members"`
}
type MemberInput struct {
	UserID      int64      `json:"user_id"`
	Independent bool       `json:"independent_expiry"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Adopt       bool       `json:"adopt_existing"`
}
type Input struct {
	Action    string          `json:"action"`
	FleetID   int64           `json:"fleet_id,omitempty"`
	TargetID  int64           `json:"target_fleet_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	GroupID   int64           `json:"group_id,omitempty"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	Days      int             `json:"days,omitempty"`
	Members   []MemberInput   `json:"members,omitempty"`
	UserID    int64           `json:"user_id,omitempty"`
	Daily     bool            `json:"daily,omitempty"`
	Weekly    bool            `json:"weekly,omitempty"`
	Monthly   bool            `json:"monthly,omitempty"`
	Versions  map[int64]int64 `json:"versions,omitempty"`
}
type Pair struct {
	UserID  int64 `json:"user_id"`
	GroupID int64 `json:"group_id"`
}
type Result struct {
	FleetID      int64  `json:"fleet_id"`
	Changed      []Pair `json:"changed"`
	CachePending bool   `json:"cache_pending"`
	Replayed     bool   `json:"replayed"`
}

func bad(message string) error      { return apperrors.BadRequest("INVALID_FLEET_OPERATION", message) }
func conflict(message string) error { return apperrors.Conflict("FLEET_CONFLICT", message) }
func missing() error                { return apperrors.NotFound("FLEET_NOT_FOUND", "Fleet or member was not found") }
func validExpiry(t time.Time) bool  { return t.Year() >= 2000 && t.Year() <= 2099 }

func (s *Store) List(ctx context.Context) ([]Fleet, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT f.id,f.name,f.group_id,g.name,g.platform,f.expires_at,f.version,g.daily_limit_usd,g.weekly_limit_usd,g.monthly_limit_usd FROM jack_fleets f JOIN groups g ON g.id=f.group_id WHERE f.archived_at IS NULL ORDER BY f.id`)
	if err != nil {
		return nil, err
	}
	fleets := []Fleet{}
	for rows.Next() {
		var f Fleet
		if err = rows.Scan(&f.ID, &f.Name, &f.GroupID, &f.GroupName, &f.Platform, &f.ExpiresAt, &f.Version, &f.DailyLimit, &f.WeeklyLimit, &f.MonthlyLimit); err != nil {
			_ = rows.Close()
			return nil, err
		}
		fleets = append(fleets, f)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range fleets {
		fleets[i].Members, err = s.members(ctx, fleets[i].GroupID, &fleets[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return fleets, nil
}
func (s *Store) Preview(ctx context.Context, groupID int64) ([]Member, error) {
	return s.members(ctx, groupID, nil)
}
func (s *Store) members(ctx context.Context, groupID int64, fleetID *int64) ([]Member, error) {
	query := `SELECT s.user_id,u.email,COALESCE(u.username,''),u.role,s.id,COALESCE(m.independent_expiry,false),COALESCE(m.active,true),s.expires_at,s.status,s.daily_usage_usd,s.weekly_usage_usd,s.monthly_usage_usd,s.daily_window_start,s.weekly_window_start,s.monthly_window_start,(SELECT count(*) FROM api_keys k WHERE k.user_id=s.user_id AND k.group_id=s.group_id AND k.deleted_at IS NULL),m.fleet_id FROM user_subscriptions s JOIN users u ON u.id=s.user_id LEFT JOIN jack_fleet_members m ON m.subscription_id=s.id WHERE s.group_id=$1 AND s.deleted_at IS NULL AND u.deleted_at IS NULL`
	args := []any{groupID}
	if fleetID != nil {
		query += ` AND m.fleet_id=$2`
		args = append(args, *fleetID)
	}
	rows, err := s.DB.QueryContext(ctx, query+` ORDER BY s.user_id`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []Member{}
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.UserID, &m.Email, &m.Username, &m.Role, &m.SubscriptionID, &m.Independent, &m.Active, &m.ExpiresAt, &m.Status, &m.Daily, &m.Weekly, &m.Monthly, &m.DailyStart, &m.WeeklyStart, &m.MonthlyStart, &m.KeyCount, &m.OwnerFleetID); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// Mutate records the operation result in the same transaction as every business
// write. A lost response or failed cache invalidation cannot double-extend/reset.
func (s *Store) Mutate(ctx context.Context, actor int64, key string, in Input) (*Result, error) {
	if actor <= 0 || len(key) < 8 || len(key) > 128 {
		return nil, bad("An Idempotency-Key (8-128 characters) is required")
	}
	if len(in.Members) > 200 {
		return nil, bad("At most 200 members per request")
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(body))
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Fleets are small admin-managed sets. One advisory lock gives all operations,
	// including reset-all and cross-fleet transfers, a consistent lock ordering.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7241060302)`); err != nil {
		return nil, err
	}
	var oldHash string
	var oldResult []byte
	var pending bool
	err = tx.QueryRowContext(ctx, `SELECT request_hash,result,cache_pending FROM jack_fleet_operations WHERE actor_id=$1 AND operation_key=$2`, actor, key).Scan(&oldHash, &oldResult, &pending)
	if err == nil {
		if oldHash != hash {
			return nil, conflict("Idempotency key was already used with different input")
		}
		var out Result
		if err = json.Unmarshal(oldResult, &out); err != nil {
			return nil, err
		}
		out.Replayed = true
		out.CachePending = pending
		return &out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('jack.fleet_mutation','on',true)`); err != nil {
		return nil, err
	}
	out := &Result{FleetID: in.FleetID, Changed: []Pair{}, CachePending: true}
	now := s.Now()
	if err = s.apply(ctx, tx, actor, in, out, now); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO jack_fleet_operations(actor_id,operation_key,request_hash,result) VALUES($1,$2,$3,$4)`, actor, key, hash, string(encoded)); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Store) Pending(ctx context.Context) ([]struct {
	Actor  int64
	Key    string
	Result Result
}, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT actor_id,operation_key,result FROM jack_fleet_operations WHERE cache_pending ORDER BY created_at LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []struct {
		Actor  int64
		Key    string
		Result Result
	}{}
	for rows.Next() {
		var r struct {
			Actor  int64
			Key    string
			Result Result
		}
		var data []byte
		if err = rows.Scan(&r.Actor, &r.Key, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &r.Result); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (s *Store) MarkSynced(ctx context.Context, actor int64, key string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jack_fleet_operations SET cache_pending=false WHERE actor_id=$1 AND operation_key=$2`, actor, key)
	return err
}

func getFleet(ctx context.Context, tx *sql.Tx, id int64, versions map[int64]int64) (Fleet, error) {
	var f Fleet
	err := tx.QueryRowContext(ctx, `SELECT f.id,f.group_id,f.expires_at,f.version,g.platform FROM jack_fleets f JOIN groups g ON g.id=f.group_id WHERE f.id=$1 AND f.archived_at IS NULL AND g.deleted_at IS NULL FOR UPDATE OF f`, id).Scan(&f.ID, &f.GroupID, &f.ExpiresAt, &f.Version, &f.Platform)
	if errors.Is(err, sql.ErrNoRows) {
		return f, missing()
	}
	if err != nil {
		return f, err
	}
	if versions[id] != f.Version {
		return f, conflict("Fleet changed; refresh before retrying")
	}
	return f, nil
}
func touch(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE jack_fleets SET version=version+1,updated_at=now() WHERE id=$1`, id)
	return err
}
func (s *Store) apply(ctx context.Context, tx *sql.Tx, actor int64, in Input, out *Result, now time.Time) error {
	if in.Action == "create" {
		name := strings.TrimSpace(in.Name)
		if name == "" || len([]rune(name)) > 100 || in.ExpiresAt == nil || !validExpiry(*in.ExpiresAt) {
			return bad("Name and valid expiry are required")
		}
		var typ, status string
		if err := tx.QueryRowContext(ctx, `SELECT subscription_type,status FROM groups WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, in.GroupID).Scan(&typ, &status); err != nil {
			return bad("Group not found")
		}
		if typ != "subscription" || status != "active" {
			return bad("Choose an active subscription group")
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jack_fleets WHERE group_id=$1 AND archived_at IS NULL)`, in.GroupID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return conflict("This group already has a fleet")
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO jack_fleets(name,group_id,expires_at) VALUES($1,$2,$3) RETURNING id`, name, in.GroupID, *in.ExpiresAt).Scan(&out.FleetID); err != nil {
			return err
		}
		f := Fleet{ID: out.FleetID, GroupID: in.GroupID, ExpiresAt: *in.ExpiresAt}
		for _, m := range in.Members {
			if err := s.join(ctx, tx, actor, f, m, out, now); err != nil {
				return err
			}
		}
		return nil
	}
	if in.Action == "reset_all" {
		if !in.Daily && !in.Weekly && !in.Monthly {
			return bad("Select at least one quota window")
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,version FROM jack_fleets WHERE archived_at IS NULL ORDER BY id FOR UPDATE`)
		if err != nil {
			return err
		}
		ids := []int64{}
		for rows.Next() {
			var id, v int64
			if err = rows.Scan(&id, &v); err != nil {
				_ = rows.Close()
				return err
			}
			if in.Versions[id] != v {
				_ = rows.Close()
				return conflict("Fleet list changed; refresh before retrying")
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if len(ids) != len(in.Versions) {
			return conflict("Fleet list changed; refresh before retrying")
		}
		for _, id := range ids {
			if err = s.reset(ctx, tx, id, in, out, now); err != nil {
				return err
			}
			if err = touch(ctx, tx, id); err != nil {
				return err
			}
		}
		return nil
	}
	f, err := getFleet(ctx, tx, in.FleetID, in.Versions)
	if err != nil {
		return err
	}
	switch in.Action {
	case "rename":
		name := strings.TrimSpace(in.Name)
		if name == "" || len([]rune(name)) > 100 {
			return bad("Name must contain 1-100 characters")
		}
		_, err = tx.ExecContext(ctx, `UPDATE jack_fleets SET name=$2 WHERE id=$1`, f.ID, name)
	case "add":
		if len(in.Members) == 0 {
			return bad("Choose at least one member")
		}
		for _, m := range in.Members {
			if err = s.join(ctx, tx, actor, f, m, out, now); err != nil {
				return err
			}
		}
	case "remove":
		err = s.leave(ctx, tx, f, in.UserID, out)
	case "transfer":
		if in.TargetID == f.ID {
			return nil
		}
		target, e := getFleet(ctx, tx, in.TargetID, in.Versions)
		if e != nil {
			return e
		}
		if target.Platform != f.Platform {
			return bad("API keys can only be moved between fleets on the same platform")
		}
		if err = s.leave(ctx, tx, f, in.UserID, out); err != nil {
			return err
		}
		m := MemberInput{UserID: in.UserID}
		if len(in.Members) > 0 {
			m = in.Members[0]
			if m.UserID != in.UserID {
				return bad("Transfer member does not match")
			}
		}
		if err = s.join(ctx, tx, actor, target, m, out, now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE api_keys SET group_id=$3,updated_at=now() WHERE user_id=$1 AND group_id=$2 AND deleted_at IS NULL`, in.UserID, f.GroupID, target.GroupID); err != nil {
			return err
		}
		err = touch(ctx, tx, target.ID)
	case "member_expiry":
		if len(in.Members) != 1 {
			return bad("Choose one member")
		}
		m := in.Members[0]
		expiry := f.ExpiresAt
		if m.Independent {
			if m.ExpiresAt == nil || !validExpiry(*m.ExpiresAt) {
				return bad("Independent expiry is required")
			}
			expiry = *m.ExpiresAt
		}
		var subID int64
		if err = tx.QueryRowContext(ctx, `UPDATE jack_fleet_members SET independent_expiry=$3 WHERE fleet_id=$1 AND user_id=$2 AND active RETURNING subscription_id`, f.ID, m.UserID, m.Independent).Scan(&subID); errors.Is(err, sql.ErrNoRows) {
			return missing()
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET expires_at=$2,status=CASE WHEN $2>now() THEN 'active' ELSE 'expired' END,updated_at=now() WHERE id=$1`, subID, expiry)
		out.Changed = append(out.Changed, Pair{m.UserID, f.GroupID})
	case "expiry":
		expiry, e := adjustExpiry(f.ExpiresAt, now, in.Days, in.ExpiresAt)
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, `UPDATE jack_fleets SET expires_at=$2 WHERE id=$1`, f.ID, expiry); err != nil {
			return err
		}
		rows, e := tx.QueryContext(ctx, `UPDATE user_subscriptions s SET expires_at=$2,status=CASE WHEN $2>now() THEN 'active' ELSE 'expired' END,updated_at=now() FROM jack_fleet_members m WHERE m.fleet_id=$1 AND m.active AND NOT m.independent_expiry AND m.subscription_id=s.id RETURNING s.user_id,s.group_id`, f.ID, expiry)
		if e != nil {
			return e
		}
		err = collectPairs(rows, out)
	case "reset":
		err = s.reset(ctx, tx, f.ID, in, out, now)
	case "archive":
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM jack_fleet_members WHERE fleet_id=$1 AND active`, f.ID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return conflict("Remove all members before archiving")
		}
		_, err = tx.ExecContext(ctx, `UPDATE jack_fleets SET archived_at=now() WHERE id=$1`, f.ID)
	default:
		return bad("Unknown fleet action")
	}
	if err != nil {
		return err
	}
	return touch(ctx, tx, f.ID)
}
func adjustExpiry(old, now time.Time, days int, exact *time.Time) (time.Time, error) {
	if exact != nil {
		if days != 0 || !validExpiry(*exact) {
			return time.Time{}, bad("Choose a valid date or days, not both")
		}
		return *exact, nil
	}
	if days == 0 || days < -36500 || days > 36500 {
		return time.Time{}, bad("Days must be nonzero and within 36500")
	}
	if days > 0 && old.Before(now) {
		old = now
	}
	next := old.AddDate(0, 0, days)
	if !validExpiry(next) {
		return time.Time{}, bad("Expiry must be between years 2000 and 2099")
	}
	return next, nil
}
func collectPairs(rows *sql.Rows, out *Result) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var p Pair
		if err := rows.Scan(&p.UserID, &p.GroupID); err != nil {
			return err
		}
		out.Changed = append(out.Changed, p)
	}
	return rows.Err()
}
func (s *Store) leave(ctx context.Context, tx *sql.Tx, f Fleet, userID int64, out *Result) error {
	var subID int64
	err := tx.QueryRowContext(ctx, `UPDATE jack_fleet_members SET active=false,left_at=now() WHERE fleet_id=$1 AND user_id=$2 AND active RETURNING subscription_id`, f.ID, userID).Scan(&subID)
	if errors.Is(err, sql.ErrNoRows) {
		return missing()
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET status='suspended',updated_at=now() WHERE id=$1`, subID)
	out.Changed = append(out.Changed, Pair{userID, f.GroupID})
	return err
}
func (s *Store) reset(ctx context.Context, tx *sql.Tx, id int64, in Input, out *Result, now time.Time) error {
	if !in.Daily && !in.Weekly && !in.Monthly {
		return bad("Select at least one quota window")
	}
	day := timezone.StartOfDay(now)
	if _, err := tx.ExecContext(ctx, `UPDATE jack_fleets SET daily_reset_at=CASE WHEN $2 THEN $5 ELSE daily_reset_at END,weekly_reset_at=CASE WHEN $3 THEN $5 ELSE weekly_reset_at END,monthly_reset_at=CASE WHEN $4 THEN $5 ELSE monthly_reset_at END WHERE id=$1`, id, in.Daily, in.Weekly, in.Monthly, now); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `UPDATE user_subscriptions s SET daily_usage_usd=CASE WHEN $2 THEN 0 ELSE daily_usage_usd END,weekly_usage_usd=CASE WHEN $3 THEN 0 ELSE weekly_usage_usd END,monthly_usage_usd=CASE WHEN $4 THEN 0 ELSE monthly_usage_usd END,daily_window_start=CASE WHEN $2 THEN $5 ELSE daily_window_start END,weekly_window_start=CASE WHEN $3 THEN $6 ELSE weekly_window_start END,monthly_window_start=CASE WHEN $4 THEN $6 ELSE monthly_window_start END,updated_at=now() FROM jack_fleet_members m WHERE m.fleet_id=$1 AND m.active AND m.subscription_id=s.id RETURNING s.user_id,s.group_id`, id, in.Daily, in.Weekly, in.Monthly, day, now)
	if err != nil {
		return err
	}
	return collectPairs(rows, out)
}
