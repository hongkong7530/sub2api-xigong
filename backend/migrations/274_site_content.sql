-- 独立内容实体；发布快照与草稿分离，回退应用时不删除内容。
CREATE TABLE IF NOT EXISTS site_doc_categories (
 id bigserial PRIMARY KEY, name varchar(80) NOT NULL, position integer NOT NULL DEFAULT 0,
 version integer NOT NULL DEFAULT 1 CHECK (version > 0)
);
CREATE TABLE IF NOT EXISTS site_documents (
 id bigserial PRIMARY KEY, slug varchar(96) NOT NULL UNIQUE,
 draft jsonb NOT NULL, published jsonb,
 version integer NOT NULL DEFAULT 1 CHECK (version > 0),
 updated_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz
);
CREATE INDEX IF NOT EXISTS site_documents_published_idx ON site_documents(id) WHERE published IS NOT NULL;
CREATE TABLE IF NOT EXISTS site_partners (
 id bigserial PRIMARY KEY, name varchar(100) NOT NULL, description text NOT NULL DEFAULT '',
 url text NOT NULL, icon text NOT NULL DEFAULT '', tag varchar(120) NOT NULL DEFAULT '',
 position integer NOT NULL DEFAULT 0, enabled boolean NOT NULL DEFAULT true,
 version integer NOT NULL DEFAULT 1 CHECK (version > 0)
);
-- 既有集群节点角色按属主迁移约定取得新内容表权限；不改现有对象属主。
DO $$
DECLARE node_role text;
BEGIN
 FOR node_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('sub2api_xigong','sub2api_xigong2','sub2api_xigong3') LOOP
  EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON site_doc_categories,site_documents,site_partners TO %I',node_role);
  EXECUTE format('GRANT USAGE,SELECT ON SEQUENCE site_doc_categories_id_seq,site_documents_id_seq,site_partners_id_seq TO %I',node_role);
 END LOOP;
END $$;
