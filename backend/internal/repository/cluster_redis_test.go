package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestClusterRedisGenerationAtomicAdmission(t *testing.T) {
	socket := os.Getenv("SUB2API_CLUSTER_REDIS_SOCKET")
	if socket == "" {
		t.Skip("isolated Unix Redis socket required")
	}
	rdb := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gate := "cluster:v3:integration:admission-generation"
	cache := newConcurrencyCacheWithTTL(rdb, time.Minute, 60).(*concurrencyCache)
	cache.clusterGateKey = gate
	_, err := cache.AcquireAccountSlot(ctx, 990001, 1, "new-process")
	require.Error(t, err)
	info, err := rdb.Info(ctx, "server").Result()
	require.NoError(t, err)
	generation := ""
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "run_id:") {
			generation = strings.TrimSpace(strings.TrimPrefix(line, "run_id:"))
		}
	}
	require.NotEmpty(t, generation)
	require.NoError(t, rdb.Set(ctx, gate, generation, time.Minute).Err())
	ok, err := cache.AcquireAccountSlot(ctx, 990001, 1, "first")
	require.NoError(t, err)
	require.True(t, ok)
	// Simulate a persisted gate from a previous Redis process: the Lua check must reject it atomically.
	require.NoError(t, rdb.Set(ctx, gate, strings.Repeat("0", 40), time.Minute).Err())
	_, err = cache.AcquireAccountSlot(ctx, 990002, 10, "stale-generation")
	require.Error(t, err)
	require.NoError(t, rdb.Del(ctx, gate).Err())
	_, err = cache.AcquireUserSlot(ctx, 990001, 10, "missing-generation")
	require.Error(t, err)
	_, err = cache.AcquireOpenAIWSIngressLease(ctx, 990001, 10, "missing-generation")
	require.Error(t, err)
	_, err = cache.AcquireLiveLease(ctx, 990001, 10, 990001, 10, 990001, "missing-generation", false)
	require.Error(t, err)
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 990001, "first"))
}
