package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSiteContentAppearanceCAS(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewSiteContentRepository(db)
	m.ExpectExec("INSERT INTO settings.*ON CONFLICT.*WHERE COALESCE").WithArgs(service.SiteAppearanceKey, `{"version":2}`, 1).WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, r.CompareAndSwapAppearance(context.Background(), 1, `{"version":2}`), service.ErrContentConflict)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestSiteContentMetadataPagination(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewSiteContentRepository(db)
	rows := sqlmock.NewRows([]string{"id", "slug", "draft", "published", "version", "updated_at", "published_at"})
	for i := 0; i < 51; i++ {
		rows.AddRow(i+51, "guide", `{}`, `{"title":"Title","position":0}`, 1, time.Now(), time.Now())
	}
	m.ExpectQuery("SELECT .*jsonb_build_object.* FROM site_documents WHERE published IS NOT NULL ORDER BY .* LIMIT 51 OFFSET \\$1").WithArgs(50).WillReturnRows(rows)
	docs, more, e := r.Documents(context.Background(), true, "", 2)
	require.NoError(t, e)
	require.Len(t, docs, 50)
	require.True(t, more)
	require.Empty(t, docs[0].Published.Content)
	require.NoError(t, m.ExpectationsWereMet())
}
