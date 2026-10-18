package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 破甲（Jailbreak）注入
//
// 所有协议共用一个注入入口 ApplyJailbreak，由 ParsedRequest.protocol 分派到具体槽位：
//
//	anthropic / claude code  -> 顶层 system（string | []ContentBlock）
//	openai / grok / 其它兼容 -> messages[] 中首条 system|developer 角色消息
//	responses                -> 顶层 instructions 字符串
//	gemini                   -> systemInstruction.parts[]
//
// 设计约束：
//  1. 最高优先级 —— 注入块永远位于槽位最前面（prepend 为默认），
//     因此在被截断、被用户自带 system 内容挤压时都能保留。
//  2. 幂等 —— 注入块带 JailbreakMarker，重试或 failover 重建 body 后不会重复注入。
//  3. 非破坏性 —— prepend / append 模式下调用方自带的 system 内容逐字保留。
const (
	// JailbreakMarker 标记一段由网关注入的破甲文本。
	// 它保留在最终请求体中：既作为幂等判断依据，也便于日志与审计定位来源。
	JailbreakMarker = "<!--XGJB:v1-->"

	// jailbreakMarkerEscaped 是标记经 JSON 编码后的转义形态：sjson 会把 '<' / '>'
	// 写成 \u003c / \u003e。判重必须同时接受两种形态，否则「已写入标记」的 body
	// 会被当作未注入，在重试/failover 时叠加第二份注入。
	jailbreakMarkerEscaped = "\\u003c!--XGJB:v1--\\u003e"

	jailbreakSlotGlue = "\n\n"

	jailbreakBlockStyleText   = "text"
	jailbreakBlockStyleGemini = "gemini"

	// jailbreakProtocolResponses 对应 OpenAI Responses API（网关内部以 "responses" 标识）。
	jailbreakProtocolResponses = "responses"
)

// JailbreakMode 决定注入文本与调用方已有 system 内容的组合方式。
type JailbreakMode string

const (
	// JailbreakModePrepend 注入文本置于最前，保留调用方 system 内容（默认，优先级最高）。
	JailbreakModePrepend JailbreakMode = "prepend"
	// JailbreakModeAppend 注入文本置于最后，保留调用方 system 内容。
	JailbreakModeAppend JailbreakMode = "append"
	// JailbreakModeReplace 丢弃调用方 system 内容，仅使用注入文本。
	JailbreakModeReplace JailbreakMode = "replace"
)

// JailbreakProfile 是单个 API Key 上解析完成的破甲配置。
// 它不依赖存储层，调用方可以从数据库字段、预设文件或请求参数构造。
type JailbreakProfile struct {
	Enabled    bool
	Mode       JailbreakMode
	SystemText string
	UserText   string
	// Protocols 限定生效协议（如 ["anthropic","grok"]）；为空表示全部协议生效。
	Protocols []string
}

// Active 报告该配置是否有实际可注入的内容。
func (p *JailbreakProfile) Active() bool {
	if p == nil || !p.Enabled {
		return false
	}
	return strings.TrimSpace(p.SystemText) != "" || strings.TrimSpace(p.UserText) != ""
}

// AppliesTo 报告该配置是否对给定协议生效。
func (p *JailbreakProfile) AppliesTo(protocol string) bool {
	if !p.Active() {
		return false
	}
	if len(p.Protocols) == 0 {
		return true
	}
	for _, item := range p.Protocols {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(protocol)) {
			return true
		}
	}
	return false
}

func (p *JailbreakProfile) effectiveMode() JailbreakMode {
	switch p.Mode {
	case JailbreakModeAppend:
		return JailbreakModeAppend
	case JailbreakModeReplace:
		return JailbreakModeReplace
	default:
		return JailbreakModePrepend
	}
}

// jailbreakAlreadyInjected 判断 body 是否已含注入了的破甲文本。
// 同时接受原样标记与 JSON 转义形态，二者语义相同。
func jailbreakAlreadyInjected(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if bytes.Contains(body, []byte(JailbreakMarker)) {
		return true
	}
	return bytes.Contains(body, []byte(jailbreakMarkerEscaped))
}

