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

// autoKeyRepoStub 记录 Create/Delete/Update，用于验证 auto 配给链路的落库与回滚。
type autoKeyRepoStub struct {
	ChannelMonitorRepository
	created   []*ChannelMonitor
	deleted   []int64
	updated   []*ChannelMonitor
	createErr error
	getByID   *ChannelMonitor
}

func (r *autoKeyRepoStub) Create(_ context.Context, m *ChannelMonitor) error {
	if r.createErr != nil {
		return r.createErr
	}
	clone := *m
	if clone.ID == 0 {
		clone.ID = int64(len(r.created) + 1)
	}
	*m = clone
	r.created = append(r.created, &clone)
	return nil
}

func (r *autoKeyRepoStub) GetByID(_ context.Context, id int64) (*ChannelMonitor, error) {
	if r.getByID == nil {
		return nil, ErrChannelMonitorNotFound
	}
	clone := *r.getByID
	clone.ID = id
	return &clone, nil
}

func (r *autoKeyRepoStub) Update(_ context.Context, m *ChannelMonitor) error {
	clone := *m
	r.updated = append(r.updated, &clone)
	return nil
}

func (r *autoKeyRepoStub) Delete(_ context.Context, id int64) error {
	r.deleted = append(r.deleted, id)
	return nil
}

// autoKeyProvisionerStub 记录配给与回收调用。
type autoKeyProvisionerStub struct {
	provisioned []int64 // groupID
	released    []int64 // keyID
	provisionErr error
}

func (p *autoKeyProvisionerStub) ProvisionChannelMonitorKey(
	_ context.Context, _ int64, groupID int64, _ string,
) (*ChannelMonitorProvisionedKey, error) {
	if p.provisionErr != nil {
		return nil, p.provisionErr
	}
	p.provisioned = append(p.provisioned, groupID)
	return &ChannelMonitorProvisionedKey{
		ID:        int64(100 + len(p.provisioned)),
		Key:       "sk-auto-provisioned",
		GroupID:   groupID,
		GroupName: "group",
	}, nil
}

func (p *autoKeyProvisionerStub) ReleaseChannelMonitorKey(_ context.Context, keyID int64) error {
	p.released = append(p.released, keyID)
	return nil
}

func newAutoKeyService(repo *autoKeyRepoStub) *ChannelMonitorService {
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{})
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{
		Enabled: true,
		Mode:    ChannelMonitorModeV2,
	}})
	return svc
}

func autoKeyCreateParams(groupID int64) ChannelMonitorCreateParams {
	return ChannelMonitorCreateParams{
		Name:             "GPT-5xTeam · 自动监控",
		Provider:         MonitorProviderOpenAI,
		APIMode:          MonitorAPIModeResponses,
		PrimaryModel:     "gpt-5.1",
		GroupName:        "GPT-5xTeam",
		GroupID:          &groupID,
		KeyProvisionMode: "auto",
		Description:      "自动化创建：公开分组监控",
		Enabled:          true,
		IntervalSeconds:  autoMonitorIntervalSeconds,
		JitterSeconds:    autoMonitorJitterSeconds,
		CreatedBy:        1,
	}
}

// 公开分组自动化不传 api_key，靠 auto 配给；缺了这条分支每个分组都会以
// 「缺 API Key」失败（生产表现为 channels 建成功、channel_monitors 为 0）。
func TestCreate_AutoKeyProvisioningSucceedsWithoutPlainAPIKey(t *testing.T) {
	repo := &autoKeyRepoStub{}
	svc := newAutoKeyService(repo)
	provisioner := &autoKeyProvisionerStub{}
	svc.SetKeyProvisioner(provisioner)

	created, err := svc.Create(context.Background(), autoKeyCreateParams(9))
	require.NoError(t, err)
	require.NotNil(t, created)

	// 分组绑定与 auto 模式必须落库，否则监控不算「绑定分组」。
	require.Len(t, repo.created, 1)
	require.NotNil(t, repo.created[0].GroupID)
	require.Equal(t, int64(9), *repo.created[0].GroupID)
	require.Equal(t, "auto", repo.created[0].KeyProvisionMode)
	require.Equal(t, "自动化创建：公开分组监控", repo.created[0].Description)

	// Key 由服务端配给并持久化到监控行。
	require.Equal(t, []int64{9}, provisioner.provisioned)
	require.NotEmpty(t, repo.updated)
	require.NotNil(t, repo.updated[len(repo.updated)-1].ProvisionedAPIKeyID)
	// 明文回填给调用方，密文不出 service 层。
	require.Equal(t, "sk-auto-provisioned", created.APIKey)
}

// auto 模式缺少分组必须被拒（分组是配给 Key 的前提）。
func TestCreate_AutoKeyRequiresGroup(t *testing.T) {
	repo := &autoKeyRepoStub{}
	svc := newAutoKeyService(repo)
	svc.SetKeyProvisioner(&autoKeyProvisionerStub{})

	params := autoKeyCreateParams(9)
	params.GroupID = nil

	_, err := svc.Create(context.Background(), params)
	require.ErrorIs(t, err, ErrChannelMonitorGroupRequired)
	require.Empty(t, repo.created)
}

// 配给失败必须回滚整条监控：留下没有 Key 的 monitor 会让 runner 反复失败。
func TestCreate_AutoKeyProvisioningFailureRollsBackMonitor(t *testing.T) {
	repo := &autoKeyRepoStub{}
	svc := newAutoKeyService(repo)
	svc.SetKeyProvisioner(&autoKeyProvisionerStub{provisionErr: errors.New("gateway rejected")})

	_, err := svc.Create(context.Background(), autoKeyCreateParams(10))
	require.Error(t, err)
	require.Len(t, repo.created, 1)
	require.Equal(t, []int64{repo.created[0].ID}, repo.deleted)
}

