-- Attribute operational metrics to the application node that observed them.
-- Existing rows predate node attribution and remain grouped under the legacy identity.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS node_id VARCHAR(64) NOT NULL DEFAULT 'legacy';
CREATE INDEX IF NOT EXISTS idx_usage_logs_node_created ON usage_logs (node_id, created_at DESC);

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS node_id VARCHAR(64) NOT NULL DEFAULT 'legacy';
CREATE INDEX IF NOT EXISTS idx_ops_error_logs_node_created ON ops_error_logs (node_id, created_at DESC);
