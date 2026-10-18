package cluster

import (
	"context"
	"fmt"
	"time"
)

func TrafficCycleStart(now time.Time, day int, zone string) (time.Time, error) {
	if day < 1 || day > 28 {
		return time.Time{}, fmt.Errorf("cycle day must be between 1 and 28")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), day, 0, 0, 0, 0, loc)
	if local.Before(start) {
		start = start.AddDate(0, -1, 0)
	}
	return start.UTC(), nil
}

// ObserveTraffic checkpoints at minute cadence. Initial counters are not a provider bill.
// Reboots and counter resets retain accumulated usage rather than making it negative.
func (s *Store) ObserveTraffic(ctx context.Context, n Node, boot string, raw int64, now time.Time) (int64, error) {
	start, err := TrafficCycleStart(now, n.TrafficCycleDay, n.TrafficTimezone)
	if err != nil {
		return 0, err
	}
	if raw < 0 {
		return 0, fmt.Errorf("negative interface counter")
	}
	var total int64
	err = s.DB.QueryRowContext(ctx, `INSERT INTO cluster_traffic_counters(cluster_id,node_id,cycle_start,boot_id,raw_bytes,observed_bytes)
VALUES($1,$2,$3,$4,$5,0)
ON CONFLICT(cluster_id,node_id,cycle_start) DO UPDATE SET
observed_bytes=cluster_traffic_counters.observed_bytes+CASE WHEN cluster_traffic_counters.boot_id=EXCLUDED.boot_id AND EXCLUDED.raw_bytes>=cluster_traffic_counters.raw_bytes THEN EXCLUDED.raw_bytes-cluster_traffic_counters.raw_bytes ELSE EXCLUDED.raw_bytes END,
boot_id=EXCLUDED.boot_id,raw_bytes=EXCLUDED.raw_bytes,updated_at=NOW()
RETURNING observed_bytes`, s.ClusterID, n.ID, start, boot, raw).Scan(&total)
	return total, err
}

// Cycle observations deliberately remain estimates; complete provider accounting is never inferred.
func TrafficBudgetExhausted(n Node, used *int64) bool {
	return n.MonthlyLimitBytes != nil && used != nil && *used >= *n.MonthlyLimitBytes-n.TrafficReserveBytes
}
