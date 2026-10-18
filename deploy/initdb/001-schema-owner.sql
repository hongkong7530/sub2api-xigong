-- Historical migrations transfer selected objects to this stable non-login owner.
-- Run before application migrations; never rewrite already-published migration SQL.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sub2api_owner') THEN
        CREATE ROLE sub2api_owner NOLOGIN;
    END IF;
    IF current_user <> 'sub2api_owner' THEN
        EXECUTE format('GRANT sub2api_owner TO %I', current_user);
    END IF;
END
$$;

GRANT USAGE, CREATE ON SCHEMA public TO sub2api_owner;

-- The pricing trigger executes as sub2api_owner after migration 265. Grant it
-- access when its table is created later by the initializing database role.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO sub2api_owner;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO sub2api_owner;
