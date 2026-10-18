package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Real requests are read, not copied into probe history. Monitor-owned usage is
// excluded so the same paid active check cannot count twice. Resolved retry
// failures and local business-limit rejections do not describe channel health.
// A terminal error can precede model resolution. Keep it in health statistics
// with an empty model instead of allowing NULL to abort the whole read.
const monitorHealthFactsSQL = `WITH targets AS (
 SELECT id AS monitor_id, group_id FROM channel_monitors WHERE id=ANY($1::bigint[])
), terminal_errors AS (
 SELECT DISTINCT ON (e.api_key_id,COALESCE(NULLIF(e.request_id,''),'error:'||e.id::text)) e.*
 FROM ops_error_logs e
 WHERE e.created_at>=NOW()-($2::int*INTERVAL '1 day')
 AND NOT e.resolved AND NOT e.is_count_tokens AND NOT e.is_business_limited
 AND COALESCE(e.status_code,0)>=400 AND e.error_owner<>'client'
 ORDER BY e.api_key_id,COALESCE(NULLIF(e.request_id,''),'error:'||e.id::text),e.id DESC
), error_keys AS (
 SELECT e.api_key_id,keys.request_id FROM terminal_errors e
 CROSS JOIN LATERAL unnest(ARRAY[e.request_id,'local:'||e.request_id,'client:'||e.request_id]) AS keys(request_id)
 WHERE NULLIF(e.request_id,'') IS NOT NULL
), facts AS (
 SELECT t.monitor_id,u.id AS fact_id,COALESCE(NULLIF(u.requested_model,''),u.model,'') AS model,
 CASE WHEN COALESCE(u.first_token_ms,0)>=10000 THEN 'degraded' ELSE 'operational' END AS status,
 u.duration_ms AS latency_ms,u.first_token_ms,CASE WHEN u.output_tokens>0 AND u.output_duration_ms>=100 AND COALESCE(u.request_type,0) IN(2,3) AND COALESCE(u.image_count,0)=0 AND COALESCE(u.image_output_tokens,0)=0 AND COALESCE(u.billing_mode,'token')='token' THEN u.output_tokens::float8*1000/u.output_duration_ms ELSE NULL::float8 END AS output_tokens_per_second,u.created_at AS checked_at,'user_request'::text AS source
 FROM targets t JOIN usage_logs u ON u.group_id=t.group_id
 WHERE u.created_at>=NOW()-($2::int*INTERVAL '1 day') AND COALESCE(u.request_type,0)<>4
 AND (u.input_tokens+u.output_tokens+u.cache_read_tokens+u.cache_creation_tokens>0 OR u.actual_cost>0 OR COALESCE(u.image_count,0)>0)
 AND NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=u.api_key_id)
 AND NOT EXISTS(SELECT 1 FROM error_keys e WHERE e.api_key_id=u.api_key_id AND e.request_id=u.request_id)
 UNION ALL
 SELECT t.monitor_id,e.id,COALESCE(NULLIF(e.requested_model,''),e.model,''),'error',e.duration_ms,e.time_to_first_token_ms,NULL::float8,e.created_at,'user_request'
 FROM targets t JOIN terminal_errors e ON e.group_id=t.group_id
 WHERE NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=e.api_key_id)
 UNION ALL
 SELECT t.monitor_id,h.id,h.model,h.status::text,h.latency_ms,h.first_token_ms,CASE WHEN h.status IN('operational','degraded') AND h.output_rate_version=1 THEN h.output_tokens_per_second ELSE NULL::float8 END,h.checked_at,'probe'
 FROM targets t JOIN channel_monitor_histories h ON h.monitor_id=t.monitor_id
 WHERE h.checked_at>=NOW()-($2::int*INTERVAL '1 day')
)`

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
	rows, err := r.db.QueryContext(ctx, monitorHealthFactsSQL+` SELECT monitor_id,model,COUNT(*),COUNT(*) FILTER(WHERE status IN ('operational','degraded')),AVG(latency_ms) FROM facts GROUP BY monitor_id,model`, pq.Array(ids), days)
	if err != nil {
		return nil, fmt.Errorf("monitor health availability: %w", err)
	}
	for rows.Next() {
		var id int64
		var avg sql.NullFloat64
		a := &service.ChannelMonitorAvailability{WindowDays: days}
		if err = rows.Scan(&id, &a.Model, &a.TotalChecks, &a.OperationalChecks, &avg); err != nil {
			rows.Close()
			return nil, err
		}
		finalizeAvailabilityRow(a, avg)
		h := out[id]
		h.Models[a.Model] = a
		h.Availability.TotalChecks += a.TotalChecks
		h.Availability.OperationalChecks += a.OperationalChecks
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, h := range out {
		finalizeAvailabilityRow(h.Availability, sql.NullFloat64{})
	}
	if limit == 0 {
		return out, nil
	}
	rows, err = r.db.QueryContext(ctx, monitorHealthFactsSQL+`, ranked AS (SELECT *,ROW_NUMBER() OVER(PARTITION BY monitor_id ORDER BY checked_at DESC,source,fact_id DESC) AS rn FROM facts) SELECT monitor_id,model,status,latency_ms,first_token_ms,output_tokens_per_second,checked_at,source FROM ranked WHERE rn<=$3 ORDER BY monitor_id,checked_at DESC,source,fact_id DESC`, pq.Array(ids), 30, limit)
	if err != nil {
		return nil, fmt.Errorf("monitor health recent: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var latency, first sql.NullInt64
		var rate sql.NullFloat64
		e := &service.ChannelMonitorHistoryEntry{}
		if err = rows.Scan(&id, &e.Model, &e.Status, &latency, &first, &rate, &e.CheckedAt, &e.Source); err != nil {
			return nil, err
		}
		assignNullInt(&e.LatencyMs, latency)
		assignNullInt(&e.FirstTokenMs, first)
		if rate.Valid {
			e.OutputTokensPerSecond = &rate.Float64
		}
		out[id].Recent = append(out[id].Recent, e)
	}
	return out, rows.Err()
}
