package service

import (
	"context"
	"fmt"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// CreateJailbreakRequest request to create/configure jailbreak for an API key
type CreateJailbreakRequest struct {
	Enabled      bool   `json:"enabled"`
	SystemPrompt string `json:"system_prompt,omitempty"` // Custom system prompt (highest priority)
	UserPrompt   string `json:"user_prompt,omitempty"`   // Custom user prompt
	Template     string `json:"template,omitempty"`      // Predefined template name
}

// 用户侧破甲提示词长度上限（按 Unicode 字符计）。
// 与管理员模板上限保持一致：提示词是整段可复用的破甲词，1000 字会截断正常内容。
const (
	JailbreakUserSystemPromptMaxRunes = 20000
	JailbreakUserUserPromptMaxRunes   = 20000
)

// ErrJailbreakPromptTooLong 提示词超长时返回，消息中带上限便于前端直接展示。
var ErrJailbreakPromptTooLong = infraerrors.BadRequest("JAILBREAK_PROMPT_TOO_LONG", "prompt too long (max 20000 chars)")

// UpdateJailbreakRequest request to update jailbreak configuration
type UpdateJailbreakRequest struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	SystemPrompt *string `json:"system_prompt,omitempty"`
	UserPrompt   *string `json:"user_prompt,omitempty"`
	Template     *string `json:"template,omitempty"`
}

// JailbreakConfig holds jailbreak configuration state
type JailbreakConfig struct {
	Enabled      bool   `json:"enabled"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	UserPrompt   string `json:"user_prompt,omitempty"`
	Template     string `json:"template,omitempty"`
}

// GetJailbreakConfig retrieves jailbreak configuration for an API key
func (s *APIKeyService) GetJailbreakConfig(ctx context.Context, keyID int64, userID int64) (*JailbreakConfig, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}

	// Ownership verification
	if apiKey.UserID != userID {
		return nil, ErrInsufficientPerms
	}

	return &JailbreakConfig{
		Enabled:      apiKey.JailbreakEnabled,
		SystemPrompt: apiKey.JailbreakSystemPrompt,
		UserPrompt:   apiKey.JailbreakUserPrompt,
		Template:     apiKey.JailbreakTemplate,
	}, nil
}

// UpdateJailbreakConfig updates jailbreak settings for an API key
func (s *APIKeyService) UpdateJailbreakConfig(ctx context.Context, keyID int64, userID int64, req UpdateJailbreakRequest) (*JailbreakConfig, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}

	// Ownership verification
	if apiKey.UserID != userID {
		return nil, ErrInsufficientPerms
	}

	// Validation: template must exist if specified
	if req.Template != nil && *req.Template != "" {
		if _, err := LoadJailbreakTemplate(*req.Template); err != nil {
			return nil, fmt.Errorf("invalid jailbreak template: %w", err)
		}
	}
	// 长度按 Unicode 字符计：按字节计会把中文提示词限额压到 1/3。
	if req.SystemPrompt != nil && utf8.RuneCountInString(*req.SystemPrompt) > JailbreakUserSystemPromptMaxRunes {
		return nil, ErrJailbreakPromptTooLong
	}
	if req.UserPrompt != nil && utf8.RuneCountInString(*req.UserPrompt) > JailbreakUserUserPromptMaxRunes {
		return nil, ErrJailbreakPromptTooLong
	}

	// Update fields
	fields := APIKeyUpdateFields{}
	hasChanges := false

	if req.Enabled != nil {
		apiKey.JailbreakEnabled = *req.Enabled
		hasChanges = true
	}
	if req.SystemPrompt != nil {
		apiKey.JailbreakSystemPrompt = *req.SystemPrompt
		hasChanges = true
	}
	if req.UserPrompt != nil {
		apiKey.JailbreakUserPrompt = *req.UserPrompt
		hasChanges = true
	}
	if req.Template != nil {
		apiKey.JailbreakTemplate = *req.Template
		hasChanges = true
	}

	if hasChanges {
		fields.Jailbreak = true
		if err := s.apiKeyRepo.Update(ctx, apiKey, fields); err != nil {
			return nil, fmt.Errorf("update jailbreak config: %w", err)
		}
		// Invalidate auth cache
		if s.cache != nil {
			s.cache.DeleteAuthCache(ctx, apiKey.Key)
		}
	}

	return &JailbreakConfig{
		Enabled:      apiKey.JailbreakEnabled,
		SystemPrompt: apiKey.JailbreakSystemPrompt,
		UserPrompt:   apiKey.JailbreakUserPrompt,
		Template:     apiKey.JailbreakTemplate,
	}, nil
}

// ListAvailableJailbreakTemplates 返回用户可选的模板：管理员维护的（启用中）+ 未被覆盖的内置文件模板。
func (s *APIKeyService) ListAvailableJailbreakTemplates(ctx context.Context) ([]map[string]string, error) {
	if err := maybeRefreshManagedJailbreakTemplates(ctx); err != nil {
		return nil, fmt.Errorf("list managed jailbreak templates: %w", err)
	}

	managed := managedJailbreakTemplateRecords()
	shadowed := make(map[string]bool, len(managed))
	templates := make([]map[string]string, 0, len(managed)+4)
	for _, record := range managed {
		// 停用模板不再对用户展示，但已配置的密钥仍按原内容注入。
		shadowed[record.Name] = true
		if !record.Enabled {
			continue
		}
		templates = append(templates, map[string]string{
			"name":        record.Name,
			"description": record.Description,
			"source":      "custom",
		})
	}
	for _, file := range ListJailbreakFileTemplates() {
		if shadowed[file.Name] {
			continue
		}
		templates = append(templates, map[string]string{
			"name":        file.Name,
			"description": file.Description,
			"source":      "builtin",
		})
	}
	return templates, nil
}
