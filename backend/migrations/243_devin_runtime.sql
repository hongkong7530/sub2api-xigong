-- Devin 的全部持久状态归宿主 PostgreSQL；不引入第二个 SQLite 权威。
CREATE TABLE IF NOT EXISTS devin_gate_latches (
    identity text PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 0,
    limited_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS devin_runtime_states (
    key text PRIMARY KEY,
    value jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS devin_account_health (
    account_id bigint PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    credential_version bigint NOT NULL,
    revision bigint NOT NULL DEFAULT 0,
    state jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS devin_requests (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dir text NOT NULL UNIQUE,
    node_id text NOT NULL,
    request_id text NOT NULL,
    user_id bigint,
    api_key_id bigint,
    account_id bigint,
    group_id bigint,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    active_until timestamptz NOT NULL DEFAULT NOW(),
    status_code integer NOT NULL DEFAULT 0,
    result text NOT NULL DEFAULT 'active',
    model text NOT NULL DEFAULT '',
    api text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT 'proxy',
    summary jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS devin_requests_started_idx ON devin_requests (started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS devin_requests_account_idx ON devin_requests (account_id, started_at DESC);
CREATE INDEX IF NOT EXISTS devin_requests_key_idx ON devin_requests (api_key_id, started_at DESC);
CREATE INDEX IF NOT EXISTS devin_requests_model_idx ON devin_requests (model, started_at DESC);
CREATE INDEX IF NOT EXISTS devin_requests_active_idx ON devin_requests (node_id, result) WHERE result = 'active';

-- dir 是原诊断引用；创建占位在请求进入时执行，生命周期与计费无关。
CREATE TABLE IF NOT EXISTS devin_debug_files (
    dir text NOT NULL,
    name text NOT NULL,
    content bytea NOT NULL,
    usize bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dir, name)
);
CREATE TABLE IF NOT EXISTS devin_debug_chunks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dir text NOT NULL,
    name text NOT NULL,
    data bytea NOT NULL,
    usize bigint NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS devin_debug_chunks_read_idx ON devin_debug_chunks (dir, name, id);

CREATE TABLE IF NOT EXISTS devin_quota_samples (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    credential_version bigint NOT NULL,
    node_id text NOT NULL,
    sampled_at timestamptz NOT NULL DEFAULT NOW(),
    sample jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS devin_quota_samples_account_idx ON devin_quota_samples (account_id, sampled_at DESC);

-- 一条宿主 API Key 的 Devin 专属策略，不保存密钥，不单独签发令牌。
-- 五小时/日/周限额继续走宿主原子账单；自然月限额以实际账单累计补齐。
CREATE TABLE IF NOT EXISTS devin_key_policies (
    api_key_id bigint PRIMARY KEY REFERENCES api_keys(id) ON DELETE CASCADE,
    class text NOT NULL DEFAULT 'fg' CHECK (class IN ('fg', 'bg')),
    max_rpm integer NOT NULL DEFAULT 0 CHECK (max_rpm >= 0),
    max_concurrency integer NOT NULL DEFAULT 0 CHECK (max_concurrency >= 0),
    allowed_models jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(allowed_models) = 'array'),
    monthly_limit_usd numeric(20,8) NOT NULL DEFAULT 0 CHECK (monthly_limit_usd >= 0),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

-- 不同会话 token 若实际属于同一上游用户/团队，不能重复启用为独立配额。
CREATE UNIQUE INDEX IF NOT EXISTS accounts_devin_verified_identity_unique
    ON accounts ((extra->'devin_verified'->>'user_id'), (COALESCE(extra->'devin_verified'->>'team_id', '')))
    WHERE platform = 'devin' AND deleted_at IS NULL
      AND COALESCE(extra->'devin_verified'->>'user_id', '') <> '';
