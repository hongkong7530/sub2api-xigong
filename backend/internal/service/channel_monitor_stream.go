package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// 流式首字（TTFT）探测。
//
// 计时公式：first_token_ms = 首个非空内容增量（SSE content / output_text / text delta）
// 到达时间 − 请求发出时间。在原Token预算和超时内收完整短响应，
// 以准确上游usage测量生成阶段速度；缺失usage保持未知，不用字数猜Token。
//
// 可用性与速度分离：收到首字但慢只降级，红色仅表示请求失败或没有内容。
//
// 仅 body_override_mode=off 的默认探测走此路径；merge/replace 自定义 body
// 保留 channel_monitor_checker.go 的非流式算术 challenge 路径。

// streamAdapter 描述一个 provider 流式首字探测的差异点。
type streamAdapter struct {
	buildPath    func(model string) string
	buildBody    func(model string) ([]byte, error)
	buildHeaders func(apiKey string) map[string]string
	// extractDelta 从单个 SSE data JSON 帧提取内容增量；空串表示该帧无内容。
	// 仅用于判定「流已产出内容」（failed 兜底）；首字计时见 isMeaningfulStreamFrame。
	extractDelta func(data []byte) string
}

// isMeaningfulStreamFrame 与网关记账 usage_logs.first_token_ms 的口径一致：
// 首个非 preamble、非 usage-only 的 SSE data 帧即视为首字（含推理/角色帧）。
// 此前只认文本增量，导致带推理的模型把整段推理耗时算进首字，探针 p50 18s
// 而真实用户同模型 p50 约 1s，全部落到红档。
func isMeaningfulStreamFrame(payload []byte) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return len(strings.TrimSpace(string(payload))) > 0
	}
	switch gjson.GetBytes(payload, "type").String() {
	case "response.created", "response.in_progress", "response.failed", "error", "message_start", "ping":
		return false
	}
	if gjson.GetBytes(payload, "error").Exists() {
		return false
	}
	// Chat Completions 结尾的 usage-only chunk（choices 为空）不算内容。
	if gjson.GetBytes(payload, "usage").Exists() && len(gjson.GetBytes(payload, "choices").Array()) == 0 {
		return false
	}
	return true
}

// newOpenAICompatibleChatStreamAdapter Chat Completions 同构流式 adapter
// （OpenAI / Grok / Kimi / DeepSeek / 智谱，仅路径不同）。
func newOpenAICompatibleChatStreamAdapter(path string) streamAdapter {
	return streamAdapter{
		buildPath: func(string) string { return path },
		buildBody: func(model string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"model":          model,
				"messages":       []map[string]string{{"role": "user", "content": monitorStreamProbePrompt}},
				"max_tokens":     monitorChallengeMaxTokens,
				"stream":         true,
				"stream_options": map[string]bool{"include_usage": true},
			})
		},
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{"Authorization": "Bearer " + apiKey}
		},
		extractDelta: func(data []byte) string {
			if delta := gjson.GetBytes(data, "choices.0.delta.content").String(); delta != "" {
				return delta
			}
			// 兼容把首帧内容放进 message.content 的网关实现。
			return gjson.GetBytes(data, "choices.0.message.content").String()
		},
	}
}

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var openAIResponsesStreamAdapter = streamAdapter{
	buildPath: func(string) string { return providerOpenAIResponsesPath },
	buildBody: func(model string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"model":             model,
			"input":             monitorStreamProbePrompt,
			"max_output_tokens": monitorChallengeMaxTokens,
			"stream":            true,
		})
	},
	buildHeaders: func(apiKey string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + apiKey}
	},
	extractDelta: func(data []byte) string {
		if gjson.GetBytes(data, "type").String() == "response.output_text.delta" {
			return gjson.GetBytes(data, "delta").String()
		}
		return ""
	},
}

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var anthropicStreamAdapter = streamAdapter{
	buildPath: func(string) string { return providerAnthropicPath },
	buildBody: func(model string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"model":      model,
			"messages":   []map[string]string{{"role": "user", "content": monitorStreamProbePrompt}},
			"max_tokens": monitorChallengeMaxTokens,
			"stream":     true,
		})
	},
	buildHeaders: func(apiKey string) map[string]string {
		return map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": monitorAnthropicAPIVersion,
		}
	},
	extractDelta: func(data []byte) string {
		if gjson.GetBytes(data, "type").String() == "content_block_delta" {
			return gjson.GetBytes(data, "delta.text").String()
		}
		return ""
	},
}

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var geminiStreamAdapter = streamAdapter{
	buildPath: func(model string) string { return fmt.Sprintf(providerGeminiStreamPathTemplate, model) },
	buildBody: func(string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"contents": []map[string]any{
				{"parts": []map[string]any{{"text": monitorStreamProbePrompt}}},
			},
			"generationConfig": map[string]any{"maxOutputTokens": monitorChallengeMaxTokens},
		})
	},
	buildHeaders: func(apiKey string) map[string]string {
		return map[string]string{"x-goog-api-key": apiKey}
	},
	extractDelta: func(data []byte) string {
		return gjson.GetBytes(data, "candidates.0.content.parts.0.text").String()
	},
}

