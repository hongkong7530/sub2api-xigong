//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type automationCleanupRepo struct {
	autoKeyRepoStub
	deleteContextErr error
	deleteErr        error
	updateErr        error
	autoMonitor      *ChannelMonitor
}

func (r *automationCleanupRepo) Delete(ctx context.Context, id int64) error {
	r.deleteContextErr = ctx.Err()
	if r.deleteErr != nil {
		return r.deleteErr
	}
	return r.autoKeyRepoStub.Delete(ctx, id)
}

func (r *automationCleanupRepo) Update(ctx context.Context, m *ChannelMonitor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.autoKeyRepoStub.Update(ctx, m)
}

func (r *automationCleanupRepo) GetAutoByGroupID(context.Context, int64) (*ChannelMonitor, error) {
	if r.autoMonitor == nil {
		return nil, ErrChannelMonitorNotFound
	}
	copy := *r.autoMonitor
	return &copy, nil
}

type automationCancelProvisioner struct {
	autoKeyProvisionerStub
	cancel            context.CancelFunc
	provisionErr      error
	releaseContextErr error
}

func (p *automationCancelProvisioner) ProvisionChannelMonitorKey(ctx context.Context, userID, groupID int64, name string) (*ChannelMonitorProvisionedKey, error) {
	key, err := p.autoKeyProvisionerStub.ProvisionChannelMonitorKey(ctx, userID, groupID, name)
	p.cancel()
	if p.provisionErr != nil {
		return nil, p.provisionErr
	}
	return key, err
}

func (p *automationCancelProvisioner) ReleaseChannelMonitorKey(ctx context.Context, id int64) error {
	p.releaseContextErr = ctx.Err()
	return p.autoKeyProvisionerStub.ReleaseChannelMonitorKey(ctx, id)
}

func TestGroupAutomationCancelledCreateRollsBackWithLiveCleanupContext(t *testing.T) {
	for _, afterProvision := range []bool{false, true} {
		t.Run(map[bool]string{false: "provision", true: "persist"}[afterProvision], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &automationCleanupRepo{}
			provisioner := &automationCancelProvisioner{cancel: cancel}
			if !afterProvision {
				provisioner.provisionErr = context.Canceled
			}
			svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
			svc.SetKeyProvisioner(provisioner)
			_, err := svc.Create(ctx, autoKeyCreateParams(23))
			require.ErrorIs(t, err, context.Canceled)
			require.Len(t, repo.created, 1)
			require.Equal(t, []int64{repo.created[0].ID}, repo.deleted)
			require.NoError(t, repo.deleteContextErr)
			if afterProvision {
				require.Len(t, provisioner.released, 1)
				require.NoError(t, provisioner.releaseContextErr)
			}
		})
	}
}

func TestGroupAutomationRollbackFailureIsNotSwallowed(t *testing.T) {
	cleanupErr := errors.New("delete failed")
	repo := &automationCleanupRepo{deleteErr: cleanupErr}
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	svc.SetKeyProvisioner(&autoKeyProvisionerStub{provisionErr: context.Canceled})
	_, err := svc.Create(context.Background(), autoKeyCreateParams(23))
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, cleanupErr)
}

func TestGroupAutomationRepairsOnlyIncompleteOwnedMonitor(t *testing.T) {
	for _, test := range []struct {
		name     string
		monitor  ChannelMonitor
		repaired bool
	}{
		{"incomplete", ChannelMonitor{ID: 1, GroupID: int64Ptr(23), KeyProvisionMode: "auto", Description: autoGroupMonitorDescription, APIKey: "OLD:"}, true},
		{"existing-key", ChannelMonitor{ID: 1, GroupID: int64Ptr(23), KeyProvisionMode: "auto", Description: autoGroupMonitorDescription, APIKey: "OLD:operator-key"}, false},
		{"manual-description", ChannelMonitor{ID: 1, GroupID: int64Ptr(23), KeyProvisionMode: "auto", Description: "operator configuration", APIKey: "OLD:"}, false},
		{"completed", ChannelMonitor{ID: 1, GroupID: int64Ptr(23), KeyProvisionMode: "auto", Description: autoGroupMonitorDescription, ProvisionedAPIKeyID: int64Ptr(99)}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &automationCleanupRepo{autoMonitor: &test.monitor}
			svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
			provisioner := &autoKeyProvisionerStub{}
			svc.SetKeyProvisioner(provisioner)
			repaired, err := svc.repairIncompleteAutoMonitor(context.Background(), 23)
			require.NoError(t, err)
			require.Equal(t, test.repaired, repaired)
			if test.repaired {
				require.Equal(t, []int64{23}, provisioner.provisioned)
				require.Len(t, repo.updated, 1)
			} else {
				require.Empty(t, provisioner.provisioned)
				require.Empty(t, repo.updated)
			}
		})
	}
}

