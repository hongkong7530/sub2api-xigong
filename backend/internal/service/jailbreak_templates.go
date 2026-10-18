package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// SettingKeyJailbreakTemplates 保存管理员维护的破甲模板（JSON 数组）。
// 与内置文件模板（破甲资源/*.txt）并存：同名时管理员版本优先。
const SettingKeyJailbreakTemplates = "jailbreak_templates"

const (
	JailbreakTemplateMaxCount           = 50
	jailbreakTemplateDescriptionMaxLen  = 200
	jailbreakTemplateSystemPromptMaxLen = 100000
	jailbreakTemplateUserPromptMaxLen   = 20000
	// 模板正文（含系统提示词）单条上限 20000 字：模板是给用户选的整段提示词，
	// 1000 字上限会把正常长模板截断，故统一放宽。
	jailbreakTemplateContentMaxLen   = 20000
	jailbreakTemplateRefreshInterval = 30 * time.Second
)

var jailbreakTemplateNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)

// JailbreakTemplateRecord 管理员维护的模板条目。
type JailbreakTemplateRecord struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt,omitempty"`
	Enabled      bool   `json:"enabled"`
}

// JailbreakFileTemplateInfo 内置文件模板摘要。
type JailbreakFileTemplateInfo struct {
	Name        string
	Description string
}

// 内存注册表：网关注入与用户侧校验都读它，避免每请求查库；
// 管理员写入后立即刷新，跨节点最迟 30 秒对齐。
var (
	managedJailbreakMu         sync.RWMutex
	managedJailbreakTemplates  = map[string]JailbreakTemplateRecord{}
	managedJailbreakLoaded     bool
	managedJailbreakRefreshed  time.Time
	managedJailbreakInFlight   chan struct{}
	managedJailbreakAttempted  time.Time
	managedJailbreakLastError  error
	managedJailbreakGeneration uint64
	managedJailbreakRefresher  func(context.Context) error
)

// RegisterJailbreakTemplateRefresher 由 SettingService 初始化时注册。
func RegisterJailbreakTemplateRefresher(fn func(context.Context) error) {
	managedJailbreakMu.Lock()
	managedJailbreakRefresher = fn
	managedJailbreakMu.Unlock()
}

func applyManagedJailbreakTemplates(records []JailbreakTemplateRecord) {
	applyManagedJailbreakTemplateVersion(records, nil)
}

// 保存会推进版本；早于该保存开始的查询不能回写旧快照。
func applyManagedJailbreakTemplateVersion(records []JailbreakTemplateRecord, expected *uint64) {
	next := make(map[string]JailbreakTemplateRecord, len(records))
	for _, record := range records {
		name := strings.TrimSpace(record.Name)
		if name == "" {
			continue
		}
		next[name] = record
	}
	managedJailbreakMu.Lock()
	defer managedJailbreakMu.Unlock()
	if expected != nil && *expected != managedJailbreakGeneration {
		return
	}
	managedJailbreakTemplates = next
	managedJailbreakLoaded = true
	managedJailbreakRefreshed = time.Now()
	managedJailbreakLastError = nil
	managedJailbreakGeneration++
}

// managedJailbreakTemplateContent 返回管理员模板内容（含停用条目；停用只影响用户侧展示）。
func managedJailbreakTemplateContent(name string) (*JailbreakTemplate, bool) {
	managedJailbreakMu.RLock()
	record, ok := managedJailbreakTemplates[name]
	managedJailbreakMu.RUnlock()
	if !ok {
		return nil, false
	}
	return &JailbreakTemplate{
		Name:         record.Name,
		SystemPrompt: record.SystemPrompt,
		UserPrompt:   record.UserPrompt,
	}, true
}

// 冷启动共享同次读取结果并允许取消；有成功快照时异步刷新，不阻塞网关。
// 失败在短冷却内共享，不让排队请求逐个消耗一次数据库超时。
func maybeRefreshManagedJailbreakTemplates(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	managedJailbreakMu.Lock()
	refresher := managedJailbreakRefresher
	if refresher == nil || (managedJailbreakLoaded && time.Since(managedJailbreakRefreshed) < jailbreakTemplateRefreshInterval) {
		managedJailbreakMu.Unlock()
		return nil
	}
	if managedJailbreakInFlight == nil && time.Since(managedJailbreakAttempted) < 5*time.Second {
		err := managedJailbreakLastError
		if managedJailbreakLoaded {
			err = nil
		}
		managedJailbreakMu.Unlock()
		return err
	}
	done := managedJailbreakInFlight
	if done == nil {
		done = make(chan struct{})
		managedJailbreakInFlight = done
		managedJailbreakAttempted = time.Now()
		go func() {
			readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer readCancel()
			err := refresher(readCtx)
			managedJailbreakMu.Lock()
			managedJailbreakLastError = err
			managedJailbreakAttempted = time.Now()
			managedJailbreakInFlight = nil
			close(done)
			managedJailbreakMu.Unlock()
		}()
	}
	loaded := managedJailbreakLoaded
	managedJailbreakMu.Unlock()
	if loaded {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		managedJailbreakMu.RLock()
		err := managedJailbreakLastError
		if managedJailbreakLoaded {
			err = nil
		}
		managedJailbreakMu.RUnlock()
		return err
	}
}

