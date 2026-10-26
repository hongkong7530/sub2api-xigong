package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"golang.org/x/sync/errgroup"
)

// Real requests are read, not copied into probe history. Monitor-owned usage is
// excluded so the same paid active check cannot count twice. Resolved retry
// failures and local business-limit rejections do not describe channel health.
// A terminal error can precede model resolution. Keep it in health statistics
// with an empty model instead of allowing NULL to abort the whole read.
const monitorHealthSourcesSQL = `WITH targets AS (
 SELECT id AS monitor_id, group_id,probe_mode FROM channel_monitors WHERE id=ANY($1::bigint[])
), terminal_errors AS (
 SELECT DISTINCT ON (e.api_key_id,COALESCE(NULLIF(e.request_id,''),'error:'||e.id::text))
  e.id,e.api_key_id,e.request_id,e.group_id,e.requested_model,e.model,e.duration_ms,e.time_to_first_token_ms,e.created_at
 FROM ops_error_logs e
 WHERE e.created_at>=NOW()-($2::int*INTERVAL '1 day')
 AND NOT e.resolved AND NOT e.is_count_tokens AND NOT e.is_business_limited
 AND COALESCE(e.status_code,0)>=400 AND e.error_owner<>'client'
 ORDER BY e.api_key_id,COALESCE(NULLIF(e.request_id,''),'error:'||e.id::text),e.id DESC
), error_keys AS (
 SELECT e.api_key_id,keys.request_id FROM terminal_errors e
 CROSS JOIN LATERAL unnest(ARRAY[e.request_id,'local:'||e.request_id,'client:'||e.request_id]) AS keys(request_id)
 WHERE NULLIF(e.request_id,'') IS NOT NULL
)`

// Card rates describe the observed receive window, not reward eligibility.
// Keep uncertain forwarding timing separate from ratemetric.IsEligible.
const monitorHealthFactsSQL = monitorHealthSourcesSQL + `, facts AS (
 SELECT t.monitor_id,u.id AS fact_id,COALESCE(NULLIF(u.requested_model,''),u.model,'') AS model,
 CASE WHEN COALESCE(u.first_token_ms,0)>=10000 THEN 'degraded' ELSE 'operational' END AS status,
 u.duration_ms AS latency_ms,u.first_token_ms,CASE WHEN u.output_tokens>0 AND u.output_duration_ms>=1000 AND u.rate_observation->>'quality' IN ('observed','timing_uncertain') AND (u.rate_observation->>'version')::int=1 AND (u.rate_observation->>'window_ms')::int=u.output_duration_ms AND (u.rate_observation->>'output_events')::int>=3 AND (u.rate_observation->>'event_span_ms')::int>=250 AND (u.rate_observation->>'output_bytes')::bigint>0 AND (u.rate_observation->>'max_burst_bytes')::bigint>=0 AND (u.rate_observation->>'max_burst_bytes')::bigint*5<(u.rate_observation->>'output_bytes')::bigint*4 AND COALESCE(u.request_type,0) IN(2,3) AND COALESCE(u.image_count,0)=0 AND COALESCE(u.image_output_tokens,0)=0 AND COALESCE(u.billing_mode,'token')='token' THEN u.output_tokens::float8*1000/u.output_duration_ms ELSE NULL::float8 END AS output_tokens_per_second,u.created_at AS checked_at,'user_request'::text AS source,1 AS fact_kind
 FROM targets t JOIN usage_logs u ON u.group_id=t.group_id
 WHERE u.created_at>=NOW()-($2::int*INTERVAL '1 day') AND COALESCE(u.request_type,0)<>4
 AND (u.input_tokens+u.output_tokens+u.cache_read_tokens+u.cache_creation_tokens>0 OR u.actual_cost>0 OR COALESCE(u.image_count,0)>0)
 AND NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=u.api_key_id)
 AND NOT EXISTS(SELECT 1 FROM error_keys e WHERE e.api_key_id=u.api_key_id AND e.request_id=u.request_id)
 UNION ALL
 SELECT t.monitor_id,e.id,COALESCE(NULLIF(e.requested_model,''),e.model,''),'error',e.duration_ms,e.time_to_first_token_ms,NULL::float8,e.created_at,'user_request',2 AS fact_kind
 FROM targets t JOIN terminal_errors e ON e.group_id=t.group_id
 WHERE NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=e.api_key_id)
 UNION ALL
 SELECT t.monitor_id,h.id,h.model,h.status::text,h.latency_ms,h.first_token_ms,CASE WHEN h.status IN('operational','degraded') AND h.output_rate_version=2 THEN h.output_tokens_per_second ELSE NULL::float8 END,h.checked_at,'probe',0 AS fact_kind
 FROM targets t JOIN channel_monitor_histories h ON h.monitor_id=t.monitor_id
 WHERE t.probe_mode<>'passive' AND h.checked_at>=NOW()-($2::int*INTERVAL '1 day')
)`

