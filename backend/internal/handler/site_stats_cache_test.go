package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestSiteStatsThreeSecondSharedCache(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	ctx := context.Background()
	calls := 0
	load := func() (*service.SiteUsageSnapshot, error) {
		calls++
		return &service.SiteUsageSnapshot{Requests: int64(calls)}, nil
	}
	first, stale := cachedPublicSiteUsage(ctx, client, "test", load)
	require.False(t, stale)
	require.Equal(t, int64(1), first.Requests)
	require.False(t, server.Exists("test:lock"))
	second, stale := cachedPublicSiteUsage(ctx, client, "test", load)
	require.False(t, stale)
	require.Equal(t, first.Requests, second.Requests)
	require.Equal(t, 1, calls)
	server.FastForward(3 * time.Second)
	third, stale := cachedPublicSiteUsage(ctx, client, "test", load)
	require.False(t, stale)
	require.Equal(t, int64(2), third.Requests)
	require.Equal(t, 2, calls)
	server.FastForward(3 * time.Second)
	old, stale := cachedPublicSiteUsage(ctx, client, "test", func() (*service.SiteUsageSnapshot, error) { return nil, errors.New("offline") })
	require.True(t, stale)
	require.Equal(t, third.Requests, old.Requests)
	require.False(t, server.Exists("test:lock"))
	require.NoError(t, client.Set(ctx, "test:lock", "other", 10*time.Second).Err())
	old, stale = cachedPublicSiteUsage(ctx, client, "test", load)
	require.True(t, stale)
	require.Equal(t, third.Requests, old.Requests)
	require.Equal(t, 2, calls)
}

func TestSiteStatsCacheDoesNotReleaseAnotherOwner(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	ctx := context.Background()
	_, _ = cachedPublicSiteUsage(ctx, client, "test", func() (*service.SiteUsageSnapshot, error) {
		server.FastForward(11 * time.Second)
		require.NoError(t, client.Set(ctx, "test:lock", "replacement", 10*time.Second).Err())
		return nil, errors.New("expired")
	})
	require.Equal(t, "replacement", client.Get(ctx, "test:lock").Val())
}
