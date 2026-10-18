package service

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
)

// MonitorOutputRate measures generation throughput, not whole-request latency.
// Counts come from upstream usage (including reasoning where reported). Missing
// timing/counts stay unknown; neither text length nor an older sample is a fallback.
func MonitorOutputRate(outputTokens int64, durationMs, firstTokenMs *int) *float64 {
	// 亚100ms的短流通常是缓冲成批到达；毫秒精度不足以可靠测量吞吐，不公布放大值。
	if outputTokens <= 0 || durationMs == nil || firstTokenMs == nil || *firstTokenMs < 0 || *durationMs-*firstTokenMs < 100 {
		return nil
	}
	rate := float64(outputTokens) * 1000 / float64(*durationMs-*firstTokenMs)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil
	}
	return &rate
}

type monitorStreamMeasurement struct {
	first        *time.Duration
	elapsed      time.Duration
	outputTokens int64
	complete     bool
	content      bool
	outputWindow streamOutputWindow
}

// Upstream output counts are cumulative snapshots, never per-frame increments.
func monitorOutputTokens(payload []byte) (int64, bool) {
	for _, path := range []string{"response.usage.output_tokens", "usage.completion_tokens", "usage.output_tokens", "message.usage.output_tokens"} {
		v := gjson.GetBytes(payload, path)
		if v.Type == gjson.Number && v.Float() >= 0 && v.Float() == math.Trunc(v.Float()) {
			usagePath := strings.TrimSuffix(path, ".output_tokens")
			usagePath = strings.TrimSuffix(usagePath, ".completion_tokens")
			usage := gjson.GetBytes(payload, usagePath)
			input := usage.Get("input_tokens").Int()
			if input == 0 {
				input = usage.Get("prompt_tokens").Int()
			}
			reasoning := usage.Get("output_tokens_details.reasoning_tokens").Int()
			if reasoning == 0 {
				reasoning = usage.Get("completion_tokens_details.reasoning_tokens").Int()
			}
			return xai.IncludeIndependentReasoningTokens(input, v.Int(), usage.Get("total_tokens").Int(), reasoning), true
		}
	}
	usage := gjson.GetBytes(payload, "usageMetadata")
	if !usage.Exists() {
		usage = gjson.GetBytes(payload, "response.usageMetadata")
	}
	candidates := usage.Get("candidatesTokenCount")
	if candidates.Type == gjson.Number && candidates.Int() >= 0 {
		return candidates.Int() + usage.Get("thoughtsTokenCount").Int(), true
	}
	return 0, false
}

// Responses metadata is not constrained by the output-token budget; keep the
// complete stream bounded separately from non-streaming/error snippets.
const monitorStreamMaxBytes = 1 << 20

// Read through the short response's terminal event and usage. This does not
// increase the configured output-token budget or issue a second upstream call.
func readMonitorOutputStream(body io.Reader, adapter streamAdapter, start time.Time) (monitorStreamMeasurement, error) {
	m := monitorStreamMeasurement{}
	limited := &io.LimitedReader{R: body, N: monitorStreamMaxBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 0, 64*1024), monitorStreamMaxBytes+1)
	var data []string
	failed := ""
	done := false
	consume := func() {
		if len(data) == 0 {
			return
		}
		payload := strings.Join(data, "\n")
		data = nil
		m.outputWindow.observe([]byte(payload), "", time.Now())
		if payload == "[DONE]" {
			m.complete = true
			if m.elapsed == 0 {
				m.elapsed = time.Since(start)
			}
			done = true
			return
		}
		raw := []byte(payload)
		if !gjson.ValidBytes(raw) {
			failed = "invalid stream JSON"
			return
		}
		kind := gjson.GetBytes(raw, "type").String()
		if gjson.GetBytes(raw, "error").Exists() || kind == "response.failed" || kind == "response.incomplete" {
			failed = gjson.GetBytes(raw, "error.message").String()
			if failed == "" {
				failed = gjson.GetBytes(raw, "response.error.message").String()
			}
			if failed == "" {
				failed = "upstream stream failed or incomplete"
			}
			return
		}
		if m.first == nil && (isMeaningfulStreamFrame(raw) || strings.TrimSpace(adapter.extractDelta(raw)) != "") {
			elapsed := time.Since(start)
			m.first = &elapsed
		}
		if strings.TrimSpace(adapter.extractDelta(raw)) != "" {
			m.content = true
		}
		// Anthropic message_start may contain a preliminary count, not final output usage.
		if kind != "message_start" {
			if tokens, ok := monitorOutputTokens(raw); ok {
				m.outputTokens = tokens
			}
		}
		terminal := kind == "response.completed" || kind == "response.done" || kind == "message_stop"
		if gjson.GetBytes(raw, "choices.0.finish_reason").Type == gjson.String {
			terminal = true
		}
		geminiFinish := gjson.GetBytes(raw, "candidates.0.finishReason").String()
		if geminiFinish != "" && geminiFinish != "STOP" && geminiFinish != "MAX_TOKENS" {
			failed = "upstream stream terminated without a successful completion"
			return
		}
		if geminiFinish != "" {
			terminal = true
		}
		if terminal {
			m.complete = true
			m.elapsed = time.Since(start)
		}
		// Responses/Anthropic/Gemini carry final usage at or before their terminal event.
		// Chat can send usage after finish_reason, so continue until DONE or EOF.
		if kind == "response.completed" || kind == "response.done" || kind == "message_stop" || gjson.GetBytes(raw, "candidates.0.finishReason").String() != "" {
			done = true
		}
	}
	for scanner.Scan() {
		if limited.N <= 0 {
			return m, fmt.Errorf("monitor stream exceeds response limit")
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			consume()
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		if failed != "" {
			return m, fmt.Errorf("stream error: %s", failed)
		}
		if done {
			break
		}
	}
	// A LimitedReader EOF may be inside JSON; classify the byte boundary first.
	if limited.N <= 0 {
		return m, fmt.Errorf("monitor stream exceeds response limit")
	}
	if !done {
		consume()
	}
	if failed != "" {
		return m, fmt.Errorf("stream error: %s", failed)
	}
	if err := scanner.Err(); err != nil {
		return m, fmt.Errorf("read stream: %w", err)
	}
	if m.elapsed == 0 {
		m.elapsed = time.Since(start)
	}
	return m, nil
}
