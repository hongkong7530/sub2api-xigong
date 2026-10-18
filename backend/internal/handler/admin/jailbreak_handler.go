package admin

import (
	"context"
	"errors"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminJailbreakHandler handles admin-level jailbreak management.
type AdminJailbreakHandler struct {
	apiKeyRepo service.APIKeyRepository
	settings   *service.SettingService
}

func NewAdminJailbreakHandler(apiKeyRepo service.APIKeyRepository, settings *service.SettingService) *AdminJailbreakHandler {
	return &AdminJailbreakHandler{apiKeyRepo: apiKeyRepo, settings: settings}
}

func (h *AdminJailbreakHandler) GetConfig(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "Invalid key ID")
		return
	}
	apiKey, err := h.apiKeyRepo.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.JailbreakConfigFromService(&service.JailbreakConfig{Enabled: apiKey.JailbreakEnabled, SystemPrompt: apiKey.JailbreakSystemPrompt, UserPrompt: apiKey.JailbreakUserPrompt, Template: apiKey.JailbreakTemplate}))
}

func (h *AdminJailbreakHandler) UpdateConfig(c *gin.Context) {
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
	if req.SystemPrompt != nil && len(*req.SystemPrompt) > 50000 {
		response.BadRequest(c, "System prompt too long (max 50000 chars)")
		return
	}
	if req.UserPrompt != nil && len(*req.UserPrompt) > 10000 {
		response.BadRequest(c, "User prompt too long (max 10000 chars)")
		return
	}
	apiKey, err := h.apiKeyRepo.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	changed := false
	if req.Enabled != nil {
		apiKey.JailbreakEnabled = *req.Enabled
		changed = true
	}
	if req.SystemPrompt != nil {
		apiKey.JailbreakSystemPrompt = *req.SystemPrompt
		changed = true
	}
	if req.UserPrompt != nil {
		apiKey.JailbreakUserPrompt = *req.UserPrompt
		changed = true
	}
	if req.Template != nil {
		apiKey.JailbreakTemplate = *req.Template
		changed = true
	}
	if changed {
		if err := h.apiKeyRepo.Update(c.Request.Context(), apiKey, service.APIKeyUpdateFields{Jailbreak: true}); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	response.Success(c, dto.JailbreakConfigFromService(&service.JailbreakConfig{Enabled: apiKey.JailbreakEnabled, SystemPrompt: apiKey.JailbreakSystemPrompt, UserPrompt: apiKey.JailbreakUserPrompt, Template: apiKey.JailbreakTemplate}))
}

func (h *AdminJailbreakHandler) ListUserKeys(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	keys, _, err := h.apiKeyRepo.ListByUserID(c.Request.Context(), userID, pagination.PaginationParams{PageSize: 1000}, service.APIKeyListFilters{})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]map[string]interface{}, 0)
	for _, key := range keys {
		if key.JailbreakEnabled {
			out = append(out, map[string]interface{}{"id": key.ID, "name": key.Name, "enabled": key.JailbreakEnabled, "template": key.JailbreakTemplate})
		}
	}
	response.Success(c, out)
}

func (h *AdminJailbreakHandler) DisableAllUserJailbreaks(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	keys, _, err := h.apiKeyRepo.ListByUserID(c.Request.Context(), userID, pagination.PaginationParams{PageSize: 1000}, service.APIKeyListFilters{})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	count := 0
	for i := range keys {
		if !keys[i].JailbreakEnabled {
			continue
		}
		keys[i].JailbreakEnabled = false
		if err := h.apiKeyRepo.Update(c.Request.Context(), &keys[i], service.APIKeyUpdateFields{Jailbreak: true}); err == nil {
			count++
		}
	}
	response.Success(c, map[string]interface{}{"message": "jailbreak disabled for user", "keys_updated": count})
}

func (h *AdminJailbreakHandler) AuditJailbreakUsage(c *gin.Context) {
	response.Success(c, map[string]string{"status": "not_implemented"})
}

