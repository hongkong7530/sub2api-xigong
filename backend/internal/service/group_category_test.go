package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type groupCategoryRepoStub struct {
	items []GroupCategory
}

func (s *groupCategoryRepoStub) List(context.Context) ([]GroupCategory, error) {
	return append([]GroupCategory(nil), s.items...), nil
}

func (s *groupCategoryRepoStub) GetByID(_ context.Context, id int64) (*GroupCategory, error) {
	for i := range s.items {
		if s.items[i].ID == id {
			item := s.items[i]
			return &item, nil
		}
	}
	return nil, errNotFound("GROUP_CATEGORY_NOT_FOUND")
}

func (s *groupCategoryRepoStub) Create(_ context.Context, category *GroupCategory) error {
	category.ID = int64(len(s.items) + 1)
	s.items = append(s.items, *category)
	return nil
}

func (s *groupCategoryRepoStub) Update(_ context.Context, category *GroupCategory) error {
	for i := range s.items {
		if s.items[i].ID == category.ID {
			s.items[i] = *category
			return nil
		}
	}
	return errNotFound("GROUP_CATEGORY_NOT_FOUND")
}

func (s *groupCategoryRepoStub) Delete(_ context.Context, id int64) error {
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return nil
		}
	}
	return errNotFound("GROUP_CATEGORY_NOT_FOUND")
}

func (s *groupCategoryRepoStub) Count(context.Context) (int, error) {
	return len(s.items), nil
}

func (s *groupCategoryRepoStub) ExistsByName(_ context.Context, name string, excludeID int64) (bool, error) {
	for _, item := range s.items {
		if item.ID == excludeID {
			continue
		}
		if equalFoldASCII(item.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

func (s *groupCategoryRepoStub) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	return nil
}

func errNotFound(reason string) error {
	return &stubAppError{reason: reason}
}

type stubAppError struct {
	reason string
}

func (e *stubAppError) Error() string { return e.reason }

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func TestNormalizeGroupCategoryName(t *testing.T) {
	_, err := NormalizeGroupCategoryName("  ")
	require.Error(t, err)
	name, err := NormalizeGroupCategoryName("  Claude  ")
	require.NoError(t, err)
	require.Equal(t, "Claude", name)
}

func TestGroupCategoryServiceCreateRejectsDuplicate(t *testing.T) {
	svc := NewGroupCategoryService(&groupCategoryRepoStub{items: []GroupCategory{{ID: 1, Name: "Claude"}}})
	_, err := svc.Create(context.Background(), "claude")
	require.Error(t, err)
}

func TestAttachGroupCategories(t *testing.T) {
	id := int64(7)
	missing := int64(9)
	groups := []Group{
		{ID: 1, CategoryID: &id},
		{ID: 2},
		{ID: 3, CategoryID: &missing},
	}
	AttachGroupCategories(groups, []GroupCategory{{ID: 7, Name: "Claude", SortOrder: 2}})
	require.Equal(t, "Claude", groups[0].CategoryName)
	require.Equal(t, 2, groups[0].CategorySortOrder)
	require.Equal(t, "", groups[1].CategoryName)
	require.Nil(t, groups[2].CategoryID)
}
