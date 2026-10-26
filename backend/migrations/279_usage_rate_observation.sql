-- Arrival observations are not provider-internal generation timings.
-- Nullable and additive for mixed-version deployment. Never infer/backfill old rows.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS rate_observation jsonb;
COMMENT ON COLUMN usage_logs.rate_observation IS 'Versioned, non-sensitive app/turn duration and output arrival distribution; NULL means unknown';
-- No history rewrite: old probe rates remain version 1 and do not qualify under new arrival-quality rules.
COMMENT ON COLUMN channel_monitor_histories.output_rate_version IS '1 = legacy formal window; 2 = complete, distributed arrival sample eligible for observed-rate statistics; NULL = unknown';
