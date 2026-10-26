//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 仅使用 CTE 合成边界，不向真实用量、监控或错误表插入测试记录。
const monitorRecentBoundaryFixtures = `WITH fixture_monitors(id,group_id,provisioned_api_key_id) AS (
 VALUES(1::bigint,1::bigint,NULL::bigint),(2,2,NULL),(3,3,NULL),(4,4,NULL),(5,5,NULL),(6,6,NULL),(7,7,NULL),(99,99,999)
), channel_monitors AS (SELECT *, 'hybrid'::text AS probe_mode FROM fixture_monitors), fixture_errors(id,api_key_id,request_id,group_id,offset_seconds) AS (
 VALUES(100::bigint,10::bigint,'error:101'::text,1::bigint,100),(101,10,NULL,1,99),
 (210,NULL,'null-key',2,95),(211,20,NULL,2,94),(212,20,'',2,93),
 (310,30,'base',3,90),(311,30,'base',3,89),(312,30,'other',3,88),
 (410,999,'probe-only',4,85),(510,50,'resolved',5,80),(511,50,'client-owned',5,79),(701,70,'tie-error',7,70)
), ops_error_logs AS (
 SELECT id,api_key_id,request_id,group_id,'actual'::text requested_model,NULL::text model,1500::int duration_ms,NULL::int time_to_first_token_ms,
 NOW()-offset_seconds*INTERVAL '1 second' created_at,(id=510) resolved,FALSE is_count_tokens,FALSE is_business_limited,500 status_code,
 CASE WHEN id=511 THEN 'client' ELSE 'provider' END::text error_owner FROM fixture_errors
), fixture_usage(id,api_key_id,request_id,group_id,offset_seconds) AS (
 VALUES(200::bigint,10::bigint,'local:error:101'::text,1::bigint,98),
 (220,NULL,'null-key',2,92),(221,20,NULL,2,91),(222,20,'',2,90),
 (320,30,'base',3,87),(321,30,'local:base',3,86),(322,30,'client:base',3,85),(323,30,'kept',3,84),
 (420,999,'probe-only',4,83),(421,40,'real',4,82),(520,50,'resolved',5,78),(521,50,'client-owned',5,77),(701,70,'tie-usage',7,70)
 UNION ALL SELECT 6000+n,60,'boundary-'||n,6,200-n FROM generate_series(1,75) n
), usage_logs AS (
 SELECT id,api_key_id,request_id,group_id,'actual'::text requested_model,'fallback'::text model,100::int first_token_ms,500::int duration_ms,
 100::int output_tokens,200::int output_duration_ms,NULL::jsonb rate_observation,2::int request_type,0::int image_count,0::int image_output_tokens,'token'::text billing_mode,
 NOW()-offset_seconds*INTERVAL '1 second' created_at,10::int input_tokens,0::int cache_read_tokens,0::int cache_creation_tokens,1::numeric actual_cost FROM fixture_usage
), channel_monitor_histories AS (
 SELECT n::bigint id,6::bigint monitor_id,'probe-model'::text model,'failed'::text status,500::int latency_ms,NULL::int first_token_ms,
 1::smallint output_rate_version,NULL::float8 output_tokens_per_second,NOW()-(130-n)*INTERVAL '1 second' checked_at FROM generate_series(1,70) n
)`

// Certification is not required for the plain card, but every existing safety
// gate remains: malformed, incomplete, bursty, short and non-token samples stay unknown.
const monitorDisplayedRateFixtures = `WITH cases(name,quality,version,window_ms,duration_ms,events,span,bytes,burst,request_type,image_count,image_output_tokens,billing_mode,expected) AS (
 VALUES ('observed','observed',1,5000,5000,20,4500,1000,100,2,0,0,'token',200::float8),
 ('uncertain','timing_uncertain',1,5000,5000,20,4500,1000,100,2,0,0,'token',200),
 ('concentrated','concentrated',1,5000,5000,20,4500,1000,100,2,0,0,'token',NULL),
 ('single_batch','single_batch',1,5000,5000,20,4500,1000,100,2,0,0,'token',NULL),
 ('incomplete','incomplete',1,5000,5000,20,4500,1000,100,2,0,0,'token',NULL),
 ('unknown','unknown',1,5000,5000,20,4500,1000,100,2,0,0,'token',NULL),
 ('old_version','timing_uncertain',0,5000,5000,20,4500,1000,100,2,0,0,'token',NULL),
 ('window_mismatch','timing_uncertain',1,4000,5000,20,4500,1000,100,2,0,0,'token',NULL),
 ('short','timing_uncertain',1,900,900,20,500,1000,100,2,0,0,'token',NULL),
 ('few_events','timing_uncertain',1,5000,5000,2,4500,1000,100,2,0,0,'token',NULL),
 ('small_span','timing_uncertain',1,5000,5000,20,249,1000,100,2,0,0,'token',NULL),
 ('burst_boundary','timing_uncertain',1,5000,5000,20,4500,1000,800,2,0,0,'token',NULL),
 ('no_bytes','timing_uncertain',1,5000,5000,20,4500,0,0,2,0,0,'token',NULL),
 ('sync','timing_uncertain',1,5000,5000,20,4500,1000,100,1,0,0,'token',NULL),
 ('image','timing_uncertain',1,5000,5000,20,4500,1000,100,2,1,0,'token',NULL),
 ('image_tokens','timing_uncertain',1,5000,5000,20,4500,1000,100,2,0,1,'token',NULL),
 ('per_request','timing_uncertain',1,5000,5000,20,4500,1000,100,2,0,0,'request',NULL)
), sample AS (
 SELECT *,1000::int AS output_tokens,duration_ms AS output_duration_ms,
 jsonb_build_object('quality',quality,'version',version,'window_ms',window_ms,'output_events',events,'event_span_ms',span,'output_bytes',bytes,'max_burst_bytes',burst) AS rate_observation
 FROM cases
)`

