package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// Ordinary cluster services only verify compatibility; they never attempt DDL.
func verifyClusterSchema(ctx context.Context, db *sql.DB) error {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = '253_cluster_execution_completion.sql')`).Scan(&exists); err != nil {
		return fmt.Errorf("cluster schema is not provisioned; run the standalone migrator: %w", err)
	}
	if !exists {
		return fmt.Errorf("cluster schema is not provisioned; run the standalone migrator")
	}
	return nil
}
