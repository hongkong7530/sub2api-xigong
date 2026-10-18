-- Workers may observe a Redis restart, but cannot acknowledge or disable the recovery hold.
CREATE OR REPLACE FUNCTION cluster_observe_redis_generation(target_cluster TEXT, observed_generation TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE current_generation TEXT; held BOOLEAN;
BEGIN
    IF observed_generation IS NULL OR length(observed_generation) <> 40 THEN
        RAISE EXCEPTION 'invalid Redis generation' USING ERRCODE='22023';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=session_user AND rolsuper)
       AND NOT EXISTS(SELECT 1 FROM public.cluster_node_credentials c JOIN public.cluster_nodes n USING(cluster_id,node_id)
                      WHERE c.cluster_id=target_cluster AND c.database_role=session_user AND c.status='active' AND n.state<>'retired') THEN
        RAISE EXCEPTION 'node state identity is not active' USING ERRCODE='42501';
    END IF;
    SELECT redis_generation,redis_recovery_required INTO STRICT current_generation,held
      FROM public.cluster_control WHERE cluster_id=target_cluster;
    IF current_generation=observed_generation THEN RETURN NOT held; END IF;
    UPDATE public.cluster_control SET redis_generation=observed_generation,redis_recovery_required=TRUE,updated_at=NOW()
      WHERE cluster_id=target_cluster AND redis_generation=current_generation;
    RETURN FALSE;
END;
$$;
REVOKE ALL ON FUNCTION cluster_observe_redis_generation(TEXT,TEXT) FROM PUBLIC;

-- Group pricing changes also invalidate request admission snapshots.
CREATE TRIGGER cluster_group_pricing_revision_change AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON groups
    FOR EACH STATEMENT EXECUTE FUNCTION bump_cluster_pricing_revision();
ALTER TABLE cluster_nodes ADD COLUMN IF NOT EXISTS applied_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE cluster_nodes ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ;
ALTER TABLE cluster_nodes ADD COLUMN IF NOT EXISTS apply_error TEXT;