func TestGroupAutomationProtectsAllManualPricingOnMarkedChannels(t *testing.T) {
	price := 0.0
	for _, test := range []struct {
		name    string
		pricing ChannelModelPricing
	}{
		{"zero-price", ChannelModelPricing{InputPrice: &price}},
		{"cache-1h", ChannelModelPricing{CacheWrite1hPrice: &price}},
		{"image-input", ChannelModelPricing{ImageInputPrice: &price}},
		{"fast", ChannelModelPricing{FastMultiplier: &price}},
		{"reasoning", ChannelModelPricing{ReasoningEffortMultipliers: map[string]float64{"high": 2}}},
		{"interval", ChannelModelPricing{Intervals: []PricingInterval{{MinTokens: 100}}}},
		{"time", ChannelModelPricing{TimePricing: &ChannelTimePricing{Timezone: "UTC"}}},
		{"request-mode", ChannelModelPricing{BillingMode: BillingModePerRequest}},
	} {
		t.Run(test.name, func(t *testing.T) {
			channel := &Channel{ID: 7, Description: autoPlazaChannelDescription, ModelPricing: []ChannelModelPricing{test.pricing}}
			repo := &automationChannelRepoStub{channel: channel}
			svc := NewGroupAutomationService(nil, NewChannelService(repo, nil, nil, nil, nil), nil, nil)
			report := &GroupAutomationReport{}
			svc.resyncAutoPlazaChannel(context.Background(), automationTestGroup(), 7, []string{"gpt-6-astra"}, "gateway", report)
			require.Empty(t, repo.replaced)
			require.Equal(t, "plaza_skipped", report.Items[0].Action)
			require.Equal(t, test.pricing, channel.ModelPricing[0])
		})
	}
}

