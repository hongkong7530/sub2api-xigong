-- Keep first_token_ms unchanged: health/TTFT is not the stream's output window.
-- Additive and nullable for mixed-version rolling deployment; do not backfill history.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS output_duration_ms integer;
COMMENT ON COLUMN usage_logs.output_duration_ms IS 'First formal upstream stream event to successful output completion, milliseconds; includes reasoning phase';

-- Probe rates written before this migration use the old TTFT-based denominator.
-- Version them without rewriting historical measurements or availability.
ALTER TABLE channel_monitor_histories ADD COLUMN IF NOT EXISTS output_rate_version smallint;
COMMENT ON COLUMN channel_monitor_histories.output_rate_version IS '1 = formal stream window including reasoning; NULL = legacy or unknown';
