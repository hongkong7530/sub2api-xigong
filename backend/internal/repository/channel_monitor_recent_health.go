package repository

// 每个来源先取 N 条再合并取 N，与全量事实的同序 TopN 等价。
// 可用率仍使用完整时间窗；这里只限制卡片时间线，不改变失败去重和状态来源。
// 跨表同时间、同 ID 的旧顺序未定义；末级 fact_kind 让错误优先，避免首项抖动。
// 普通卡片可显示时序未认证的接收速率，但不授予奖励、排行或高速特效资格。
// 其他窗口、分布和请求类型门槛与完整事实查询一致。
const monitorRecentHealthSQL = monitorHealthSourcesSQL + `, recent_facts AS (
 SELECT t.monitor_id,recent.* FROM targets t CROSS JOIN LATERAL (
  SELECT u.id AS fact_id,COALESCE(NULLIF(u.requested_model,''),u.model,'') AS model,
   CASE WHEN COALESCE(u.first_token_ms,0)>=10000 THEN 'degraded' ELSE 'operational' END AS status,
   u.duration_ms AS latency_ms,u.first_token_ms,
   CASE WHEN u.output_tokens>0 AND u.output_duration_ms>=1000 AND u.rate_observation->>'quality' IN ('observed','timing_uncertain') AND (u.rate_observation->>'version')::int=1 AND (u.rate_observation->>'window_ms')::int=u.output_duration_ms AND (u.rate_observation->>'output_events')::int>=3 AND (u.rate_observation->>'event_span_ms')::int>=250 AND (u.rate_observation->>'output_bytes')::bigint>0 AND (u.rate_observation->>'max_burst_bytes')::bigint>=0 AND (u.rate_observation->>'max_burst_bytes')::bigint*5<(u.rate_observation->>'output_bytes')::bigint*4 AND COALESCE(u.request_type,0) IN(2,3) AND COALESCE(u.image_count,0)=0 AND COALESCE(u.image_output_tokens,0)=0 AND COALESCE(u.billing_mode,'token')='token' THEN u.output_tokens::float8*1000/u.output_duration_ms ELSE NULL::float8 END AS output_tokens_per_second,
   u.created_at AS checked_at,'user_request'::text AS source,1 AS fact_kind
  FROM usage_logs u
  WHERE u.group_id=t.group_id AND u.created_at>=NOW()-($2::int*INTERVAL '1 day') AND COALESCE(u.request_type,0)<>4
   AND (u.input_tokens+u.output_tokens+u.cache_read_tokens+u.cache_creation_tokens>0 OR u.actual_cost>0 OR COALESCE(u.image_count,0)>0)
   AND NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=u.api_key_id)
   AND (u.api_key_id IS NULL OR u.request_id IS NULL OR
    (u.api_key_id,u.request_id) NOT IN (
     SELECT e.api_key_id,e.request_id FROM error_keys e WHERE e.api_key_id IS NOT NULL
    ))
  ORDER BY u.created_at DESC,u.id DESC LIMIT $3
 ) recent
 UNION ALL
 SELECT t.monitor_id,recent.* FROM targets t CROSS JOIN LATERAL (
  SELECT e.id AS fact_id,COALESCE(NULLIF(e.requested_model,''),e.model,'') AS model,'error'::text AS status,
   e.duration_ms AS latency_ms,e.time_to_first_token_ms AS first_token_ms,NULL::float8 AS output_tokens_per_second,
   e.created_at AS checked_at,'user_request'::text AS source,2 AS fact_kind
  FROM terminal_errors e
  WHERE e.group_id=t.group_id
   AND NOT EXISTS(SELECT 1 FROM channel_monitors p WHERE p.provisioned_api_key_id=e.api_key_id)
  ORDER BY e.created_at DESC,e.id DESC LIMIT $3
 ) recent
 UNION ALL
 SELECT t.monitor_id,recent.* FROM targets t CROSS JOIN LATERAL (
  SELECT h.id AS fact_id,h.model,h.status::text AS status,h.latency_ms,h.first_token_ms,
   CASE WHEN h.status IN('operational','degraded') AND h.output_rate_version=2 THEN h.output_tokens_per_second ELSE NULL::float8 END AS output_tokens_per_second,
   h.checked_at,'probe'::text AS source,0 AS fact_kind
  FROM channel_monitor_histories h
  WHERE t.probe_mode<>'passive' AND h.monitor_id=t.monitor_id AND h.checked_at>=NOW()-($2::int*INTERVAL '1 day')
  ORDER BY h.checked_at DESC,h.id DESC LIMIT $3
 ) recent
), ranked AS (
 SELECT *,ROW_NUMBER() OVER(PARTITION BY monitor_id ORDER BY checked_at DESC,source,fact_id DESC,fact_kind DESC) AS rn FROM recent_facts
)
SELECT monitor_id,model,status,latency_ms,first_token_ms,output_tokens_per_second,checked_at,source
FROM ranked WHERE rn<=$3 ORDER BY monitor_id,checked_at DESC,source,fact_id DESC,fact_kind DESC`
