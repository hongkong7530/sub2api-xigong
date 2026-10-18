package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type apiKeyFolderRepoStub struct {
	items []APIKeyFolder
}

func (s *apiKeyFolderRepoStub) ListByUserID(_ context.Context, userID int64) ([]APIKeyFolder, error) {
	out := make([]APIKeyFolder, 0)
	for _, item := range s.items {
		if item.UserID == userID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *apiKeyFolderRepoStub) GetByID(_ context.Context, id int64) (*APIKeyFolder, error) {
	for i := range s.items {
		if s.items[i].ID == id {
			item := s.items[i]
			return &item, nil
		}
	}
	return nil, errNotFound("API_KEY_FOLDER_NOT_FOUND")
}

func (s *apiKeyFolderRepoStub) Create(_ context.Context, folder *APIKeyFolder) error {
	folder.ID = int64(len(s.items) + 1)
	s.items = append(s.items, *folder)
	return nil
}

func (s *apiKeyFolderRepoStub) Update(_ context.Context, folder *APIKeyFolder) error {
	for i := range s.items {
		if s.items[i].ID == folder.ID {
			s.items[i] = *folder
			return nil
		}
	}
	return errNotFound("API_KEY_FOLDER_NOT_FOUND")
}

func (s *apiKeyFolderRepoStub) Delete(_ context.Context, id int64) error {
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return nil
		}
	}
	return errNotFound("API_KEY_FOLDER_NOT_FOUND")
}

func (s *apiKeyFolderRepoStub) CountByUserID(_ context.Context, userID int64) (int, error) {
	count := 0
	for _, item := range s.items {
		if item.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (s *apiKeyFolderRepoStub) ExistsByName(_ context.Context, userID int64, name string, excludeID int64) (bool, error) {
	for _, item := range s.items {
		if item.UserID != userID || item.ID == excludeID {
			continue
		}
		if equalFoldASCII(item.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

func TestAPIKeyFolderServiceRejectsForeignFolder(t *testing.T) {
	svc := NewAPIKeyFolderService(&apiKeyFolderRepoStub{items: []APIKeyFolder{{ID: 3, UserID: 2, Name: "work"}}})
	_, err := svc.Update(context.Background(), 1, 3, "home")
	require.Error(t, err)
	_, err = svc.ResolveForUser(context.Background(), 1, folderIDPtr(3))
	require.Error(t, err)
}

func TestAPIKeyFolderServiceCreateAndResolve(t *testing.T) {
	svc := NewAPIKeyFolderService(&apiKeyFolderRepoStub{})
	created, err := svc.Create(context.Background(), 8, "  Work  ")
	require.NoError(t, err)
	require.Equal(t, "Work", created.Name)
	resolved, err := svc.ResolveForUser(context.Background(), 8, &created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, *resolved)
}

func folderIDPtr(v int64) *int64 { return &v }