func TestGroupAutomationConfirmedEmptyClearsOnlyOwnedModelList(t *testing.T) {
	g := automationTestGroup()
	g.Status = StatusActive
	repo := &automationChannelRepoStub{channel: &Channel{ID: 7, Description: autoPlazaChannelDescription, ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-6.1-sol"}}}}}
	svc := NewGroupAutomationService(&automationGroupRepoStub{groups: []Group{*g}}, NewChannelService(repo, nil, nil, nil, nil), nil, nil)
	svc.ResyncAutoPlazaForGroup(context.Background(), g.ID, nil)
	require.Len(t, repo.replaced, 1)
	require.Empty(t, repo.channel.SupportedModels())
	svc.ResyncAutoPlazaForGroup(context.Background(), g.ID, nil)
	require.Len(t, repo.replaced, 1, "empty reconciliation must be idempotent")
}

func TestGroupAutomationAllEntrypointsShareMutationLock(t *testing.T) {
	groups := &automationGroupRepoStub{groups: []Group{*automationTestGroup()}}
	repo := &automationChannelRepoStub{channel: &Channel{ID: 7, Description: autoPlazaChannelDescription}}
	svc := NewGroupAutomationService(groups, NewChannelService(repo, nil, nil, nil, nil), nil, nil)
	locks := &fakeLeaderLockCache{}
	_, err := locks.TryAcquireLeaderLock(context.Background(), autoPlazaReconcileLockKey, "peer", 0)
	require.NoError(t, err)
	svc.SetLeaderLock(locks, nil)
	require.NoError(t, svc.ReconcileAutoPlazaChannels(context.Background()))
	svc.ResyncAutoPlazaForGroup(context.Background(), 23, []string{"gpt-6.1-sol"})
	_, err = svc.AutoProvisionPublicGroups(context.Background(), 1)
	require.Error(t, err)
	require.Zero(t, groups.scans)
	require.Empty(t, repo.replaced)
}

func TestGroupAutomationFailedKeyRotationReleasesNewKeyAndPreservesOldKey(t *testing.T) {
	oldID := int64(99)
	persistErr := errors.New("monitor persistence failed")
	repo := &automationCleanupRepo{
		autoKeyRepoStub: autoKeyRepoStub{getByID: &ChannelMonitor{
			ID: 1, Provider: MonitorProviderOpenAI, APIMode: MonitorAPIModeResponses,
			GroupID: int64Ptr(9), KeyProvisionMode: "auto", ProvisionedAPIKeyID: &oldID,
			APIKey: "OLD:existing-key", PrimaryModel: "gpt-6.1-sol", IntervalSeconds: 60,
		}},
		updateErr: persistErr,
	}
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	provisioner := &autoKeyProvisionerStub{}
	svc.SetKeyProvisioner(provisioner)
	_, err := svc.Update(context.Background(), 1, ChannelMonitorUpdateParams{GroupID: int64Ptr(23)})
	require.ErrorIs(t, err, persistErr)
	require.Equal(t, []int64{23}, provisioner.provisioned)
	require.Equal(t, []int64{101}, provisioner.released)
	require.NotContains(t, provisioner.released, oldID)
}

func TestGroupAutomationRerunRepairsIncompleteMonitorInsteadOfSkipping(t *testing.T) {
	g := automationTestGroup()
	repo := &automationCleanupRepo{autoMonitor: &ChannelMonitor{
		ID: 1, GroupID: &g.ID, KeyProvisionMode: "auto", Description: autoGroupMonitorDescription, APIKey: "OLD:",
	}}
	monitor := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	monitor.SetKeyProvisioner(&autoKeyProvisionerStub{})
	svc := NewGroupAutomationService(nil, nil, monitor, nil)
	report := &GroupAutomationReport{}
	svc.ensureGroupMonitor(context.Background(), g, map[int64]struct{}{g.ID: {}}, func() ([]string, error) {
		t.Fatal("repairing a bound monitor must not load model candidates")
		return nil, nil
	}, 1, report)
	require.Len(t, report.Items, 1)
	require.Equal(t, "monitor_repaired", report.Items[0].Action)
	require.Len(t, repo.updated, 1)
}

func TestGroupAutomationModelCallbackUsesTheSameAllowlist(t *testing.T) {
	g := automationTestGroup()
	g.Status = StatusActive
	repo := &automationChannelRepoStub{channel: &Channel{ID: 7, Description: autoPlazaChannelDescription}}
	svc := NewGroupAutomationService(&automationGroupRepoStub{groups: []Group{*g}}, NewChannelService(repo, nil, nil, nil, nil), nil, nil)
	svc.ResyncAutoPlazaForGroup(context.Background(), g.ID, []string{"gpt-6-sol", "gpt-6.1-sol"})
	require.Len(t, repo.replaced, 1)
	require.Equal(t, []string{"gpt-6.1-sol"}, repo.replaced[0][0].Models)
}

func TestGroupAutomationMalformedDiscoveryDoesNotClearExistingModels(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":null}`, `{"error":{"message":"unavailable"}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			monitor := NewChannelMonitorService(&automationMonitorRepoStub{auto: &ChannelMonitor{APIKey: "OLD:test"}}, &duplicateChannelMonitorEncryptor{})
			monitor.SetLocalGatewayBase(server.URL)
			svc := NewGroupAutomationService(nil, nil, monitor, nil)
			models, source := svc.resolveGroupModels(context.Background(), automationTestGroup(), func() ([]string, error) {
				return []string{"gpt-6.1-sol"}, nil
			})
			require.Equal(t, "candidates", source)
			require.Equal(t, []string{"gpt-6.1-sol"}, models)
		})
	}
}

func TestGroupAutomationEmptyCatalogDoesNotReappearFromGroupCards(t *testing.T) {
	g := *automationTestGroup()
	channel := Channel{ID: 7, Status: StatusActive, Description: autoPlazaChannelDescription, GroupIDs: []int64{g.ID}, ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, BillingMode: BillingModeToken}}}
	svc := newPlazaService([]Channel{channel}, []Group{g}, nil)
	groups, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Empty(t, groups)
}
