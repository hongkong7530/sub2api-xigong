-- Restore the local Ops cluster telemetry contract under a fresh migration number.
-- Existing numbered migrations are left untouched; no production nodes are seeded here.
ALTER TABLE ops_system_metrics
  ADD COLUMN IF NOT EXISTS node_id VARCHAR(64) NOT NULL DEFAULT 'legacy',
  ADD COLUMN IF NOT EXISTS node_name VARCHAR(128) NOT NULL DEFAULT 'Legacy node',
  ADD COLUMN IF NOT EXISTS cpu_cores INTEGER,
  ADD COLUMN IF NOT EXISTS load_1 DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS load_5 DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS load_15 DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS memory_available_mb BIGINT,
  ADD COLUMN IF NOT EXISTS swap_used_mb BIGINT,
  ADD COLUMN IF NOT EXISTS swap_total_mb BIGINT;

CREATE INDEX IF NOT EXISTS idx_ops_system_metrics_node_window_time
  ON ops_system_metrics (node_id, window_minutes, created_at DESC)
  WHERE platform IS NULL AND group_id IS NULL;

ALTER TABLE ops_job_heartbeats
  ADD COLUMN IF NOT EXISTS node_id VARCHAR(64) NOT NULL DEFAULT 'legacy';
ALTER TABLE ops_job_heartbeats DROP CONSTRAINT IF EXISTS ops_job_heartbeats_pkey;
ALTER TABLE ops_job_heartbeats ADD CONSTRAINT ops_job_heartbeats_pkey PRIMARY KEY (node_id, job_name);
CREATE INDEX IF NOT EXISTS idx_ops_job_heartbeats_node_updated
  ON ops_job_heartbeats (node_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS ops_cluster_node_configs (
  node_id VARCHAR(64) PRIMARY KEY,
  node_name VARCHAR(128) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  cpu_limit_percent DOUBLE PRECISION NOT NULL CHECK (cpu_limit_percent >= 50 AND cpu_limit_percent <= 100),
  min_available_memory_mb BIGINT NOT NULL CHECK (min_available_memory_mb >= 128),
  desired_weight INTEGER NOT NULL CHECK (desired_weight BETWEEN 0 AND 20),
  recovery_stable_seconds INTEGER NOT NULL CHECK (recovery_stable_seconds BETWEEN 10 AND 600),
  desired_revision BIGINT NOT NULL DEFAULT 1,
  updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS ops_cluster_node_status (
  node_id VARCHAR(64) PRIMARY KEY REFERENCES ops_cluster_node_configs(node_id) ON DELETE CASCADE,
  heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  observed_revision BIGINT NOT NULL DEFAULT 0,
  apply_status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (apply_status IN ('pending','applied','error')),
  apply_error TEXT,
  applied_at TIMESTAMPTZ,
  admission_state VARCHAR(32) NOT NULL DEFAULT 'stale',
  admission_reason TEXT,
  recovery_started_at TIMESTAMPTZ,
  actual_weight INTEGER NOT NULL DEFAULT 0 CHECK (actual_weight BETWEEN 0 AND 20),
  haproxy_state VARCHAR(16) NOT NULL DEFAULT 'unknown',
  active_requests INTEGER NOT NULL DEFAULT 0,
  queue_depth INTEGER NOT NULL DEFAULT 0,
  postgres_sync_state VARCHAR(32) NOT NULL DEFAULT 'unknown',
  wal_replay_lag_bytes BIGINT,
  live_metrics JSONB,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS ops_cluster_config_audit (
  id BIGSERIAL PRIMARY KEY,
  node_id VARCHAR(64) NOT NULL,
  previous_revision BIGINT NOT NULL,
  desired_revision BIGINT NOT NULL,
  old_config JSONB NOT NULL,
  new_config JSONB NOT NULL,
  operator_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ops_cluster_node_status_heartbeat
  ON ops_cluster_node_status (heartbeat_at DESC);
CREATE INDEX IF NOT EXISTS idx_ops_cluster_config_audit_node_revision
  ON ops_cluster_config_audit (node_id, desired_revision DESC);
