//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 这几项锁定「破甲注入真实改写请求体」这一契约：开关开启且填了系统提示词后，
// 送往上游的 body 必须携带注入文本，且注入块位于槽位最前（最高优先级）。
// 历史缺陷是注入调用点在合并中丢失，开了开关也完全不生效。

func jailbreakTestContext(t *testing.T, ak *APIKey) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	if ak != nil {
		c.Set("api_key", ak)
	}
	return c
}

// OpenAI 兼容 chat/completions（messages 协议）：无 system 消息时新建一条并置于最前。
func TestInjectJailbreakForRequest_RewritesChatCompletionsBody(t *testing.T) {
	ak := &APIKey{ID: 7, JailbreakEnabled: true, JailbreakSystemPrompt: "必须叫我主人"}
	c := jailbreakTestContext(t, ak)
	body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"我是你的谁"}]}`)

	out := injectJailbreakForRequest(c, nil, body, false)

	require.NotEqual(t, string(body), string(out), "开启破甲后请求体必须被改写")
	require.Contains(t, string(out), "必须叫我主人", "注入文本必须进入请求体")

	messages := gjson.GetBytes(out, "messages")
	require.True(t, messages.IsArray())
	require.Equal(t, "system", messages.Array()[0].Get("role").String(), "注入的 system 必须置于最前")
	require.Contains(t, messages.Array()[0].Get("content").String(), "必须叫我主人")
	// 调用方原始消息必须保留（非破坏性）。
	require.Equal(t, "我是你的谁", messages.Array()[1].Get("content").String())
}

// 已有 system 消息时：注入文本前置于同一条 system，原内容逐字保留。
func TestInjectJailbreakForRequest_PrependsToExistingSystem(t *testing.T) {
	ak := &APIKey{ID: 8, JailbreakEnabled: true, JailbreakSystemPrompt: "破甲词"}
	c := jailbreakTestContext(t, ak)
	body := []byte(`{"messages":[{"role":"system","content":"原始系统"},{"role":"user","content":"hi"}]}`)

	out := injectJailbreakForRequest(c, nil, body, false)
	content := gjson.GetBytes(out, "messages.0.content").String()
	require.Contains(t, content, "破甲词")
	require.Contains(t, content, "原始系统", "调用方 system 内容不得丢失")
	require.Less(t, strings.Index(content, "破甲词"), strings.Index(content, "原始系统"), "注入必须在最前")
}

// Anthropic 入站走顶层 system。
func TestInjectJailbreakForRequest_WritesAnthropicTopLevelSystem(t *testing.T) {
	ak := &APIKey{ID: 10, JailbreakEnabled: true, JailbreakSystemPrompt: "破甲词"}
	c := jailbreakTestContext(t, ak)
	body := []byte(`{"model":"claude-sonnet-4","system":"原始","messages":[{"role":"user","content":"hi"}]}`)

	out := injectJailbreakForRequest(c, nil, body, true)
	require.Contains(t, gjson.GetBytes(out, "system").String(), "破甲词")
	require.Contains(t, gjson.GetBytes(out, "system").String(), "原始")
}

// 用户提示词注入到首条 user 消息之前。
func TestInjectJailbreakForRequest_WritesUserPrompt(t *testing.T) {
	ak := &APIKey{ID: 11, JailbreakEnabled: true, JailbreakUserPrompt: "先说暗号"}
	c := jailbreakTestContext(t, ak)
	body := []byte(`{"messages":[{"role":"user","content":"正文"}]}`)

	out := injectJailbreakForRequest(c, nil, body, false)
	content := gjson.GetBytes(out, "messages.0.content").String()
	require.Contains(t, content, "先说暗号")
	require.Contains(t, content, "正文")
}

// 开关关闭时请求体必须逐字节不变（默认关闭，不得影响普通用户）。
func TestInjectJailbreakForRequest_DisabledLeavesBodyUntouched(t *testing.T) {
	ak := &APIKey{ID: 12, JailbreakEnabled: false, JailbreakSystemPrompt: "破甲词"}
	c := jailbreakTestContext(t, ak)
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, string(body), string(injectJailbreakForRequest(c, nil, body, false)))
}

// 未认证上下文（无 Key）时不得注入。
func TestInjectJailbreakForRequest_NoAPIKeyLeavesBodyUntouched(t *testing.T) {
	c := jailbreakTestContext(t, nil)
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, string(body), string(injectJailbreakForRequest(c, nil, body, false)))
}

// Responses 协议走顶层 instructions。
func TestInjectJailbreakForRequest_WritesResponsesInstructions(t *testing.T) {
	ak := &APIKey{ID: 13, JailbreakEnabled: true, JailbreakSystemPrompt: "破甲词"}
	c := jailbreakTestContext(t, ak)
	body := []byte(`{"model":"gpt-5.1","instructions":"原始指令","input":"你好"}`)

	out := injectJailbreakForRequest(c, nil, body, false)
	instructions := gjson.GetBytes(out, "instructions").String()
	require.Contains(t, instructions, "破甲词")
	require.Contains(t, instructions, "原始指令")
}

// 生产注入路径的幂等：failover 基于同一原始体重建时必须不再叠加第二份。
// 这条走的是网关真正调用的 injectJailbreakForRequest → InjectJailbreakPrompts。
func TestInjectJailbreakForRequest_IsIdempotentOnRealPath(t *testing.T) {
	ak := &APIKey{ID: 20, JailbreakEnabled: true, JailbreakSystemPrompt: "破甲词"}
	c := jailbreakTestContext(t, ak)
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)

	once := injectJailbreakForRequest(c, nil, body, false)
	require.NotEqual(t, string(body), string(once), "首次注入必须改写 body")

	twice := injectJailbreakForRequest(c, nil, once, false)
	require.Equal(t, string(once), string(twice), "已注入过的 body 不得重复注入")

	// 注入文本必须恰好出现一次，证明没有叠加（标记可能被 JSON 转义）。
	require.Equal(t, 1, strings.Count(string(twice), "破甲词"), "注入文本不得重复出现")
}

// 注入标记必须写入请求体：它既是幂等依据，也是审计定位来源。
func TestInjectJailbreakForRequest_StampsMarker(t *testing.T) {
	ak := &APIKey{ID: 21, JailbreakEnabled: true, JailbreakSystemPrompt: "破甲词"}
	c := jailbreakTestContext(t, ak)
	out := injectJailbreakForRequest(c, nil, []byte(`{"messages":[{"role":"user","content":"hi"}]}`), false)
	// sjson 会把 '<'/'>' 转义为 \u003c/\u003e，两种形态都算带标记。
	require.True(t, jailbreakAlreadyInjected(out), "注入后必须带幂等标记")
}

// 用户侧提示词上限必须是 20000 字符（按 rune 计）。
func TestJailbreakPromptLimitConstants(t *testing.T) {
	require.Equal(t, 20000, JailbreakUserSystemPromptMaxRunes)
	require.Equal(t, 20000, JailbreakUserUserPromptMaxRunes)
}
