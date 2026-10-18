package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// Retention rollups no longer have node identity. Keep them explicitly unassigned
// rather than losing requests or inventing a worker for retained usage.
func (r *usageLogRepository) fillClusterPerformance(ctx context.Context, stats *DashboardStats, end time.Time) error {
	start := end.Add(-5 * time.Minute)
	rows, err := r.sql.QueryContext(ctx, `
WITH recent AS (
 SELECT COALESCE(NULLIF(node_id,''),'unassigned') AS node_id,
        1::bigint AS requests, (input_tokens+output_tokens)::bigint AS tokens
 FROM usage_logs WHERE created_at >= $1 AND created_at < $2
 UNION ALL
 SELECT 'unassigned', request_count, input_tokens+output_tokens
 FROM usage_retained_hourly WHERE bucket_start >= $1 AND bucket_start < $2
), totals AS (
 SELECT node_id, SUM(requests)::bigint AS requests, SUM(tokens)::bigint AS tokens
 FROM recent GROUP BY node_id
), members AS (
 SELECT node_id FROM totals
 UNION
 SELECT node_id FROM cluster_nodes WHERE state <> 'retired'
)
SELECT m.node_id, COALESCE(t.requests,0), COALESCE(t.tokens,0)
FROM members m LEFT JOIN totals t USING(node_id) ORDER BY m.node_id`, start, end)
	if err != nil {
		return err
	}
	defer rows.Close()
	stats.RPMWindowMinutes, stats.RPMScope, stats.PerformanceSampledAt = 5, "cluster_completed_usage", end
	stats.RPMNodes = make([]usagestats.NodePerformance, 0)
	var requests, tokens int64
	for rows.Next() {
		var item usagestats.NodePerformance
		if err := rows.Scan(&item.NodeID, &item.Requests, &item.Tokens); err != nil {
			return err
		}
		item.RPM, item.TPM = float64(item.Requests)/5, float64(item.Tokens)/5
		requests += item.Requests
		tokens += item.Tokens
		stats.RPMNodes = append(stats.RPMNodes, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	stats.Rpm, stats.Tpm = requests/5, tokens/5
	stats.ClusterRPM, stats.ClusterTPM = float64(requests)/5, float64(tokens)/5
	return nil
}
