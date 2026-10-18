package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	groupCategoryNameMaxRunes = 40
	groupCategoryMaxCount     = 50
)

// GroupCategory is an admin-managed catalog used only to organize routing groups in pickers.
type GroupCategory struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GroupCategoryRepository interface {
	List(ctx context.Context) ([]GroupCategory, error)
	GetByID(ctx context.Context, id int64) (*GroupCategory, error)
	Create(ctx context.Context, category *GroupCategory) error
	Update(ctx context.Context, category *GroupCategory) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int, error)
	ExistsByName(ctx context.Context, name string, excludeID int64) (bool, error)
	UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error
}

type GroupCategoryService struct {
	repo GroupCategoryRepository
}

func NewGroupCategoryService(repo GroupCategoryRepository) *GroupCategoryService {
	return &GroupCategoryService{repo: repo}
}

func NormalizeGroupCategoryName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", infraerrors.BadRequest("GROUP_CATEGORY_NAME_REQUIRED", "category name is required")
	}
	if utf8.RuneCountInString(name) > groupCategoryNameMaxRunes {
		return "", infraerrors.BadRequest("GROUP_CATEGORY_NAME_TOO_LONG", "category name exceeds 40 characters")
	}
	return name, nil
}

func (s *GroupCategoryService) List(ctx context.Context) ([]GroupCategory, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured")
	}
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []GroupCategory{}, nil
	}
	return items, nil
}

func (s *GroupCategoryService) Create(ctx context.Context, name string) (*GroupCategory, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured")
	}
	normalized, err := NormalizeGroupCategoryName(name)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count >= groupCategoryMaxCount {
		return nil, infraerrors.BadRequest("GROUP_CATEGORY_LIMIT", "at most 50 categories are allowed")
	}
	exists, err := s.repo.ExistsByName(ctx, normalized, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, infraerrors.BadRequest("GROUP_CATEGORY_EXISTS", "category name already exists")
	}
	item := &GroupCategory{Name: normalized, SortOrder: count}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *GroupCategoryService) Update(ctx context.Context, id int64, name string) (*GroupCategory, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured")
	}
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CATEGORY_ID", "invalid category id")
	}
	normalized, err := NormalizeGroupCategoryName(name)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	exists, err := s.repo.ExistsByName(ctx, normalized, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, infraerrors.BadRequest("GROUP_CATEGORY_EXISTS", "category name already exists")
	}
	item.Name = normalized
	if err := s.repo.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *GroupCategoryService) Delete(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured")
	}
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_CATEGORY_ID", "invalid category id")
	}
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *GroupCategoryService) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	if s == nil || s.repo == nil {
		return infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured")
	}
	return s.repo.UpdateSortOrders(ctx, updates)
}

func (s *GroupCategoryService) ResolveID(ctx context.Context, id *int64) (*int64, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	if *id < 0 {
		return nil, infraerrors.BadRequest("INVALID_CATEGORY_ID", "invalid category id")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured")
	}
	if _, err := s.repo.GetByID(ctx, *id); err != nil {
		return nil, err
	}
	return id, nil
}

func AttachGroupCategories(groups []Group, categories []GroupCategory) {
	if len(groups) == 0 || len(categories) == 0 {
		return
	}
	byID := make(map[int64]GroupCategory, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	for i := range groups {
		if groups[i].CategoryID == nil {
			continue
		}
		if category, ok := byID[*groups[i].CategoryID]; ok {
			groups[i].CategoryName = category.Name
			groups[i].CategorySortOrder = category.SortOrder
		} else {
			groups[i].CategoryID = nil
			groups[i].CategoryName = ""
		}
	}
}
