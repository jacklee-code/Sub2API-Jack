CREATE TABLE jack_fleet_reset_sync (
 fleet_id bigint PRIMARY KEY REFERENCES jack_fleets(id),
 account_id bigint NOT NULL REFERENCES accounts(id),
 enabled boolean NOT NULL DEFAULT false,
 generation bigint NOT NULL DEFAULT 1,
 configured_at timestamptz NOT NULL DEFAULT now(),
 baseline jsonb,
 candidate jsonb,
 next_reset_at timestamptz,
 observed_at timestamptz,
 status text NOT NULL DEFAULT 'watching',
 error_code text NOT NULL DEFAULT '',
 rebaseline boolean NOT NULL DEFAULT false
);
CREATE INDEX jack_fleet_reset_sync_account ON jack_fleet_reset_sync(account_id) WHERE enabled;
CREATE TABLE jack_fleet_reset_jobs (
 account_id bigint PRIMARY KEY REFERENCES accounts(id),
 due_at timestamptz NOT NULL,
 last_attempt_at timestamptz,
 lease_until timestamptz
);
CREATE TABLE jack_fleet_reset_events (
 id bigserial PRIMARY KEY,
 fleet_id bigint NOT NULL REFERENCES jack_fleets(id),
 account_id bigint NOT NULL,
 source text NOT NULL,
 operation_key text NOT NULL,
 before_snapshot jsonb,
 after_snapshot jsonb,
 members integer NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(fleet_id,operation_key)
);
CREATE TABLE jack_fleet_credit_operations (
 id bigserial PRIMARY KEY,
 actor_id bigint NOT NULL,
 account_id bigint NOT NULL REFERENCES accounts(id),
 operation_key text NOT NULL,
 redeem_id text NOT NULL,
 account_identity text NOT NULL DEFAULT '',
 targets jsonb NOT NULL,
 state text NOT NULL DEFAULT 'pending',
 result jsonb,
 error_code text NOT NULL DEFAULT '',
 lease_until timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 UNIQUE(actor_id,account_id,operation_key)
);
CREATE INDEX jack_fleet_credit_pending ON jack_fleet_credit_operations(next_attempt_at) WHERE state='pending';
