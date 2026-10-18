package handler

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var assetIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (h *SiteContentHandler) UploadAsset(c *gin.Context) {
	if !h.mediaPermit(c) {
		return
	}
	select {
	case h.mediaSlots <- struct{}{}:
		defer func() { <-h.mediaSlots }()
	default:
		response.Error(c, 429, "Upload busy; retry shortly")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.SiteAssetMaxBytes+(1<<20))
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Choose a JPEG or PNG up to 8 MiB")
		return
	}
	defer file.Close()
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	data, err := io.ReadAll(io.LimitReader(file, service.SiteAssetMaxBytes+1))
	if err != nil {
		response.BadRequest(c, "Unable to read image")
		return
	}
	value, err := h.svc.SaveAsset(c.Request.Context(), data)
	contentReply(c, gin.H{"url": value}, err)
}
func (h *SiteContentHandler) Asset(c *gin.Context) {
	id := c.Param("asset")
	if !assetIDPattern.MatchString(id) {
		response.NotFound(c, "Image not found")
		return
	}
	etag := "\"" + id + "\""
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	mime, body, err := h.svc.Repo.SiteAsset(c.Request.Context(), id)
	if err != nil {
		contentReply(c, nil, err)
		return
	}
	c.Data(200, mime, body)
}
func (h *SiteContentHandler) mediaPermit(c *gin.Context) bool {
	if h.redis == nil {
		response.Error(c, 503, "Media service unavailable")
		return false
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Authentication required")
		return false
	}
	v, err := h.redis.Eval(c.Request.Context(), `local n=redis.call('INCR',KEYS[1]);if n==1 then redis.call('EXPIRE',KEYS[1],60) end;return n`, []string{fmt.Sprintf("site:media:rate:%d", subject.UserID)}).Int()
	if err != nil {
		response.Error(c, 503, "Media service unavailable")
		return false
	}
	if v > 12 {
		response.Error(c, 429, "Too many image operations; retry in a minute")
		return false
	}
	return true
}
func (h *SiteContentHandler) ResolvePartner(c *gin.Context) {
	if !h.mediaPermit(c) {
		return
	}
	var in struct {
		URL string `json:"url"`
	}
	if !contentBind(c, &in) {
		return
	}
	select {
	case h.mediaSlots <- struct{}{}:
		defer func() { <-h.mediaSlots }()
	default:
		response.Error(c, 429, "Preview busy; retry shortly")
		return
	}
	preview, err := h.svc.ResolvePartner(c.Request.Context(), in.URL)
	contentReply(c, preview, err)
}
func (h *SiteContentHandler) Presence(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Authentication required")
		return
	}
	if h.redis == nil {
		response.Error(c, 503, "Presence unavailable")
		return
	}
	now := time.Now().Unix()
	_, err := h.redis.Eval(c.Request.Context(), `redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',ARGV[1]-300);redis.call('ZADD',KEYS[1],ARGV[1],ARGV[2]);redis.call('EXPIRE',KEYS[1],600);return 1`, []string{"site:presence:v1"}, now, strconv.FormatInt(subject.UserID, 10)).Result()
	contentReply(c, gin.H{"ok": err == nil}, err)
}
func (h *SiteContentHandler) SiteStats(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	now := time.Now()
	zone, err := time.LoadLocation(h.timezone)
	if err != nil {
		zone = time.UTC
	}
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	usage, usageStale := cachedPublicSiteUsage(ctx, h.redis, "site:public:usage:v2:"+start.Format("2006-01-02"), func() (*service.SiteUsageSnapshot, error) {
		u, err := h.svc.Repo.PublicSiteUsage(ctx, start, now)
		if err != nil {
			return nil, err
		}
		groups, err := h.plaza.ListGroups(ctx)
		if err != nil {
			return nil, err
		}
		allowed := map[string]bool{}
		for _, g := range groups {
			if !g.IsExclusive {
				for _, m := range g.Models {
					allowed[m.Name] = true
				}
			}
		}
		visible := []service.SiteModelUsage{}
		for _, m := range u.Models {
			if allowed[m.Model] {
				visible = append(visible, m)
			}
		}
		u.Models = visible
		return u, nil
	})
	online := (*int64)(nil)
	if h.redis != nil {
		n, e := h.redis.ZCount(ctx, "site:presence:v1", strconv.FormatInt(now.Unix()-300, 10), "+inf").Result()
		if e == nil {
			online = &n
		}
	}
	inFlight := (*int)(nil)
	if h.runtime != nil && h.runtime.Enabled() {
		nodes, e := h.runtime.Store.Nodes(ctx)
		if e == nil {
			total, complete, workers := 0, true, 0
			for _, n := range nodes {
				if !slices.Contains(n.Roles, "worker") || n.State == "retired" || n.State == "pending" {
					continue
				}
				workers++
				snap, e := h.runtime.Snapshot(ctx, n.ID)
				if e != nil || snap == nil || snap.NodeID != n.ID || !snap.Fresh(now) || snap.Active < 0 {
					complete = false
					continue
				}
				total += snap.Active
			}
			if complete && workers > 0 {
				inFlight = &total
			}
		}
	}
	if usage != nil && usage.AveragePerMillion != nil {
		v := math.Round(*usage.AveragePerMillion*100) / 100
		usage.AveragePerMillion = &v
	}
	// 模型广场关闭或要求登录时，首页榜单不能绕过同样的公开边界。
	if usage != nil {
		public, e := h.settings.GetPublicSettings(ctx)
		if e != nil || !public.ModelPlazaEnabled || public.ModelPlazaRequireAuth {
			copy := *usage
			copy.Models = []service.SiteModelUsage{}
			usage = &copy
		}
	}
	contentReply(c, gin.H{"usage": usage, "online_users": online, "in_flight": inFlight, "sampled_at": now, "online_window_seconds": 300, "refresh_seconds": 3, "usage_stale": usageStale}, nil)
}
