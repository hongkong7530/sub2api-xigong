package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// 破甲注入依赖认证快照把配置带到网关热路径。
// 快照或 JSON 往返丢字段时，注入会静默失效，因此这里做完整的往返断言。
func TestAPIKeyAuthSnapshotCarriesJailbreakConfig(t *testing.T) {
	svc := &APIKeyService{}
	apiKey := &APIKey{
		ID:     1,
		UserID: 2,
		Key:    "sk-jailbreak",
		Name:   "jb-key",
		Status: StatusActive,
		User: &User{
			ID:     2,
			Status: StatusActive,
			Role:   RoleUser,
		},
		JailbreakEnabled:      true,
		JailbreakSystemPrompt: "system-prompt",
		JailbreakUserPrompt:   "user-prompt",
		JailbreakTemplate:     "guomoPG",
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)

	// 模拟 L2 缓存的完整 JSON 往返（与 apiKeyCache.SetAuthCache/GetAuthCache 同构）。
	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)

	var decoded APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &decoded))

	materialized := svc.snapshotToAPIKey("sk-jailbreak", decoded.Snapshot)
	require.NotNil(t, materialized)
	require.True(t, materialized.JailbreakEnabled)
	require.Equal(t, "system-prompt", materialized.JailbreakSystemPrompt)
	require.Equal(t, "user-prompt", materialized.JailbreakUserPrompt)
	require.Equal(t, "guomoPG", materialized.JailbreakTemplate)
}
