package repository

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func rateObservationJSON(value *service.RateObservation) any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return string(raw)
}

// 统计仅使用分布充分的到达观测；旧记录和异常/短窗口不得进入速度排行。
// 条件与 ratemetric.IsEligible 使用同一版本门槛，JSON数字来源仅由本站写入。
const eligibleRateObservationSQL = ` AND (ul.rate_observation->>'version')::int=1
 AND ul.rate_observation->>'quality'='observed'
 AND (ul.rate_observation->>'window_ms')::int>=1000
 AND (ul.rate_observation->>'window_ms')::int=ul.output_duration_ms
 AND (ul.rate_observation->>'output_events')::int>=3
 AND (ul.rate_observation->>'event_span_ms')::int>=250
 AND (ul.rate_observation->>'output_bytes')::bigint>0
 AND (ul.rate_observation->>'max_burst_bytes')::bigint>=0
 AND (ul.rate_observation->>'max_burst_bytes')::bigint*5<(ul.rate_observation->>'output_bytes')::bigint*4`
