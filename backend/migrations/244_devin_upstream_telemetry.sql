-- Devin 上游增量的诊断与准入遥测仍归宿主 PostgreSQL，不创建独立 SQLite。
CREATE TABLE IF NOT EXISTS devin_debug_blobs (
    hash bytea PRIMARY KEY CHECK (octet_length(hash) = 16),
    content bytea NOT NULL,
    usize bigint NOT NULL DEFAULT 0 CHECK (usize >= 0),
    created_at timestamptz NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS devin_debug_chunk_refs (
    dir text NOT NULL,
    name text NOT NULL,
    hash bytea NOT NULL REFERENCES devin_debug_blobs(hash),
    PRIMARY KEY (dir, name, hash),
    FOREIGN KEY (dir, name) REFERENCES devin_debug_files(dir, name) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS devin_debug_chunk_refs_hash_idx ON devin_debug_chunk_refs(hash);

-- 同一跨节点逻辑批次可安全重发，避免提交成功但确认丢失时追加重复事件。
ALTER TABLE devin_debug_chunks ADD COLUMN IF NOT EXISTS batch_id text;
ALTER TABLE devin_debug_chunks ADD COLUMN IF NOT EXISTS batch_index integer;
CREATE UNIQUE INDEX IF NOT EXISTS devin_debug_chunks_batch_idx
    ON devin_debug_chunks(batch_id, batch_index) WHERE batch_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS devin_quota_samples_identity_time_idx
    ON devin_quota_samples(account_id, credential_version, sampled_at);

CREATE TABLE IF NOT EXISTS devin_gate_windows (
    lane text NOT NULL,
    window_start bigint NOT NULL,
    summary jsonb NOT NULL,
    PRIMARY KEY (lane, window_start)
);
CREATE INDEX IF NOT EXISTS devin_gate_windows_time_idx ON devin_gate_windows(window_start DESC);

CREATE TABLE IF NOT EXISTS devin_detached_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sampled_at timestamptz NOT NULL,
    lane text NOT NULL,
    request_key text NOT NULL,
    origin_dir text NOT NULL DEFAULT '',
    kind text NOT NULL,
    detail text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS devin_detached_events_origin_idx ON devin_detached_events(origin_dir, sampled_at DESC);
CREATE INDEX IF NOT EXISTS devin_detached_events_time_idx ON devin_detached_events(sampled_at DESC);

-- 单个请求的换号原因幂等覆盖，避免日志重放导致累计多算；日聚合由读取方完成。
CREATE TABLE IF NOT EXISTS devin_lane_attempt_causes (
    dir text NOT NULL,
    day date NOT NULL,
    lane text NOT NULL,
    cause text NOT NULL,
    n bigint NOT NULL CHECK (n > 0),
    PRIMARY KEY (dir, lane, cause)
);
CREATE INDEX IF NOT EXISTS devin_lane_attempt_causes_day_idx ON devin_lane_attempt_causes(day, lane, cause);

-- 表行级触发器维护共享库存计数：适用于跨节点写入和旧版本回滚后的删除。
-- 不依赖进程私有累计值，也不把整个宿主数据库大小误作 Devin 的容量。
CREATE TABLE IF NOT EXISTS devin_payload_inventory (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    bytes bigint NOT NULL DEFAULT 0 CHECK (bytes >= 0)
);
LOCK TABLE devin_debug_files, devin_debug_chunks, devin_debug_blobs, devin_debug_chunk_refs IN SHARE ROW EXCLUSIVE MODE;
INSERT INTO devin_payload_inventory(id, bytes)
SELECT true, COALESCE(SUM(n),0) FROM (
    SELECT octet_length(content)::bigint AS n FROM devin_debug_files
    UNION ALL SELECT octet_length(data)::bigint FROM devin_debug_chunks
    UNION ALL SELECT octet_length(content)::bigint FROM devin_debug_blobs
    UNION ALL SELECT (octet_length(dir)+octet_length(name)+octet_length(hash))::bigint FROM devin_debug_chunk_refs
) payload ON CONFLICT(id) DO NOTHING;

CREATE OR REPLACE FUNCTION devin_track_payload_inventory() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    old_size bigint := 0;
    new_size bigint := 0;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        IF TG_TABLE_NAME = 'devin_debug_chunks' THEN old_size := octet_length(OLD.data);
        ELSIF TG_TABLE_NAME = 'devin_debug_chunk_refs' THEN old_size := octet_length(OLD.dir)+octet_length(OLD.name)+octet_length(OLD.hash);
        ELSE old_size := octet_length(OLD.content);
        END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        IF TG_TABLE_NAME = 'devin_debug_chunks' THEN new_size := octet_length(NEW.data);
        ELSIF TG_TABLE_NAME = 'devin_debug_chunk_refs' THEN new_size := octet_length(NEW.dir)+octet_length(NEW.name)+octet_length(NEW.hash);
        ELSE new_size := octet_length(NEW.content);
        END IF;
    END IF;
    UPDATE devin_payload_inventory SET bytes=GREATEST(0, bytes + new_size - old_size) WHERE id=true;
    RETURN NULL;
END;
$$;
CREATE TRIGGER devin_debug_files_inventory AFTER INSERT OR UPDATE OR DELETE ON devin_debug_files FOR EACH ROW EXECUTE FUNCTION devin_track_payload_inventory();
CREATE TRIGGER devin_debug_chunks_inventory AFTER INSERT OR UPDATE OR DELETE ON devin_debug_chunks FOR EACH ROW EXECUTE FUNCTION devin_track_payload_inventory();
CREATE TRIGGER devin_debug_blobs_inventory AFTER INSERT OR UPDATE OR DELETE ON devin_debug_blobs FOR EACH ROW EXECUTE FUNCTION devin_track_payload_inventory();
CREATE TRIGGER devin_debug_refs_inventory AFTER INSERT OR UPDATE OR DELETE ON devin_debug_chunk_refs FOR EACH ROW EXECUTE FUNCTION devin_track_payload_inventory();
