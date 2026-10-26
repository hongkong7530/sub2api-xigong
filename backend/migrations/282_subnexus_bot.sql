-- Dedicated, revocable read-only installations. Token plaintext is never persisted;
-- the HMAC material uses the application's authenticated secret encryption service.
CREATE TABLE bot_installations (
 id varchar(32) PRIMARY KEY CHECK(id ~ '^[0-9a-f]{32}$'),
 policy jsonb NOT NULL CHECK(jsonb_typeof(policy)='object'),
 token_hash varchar(64) NOT NULL CHECK(token_hash ~ '^[0-9a-f]{64}$'),
 signing_ciphertext text NOT NULL,
 scope text NOT NULL DEFAULT 'bot:read' CHECK(scope='bot:read'),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
 revoked_at timestamptz
);
REVOKE ALL ON bot_installations FROM PUBLIC;
-- Only ingress nodes issue/revoke or validate bot installations, not gateway-only workers.
DO $$ DECLARE item record; BEGIN
 IF to_regclass('public.cluster_node_credentials') IS NOT NULL THEN
  FOR item IN SELECT DISTINCT c.database_role,n.roles FROM public.cluster_node_credentials c
   JOIN public.cluster_nodes n USING(cluster_id,node_id)
   JOIN pg_roles r ON r.rolname=c.database_role
   WHERE c.status='active' AND r.rolcanlogin LOOP
   IF item.roles ? 'ingress' THEN
    EXECUTE format('GRANT SELECT,INSERT ON public.bot_installations TO %I',item.database_role);
    EXECUTE format('GRANT UPDATE (revoked_at) ON public.bot_installations TO %I',item.database_role);
   END IF;
  END LOOP;
 END IF;
END $$;
