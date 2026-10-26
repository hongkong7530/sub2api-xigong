package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SiteProfitHandler struct{ svc *service.SiteProfitService }

func NewSiteProfitHandler(svc *service.SiteProfitService) *SiteProfitHandler {
	return &SiteProfitHandler{svc: svc}
}
func (h *SiteProfitHandler) Disclosure(c *gin.Context) {
	v, e := h.svc.Snapshot(c.Request.Context(), false)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	raw, e := json.Marshal(v)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	sum := sha256.Sum256(raw)
	tag := `"` + hex.EncodeToString(sum[:]) + `"`
	c.Header("Cache-Control", "private, no-cache")
	c.Header("ETag", tag)
	if c.GetHeader("If-None-Match") == tag {
		c.Status(http.StatusNotModified)
		return
	}
	response.Success(c, v)
}
func (h *SiteProfitHandler) Overview(c *gin.Context) {
	v, e := h.svc.Snapshot(c.Request.Context(), true)
	profitReply(c, v, e)
}
func (h *SiteProfitHandler) Contribution(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	v, e := h.svc.Repo.Contribution(c.Request.Context(), subject.UserID)
	profitReply(c, v, e)
}
func (h *SiteProfitHandler) SetCost(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	group, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || group <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	var input struct {
		Multiplier json.RawMessage `json:"multiplier"`
		Version    *int64          `json:"version"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if e = c.ShouldBindJSON(&input); e != nil || input.Version == nil || len(input.Multiplier) == 0 {
		response.BadRequest(c, "Multiplier and expected version are required")
		return
	}
	var multiplier *string
	if e = json.Unmarshal(input.Multiplier, &multiplier); e != nil {
		response.BadRequest(c, "Multiplier must be a decimal string or null")
		return
	}
	v, e := h.svc.SetCost(c.Request.Context(), group, subject.UserID, service.ProfitCostInput{Multiplier: multiplier, Version: *input.Version})
	profitReply(c, v, e)
}
func (h *SiteProfitHandler) Entries(c *gin.Context) {
	f := service.ProfitEntryFilter{Page: 1}
	var e error
	if p := c.Query("page"); p != "" {
		f.Page, e = strconv.Atoi(p)
		if e != nil || f.Page < 1 || f.Page > 10000 {
			response.BadRequest(c, "Invalid page")
			return
		}
	}
	if g := c.Query("group_id"); g != "" {
		f.GroupID, e = strconv.ParseInt(g, 10, 64)
		if e != nil || f.GroupID < 0 {
			response.BadRequest(c, "Invalid group ID")
			return
		}
	}
	for _, p := range []struct {
		key    string
		target **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if raw := c.Query(p.key); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				response.BadRequest(c, "Invalid time filter")
				return
			}
			*p.target = &t
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		response.BadRequest(c, "Invalid time range")
		return
	}
	v, e := h.svc.Repo.Entries(c.Request.Context(), f)
	profitReply(c, v, e)
}
func profitReply(c *gin.Context, v any, e error) {
	c.Header("Cache-Control", "no-store")
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, v)
}
