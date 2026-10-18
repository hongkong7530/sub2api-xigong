-- Completed responses are restart-cache data, never another billing authority.
-- Publication requires the original host usage transaction to be visible.
CREATE TABLE IF NOT EXISTS devin_detached_blobs (
    request_key TEXT PRIMARY KEY,
    lane TEXT NOT NULL,
    origin_dir TEXT NOT NULL DEFAULT '',
    billing_request_id TEXT NOT NULL,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    credential_version BIGINT NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    payload BYTEA NOT NULL,
    CONSTRAINT devin_detached_key_shape CHECK (request_key ~ '^[0-9a-f]{64}$'),
    CONSTRAINT devin_detached_payload_bound CHECK (octet_length(payload) BETWEEN 1 AND 67108864)
);

CREATE INDEX IF NOT EXISTS idx_devin_detached_lane_finished
    ON devin_detached_blobs(lane, finished_at);
CREATE INDEX IF NOT EXISTS idx_devin_detached_finished
    ON devin_detached_blobs(finished_at);
