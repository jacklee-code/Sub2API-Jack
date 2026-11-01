-- Older binaries coalesced effective model/effort into requested metadata. Keep
-- those values for administrators, but never certify them as client input.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS jack_request_metadata_verified boolean NOT NULL DEFAULT false;
-- The new writer explicitly certifies inserts. Keep DEFAULT false so an older
-- binary used during rollback cannot accidentally certify its coalesced values.
