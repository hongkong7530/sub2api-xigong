-- Migration: 270_api_keys_jailbreak_fields
-- 破甲注入所需的 Key 级字段（与 ent schema api_key.go 的 jailbreak 字段保持同步）。
-- 均为带默认值的标量列，避免长时间锁热表。

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_system_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_user_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_template VARCHAR(50) NOT NULL DEFAULT '';

COMMENT ON COLUMN api_keys.jailbreak_enabled IS '是否启用破甲提示词注入（用户自选项，默认关闭）';
COMMENT ON COLUMN api_keys.jailbreak_system_prompt IS '破甲系统提示词（最高优先级，前置到所有系统消息之前）';
COMMENT ON COLUMN api_keys.jailbreak_user_prompt IS '破甲用户提示词（作为第一条用户消息注入）';
COMMENT ON COLUMN api_keys.jailbreak_template IS '预设破甲模板名称（如 guomoPG）';
