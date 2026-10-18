package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Each lateral branch reads one recent observation through the group/time indexes.
// No user, key, account identifiers or error bodies leave the repository.
func (r *channelMonitorRepository) ListLatestGroupObservations(ctx context.Context, groupIDs []int64, since time.Time) (map[int64]*service.ChannelMonitorObservation, error) {
	out := make(map[int64]*service.ChannelMonitorObservation, len(groupIDs))
	if len(groupIDs) == 0 {
		return out, nil
	}
	const query = `SELECT targets.group_id, recent.model, recent.first_token_ms, recent.duration_ms, recent.observed_at, recent.failed
	FROM unnest($1::bigint[]) AS targets(group_id)
	CROSS JOIN LATERAL (
		SELECT * FROM (
			(SELECT COALESCE(NULLIF(u.requested_model,''),u.model) AS model,
			 u.first_token_ms, u.duration_ms, u.created_at AS observed_at, FALSE AS failed
			 FROM usage_logs u
			 WHERE u.group_id=targets.group_id AND u.created_at >= $2
			 AND NOT EXISTS (SELECT 1 FROM channel_monitors m WHERE m.provisioned_api_key_id=u.api_key_id)
			 AND (u.request_type IS NULL OR u.request_type<>4)
			 AND (u.input_tokens+u.output_tokens+u.cache_read_tokens+u.cache_creation_tokens>0 OR u.actual_cost>0 OR COALESCE(u.image_count,0)>0)
			 ORDER BY u.created_at DESC,u.id DESC LIMIT 1)
			UNION ALL
			(SELECT COALESCE(NULLIF(e.requested_model,''),e.model),
			 e.time_to_first_token_ms,e.duration_ms,e.created_at,TRUE
			 FROM ops_error_logs e
			 WHERE e.group_id=targets.group_id AND e.created_at >= $2
			 AND NOT e.is_count_tokens AND NOT e.is_business_limited AND NOT e.resolved
			 AND COALESCE(e.status_code,0)>=400 AND e.error_owner<>'client'
			 AND NOT EXISTS (SELECT 1 FROM channel_monitors m WHERE m.provisioned_api_key_id=e.api_key_id)
			 AND NOT EXISTS (SELECT 1 FROM usage_logs u WHERE u.api_key_id=e.api_key_id
			   AND NULLIF(e.request_id,'') IS NOT NULL AND u.request_id=e.request_id)
			 ORDER BY e.created_at DESC,e.id DESC LIMIT 1)
		) observations ORDER BY observed_at DESC,failed DESC LIMIT 1
	) recent`
	rows, err := r.db.QueryContext(ctx, query, pq.Array(groupIDs), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var groupID int64
		var firstToken, duration sql.NullInt64
		o := &service.ChannelMonitorObservation{}
		if err := rows.Scan(&groupID, &o.Model, &firstToken, &duration, &o.ObservedAt, &o.Failed); err != nil {
			return nil, err
		}
		assignNullInt(&o.FirstTokenMs, firstToken)
		assignNullInt(&o.DurationMs, duration)
		out[groupID] = o
	}
	return out, rows.Err()
}
