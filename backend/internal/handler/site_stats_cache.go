package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// 三秒快照由全站共享；聚合失败或另一节点刷新时只返回带陈旧标识的旧值。
func cachedPublicSiteUsage(ctx context.Context, client *redis.Client, key string, load func() (*service.SiteUsageSnapshot, error)) (*service.SiteUsageSnapshot, bool) {
	if client == nil {
		return nil, true
	}
	read := func(key string) *service.SiteUsageSnapshot {
		raw, err := client.Get(ctx, key).Bytes()
		if err != nil {
			return nil
		}
		var value service.SiteUsageSnapshot
		if json.Unmarshal(raw, &value) != nil {
			return nil
		}
		return &value
	}
	if fresh := read(key); fresh != nil {
		return fresh, false
	}
	stale := read(key + ":last")
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return stale, true
	}
	token := hex.EncodeToString(random[:])
	owner, err := client.SetNX(ctx, key+":lock", token, 10*time.Second).Result()
	if err != nil || !owner {
		return stale, true
	}
	defer func() {
		// 请求取消也要释放自己的锁，但不能删除超时后其他刷新者的新锁。
		release, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Eval(release, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end;return 0`, []string{key + ":lock"}, token).Err()
	}()
	// 锁竞争前另一读者可能已发布快照，取得锁后不重复扫描。
	if fresh := read(key); fresh != nil {
		return fresh, false
	}
	value, err := load()
	if err != nil || value == nil {
		return stale, true
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return stale, true
	}
	pipe := client.TxPipeline()
	pipe.Set(ctx, key, raw, 3*time.Second)
	pipe.Set(ctx, key+":last", raw, time.Minute)
	if _, err = pipe.Exec(ctx); err != nil {
		return value, true
	}
	return value, false
}
