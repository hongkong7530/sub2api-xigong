package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func resetManagedJailbreakTemplatesForTest() {
	managedJailbreakMu.Lock()
	managedJailbreakTemplates = map[string]JailbreakTemplateRecord{}
	managedJailbreakLoaded = false
	managedJailbreakRefreshed = time.Time{}
	managedJailbreakInFlight = nil
	managedJailbreakAttempted = time.Time{}
	managedJailbreakLastError = nil
	managedJailbreakGeneration = 0
	managedJailbreakRefresher = nil
	managedJailbreakMu.Unlock()
}

func TestJailbreakTemplatesLoadFromPersistenceAfterRestart(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	t.Cleanup(resetManagedJailbreakTemplatesForTest)
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{SettingKeyJailbreakTemplates: `[{"name":"restart-template","system_prompt":"persisted","enabled":true}]`})
	NewSettingService(repo, nil)
	list, err := (&APIKeyService{}).ListAvailableJailbreakTemplates(context.Background())
	require.NoError(t, err)
	require.Contains(t, list, map[string]string{"name": "restart-template", "description": "", "source": "custom"})
	tpl, err := LoadJailbreakTemplate("restart-template")
	require.NoError(t, err)
	require.Equal(t, "persisted", tpl.SystemPrompt)
	// 模拟重新部署：进程注册表清空，数据库不变，不执行管理员保存。
	resetManagedJailbreakTemplatesForTest()
	NewSettingService(repo, nil)
	list, err = (&APIKeyService{}).ListAvailableJailbreakTemplates(context.Background())
	require.NoError(t, err)
	require.Contains(t, list, map[string]string{"name": "restart-template", "description": "", "source": "custom"})
	require.Empty(t, repo.syncedWrites())
}

func TestJailbreakTemplatesRefreshExistingHitFromAnotherNode(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	t.Cleanup(resetManagedJailbreakTemplatesForTest)
	repo := newCodexVersionSyncSettingRepoStub(map[string]string{SettingKeyJailbreakTemplates: `[{"name":"remote-template","system_prompt":"new-content","enabled":true}]`})
	NewSettingService(repo, nil)
	applyManagedJailbreakTemplates([]JailbreakTemplateRecord{{Name: "remote-template", SystemPrompt: "old-content", Enabled: true}})
	managedJailbreakMu.Lock()
	managedJailbreakRefreshed = time.Now().Add(-time.Minute)
	managedJailbreakMu.Unlock()
	// 热快照读取不等数据库；同名缓存会在后台刷新后更新。
	_, err := LoadJailbreakTemplate("remote-template")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		tpl, ok := managedJailbreakTemplateContent("remote-template")
		return ok && tpl.SystemPrompt == "new-content"
	}, time.Second, time.Millisecond)
}

func TestJailbreakTemplatesConcurrentColdReadWaitsForLoad(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	t.Cleanup(resetManagedJailbreakTemplatesForTest)
	entered, release := make(chan struct{}), make(chan struct{})
	RegisterJailbreakTemplateRefresher(func(context.Context) error {
		close(entered)
		<-release
		applyManagedJailbreakTemplates([]JailbreakTemplateRecord{{Name: "cold-template", SystemPrompt: "content", Enabled: true}})
		return nil
	})
	first := make(chan struct{})
	go func() { maybeRefreshManagedJailbreakTemplates(context.Background()); close(first) }()
	<-entered
	second := make(chan []map[string]string, 1)
	go func() {
		list, _ := (&APIKeyService{}).ListAvailableJailbreakTemplates(context.Background())
		second <- list
	}()
	select {
	case <-second:
		t.Fatal("cold read returned an incomplete registry")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-first
	require.Contains(t, <-second, map[string]string{"name": "cold-template", "description": "", "source": "custom"})
}

func TestValidateJailbreakTemplateRecord(t *testing.T) {
	valid := &JailbreakTemplateRecord{Name: "my-template_1", SystemPrompt: "sys"}
	require.NoError(t, ValidateJailbreakTemplateRecord(valid))
	require.Equal(t, "my-template_1", valid.Name)

	require.Error(t, ValidateJailbreakTemplateRecord(&JailbreakTemplateRecord{Name: "../evil", SystemPrompt: "sys"}))
	require.Error(t, ValidateJailbreakTemplateRecord(&JailbreakTemplateRecord{Name: "ok"}), "两个提示词都为空必须拒绝")
	require.Error(t, ValidateJailbreakTemplateRecord(&JailbreakTemplateRecord{
		Name:         "ok",
		SystemPrompt: strings.Repeat("a", jailbreakTemplateSystemPromptMaxLen+1),
	}))

	trimmed := &JailbreakTemplateRecord{Name: " ok ", Description: "  desc  ", SystemPrompt: "sys"}
	require.NoError(t, ValidateJailbreakTemplateRecord(trimmed))
	require.Equal(t, "ok", trimmed.Name)
	require.Equal(t, "desc", trimmed.Description)
}

func TestLoadJailbreakTemplatePrefersManagedRecord(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	defer resetManagedJailbreakTemplatesForTest()

	applyManagedJailbreakTemplates([]JailbreakTemplateRecord{
		{Name: "unit-jb-managed", SystemPrompt: "managed-system", UserPrompt: "managed-user", Enabled: true},
	})

	tpl, err := LoadJailbreakTemplate("unit-jb-managed")
	require.NoError(t, err)
	require.Equal(t, "managed-system", tpl.SystemPrompt)
	require.Equal(t, "managed-user", tpl.UserPrompt)
}

func TestListAvailableJailbreakTemplatesHidesDisabledManaged(t *testing.T) {
	resetManagedJailbreakTemplatesForTest()
	defer resetManagedJailbreakTemplatesForTest()

	applyManagedJailbreakTemplates([]JailbreakTemplateRecord{
		{Name: "unit-jb-on", Description: "启用中", SystemPrompt: "s", Enabled: true},
		{Name: "unit-jb-off", Description: "已停用", SystemPrompt: "s", Enabled: false},
	})

	svc := &APIKeyService{}
	list, err := svc.ListAvailableJailbreakTemplates(context.Background())
	require.NoError(t, err)

	sources := map[string]string{}
	for _, item := range list {
		sources[item["name"]] = item["source"]
	}
	require.Equal(t, "custom", sources["unit-jb-on"])
	_, hidden := sources["unit-jb-off"]
	require.False(t, hidden, "停用模板不应出现在用户可选列表")
}