// managedJailbreakTemplateRecords 返回注册表快照（按名称排序）。
func managedJailbreakTemplateRecords() []JailbreakTemplateRecord {
	managedJailbreakMu.RLock()
	out := make([]JailbreakTemplateRecord, 0, len(managedJailbreakTemplates))
	for _, record := range managedJailbreakTemplates {
		out = append(out, record)
	}
	managedJailbreakMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GetJailbreakTemplateRecords 读取管理员模板列表；未配置时返回空列表。
func (s *SettingService) GetJailbreakTemplateRecords(ctx context.Context) ([]JailbreakTemplateRecord, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository is unavailable")
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyJailbreakTemplates)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []JailbreakTemplateRecord{}, nil
		}
		return nil, fmt.Errorf("get jailbreak templates: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return []JailbreakTemplateRecord{}, nil
	}
	var records []JailbreakTemplateRecord
	if err := json.Unmarshal([]byte(value), &records); err != nil {
		return nil, fmt.Errorf("parse jailbreak templates: %w", err)
	}
	return records, nil
}

// SaveJailbreakTemplateRecords 持久化并立即刷新内存注册表。
func (s *SettingService) SaveJailbreakTemplateRecords(ctx context.Context, records []JailbreakTemplateRecord) error {
	if s == nil || s.settingRepo == nil {
		return errors.New("setting repository is unavailable")
	}
	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal jailbreak templates: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyJailbreakTemplates, string(data)); err != nil {
		return fmt.Errorf("save jailbreak templates: %w", err)
	}
	applyManagedJailbreakTemplates(records)
	return nil
}

// RefreshManagedJailbreakTemplates 从 settings 拉取模板并刷新注册表。
func (s *SettingService) RefreshManagedJailbreakTemplates(ctx context.Context) error {
	managedJailbreakMu.RLock()
	generation := managedJailbreakGeneration
	managedJailbreakMu.RUnlock()
	records, err := s.GetJailbreakTemplateRecords(ctx)
	if err != nil {
		return err
	}
	applyManagedJailbreakTemplateVersion(records, &generation)
	return nil
}

// ValidateJailbreakTemplateRecord 校验模板字段并原地规整。
func ValidateJailbreakTemplateRecord(record *JailbreakTemplateRecord) error {
	if record == nil {
		return infraerrors.BadRequest("INVALID_JAILBREAK_TEMPLATE", "template is required")
	}
	record.Name = strings.TrimSpace(record.Name)
	if !jailbreakTemplateNamePattern.MatchString(record.Name) {
		return infraerrors.BadRequest("INVALID_JAILBREAK_TEMPLATE_NAME", "模板名称只能包含字母、数字、下划线、连字符，长度 1-50")
	}
	record.Description = strings.TrimSpace(record.Description)
	if len([]rune(record.Description)) > jailbreakTemplateDescriptionMaxLen {
		return infraerrors.BadRequest("INVALID_JAILBREAK_TEMPLATE_DESCRIPTION", "模板描述过长")
	}
	if strings.TrimSpace(record.SystemPrompt) == "" && strings.TrimSpace(record.UserPrompt) == "" {
		return infraerrors.BadRequest("INVALID_JAILBREAK_TEMPLATE_PROMPT", "系统提示词与用户提示词不能同时为空")
	}
	if len([]rune(record.SystemPrompt)) > jailbreakTemplateSystemPromptMaxLen {
		return infraerrors.BadRequest("INVALID_JAILBREAK_TEMPLATE_SYSTEM_PROMPT", "系统提示词过长")
	}
	if len([]rune(record.UserPrompt)) > jailbreakTemplateUserPromptMaxLen {
		return infraerrors.BadRequest("INVALID_JAILBREAK_TEMPLATE_USER_PROMPT", "用户提示词过长")
	}
	return nil
}

// ListJailbreakFileTemplates 列出内置文件模板。
func ListJailbreakFileTemplates() []JailbreakFileTemplateInfo {
	out := []JailbreakFileTemplateInfo{}
	seen := map[string]bool{}
	for _, dir := range jailbreakTemplateDirCandidates() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".txt")
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, JailbreakFileTemplateInfo{Name: name, Description: "内置文件模板：" + entry.Name()})
		}
		break
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LoadJailbreakFileTemplateContent 读取内置文件模板内容（忽略管理员覆盖）。
func LoadJailbreakFileTemplateContent(name string) (*JailbreakTemplate, error) {
	return loadJailbreakFileTemplate(name)
}
