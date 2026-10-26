-- Profit is a reporting fact captured atomically with committed usage billing.
-- No historical backfill and no change to balances, quotas or billing fingerprints.
CREATE TABLE site_profit_meta (
 id smallint PRIMARY KEY CHECK(id=1), started_at timestamptz NOT NULL DEFAULT statement_timestamp(),
 timezone text NOT NULL DEFAULT current_setting('TimeZone'), backlog boolean NOT NULL DEFAULT false, aggregated_at timestamptz
);
INSERT INTO site_profit_meta(id) VALUES(1);
CREATE TABLE site_profit_cost_versions (
 id bigserial PRIMARY KEY, group_id bigint NOT NULL, version bigint NOT NULL CHECK(version>0),
 cost_multiplier numeric(14,8) CHECK(cost_multiplier>=0 AND cost_multiplier<=10000),
 effective_at timestamptz NOT NULL DEFAULT statement_timestamp(), actor_id bigint NOT NULL,
 UNIQUE(group_id,version)
);
CREATE INDEX site_profit_cost_lookup ON site_profit_cost_versions(group_id,effective_at DESC,version DESC);
CREATE TABLE site_profit_entries (
 id bigserial PRIMARY KEY, billing_entry_id bigint NOT NULL UNIQUE,
 user_id bigint NOT NULL, group_id bigint NOT NULL, group_name text NOT NULL, platform text NOT NULL,
 billing_type smallint NOT NULL, settled_at timestamptz NOT NULL, day date NOT NULL,
 base_usd numeric(20,8) NOT NULL CHECK(base_usd>=0), revenue_usd numeric(20,8) NOT NULL CHECK(revenue_usd>=0),
 cost_version bigint, cost_multiplier numeric(14,8), cost_usd numeric(20,8), profit_usd numeric(20,8),
 aggregated boolean NOT NULL DEFAULT false,
 CHECK((cost_multiplier IS NULL AND cost_usd IS NULL AND profit_usd IS NULL) OR
       (cost_multiplier IS NOT NULL AND cost_usd IS NOT NULL AND profit_usd IS NOT NULL AND profit_usd=revenue_usd-cost_usd))
);
CREATE INDEX site_profit_pending ON site_profit_entries(id) WHERE NOT aggregated;
CREATE INDEX site_profit_entries_time ON site_profit_entries(settled_at DESC,id DESC);
CREATE INDEX site_profit_entries_group_time ON site_profit_entries(group_id,settled_at DESC,id DESC);
-- No cascading FK: optional usage/keys/users/groups may be removed without deleting financial facts.
CREATE TABLE site_profit_group_daily (
 group_id bigint NOT NULL, day date NOT NULL, billing_type smallint NOT NULL,
 requests bigint NOT NULL DEFAULT 0, known_requests bigint NOT NULL DEFAULT 0,
 revenue_usd numeric(30,8) NOT NULL DEFAULT 0, known_revenue_usd numeric(30,8) NOT NULL DEFAULT 0,
 cost_usd numeric(30,8) NOT NULL DEFAULT 0, profit_usd numeric(30,8) NOT NULL DEFAULT 0,
 PRIMARY KEY(group_id,day,billing_type)
);
CREATE INDEX site_profit_daily_day ON site_profit_group_daily(day,group_id);
CREATE TABLE site_profit_group_totals (
 group_id bigint NOT NULL, billing_type smallint NOT NULL, group_name text NOT NULL, platform text NOT NULL,
 requests bigint NOT NULL DEFAULT 0, known_requests bigint NOT NULL DEFAULT 0,
 revenue_usd numeric(30,8) NOT NULL DEFAULT 0, known_revenue_usd numeric(30,8) NOT NULL DEFAULT 0,
 cost_usd numeric(30,8) NOT NULL DEFAULT 0, profit_usd numeric(30,8) NOT NULL DEFAULT 0,
 PRIMARY KEY(group_id,billing_type)
);
CREATE TABLE site_profit_user_totals (
 user_id bigint NOT NULL, billing_type smallint NOT NULL,
 requests bigint NOT NULL DEFAULT 0, known_requests bigint NOT NULL DEFAULT 0,
 revenue_usd numeric(30,8) NOT NULL DEFAULT 0, known_revenue_usd numeric(30,8) NOT NULL DEFAULT 0,
 cost_usd numeric(30,8) NOT NULL DEFAULT 0, profit_usd numeric(30,8) NOT NULL DEFAULT 0,
 PRIMARY KEY(user_id,billing_type)
);
CREATE FUNCTION capture_site_profit() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE u record; v record; at_time timestamptz:=statement_timestamp(); zone text; amount numeric(20,8);
BEGIN
 IF NOT NEW.applied OR NEW.usage_log_id IS NULL THEN RETURN NEW; END IF;
 SELECT l.group_id,l.total_cost,l.actual_cost,COALESCE(g.name,'Ungrouped') AS group_name,
        COALESCE(g.platform,'unknown') AS platform INTO u
 FROM public.usage_logs l LEFT JOIN public.groups g ON g.id=l.group_id WHERE l.id=NEW.usage_log_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'profit capture requires its billing usage fact'; END IF;
 -- Simple mode may persist displayed key-rate-limit cost without a balance/subscription charge.
 IF NEW.delta_usd=0 AND u.actual_cost>0 THEN RETURN NEW; END IF;
 SELECT timezone INTO zone FROM public.site_profit_meta WHERE id=1;
 SELECT id,cost_multiplier INTO v FROM public.site_profit_cost_versions
 WHERE group_id=u.group_id AND effective_at<=at_time ORDER BY effective_at DESC,version DESC LIMIT 1;
 amount:=round(NEW.delta_usd,8);
 INSERT INTO public.site_profit_entries(billing_entry_id,user_id,group_id,group_name,platform,billing_type,settled_at,day,
   base_usd,revenue_usd,cost_version,cost_multiplier,cost_usd,profit_usd)
 VALUES(NEW.id,NEW.user_id,COALESCE(u.group_id,0),u.group_name,u.platform,NEW.billing_type,at_time,(at_time AT TIME ZONE zone)::date,
   round(u.total_cost,8),amount,v.id,v.cost_multiplier,round(u.total_cost*v.cost_multiplier,8),amount-round(u.total_cost*v.cost_multiplier,8));
 RETURN NEW;
