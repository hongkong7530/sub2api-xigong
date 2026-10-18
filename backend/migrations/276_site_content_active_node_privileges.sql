-- 274 仅授权旧静态节点角色；集群实际登录账号来自凭据注册表，轮换后不继承这些权限。
-- 只为已登记、在役且仍可登录的节点补内容表和序列权限，不恢复退役凭据。
DO $$
DECLARE
    node_role text;
BEGIN
    FOR node_role IN
        SELECT DISTINCT c.database_role
        FROM public.cluster_node_credentials c
        JOIN pg_roles r ON r.rolname = c.database_role
        WHERE c.status = 'active' AND r.rolcanlogin
    LOOP
        EXECUTE format(
            'GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.site_doc_categories, public.site_documents, public.site_partners TO %I',
            node_role
        );
        EXECUTE format(
            'GRANT USAGE, SELECT ON SEQUENCE public.site_doc_categories_id_seq, public.site_documents_id_seq, public.site_partners_id_seq TO %I',
            node_role
        );
    END LOOP;
END $$;
