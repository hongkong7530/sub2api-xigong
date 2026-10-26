-- Monitor availability reads the full requested window and preserves failed-stream
-- deduplication. Cover its narrow projection instead of reading wide usage rows.
-- The predicate matches the existing business-fact filter; no facts are removed.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_monitor_health_cover
    ON usage_logs (group_id, created_at)
    INCLUDE (api_key_id, request_id, requested_model, model, duration_ms)
    WHERE group_id IS NOT NULL
      AND COALESCE(request_type, 0) <> 4
      AND (input_tokens + output_tokens + cache_read_tokens + cache_creation_tokens > 0
           OR actual_cost > 0 OR COALESCE(image_count, 0) > 0);
