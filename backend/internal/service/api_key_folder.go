package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	apiKeyFolderNameMaxRunes = 40
	apiKeyFolderMaxCount     = 30
)

// APIKeyFolder is a per-user display folder for organizing API keys.
type APIKeyFolder struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	KeyCount  int64     `json:"key_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type APIKeyFolderRepository interface {
	ListByUserID(ctx context.Context, userID int64) ([]APIKeyFolder, error)
	GetByID(ctx context.Context, id int64) (*APIKeyFolder, error)
	Create(ctx context.Context, folder *APIKeyFolder) error
	Update(ctx context.Context, folder *APIKeyFolder) error
	Delete(ctx context.Context, id int64) error
	CountByUserID(ctx context.Context, userID int64) (int, error)
	ExistsByName(ctx context.Context, userID int64, name string, excludeID int64) (bool, error)
}

type APIKeyFolderService struct {
	repo APIKeyFolderRepository
}

func NewAPIKeyFolderService(repo APIKeyFolderRepository) *APIKeyFolderService {
	return &APIKeyFolderService{repo: repo}
}

func NormalizeAPIKeyFolderName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", infraerrors.BadRequest("API_KEY_FOLDER_NAME_REQUIRED", "folder name is required")
	}
	if utf8.RuneCountInString(name) > apiKeyFolderNameMaxRunes {
		return "", infraerrors.BadRequest("API_KEY_FOLDER_NAME_TOO_LONG", "folder name exceeds 40 characters")
	}
	return name, nil
}

func (s *APIKeyFolderService) List(ctx context.Context, userID int64) ([]APIKeyFolder, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured")
	}
	items, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []APIKeyFolder{}, nil
	}
	return items, nil
}

func (s *APIKeyFolderService) Create(ctx context.Context, userID int64, name string) (*APIKeyFolder, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured")
	}
	normalized, err := NormalizeAPIKeyFolderName(name)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if count >= apiKeyFolderMaxCount {
		return nil, infraerrors.BadRequest("API_KEY_FOLDER_LIMIT", "at most 30 folders are allowed")
	}
	exists, err := s.repo.ExistsByName(ctx, userID, normalized, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, infraerrors.BadRequest("API_KEY_FOLDER_EXISTS", "folder name already exists")
	}
	item := &APIKeyFolder{UserID: userID, Name: normalized, SortOrder: count}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *APIKeyFolderService) Update(ctx context.Context, userID, id int64, name string) (*APIKeyFolder, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured")
	}
	item, err := s.requireOwned(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeAPIKeyFolderName(name)
	if err != nil {
		return nil, err
	}
	exists, err := s.repo.ExistsByName(ctx, userID, normalized, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, infraerrors.BadRequest("API_KEY_FOLDER_EXISTS", "folder name already exists")
	}
	item.Name = normalized
	if err := s.repo.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *APIKeyFolderService) Delete(ctx context.Context, userID, id int64) error {
	if s == nil || s.repo == nil {
		return infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured")
	}
	if _, err := s.requireOwned(ctx, userID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *APIKeyFolderService) ResolveForUser(ctx context.Context, userID int64, id *int64) (*int64, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	if *id < 0 {
		return nil, infraerrors.BadRequest("INVALID_FOLDER_ID", "invalid folder id")
	}
	if _, err := s.requireOwned(ctx, userID, *id); err != nil {
		return nil, err
	}
	return id, nil
}

func (s *APIKeyFolderService) requireOwned(ctx context.Context, userID, id int64) (*APIKeyFolder, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_FOLDER_ID", "invalid folder id")
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.UserID != userID {
		return nil, infraerrors.NotFound("API_KEY_FOLDER_NOT_FOUND", "folder not found")
	}
	return item, nil
}
