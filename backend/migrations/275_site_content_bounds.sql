-- 对齐原生 SQL 和 Ent 建库的默认值；仅新增内容表，旧业务表不受影响。
ALTER TABLE site_documents ALTER COLUMN updated_at SET DEFAULT now();
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='site_documents'::regclass AND conname='site_documents_position_check') THEN
  ALTER TABLE site_documents ADD CONSTRAINT site_documents_position_check CHECK ((draft->>'position')::bigint BETWEEN 0 AND 2147483647 AND (published IS NULL OR (published->>'position')::bigint BETWEEN 0 AND 2147483647));
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='site_doc_categories'::regclass AND conname='site_doc_categories_position_check') THEN
  ALTER TABLE site_doc_categories ADD CONSTRAINT site_doc_categories_position_check CHECK (position >= 0);
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='site_partners'::regclass AND conname='site_partners_position_check') THEN
  ALTER TABLE site_partners ADD CONSTRAINT site_partners_position_check CHECK (position >= 0);
 END IF;
END $$;