END $$;
CREATE TRIGGER capture_site_profit_entry AFTER INSERT ON billing_usage_entries FOR EACH ROW EXECUTE FUNCTION capture_site_profit();
REVOKE ALL ON FUNCTION capture_site_profit() FROM PUBLIC;
-- Match existing active cluster-node role provisioning, without granting schema/owner privileges.
DO $$ DECLARE node_role text; BEGIN
 IF to_regclass('public.cluster_node_credentials') IS NOT NULL THEN
  FOR node_role IN SELECT DISTINCT c.database_role FROM public.cluster_node_credentials c JOIN pg_roles r ON r.rolname=c.database_role
     WHERE c.status='active' AND r.rolcanlogin LOOP
   EXECUTE format('GRANT SELECT ON public.site_profit_meta,public.site_profit_cost_versions,public.site_profit_entries,public.site_profit_group_daily,public.site_profit_group_totals,public.site_profit_user_totals TO %I',node_role);
   EXECUTE format('GRANT INSERT ON public.site_profit_cost_versions TO %I',node_role);
   EXECUTE format('GRANT UPDATE (aggregated_at,backlog) ON public.site_profit_meta TO %I',node_role);
   EXECUTE format('GRANT UPDATE (aggregated) ON public.site_profit_entries TO %I',node_role);
   EXECUTE format('GRANT INSERT,UPDATE ON public.site_profit_group_daily,public.site_profit_group_totals,public.site_profit_user_totals TO %I',node_role);
   EXECUTE format('GRANT USAGE,SELECT ON SEQUENCE public.site_profit_cost_versions_id_seq TO %I',node_role);
  END LOOP;
 END IF;
END $$;
