CREATE TABLE jack_fleets (
 id bigserial PRIMARY KEY,
 name varchar(100) NOT NULL,
 group_id bigint NOT NULL REFERENCES groups(id),
 expires_at timestamptz NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 archived_at timestamptz,
 daily_reset_at timestamptz,
 weekly_reset_at timestamptz,
 monthly_reset_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX jack_fleets_live_group ON jack_fleets(group_id) WHERE archived_at IS NULL;
CREATE TABLE jack_fleet_members (
 fleet_id bigint NOT NULL REFERENCES jack_fleets(id),
 user_id bigint NOT NULL REFERENCES users(id),
 subscription_id bigint NOT NULL UNIQUE REFERENCES user_subscriptions(id),
 independent_expiry boolean NOT NULL DEFAULT false,
 active boolean NOT NULL DEFAULT true,
 joined_at timestamptz NOT NULL DEFAULT now(),
 left_at timestamptz,
 PRIMARY KEY(fleet_id,user_id)
);
CREATE TABLE jack_fleet_operations (
 actor_id bigint NOT NULL,
 operation_key varchar(128) NOT NULL,
 request_hash text NOT NULL,
 result jsonb NOT NULL,
 cache_pending boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,operation_key)
);
CREATE INDEX jack_fleet_operations_pending ON jack_fleet_operations(created_at) WHERE cache_pending;

-- Protect ownership even when an upstream purchase/redeem/admin path changes.
-- Usage charging and normal expiry/window maintenance remain upstream-owned.
CREATE FUNCTION jack_guard_subscription_terms() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF current_setting('jack.fleet_mutation',true) = 'on' THEN
  IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
 END IF;
 IF EXISTS (SELECT 1 FROM jack_fleet_members WHERE subscription_id=OLD.id) THEN
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'FLEET_MANAGED_SUBSCRIPTION: use fleet management'; END IF;
  IF NEW.user_id IS DISTINCT FROM OLD.user_id OR NEW.group_id IS DISTINCT FROM OLD.group_id
    OR NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.starts_at IS DISTINCT FROM OLD.starts_at
    OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at
    OR (NEW.status IS DISTINCT FROM OLD.status AND NOT (OLD.status='active' AND NEW.status='expired' AND OLD.expires_at<=now()))
  THEN RAISE EXCEPTION 'FLEET_MANAGED_SUBSCRIPTION: use fleet management'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER jack_subscription_terms BEFORE UPDATE OR DELETE ON user_subscriptions
 FOR EACH ROW EXECUTE FUNCTION jack_guard_subscription_terms();

CREATE FUNCTION jack_guard_fleet_group() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM jack_fleets WHERE group_id=OLD.id AND archived_at IS NULL) THEN
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'FLEET_MANAGED_GROUP: archive the fleet first'; END IF;
  IF NEW.deleted_at IS DISTINCT FROM OLD.deleted_at OR NEW.subscription_type IS DISTINCT FROM OLD.subscription_type
    OR NEW.platform IS DISTINCT FROM OLD.platform
  THEN RAISE EXCEPTION 'FLEET_MANAGED_GROUP: archive the fleet first'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER jack_fleet_group BEFORE UPDATE OR DELETE ON groups
 FOR EACH ROW EXECUTE FUNCTION jack_guard_fleet_group();