// jailbreakSystemPayload 组装系统槽位的注入文本，始终以 marker 开头。
func jailbreakSystemPayload(p *JailbreakProfile) string {
	text := strings.TrimSpace(p.SystemText)
	if text == "" {
		return ""
	}
	return JailbreakMarker + "\n" + text
}

// ApplyJailbreak 把破甲配置写入请求体。
// 返回 applied 表示请求体是否被改写；改写后 ParsedRequest 的 Body 与 raw range 均已刷新。
func ApplyJailbreak(parsed *ParsedRequest, profile *JailbreakProfile) (bool, error) {
	if parsed == nil || parsed.Body == nil || !profile.Active() {
		return false, nil
	}
	if !profile.AppliesTo(parsed.protocol) {
		return false, nil
	}

	body := parsed.Body.Bytes()
	if len(body) == 0 {
		return false, nil
	}
	// 幂等：重试与 failover 会基于同一原始体重建请求，已注入过的不再重复注入。
	if strings.Contains(string(body), JailbreakMarker) {
		return false, nil
	}

	out := body
	changed := false

	if systemText := jailbreakSystemPayload(profile); systemText != "" {
		rewritten, applied, err := injectJailbreakSystem(out, parsed.protocol, profile.effectiveMode(), systemText)
		if err != nil {
			return false, fmt.Errorf("jailbreak: inject system slot: %w", err)
		}
		out, changed = rewritten, applied
	}

	if userText := strings.TrimSpace(profile.UserText); userText != "" {
		rewritten, applied, err := injectJailbreakUserPrompt(out, userText)
		if err != nil {
			return false, fmt.Errorf("jailbreak: inject user prompt: %w", err)
		}
		out = rewritten
		changed = changed || applied
	}

	if !changed {
		return false, nil
	}
	if err := parsed.ReplaceBody(out); err != nil {
		return false, err
	}
	return true, nil
}

// injectJailbreakSystem 按协议把注入文本写入对应的系统槽位。
func injectJailbreakSystem(body []byte, protocol string, mode JailbreakMode, text string) ([]byte, bool, error) {
	switch protocol {
	case domain.PlatformGemini:
		parts, err := mergeJailbreakSlot(
			gjson.GetBytes(body, "systemInstruction.parts"), mode, text, jailbreakBlockStyleGemini, true)
		if err != nil {
			return body, false, err
		}
		out, err := sjson.SetBytes(body, "systemInstruction.parts", parts)
		if err != nil {
			return body, false, err
		}
		return out, true, nil

	case jailbreakProtocolResponses:
		merged, err := mergeJailbreakSlot(
			gjson.GetBytes(body, "instructions"), mode, text, jailbreakBlockStyleText, false)
		if err != nil {
			return body, false, err
		}
		out, err := sjson.SetBytes(body, "instructions", merged)
		if err != nil {
			return body, false, err
		}
		return out, true, nil

	case domain.PlatformAnthropic:
		return setJailbreakTopLevelSystem(body, mode, text)

	default:
		// OpenAI Chat Completions、Grok 以及各 OpenAI 兼容渠道把 system 放在 messages 里。
		if gjson.GetBytes(body, "messages").IsArray() {
			return injectJailbreakMessageSystem(body, mode, text)
		}
		return setJailbreakTopLevelSystem(body, mode, text)
	}
}

// setJailbreakTopLevelSystem 写入顶层 system 字段（Anthropic Messages 协议形态）。
func setJailbreakTopLevelSystem(body []byte, mode JailbreakMode, text string) ([]byte, bool, error) {
	merged, err := mergeJailbreakSlot(
		gjson.GetBytes(body, "system"), mode, text, jailbreakBlockStyleText, false)
	if err != nil {
		return body, false, err
	}
	out, err := sjson.SetBytes(body, "system", merged)
	if err != nil {
		return body, false, err
	}
	return out, true, nil
}

