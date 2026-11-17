-- Persist the per-redemption choice through ambiguous outcomes and restarts.
ALTER TABLE jack_fleet_credit_operations ADD COLUMN independent_reset boolean NOT NULL DEFAULT false;
-- A successful independent redemption must not become a passive reset later.
ALTER TABLE jack_fleet_reset_sync ADD COLUMN independent_reset_pending boolean NOT NULL DEFAULT false;
