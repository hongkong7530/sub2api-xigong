//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type automationGroupRepoStub struct {
	GroupRepository
	groups []Group
	scans  int
}

func (r *automationGroupRepoStub) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	for i := range r.groups {
		if r.groups[i].ID == id {
			return &r.groups[i], nil
		}
	}
	return nil, errors.New("group not found")
}

func (r *automationGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	r.scans++
	return r.groups, nil
}

type automationMonitorRepoStub struct {
	ChannelMonitorRepository
	bound []int64
	auto  *ChannelMonitor
}

func (r *automationMonitorRepoStub) ListBoundGroupIDs(context.Context) ([]int64, error) {
	return r.bound, nil
}

func (r *automationMonitorRepoStub) GetAutoByGroupID(context.Context, int64) (*ChannelMonitor, error) {
	if r.auto == nil {
		return nil, ErrChannelMonitorNotFound
	}
	return r.auto, nil
}

type automationChannelRepoStub struct {
	ChannelRepository
	channel  *Channel
	replaced [][]ChannelModelPricing
}

func (r *automationChannelRepoStub) GetChannelIDByGroupID(context.Context, int64) (int64, error) {
	return r.channel.ID, nil
}

func (r *automationChannelRepoStub) GetByID(context.Context, int64) (*Channel, error) {
	return r.channel, nil
}

func (r *automationChannelRepoStub) ReplaceModelPricing(_ context.Context, _ int64, pricing []ChannelModelPricing) error {
	r.replaced = append(r.replaced, pricing)
	r.channel.ModelPricing = pricing
	return nil
}

func (r *automationChannelRepoStub) ListAll(context.Context) ([]Channel, error) {
	return nil, nil
}

func (r *automationChannelRepoStub) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return nil, nil
}

