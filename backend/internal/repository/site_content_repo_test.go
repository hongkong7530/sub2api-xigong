package repository

import (
	"context"
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSiteContentPublishedReadNeverSelectsDraft(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewSiteContentRepository(db)
	m.ExpectQuery("SELECT id,slug,'\\{\\}'::jsonb,published,version,updated_at,published_at FROM site_documents WHERE slug=\\$1 AND published IS NOT NULL").WithArgs("guide").WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "draft", "published", "version", "updated_at", "published_at"}).AddRow(1, "guide", `{}`, `{"title":"Live","content":"Public only"}`, 3, time.Now(), time.Now()))
	d, e := r.PublishedDocument(context.Background(), "guide")
	require.NoError(t, e)
	require.Empty(t, d.Draft.Content)
	require.Equal(t, "Public only", d.Published.Content)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestSiteContentDraftConflictDoesNotOverwrite(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewSiteContentRepository(db)
	m.ExpectBegin()
	m.ExpectQuery("UPDATE site_documents SET draft=.*WHERE id=.*AND version=.*AND slug=").WithArgs(sqlmock.AnyArg(), int64(1), 2, "guide").WillReturnError(sql.ErrNoRows)
	m.ExpectRollback()
	e = r.SaveDocument(context.Background(), &service.SiteDocument{ID: 1, Slug: "guide", Version: 2, Draft: service.DocumentBody{Title: "Draft", Content: "private"}})
	require.ErrorIs(t, e, service.ErrContentConflict)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestSiteContentUnpublishClearsSnapshot(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewSiteContentRepository(db)
	m.ExpectExec("UPDATE site_documents SET published=NULL,published_at=NULL.*WHERE id=.*AND version=").WithArgs(int64(1), 3).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, r.PublishDocument(context.Background(), 1, 3, false))
	require.NoError(t, m.ExpectationsWereMet())
}
func TestSiteContentPublicPartnerFilter(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewSiteContentRepository(db)
	m.ExpectQuery("SELECT .* FROM site_partners WHERE .* OR enabled").WithArgs(true).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "url", "icon", "tag", "position", "enabled", "version"}))
	p, e := r.Partners(context.Background(), true)
	require.NoError(t, e)
	require.Empty(t, p)
	require.NoError(t, m.ExpectationsWereMet())
}
