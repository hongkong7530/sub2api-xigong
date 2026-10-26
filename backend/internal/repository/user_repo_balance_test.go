package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserRepositoryGetBalanceMinimalColumnsAndFreshRead(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	for _, balance := range []float64{12.5, 0, -0.25} {
		mock.ExpectQuery(`SELECT "users"\."id", "users"\."balance" FROM "users" WHERE "users"\."id" = \$1 AND "users"\."deleted_at" IS NULL`).
			WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "balance"}).AddRow(int64(42), balance)).RowsWillBeClosed()
		got, err := repo.GetBalance(context.Background(), 42)
		require.NoError(t, err)
		require.Equal(t, balance, got, "每次读取当前余额，不复用先前值或加载授权组")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepositoryGetBalanceMissingAndReadFailure(t *testing.T) {
	for _, test := range []struct {
		name       string
		queryError error
		rows       *sqlmock.Rows
		want       error
	}{
		{name: "missing or deleted", rows: sqlmock.NewRows([]string{"id", "balance"}), want: service.ErrUserNotFound},
		{name: "database unavailable", queryError: sql.ErrConnDone, want: sql.ErrConnDone},
		{name: "iteration error", rows: sqlmock.NewRows([]string{"id", "balance"}).AddRow(int64(42), 1).RowError(0, errors.New("read failed"))},
		{name: "malformed balance", rows: sqlmock.NewRows([]string{"id", "balance"}).AddRow(int64(42), "invalid")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := newRedeemAdjustmentRepoMock(t)
			expect := mock.ExpectQuery(`SELECT "users"\."id", "users"\."balance" FROM "users"`).WithArgs(int64(42))
			if test.queryError != nil {
				expect.WillReturnError(test.queryError)
			} else {
				expect.WillReturnRows(test.rows).RowsWillBeClosed()
			}
			balance, err := repo.GetBalance(context.Background(), 42)
			require.Error(t, err)
			require.Zero(t, balance)
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUserRepositoryGetBalancePreservesSkipSoftDeleteContext(t *testing.T) {
	repo, mock := newRedeemAdjustmentRepoMock(t)
	mock.ExpectQuery(`^SELECT "users"\."id", "users"\."balance" FROM "users" WHERE "users"\."id" = \$1 LIMIT 2$`).
		WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "balance"}).AddRow(int64(42), 3.0)).RowsWillBeClosed()
	balance, err := repo.GetBalance(mixins.SkipSoftDelete(context.Background()), 42)
	require.NoError(t, err)
	require.Equal(t, 3.0, balance)
	require.NoError(t, mock.ExpectationsWereMet())
}
