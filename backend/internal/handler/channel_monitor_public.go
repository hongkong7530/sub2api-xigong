package handler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const publicMonitorRefreshSeconds = 5

// 匿名卡片使用显式白名单；用户视图今后新增配额、账号等字段时不能自动公开。
type publicMonitorCard struct {
	ID                           int64                              `json:"id"`
	Name                         string                             `json:"name"`
	Provider                     string                             `json:"provider"`
	GroupName                    string                             `json:"group_name"`
	GroupID                      *int64                             `json:"group_id"`
	PrimaryModel                 string                             `json:"primary_model"`
	PrimaryStatus                string                             `json:"primary_status"`
	StatusSource                 string                             `json:"status_source"`
	StatusObservedAt             *time.Time                         `json:"status_observed_at"`
	OverallStatus                string                             `json:"overall_status"`
	PrimaryLatencyMs             *int                               `json:"primary_latency_ms"`
	PrimaryFirstTokenMs          *int                               `json:"primary_first_token_ms"`
	PrimaryOutputTokensPerSecond *float64                           `json:"primary_output_tokens_per_second"`
	Availability7d               *float64                           `json:"availability_7d"`
	AvailabilitySamples7d        int                                `json:"availability_samples_7d"`
	ExtraModels                  []dto.ChannelMonitorExtraModelStatus `json:"extra_models"`
	Timeline                     []channelMonitorUserTimelinePoint  `json:"timeline"`
	GroupInsight                 *channelMonitorGroupInsightPayload `json:"group_insight,omitempty"`
}

type publicMonitorResponse struct {
	Enabled        bool                `json:"enabled"`
	Items          []publicMonitorCard `json:"items"`
	SampledAt      time.Time           `json:"sampled_at"`
	RefreshSeconds int                 `json:"refresh_seconds"`
}

func publicMonitorCardFromView(v *service.UserMonitorView) publicMonitorCard {
	item := userMonitorViewToItem(v, false)
	return publicMonitorCard{
		ID: item.ID, Name: item.Name, Provider: item.Provider,
		GroupName: item.GroupName, GroupID: item.GroupID,
		PrimaryModel: item.PrimaryModel, PrimaryStatus: item.PrimaryStatus,
		StatusSource: item.StatusSource, StatusObservedAt: item.StatusObservedAt,
		OverallStatus: item.OverallStatus, PrimaryLatencyMs: item.PrimaryLatencyMs,
		PrimaryFirstTokenMs: item.PrimaryFirstTokenMs,
		PrimaryOutputTokensPerSecond: item.PrimaryOutputTokensPerSecond,
		Availability7d: item.Availability7d, AvailabilitySamples7d: item.AvailabilitySamples7d,
		ExtraModels: item.ExtraModels, Timeline: item.Timeline, GroupInsight: item.GroupInsight,
	}
}

// 同一节点的匿名读者共用一个短快照，不随访客数重复扫描真实请求历史。
// 缓存只保存成功结果；失败时不伪装成最新成功或无限期公开旧卡片。
type publicMonitorCache struct {
	mu        sync.Mutex
	value     *publicMonitorResponse
	expiresAt time.Time
}

func (cache *publicMonitorCache) get(ctx context.Context, load func(context.Context) ([]*service.UserMonitorView, error)) (*publicMonitorResponse, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cache.value != nil && time.Now().Before(cache.expiresAt) {
		return cache.value, nil
	}
	views, err := load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]publicMonitorCard, 0, len(views))
	for _, view := range views {
		items = append(items, publicMonitorCardFromView(view))
	}
	cache.value = &publicMonitorResponse{
		Enabled: true, Items: items, SampledAt: time.Now().UTC(), RefreshSeconds: publicMonitorRefreshSeconds,
	}
	cache.expiresAt = time.Now().Add(publicMonitorRefreshSeconds * time.Second)
	return cache.value, nil
}

// PublicList 只读已启用渠道的真实聚合，不触发探测；关闭功能时先于缓存返回空列表。
func (h *ChannelMonitorUserHandler) PublicList(c *gin.Context) {
	if !h.featureEnabled(c) {
		response.Success(c, publicMonitorResponse{
			Enabled: false, Items: []publicMonitorCard{}, SampledAt: time.Now().UTC(), RefreshSeconds: publicMonitorRefreshSeconds,
		})
		return
	}
	value, err := h.publicCache.get(c.Request.Context(), h.monitorService.ListUserView)
	if err != nil {
		// 匿名错误不回传仓储或上游错误正文。
		response.Error(c, http.StatusServiceUnavailable, "Channel status temporarily unavailable")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, value)
}
