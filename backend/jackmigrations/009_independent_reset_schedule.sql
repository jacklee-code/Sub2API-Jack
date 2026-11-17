-- Keep the member-facing deadline separate from the account's reset cycle.
ALTER TABLE jack_fleet_reset_sync
 ADD COLUMN independent_reset_due_at timestamptz,
 ADD COLUMN independent_reset_window jsonb;
CREATE INDEX jack_fleet_independent_reset_due
 ON jack_fleet_reset_sync(independent_reset_due_at)
 WHERE enabled AND independent_reset_due_at IS NOT NULL;
