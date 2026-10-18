package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// JailbreakTemplate represents a loaded jailbreak template
type JailbreakTemplate struct {
	Name          string
	SystemPrompt  string
	UserPrompt    string
	TriggerMarker string // e.g., "[xigong]"
}

var (
	jailbreakTemplates     = make(map[string]*JailbreakTemplate)
	jailbreakTemplatesMu   sync.RWMutex
	jailbreakTemplatesPath = filepath.Join("破甲资源")
)

// LoadJailbreakTemplate 解析模板：管理员维护的模板优先，其次回退内置文件模板。
func LoadJailbreakTemplate(name string) (*JailbreakTemplate, error) {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid jailbreak template name: %s", name)
	}
	// 先检查刷新期限，命中旧缓存也必须接收其他节点保存的更新。
	if err := maybeRefreshManagedJailbreakTemplates(context.Background()); err != nil {
		return nil, fmt.Errorf("load managed jailbreak templates: %w", err)
	}
	if tpl, ok := managedJailbreakTemplateContent(name); ok {
		return tpl, nil
	}
	return loadJailbreakFileTemplate(name)
}

// loadJailbreakFileTemplate 从内置文件目录读取模板并缓存。
func loadJailbreakFileTemplate(name string) (*JailbreakTemplate, error) {
	jailbreakTemplatesMu.RLock()
	if tpl, ok := jailbreakTemplates[name]; ok {
		jailbreakTemplatesMu.RUnlock()
		return tpl, nil
	}
	jailbreakTemplatesMu.RUnlock()

	// Try to load from disk（兼容仓库根目录、二进制旁、上级目录三种部署形态）
	var data []byte
	var loadErr error
	for _, dir := range jailbreakTemplateDirCandidates() {
		path := filepath.Join(dir, name+".txt")
		if data, loadErr = os.ReadFile(path); loadErr == nil {
			break
		}
	}
	if loadErr != nil {
		return nil, fmt.Errorf("load jailbreak template %s: %w", name, loadErr)
	}

	content := string(data)
	tpl := &JailbreakTemplate{
		Name:          name,
		SystemPrompt:  content,
		UserPrompt:    "",
		TriggerMarker: extractTriggerMarker(content),
	}

	// Cache it
	jailbreakTemplatesMu.Lock()
	jailbreakTemplates[name] = tpl
	jailbreakTemplatesMu.Unlock()

	return tpl, nil
}

// extractTriggerMarker extracts the trigger marker from template content
// Looks for patterns like [xigong], [Kovak], etc.
func extractTriggerMarker(content string) string {
	// Simple heuristic: find first occurrence of [word]
	start := strings.Index(content, "[")
	if start == -1 {
		return ""
	}
	end := strings.Index(content[start:], "]")
	if end == -1 {
		return ""
	}
	return content[start : start+end+1]
}

// InjectJailbreakPrompts injects jailbreak prompts into request body with highest priority
// Supports OpenAI, Claude (Anthropic), and Grok protocols
func InjectJailbreakPrompts(body []byte, apiKey *APIKey, protocol string) ([]byte, error) {
	if apiKey == nil || !apiKey.JailbreakEnabled {
		return body, nil
	}

	systemPrompt := apiKey.JailbreakSystemPrompt
	userPrompt := apiKey.JailbreakUserPrompt

	// Load template if specified
	if apiKey.JailbreakTemplate != "" && systemPrompt == "" {
		tpl, err := LoadJailbreakTemplate(apiKey.JailbreakTemplate)
		if err == nil {
			systemPrompt = tpl.SystemPrompt
			if userPrompt == "" {
				userPrompt = tpl.UserPrompt
			}
		}
	}

	// 幂等：重试与 failover 会基于同一原始体重建请求，已注入过的不再叠加。
	// sjson 会把 '<' 转义为 \u003c，因此判重必须同时接受两种形态，
	// 否则转义后的 body 会被判定为未注入而叠加第二份。
	if jailbreakAlreadyInjected(body) {
		return body, nil
	}

	// 按 body 形态选择注入策略：由实际上游线格式决定，而非账号平台。
	// （/v1/messages 转换、Responses 桥接等场景下，同一平台会承载不同形态的 body。）
	var out []byte
	var err error
	switch {
	case protocol == "anthropic":
		out, err = injectClaudeJailbreak(body, systemPrompt, userPrompt)
	case gjson.GetBytes(body, "messages").Exists():
		out, err = injectOpenAIJailbreak(body, systemPrompt, userPrompt)
	default:
		out, err = injectOpenAIResponsesJailbreak(body, systemPrompt, userPrompt)
	}
	if err != nil {
		return body, err
	}
	if bytes.Equal(out, body) {
		return out, nil
	}
	return stampJailbreakMarker(out), nil
}