func TestMonitorDisplayedReceiveRatePostgresEligibility(t *testing.T) {
	start := strings.Index(monitorRecentHealthSQL, "CASE WHEN u.output_tokens>0")
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(monitorRecentHealthSQL[start:], " AS output_tokens_per_second")
	require.Greater(t, end, 0)
	rate := monitorRecentHealthSQL[start : start+end]
	query := monitorDisplayedRateFixtures + ` SELECT COUNT(*) FROM (SELECT expected,` + rate + ` AS actual FROM sample u) checked WHERE actual IS DISTINCT FROM expected`
	var differences int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), query).Scan(&differences))
	require.Zero(t, differences, "only complete, non-bursty timing-uncertain observations gain plain display values")
}

func TestMonitorAvailabilityPostgresBoundaryEquivalence(t *testing.T) {
	bind := func(query string) string {
		query = strings.Replace(query, "WITH targets AS", monitorRecentBoundaryFixtures+", targets AS", 1)
		query = strings.ReplaceAll(query, "$1::bigint[]", "ARRAY[1,2,3,4,5,6,7]::bigint[]")
		return strings.ReplaceAll(query, "$2::int", "7")
	}
	old := bind(monitorHealthFactsSQL + ` SELECT monitor_id,model,COUNT(*) AS total,
 COUNT(*) FILTER(WHERE status IN ('operational','degraded')) AS operational,AVG(latency_ms) AS latency
 FROM facts GROUP BY monitor_id,model`)
	newQuery := bind(monitorHealthAvailabilitySQL)
	query := `WITH old_rows AS (` + old + `),new_rows AS (` + newQuery + `)
 SELECT (SELECT count(*) FROM (SELECT * FROM old_rows EXCEPT ALL SELECT * FROM new_rows) x)
 + (SELECT count(*) FROM (SELECT * FROM new_rows EXCEPT ALL SELECT * FROM old_rows) x)`
	var differences int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), query).Scan(&differences))
	require.Zero(t, differences, "NULL identities, failed-stream aliases, probes and weighted latency remain equivalent")
}

func TestMonitorRecentHealthPostgresBoundaryEquivalence(t *testing.T) {
	bind := func(query string) string {
		query = strings.Replace(query, "WITH targets AS", monitorRecentBoundaryFixtures+", targets AS", 1)
		query = strings.ReplaceAll(query, "$1::bigint[]", "ARRAY[1,2,3,4,5,6,7]::bigint[]")
		query = strings.ReplaceAll(query, "$2::int", "30")
		return strings.ReplaceAll(query, "$3", "60")
	}
	// 完整事实也使用确定性末级排序，保留原先有定义的顺序。
	full := bind(monitorHealthFactsSQL + `, ranked AS (
 SELECT *,ROW_NUMBER() OVER(PARTITION BY monitor_id ORDER BY checked_at DESC,source,fact_id DESC,fact_kind DESC) AS rn FROM facts
) SELECT monitor_id,model,status,latency_ms,first_token_ms,output_tokens_per_second,checked_at,source
 FROM ranked WHERE rn<=60 ORDER BY monitor_id,checked_at DESC,source,fact_id DESC,fact_kind DESC`)
	query := `WITH old_rows AS (` + full + `),new_rows AS (` + bind(monitorRecentHealthSQL) + `)
 SELECT json_build_object(
 'ordered_equal',((SELECT jsonb_agg(to_jsonb(o)) FROM old_rows o)=(SELECT jsonb_agg(to_jsonb(n)) FROM new_rows n)),
 'missing',(SELECT count(*) FROM (SELECT * FROM old_rows EXCEPT ALL SELECT * FROM new_rows) x),
 'extra',(SELECT count(*) FROM (SELECT * FROM new_rows EXCEPT ALL SELECT * FROM old_rows) x),
 'collision_success',(SELECT count(*) FROM new_rows WHERE monitor_id=1 AND status='operational'),
 'null_success',(SELECT count(*) FROM new_rows WHERE monitor_id=2 AND status='operational'),
 'alias_success',(SELECT count(*) FROM new_rows WHERE monitor_id=3 AND status='operational'),
 'probe_success',(SELECT count(*) FROM new_rows WHERE monitor_id=4 AND status='operational'),
 'resolved_errors',(SELECT count(*) FROM new_rows WHERE monitor_id=5 AND status='error'),
 'boundary',(SELECT count(*) FROM new_rows WHERE monitor_id=6),
 'tie_status',(SELECT status FROM new_rows WHERE monitor_id=7 LIMIT 1))`
	var raw []byte
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), query).Scan(&raw))
	var got struct {
		OrderedEqual     bool   `json:"ordered_equal"`
		Missing          int    `json:"missing"`
		Extra            int    `json:"extra"`
		CollisionSuccess int    `json:"collision_success"`
		NullSuccess      int    `json:"null_success"`
		AliasSuccess     int    `json:"alias_success"`
		ProbeSuccess     int    `json:"probe_success"`
		ResolvedErrors   int    `json:"resolved_errors"`
		Boundary         int    `json:"boundary"`
		TieStatus        string `json:"tie_status"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	require.True(t, got.OrderedEqual)
	require.Zero(t, got.Missing)
	require.Zero(t, got.Extra)
	require.Equal(t, 1, got.CollisionSuccess)
	require.Equal(t, 3, got.NullSuccess)
	require.Equal(t, 1, got.AliasSuccess)
	require.Equal(t, 1, got.ProbeSuccess)
	require.Zero(t, got.ResolvedErrors)
	require.Equal(t, 60, got.Boundary)
	require.Equal(t, "error", got.TieStatus)
}
