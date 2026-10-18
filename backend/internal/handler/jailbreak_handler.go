// Package handler provides HTTP request handlers for the application.
package handler

import (
	"strconv"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// JailbreakHandler handles user-owned API key jailbreak configuration.
type JailbreakHandler struct {
	apiKeyService *service.APIKeyService
}

func NewJailbreakHandler(apiKeyService *service.APIKeyService) *JailbreakHandler {
	return &JailbreakHandler{apiKeyService: apiKeyService}
}

func (h *JailbreakHandler) GetConfig(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "Invalid key ID")
		return
	}
	config, err := h.apiKeyService.GetJailbreakConfig(c.Request.Context(), keyID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.JailbreakConfigFromService(config))
}

func (h *JailbreakHandler) UpdateConfig(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "Invalid key ID")
		return
	}
	var req struct {
		Enabled      *bool   `json:"enabled"`
		SystemPrompt *string `json:"system_prompt"`
		UserPrompt   *string `json:"user_prompt"`
		Template     *string `json:"template"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	// 与 service 层使用同一上限口径（Unicode 字符），避免按字节计把中文限额压到 1/3。
	if req.SystemPrompt != nil && utf8.RuneCountInString(*req.SystemPrompt) > service.JailbreakUserSystemPromptMaxRunes {
		response.BadRequest(c, "System prompt too long (max 20000 chars)")
		return
	}
	if req.UserPrompt != nil && utf8.RuneCountInString(*req.UserPrompt) > service.JailbreakUserUserPromptMaxRunes {
		response.BadRequest(c, "User prompt too long (max 20000 chars)")
		return
	}
	config, err := h.apiKeyService.UpdateJailbreakConfig(c.Request.Context(), keyID, subject.UserID, service.UpdateJailbreakRequest{
		Enabled: req.Enabled, SystemPrompt: req.SystemPrompt, UserPrompt: req.UserPrompt, Template: req.Template,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.JailbreakConfigFromService(config))
}

func (h *JailbreakHandler) ListTemplates(c *gin.Context) {
	templates, err := h.apiKeyService.ListAvailableJailbreakTemplates(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, templates)
}

func (h *JailbreakHandler) ClearConfig(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "Invalid key ID")
		return
	}
	disabled := false
	config, err := h.apiKeyService.UpdateJailbreakConfig(c.Request.Context(), keyID, subject.UserID, service.UpdateJailbreakRequest{Enabled: &disabled})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.JailbreakConfigFromService(config))
}
