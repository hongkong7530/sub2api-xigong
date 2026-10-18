package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/groupcategory"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type groupCategoryRepository struct {
	client *dbent.Client
}

func NewGroupCategoryRepository(client *dbent.Client) service.GroupCategoryRepository {
	return &groupCategoryRepository{client: client}
}

func (r *groupCategoryRepository) List(ctx context.Context) ([]service.GroupCategory, error) {
	rows, err := r.client.GroupCategory.Query().
		Order(dbent.Asc(groupcategory.FieldSortOrder), dbent.Asc(groupcategory.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list group categories: %w", err)
	}
	out := make([]service.GroupCategory, 0, len(rows))
	for _, row := range rows {
		out = append(out, *groupCategoryEntityToService(row))
	}
	return out, nil
}

func (r *groupCategoryRepository) GetByID(ctx context.Context, id int64) (*service.GroupCategory, error) {
	row, err := r.client.GroupCategory.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("GROUP_CATEGORY_NOT_FOUND", "category not found")
		}
		return nil, fmt.Errorf("get group category: %w", err)
	}
	return groupCategoryEntityToService(row), nil
}

func (r *groupCategoryRepository) Create(ctx context.Context, category *service.GroupCategory) error {
	if category == nil {
		return fmt.Errorf("category is nil")
	}
	created, err := r.client.GroupCategory.Create().
		SetName(category.Name).
		SetSortOrder(category.SortOrder).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create group category: %w", err)
	}
	category.ID = created.ID
	category.CreatedAt = created.CreatedAt
	category.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *groupCategoryRepository) Update(ctx context.Context, category *service.GroupCategory) error {
	if category == nil {
		return fmt.Errorf("category is nil")
	}
	updated, err := r.client.GroupCategory.UpdateOneID(category.ID).
		SetName(category.Name).
		SetSortOrder(category.SortOrder).
		Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("GROUP_CATEGORY_NOT_FOUND", "category not found")
		}
		return fmt.Errorf("update group category: %w", err)
	}
	category.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *groupCategoryRepository) Delete(ctx context.Context, id int64) error {
	err := r.client.GroupCategory.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("GROUP_CATEGORY_NOT_FOUND", "category not found")
		}
		return fmt.Errorf("delete group category: %w", err)
	}
	return nil
}

func (r *groupCategoryRepository) Count(ctx context.Context) (int, error) {
	return r.client.GroupCategory.Query().Count(ctx)
}

func (r *groupCategoryRepository) ExistsByName(ctx context.Context, name string, excludeID int64) (bool, error) {
	q := r.client.GroupCategory.Query().Where(groupcategory.NameEqualFold(name))
	if excludeID > 0 {
		q = q.Where(groupcategory.IDNEQ(excludeID))
	}
	return q.Exist(ctx)
}

func (r *groupCategoryRepository) UpdateSortOrders(ctx context.Context, updates []service.GroupSortOrderUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	sortOrderByID := make(map[int64]int, len(updates))
	ids := make([]int64, 0, len(updates))
	for _, update := range updates {
		if update.ID <= 0 {
			continue
		}
		if _, exists := sortOrderByID[update.ID]; !exists {
			ids = append(ids, update.ID)
		}
		sortOrderByID[update.ID] = update.SortOrder
	}
	if len(ids) == 0 {
		return nil
	}
	existing, err := r.client.GroupCategory.Query().Where(groupcategory.IDIn(ids...)).Count(ctx)
	if err != nil {
		return err
	}
	if existing != len(ids) {
		return infraerrors.NotFound("GROUP_CATEGORY_NOT_FOUND", "category not found")
	}
	args := make([]any, 0, len(ids)*2+1)
	clauses := make([]string, 0, len(ids))
	placeholder := 1
	for _, id := range ids {
		clauses = append(clauses, fmt.Sprintf("WHEN $%d THEN $%d", placeholder, placeholder+1))
		args = append(args, id, sortOrderByID[id])
		placeholder += 2
	}
	args = append(args, pq.Array(ids))
	query := fmt.Sprintf(`
		UPDATE group_categories
		SET sort_order = CASE id
			%s
			ELSE sort_order
		END,
		updated_at = NOW()
		WHERE id = ANY($%d)
	`, strings.Join(clauses, "\n\t\t\t"), placeholder)
	_, err = r.client.ExecContext(ctx, query, args...)
	return err
}

func groupCategoryEntityToService(row *dbent.GroupCategory) *service.GroupCategory {
	if row == nil {
		return nil
	}
	return &service.GroupCategory{
		ID:        row.ID,
		Name:      row.Name,
		SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