func automationTestGroup() *Group {
	return &Group{ID: 23, Name: "Ultrafast", Platform: PlatformOpenAI,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-6-astra", "gpt-6.1-sol"}}}
}

func TestGroupAutomationFallbackRespectsAllowlistAndDoesNotMutateCandidates(t *testing.T) {
	svc := NewGroupAutomationService(nil, nil, nil, nil)
	original := []string{"gpt-6.1-sol", "gpt-6-sol", " gpt-6-astra ", "GPT-6-ASTRA", ""}
	models, source := svc.resolveGroupModels(context.Background(), automationTestGroup(), func() ([]string, error) {
		return original, nil
	})
	require.Equal(t, "candidates", source)
	require.Equal(t, []string{"gpt-6-astra", "gpt-6.1-sol"}, models)
	require.Equal(t, " gpt-6-astra ", original[2])
	require.True(t, sameModelSet([]ChannelModelPricing{{Platform: PlatformOpenAI, Models: models}}, PlatformOpenAI, []string{"gpt-6.1-sol", "gpt-6-astra", "GPT-6-ASTRA"}))
}

func TestGroupAutomationGatewayEmptyIsAuthoritative(t *testing.T) {
	for _, body := range []string{`{"data":[]}`, `{"data":[{"id":"gpt-6-sol"}]}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/models", r.URL.Path)
				require.Equal(t, "Bearer test", r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			repo := &automationMonitorRepoStub{auto: &ChannelMonitor{APIKey: "OLD:test"}}
			monitor := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
			monitor.SetLocalGatewayBase(server.URL)
			svc := NewGroupAutomationService(nil, nil, monitor, nil)
			models, source := svc.resolveGroupModels(context.Background(), automationTestGroup(), func() ([]string, error) {
				t.Fatal("successful discovery must not fall back to candidates")
				return nil, nil
			})
			require.Empty(t, models)
			require.Equal(t, "gateway", source)
		})
	}
}

func TestGroupAutomationDiscoveryErrorFallsBackThroughAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	monitor := NewChannelMonitorService(&automationMonitorRepoStub{auto: &ChannelMonitor{APIKey: "OLD:test"}}, &duplicateChannelMonitorEncryptor{})
	monitor.SetLocalGatewayBase(server.URL)
	svc := NewGroupAutomationService(nil, nil, monitor, nil)
	models, source := svc.resolveGroupModels(context.Background(), automationTestGroup(), func() ([]string, error) {
		return []string{"gpt-6-sol", "gpt-6.1-sol"}, nil
	})
	require.Equal(t, "candidates", source)
	require.Equal(t, []string{"gpt-6.1-sol"}, models)
}

func TestGroupAutomationManualChannelSkipsDiscoveryAndWrites(t *testing.T) {
	g := automationTestGroup()
	channels := &automationChannelRepoStub{channel: &Channel{ID: 7, Description: "operator configured"}}
	svc := NewGroupAutomationService(nil, NewChannelService(channels, nil, nil, nil, nil), nil, nil)
	report := &GroupAutomationReport{}
	svc.ensurePlazaChannel(context.Background(), g, func() ([]string, error) {
		t.Fatal("manual channels must not discover models")
		return nil, nil
	}, report)
	require.Empty(t, channels.replaced)
	require.Len(t, report.Items, 1)
	require.Equal(t, "plaza_skipped", report.Items[0].Action)
}

func TestGroupAutomationReconcileManualChannelSkipsDiscovery(t *testing.T) {
	g := automationTestGroup()
	groups := &automationGroupRepoStub{groups: []Group{*g}}
	channels := &automationChannelRepoStub{channel: &Channel{ID: 7, Description: "manual"}}
	// No model services are installed: discovery would panic if not skipped.
	svc := NewGroupAutomationService(groups, NewChannelService(channels, nil, nil, nil, nil), nil, nil)
	require.NoError(t, svc.ReconcileAutoPlazaChannels(context.Background()))
	require.Empty(t, channels.replaced)
}

func TestGroupAutomationAutoChannelReconcilesIdempotently(t *testing.T) {
	g := automationTestGroup()
	channels := &automationChannelRepoStub{channel: &Channel{ID: 7, Description: autoPlazaChannelDescription,
		ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-6-sol", "gpt-6.1-sol"}}}}}
	svc := NewGroupAutomationService(nil, NewChannelService(channels, nil, nil, nil, nil), nil, nil)
	for i := 0; i < 2; i++ {
		report := &GroupAutomationReport{}
		svc.ensurePlazaChannel(context.Background(), g, func() ([]string, error) {
			return []string{"gpt-6-sol", "gpt-6-astra", "gpt-6.1-sol"}, nil
		}, report)
		if i == 0 {
			require.Equal(t, "plaza_channel_updated", report.Items[0].Action)
		} else {
			require.Equal(t, "plaza_skipped", report.Items[0].Action)
		}
	}
	require.Len(t, channels.replaced, 1)
	require.Equal(t, []string{"gpt-6-astra", "gpt-6.1-sol"}, channels.replaced[0][0].Models)
}

func TestGroupAutomationRejectsConcurrentInvocationBeforeScanning(t *testing.T) {
	groups := &automationGroupRepoStub{groups: []Group{*automationTestGroup()}}
	monitors := NewChannelMonitorService(&automationMonitorRepoStub{bound: []int64{23}}, &duplicateChannelMonitorEncryptor{})
	svc := NewGroupAutomationService(groups, nil, monitors, nil)
	svc.provisionMu.Lock()
	_, err := svc.AutoProvisionPublicGroups(context.Background(), 1, GroupAutomationSelection{GroupID: 23, ProbeMode: MonitorProbeModeHybrid, IntervalSeconds: 60})
	svc.provisionMu.Unlock()
	require.Error(t, err)
	require.Zero(t, groups.scans)
	locks := &fakeLeaderLockCache{}
	_, err = locks.TryAcquireLeaderLock(context.Background(), autoPlazaReconcileLockKey, "peer", time.Minute)
	require.NoError(t, err)
	svc.SetLeaderLock(locks, nil)
	_, err = svc.AutoProvisionPublicGroups(context.Background(), 1, GroupAutomationSelection{GroupID: 23, ProbeMode: MonitorProbeModeHybrid, IntervalSeconds: 60})
	require.Error(t, err)
	require.Zero(t, groups.scans)
	require.NoError(t, locks.ReleaseLeaderLock(context.Background(), autoPlazaReconcileLockKey, "peer"))
	_, err = svc.AutoProvisionPublicGroups(context.Background(), 1, GroupAutomationSelection{GroupID: 23, ProbeMode: MonitorProbeModeHybrid, IntervalSeconds: 60})
	require.NoError(t, err)
	require.Equal(t, 1, groups.scans)
}

func TestGroupAutomationCancellationStopsBeforeMutatingGroups(t *testing.T) {
	groups := &automationGroupRepoStub{groups: []Group{*automationTestGroup()}}
	monitors := NewChannelMonitorService(&automationMonitorRepoStub{}, &duplicateChannelMonitorEncryptor{})
	svc := NewGroupAutomationService(groups, nil, monitors, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.AutoProvisionPublicGroups(ctx, 1, GroupAutomationSelection{GroupID: 23, ProbeMode: MonitorProbeModeHybrid, IntervalSeconds: 60})
	require.ErrorIs(t, err, context.Canceled)
}

func TestGroupAutomationModelDiscoveryFailureRemainsEmpty(t *testing.T) {
	svc := NewGroupAutomationService(nil, nil, nil, nil)
	models, source := svc.resolveGroupModels(context.Background(), automationTestGroup(), func() ([]string, error) {
		return nil, errors.New("catalog unavailable")
	})
	require.Empty(t, models)
	require.Equal(t, "unavailable", source)
}
