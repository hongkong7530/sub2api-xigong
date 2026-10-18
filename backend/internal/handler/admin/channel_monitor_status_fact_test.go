//go:build unit

package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorListIncludesBindingAndObservedStatus(t *testing.T) {
	gid := int64(23)
	first, duration := 800, 1600
	at := time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)
	monitor := &service.ChannelMonitor{ID: 26, Name: "test", GroupID: &gid, GroupName: "group", KeyProvisionMode: "auto", Description: "description", DescriptionColor: "blue", PrimaryModel: "configured-model", APIKey: "test-secret"}
	summary := service.MonitorStatusSummary{PrimaryStatus: service.MonitorStatusError, OverallStatus: service.MonitorStatusError, PrimaryLatencyMs: &duration, PrimaryFirstTokenMs: &first, StatusModel: "observed-model", StatusSource: "user_request", StatusObservedAt: &at}
	row := buildListItemResponse(monitor, summary)
	encoded, err := json.Marshal(row)
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(encoded, &data))
	require.EqualValues(t, gid, data["group_id"])
	require.Equal(t, "auto", data["key_provision_mode"])
	require.Equal(t, "description", data["description"])
	require.Equal(t, "blue", data["description_color"])
	require.Equal(t, "configured-model", data["primary_model"])
	require.Equal(t, "observed-model", data["status_model"])
	require.Equal(t, "user_request", data["status_source"])
	require.Equal(t, at.Format(time.RFC3339), data["status_observed_at"])
	require.EqualValues(t, first, data["primary_first_token_ms"])
	require.Equal(t, service.MonitorStatusError, data["overall_status"])
	require.Nil(t, data["availability_7d"])
	require.NotContains(t, string(encoded), "test-secret")
}

func TestChannelMonitorNoObservationStaysUnknown(t *testing.T) {
	row := buildListItemResponse(&service.ChannelMonitor{ID: 1}, service.MonitorStatusSummary{})
	require.Empty(t, row.PrimaryStatus)
	require.Empty(t, row.StatusSource)
	require.Empty(t, row.StatusModel)
	require.Nil(t, row.StatusObservedAt)
	require.Nil(t, row.PrimaryFirstTokenMs)
}
