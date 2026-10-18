-- 站点素材由共享主库存储，避免多节点本地文件和过期预签名地址。
CREATE TABLE IF NOT EXISTS site_assets (
    id varchar(64) PRIMARY KEY CHECK (id ~ '^[0-9a-f]{64}$'),
    content_type text NOT NULL CHECK (content_type = 'image/jpeg'),
    body bytea NOT NULL CHECK (octet_length(body) <= 8388608),
    created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE site_partners ADD COLUMN IF NOT EXISTS thumbnail text NOT NULL DEFAULT '';
DO $$
DECLARE node_role text;
BEGIN
    FOR node_role IN
        SELECT DISTINCT c.database_role FROM public.cluster_node_credentials c
        JOIN pg_roles r ON r.rolname=c.database_role
        WHERE c.status='active' AND r.rolcanlogin
    LOOP
        EXECUTE format('GRANT SELECT, INSERT ON TABLE public.site_assets TO %I', node_role);
    END LOOP;
END $$;
