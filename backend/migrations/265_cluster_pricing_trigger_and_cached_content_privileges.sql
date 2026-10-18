-- Repair two privilege defects introduced while the v0.2.16 rollout ran its DDL with
-- elevated migrator rights.
--
-- 1. cluster_pricing_revision is a control table: node roles deliberately hold SELECT
--    only, and the AFTER triggers on every pricing-bearing table bump the counter on
--    ordinary application writes (groups, channels, ...). Running that trigger with the
--    caller's rights therefore failed every such write with
--    "permission denied for table cluster_pricing_revision". The counter refresh belongs
--    to the schema owner, so the trigger function executes as its owner; search_path is
--    pinned to the catalog plus public to keep the definer context safe.
CREATE OR REPLACE FUNCTION bump_cluster_pricing_revision() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
    UPDATE cluster_pricing_revision SET revision=revision+1 WHERE id=1;
    RETURN NULL;
END;
$$;
ALTER FUNCTION bump_cluster_pricing_revision() OWNER TO sub2api_owner;

-- 2. gemini_cached_contents and its sequence (migration 262) were created by the
--    standalone migrator running with elevated rights, so they stayed owned by that role
--    with no grants for the node roles. Align them with every other application table:
--    owned by the schema owner and writable by the node roles.
ALTER TABLE gemini_cached_contents OWNER TO sub2api_owner;
ALTER SEQUENCE gemini_cached_contents_id_seq OWNER TO sub2api_owner;

DO $$
DECLARE role_name text;
BEGIN
    FOR role_name IN
        SELECT rolname FROM pg_roles
        WHERE rolname LIKE 's2c\_%' OR rolname IN ('sub2api_xigong2', 'sub2api_xigong')
    LOOP
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.gemini_cached_contents TO %I', role_name);
        EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE public.gemini_cached_contents_id_seq TO %I', role_name);
    END LOOP;
END $$;
