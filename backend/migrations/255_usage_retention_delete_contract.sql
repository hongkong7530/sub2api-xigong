-- Every ordinary detail deletion must preserve cumulative statistics, including
-- legacy administrative and scheduled DELETE paths. No deletion runs on migration.
CREATE FUNCTION archive_deleted_usage_statistics() RETURNS trigger LANGUAGE plpgsql AS $$
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
           SUM(COALESCE(duration_ms,0)),COUNT(duration_ms)
    FROM deleted_usage
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
        duration_count=usage_retained_hourly.duration_count+EXCLUDED.duration_count;
    RETURN NULL;
END;
$$;
CREATE TRIGGER usage_logs_preserve_statistics AFTER DELETE ON usage_logs
REFERENCING OLD TABLE AS deleted_usage FOR EACH STATEMENT
EXECUTE FUNCTION archive_deleted_usage_statistics();

-- TRUNCATE does not fire DELETE triggers. Only an already empty table may be
-- truncated for physical reclamation; the exclusive lock also fences inserts.
CREATE FUNCTION prevent_unarchived_usage_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM usage_logs LIMIT 1) THEN
        RAISE EXCEPTION 'archive request detail with DELETE before truncating';
    END IF;
    RETURN NULL;
END;
$$;
CREATE TRIGGER usage_logs_prevent_unarchived_truncate BEFORE TRUNCATE ON usage_logs
FOR EACH STATEMENT EXECUTE FUNCTION prevent_unarchived_usage_truncate();

-- The statement trigger is the sole archive owner: the maintenance wrapper must
-- not archive the same DELETE RETURNING rows again. Row locations are partition-safe.
CREATE OR REPLACE FUNCTION retain_and_delete_usage_batch(cutoff TIMESTAMPTZ, batch_size INTEGER)
RETURNS BIGINT LANGUAGE plpgsql AS $$
DECLARE removed BIGINT;
BEGIN
    IF cutoff IS NULL OR batch_size IS NULL OR batch_size < 1 OR batch_size > 100000 THEN
        RAISE EXCEPTION 'invalid usage retention batch input';
    END IF;
    WITH picked AS (
        SELECT tableoid,ctid FROM usage_logs WHERE created_at < cutoff
        ORDER BY created_at,id LIMIT batch_size FOR UPDATE SKIP LOCKED
    ), removed_rows AS (
        DELETE FROM usage_logs u USING picked p
        WHERE (u.tableoid,u.ctid)=(p.tableoid,p.ctid) RETURNING 1
    ) SELECT COUNT(*) INTO removed FROM removed_rows;
    RETURN removed;
END;
$$;

-- Newly enrolled nodes get the same read access through their provisioning path;
-- existing explicit cluster identities require these statistics-only permissions.
DO $$
DECLARE identity RECORD;
BEGIN
    FOR identity IN SELECT database_role FROM cluster_node_credentials WHERE status='active'
    LOOP
        IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=identity.database_role) THEN
            EXECUTE format('GRANT SELECT ON usage_statistics,usage_retained_hourly TO %I',identity.database_role);
        END IF;
    END LOOP;
END;
$$;
