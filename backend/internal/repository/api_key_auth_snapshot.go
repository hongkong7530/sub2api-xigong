package repository

import (
	"context"
	"encoding/json"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 集群鉴权必须逐请求读取权威状态；关联预加载不能变成多次跨区往返。
// 同一语句的快照同时覆盖 Key、用户、绑定分组和授权，仍返回原有完整字段。
const apiKeyAuthSnapshotQuery = `
SELECT to_jsonb(k), to_jsonb(u), to_jsonb(g),
       COALESCE((SELECT jsonb_agg(allowed.id ORDER BY allowed.id)
                 FROM user_allowed_groups ug
                 JOIN groups allowed ON allowed.id = ug.group_id AND allowed.deleted_at IS NULL
                 WHERE ug.user_id = u.id), '[]'::jsonb)
FROM api_keys k
LEFT JOIN users u ON u.id = k.user_id AND u.deleted_at IS NULL
LEFT JOIN groups g ON g.id = k.group_id AND g.deleted_at IS NULL
WHERE k.key = $1 AND k.deleted_at IS NULL`

func (r *apiKeyRepository) getByKeyPostgres(ctx context.Context, key string) (*service.APIKey, error) {
	rows, err := r.sql.QueryContext(ctx, apiKeyAuthSnapshotQuery, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAPIKeyNotFound
	}
	var keyJSON, userJSON, groupJSON, allowedJSON []byte
	if err := rows.Scan(&keyJSON, &userJSON, &groupJSON, &allowedJSON); err != nil {
		return nil, err
	}
	var entity dbent.APIKey
	if err := json.Unmarshal(keyJSON, &entity); err != nil {
		return nil, fmt.Errorf("decode authoritative API key: %w", err)
	}
	// LEFT JOIN 的空关联保持 nil，由现有鉴权边界拒绝，不伪造有效用户或分组。
	if len(userJSON) != 0 {
		if err := json.Unmarshal(userJSON, &entity.Edges.User); err != nil {
			return nil, fmt.Errorf("decode authoritative API key user: %w", err)
		}
	}
	if len(groupJSON) != 0 {
		if err := json.Unmarshal(groupJSON, &entity.Edges.Group); err != nil {
			return nil, fmt.Errorf("decode authoritative API key group: %w", err)
		}
	}
	var allowedIDs []int64
	if err := json.Unmarshal(allowedJSON, &allowedIDs); err != nil {
		return nil, fmt.Errorf("decode authoritative API key permissions: %w", err)
	}
	if entity.Edges.User != nil {
		for _, id := range allowedIDs {
			entity.Edges.User.Edges.AllowedGroups = append(entity.Edges.User.Edges.AllowedGroups, &dbent.Group{ID: id})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return apiKeyEntityToService(&entity), nil
}
