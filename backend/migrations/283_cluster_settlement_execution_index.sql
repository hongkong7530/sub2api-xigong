-- Completion and recovery resolve monetary facts by execution identity. Without
-- this index, every recovered execution scans the entire settlement history.
-- Production may prebuild the same index CONCURRENTLY to avoid blocking writes;
-- IF NOT EXISTS makes the transactional migration safe after that online build.
CREATE INDEX IF NOT EXISTS cluster_settlement_events_execution
    ON cluster_settlement_events (execution_id)
    WHERE execution_id IS NOT NULL;