// stampJailbreakMarker 把幂等标记写入已注入的 body：system 槽位前置于注入文本之前。
// 单独成函数便于三条注入路径共用同一标记语义。
func stampJailbreakMarker(body []byte) []byte {
	if jailbreakAlreadyInjected(body) {
		return body
	}
	// messages 形态：标记并入首条 system 的 content 之前。
	if gjson.GetBytes(body, "messages").IsArray() {
		items := gjson.GetBytes(body, "messages").Array()
		for i, item := range items {
			if item.Get("role").String() != "system" {
				continue
			}
			path := fmt.Sprintf("messages.%d.content", i)
			cur := gjson.GetBytes(body, path)
			var merged interface{}
			if cur.Type == gjson.String {
				merged = JailbreakMarker + jailbreakSlotGlue + cur.String()
			} else {
				merged = mergeJailbreakTextContent(rawToAny(cur), JailbreakMarker)
			}
			if next, setErr := sjson.SetBytes(body, path, merged); setErr == nil {
				return next
			}
			return body
		}
	}
	// 顶层 system / instructions 形态。
	for _, path := range []string{"system", "instructions"} {
		cur := gjson.GetBytes(body, path)
		if !cur.Exists() {
			continue
		}
		var merged interface{}
		if cur.Type == gjson.String {
			merged = JailbreakMarker + jailbreakSlotGlue + cur.String()
		} else {
			merged = mergeJailbreakTextContent(rawToAny(cur), JailbreakMarker)
		}
		if next, setErr := sjson.SetBytes(body, path, merged); setErr == nil {
			return next
		}
	}
	return body
}

func rawToAny(r gjson.Result) interface{} {
	if r.Type == gjson.String {
		return r.String()
	}
	var value interface{}
	if err := json.Unmarshal([]byte(r.Raw), &value); err == nil {
		return value
	}
	return r.Raw
}

// mergeJailbreakTextContent 把注入文本前置于一条消息的 content。
// content 既可能是字符串，也可能是 content block 数组（多模态消息）。
func mergeJailbreakTextContent(existing interface{}, inject string) interface{} {
	if inject == "" {
		return existing
	}
	switch v := existing.(type) {
	case nil:
		return inject
	case string:
		if strings.TrimSpace(v) == "" {
			return inject
		}
		return inject + jailbreakSlotGlue + v
	case []interface{}:
		return append([]interface{}{map[string]interface{}{"type": "text", "text": inject}}, v...)
	default:
		// 非预期类型：保留原值并另起一条文本块，避免静默丢内容。
		return []interface{}{
			map[string]interface{}{"type": "text", "text": inject},
			map[string]interface{}{"type": "text", "text": fmt.Sprintf("%v", v)},
		}
	}
}

// mergeJailbreakSystemSlot 把注入文本前置于顶层 system 槽位（Anthropic 形态）。
// 槽位可为缺失、字符串或 content block 数组；任一情况下调用方内容都不得丢失。
func mergeJailbreakSystemSlot(existing gjson.Result, inject string) interface{} {
	if !existing.Exists() || existing.Type == gjson.Null {
		return inject
	}
	if existing.Type == gjson.String {
		prev := existing.String()
		if strings.TrimSpace(prev) == "" {
			return inject
		}
		return inject + jailbreakSlotGlue + prev
	}
	if existing.IsArray() {
		blocks := make([]interface{}, 0, len(existing.Array())+1)
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": inject})
		for _, item := range existing.Array() {
			var value interface{}
			if err := json.Unmarshal([]byte(item.Raw), &value); err == nil {
				blocks = append(blocks, value)
			}
		}
		return blocks
	}
	return inject
}

// DeterminePlatformProtocol maps account platform to protocol string for jailbreak injection
func DeterminePlatformProtocol(platform string) string {
	switch platform {
	case "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	case "grok":
		return "grok"
	default:
		return "openai" // fallback
	}
}