// streamAdapterFor 返回 provider + api_mode 对应的流式 adapter。
func streamAdapterFor(provider, apiMode string) (streamAdapter, bool) {
	if provider == MonitorProviderOpenAI && defaultAPIMode(apiMode) == MonitorAPIModeResponses {
		return openAIResponsesStreamAdapter, true
	}
	switch provider {
	case MonitorProviderOpenAI, MonitorProviderKimi, MonitorProviderDeepseek:
		return newOpenAICompatibleChatStreamAdapter(providerOpenAIPath), true
	case MonitorProviderGrok:
		return newOpenAICompatibleChatStreamAdapter(providerGrokPath), true
	case MonitorProviderZhipu:
		return newOpenAICompatibleChatStreamAdapter(providerZhipuPath), true
	case MonitorProviderAnthropic:
		return anthropicStreamAdapter, true
	case MonitorProviderGemini:
		return geminiStreamAdapter, true
	default:
		return streamAdapter{}, false
	}
}

// runStreamingCheckForModel 执行一次流式首字探测。
// 第二个返回值为 false 表示该 provider/api_mode 无流式 adapter，
// 调用方回退非流式 challenge 路径。
func runStreamingCheckForModel(ctx context.Context, provider, endpoint, apiKey, model string, opts *CheckOptions) (*CheckResult, bool) {
	apiMode := checkAPIMode(opts)
	adapter, ok := streamAdapterFor(provider, apiMode)
	if !ok {
		return nil, false
	}
	if provider == MonitorProviderZhipu {
		buildPath := adapter.buildPath
		adapter.buildPath = func(model string) string {
			return monitorRequestPath(provider, endpoint, providerAdapter{buildPath: buildPath}, model)
		}
	}
	// 探测必须遵循配置的协议；本站网关自行处理上游能力转换。
	// 不把 Responses 的错误改成第二次 Chat 请求，以免掩盖原错误或重复付费。
	res, _ := runStreamingProbe(ctx, adapter, endpoint, apiKey, model, opts)
	return res, true
}

// runStreamingProbe 执行单次流式探测，返回结果与上游 HTTP 状态码（网络/构造错误为 0）。
func runStreamingProbe(ctx context.Context, adapter streamAdapter, endpoint, apiKey, model string, opts *CheckOptions) (*CheckResult, int) {
	res := &CheckResult{
		Model:     model,
		Status:    MonitorStatusError,
		CheckedAt: time.Now(),
	}
	body, err := adapter.buildBody(model)
	if err != nil {
		res.Message = truncateMessage(sanitizeErrorMessage("marshal stream body: " + err.Error()))
		return res, 0
	}
	headers := mergeHeaders(adapter.buildHeaders(apiKey), opts)
	fullURL := joinURL(endpoint, adapter.buildPath(model))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		res.Message = truncateMessage(sanitizeErrorMessage("build stream request: " + err.Error()))
		return res, 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := checkHTTPClient(opts).Do(req)
	if err != nil {
		setStreamCheckLatency(res, start)
		res.Message = truncateMessage(sanitizeErrorMessage("do request: " + err.Error()))
		return res, 0
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, monitorResponseMaxBytes))
		setStreamCheckLatency(res, start)
		res.Message = truncateMessage(sanitizeErrorMessage(
			fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, truncateForErrorBody(string(raw)))))
		return res, resp.StatusCode
	}

	measurement, streamErr := readMonitorOutputStream(resp.Body, adapter, start)
	elapsedMs := int(measurement.elapsed / time.Millisecond)
	res.LatencyMs = &elapsedMs
	if streamErr != nil {
		setStreamCheckLatency(res, start)
		res.Message = truncateMessage(sanitizeErrorMessage(streamErr.Error()))
		return res, resp.StatusCode
	}
	if measurement.first == nil || !measurement.content {
		res.Status = MonitorStatusFailed
		res.Message = truncateMessage("stream ended without content")
		return res, resp.StatusCode
	}
	ms := int(*measurement.first / time.Millisecond)
	res.FirstTokenMs = &ms
	finalizeFirstTokenStatus(res, *measurement.first, ms)
	if measurement.complete {
		// Formal preamble/reasoning starts the output window; TTFT health is unchanged.
		if outputMs := measurement.outputWindow.durationMs(); outputMs != nil {
			zero := 0
			res.OutputTokensPerSecond = MonitorOutputRate(measurement.outputTokens, outputMs, &zero)
		}
	}
	return res, resp.StatusCode
}

// finalizeFirstTokenStatus 不把已成功产出首字的慢请求误判为不可用。
func finalizeFirstTokenStatus(res *CheckResult, firstToken time.Duration, ms int) {
	switch {
	case firstToken < monitorFirstTokenOperationalThreshold:
		res.Status = MonitorStatusOperational
	default:
		res.Status = MonitorStatusDegraded
		res.Message = truncateMessage(fmt.Sprintf("slow first token: %dms", ms))
	}
}

// setStreamCheckLatency 记录从请求发出到当前的总耗时（探测生命周期总延迟）。
func setStreamCheckLatency(res *CheckResult, start time.Time) {
	ms := int(time.Since(start) / time.Millisecond)
	res.LatencyMs = &ms
}
