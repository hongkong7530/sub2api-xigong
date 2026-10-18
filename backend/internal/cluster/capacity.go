package cluster

import "context"

type DatabaseCapacity struct {
	Budget         int  `json:"budget"`
	Allocated      int  `json:"allocated"`
	Available      int  `json:"available"`
	ServerMax      *int `json:"server_max"`
	ServerUsed     *int `json:"server_used"`
	ServerReserved *int `json:"server_reserved"`
}

// Pool allocations and simultaneous gateway requests are independent budgets.
func (s *Store) DatabaseCapacity(ctx context.Context) (*DatabaseCapacity, error) {
	result := &DatabaseCapacity{Budget: s.ConnectionBudget}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(db_connections),0) FROM cluster_nodes WHERE cluster_id=$1 AND state<>'retired'`, s.ClusterID).Scan(&result.Allocated); err != nil {
		return nil, err
	}
	result.Available = max(0, result.Budget-result.Allocated)
	var maximum, used, reserved int
	if err := s.DB.QueryRowContext(ctx, `SELECT current_setting('max_connections')::int, (SELECT count(*) FROM pg_stat_activity), current_setting('superuser_reserved_connections')::int + COALESCE(NULLIF(current_setting('reserved_connections',true),''),'0')::int`).Scan(&maximum, &used, &reserved); err == nil {
		result.ServerMax, result.ServerUsed, result.ServerReserved = &maximum, &used, &reserved
	}
	return result, nil
}
