package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newAuthSnapshotRepository(t *testing.T) (*apiKeyRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	return &apiKeyRepository{client: client, sql: db}, mock
}

func TestAPIKeyAuthoritativeSnapshotSingleRoundTripAndFreshState(t *testing.T) {
	repo, mock := newAuthSnapshotRepository(t)
	now := time.Now().UTC().Truncate(time.Second)
	groupID := int64(7)
	entity := &dbent.APIKey{ID: 3, UserID: 5, GroupID: &groupID, Key: "test-authoritative", Status: service.StatusActive,
		Quota: 9, QuotaUsed: 4, Usage5h: 2, RateLimit5h: 10, ExpiresAt: &now,
		JailbreakEnabled: true, JailbreakSystemPrompt: "system", JailbreakUserPrompt: "user", JailbreakTemplate: "template"}
	entity.Edges.User = &dbent.User{ID: 5, Status: service.StatusActive, Balance: 12, FrozenBalance: 2, Concurrency: 4, RestrictPublicGroups: true}
	entity.Edges.User.Edges.AllowedGroups = []*dbent.Group{{ID: 7}, {ID: 9}}
	entity.Edges.Group = &dbent.Group{ID: 7, Name: "group", Status: service.StatusActive, Platform: service.PlatformOpenAI,
		RateMultiplier: 0.3, ProfitControlEnabled: true, ProfitMinMargin: 0.1,
		ModelPricing: json.RawMessage(`[{"model":"example","input_price":1}]`), BatchImageDiscountMultiplier: 0.5, BatchImageHoldMultiplier: 2}
	for _, state := range []string{service.StatusActive, "disabled"} {
		entity.Status = state
		// PostgreSQL 行 JSON 不含 Ent 的合成 edges 字段。
		rowJSON := func(value any) []byte {
			raw, err := json.Marshal(value)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &fields))
			delete(fields, "edges")
			raw, err = json.Marshal(fields)
			require.NoError(t, err)
			return raw
		}
		keyJSON := rowJSON(entity)
		userJSON := rowJSON(entity.Edges.User)
		groupJSON := rowJSON(entity.Edges.Group)
		mock.ExpectQuery(regexp.QuoteMeta(apiKeyAuthSnapshotQuery)).WithArgs(entity.Key).
			WillReturnRows(sqlmock.NewRows([]string{"key", "user", "group", "allowed"}).AddRow(keyJSON, userJSON, groupJSON, []byte(`[7,9]`))).RowsWillBeClosed()
		got, err := repo.GetByKey(context.Background(), entity.Key)
		require.NoError(t, err)
		require.Equal(t, apiKeyEntityToService(entity), got)
		require.Equal(t, state, got.Status, "每次读取主库，不用旧快照掩盖停用")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAPIKeyAuthoritativeSnapshotMissingAndInvalidData(t *testing.T) {
	for _, test := range []struct {
		name string
		rows *sqlmock.Rows
		err  error
		want error
	}{
		{name: "missing or deleted", rows: sqlmock.NewRows([]string{"key", "user", "group", "allowed"}), want: service.ErrAPIKeyNotFound},
		{name: "database unavailable", err: sql.ErrConnDone, want: sql.ErrConnDone},
		{name: "bad key", rows: sqlmock.NewRows([]string{"key", "user", "group", "allowed"}).AddRow([]byte(`{`), nil, nil, []byte(`[]`))},
		{name: "bad permission", rows: sqlmock.NewRows([]string{"key", "user", "group", "allowed"}).AddRow([]byte(`{"id":1}`), nil, nil, []byte(`{}`))},
		{name: "scan failure", rows: sqlmock.NewRows([]string{"key"}).AddRow([]byte(`{}`))},
		{name: "iteration failure", rows: sqlmock.NewRows([]string{"key", "user", "group", "allowed"}).AddRow(nil, nil, nil, nil).RowError(0, errors.New("read failed"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := newAuthSnapshotRepository(t)
			expect := mock.ExpectQuery(regexp.QuoteMeta(apiKeyAuthSnapshotQuery)).WithArgs("test")
			if test.err != nil {
				expect.WillReturnError(test.err)
			} else {
				expect.WillReturnRows(test.rows).RowsWillBeClosed()
			}
			got, err := repo.GetByKey(context.Background(), "test")
			require.Error(t, err)
			require.Nil(t, got)
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAPIKeyAuthoritativeSnapshotSkipSoftDeleteUsesOriginalEntPath(t *testing.T) {
	repo, mock := newAuthSnapshotRepository(t)
	ctx := mixins.SkipSoftDelete(context.Background())
	require.False(t, mixins.IsSoftDeleteSkipped(context.Background()))
	require.True(t, mixins.IsSoftDeleteSkipped(ctx))
	// Key 的显式软删除条件不受 skip 影响；关联可见性继续由 Ent 遍历处理。
	mock.ExpectQuery(`SELECT .* FROM "api_keys" WHERE .*"deleted_at" IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id"})).RowsWillBeClosed()
	got, err := repo.GetByKey(ctx, "test")
	require.Nil(t, got)
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAPIKeyAuthoritativeSnapshotMissingRelationsRemainNil(t *testing.T) {
	repo, mock := newAuthSnapshotRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(apiKeyAuthSnapshotQuery)).WithArgs("test").
		WillReturnRows(sqlmock.NewRows([]string{"key", "user", "group", "allowed"}).AddRow([]byte(`{"id":1,"user_id":2}`), nil, []byte(`null`), []byte(`[]`))).RowsWillBeClosed()
	got, err := repo.GetByKey(context.Background(), "test")
	require.NoError(t, err)
	require.Nil(t, got.User)
	require.Nil(t, got.Group)
	require.NoError(t, mock.ExpectationsWereMet())
}
