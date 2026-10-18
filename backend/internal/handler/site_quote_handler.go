package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *SiteContentHandler) Quote(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	// 公共语句池独立缓存，不把用户信息或凭据发送到外部服务。
	if h.redis != nil {
		raw, _ := h.redis.Get(ctx, "site:quote:pool:v1").Bytes()
		var pool []map[string]string
		_ = json.Unmarshal(raw, &pool)
		if len(pool) == 0 {
			owner, e := h.redis.SetNX(ctx, "site:quote:fetch-lock:v1", "1", 10*time.Second).Result()
			if e != nil || !owner {
				response.Success(c, nil)
				return
			}
			q, err := h.svc.FetchQuote(ctx)
			if err == nil {
				pool = append(pool, q)
				raw, _ = json.Marshal(pool)
				_ = h.redis.Set(ctx, "site:quote:pool:v1", raw, 10*time.Second).Err()
			}
		}
		if len(pool) > 0 {
			response.Success(c, pool[time.Now().UnixNano()%int64(len(pool))])
			return
		}
	}
	response.Success(c, nil)
}