type adminJailbreakTemplateItem struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Enabled      bool   `json:"enabled"`
	Source       string `json:"source"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	UserPrompt   string `json:"user_prompt,omitempty"`
	Shadowed     bool   `json:"shadowed,omitempty"`
}

type adminJailbreakTemplateCreateRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
	Enabled      *bool  `json:"enabled"`
}

type adminJailbreakTemplateUpdateRequest struct {
	Description  *string `json:"description"`
	SystemPrompt *string `json:"system_prompt"`
	UserPrompt   *string `json:"user_prompt"`
	Enabled      *bool   `json:"enabled"`
}

func (h *AdminJailbreakHandler) ListTemplates(c *gin.Context) { h.respondWithTemplateList(c) }

func (h *AdminJailbreakHandler) CreateTemplate(c *gin.Context) {
	var req adminJailbreakTemplateCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	record := service.JailbreakTemplateRecord{Name: req.Name, Description: req.Description, SystemPrompt: req.SystemPrompt, UserPrompt: req.UserPrompt, Enabled: enabled}
	if err := service.ValidateJailbreakTemplateRecord(&record); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	records, err := h.settings.GetJailbreakTemplateRecords(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if len(records) >= service.JailbreakTemplateMaxCount {
		response.BadRequest(c, "模板数量已达上限")
		return
	}
	for _, existing := range records {
		if existing.Name == record.Name {
			response.BadRequest(c, "同名模板已存在，请直接编辑或更换名称")
			return
		}
	}
	if err := h.settings.SaveJailbreakTemplateRecords(c.Request.Context(), append(records, record)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.respondWithTemplateList(c)
}

func (h *AdminJailbreakHandler) UpdateTemplate(c *gin.Context) {
	name := c.Param("name")
	var req adminJailbreakTemplateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	records, err := h.settings.GetJailbreakTemplateRecords(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	index := -1
	for i, record := range records {
		if record.Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		response.NotFound(c, "自定义模板不存在（内置模板请使用同名新建覆盖）")
		return
	}
	record := records[index]
	if req.Description != nil {
		record.Description = *req.Description
	}
	if req.SystemPrompt != nil {
		record.SystemPrompt = *req.SystemPrompt
	}
	if req.UserPrompt != nil {
		record.UserPrompt = *req.UserPrompt
	}
	if req.Enabled != nil {
		record.Enabled = *req.Enabled
	}
	if err := service.ValidateJailbreakTemplateRecord(&record); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	records[index] = record
	if err := h.settings.SaveJailbreakTemplateRecords(c.Request.Context(), records); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.respondWithTemplateList(c)
}

func (h *AdminJailbreakHandler) DeleteTemplate(c *gin.Context) {
	name := c.Param("name")
	records, err := h.settings.GetJailbreakTemplateRecords(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	filtered := make([]service.JailbreakTemplateRecord, 0, len(records))
	found := false
	for _, record := range records {
		if record.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, record)
	}
	if !found {
		response.NotFound(c, "自定义模板不存在")
		return
	}
	if err := h.settings.SaveJailbreakTemplateRecords(c.Request.Context(), filtered); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.respondWithTemplateList(c)
}

func (h *AdminJailbreakHandler) respondWithTemplateList(c *gin.Context) {
	items, err := h.buildJailbreakTemplateList(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *AdminJailbreakHandler) buildJailbreakTemplateList(ctx context.Context) ([]adminJailbreakTemplateItem, error) {
	if h.settings == nil {
		return nil, errors.New("setting service is unavailable")
	}
	records, err := h.settings.GetJailbreakTemplateRecords(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]adminJailbreakTemplateItem, 0, len(records)+4)
	shadowed := make(map[string]bool, len(records))
	for _, record := range records {
		shadowed[record.Name] = true
		items = append(items, adminJailbreakTemplateItem{Name: record.Name, Description: record.Description, Enabled: record.Enabled, Source: "custom", SystemPrompt: record.SystemPrompt, UserPrompt: record.UserPrompt})
	}
	for _, file := range service.ListJailbreakFileTemplates() {
		item := adminJailbreakTemplateItem{Name: file.Name, Description: file.Description, Enabled: true, Source: "builtin", Shadowed: shadowed[file.Name]}
		if content, err := service.LoadJailbreakFileTemplateContent(file.Name); err == nil && content != nil {
			item.SystemPrompt, item.UserPrompt = content.SystemPrompt, content.UserPrompt
		}
		items = append(items, item)
	}
	return items, nil
}
