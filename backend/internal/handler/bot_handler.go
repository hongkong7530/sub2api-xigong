package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type BotHandler struct {
	svc   *service.BotService
	audit *service.AuditLogService
}

func NewBotHandler(svc *service.BotService, audit *service.AuditLogService) *BotHandler {
	return &BotHandler{svc: svc, audit: audit}
}

func (h *BotHandler) Create(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	var input service.BotPolicy
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&input); e != nil {
		response.ErrorFrom(c, service.ErrBotInvalid)
		return
	}
	if e := decoder.Decode(new(any)); e != io.EOF {
		response.ErrorFrom(c, service.ErrBotInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	v, e := h.svc.Create(ctx, input)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Created(c, v)
}
func (h *BotHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	v, e := h.svc.List(ctx)
	if e != nil {
		response.ErrorFrom(c, service.ErrBotUnavailable)
		return
	}
	response.Success(c, v)
}
func (h *BotHandler) Revoke(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if e := h.svc.Revoke(ctx, c.Param("id")); e != nil {
		if errors.Is(e, service.ErrBotInvalid) {
			response.ErrorFrom(c, e)
		} else {
			response.ErrorFrom(c, service.ErrBotUnavailable)
		}
		return
	}
	response.Success(c, map[string]bool{"revoked": true})
}

func (h *BotHandler) Read(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	began := time.Now()
	installation := ""
	defer func() {
		// No request query, body, token, signatures or raw chat identities enter bot audit records.
		h.audit.Record(&service.AuditLog{Action: "bot.read", AuthMethod: "astrbot_hmac", ActorRole: "bot:read", CredentialMasked: installation, Method: c.Request.Method, Path: c.FullPath(), StatusCode: c.Writer.Status(), LatencyMs: time.Since(began).Milliseconds()})
	}()
	if c.Request.Method != http.MethodGet {
		response.ErrorFrom(c, service.ErrBotForbidden)
		return
	}
	if c.Request.Body != nil {
		raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 1))
		if e != nil || len(raw) > 0 {
			response.ErrorFrom(c, service.ErrBotInvalid)
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	v, e := h.svc.Authenticate(ctx, c.Request)
	if e != nil {
		if errors.Is(e, service.ErrBotRateLimit) {
			c.Header("Retry-After", "60")
		}
		response.ErrorFrom(c, e)
		return
	}
	installation = v.ID
	route := strings.TrimPrefix(c.Param("path"), "/")
	data, e := h.svc.Query(ctx, route, c.Request.URL.RawQuery, v)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, data)
}
