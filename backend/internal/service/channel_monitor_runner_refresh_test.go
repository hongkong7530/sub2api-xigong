//go:build unit

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorRunnerRefreshFindsRemoteCreateAndRemovesDisabled(t *testing.T) {
	svc := &stubMonitorSvc{runCalled: make(chan int64, 4)}
	runner := newRunnerForTest(svc)
	runner.Start()
	defer runner.Stop()
	require.Zero(t, runnerTaskCount(runner))
	svc.enabled = []*ChannelMonitor{{ID: 30, Name: "remote-create", Enabled: true, IntervalSeconds: 60}}
	runner.reconcileSchedules()
	select {
	case id := <-svc.runCalled:
		require.EqualValues(t, 30, id)
	case <-time.After(2 * time.Second):
		t.Fatal("remote-created monitor was not scheduled")
	}
	first := runnerTaskPtr(runner, 30)
	// MarkChecked updates the row timestamp without changing its schedule.
	svc.enabled[0].UpdatedAt = time.Now().UTC()
	runner.reconcileSchedules()
	require.Same(t, first, runnerTaskPtr(runner, 30), "unchanged database rows must not restart their timer or immediately probe again")
	svc.enabled = nil
	runner.reconcileSchedules()
	require.Zero(t, runnerTaskCount(runner), "disabled/deleted monitors must leave the schedule")
}

func TestMonitorRunnerRefreshReschedulesChangedConfiguration(t *testing.T) {
	updated := time.Now().UTC()
	svc := &stubMonitorSvc{enabled: []*ChannelMonitor{{ID: 1, Name: "before", Enabled: true, IntervalSeconds: 60, UpdatedAt: updated}}}
	runner := newRunnerForTest(svc)
	runner.Start()
	defer runner.Stop()
	first := runnerTaskPtr(runner, 1)
	require.NotNil(t, first)
	svc.enabled = []*ChannelMonitor{{ID: 1, Name: "after", Enabled: true, IntervalSeconds: 90, UpdatedAt: updated.Add(time.Second)}}
	runner.reconcileSchedules()
	second := runnerTaskPtr(runner, 1)
	require.NotSame(t, first, second)
	require.Equal(t, "after", second.name)
	require.Equal(t, 90*time.Second, second.interval)
}

func TestMonitorRunnerRefreshReadFailureKeepsExistingTasks(t *testing.T) {
	svc := &stubMonitorSvc{enabled: []*ChannelMonitor{{ID: 1, Enabled: true, IntervalSeconds: 60}}}
	runner := newRunnerForTest(svc)
	runner.Start()
	defer runner.Stop()
	first := runnerTaskPtr(runner, 1)
	svc.listErr = errors.New("database temporarily unavailable")
	runner.reconcileSchedules()
	require.Same(t, first, runnerTaskPtr(runner, 1))
}

func TestMonitorRunnerRefreshRecoversAfterStartupReadFailure(t *testing.T) {
	svc := &stubMonitorSvc{listErr: errors.New("startup read failed")}
	runner := newRunnerForTest(svc)
	runner.Start()
	defer runner.Stop()
	require.Zero(t, runnerTaskCount(runner))
	svc.listErr = nil
	svc.enabled = []*ChannelMonitor{{ID: 1, Enabled: true, IntervalSeconds: 60}}
	runner.reconcileSchedules()
	require.Equal(t, 1, runnerTaskCount(runner))
}

func TestMonitorRunnerRefreshKeepsDecryptFailureDisabled(t *testing.T) {
	svc := &stubMonitorSvc{enabled: []*ChannelMonitor{{ID: 1, Enabled: true, IntervalSeconds: 60}}}
	runner := newRunnerForTest(svc)
	runner.Start()
	defer runner.Stop()
	svc.enabled = []*ChannelMonitor{{ID: 1, Enabled: true, IntervalSeconds: 60, APIKeyDecryptFailed: true}}
	runner.reconcileSchedules()
	require.Zero(t, runnerTaskCount(runner))
}
