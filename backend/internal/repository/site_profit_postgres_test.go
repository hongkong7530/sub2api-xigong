package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Opt-in, disposable, loopback-only database. Never use the normal application DSN.
func TestSiteProfitPostgresAtomicLedgerAndAggregation(t *testing.T) {
	dsn := os.Getenv("SITE_PROFIT_TEST_DSN")
	if dsn == "" {
		t.Skip("dedicated loopback profit test database not configured")
	}
	parsed, e := url.Parse(dsn)
	require.NoError(t, e)
	require.Equal(t, "127.0.0.1", parsed.Hostname())
	require.True(t, strings.HasPrefix(parsed.Path, "/profit_test_"))
	db, e := sql.Open("postgres", dsn)
	require.NoError(t, e)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	_, e = db.ExecContext(ctx, `CREATE TABLE groups(id bigint PRIMARY KEY,name text,platform text,status text DEFAULT 'active',deleted_at timestamptz,is_exclusive boolean DEFAULT false,rate_multiplier numeric DEFAULT 0.3,peak_rate_enabled boolean DEFAULT false,peak_start text DEFAULT '',peak_end text DEFAULT '',peak_rate_multiplier numeric DEFAULT 1);
 CREATE TABLE usage_logs(id bigint PRIMARY KEY,group_id bigint,total_cost numeric(20,8),actual_cost numeric(20,8));
 CREATE TABLE billing_usage_entries(id bigserial PRIMARY KEY,usage_log_id bigint UNIQUE,user_id bigint,billing_type smallint,applied boolean,delta_usd numeric(20,8));
 CREATE TABLE cluster_node_credentials(database_role text,status text);
 INSERT INTO groups(id,name,platform,is_exclusive) VALUES(1,'Public','openai',false),(2,'Private','anthropic',true),(3,'At cost','gemini',false),(4,'Unconfigured','grok',false);`)
	require.NoError(t, e)
	// Existing node credentials must receive only the same bounded permissions as future nodes.
	nodeRole := "profit_node_fixture"
	_, e = db.ExecContext(ctx, `CREATE ROLE profit_node_fixture LOGIN NOSUPERUSER;
 GRANT USAGE ON SCHEMA public TO profit_node_fixture;
 GRANT SELECT,INSERT,UPDATE ON groups,usage_logs,billing_usage_entries TO profit_node_fixture;
 GRANT USAGE ON SEQUENCE billing_usage_entries_id_seq TO profit_node_fixture;
 INSERT INTO cluster_node_credentials(database_role,status) VALUES('profit_node_fixture','active')`)
	require.NoError(t, e)
	defer func() { _, _ = db.Exec(`DROP OWNED BY profit_node_fixture; DROP ROLE profit_node_fixture`) }()
	migration, e := os.ReadFile(filepath.Join("..", "..", "migrations", "280_site_profit_ledger.sql"))
	require.NoError(t, e)
	_, e = db.ExecContext(ctx, string(migration))
	require.NoError(t, e)
	repo := &siteProfitRepository{db: db}
	svc := service.NewSiteProfitService(repo)
	cost := "0.2"
	v, e := svc.SetCost(ctx, 1, 99, service.ProfitCostInput{Version: 0, Multiplier: &cost})
	require.NoError(t, e)
	require.EqualValues(t, 1, v.Version)
	_, e = svc.SetCost(ctx, 1, 99, service.ProfitCostInput{Version: 0, Multiplier: &cost})
	require.ErrorIs(t, e, service.ErrProfitConflict)
	for _, g := range []int64{2, 3} {
		c := cost
		if g == 3 {
			c = "0.3"
		}
		_, e = svc.SetCost(ctx, g, 99, service.ProfitCostInput{Multiplier: &c})
		require.NoError(t, e)
	}
	settle := func(q interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	}, id, group int64, revenue string) {
		_, err := q.ExecContext(ctx, `INSERT INTO usage_logs(id,group_id,total_cost,actual_cost) VALUES($1,$2,1,$3);`, id, group, revenue)
		require.NoError(t, err)
		_, err = q.ExecContext(ctx, `INSERT INTO billing_usage_entries(usage_log_id,user_id,billing_type,applied,delta_usd) VALUES($1,7,0,true,$2)`, id, revenue)
		require.NoError(t, err)
	}
	settle(db, 1, 1, "0.3")
	settle(db, 2, 2, "0.9")
	settle(db, 3, 3, "0.3")
	settle(db, 4, 4, "0.4")
	tx, e := db.BeginTx(ctx, nil)
	require.NoError(t, e)
	settle(tx, 5, 1, "0.3")
	require.NoError(t, tx.Rollback())
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM site_profit_entries`).Scan(&count))
	require.Equal(t, 4, count)
	_, e = db.ExecContext(ctx, `INSERT INTO billing_usage_entries(usage_log_id,user_id,billing_type,applied,delta_usd) VALUES(1,7,0,true,.3) ON CONFLICT(usage_log_id) DO NOTHING`)
	require.NoError(t, e)
	cost = "0.4"
	_, e = svc.SetCost(ctx, 1, 99, service.ProfitCostInput{Version: 1, Multiplier: &cost})
	require.NoError(t, e)
	settle(db, 6, 1, "0.3")
	// Known loss, zero-revenue subsidy, and an older uncommitted ID must not be skipped by a max-id watermark.
	early, e := db.BeginTx(ctx, nil)
	require.NoError(t, e)
	settle(early, 7, 1, "0")
	settle(db, 8, 1, "0.3")
	n, e := repo.Aggregate(ctx, 500)
	require.NoError(t, e)
	require.Equal(t, 6, n)
	require.NoError(t, early.Commit())
	n, e = repo.Aggregate(ctx, 500)
	require.NoError(t, e)
	require.Equal(t, 1, n)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := repo.Aggregate(ctx, 500); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	pub, e := repo.Snapshot(ctx, false)
	require.NoError(t, e)
	require.Len(t, pub.Groups, 3)
	require.EqualValues(t, 6, pub.Metrics.Total.Requests)
	require.EqualValues(t, 5, pub.Metrics.Total.KnownRequests)
	require.Equal(t, "-0.50000000", pub.Metrics.Total.Profit)
	raw, e := json.Marshal(pub)
	require.NoError(t, e)
	require.NotContains(t, string(raw), "Private")
	require.NotContains(t, string(raw), "cost_multiplier")
	require.NotContains(t, string(raw), "cost_version")
	admin, e := repo.Snapshot(ctx, true)
	require.NoError(t, e)
	require.Len(t, admin.Groups, 4)
	require.EqualValues(t, 7, admin.Metrics.Total.Requests)
	require.Equal(t, "0.20000000", admin.Metrics.Total.Profit)
	own, e := repo.Contribution(ctx, 7)
	require.NoError(t, e)
	require.Equal(t, admin.Metrics.Total.Profit, own.Total.Profit)
	other, e := repo.Contribution(ctx, 8)
	require.NoError(t, e)
	require.Zero(t, other.Total.Requests)
	// Detail deletion must not delete reporting facts or their historical cost versions.
	_, e = db.ExecContext(ctx, `DELETE FROM usage_logs`)
	require.NoError(t, e)
	var firstCost, latestCost string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cost_usd::text FROM site_profit_entries WHERE billing_entry_id=1`).Scan(&firstCost))
	require.Equal(t, "0.20000000", firstCost)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cost_usd::text FROM site_profit_entries ORDER BY id DESC LIMIT 1`).Scan(&latestCost))
	require.Equal(t, "0.40000000", latestCost)
	n, e = repo.Aggregate(ctx, 500)
	require.NoError(t, e)
	require.Zero(t, n)
	entries, e := repo.Entries(ctx, service.ProfitEntryFilter{Page: 1})
	require.NoError(t, e)
	require.Len(t, entries.Items, 7)
	// A later cost configuration cannot turn an unknown historical request into a known-cost request.
	_, e = svc.SetCost(ctx, 4, 99, service.ProfitCostInput{Multiplier: &cost})
	require.NoError(t, e)
	var historicalCost *string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cost_usd::text FROM site_profit_entries WHERE group_id=4`).Scan(&historicalCost))
	require.Nil(t, historicalCost)
	// Clear is an explicit version, not zero. An explicit zero is a real, known cost.
	_, e = svc.SetCost(ctx, 1, 99, service.ProfitCostInput{Version: 2, Multiplier: nil})
	require.NoError(t, e)
	settle(db, 9, 1, "0.3")
	zero := "0"
	_, e = svc.SetCost(ctx, 1, 99, service.ProfitCostInput{Version: 3, Multiplier: &zero})
	require.NoError(t, e)
	settle(db, 10, 1, "0.3")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT cost_usd::text FROM site_profit_entries ORDER BY id DESC LIMIT 1`).Scan(&latestCost))
	require.Equal(t, "0.00000000", latestCost)
	// A pending bounded batch is visibly stale; successful aggregation clears the backlog flag.
	n, e = repo.Aggregate(ctx, 1)
	require.NoError(t, e)
	require.Equal(t, 1, n)
	partial, e := repo.Snapshot(ctx, true)
	require.NoError(t, e)
	require.True(t, partial.Backlog)
	require.True(t, partial.Stale)
	n, e = repo.Aggregate(ctx, 500)
	require.NoError(t, e)
	require.Equal(t, 1, n)
	// Older natural days remain in all-time totals, but the 7-day window excludes day -7.
	_, e = db.ExecContext(ctx, `INSERT INTO site_profit_group_daily(group_id,day,billing_type,requests,known_requests,revenue_usd,known_revenue_usd,cost_usd,profit_usd)
 SELECT 1,(statement_timestamp() AT TIME ZONE 'Asia/Shanghai')::date-days_ago,0,1,1,1,1,0,1 FROM (VALUES(6),(7))days(days_ago)`)
	require.NoError(t, e)
	windows, e := repo.Snapshot(ctx, true)
	require.NoError(t, e)
	require.Equal(t, windows.Metrics.Today.Requests+1, windows.Metrics.Week.Requests)
	// Definer capture must work for a non-owner writer without direct access to the reporting ledger.
	role := "profit_writer_fixture"
	_, e = db.ExecContext(ctx, `CREATE ROLE profit_writer_fixture NOLOGIN; GRANT USAGE ON SCHEMA public TO profit_writer_fixture;
 GRANT INSERT ON usage_logs,billing_usage_entries TO profit_writer_fixture; GRANT USAGE ON SEQUENCE billing_usage_entries_id_seq TO profit_writer_fixture`)
	require.NoError(t, e)
	defer func() { _, _ = db.Exec(`DROP OWNED BY profit_writer_fixture; DROP ROLE profit_writer_fixture`) }()
	writer, e := db.BeginTx(ctx, nil)
	require.NoError(t, e)
	_, e = writer.ExecContext(ctx, "SET LOCAL ROLE "+role)
	require.NoError(t, e)
	settle(writer, 11, 1, "0.3")
	require.NoError(t, writer.Commit())
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM site_profit_entries WHERE billing_entry_id=(SELECT id FROM billing_usage_entries WHERE usage_log_id=11)`).Scan(&count))
	require.Equal(t, 1, count)
	// Exercise migration grants as a real non-owner connection, including aggregate RETURNING columns.
	nodeURL := *parsed
	nodeURL.User = url.User(nodeRole)
	nodeDB, e := sql.Open("postgres", nodeURL.String())
	require.NoError(t, e)
	defer nodeDB.Close()
	nodeRepo := &siteProfitRepository{db: nodeDB}
	nodeSvc := service.NewSiteProfitService(nodeRepo)
	_, e = nodeSvc.SetCost(ctx, 1, 99, service.ProfitCostInput{Version: 4, Multiplier: &cost})
	require.NoError(t, e)
	settle(nodeDB, 12, 1, "0.3")
	_, e = nodeRepo.Aggregate(ctx, 500)
	require.NoError(t, e)
	_, e = nodeRepo.Snapshot(ctx, false)
	require.NoError(t, e)
	for _, statement := range []string{
		`DELETE FROM site_profit_entries WHERE false`,
		`UPDATE site_profit_entries SET revenue_usd=0 WHERE false`,
		`INSERT INTO site_profit_entries(id) VALUES(-1)`,
		`UPDATE site_profit_meta SET timezone='UTC' WHERE false`,
		`UPDATE site_profit_cost_versions SET cost_multiplier=0 WHERE false`,
		`DELETE FROM site_profit_group_totals WHERE false`,
	} {
		_, e = nodeDB.ExecContext(ctx, statement)
		require.ErrorContains(t, e, "permission denied", statement)
	}
}
