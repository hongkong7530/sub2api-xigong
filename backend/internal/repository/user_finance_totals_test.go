package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestFinanceRechargeSumIncludesDeletedOnlyWhenRequested(t *testing.T) {
	repo,mock:=newRedeemAdjustmentRepoMock(t)
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(total_recharged\),0\) FROM users WHERE deleted_at IS NULL`).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(125.0))
	got,err:=repo.SumTotalRecharged(context.Background(),false)
	require.NoError(t,err);require.Equal(t,125.0,got)
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(total_recharged\),0\) FROM users$`).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(150.0))
	got,err=repo.SumTotalRecharged(context.Background(),true)
	require.NoError(t,err);require.Equal(t,150.0,got)
	require.NoError(t,mock.ExpectationsWereMet())
}

func TestFinanceAdminCreditIsAtomicWithRechargeCounter(t *testing.T) {
	repo,mock:=newRedeemAdjustmentRepoMock(t)
	mock.ExpectQuery(`UPDATE users SET balance = balance \+ \$1, total_recharged = total_recharged \+ GREATEST\(\$1, 0\), updated_at = NOW\(\)`).WithArgs(5.0,int64(42)).WillReturnRows(sqlmock.NewRows([]string{"old","new"}).AddRow(10.0,15.0))
	got,err:=repo.AdjustBalance(context.Background(),42,5)
	require.NoError(t,err);require.Equal(t,10.0,got.Old);require.Equal(t,15.0,got.New)
	require.NoError(t,mock.ExpectationsWereMet())
}

func TestFinanceSetBalanceCountsOnlyPositiveLockedDifference(t *testing.T) {
	repo,mock:=newRedeemAdjustmentRepoMock(t)
	mock.ExpectQuery(`UPDATE users AS u SET balance = \$1, total_recharged = u.total_recharged \+ GREATEST\(\$1 - prev.balance, 0\), updated_at = NOW\(\) FROM \(SELECT id, balance FROM users WHERE id = \$2 AND deleted_at IS NULL FOR UPDATE\) AS prev`).WithArgs(20.0,int64(42)).WillReturnRows(sqlmock.NewRows([]string{"old","new"}).AddRow(15.0,20.0))
	got,err:=repo.SetBalance(context.Background(),42,20)
	require.NoError(t,err);require.Equal(t,15.0,got.Old);require.Equal(t,20.0,got.New)
	require.NoError(t,mock.ExpectationsWereMet())
}
