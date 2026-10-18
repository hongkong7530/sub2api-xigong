-- 271_fix_restored_table_ownership.sql
--
-- 修复迁移 269 建立的两张表：它们在以 postgres 身份执行时未随宿主约定
-- 归属 sub2api_owner，且没有任何角色 ACL，导致应用角色每次读写都报
-- `permission denied for table api_key_folders`（用户侧密钥夹接口 500）。
--
-- 与迁移 265 的同类修复保持一致：只改属主与授权，不改表结构、不重编号历史迁移。
-- 幂等：重复执行结果一致。

DO $$
BEGIN
    -- 属主对齐：与其它表一致，便于后续 GRANT 由属主继承。
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'api_key_folders') THEN
        ALTER TABLE public.api_key_folders OWNER TO sub2api_owner;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'group_categories') THEN
        ALTER TABLE public.group_categories OWNER TO sub2api_owner;
    END IF;

    -- 序列一并归属，否则 INSERT 取 nextval 仍会被拒。
    IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
               WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname = 'api_key_folders_id_seq') THEN
        ALTER SEQUENCE public.api_key_folders_id_seq OWNER TO sub2api_owner;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
               WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname = 'group_categories_id_seq') THEN
        ALTER SEQUENCE public.group_categories_id_seq OWNER TO sub2api_owner;
    END IF;
END
$$;

-- 授权给当前与在役的应用角色；角色不存在时跳过，避免迁移在其它环境失败。
DO $$
DECLARE
    r text;
BEGIN
    FOR r IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('s2c_d590b46fd7cf9e7a82f4', 's2c_8d920786365f3ef4a572', 'sub2api_owner')
    LOOP
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.api_key_folders TO %I', r);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.group_categories TO %I', r);
        IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
                   WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname = 'api_key_folders_id_seq') THEN
            EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE public.api_key_folders_id_seq TO %I', r);
        END IF;
        IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
                   WHERE n.nspname = 'public' AND c.relkind = 'S' AND c.relname = 'group_categories_id_seq') THEN
            EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE public.group_categories_id_seq TO %I', r);
        END IF;
    END LOOP;
END
$$;
