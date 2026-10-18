package repository

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsNodeAttributionMigrationAddsNodeColumnAndIndex(t *testing.T) {
	migration, err := os.ReadFile("../../migrations/260_ops_node_error_attribution.sql")
	require.NoError(t, err)
	sql := string(migration)
	require.Contains(t, sql, "ALTER TABLE usage_logs")
	require.Contains(t, sql, "ALTER TABLE ops_error_logs")
	require.Equal(t, 2, strings.Count(sql, "ADD COLUMN IF NOT EXISTS node_id VARCHAR(64) NOT NULL DEFAULT 'legacy'"))
	require.Contains(t, sql, "ON usage_logs (node_id, created_at DESC)")
	require.Contains(t, sql, "ON ops_error_logs (node_id, created_at DESC)")
}
