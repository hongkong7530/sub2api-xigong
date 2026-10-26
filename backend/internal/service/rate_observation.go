package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ratemetric"
	"github.com/gin-gonic/gin"
)

// RateObservation 只保存本站接收分布与计时范围，不声称模型内部生成速度。
type RateObservation = ratemetric.Observation

func finishOpenAIRateObservation(ctx context.Context, result *OpenAIForwardResult, errors ...error) {
	if result != nil {
		result.RateObservation = ratemetric.ForRequest(ctx, result.RateObservation, time.Now())
		result.RateObservation = failedRateObservation(result.RateObservation, errors...)
	}
}
func finishGatewayRateObservation(ctx context.Context, result *ForwardResult, errors ...error) {
	if result != nil {
		result.RateObservation = ratemetric.ForRequest(ctx, result.RateObservation, time.Now())
		result.RateObservation = failedRateObservation(result.RateObservation, errors...)
	}
}
func finishWSTurnRateObservation(result *OpenAIForwardResult, start time.Time, errors ...error) {
	if result != nil {
		result.RateObservation = ratemetric.WithDuration(result.RateObservation, start, time.Now(), "ws_turn")
		result.RateObservation = failedRateObservation(result.RateObservation, errors...)
	}
}
func failedRateObservation(o *RateObservation, errors ...error) *RateObservation {
	for _, err := range errors {
		if err != nil && o != nil {
			copy := *o
			copy.Quality = "incomplete"
			return &copy
		}
	}
	return o
}
func observationContext(c *gin.Context, fallback context.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return fallback
}
