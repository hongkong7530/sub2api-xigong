package repository

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// Run only against the existing explicitly isolated acceptance database.
func TestUsageRetainedStatisticsPostgres(t *testing.T) {
	dsn := os.Getenv("SUB2API_CLUSTER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	require.Contains(t, dsn, "cluster_acceptance")
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	var uid, keyID, accountID int64
	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,balance) VALUES($1,'fixture',20) RETURNING id`, suffix+"@retention.invalid").Scan(&uid))
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'retention') RETURNING id`, uid, "fixture-"+suffix).Scan(&keyID))
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES('retention','openai','apikey') RETURNING id`).Scan(&accountID))
	at := time.Date(1999, 1, 2, 3, 0, 0, 0, time.UTC)
	for i, duration := range []any{10, nil, 40} {
		var id int64
		require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO usage_logs(user_id,api_key_id,account_id,model,input_tokens,output_tokens,total_cost,actual_cost,duration_ms,created_at) VALUES($1,$2,$3,'gpt-fixture',100,10,.002,.001,$4,$5) RETURNING id`, uid, keyID, accountID, duration, at.Add(time.Duration(i)*time.Second)).Scan(&id))
		_, err = tx.ExecContext(ctx, `INSERT INTO billing_usage_entries(usage_log_id,user_id,api_key_id,billing_type,applied,delta_usd) VALUES($1,$2,$3,0,TRUE,.001)`, id, uid, keyID)
		require.NoError(t, err)
	}
	repo := newUsageLogRepositoryWithSQL(nil, tx)
	filters := usagestats.UsageLogFilters{UserID: uid}
	before, err := repo.GetStatsWithFilters(ctx, filters)
	require.NoError(t, err)
	userBefore, err := repo.GetUserDashboardStats(ctx, uid)
	require.NoError(t, err)
	aggregatedBefore, err := repo.GetUserStatsAggregated(ctx, uid, at.Add(-time.Hour), at.Add(time.Hour))
	require.NoError(t, err)
	dashboardBefore := &DashboardStats{}
	require.NoError(t, repo.fillDashboardUsageStatsAggregated(ctx, dashboardBefore, at, at.Add(time.Hour)))
	_, err = tx.ExecContext(ctx, "SAVEPOINT truncate_check")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "TRUNCATE usage_logs")
	require.Error(t, err, "nonempty detail cannot bypass archival")
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT truncate_check")
	require.NoError(t, err)
	requestType := int16(1)
	imageFilter := usagestats.UsageLogFilters{UserID: uid, RequestType: &requestType, BillingMode: "token"}
	filteredBefore, err := repo.GetStatsWithFilters(ctx, imageFilter)
	require.NoError(t, err)
	var removed int64
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT retain_and_delete_usage_batch($1,2)`, at.Add(time.Hour)).Scan(&removed))
	require.Equal(t, int64(2), removed)
	res, err := tx.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id=$1`, uid)
	require.NoError(t, err)
	removed, err = res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), removed)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT retain_and_delete_usage_batch($1,2)`, at.Add(time.Hour)).Scan(&removed))
	require.Zero(t, removed)
	filteredAfter, err := repo.GetStatsWithFilters(ctx, imageFilter)
	require.NoError(t, err)
	require.Equal(t, filteredBefore, filteredAfter)
	after, err := repo.GetStatsWithFilters(ctx, filters)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(before, after), "weighted statistics must survive detail deletion")
	userAfter, err := repo.GetUserDashboardStats(ctx, uid)
	require.NoError(t, err)
	require.Equal(t, userBefore, userAfter)
	aggregatedAfter, err := repo.GetUserStatsAggregated(ctx, uid, at.Add(-time.Hour), at.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, aggregatedBefore, aggregatedAfter)
	dashboardAfter := &DashboardStats{}
	require.NoError(t, repo.fillDashboardUsageStatsAggregated(ctx, dashboardAfter, at, at.Add(time.Hour)))
	require.Equal(t, dashboardBefore, dashboardAfter, "default totals do not depend on expiring day buckets")
	_, err = tx.ExecContext(ctx, "SAVEPOINT null_batch_check")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SELECT retain_and_delete_usage_batch($1,NULL)`, at.Add(time.Hour))
	require.Error(t, err)
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT null_batch_check")
	require.NoError(t, err)
	var ledger, detached, details int
	var cost float64
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE usage_log_id IS NULL),SUM(delta_usd) FROM billing_usage_entries WHERE user_id=$1`, uid).Scan(&ledger, &detached, &cost))
	require.Equal(t, 3, ledger)
	require.Equal(t, ledger, detached)
	require.InDelta(t, .003, cost, 1e-9)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs WHERE user_id=$1`, uid).Scan(&details))
	require.Zero(t, details)
	t.Log("request detail removed; cumulative usage, weighted duration and three monetary entries unchanged")
}
