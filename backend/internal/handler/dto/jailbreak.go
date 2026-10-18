package dto

import "github.com/Wei-Shaw/sub2api/internal/service"

// JailbreakConfig DTO for API responses
type JailbreakConfig struct {
	Enabled      bool   `json:"enabled"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	UserPrompt   string `json:"user_prompt,omitempty"`
	Template     string `json:"template,omitempty"`
}

// JailbreakConfigFromService converts a service model to its API representation.
func JailbreakConfigFromService(config *service.JailbreakConfig) *JailbreakConfig {
	if config == nil {
		return nil
	}
	return &JailbreakConfig{
		Enabled:      config.Enabled,
		SystemPrompt: config.SystemPrompt,
		UserPrompt:   config.UserPrompt,
		Template:     config.Template,
	}
}
