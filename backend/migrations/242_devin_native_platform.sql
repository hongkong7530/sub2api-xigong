-- Devin 是独立上游渠道，不按其模型品牌归入 OpenAI/Anthropic。
-- 只扩展约束；回滚应用时保留 Devin 数据，不缩减既有平台集合。
ALTER TABLE user_platform_quotas DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;
ALTER TABLE user_platform_quotas ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'devin'));

ALTER TABLE composite_model_routes DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;
ALTER TABLE composite_model_routes ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'devin'));

ALTER TABLE channel_monitors DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
ALTER TABLE channel_monitors ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                       'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'devin'));

ALTER TABLE channel_monitor_request_templates DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
ALTER TABLE channel_monitor_request_templates ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                       'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'devin'));

-- 指纹由下方 BEFORE 触发器强制重算，调用方无法伪造。
-- 索引只读 JSONB 字段，避免 convert_to 的 STABLE 属性阻止表达式索引创建。
CREATE UNIQUE INDEX IF NOT EXISTS accounts_devin_session_identity_unique
    ON accounts ((credentials->>'credential_fingerprint'))
    WHERE platform = 'devin' AND deleted_at IS NULL
      AND COALESCE(credentials->>'session_token', '') <> '';

-- 文件引用是配置根内的规范化相对路径；同一节点/文件只允许一个活账号。
CREATE UNIQUE INDEX IF NOT EXISTS accounts_devin_source_identity_unique
    ON accounts ((credentials->>'credentials_node'), (credentials->>'credentials_file'))
    WHERE platform = 'devin' AND deleted_at IS NULL
      AND COALESCE(credentials->>'credentials_file', '') <> '';

-- credential_version 只在身份/端点/凭据源改变时递增；普通名称编辑不解禁旧冷却。
-- 指纹由数据库重算，批量 JSONB 合并也不能绕过同一约束。
CREATE OR REPLACE FUNCTION normalize_devin_account_credentials() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    incoming_token text;
    old_version bigint;
BEGIN
    IF NEW.platform <> 'devin' THEN
        RETURN NEW;
    END IF;
    incoming_token := NEW.credentials->>'session_token';
    IF NEW.type <> 'apikey'
       OR NEW.credentials->>'auth_mode' IS DISTINCT FROM 'session_token'
       OR jsonb_typeof(NEW.credentials->'session_token') IS DISTINCT FROM 'string'
       OR COALESCE(incoming_token, '') = ''
       OR length(incoming_token) > 16384
       OR incoming_token ~ '[[:space:]]'
       OR NEW.credentials ?| ARRAY['credentials_content', 'token', 'api_key', 'windsurf_api_key'] THEN
        RAISE EXCEPTION 'invalid Devin credential record' USING ERRCODE = '23514';
    END IF;
    IF (COALESCE(NEW.credentials->>'credentials_file', '') = '')
       <> (COALESCE(NEW.credentials->>'credentials_node', '') = '') THEN
        RAISE EXCEPTION 'incomplete Devin credential source' USING ERRCODE = '23514';
    END IF;
    IF COALESCE(NEW.credentials->>'credentials_file', '') <> '' AND (
        NEW.credentials->>'credentials_file' ~ '(^/|(^|/)\.\.?(/|$)|//|:)'
        OR position(chr(92) in NEW.credentials->>'credentials_file') > 0
        OR NEW.credentials->>'credentials_node' !~ '^[a-z0-9._-]+$') THEN
        RAISE EXCEPTION 'noncanonical Devin credential source' USING ERRCODE = '23514';
    END IF;
    NEW.credentials := jsonb_set(NEW.credentials, '{credential_fingerprint}',
        to_jsonb(encode(sha256(convert_to(incoming_token, 'UTF8')), 'hex')), true);
    IF TG_OP = 'INSERT' THEN
        NEW.credentials := jsonb_set(NEW.credentials, '{credential_version}', '1'::jsonb, true);
    ELSE
        old_version := COALESCE((OLD.credentials->>'credential_version')::bigint, 0);
        IF OLD.platform IS DISTINCT FROM NEW.platform
           OR OLD.credentials->'session_token' IS DISTINCT FROM NEW.credentials->'session_token'
           OR OLD.credentials->'base_url' IS DISTINCT FROM NEW.credentials->'base_url'
           OR OLD.credentials->'credentials_file' IS DISTINCT FROM NEW.credentials->'credentials_file'
           OR OLD.credentials->'credentials_node' IS DISTINCT FROM NEW.credentials->'credentials_node' THEN
            old_version := old_version + 1;
        END IF;
        NEW.credentials := jsonb_set(NEW.credentials, '{credential_version}', to_jsonb(GREATEST(old_version, 1)), true);
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS accounts_devin_credentials ON accounts;
CREATE TRIGGER accounts_devin_credentials BEFORE INSERT OR UPDATE OF platform, type, credentials
    ON accounts FOR EACH ROW EXECUTE FUNCTION normalize_devin_account_credentials();
