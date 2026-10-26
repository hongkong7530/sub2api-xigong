//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type selectedMonitorRepo struct {
	autoKeyRepoStub
	bound []int64
	auto  *ChannelMonitor
}

func (r *selectedMonitorRepo) ListBoundGroupIDs(context.Context) ([]int64, error) {
	return r.bound, nil
}
func (r *selectedMonitorRepo) GetAutoByGroupID(context.Context, int64) (*ChannelMonitor, error) {
	if r.auto == nil {
		return nil, ErrChannelMonitorNotFound
	}
	m := *r.auto
	return &m, nil
}

func TestMonitorOptionsOnlySelectedGroupCreatesPassiveWithoutKey(t *testing.T) {
	groups := &automationGroupRepoStub{groups: []Group{{ID: 9, Name: "Chosen", Platform: PlatformOpenAI}, {ID: 10, Name: "Not chosen", Platform: PlatformOpenAI}}}
	repo := &selectedMonitorRepo{}
	monitor := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	provisioner := &autoKeyProvisionerStub{}
	monitor.SetKeyProvisioner(provisioner)
	svc := NewGroupAutomationService(groups, nil, monitor, nil)
	report, err := svc.AutoProvisionPublicGroups(context.Background(), 99, GroupAutomationSelection{GroupID: 9, ProbeMode: MonitorProbeModePassive, IntervalSeconds: 300})
	require.NoError(t, err)
	require.Equal(t, 1, report.GroupsScanned)
	require.Equal(t, 1, report.MonitorsCreated)
	require.Len(t, repo.created, 1)
	require.EqualValues(t, 9, *repo.created[0].GroupID)
	require.Equal(t, MonitorProbeModePassive, repo.created[0].ProbeMode)
	require.Equal(t, 300, repo.created[0].IntervalSeconds)
	require.Empty(t, provisioner.provisioned, "passive creation must not provision a probe key")
}

func TestMonitorOptionsRejectInvalidSelectionsBeforeMutation(t *testing.T) {
	for _, choices := range [][]GroupAutomationSelection{
		nil, {{GroupID: 404, IntervalSeconds: 60}}, {{GroupID: 9, ProbeMode: "invalid", IntervalSeconds: 60}}, {{GroupID: 9, IntervalSeconds: 14}}, {{GroupID: 9, IntervalSeconds: 3601}}, {{GroupID: 9, IntervalSeconds: 60}, {GroupID: 9, IntervalSeconds: 60}},
	} {
		repo := &selectedMonitorRepo{}
		monitor := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
		svc := NewGroupAutomationService(&automationGroupRepoStub{groups: []Group{{ID: 9, Platform: PlatformOpenAI}}}, nil, monitor, nil)
		_, err := svc.AutoProvisionPublicGroups(context.Background(), 1, choices...)
		require.Error(t, err)
		require.Empty(t, repo.created)
	}
}

func TestMonitorOptionsExistingAutoChangesModeWithoutRotatingKey(t *testing.T) {
	groupID, keyID := int64(9), int64(100)
	m := &ChannelMonitor{ID: 7, Name: "Auto", Provider: MonitorProviderOpenAI, GroupID: &groupID, KeyProvisionMode: "auto", ProvisionedAPIKeyID: &keyID, APIKey: "OLD:key", Description: autoGroupMonitorDescription, PrimaryModel: "gpt-6.1-sol", Enabled: true, IntervalSeconds: 60}
	repo := &selectedMonitorRepo{autoKeyRepoStub: autoKeyRepoStub{getByID: m}, bound: []int64{9}, auto: m}
	monitor := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	provisioner := &autoKeyProvisionerStub{}
	monitor.SetKeyProvisioner(provisioner)
	svc := NewGroupAutomationService(nil, nil, monitor, nil)
	report := &GroupAutomationReport{}
	svc.ensureGroupMonitor(context.Background(), &Group{ID: 9, Name: "Auto"}, map[int64]struct{}{9: {}}, func() ([]string, error) { t.Fatal("existing monitor must not rediscover models"); return nil, nil }, 1, report, GroupAutomationSelection{GroupID: 9, ProbeMode: MonitorProbeModePassive, IntervalSeconds: 300})
	require.Len(t, repo.updated, 1)
	require.Equal(t, 300, repo.updated[0].IntervalSeconds)
	require.Equal(t, MonitorProbeModePassive, repo.updated[0].ProbeMode)
	require.Equal(t, "monitor_updated", report.Items[0].Action)
	require.Empty(t, provisioner.provisioned)
	require.Empty(t, provisioner.released)
}

