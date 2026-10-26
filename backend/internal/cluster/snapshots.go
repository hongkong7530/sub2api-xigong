package cluster

import (
	"context"
	"encoding/json"
	"fmt"
)

// Snapshots 在同一 Redis 往返读取节点观测；不缓存或延长观测的新鲜度。
// 缺失节点不补零，调用方仍须检查身份和 Fresh，避免不完整数据看起来健康。
func (r *Runtime) Snapshots(ctx context.Context, ids []string) (map[string]*Snapshot, error) {
	out := make(map[string]*Snapshot, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if r.redis == nil {
		return nil, ErrNotReady
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = r.snapshotKey(id)
	}
	values, err := r.redis.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for i, value := range values {
		if value == nil {
			continue
		}
		raw, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid cluster snapshot encoding")
		}
		var snapshot Snapshot
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			return nil, err
		}
		out[ids[i]] = &snapshot
	}
	return out, nil
}
