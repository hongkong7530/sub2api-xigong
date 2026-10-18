-- No runtime mode is activated by this additive migration.
ALTER TABLE cluster_control ADD COLUMN IF NOT EXISTS redis_generation TEXT NOT NULL DEFAULT '';
ALTER TABLE cluster_control ADD COLUMN IF NOT EXISTS redis_recovery_required BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE cluster_control ADD COLUMN IF NOT EXISTS settlement_mode VARCHAR(16) NOT NULL DEFAULT 'legacy';
CREATE TABLE IF NOT EXISTS cluster_executions (
    execution_id UUID PRIMARY KEY,
    cluster_id VARCHAR(64) NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    boot_id UUID NOT NULL,
    protocol TEXT NOT NULL,
    state VARCHAR(32) NOT NULL DEFAULT 'accepted',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS cluster_executions_unfinished ON cluster_executions(cluster_id,node_id,state) WHERE state NOT IN ('settled','rejected');
CREATE TABLE IF NOT EXISTS cluster_settlement_events (
    event_id TEXT NOT NULL,
    api_key_id BIGINT NOT NULL,
    execution_id UUID,
    cluster_id VARCHAR(64) NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    command JSONB NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    error_code VARCHAR(64),
    PRIMARY KEY(event_id,api_key_id)
);
CREATE INDEX IF NOT EXISTS cluster_settlements_pending ON cluster_settlement_events(cluster_id,node_id,created_at) WHERE state <> 'committed';