// injectJailbreakMessageSystem 写入 messages[] 中首条 system / developer 角色消息。
func injectJailbreakMessageSystem(body []byte, mode JailbreakMode, text string) ([]byte, bool, error) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body, false, nil
	}

	items := messages.Array()
	slotIndex := -1
	for i, item := range items {
		switch item.Get("role").String() {
		case "system", "developer":
			slotIndex = i
		}
		if slotIndex >= 0 {
			break
		}
	}

	if slotIndex >= 0 {
		path := fmt.Sprintf("messages.%d.content", slotIndex)
		merged, err := mergeJailbreakSlot(
			gjson.GetBytes(body, path), mode, text, jailbreakBlockStyleText, false)
		if err != nil {
			return body, false, err
		}
		out, err := sjson.SetBytes(body, path, merged)
		if err != nil {
			return body, false, err
		}
		return out, true, nil
	}

	// 客户端完全没传 system 角色消息：需要新建一条并置于最前。
	// sjson 没有 unshift 语义，这里重建数组；该分支只在无 system 消息时发生。
	rebuilt := make([]any, 0, len(items)+1)
	rebuilt = append(rebuilt, map[string]any{"role": "system", "content": text})
	for _, item := range items {
		var value any
		if err := json.Unmarshal([]byte(item.Raw), &value); err != nil {
			return body, false, err
		}
		rebuilt = append(rebuilt, value)
	}
	out, err := sjson.SetBytes(body, "messages", rebuilt)
	if err != nil {
		return body, false, err
	}
	return out, true, nil
}

// injectJailbreakUserPrompt 把自定义用户提示词前置到首条 user 消息。
// 只改首条 user 消息而不新增合成轮次，避免破坏 Anthropic 的角色交替约束与 prompt cache 前缀。
func injectJailbreakUserPrompt(body []byte, text string) ([]byte, bool, error) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body, false, nil
	}
	for i, item := range messages.Array() {
		if item.Get("role").String() != "user" {
			continue
		}
		path := fmt.Sprintf("messages.%d.content", i)
		merged, err := mergeJailbreakSlot(
			gjson.GetBytes(body, path), JailbreakModePrepend, text, jailbreakBlockStyleText, false)
		if err != nil {
			return body, false, err
		}
		out, err := sjson.SetBytes(body, path, merged)
		if err != nil {
			return body, false, err
		}
		return out, true, nil
	}
	return body, false, nil
}

// mergeJailbreakSlot 按模式把注入文本合入一个既有的文本槽位。
// 槽位既可能是 JSON 字符串，也可能是内容块数组；forceArray 用于 gemini parts 这类只接受数组的槽位。
func mergeJailbreakSlot(raw gjson.Result, mode JailbreakMode, text, style string, forceArray bool) (any, error) {
	if mode == JailbreakModeReplace {
		return emptyOrArray(text, style, forceArray), nil
	}

	// 槽位缺失或为 null：直接写入注入文本。
	if !raw.Exists() || raw.Type == gjson.Null {
		return emptyOrArray(text, style, forceArray), nil
	}

	if raw.Type == gjson.String {
		existing := raw.String()
		var combined string
		switch {
		case strings.TrimSpace(existing) == "":
			combined = text
		case mode == JailbreakModeAppend:
			combined = existing + jailbreakSlotGlue + text
		default:
			combined = text + jailbreakSlotGlue + existing
		}
		if forceArray {
			return []any{jailbreakBlock(combined, style)}, nil
		}
		return combined, nil
	}

	if raw.IsArray() {
		blocks := make([]any, 0, len(raw.Array())+1)
		if mode == JailbreakModeAppend {
			for _, item := range raw.Array() {
				value, err := decodeRawItem(item)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, value)
			}
			blocks = append(blocks, jailbreakBlock(text, style))
		} else {
			blocks = append(blocks, jailbreakBlock(text, style))
			for _, item := range raw.Array() {
				value, err := decodeRawItem(item)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, value)
			}
		}
		return blocks, nil
	}

	// 非预期类型（数字、对象等）：整体替换，保证注入一定生效。
	return emptyOrArray(text, style, forceArray), nil
}

func emptyOrArray(text, style string, forceArray bool) any {
	if forceArray {
		return []any{jailbreakBlock(text, style)}
	}
	return text
}

func jailbreakBlock(text, style string) map[string]any {
	if style == jailbreakBlockStyleGemini {
		return map[string]any{"text": text}
	}
	return map[string]any{"type": "text", "text": text}
}

func decodeRawItem(item gjson.Result) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(item.Raw), &value); err != nil {
		return nil, err
	}
	return value, nil
}
