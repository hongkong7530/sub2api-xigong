package repository

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/apikeyfolder"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type apiKeyFolderRepository struct {
	client *dbent.Client
}

func NewAPIKeyFolderRepository(client *dbent.Client) service.APIKeyFolderRepository {
	return &apiKeyFolderRepository{client: client}
}

func (r *apiKeyFolderRepository) ListByUserID(ctx context.Context, userID int64) ([]service.APIKeyFolder, error) {
	rows, err := r.client.APIKeyFolder.Query().
		Where(apikeyfolder.UserIDEQ(userID)).
		Order(dbent.Asc(apikeyfolder.FieldSortOrder), dbent.Asc(apikeyfolder.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list api key folders: %w", err)
	}
	out := make([]service.APIKeyFolder, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, *apiKeyFolderEntityToService(row))
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	for i, folderID := range ids {
		count, err := r.client.APIKey.Query().
			Where(
				apikey.UserIDEQ(userID),
				apikey.FolderIDEQ(folderID),
				apikey.DeletedAtIsNil(),
			).
			Count(ctx)
		if err != nil {
			return nil, fmt.Errorf("count api key folders: %w", err)
		}
		out[i].KeyCount = int64(count)
	}
	return out, nil
}

func (r *apiKeyFolderRepository) GetByID(ctx context.Context, id int64) (*service.APIKeyFolder, error) {
	row, err := r.client.APIKeyFolder.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("API_KEY_FOLDER_NOT_FOUND", "folder not found")
		}
		return nil, fmt.Errorf("get api key folder: %w", err)
	}
	return apiKeyFolderEntityToService(row), nil
}

func (r *apiKeyFolderRepository) Create(ctx context.Context, folder *service.APIKeyFolder) error {
	if folder == nil {
		return fmt.Errorf("folder is nil")
	}
	created, err := r.client.APIKeyFolder.Create().
		SetUserID(folder.UserID).
		SetName(folder.Name).
		SetSortOrder(folder.SortOrder).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create api key folder: %w", err)
	}
	folder.ID = created.ID
	folder.CreatedAt = created.CreatedAt
	folder.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *apiKeyFolderRepository) Update(ctx context.Context, folder *service.APIKeyFolder) error {
	if folder == nil {
		return fmt.Errorf("folder is nil")
	}
	updated, err := r.client.APIKeyFolder.UpdateOneID(folder.ID).
		SetName(folder.Name).
		SetSortOrder(folder.SortOrder).
		Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("API_KEY_FOLDER_NOT_FOUND", "folder not found")
		}
		return fmt.Errorf("update api key folder: %w", err)
	}
	folder.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *apiKeyFolderRepository) Delete(ctx context.Context, id int64) error {
	err := r.client.APIKeyFolder.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("API_KEY_FOLDER_NOT_FOUND", "folder not found")
		}
		return fmt.Errorf("delete api key folder: %w", err)
	}
	return nil
}

func (r *apiKeyFolderRepository) CountByUserID(ctx context.Context, userID int64) (int, error) {
	return r.client.APIKeyFolder.Query().Where(apikeyfolder.UserIDEQ(userID)).Count(ctx)
}

func (r *apiKeyFolderRepository) ExistsByName(ctx context.Context, userID int64, name string, excludeID int64) (bool, error) {
	q := r.client.APIKeyFolder.Query().
		Where(
			apikeyfolder.UserIDEQ(userID),
			apikeyfolder.NameEqualFold(name),
		)
	if excludeID > 0 {
		q = q.Where(apikeyfolder.IDNEQ(excludeID))
	}
	return q.Exist(ctx)
}

func apiKeyFolderEntityToService(row *dbent.APIKeyFolder) *service.APIKeyFolder {
	if row == nil {
		return nil
	}
	return &service.APIKeyFolder{
		ID:        row.ID,
		UserID:    row.UserID,
		Name:      row.Name,
		SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
