//go:build unit

package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIFastPolicyGroupScopeAndUltrafastDTORoundTrip(t *testing.T) {
	input := &dto.OpenAIFastPolicySettings{Rules: []dto.OpenAIFastPolicyRule{{ServiceTier: "all", Action: "force_ultrafast", Scope: "apikey", GroupIDs: []int64{23}, ModelWhitelist: []string{"gpt-6-astra", "gpt-6.1-sol"}, FallbackAction: "block"}}}
	value := openaiFastPolicySettingsFromDTO(input)
	require.Equal(t, []int64{23}, value.Rules[0].GroupIDs)
	require.Equal(t, service.OpenAIFastPolicyActionForceUltrafast, value.Rules[0].Action)
	require.Equal(t, input, openaiFastPolicySettingsToDTO(value))
}
