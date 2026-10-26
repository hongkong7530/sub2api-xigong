package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRateObservationUserAndAdminDTOStayNullable(t *testing.T) {
	ms := 2500
	observation := &service.RateObservation{Version: 1, RequestDurationMs: &ms, TimingScope: "app_request", WindowMs: &ms, Quality: "timing_uncertain", OutputEvents: 4, EventSpanMs: 1500, OutputBytes: 400, MaxBurstBytes: 100}
	for _, value := range []*service.RateObservation{observation, nil} {
		log := &service.UsageLog{Model: "mock", OutputTokens: 1000, RateObservation: value}
		user := UsageLogFromService(log)
		require.Equal(t, value, user.RateObservation)
		for _, dto := range []any{user, UsageLogFromServiceAdmin(log)} {
			raw, err := json.Marshal(dto)
			require.NoError(t, err)
			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &decoded))
			require.Contains(t, decoded, "rate_observation")
			if value == nil {
				require.JSONEq(t, "null", string(decoded["rate_observation"]))
			} else {
				var restored service.RateObservation
				require.NoError(t, json.Unmarshal(decoded["rate_observation"], &restored))
				require.Equal(t, *value, restored)
			}
		}
	}
}