// 未注入配给器时必须 fail-closed（而不是建出无 Key 的坏监控）。
func TestCreate_AutoKeyProvisionerMissingFailsClosed(t *testing.T) {
	repo := &autoKeyRepoStub{}
	svc := newAutoKeyService(repo)

	_, err := svc.Create(context.Background(), autoKeyCreateParams(11))
	require.ErrorIs(t, err, ErrChannelMonitorKeyProvisionerUnavailable)
	require.Equal(t, []int64{repo.created[0].ID}, repo.deleted)
}

// manual 模式保持原语义：仍要求调用方提供 api_key。
func TestCreate_ManualModeStillRequiresAPIKey(t *testing.T) {
	repo := &autoKeyRepoStub{}
	svc := newAutoKeyService(repo)

	params := autoKeyCreateParams(12)
	params.KeyProvisionMode = ""
	params.APIKey = ""
	// manual 模式仍需合法 endpoint，才能走到 api_key 校验这一步。
	params.Endpoint = "https://1.1.1.1"

	_, err := svc.Create(context.Background(), params)
	require.ErrorIs(t, err, ErrChannelMonitorMissingAPIKey)
	require.Empty(t, repo.created)
}

// 删除监控必须回收自动配给的 Key，避免留下无人引用的网关凭据。
// 在真实 HTTP 边界验证空地址的 auto 监控，而不是只验证配给入库。
func TestRunChecks_AutoEmptyEndpointUsesPinnedLocalGateway(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodHead {
			return
		}
		require.Equal(t, "Bearer sk-auto-provisioned", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer server.Close()
	svc := newAutoKeyService(&autoKeyRepoStub{})
	svc.SetLocalGatewayBase(server.URL)
	results := svc.runChecksConcurrent(context.Background(), &ChannelMonitor{
		Provider: MonitorProviderOpenAI, APIMode: MonitorAPIModeResponses,
		KeyProvisionMode: "auto", PrimaryModel: "gpt-test", APIKey: "sk-auto-provisioned",
		BodyOverrideMode: MonitorBodyOverrideModeReplace,
		BodyOverride: map[string]any{"model": "gpt-test", "input": "hi", "instructions": "reply ok"},
	})
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusOperational, results[0].Status, results[0].Message)
	require.Nil(t, results[0].PingLatencyMs)
	require.Equal(t, []string{"POST /v1/responses"}, paths)
}

func TestPersistMonitorCheckRetainsFirstToken(t *testing.T) {
	repo := &quotaModeRepoStub{}
	svc := newQuotaModeService(repo)
	firstToken, latency := 1320, 1400
	svc.persistCheckResults(context.Background(), &ChannelMonitor{ID: 1}, []*CheckResult{{
		Model: "gpt-test", Status: MonitorStatusOperational, FirstTokenMs: &firstToken,
		LatencyMs: &latency, CheckedAt: time.Now(),
	}})
	require.Len(t, repo.history, 1)
	require.Equal(t, &firstToken, repo.history[0].FirstTokenMs)
	require.Equal(t, []int64{1}, repo.markedIDs)
}

func TestMonitorLatencyThresholdsMatchUsageHealth(t *testing.T) {
	for _, tc := range []struct { ms int; status string }{
		{1999, MonitorStatusOperational}, {2000, MonitorStatusOperational},
		{9999, MonitorStatusOperational}, {10000, MonitorStatusDegraded},
		{30000, MonitorStatusDegraded}, {60000, MonitorStatusDegraded},
	} {
		res := &CheckResult{}
		finalizeFirstTokenStatus(res, time.Duration(tc.ms)*time.Millisecond, tc.ms)
		require.Equal(t, tc.status, res.Status, "first token %dms", tc.ms)
	}
	require.Equal(t, MonitorStatusOperational, finalizeOperationalOrDegraded(&CheckResult{}, 59999*time.Millisecond, 59999).Status)
	require.Equal(t, MonitorStatusDegraded, finalizeOperationalOrDegraded(&CheckResult{}, 60*time.Second, 60000).Status)
}

func TestRunChecks_AutoMissingLocalGatewayFailsWithoutRelativeRequest(t *testing.T) {
	svc := newAutoKeyService(&autoKeyRepoStub{})
	results := svc.runChecksConcurrent(context.Background(), &ChannelMonitor{
		Provider: MonitorProviderOpenAI, KeyProvisionMode: "auto", PrimaryModel: "gpt-test",
	})
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusError, results[0].Status)
	require.Equal(t, "local gateway client not configured", results[0].Message)
}

func TestDelete_ReleasesProvisionedKey(t *testing.T) {
	keyID := int64(777)
	repo := &autoKeyRepoStub{getByID: &ChannelMonitor{
		ID:                  5,
		Name:                "bound",
		GroupID:             int64Ptr(9),
		KeyProvisionMode:    "auto",
		ProvisionedAPIKeyID: &keyID,
	}}
	svc := newAutoKeyService(repo)
	provisioner := &autoKeyProvisionerStub{}
	svc.SetKeyProvisioner(provisioner)

	require.NoError(t, svc.Delete(context.Background(), 5))
	require.Equal(t, []int64{keyID}, provisioner.released)
}
