//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type hybridMonitorRepoStub struct {
	ChannelMonitorRepository
	monitor *ChannelMonitor
	observation *ChannelMonitorObservation
	observationErr error
	history []*ChannelMonitorHistoryEntry
}

func (r *hybridMonitorRepoStub) GetByID(context.Context, int64) (*ChannelMonitor, error) { return r.monitor, nil }
func (r *hybridMonitorRepoStub) ListHistory(context.Context, int64, string, int) ([]*ChannelMonitorHistoryEntry, error) { return r.history, nil }
func (r *hybridMonitorRepoStub) ListLatestGroupObservations(_ context.Context, ids []int64, since time.Time) (map[int64]*ChannelMonitorObservation, error) {
	out := map[int64]*ChannelMonitorObservation{}
	if r.observationErr != nil { return nil, r.observationErr }
	if r.observation != nil && !r.observation.ObservedAt.Before(since) { out[ids[0]] = r.observation }
	return out, nil
}

func TestHybridMonitorGateUsesSlidingRealTrafficWindow(t *testing.T) {
	groupID := int64(9)
	for _, tc := range []struct { name string; requestAge, probeAge time.Duration; failed, wantDue bool }{
		{"recent success", 30*time.Second, 2*time.Minute, false, false},
		{"recent failure is still real traffic", 30*time.Second, 2*time.Minute, true, false},
		{"idle needs one probe", 61*time.Second, 2*time.Minute, false, true},
		{"recent probe suppresses repeats", 61*time.Second, 20*time.Second, false, false},
		{"no history idle", 61*time.Second, 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			repo := &hybridMonitorRepoStub{monitor: &ChannelMonitor{ID: 1, Enabled: true, GroupID: &groupID, KeyProvisionMode: "auto", IntervalSeconds: 60}, observation: &ChannelMonitorObservation{ObservedAt: now.Add(-tc.requestAge), Failed: tc.failed}}
			if tc.probeAge > 0 { repo.history = []*ChannelMonitorHistoryEntry{{CheckedAt: now.Add(-tc.probeAge)}} }
			svc := NewChannelMonitorService(repo, nil)
			due, err := svc.ScheduledCheckDue(context.Background(), 1)
			require.NoError(t, err)
			require.Equal(t, tc.wantDue, due)
		})
	}
}

func TestHybridManualCheckNeverReturnsHistoricalObservation(t *testing.T) {
	groupID := int64(9)
	firstToken := 1320
	repo := &hybridMonitorRepoStub{
		monitor: &ChannelMonitor{ID: 1, Enabled: true, GroupID: &groupID, KeyProvisionMode: "auto", Provider: MonitorProviderOpenAI, APIKeyDecryptFailed: true},
		observation: &ChannelMonitorObservation{Model: "real-model", FirstTokenMs: &firstToken, ObservedAt: time.Now()},
	}
	svc := NewChannelMonitorService(repo, nil)
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	results, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorAPIKeyDecryptFailed)
	require.Nil(t, results, "manual action must fail honestly rather than return a previous success")
}

func TestHybridMonitorGateFailsClosedWhenObservationsUnavailable(t *testing.T) {
	groupID := int64(9)
	repo := &hybridMonitorRepoStub{monitor: &ChannelMonitor{ID: 1, Enabled: true, GroupID: &groupID, KeyProvisionMode: "auto", IntervalSeconds: 60}, observationErr: errors.New("database unavailable")}
	svc := NewChannelMonitorService(repo, nil)
	due, err := svc.ScheduledCheckDue(context.Background(), 1)
	require.Error(t, err)
	require.False(t, due)
}

func TestHybridObservationKeepsLatestValueAndFailure(t *testing.T) {
	now := time.Now()
	first := 12000
	view := &UserMonitorView{PrimaryStatus: MonitorStatusError, Timeline: []UserMonitorTimelinePoint{{CheckedAt: now.Add(-time.Minute)}}}
	applyRealObservation(view, &ChannelMonitorObservation{Model: "real-model", FirstTokenMs: &first, ObservedAt: now})
	require.Equal(t, "real-model", view.PrimaryModel)
	require.Equal(t, MonitorStatusDegraded, view.PrimaryStatus)
	require.Equal(t, "user_request", view.StatusSource)
	require.Equal(t, &first, view.PrimaryFirstTokenMs)
	applyRealObservation(view, &ChannelMonitorObservation{Model: "real-model", ObservedAt: now.Add(time.Second), Failed: true})
	require.Equal(t, MonitorStatusError, view.PrimaryStatus)
	require.Nil(t, view.PrimaryFirstTokenMs)
}

type gatedMonitorRunnerStub struct { stubMonitorSvc; due bool; err error }
func (s *gatedMonitorRunnerStub) ScheduledCheckDue(context.Context, int64) (bool, error) { return s.due, s.err }

func TestHybridRunnerConsumesScheduleGate(t *testing.T) {
	for _, tc := range []struct { name string; due bool; err error; want int64 }{
		{"passive", false, nil, 0}, {"idle", true, nil, 1}, {"gate failed", false, errors.New("database unavailable"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &gatedMonitorRunnerStub{due: tc.due, err: tc.err}
			runner := newRunnerForTest(svc)
			runner.runOne(1, "hybrid")
			require.Equal(t, tc.want, svc.runCount.Load())
			runner.Stop()
		})
	}
}
