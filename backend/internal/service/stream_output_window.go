package service

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// streamOutputWindow is local to one upstream attempt/WS turn. It measures
// formal model events, including preambles/reasoning, not HTTP headers or pings.
// TTFT and health remain separate. Failed/incomplete streams have no rate.
type streamOutputWindow struct {
	first         time.Time
	end           time.Time
	failed        bool
	pendingError  bool
	choiceDone    map[int]bool
	candidateDone map[int]bool
}

func (w *streamOutputWindow) observe(payload []byte, eventType string, now time.Time) {
	if w == nil || w.failed {
		return
	}
	text := strings.TrimSpace(string(payload))
	if text == "[DONE]" {
		if !w.pendingError && !w.first.IsZero() && w.end.IsZero() {
			w.end = now
		}
		return
	}
	if !gjson.ValidBytes(payload) {
		return
	}
	value := gjson.ParseBytes(payload)
	kind := strings.TrimSpace(eventType)
	if kind == "" {
		kind = value.Get("type").String()
	}
	if kind == "ping" || kind == "heartbeat" || kind == "keepalive" {
		return
	}
	if kind == "error" {
		// Responses may follow a provisional bare error with authoritative success.
		w.pendingError = true
		return
	}
	if kind == "response.failed" || kind == "response.incomplete" || kind == "response.cancelled" || kind == "response.canceled" || value.Get("error").Type > gjson.Null {
		w.failed = true
		w.end = time.Time{}
		return
	}
	responseTerminal := kind == "response.completed" || kind == "response.done"
	if responseTerminal {
		status := value.Get("response.status").String()
		if (status != "" && status != "completed") || value.Get("response.error").Type > gjson.Null {
			w.failed = true
			w.end = time.Time{}
			return
		}
		w.pendingError = false
	}
	terminal := responseTerminal || kind == "message_stop"
	branchTerminal := false
	// Antigravity uses one or two response envelopes around Gemini events.
	for depth := 0; depth < 2 && !value.Get("candidates").Exists() && value.Get("response").IsObject(); depth++ {
		value = value.Get("response")
		if value.Get("error").Type > gjson.Null {
			w.failed = true
			return
		}
	}
	formal := strings.HasPrefix(kind, "response.") || strings.HasPrefix(kind, "content_block_") || kind == "message_start" || kind == "message_delta" || kind == "message_stop"
	if choices := value.Get("choices"); choices.IsArray() && len(choices.Array()) > 0 {
		formal = true
		if w.choiceDone == nil {
			w.choiceDone = map[int]bool{}
		}
		for _, choice := range choices.Array() {
			index := int(choice.Get("index").Int())
			finish := choice.Get("finish_reason")
			w.choiceDone[index] = finish.Type == gjson.String && finish.String() != ""
		}
		terminal = true
		for _, done := range w.choiceDone {
			terminal = terminal && done
		}
		branchTerminal = terminal
		if !terminal {
			w.end = time.Time{}
		}
	}
	if candidates := value.Get("candidates"); candidates.IsArray() && len(candidates.Array()) > 0 {
		formal = true
		if w.candidateDone == nil {
			w.candidateDone = map[int]bool{}
		}
		for _, candidate := range candidates.Array() {
			index := int(candidate.Get("index").Int())
			finish := candidate.Get("finishReason").String()
			if finish != "" && finish != "STOP" && finish != "MAX_TOKENS" {
				w.failed = true
				w.end = time.Time{}
				return
			}
			w.candidateDone[index] = finish != ""
		}
		terminal = true
		for _, done := range w.candidateDone {
			terminal = terminal && done
		}
		branchTerminal = terminal
		if !terminal {
			w.end = time.Time{}
		}
	}
	if !formal {
		return
	}
	// A lone terminal/usage payload cannot reveal when output started.
	if w.first.IsZero() && !terminal {
		w.first = now
	}
	if terminal && !w.first.IsZero() && (w.end.IsZero() || branchTerminal) {
		w.end = now
	}
}

func (w *streamOutputWindow) durationMs() *int {
	if w == nil || w.failed || w.pendingError || w.first.IsZero() || w.end.IsZero() {
		return nil
	}
	ms := int(w.end.Sub(w.first).Milliseconds())
	if ms < 100 {
		return nil
	}
	return &ms
}

func observedStreamOutputDuration(c *gin.Context) *int {
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		return nil
	}
	return observer.outputWindow.durationMs()
}
