package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCacheRateWindowFromQueryUsesDefaultAndExactAllowlist(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		want     time.Duration
		valid    bool
	}{
		{raw: "", want: 30 * time.Minute, valid: true},
		{raw: "1800", want: 30 * time.Minute, valid: true},
		{raw: "3600", want: time.Hour, valid: true},
		{raw: "21600", want: 6 * time.Hour, valid: true},
		{raw: "86400", want: 24 * time.Hour, valid: true},
		{raw: "1801", valid: false},
		{raw: "9223372036854775807", valid: false},
		{raw: "30m", valid: false},
	} {
		got, valid := cacheRateWindowFromQuery(tc.raw)
		require.Equal(t, tc.valid, valid, "raw=%q", tc.raw)
		if valid {
			require.Equal(t, tc.want, got, "raw=%q", tc.raw)
		}
	}
}

func TestPublicMonitorCardWhitelistPreservesUnknownAndLatestFailure(t *testing.T) {
	gid := int64(7)
	observed := time.Now().UTC()
	view := &service.UserMonitorView{
		ID: 1, Name: "Public channel", Provider: "openai", GroupID: &gid,
		PrimaryModel: "", PrimaryStatus: "failed", OverallStatus: "failed",
		StatusSource: "user_request", StatusObservedAt: &observed,
		LatestQuota: &domain.MonitorQuotaSnapshot{},
		GroupInsight: &service.UserGroupInsight{EffectiveRateMultiplier: 1.25, RecentFirstTokens: []*service.ChannelMonitorGroupFirstToken{{ID: 987654, Model: "public-model", CreatedAt: observed}}},
		Timeline: []service.UserMonitorTimelinePoint{{Model: "", Source: "user_request", Status: "failed", CheckedAt: observed}},
	}
	raw, err := json.Marshal(publicMonitorCardFromView(view))
	require.NoError(t, err)
	var card map[string]any
	require.NoError(t, json.Unmarshal(raw, &card))
	require.Equal(t, "failed", card["primary_status"])
	require.Equal(t, "", card["primary_model"])
	require.Nil(t, card["availability_7d"])
	require.Nil(t, card["primary_first_token_ms"])
	require.Nil(t, card["primary_output_tokens_per_second"])
	for _, field := range []string{"latest_quota", "api_key", "base_url", "account_id", "user_id", "message", "primary_ping_latency_ms"} {
		require.NotContains(t, string(raw), `"`+field+`"`)
	}
	require.NotContains(t, string(raw), "987654")
	require.Equal(t, float64(1.25), card["group_insight"].(map[string]any)["effective_rate_multiplier"])
	require.Nil(t, card["group_insight"].(map[string]any)["cache_hit_rate"])
}

func TestPublicMonitorCacheCoalescesReadersAndRetriesFailures(t *testing.T) {
	var cache publicMonitorCache
	var calls atomic.Int32
	load := func(context.Context) ([]*service.UserMonitorView, error) {
		calls.Add(1)
		return []*service.UserMonitorView{{ID: 1, PrimaryStatus: "failed"}}, nil
	}
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			value, err := cache.get(context.Background(), load)
			if err != nil || len(value.Items) != 1 {
				t.Error("public reader failed")
			}
		}()
	}
	workers.Wait()
	require.Equal(t, int32(1), calls.Load())
	cache.expiresAt = time.Time{}
	_, err := cache.get(context.Background(), func(context.Context) ([]*service.UserMonitorView, error) { return nil, errors.New("unavailable") })
	require.Error(t, err)
	value, err := cache.get(context.Background(), load)
	require.NoError(t, err)
	require.Equal(t, int32(2), calls.Load())
	require.Equal(t, "failed", value.Items[0].PrimaryStatus)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = cache.get(ctx, load)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPublicMonitorListWorksWithoutAuthAndReturnsOnlyCachedProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewChannelMonitorUserHandler(service.NewChannelMonitorService(nil, nil), nil)
	_, err := h.publicCache.get(context.Background(), func(context.Context) ([]*service.UserMonitorView, error) {
		return []*service.UserMonitorView{{ID: 1, Name: "Visible anonymously", PrimaryStatus: "operational", Availability7d: 100, AvailabilitySamples7d: 2}}, nil
	})
	require.NoError(t, err)
	router := gin.New()
	router.GET("/api/v1/site-content/channel-monitors", h.PublicList)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v1/site-content/channel-monitors", nil))
	require.Equal(t, 200, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Contains(t, recorder.Body.String(), "Visible anonymously")
	require.NotContains(t, recorder.Body.String(), "latest_quota")
}