func TestMonitorOptionsPassiveScheduleAndManualCheckNeverProbe(t *testing.T) {
	groupID := int64(9)
	m := &ChannelMonitor{ID: 1, Enabled: true, ProbeMode: MonitorProbeModePassive, GroupID: &groupID, KeyProvisionMode: "auto", Provider: MonitorProviderOpenAI, IntervalSeconds: 300}
	svc := NewChannelMonitorService(&hybridMonitorRepoStub{monitor: m}, &duplicateChannelMonitorEncryptor{})
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	due, err := svc.ScheduledCheckDue(context.Background(), 1)
	require.NoError(t, err)
	require.False(t, due)
	results, err := svc.RunCheck(context.Background(), 1)
	require.ErrorIs(t, err, ErrChannelMonitorPassive)
	require.Nil(t, results)
	runner := newRunnerForTest(&stubMonitorSvc{})
	runner.Start()
	defer runner.Stop()
	runner.Schedule(m)
	require.Zero(t, runnerTaskCount(runner))
	runner.mu.Lock()
	runner.tasks[1] = &scheduledMonitor{id: 1, name: m.Name, interval: 5 * time.Second, cancel: func() {}}
	runner.mu.Unlock()
	runner.svc = &stubMonitorSvc{enabled: []*ChannelMonitor{m}}
	runner.reconcileSchedules()
	require.Zero(t, runnerTaskCount(runner))
}

func TestMonitorOptionsHybridUsesConfiguredCooldown(t *testing.T) {
	groupID := int64(9)
	repo := &hybridMonitorRepoStub{monitor: &ChannelMonitor{ID: 1, Enabled: true, GroupID: &groupID, KeyProvisionMode: "auto", IntervalSeconds: 300}, history: []*ChannelMonitorHistoryEntry{{CheckedAt: time.Now().Add(-120 * time.Second)}}}
	svc := NewChannelMonitorService(repo, nil)
	due, err := svc.ScheduledCheckDue(context.Background(), 1)
	require.NoError(t, err)
	require.False(t, due)
	repo.history[0].CheckedAt = time.Now().Add(-301 * time.Second)
	due, err = svc.ScheduledCheckDue(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, due)
	svc.applyAutomaticProbePolicy(context.Background(), repo.monitor)
	require.Equal(t, 300, repo.monitor.IntervalSeconds)
	require.Error(t, validateMonitorProbeMode(MonitorProbeModePassive, nil, MonitorCheckModeProbe))
	require.Error(t, validateMonitorProbeMode(MonitorProbeModePassive, &groupID, MonitorCheckModeQuota))
}

type passiveHealthRepo struct{ hybridMonitorRepoStub }

func (r *passiveHealthRepo) ReadMonitorHealth(context.Context, []int64, int, int) (map[int64]*ChannelMonitorHealth, error) {
	return map[int64]*ChannelMonitorHealth{1: {Availability: &ChannelMonitorAvailability{}, Models: map[string]*ChannelMonitorAvailability{}}}, nil
}
func TestMonitorOptionsPassiveDetailDoesNotRetainOldProbeStatus(t *testing.T) {
	repo := &passiveHealthRepo{}
	svc := NewChannelMonitorService(repo, nil)
	detail := &UserMonitorDetail{Models: []ModelDetail{{Model: "old-probe", LatestStatus: MonitorStatusOperational}}}
	got, err := svc.hybridUserDetail(context.Background(), &ChannelMonitor{ID: 1, ProbeMode: MonitorProbeModePassive}, detail)
	require.NoError(t, err)
	require.Empty(t, got.Models, "passive mode must not carry a stale successful probe into an empty real-request window")
}
