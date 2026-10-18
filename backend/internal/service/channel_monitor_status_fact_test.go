//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdminMonitorHealthKeepsObservedModelSourceAndTime(t *testing.T) {
	at := time.Now().UTC()
	latency, first := 1200, 700
	summary := MonitorStatusSummary{PrimaryStatus: MonitorStatusOperational}
	health := &ChannelMonitorHealth{
		Availability: &ChannelMonitorAvailability{TotalChecks: 5, AvailabilityPct: 80},
		Recent:       []*ChannelMonitorHistoryEntry{{Model: "different-model", Status: MonitorStatusError, LatencyMs: &latency, FirstTokenMs: &first, CheckedAt: at, Source: "user_request"}},
	}
	applyAdminMonitorHealth(&summary, health)
	require.Equal(t, "different-model", summary.StatusModel)
	require.Equal(t, "user_request", summary.StatusSource)
	require.Equal(t, at, *summary.StatusObservedAt)
	require.Equal(t, MonitorStatusError, summary.PrimaryStatus)
	require.Equal(t, MonitorStatusError, summary.OverallStatus)
	require.Equal(t, &latency, summary.PrimaryLatencyMs)
	require.Equal(t, &first, summary.PrimaryFirstTokenMs)
	require.EqualValues(t, 5, summary.AvailabilitySamples7d)
}

func TestAdminMonitorHealthNoFactClearsStaleState(t *testing.T) {
	at := time.Now().UTC()
	latency := 100
	for _, health := range []*ChannelMonitorHealth{nil, {Availability: &ChannelMonitorAvailability{}}} {
		summary := MonitorStatusSummary{PrimaryStatus: MonitorStatusError, OverallStatus: MonitorStatusError, PrimaryLatencyMs: &latency, PrimaryFirstTokenMs: &latency, StatusModel: "old-model", StatusSource: "probe", StatusObservedAt: &at, Availability7d: 100, AvailabilitySamples7d: 3}
		applyAdminMonitorHealth(&summary, health)
		require.Empty(t, summary.PrimaryStatus)
		require.Empty(t, summary.OverallStatus)
		require.Empty(t, summary.StatusModel)
		require.Empty(t, summary.StatusSource)
		require.Nil(t, summary.PrimaryLatencyMs)
		require.Nil(t, summary.PrimaryFirstTokenMs)
		require.Nil(t, summary.StatusObservedAt)
		require.Zero(t, summary.AvailabilitySamples7d)
	}
}

func TestMonitorProbeSummaryPreservesFactIdentity(t *testing.T) {
	at := time.Now().UTC()
	summary := buildStatusSummary(map[string]*ChannelMonitorLatest{"probe-model": {Model: "probe-model", Status: MonitorStatusOperational, CheckedAt: at}}, nil, "probe-model", nil)
	require.Equal(t, "probe-model", summary.StatusModel)
	require.Equal(t, "probe", summary.StatusSource)
	require.Equal(t, at, *summary.StatusObservedAt)
}