// injectOpenAIJailbreak injects jailbreak into OpenAI/Grok chat completions format
func injectOpenAIJailbreak(body []byte, systemPrompt, userPrompt string) ([]byte, error) {
	if systemPrompt == "" && userPrompt == "" {
		return body, nil
	}

	// Parse existing messages
	messagesJSON := gjson.GetBytes(body, "messages")
	if !messagesJSON.Exists() {
		return body, nil
	}

	messages := messagesJSON.Array()
	newMessages := make([]map[string]interface{}, 0, len(messages)+2)

	// 1. system 槽位：注入文本必须前置于调用方原有 system 之前（最高优先级），
	//    且原有内容逐字保留 —— 丢弃它会让客户端自带的约束（含安全指令）静默失效。
	systemInjected := false
	for _, msg := range messages {
		if msg.Get("role").String() != "system" {
			continue
		}
		var msgMap map[string]interface{}
		if err := json.Unmarshal([]byte(msg.Raw), &msgMap); err != nil {
			continue
		}
		msgMap["content"] = mergeJailbreakTextContent(msgMap["content"], systemPrompt)
		newMessages = append(newMessages, msgMap)
		systemInjected = true
		break
	}
	if systemPrompt != "" && !systemInjected {
		newMessages = append(newMessages, map[string]interface{}{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	// 2. 复制其余消息，跳过已被合并处理的那条 system。
	systemConsumed := false
	for _, msg := range messages {
		if msg.Get("role").String() == "system" {
			if !systemConsumed {
				systemConsumed = true
				continue
			}
			continue
		}
		var msgMap map[string]interface{}
		if err := json.Unmarshal([]byte(msg.Raw), &msgMap); err == nil {
			newMessages = append(newMessages, msgMap)
		}
	}

	// 3. 用户提示词：前置于首条 user 消息的 content，而不是另插一条独立 user 消息 ——
	//    新增轮次会破坏 Anthropic 的角色交替约束，也会让 prompt cache 前缀失效。
	//    原内容必须逐字保留。
	if userPrompt != "" {
		merged := false
		for _, msg := range newMessages {
			if role, ok := msg["role"].(string); ok && role == "user" {
				msg["content"] = mergeJailbreakTextContent(msg["content"], userPrompt)
				merged = true
				break
			}
		}
		if !merged {
			newMessages = append(newMessages, map[string]interface{}{
				"role":    "user",
				"content": userPrompt,
			})
		}
	}

	// Replace messages in body
	newBody, err := sjson.SetBytes(body, "messages", newMessages)
	if err != nil {
		return body, fmt.Errorf("inject openai jailbreak: %w", err)
	}

	return newBody, nil
}

// injectClaudeJailbreak injects jailbreak into Claude/Anthropic messages format
func injectClaudeJailbreak(body []byte, systemPrompt, userPrompt string) ([]byte, error) {
	if systemPrompt == "" && userPrompt == "" {
		return body, nil
	}

	newBody := body

	// 1. system 槽位：注入文本前置于调用方原有 system（string 或 content block 数组），
	//    原有内容必须逐字保留，不能整体替换掉客户端自带指令。
	if systemPrompt != "" {
		existing := gjson.GetBytes(newBody, "system")
		merged := mergeJailbreakSystemSlot(existing, systemPrompt)
		var err error
		newBody, err = sjson.SetBytes(newBody, "system", merged)
		if err != nil {
			return body, fmt.Errorf("inject claude system prompt: %w", err)
		}
	}

	// 2. Inject user prompt as first user message
	if userPrompt != "" {
		messagesJSON := gjson.GetBytes(newBody, "messages")
		if messagesJSON.Exists() {
			messages := messagesJSON.Array()
			newMessages := make([]map[string]interface{}, 0, len(messages)+1)

			// 前置于首条 user 消息的 content；新增独立轮次会破坏角色交替约束。
			merged := false
			for _, msg := range messages {
				var msgMap map[string]interface{}
				if err := json.Unmarshal([]byte(msg.Raw), &msgMap); err != nil {
					continue
				}
				if role, ok := msgMap["role"].(string); ok && role == "user" && !merged {
					msgMap["content"] = mergeJailbreakTextContent(msgMap["content"], userPrompt)
					merged = true
				}
				newMessages = append(newMessages, msgMap)
			}
			if !merged {
				newMessages = append(newMessages, map[string]interface{}{
					"role":    "user",
					"content": userPrompt,
				})
			}

			var err error
			newBody, err = sjson.SetBytes(newBody, "messages", newMessages)
			if err != nil {
				return body, fmt.Errorf("inject claude user prompt: %w", err)
			}
		}
	}

	return newBody, nil
}

// injectOpenAIResponsesJailbreak injects jailbreak into OpenAI Responses API format
// (instructions + input), used by /v1/responses and chat->responses bridges.
func injectOpenAIResponsesJailbreak(body []byte, systemPrompt, userPrompt string) ([]byte, error) {
	if systemPrompt == "" && userPrompt == "" {
		return body, nil
	}
	if !gjson.GetBytes(body, "input").Exists() && !gjson.GetBytes(body, "instructions").Exists() {
		return body, nil
	}

	out := body

	// 1. instructions：破甲系统提示词置于最前（最高优先级）
	if systemPrompt != "" {
		existing := gjson.GetBytes(out, "instructions").String()
		merged := systemPrompt
		if existing != "" {
			merged = systemPrompt + "\n\n" + existing
		}
		next, err := sjson.SetBytes(out, "instructions", merged)
		if err != nil {
			return body, fmt.Errorf("inject responses instructions: %w", err)
		}
		out = next
	}

	// 2. input：破甲用户提示词作为首条用户消息
	if userPrompt != "" {
		input := gjson.GetBytes(out, "input")
		if input.Exists() && input.IsArray() {
			items := input.Array()
			newItems := make([]interface{}, 0, len(items)+1)
			// 前置于首条 user item 的 content，保留其原有文本。
			mergedUser := false
			for _, item := range items {
				if item.Get("role").String() == "user" && !mergedUser {
					var value map[string]interface{}
					if err := json.Unmarshal([]byte(item.Raw), &value); err == nil {
						value["content"] = mergeJailbreakTextContent(value["content"], userPrompt)
						newItems = append(newItems, value)
						mergedUser = true
						continue
					}
				}
				var value interface{}
				if err := json.Unmarshal([]byte(item.Raw), &value); err == nil {
					newItems = append(newItems, value)
				}
			}
			if mergedUser {
				next, err := sjson.SetBytes(out, "input", newItems)
				if err != nil {
					return body, fmt.Errorf("inject responses input: %w", err)
				}
				return next, nil
			}
			// 无 user item：新增一条 user item 作为前置项（保留原实现语义）。
			newItems = append([]interface{}{map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": userPrompt},
				},
			}}, newItems...)
			next, err := sjson.SetBytes(out, "input", newItems)
			if err != nil {
				return body, fmt.Errorf("inject responses input: %w", err)
			}
			out = next
		}
	}

	return out, nil
}

// injectJailbreakForRequest applies jailbreak prompts to the request body when the
// authenticated API key has the feature enabled. Returns the original body unchanged
// when the feature is off, the body shape is unsupported, or injection fails.
func injectJailbreakForRequest(c *gin.Context, account *Account, body []byte, anthropicInbound bool) []byte {
	if c == nil {
		return body
	}
	ak := getAPIKeyFromContext(c)
	if ak == nil || !ak.JailbreakEnabled {
		return body
	}
	protocol := "openai"
	if account != nil {
		protocol = DeterminePlatformProtocol(account.Platform)
	}
	if anthropicInbound {
		protocol = "anthropic"
	}
	out, err := InjectJailbreakPrompts(body, ak, protocol)
	if err != nil {
		logger.LegacyPrintf("service.gateway", "[Jailbreak] Injection failed for key %d: %v", ak.ID, err)
		return body
	}
	if len(out) == 0 || bytes.Equal(out, body) {
		return body
	}
	logger.LegacyPrintf("service.gateway", "[Jailbreak] Applied for key %d (template=%s)", ak.ID, ak.JailbreakTemplate)
	return out
}

// jailbreakTemplateDirCandidates returns candidate directories for the template
// folder across deployment layouts: repo root, beside the binary, parent dir.
func jailbreakTemplateDirCandidates() []string {
	dirs := []string{jailbreakTemplatesPath}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), jailbreakTemplatesPath))
	}
	dirs = append(dirs, filepath.Join("..", jailbreakTemplatesPath))
	return dirs
}
