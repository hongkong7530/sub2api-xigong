-- Only role identifiers are stored here; secret material remains in protected provisioning files.
CREATE TABLE IF NOT EXISTS cluster_node_credentials (
    cluster_id VARCHAR(64) NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    database_role VARCHAR(63) NOT NULL UNIQUE,
    redis_user VARCHAR(128) NOT NULL UNIQUE,
    status VARCHAR(24) NOT NULL DEFAULT 'provisioning',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY(cluster_id,node_id),
    FOREIGN KEY(cluster_id,node_id) REFERENCES cluster_nodes(cluster_id,node_id),
    CHECK(status IN ('provisioning','active','revoking','revoked'))
);
