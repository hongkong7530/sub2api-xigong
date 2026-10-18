-- Migration: 269_group_categories_and_key_folders
-- 管理员分组分类目录 + 用户 API Key 钥匙夹。
-- 分类只整理用户选组，不参与调度/计费；钥匙夹只整理用户自己的 Key，不改 api_keys.group_id。
-- 新表和可空列，避免长时间锁热表。

CREATE TABLE IF NOT EXISTS group_categories (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_group_categories_name_lower
    ON group_categories (LOWER(name));

CREATE INDEX IF NOT EXISTS idx_group_categories_sort_order
    ON group_categories (sort_order, id);

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS category_id BIGINT REFERENCES group_categories(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_groups_category_id
    ON groups (category_id)
    WHERE category_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS api_key_folders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(40) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_key_folders_user_name_lower
    ON api_key_folders (user_id, LOWER(name));

CREATE INDEX IF NOT EXISTS idx_api_key_folders_user_sort
    ON api_key_folders (user_id, sort_order, id);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS folder_id BIGINT REFERENCES api_key_folders(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_api_keys_user_folder
    ON api_keys (user_id, folder_id)
    WHERE deleted_at IS NULL AND folder_id IS NOT NULL;

COMMENT ON TABLE group_categories IS
    '管理员维护的路由分组分类目录，供用户创建/切换 API Key 时按分类选组。';
COMMENT ON TABLE api_key_folders IS
    '用户私有 API Key 钥匙夹，仅用于整理自己的 Key，不影响路由分组。';
