//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchAssignGroupCategory_ClearsWhenCategoryNil(t *testing.T) {
	repo := &groupRepoStub{}
	svc := &adminServiceImpl{groupRepo: repo}

	updated, err := svc.BatchAssignGroupCategory(context.Background(), []int64{11, 12, 13}, nil)
	require.NoError(t, err)
	require.Equal(t, 3, updated)
	require.Equal(t, 1, repo.categoryAssignCalls)
	require.Equal(t, []int64{11, 12, 13}, repo.categoryAssignIDs)
	require.Nil(t, repo.categoryAssignValue)
}

func TestBatchAssignGroupCategory_RejectsEmptySelection(t *testing.T) {
	repo := &groupRepoStub{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.BatchAssignGroupCategory(context.Background(), nil, nil)
	require.Error(t, err)
	require.Equal(t, 0, repo.categoryAssignCalls, "empty selection must not touch the repository")
}

func TestBatchAssignGroupCategory_RejectsOversizedSelection(t *testing.T) {
	repo := &groupRepoStub{}
	svc := &adminServiceImpl{groupRepo: repo}

	ids := make([]int64, 501)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	_, err := svc.BatchAssignGroupCategory(context.Background(), ids, nil)
	require.Error(t, err)
	require.Equal(t, 0, repo.categoryAssignCalls)
}

// 分类服务未配置时必须报错，不能把「设成某分类」静默执行成清空。
func TestBatchAssignGroupCategory_RequiresCategoryServiceForRealCategory(t *testing.T) {
	repo := &groupRepoStub{}
	svc := &adminServiceImpl{groupRepo: repo}
	categoryID := int64(5)

	_, err := svc.BatchAssignGroupCategory(context.Background(), []int64{11}, &categoryID)
	require.Error(t, err)
	require.Equal(t, 0, repo.categoryAssignCalls)
}

func TestBatchAssignGroupCategory_ResolvesCategoryThroughService(t *testing.T) {
	repo := &groupRepoStub{}
	categoryRepo := &groupCategoryRepoStub{items: []GroupCategory{{ID: 5, Name: "Claude"}}}
	svc := &adminServiceImpl{
		groupRepo:   repo,
		categorySvc: NewGroupCategoryService(categoryRepo),
	}
	categoryID := int64(5)

	updated, err := svc.BatchAssignGroupCategory(context.Background(), []int64{11, 12}, &categoryID)
	require.NoError(t, err)
	require.Equal(t, 2, updated)
	require.NotNil(t, repo.categoryAssignValue)
	require.Equal(t, int64(5), *repo.categoryAssignValue)
}
