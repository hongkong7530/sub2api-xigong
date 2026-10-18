-- Preserve legacy request-type and image-mode filters after detail retention.
ALTER TABLE usage_retained_hourly ADD COLUMN openai_ws_mode BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE usage_retained_hourly ADD COLUMN image_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_retained_hourly DROP CONSTRAINT usage_retained_hourly_dimensions;
ALTER TABLE usage_retained_hourly ADD CONSTRAINT usage_retained_hourly_dimensions
UNIQUE NULLS NOT DISTINCT (bucket_start,user_id,api_key_id,account_id,group_id,model,requested_model,upstream_model,inbound_endpoint,upstream_endpoint,request_type,stream,billing_type,billing_mode,upstream_model_mismatch,native_compaction_v2,paid,openai_ws_mode,image_count);

CREATE OR REPLACE VIEW usage_statistics AS
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
       CASE WHEN duration_ms IS NULL THEN 0 ELSE 1 END::BIGINT AS duration_count,
       openai_ws_mode,image_count
FROM usage_logs
UNION ALL
SELECT user_id, api_key_id, account_id, group_id, model, requested_model, upstream_model,
       inbound_endpoint, upstream_endpoint, request_type, stream, billing_type, billing_mode,
       upstream_model_mismatch, native_compaction_v2, bucket_start,
       request_count, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
       image_input_tokens, image_output_tokens, total_cost, actual_cost, account_cost, 1::NUMERIC,
       duration_sum_ms/NULLIF(duration_count,0), duration_sum_ms, duration_count,
       openai_ws_mode,image_count
FROM usage_retained_hourly;

CREATE OR REPLACE FUNCTION archive_deleted_usage_statistics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint WHERE conrelid='billing_usage_entries'::regclass
        AND confrelid='usage_logs'::regclass AND contype='f' AND confdeltype<>'n'
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conrelid='billing_usage_entries'::regclass
        AND confrelid='usage_logs'::regclass AND contype='f' AND confdeltype='n'
        AND conkey=ARRAY[(SELECT attnum FROM pg_attribute
            WHERE attrelid='billing_usage_entries'::regclass AND attname='usage_log_id')]::smallint[]
        AND confkey=ARRAY[(SELECT attnum FROM pg_attribute
            WHERE attrelid='usage_logs'::regclass AND attname='id')]::smallint[]
    ) OR EXISTS (
        SELECT 1 FROM pg_attribute WHERE attrelid='billing_usage_entries'::regclass
        AND attname='usage_log_id' AND attnotnull
    ) THEN
        RAISE EXCEPTION 'unsafe billing history foreign key for detail retention';
    END IF;
    INSERT INTO usage_retained_hourly
    SELECT date_trunc('hour',created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
           user_id,api_key_id,account_id,group_id,model,requested_model,upstream_model,
           inbound_endpoint,upstream_endpoint,request_type,stream,billing_type,billing_mode,
           upstream_model_mismatch,native_compaction_v2,actual_cost>0,
           COUNT(*),SUM(input_tokens),SUM(output_tokens),SUM(cache_creation_tokens),SUM(cache_read_tokens),
           SUM(image_input_tokens),SUM(image_output_tokens),SUM(total_cost),SUM(actual_cost),
           SUM(COALESCE(account_stats_cost,total_cost)*COALESCE(account_rate_multiplier,1)),
           SUM(COALESCE(duration_ms,0)),COUNT(duration_ms),openai_ws_mode,image_count
    FROM deleted_usage
    GROUP BY 1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,30,31
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
        duration_count=usage_retained_hourly.duration_count+EXCLUDED.duration_count;
    RETURN NULL;
END;
$$;
