-- 破甲注入所需的 Key 级字段。迁移241在本地升级线上已用于其它字段，故此版本沿用257。
-- 标量列带默认值，避免长时间锁热表。

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_system_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_user_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS jailbreak_template VARCHAR(50) NOT NULL DEFAULT '';

COMMENT ON COLUMN api_keys.jailbreak_enabled IS '是否启用破甲提示词注入（用户自选项，默认关闭）';
COMMENT ON COLUMN api_keys.jailbreak_system_prompt IS '破甲系统提示词（最高优先级，前置到所有系统消息之前）';
COMMENT ON COLUMN api_keys.jailbreak_user_prompt IS '破甲用户提示词（作为第一条用户消息注入）';
COMMENT ON COLUMN api_keys.jailbreak_template IS '预设破甲模板名称（如 guomoPG）';