// Aggregate each source before combining it: availability needs counts and latency
// sums, not a sorted copy of every output event. Keep the full observation window.
const monitorHealthAvailabilitySQL = monitorHealthSourcesSQL + `, source_totals AS (
 SELECT t.monitor_id,a.* FROM targets t CROSS JOIN LATERAL (
 SELECT COALESCE(NULLIF(u.requested_model,''),u.model,'') AS model,
 COUNT(*) AS total,COUNT(*) AS operational,SUM(u.duration_ms)::numeric AS latency_sum,COUNT(u.duration_ms) AS latency_count
 FROM usage_logs u
 WHERE u.group_id=t.group_id AND u.created_at>=NOW()-($2::int*INTERVAL '1 day') AND COALESCE(u.request_type,0)<>4
 AND (u.input_tokens+u.output_tokens+u.cache_read_tokens+u.cache_creation_tokens>0 OR u.actual_cost>0 OR COALESCE(u.image_count,0)>0)
 AND NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=u.api_key_id)
 AND (u.api_key_id IS NULL OR u.request_id IS NULL OR (u.api_key_id,u.request_id) NOT IN (
  SELECT e.api_key_id,e.request_id FROM error_keys e WHERE e.api_key_id IS NOT NULL))
 GROUP BY COALESCE(NULLIF(u.requested_model,''),u.model,'')
 )a
 UNION ALL
 SELECT t.monitor_id,COALESCE(NULLIF(e.requested_model,''),e.model,''),COUNT(*),0,SUM(e.duration_ms)::numeric,COUNT(e.duration_ms)
 FROM targets t JOIN terminal_errors e ON e.group_id=t.group_id
 WHERE NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=e.api_key_id)
 GROUP BY t.monitor_id,COALESCE(NULLIF(e.requested_model,''),e.model,'')
 UNION ALL
 SELECT t.monitor_id,h.model,COUNT(*),COUNT(*) FILTER(WHERE h.status IN('operational','degraded')),SUM(h.latency_ms)::numeric,COUNT(h.latency_ms)
 FROM targets t JOIN channel_monitor_histories h ON h.monitor_id=t.monitor_id
 WHERE t.probe_mode<>'passive' AND h.checked_at>=NOW()-($2::int*INTERVAL '1 day')
 GROUP BY t.monitor_id,h.model
)
SELECT monitor_id,model,SUM(total)::bigint,SUM(operational)::bigint,
 SUM(latency_sum)/NULLIF(SUM(latency_count),0)
FROM source_totals GROUP BY monitor_id,model`

func (r *channelMonitorRepository) ReadMonitorHealth(ctx context.Context, ids []int64, days, limit int) (map[int64]*service.ChannelMonitorHealth, error) {
	out := make(map[int64]*service.ChannelMonitorHealth, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if days < 1 || days > 30 {
		return nil, fmt.Errorf("monitor health window must be 1-30 days")
	}
	if limit < 0 || limit > 60 {
		return nil, fmt.Errorf("monitor health recent limit must be 0-60")
	}
	for _, id := range ids {
		out[id] = &service.ChannelMonitorHealth{Availability: &service.ChannelMonitorAvailability{WindowDays: days}, Models: map[string]*service.ChannelMonitorAvailability{}}
	}
	// Both reads use the original complete windows and filters. Keep their result
	// owners separate until both succeed, so a failed read cannot publish partial health.
	reads, readCtx := errgroup.WithContext(ctx)
	reads.Go(func() error {
		rows, err := r.db.QueryContext(readCtx, monitorHealthAvailabilitySQL, pq.Array(ids), days)
		if err != nil {
			return fmt.Errorf("monitor health availability: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var avg sql.NullFloat64
			a := &service.ChannelMonitorAvailability{WindowDays: days}
			if err := rows.Scan(&id, &a.Model, &a.TotalChecks, &a.OperationalChecks, &avg); err != nil {
				return err
			}
			finalizeAvailabilityRow(a, avg)
			h := out[id]
			h.Models[a.Model] = a
			h.Availability.TotalChecks += a.TotalChecks
			h.Availability.OperationalChecks += a.OperationalChecks
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, h := range out {
			finalizeAvailabilityRow(h.Availability, sql.NullFloat64{})
		}
		return nil
	})

	recent := make(map[int64][]*service.ChannelMonitorHistoryEntry, len(ids))
	if limit > 0 {
		reads.Go(func() error {
			rows, err := r.db.QueryContext(readCtx, monitorRecentHealthSQL, pq.Array(ids), 30, limit)
			if err != nil {
				return fmt.Errorf("monitor health recent: %w", err)
			}
			defer rows.Close()
			for rows.Next() {
				var id int64
				var latency, first sql.NullInt64
				var rate sql.NullFloat64
				e := &service.ChannelMonitorHistoryEntry{}
				if err := rows.Scan(&id, &e.Model, &e.Status, &latency, &first, &rate, &e.CheckedAt, &e.Source); err != nil {
					return err
				}
				assignNullInt(&e.LatencyMs, latency)
				assignNullInt(&e.FirstTokenMs, first)
				if rate.Valid {
					e.OutputTokensPerSecond = &rate.Float64
				}
				recent[id] = append(recent[id], e)
			}
			return rows.Err()
		})
	}
	if err := reads.Wait(); err != nil {
		return nil, err
	}
	for id, health := range out {
		health.Recent = recent[id]
	}
	return out, nil
}
