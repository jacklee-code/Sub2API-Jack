package jackfleet

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func (s *Store) join(ctx context.Context, tx *sql.Tx, actor int64, f Fleet, m MemberInput, out *Result, now time.Time) error {
	if m.UserID <= 0 {
		return bad("User ID must be positive")
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, m.UserID).Scan(&status); err != nil {
		return bad("User not found")
	}
	if status != "active" {
		return bad("Only active users may join")
	}
	var previousID int64
	var active, independent bool
	var leftAt sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT subscription_id,active,independent_expiry,left_at FROM jack_fleet_members WHERE fleet_id=$1 AND user_id=$2`, f.ID, m.UserID).Scan(&previousID, &active, &independent, &leftAt)
	if err == nil && active {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	rejoin := err == nil
	var subID int64
	var existingExpiry time.Time
	err = tx.QueryRowContext(ctx, `SELECT id,expires_at FROM user_subscriptions WHERE user_id=$1 AND group_id=$2 AND deleted_at IS NULL FOR UPDATE`, m.UserID, f.GroupID).Scan(&subID, &existingExpiry)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	expiry := f.ExpiresAt
	if exists {
		var owner int64
		ownerErr := tx.QueryRowContext(ctx, `SELECT fleet_id FROM jack_fleet_members WHERE subscription_id=$1`, subID).Scan(&owner)
		if ownerErr != nil && !errors.Is(ownerErr, sql.ErrNoRows) {
			return ownerErr
		}
		if ownerErr == nil && owner != f.ID {
			return conflict("Subscription belongs to another fleet; manage that fleet first")
		}
		if !rejoin && !m.Adopt {
			return conflict("An existing subscription needs explicit adoption; preview it first")
		}
		if rejoin {
			m.Independent = independent
		}
		if m.Independent {
			expiry = existingExpiry
		}
	} else {
		if rejoin {
			return conflict("Managed subscription is missing; restore it before rejoining")
		}
		if m.Independent {
			if m.ExpiresAt == nil || !validExpiry(*m.ExpiresAt) {
				return bad("An independent expiry is required for a new subscription")
			}
			expiry = *m.ExpiresAt
		}
		starts := now
		if !expiry.After(starts) {
			starts = expiry.Add(-time.Second)
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,assigned_by,assigned_at,created_at,updated_at) VALUES($1,$2,$3,$4,CASE WHEN $4>now() THEN 'active' ELSE 'expired' END,$5,now(),now(),now()) RETURNING id`, m.UserID, f.GroupID, starts, expiry, actor).Scan(&subID)
		if err != nil {
			return err
		}
	}
	if exists {
		if _, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET expires_at=$2,status=CASE WHEN $2>now() THEN 'active' ELSE 'expired' END,updated_at=now() WHERE id=$1`, subID, expiry); err != nil {
			return err
		}
	}
	if rejoin {
		// A reset performed while this member was away also establishes the new
		// fleet window on return. Otherwise their previous in-window usage survives.
		var daily, weekly, monthly sql.NullTime
		if err = tx.QueryRowContext(ctx, `SELECT daily_reset_at,weekly_reset_at,monthly_reset_at FROM jack_fleets WHERE id=$1`, f.ID).Scan(&daily, &weekly, &monthly); err != nil {
			return err
		}
		resets := []struct {
			stamp         sql.NullTime
			usage, window string
			day           bool
		}{{daily, "daily_usage_usd", "daily_window_start", true}, {weekly, "weekly_usage_usd", "weekly_window_start", false}, {monthly, "monthly_usage_usd", "monthly_window_start", false}}
		for _, r := range resets {
			if r.stamp.Valid && leftAt.Valid && r.stamp.Time.After(leftAt.Time) {
				anchor := r.stamp.Time
				if r.day {
					anchor = timezone.StartOfDay(anchor)
				}
				if _, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET `+r.usage+`=0,`+r.window+`=$2 WHERE id=$1`, subID, anchor); err != nil {
					return err
				}
			}
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO jack_fleet_members(fleet_id,user_id,subscription_id,independent_expiry) VALUES($1,$2,$3,$4) ON CONFLICT(fleet_id,user_id) DO UPDATE SET active=true,left_at=NULL,joined_at=now()`, f.ID, m.UserID, subID, m.Independent)
	out.Changed = append(out.Changed, Pair{m.UserID, f.GroupID})
	return err
}
