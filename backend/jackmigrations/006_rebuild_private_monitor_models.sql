-- These are disposable derived facts. Earlier versions backfilled their model
-- dimension from the upstream model; regenerate them under the public request
-- provenance policy before serving the user channel monitor. Source usage/error
-- logs, billing, subscriptions and monitor configuration are untouched.
TRUNCATE TABLE
    channel_monitor_v2_metrics_1m,
    channel_monitor_v2_user_metrics_1m,
    channel_monitor_v2_error_metrics_1m,
    channel_monitor_v2_latency_histograms_1m,
    channel_monitor_v2_metrics_rollup,
    channel_monitor_v2_user_metrics_rollup,
    channel_monitor_v2_error_metrics_rollup,
    channel_monitor_v2_latency_histograms_rollup;

UPDATE channel_monitor_v2_watermarks SET
    usage_coverage_start = NULL,
    error_coverage_start = NULL,
    data_through = NULL,
    last_successful_at = NULL,
    backfill_cursor = NULL,
    updated_at = NOW()
WHERE id = 1;
