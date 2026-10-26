package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type siteProfitRepository struct{ db *sql.DB }

func NewSiteProfitRepository(db *sql.DB) service.SiteProfitRepository {
	return &siteProfitRepository{db: db}
}

func profitMeta(ctx context.Context, tx *sql.Tx) (*service.ProfitSnapshot, error) {
	out := &service.ProfitSnapshot{Groups: []service.ProfitGroup{}, Metrics: emptyProfitWindows(), Balance: service.EmptyProfitMetric(), Subscription: service.EmptyProfitMetric()}
	err := tx.QueryRowContext(ctx, `SELECT timezone,started_at,aggregated_at,backlog FROM site_profit_meta WHERE id=1`).Scan(&out.Timezone, &out.StartedAt, &out.AsOf, &out.Backlog)
	out.Stale = out.Backlog || out.AsOf == nil || time.Since(*out.AsOf) > 20*time.Second
	return out, err
}
func emptyProfitWindows() service.ProfitWindows {
	return service.ProfitWindows{Total: service.EmptyProfitMetric(), Today: service.EmptyProfitMetric(), Week: service.EmptyProfitMetric()}
}

// Only aggregated rows are read. The same repeatable-read snapshot owns metadata, visibility and all sums.
func (r *siteProfitRepository) Snapshot(ctx context.Context, admin bool) (*service.ProfitSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	out, e := profitMeta(ctx, tx)
	if e != nil {
		return nil, e
	}
	out.Scope = "public_groups"
	if admin {
		out.Scope = "all_groups"
	}
	rows, e := tx.QueryContext(ctx, `WITH eligible AS (
 SELECT g.id,g.name,g.platform,CASE WHEN g.deleted_at IS NOT NULL THEN 'deleted' ELSE g.status END AS status,g.rate_multiplier,g.peak_rate_enabled,g.peak_start,g.peak_end,g.peak_rate_multiplier
 FROM groups g WHERE ($1 OR (g.deleted_at IS NULL AND g.status='active' AND NOT g.is_exclusive))
 UNION ALL
 SELECT t.group_id,MAX(t.group_name),MAX(t.platform),'deleted',NULL::numeric,false,'','',1::numeric
 FROM site_profit_group_totals t WHERE $1 AND NOT EXISTS(SELECT 1 FROM groups g WHERE g.id=t.group_id) GROUP BY t.group_id
), measures AS (
 SELECT group_id,billing_type,'total' AS window,requests,known_requests,revenue_usd,known_revenue_usd,cost_usd,profit_usd FROM site_profit_group_totals
 UNION ALL
 SELECT group_id,billing_type,'today',SUM(requests),SUM(known_requests),SUM(revenue_usd),SUM(known_revenue_usd),SUM(cost_usd),SUM(profit_usd)
 FROM site_profit_group_daily WHERE day=(statement_timestamp() AT TIME ZONE $2)::date GROUP BY group_id,billing_type
 UNION ALL
 SELECT group_id,billing_type,'week',SUM(requests),SUM(known_requests),SUM(revenue_usd),SUM(known_revenue_usd),SUM(cost_usd),SUM(profit_usd)
 FROM site_profit_group_daily WHERE day BETWEEN (statement_timestamp() AT TIME ZONE $2)::date-6 AND (statement_timestamp() AT TIME ZONE $2)::date GROUP BY group_id,billing_type
)
 SELECT g.id,g.name,g.platform,g.status,COALESCE(g.rate_multiplier::text,''),g.peak_rate_enabled,g.peak_start,g.peak_end,g.peak_rate_multiplier::text,
 c.cost_multiplier::text,COALESCE(c.version,0),c.effective_at,
 COALESCE(m.window,'total'),COALESCE(m.billing_type,0),COALESCE(m.requests,0),COALESCE(m.known_requests,0),
 COALESCE(m.revenue_usd,0)::text,COALESCE(m.known_revenue_usd,0)::text,COALESCE(m.cost_usd,0)::text,COALESCE(m.profit_usd,0)::text
 FROM eligible g LEFT JOIN LATERAL (SELECT cost_multiplier,version,effective_at FROM site_profit_cost_versions WHERE group_id=g.id ORDER BY version DESC LIMIT 1)c ON true
 LEFT JOIN measures m ON m.group_id=g.id ORDER BY g.id,m.window,m.billing_type`, admin, out.Timezone)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	indices := map[int64]int{}
	for rows.Next() {
		var g service.ProfitGroup
		var cost *string
		var version int64
		var effective *time.Time
		var window string
		var billing int16
		var m service.ProfitMetric
		if e = rows.Scan(&g.ID, &g.Name, &g.Platform, &g.Status, &g.Rate, &g.PeakEnabled, &g.PeakStart, &g.PeakEnd, &g.PeakMultiplier, &cost, &version, &effective, &window, &billing, &m.Requests, &m.KnownRequests, &m.Revenue, &m.KnownRevenue, &m.Cost, &m.Profit); e != nil {
			return nil, e
		}
		m.Complete()
		i, ok := indices[g.ID]
		if !ok {
			g.Metrics = emptyProfitWindows()
			if cost != nil {
				a, _ := decimal.NewFromString(*cost)
				b, be := decimal.NewFromString(g.Rate)
				g.AtCost = be == nil && a.Equal(b)
			}
			if admin {
				g.CostMultiplier = cost
				g.CostVersion = version
				g.EffectiveAt = effective
			}
			i = len(out.Groups)
			indices[g.ID] = i
			out.Groups = append(out.Groups, g)
		}
		target := &out.Groups[i].Metrics
		switch window {
		case "today":
			target.Today = service.AddProfitMetric(target.Today, m)
			out.Metrics.Today = service.AddProfitMetric(out.Metrics.Today, m)
		case "week":
			target.Week = service.AddProfitMetric(target.Week, m)
			out.Metrics.Week = service.AddProfitMetric(out.Metrics.Week, m)
		default:
			target.Total = service.AddProfitMetric(target.Total, m)
			out.Metrics.Total = service.AddProfitMetric(out.Metrics.Total, m)
			if billing == int16(service.BillingTypeSubscription) {
				out.Subscription = service.AddProfitMetric(out.Subscription, m)
			} else {
				out.Balance = service.AddProfitMetric(out.Balance, m)
			}
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if e = rows.Close(); e != nil {
		return nil, e
	}
	for i := range out.Groups {
		if out.Groups[i].AtCost {
			out.Groups[i].ReferenceMargin = service.ProfitReferenceMargin(out.Metrics.Today.Margin)
		}
	}
	if !admin {
		out.Metrics.Total.Cost = ""
		out.Metrics.Today.Cost = ""
		out.Metrics.Week.Cost = ""
		out.Balance.Cost = ""
		out.Subscription.Cost = ""
		for i := range out.Groups {
			out.Groups[i].Metrics.Total.Cost = ""
			out.Groups[i].Metrics.Today.Cost = ""
			out.Groups[i].Metrics.Week.Cost = ""
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (r *siteProfitRepository) Contribution(ctx context.Context, userID int64) (*service.ProfitContribution, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	meta, e := profitMeta(ctx, tx)
	if e != nil {
		return nil, e
	}
	out := &service.ProfitContribution{StartedAt: meta.StartedAt, AsOf: meta.AsOf, Stale: meta.Stale, Total: service.EmptyProfitMetric(), Balance: service.EmptyProfitMetric(), Subscription: service.EmptyProfitMetric()}
	rows, e := tx.QueryContext(ctx, `SELECT billing_type,requests,known_requests,revenue_usd::text,known_revenue_usd::text,cost_usd::text,profit_usd::text FROM site_profit_user_totals WHERE user_id=$1`, userID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var kind int16
		var m service.ProfitMetric
		if e = rows.Scan(&kind, &m.Requests, &m.KnownRequests, &m.Revenue, &m.KnownRevenue, &m.Cost, &m.Profit); e != nil {
			return nil, e
		}
		m.Complete()
		out.Total = service.AddProfitMetric(out.Total, m)
		if kind == int16(service.BillingTypeSubscription) {
			out.Subscription = m
		} else {
			out.Balance = m
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	rows.Close()
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (r *siteProfitRepository) SetCost(ctx context.Context, group, actor int64, input service.ProfitCostInput) (*service.ProfitCostVersion, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var id int64
	// Configuration edits serialize only with other edits of this group; gateway cost reads are MVCC reads.
	e = tx.QueryRowContext(ctx, `SELECT id FROM groups WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, group).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, service.ErrProfitGroupMissing
	}
	if e != nil {
		return nil, e
	}
	var version int64
	e = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM site_profit_cost_versions WHERE group_id=$1`, group).Scan(&version)
	if e != nil {
		return nil, e
	}
	if version != input.Version {
		return nil, service.ErrProfitConflict
	}
	out := &service.ProfitCostVersion{Version: version + 1, Multiplier: input.Multiplier}
	e = tx.QueryRowContext(ctx, `INSERT INTO site_profit_cost_versions(group_id,version,cost_multiplier,actor_id,effective_at) VALUES($1,$2,$3,$4,clock_timestamp()) RETURNING effective_at`, group, out.Version, input.Multiplier, actor).Scan(&out.EffectiveAt)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (r *siteProfitRepository) Entries(ctx context.Context, f service.ProfitEntryFilter) (*service.ProfitEntryPage, error) {
	if f.Page < 1 || f.Page > 10000 {
		return nil, service.ErrProfitInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, e := r.db.QueryContext(ctx, `SELECT id,user_id,group_id,group_name,platform,billing_type,settled_at,base_usd::text,revenue_usd::text,cost_usd::text,profit_usd::text,cost_multiplier::text
 FROM site_profit_entries WHERE ($1::bigint=0 OR group_id=$1) AND ($2::timestamptz IS NULL OR settled_at>=$2) AND ($3::timestamptz IS NULL OR settled_at<$3)
 ORDER BY settled_at DESC,id DESC LIMIT 51 OFFSET $4`, f.GroupID, f.From, f.To, (f.Page-1)*50)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := &service.ProfitEntryPage{Items: []service.ProfitEntry{}, Page: f.Page}
	for rows.Next() {
		var v service.ProfitEntry
		if e = rows.Scan(&v.ID, &v.UserID, &v.GroupID, &v.GroupName, &v.Platform, &v.BillingType, &v.SettledAt, &v.Base, &v.Revenue, &v.Cost, &v.Profit, &v.CostMultiplier); e != nil {
			return nil, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out.Items) > 50 {
		out.HasMore = true
		out.Items = out.Items[:50]
	}
	return out, nil
}

// One advisory lease serializes reporting batches only, never request settlement. Row claims remain crash-safe.
func (r *siteProfitRepository) Aggregate(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 2000 {
		return 0, service.ErrProfitInvalid
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	var locked bool
	if e = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(193701,1)`).Scan(&locked); e != nil {
		return 0, e
	}
	if !locked {
		return 0, nil
	}
	rows, e := tx.QueryContext(ctx, `SELECT id FROM site_profit_entries WHERE NOT aggregated ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if e != nil {
		return 0, e
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	if len(ids) > 0 {
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if _, e = tx.ExecContext(ctx, profitAggregateSQL, pq.Array(ids)); e != nil {
			return 0, e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE site_profit_meta SET aggregated_at=statement_timestamp(),backlog=EXISTS(SELECT 1 FROM site_profit_entries WHERE NOT aggregated LIMIT 1) WHERE id=1`); e != nil {
		return 0, e
	}
	if e = tx.Commit(); e != nil {
		return 0, e
	}
	return len(ids), nil
}

const profitAggregateSQL = `WITH facts AS (
 UPDATE site_profit_entries SET aggregated=true WHERE id=ANY($1::bigint[]) AND NOT aggregated RETURNING *
),daily AS (
 INSERT INTO site_profit_group_daily(group_id,day,billing_type,requests,known_requests,revenue_usd,known_revenue_usd,cost_usd,profit_usd)
 SELECT group_id,day,billing_type,COUNT(*),COUNT(cost_usd),SUM(revenue_usd),COALESCE(SUM(revenue_usd) FILTER(WHERE cost_usd IS NOT NULL),0),COALESCE(SUM(cost_usd),0),COALESCE(SUM(profit_usd),0) FROM facts GROUP BY group_id,day,billing_type ORDER BY group_id,day,billing_type
 ON CONFLICT(group_id,day,billing_type) DO UPDATE SET requests=site_profit_group_daily.requests+EXCLUDED.requests,known_requests=site_profit_group_daily.known_requests+EXCLUDED.known_requests,revenue_usd=site_profit_group_daily.revenue_usd+EXCLUDED.revenue_usd,known_revenue_usd=site_profit_group_daily.known_revenue_usd+EXCLUDED.known_revenue_usd,cost_usd=site_profit_group_daily.cost_usd+EXCLUDED.cost_usd,profit_usd=site_profit_group_daily.profit_usd+EXCLUDED.profit_usd RETURNING group_id
),totals AS (
 INSERT INTO site_profit_group_totals(group_id,billing_type,group_name,platform,requests,known_requests,revenue_usd,known_revenue_usd,cost_usd,profit_usd)
 SELECT group_id,billing_type,MAX(group_name),MAX(platform),COUNT(*),COUNT(cost_usd),SUM(revenue_usd),COALESCE(SUM(revenue_usd) FILTER(WHERE cost_usd IS NOT NULL),0),COALESCE(SUM(cost_usd),0),COALESCE(SUM(profit_usd),0) FROM facts GROUP BY group_id,billing_type ORDER BY group_id,billing_type
 ON CONFLICT(group_id,billing_type) DO UPDATE SET requests=site_profit_group_totals.requests+EXCLUDED.requests,known_requests=site_profit_group_totals.known_requests+EXCLUDED.known_requests,revenue_usd=site_profit_group_totals.revenue_usd+EXCLUDED.revenue_usd,known_revenue_usd=site_profit_group_totals.known_revenue_usd+EXCLUDED.known_revenue_usd,cost_usd=site_profit_group_totals.cost_usd+EXCLUDED.cost_usd,profit_usd=site_profit_group_totals.profit_usd+EXCLUDED.profit_usd RETURNING group_id
)
 INSERT INTO site_profit_user_totals(user_id,billing_type,requests,known_requests,revenue_usd,known_revenue_usd,cost_usd,profit_usd)
 SELECT user_id,billing_type,COUNT(*),COUNT(cost_usd),SUM(revenue_usd),COALESCE(SUM(revenue_usd) FILTER(WHERE cost_usd IS NOT NULL),0),COALESCE(SUM(cost_usd),0),COALESCE(SUM(profit_usd),0) FROM facts GROUP BY user_id,billing_type ORDER BY user_id,billing_type
 ON CONFLICT(user_id,billing_type) DO UPDATE SET requests=site_profit_user_totals.requests+EXCLUDED.requests,known_requests=site_profit_user_totals.known_requests+EXCLUDED.known_requests,revenue_usd=site_profit_user_totals.revenue_usd+EXCLUDED.revenue_usd,known_revenue_usd=site_profit_user_totals.known_revenue_usd+EXCLUDED.known_revenue_usd,cost_usd=site_profit_user_totals.cost_usd+EXCLUDED.cost_usd,profit_usd=site_profit_user_totals.profit_usd+EXCLUDED.profit_usd`
