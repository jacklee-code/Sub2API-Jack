-- Recompute derived monitor dimensions using the OpenAI-only privacy scope.
-- Retain all cached facts and source records while the bounded worker backfills.
UPDATE channel_monitor_v2_watermarks SET
    backfill_cursor = NULL,
    usage_coverage_start = NULL,
    error_coverage_start = NULL,
    data_through = NULL,
    last_successful_at = NULL,
    updated_at = NOW()
WHERE id = 1;
