-- Cluster v3 is opt-in; this migration does not activate or seed production nodes.
CREATE TABLE IF NOT EXISTS cluster_control (
    cluster_id VARCHAR(64) PRIMARY KEY,
    revision BIGINT NOT NULL DEFAULT 1,
    billing_epoch BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS cluster_nodes (
    cluster_id VARCHAR(64) NOT NULL REFERENCES cluster_control(cluster_id),
    node_id VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    roles JSONB NOT NULL DEFAULT '[]',
    capabilities JSONB NOT NULL DEFAULT '[]',
    address TEXT NOT NULL,
    certificate_sha256 VARCHAR(64) NOT NULL,
    state VARCHAR(24) NOT NULL DEFAULT 'pending',
    revision BIGINT NOT NULL DEFAULT 1,
    base_weight INTEGER NOT NULL DEFAULT 1 CHECK (base_weight BETWEEN 1 AND 256),
    max_in_flight INTEGER NOT NULL CHECK (max_in_flight > 0),
    max_queue INTEGER NOT NULL DEFAULT 0 CHECK (max_queue >= 0),
    db_connections INTEGER NOT NULL CHECK (db_connections > 0),
    monthly_limit_bytes BIGINT CHECK (monthly_limit_bytes >= 0),
    traffic_reserve_bytes BIGINT NOT NULL DEFAULT 0 CHECK (traffic_reserve_bytes >= 0),
    traffic_cycle_day INTEGER NOT NULL DEFAULT 1 CHECK (traffic_cycle_day BETWEEN 1 AND 28),
    traffic_timezone TEXT NOT NULL DEFAULT 'UTC',
    traffic_source VARCHAR(24) NOT NULL DEFAULT 'unknown',
    version TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (cluster_id, node_id),
    UNIQUE (cluster_id, certificate_sha256),
    CHECK (state IN ('pending','ready','paused','draining','retired'))
);
CREATE TABLE IF NOT EXISTS cluster_join_tokens (
    token_sha256 VARCHAR(64) PRIMARY KEY,
    cluster_id VARCHAR(64) NOT NULL REFERENCES cluster_control(cluster_id),
    node_id VARCHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS cluster_operations (
    id UUID PRIMARY KEY,
    cluster_id VARCHAR(64) NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    request_key UUID NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    action VARCHAR(32) NOT NULL,
    expected_revision BIGINT NOT NULL,
    target_revision BIGINT NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    actor_user_id BIGINT NOT NULL,
    error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at TIMESTAMPTZ,
    UNIQUE (cluster_id, request_key)
);
CREATE INDEX IF NOT EXISTS cluster_operations_recent ON cluster_operations(cluster_id, created_at DESC);
CREATE TABLE IF NOT EXISTS cluster_traffic_counters (
    cluster_id VARCHAR(64) NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    cycle_start TIMESTAMPTZ NOT NULL,
    observed_bytes BIGINT NOT NULL DEFAULT 0,
    boot_id TEXT NOT NULL,
    raw_bytes BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(cluster_id, node_id, cycle_start)
);
