-- Optional request detail must not own cumulative usage or monetary history.
-- This migration creates an empty archive; it does not delete any live records.
CREATE TABLE usage_retained_hourly (
    bucket_start TIMESTAMPTZ NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    group_id BIGINT,
    model TEXT NOT NULL,
    requested_model TEXT,
    upstream_model TEXT,
    inbound_endpoint TEXT,
    upstream_endpoint TEXT,
    request_type SMALLINT,
    stream BOOLEAN NOT NULL,
    billing_type SMALLINT NOT NULL,
    billing_mode TEXT,
    upstream_model_mismatch BOOLEAN,
    native_compaction_v2 BOOLEAN,
    paid BOOLEAN NOT NULL,
    request_count BIGINT NOT NULL,
    input_tokens BIGINT NOT NULL,
    output_tokens BIGINT NOT NULL,
    cache_creation_tokens BIGINT NOT NULL,
    cache_read_tokens BIGINT NOT NULL,
    image_input_tokens BIGINT NOT NULL,
    image_output_tokens BIGINT NOT NULL,
    total_cost NUMERIC NOT NULL,
    actual_cost NUMERIC NOT NULL,
    account_cost NUMERIC NOT NULL,
    duration_sum_ms NUMERIC NOT NULL,
    duration_count BIGINT NOT NULL,
    CONSTRAINT usage_retained_hourly_dimensions UNIQUE NULLS NOT DISTINCT (
        bucket_start, user_id, api_key_id, account_id, group_id,
        model, requested_model, upstream_model, inbound_endpoint, upstream_endpoint,
        request_type, stream, billing_type, billing_mode,
        upstream_model_mismatch, native_compaction_v2, paid
    )
);
CREATE INDEX usage_retained_hourly_user_time ON usage_retained_hourly(user_id,bucket_start);
CREATE INDEX usage_retained_hourly_key_time ON usage_retained_hourly(api_key_id,bucket_start);
CREATE INDEX usage_retained_hourly_account_time ON usage_retained_hourly(account_id,bucket_start);

-- Statistics only: no request ID, exact request timestamp, IP, agent, session or body.
-- Detail/list APIs continue to query usage_logs and cannot reconstruct deleted requests.
CREATE VIEW usage_statistics AS
SELECT user_id, api_key_id, account_id, group_id, model, requested_model, upstream_model,
       inbound_endpoint, upstream_endpoint, request_type, stream, billing_type, billing_mode,
       upstream_model_mismatch, native_compaction_v2, created_at,
       1::BIGINT AS request_count,
       input_tokens::BIGINT, output_tokens::BIGINT,
       cache_creation_tokens::BIGINT, cache_read_tokens::BIGINT,
       image_input_tokens::BIGINT, image_output_tokens::BIGINT,
       total_cost, actual_cost,
       COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1) AS account_stats_cost,
       1::NUMERIC AS account_rate_multiplier,
       duration_ms::NUMERIC AS duration_ms,
       COALESCE(duration_ms,0)::NUMERIC AS duration_sum_ms,
       CASE WHEN duration_ms IS NULL THEN 0 ELSE 1 END::BIGINT AS duration_count
FROM usage_logs
UNION ALL
SELECT user_id, api_key_id, account_id, group_id, model, requested_model, upstream_model,
       inbound_endpoint, upstream_endpoint, request_type, stream, billing_type, billing_mode,
       upstream_model_mismatch, native_compaction_v2, bucket_start,
       request_count, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
       image_input_tokens, image_output_tokens, total_cost, actual_cost, account_cost, 1::NUMERIC,
       duration_sum_ms/NULLIF(duration_count,0), duration_sum_ms, duration_count
FROM usage_retained_hourly;

-- Only explicitly invoked maintenance removes detail. DELETE RETURNING and the
-- additive rollup share one transaction, so retries cannot omit or double-count rows.
CREATE FUNCTION retain_and_delete_usage_batch(cutoff TIMESTAMPTZ, batch_size INTEGER)
RETURNS BIGINT LANGUAGE plpgsql AS $$
DECLARE removed BIGINT;
BEGIN
    IF batch_size < 1 OR batch_size > 100000 THEN
        RAISE EXCEPTION 'invalid usage retention batch size';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid='billing_usage_entries'::regclass
          AND confrelid='usage_logs'::regclass AND contype='f' AND confdeltype='n'
    ) THEN
        RAISE EXCEPTION 'billing history must use ON DELETE SET NULL before detail retention';
    END IF;
    WITH picked AS (
        SELECT id FROM usage_logs WHERE created_at < cutoff
        ORDER BY id LIMIT batch_size FOR UPDATE SKIP LOCKED
    ), removed_rows AS (
        DELETE FROM usage_logs u USING picked p WHERE u.id=p.id RETURNING u.*
    ), rollup AS (
        INSERT INTO usage_retained_hourly
        SELECT date_trunc('hour',created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
               user_id,api_key_id,account_id,group_id,model,requested_model,upstream_model,
               inbound_endpoint,upstream_endpoint,request_type,stream,billing_type,billing_mode,
               upstream_model_mismatch,native_compaction_v2,actual_cost>0,
               COUNT(*),SUM(input_tokens),SUM(output_tokens),SUM(cache_creation_tokens),SUM(cache_read_tokens),
               SUM(image_input_tokens),SUM(image_output_tokens),SUM(total_cost),SUM(actual_cost),
               SUM(COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)),
               SUM(COALESCE(duration_ms,0)),COUNT(duration_ms)
        FROM removed_rows
        GROUP BY 1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17
        ON CONFLICT ON CONSTRAINT usage_retained_hourly_dimensions DO UPDATE SET
            request_count=usage_retained_hourly.request_count+EXCLUDED.request_count,
            input_tokens=usage_retained_hourly.input_tokens+EXCLUDED.input_tokens,
            output_tokens=usage_retained_hourly.output_tokens+EXCLUDED.output_tokens,
            cache_creation_tokens=usage_retained_hourly.cache_creation_tokens+EXCLUDED.cache_creation_tokens,
            cache_read_tokens=usage_retained_hourly.cache_read_tokens+EXCLUDED.cache_read_tokens,
            image_input_tokens=usage_retained_hourly.image_input_tokens+EXCLUDED.image_input_tokens,
            image_output_tokens=usage_retained_hourly.image_output_tokens+EXCLUDED.image_output_tokens,
            total_cost=usage_retained_hourly.total_cost+EXCLUDED.total_cost,
            actual_cost=usage_retained_hourly.actual_cost+EXCLUDED.actual_cost,
            account_cost=usage_retained_hourly.account_cost+EXCLUDED.account_cost,
            duration_sum_ms=usage_retained_hourly.duration_sum_ms+EXCLUDED.duration_sum_ms,
            duration_count=usage_retained_hourly.duration_count+EXCLUDED.duration_count
        RETURNING 1
    ) SELECT COUNT(*) INTO removed FROM removed_rows;
    RETURN removed;
END;
$$;
REVOKE ALL ON FUNCTION retain_and_delete_usage_batch(TIMESTAMPTZ,INTEGER) FROM PUBLIC;
