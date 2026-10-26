package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type botRepository struct{ db *sql.DB }

func NewBotRepository(db *sql.DB) service.BotRepository { return &botRepository{db: db} }

func (r *botRepository) Create(ctx context.Context, v *service.BotInstallation) error {
	policy, e := json.Marshal(v.BotPolicy)
	if e != nil {
		return e
	}
	_, e = r.db.ExecContext(ctx, `INSERT INTO bot_installations(id,policy,token_hash,signing_ciphertext,scope,expires_at,created_at) VALUES($1,$2,$3,$4,'bot:read',$5,$6)`, v.ID, string(policy), v.TokenHash, v.SigningCiphertext, v.ExpiresAt, v.CreatedAt)
	return e
}
func scanBot(row interface{ Scan(...any) error }) (*service.BotInstallation, error) {
	v := &service.BotInstallation{}
	var raw []byte
	e := row.Scan(&v.ID, &raw, &v.TokenHash, &v.SigningCiphertext, &v.Scope, &v.ExpiresAt, &v.CreatedAt, &v.RevokedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	expires := v.ExpiresAt
	if e = json.Unmarshal(raw, &v.BotPolicy); e != nil {
		return nil, e
	}
	v.ExpiresAt = expires
	return v, nil
}

const botColumns = `id,policy,token_hash,signing_ciphertext,scope,expires_at,created_at,revoked_at`

func (r *botRepository) Get(ctx context.Context, id string) (*service.BotInstallation, error) {
	return scanBot(r.db.QueryRowContext(ctx, `SELECT `+botColumns+` FROM bot_installations WHERE id=$1`, id))
}
func (r *botRepository) List(ctx context.Context) ([]service.BotInstallation, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT `+botColumns+` FROM bot_installations ORDER BY created_at DESC,id LIMIT 1000`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []service.BotInstallation{}
	for rows.Next() {
		v, e := scanBot(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (r *botRepository) Revoke(ctx context.Context, id string) error {
	_, e := r.db.ExecContext(ctx, `UPDATE bot_installations SET revoked_at=COALESCE(revoked_at,statement_timestamp()) WHERE id=$1`, id)
	return e
}

type botSQLFilter struct {
	where string
	args  []any
}

func (b *botSQLFilter) add(clause string, value any) {
	b.args = append(b.args, value)
	b.where += " AND " + fmt.Sprintf(clause, "$"+strconv.Itoa(len(b.args)))
}
func botDataFilter(v *service.BotInstallation, q service.BotQuery, user, group, platform string) botSQLFilter {
	b := botSQLFilter{where: "true"}
	if !v.AllData {
		if len(v.UserIDs) > 0 {
			if user == "" {
				b.where = "false"
			} else {
				b.add(user+"=ANY(%s::bigint[])", pq.Array(v.UserIDs))
			}
		}
		if len(v.GroupIDs) > 0 {
			b.add(group+"=ANY(%s::bigint[])", pq.Array(v.GroupIDs))
		}
		if len(v.UserIDs) == 0 && len(v.GroupIDs) == 0 {
			b.where = "false"
		}
	}
	if q.ID("user_id") > 0 && user != "" {
		b.add(user+"=%s", q.ID("user_id"))
	}
	if q.ID("group_id") > 0 {
		b.add(group+"=%s", q.ID("group_id"))
	}
	if p := q.Values.Get("platform"); p != "" {
		b.add(platform+"=%s", p)
	}
	return b
}

func (r *botRepository) Query(ctx context.Context, route string, v *service.BotInstallation, q service.BotQuery) (json.RawMessage, error) {
	tx, e := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	// Bound server execution even when the client disconnects late; no bot query may become an unbounded report.
	if _, e = tx.ExecContext(ctx, `SET LOCAL statement_timeout='8000ms'`); e != nil {
		return nil, e
	}
	var out json.RawMessage
	switch route {
	case "costs", "profit":
		out, e = botFinance(ctx, tx, route, v, q)
	case "consumption", "logs", "model-audit":
		out, e = botUsage(ctx, tx, route, v, q)
	case "accounts/status", "channels/status", "models/prices":
		out, e = botInventory(ctx, tx, route, v, q)
	default:
		e = service.ErrBotUnsupported
	}
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}

func botFinance(ctx context.Context, tx *sql.Tx, route string, v *service.BotInstallation, q service.BotQuery) (json.RawMessage, error) {
	b := botDataFilter(v, q, "p.user_id", "p.group_id", "p.platform")
	b.add("p.settled_at>=%s", q.Start)
	b.add("p.settled_at<%s", q.End)
	if t := q.Values.Get("type"); t != "" {
		kind := 0
		if t == "subscription" {
			kind = int(service.BillingTypeSubscription)
		}
		b.add("p.billing_type=%s", kind)
	}
	var metrics []byte
	e := tx.QueryRowContext(ctx, `SELECT jsonb_build_object('requests',COUNT(*),'known_requests',COUNT(cost_usd),'revenue_usd',COALESCE(SUM(revenue_usd),0)::text,'known_revenue_usd',COALESCE(SUM(revenue_usd) FILTER(WHERE cost_usd IS NOT NULL),0)::text,'known_cost_usd',SUM(cost_usd)::text,'known_profit_usd',SUM(profit_usd)::text,'unknown_cost_requests',COUNT(*)-COUNT(cost_usd)) FROM site_profit_entries p WHERE `+b.where, b.args...).Scan(&metrics)
	if e != nil {
		return nil, e
	}
	// Fetch only authorized facts before aggregation. Global dashboard totals would leak hidden users/groups by subtraction.
	items, more, e := botReadPage(ctx, tx, `SELECT jsonb_build_object('group_id',p.group_id,'platform',p.platform,'billing_type',p.billing_type,'requests',COUNT(*),'known_requests',COUNT(cost_usd),'revenue_usd',SUM(revenue_usd)::text,'known_cost_usd',SUM(cost_usd)::text,'known_profit_usd',SUM(profit_usd)::text) FROM site_profit_entries p WHERE `+b.where+` GROUP BY p.group_id,p.platform,p.billing_type ORDER BY p.group_id,p.platform,p.billing_type`, b.args, q)
	if e != nil {
		return nil, e
	}
	var started time.Time
	var zone string
	if e = tx.QueryRowContext(ctx, `SELECT started_at,timezone FROM site_profit_meta WHERE id=1`).Scan(&started, &zone); e != nil {
		return nil, e
	}
	return json.Marshal(map[string]any{"query": route, "currency": "USD", "amount_encoding": "decimal_string", "basis": "committed_usage_billing_margin_not_cash_profit", "start": q.Start, "end_exclusive": q.End, "timezone": zone, "started_at": started, "history_complete": !q.Start.Before(started), "metrics": json.RawMessage(metrics), "items": items, "page": q.Page, "page_size": q.PageSize, "has_more": more})
}

func botUsage(ctx context.Context, tx *sql.Tx, route string, v *service.BotInstallation, q service.BotQuery) (json.RawMessage, error) {
	b := botDataFilter(v, q, "u.user_id", "u.group_id", "g.platform")
	b.add("u.created_at>=%s", q.Start)
	b.add("u.created_at<%s", q.End)
	for _, f := range []struct{ key, column string }{{"account_id", "u.account_id"}, {"channel_id", "u.channel_id"}} {
		if q.ID(f.key) > 0 {
			b.add(f.column+"=%s", q.ID(f.key))
		}
	}
	for _, f := range []struct{ key, column string }{{"requested_model", "COALESCE(NULLIF(TRIM(u.requested_model),''),u.model)"}, {"upstream_model", "COALESCE(NULLIF(TRIM(u.upstream_model),''),u.model)"}, {"upstream_response_model", "u.upstream_response_model"}} {
		if value := q.Values.Get(f.key); value != "" {
			b.add(f.column+"=%s", value)
		}
	}
	if value := q.Values.Get("mismatch"); value != "" {
		b.add("u.upstream_model_mismatch=%s", value == "true")
	}
	base := ` FROM usage_logs u LEFT JOIN groups g ON g.id=u.group_id WHERE ` + b.where
	if route == "consumption" {
		var result []byte
		e := tx.QueryRowContext(ctx, `SELECT jsonb_build_object('source','retained_usage_details_only','history_complete',false,'currency','USD','start',$`+strconv.Itoa(len(b.args)+1)+`::timestamptz,'end_exclusive',$`+strconv.Itoa(len(b.args)+2)+`::timestamptz,'requests',COUNT(*),'input_tokens',COALESCE(SUM(u.input_tokens),0),'output_tokens',COALESCE(SUM(u.output_tokens),0),'cache_read_tokens',COALESCE(SUM(u.cache_read_tokens),0),'cache_creation_tokens',COALESCE(SUM(u.cache_creation_tokens),0),'billed_cost_usd',COALESCE(SUM(u.actual_cost),0)::text,'base_cost_usd',COALESCE(SUM(u.total_cost),0)::text)`+base, append(b.args, q.Start, q.End)...).Scan(&result)
		return json.RawMessage(result), e
	}
	items, more, e := botReadPage(ctx, tx, `SELECT jsonb_build_object('id',u.id,'created_at',u.created_at,'user_id',u.user_id,'group_id',u.group_id,'account_id',u.account_id,'channel_id',u.channel_id,'requested_model',COALESCE(NULLIF(TRIM(u.requested_model),''),u.model),'upstream_model',COALESCE(NULLIF(TRIM(u.upstream_model),''),u.model),'upstream_response_model',u.upstream_response_model,'upstream_model_mismatch',u.upstream_model_mismatch,'input_tokens',u.input_tokens,'output_tokens',u.output_tokens,'cache_read_tokens',u.cache_read_tokens,'cache_creation_tokens',u.cache_creation_tokens,'base_cost_usd',u.total_cost::text,'billed_cost_usd',u.actual_cost::text,'first_token_ms',u.first_token_ms,'duration_ms',u.duration_ms)`+base+` ORDER BY u.created_at DESC,u.id DESC`, b.args, q)
	if e != nil {
		return nil, e
	}
	return json.Marshal(map[string]any{"items": items, "page": q.Page, "page_size": q.PageSize, "has_more": more, "source": "retained_usage_details_only", "history_complete": false, "upstream_response_model_is_observed_declaration": true})
}

func botInventory(ctx context.Context, tx *sql.Tx, route string, v *service.BotInstallation, q service.BotQuery) (json.RawMessage, error) {
	b := botDataFilter(v, q, "", "g.id", "g.platform")
	b.where += " AND g.deleted_at IS NULL"
	var query string
	switch route {
	case "accounts/status":
		if id := q.ID("account_id"); id > 0 {
			b.add("a.id=%s", id)
		}
		if t := q.Values.Get("type"); t != "" {
			b.add("a.type=%s", t)
		}
		query = `SELECT jsonb_build_object('id',a.id,'platform',a.platform,'type',a.type,'status',a.status,'status_kind','configured_state_not_runtime_health','schedulable',a.schedulable,'rate_limit_reset_at',a.rate_limit_reset_at,'overload_until',a.overload_until,'last_used_at',a.last_used_at) FROM accounts a WHERE a.deleted_at IS NULL AND EXISTS(SELECT 1 FROM account_groups ag JOIN groups g ON g.id=ag.group_id WHERE ag.account_id=a.id AND ` + b.where + `) ORDER BY a.id`
	case "channels/status":
		if id := q.ID("channel_id"); id > 0 {
			b.add("c.id=%s", id)
		}
		query = `SELECT jsonb_build_object('id',c.id,'status',c.status,'status_kind','configured_state_not_runtime_health') FROM channels c WHERE EXISTS(SELECT 1 FROM channel_groups cg JOIN groups g ON g.id=cg.group_id WHERE cg.channel_id=c.id AND ` + b.where + `) ORDER BY c.id`
	case "models/prices":
		// Do not export arbitrary model_pricing JSON: descriptions/URLs and future credential-bearing fields are not part of this projection.
		query = `SELECT jsonb_build_object('group_id',g.id,'platform',g.platform,'rate_multiplier',g.rate_multiplier::text,'configuration_only',true,'resolved_effective_price',false,'configured_model_prices',COALESCE((SELECT jsonb_agg(jsonb_build_object('models',p->'models','input_price',p->'input_price','output_price',p->'output_price','cache_write_price',p->'cache_write_price','cache_read_price',p->'cache_read_price','long_context_input_threshold',p->'long_context_input_threshold')) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(g.model_pricing)='array' THEN g.model_pricing ELSE '[]'::jsonb END) p),'[]'::jsonb)) FROM groups g WHERE ` + b.where + ` ORDER BY g.id`
	default:
		return nil, service.ErrBotUnsupported
	}
	items, more, e := botReadPage(ctx, tx, query, b.args, q)
	if e != nil {
		return nil, e
	}
	return json.Marshal(map[string]any{"items": items, "page": q.Page, "page_size": q.PageSize, "has_more": more, "scope": "authorized_groups"})
}

func botReadPage(ctx context.Context, tx *sql.Tx, query string, args []any, q service.BotQuery) ([]json.RawMessage, bool, error) {
	n := len(args)
	query += " LIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2)
	args = append(args, q.PageSize+1, (q.Page-1)*q.PageSize)
	rows, e := tx.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, false, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	bytes := 0
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			return nil, false, e
		}
		bytes += len(raw)
		if bytes > service.BotResponseLimit {
			return nil, false, service.ErrBotInvalid
		}
		out = append(out, json.RawMessage(raw))
	}
	if e = rows.Err(); e != nil {
		return nil, false, e
	}
	more := len(out) > q.PageSize
	if more {
		out = out[:q.PageSize]
	}
	return out, more, nil
}

type botRedisGuard struct{ client *redis.Client }

func NewBotRequestGuard(client *redis.Client) service.BotRequestGuard {
	return &botRedisGuard{client: client}
}

var botConsumeScript = redis.NewScript(`
if not redis.call('SET',KEYS[1],'1','NX','PX',ARGV[1]) then return 0 end
local count=redis.call('INCR',KEYS[2])
if count==1 then redis.call('PEXPIRE',KEYS[2],60000) end
if count>tonumber(ARGV[2]) then return 2 end
return 1
`)

func (g *botRedisGuard) Consume(ctx context.Context, id, nonce string, ttl time.Duration, limit int) (bool, bool, error) {
	if g.client == nil {
		return false, false, errors.New("bot replay store unavailable")
	}
	key := "subnexus:{" + id + "}:"
	value, e := botConsumeScript.Run(ctx, g.client, []string{key + "nonce:" + nonce, key + "rate"}, ttl.Milliseconds(), limit).Int()
	return value != 0, value == 2, e
}
