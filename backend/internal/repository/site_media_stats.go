package repository

import (
	"context"
	"database/sql"
	"regexp"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *siteContentRepository) SaveSiteAsset(ctx context.Context, id, mime string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 跨节点序列化配额检查；不允许并发上传绕过总字节预算。
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(27716001)"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM site_assets WHERE id=$1)", id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	var count, total int64
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(octet_length(body)),0) FROM site_assets").Scan(&count, &total); err != nil {
		return err
	}
	if count >= 256 || total+int64(len(body)) > 256<<20 {
		return service.ErrContentInvalid
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO site_assets(id,content_type,body) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", id, mime, body); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *siteContentRepository) SiteAsset(ctx context.Context, id string) (string, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var mime string
	var body []byte
	err := r.db.QueryRowContext(ctx, "SELECT content_type,body FROM site_assets WHERE id=$1", id).Scan(&mime, &body)
	return mime, body, contentError(err)
}

var publicModelName = regexp.MustCompile(`(?i)^(gpt-[0-9][a-z0-9.\-]*|claude-(?:opus|sonnet|haiku)[a-z0-9.\-]*|gemini-[0-9][a-z0-9.\-]*|deepseek-[a-z0-9.\-]+|grok-[0-9][a-z0-9.\-]*|kimi-[a-z0-9.\-]+|glm-[0-9][a-z0-9.\-]*|qwen[0-9][a-z0-9.\-]*)$`)

func (r *siteContentRepository) PublicSiteUsage(ctx context.Context, start, end time.Time) (*service.SiteUsageSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := &service.SiteUsageSnapshot{SampledAt: end, Day: start.Format("2006-01-02"), Models: []service.SiteModelUsage{}}
	var priceTokens int64
	var cost float64
	// 同一快照读取明细和保留汇总；累计不受后台90天窗口配置影响。
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(request_count),0),COALESCE(SUM(input_tokens+output_tokens+cache_creation_tokens+cache_read_tokens) FILTER(WHERE created_at >= $1),0),COALESCE(SUM(actual_cost) FILTER(WHERE created_at >= $1 AND billing_mode='token'),0),COALESCE(SUM(input_tokens+output_tokens+cache_creation_tokens+cache_read_tokens) FILTER(WHERE created_at >= $1 AND billing_mode='token'),0),COALESCE(SUM(request_count) FILTER(WHERE created_at >= $1 AND billing_mode='token'),0) FROM usage_statistics WHERE created_at < $2`, start, end).Scan(&out.Requests, &out.TodayTokens, &cost, &priceTokens, &out.PriceSamples)
	if err != nil {
		return nil, err
	}
	if priceTokens > 0 && out.PriceSamples >= 10 {
		v := cost * 1000000 / float64(priceTokens)
		out.AveragePerMillion = &v
	}
	rows, err := tx.QueryContext(ctx, `SELECT COALESCE(NULLIF(TRIM(requested_model),''),model,''),SUM(input_tokens+output_tokens+cache_creation_tokens+cache_read_tokens),SUM(request_count) FROM usage_statistics WHERE created_at >= $1 AND created_at < $2 GROUP BY 1 ORDER BY 2 DESC,1 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m service.SiteModelUsage
		if err = rows.Scan(&m.Model, &m.Tokens, &m.Requests); err != nil {
			rows.Close()
			return nil, err
		}
		if publicModelName.MatchString(m.Model) && len(out.Models) < 5 {
			out.Models = append(out.Models, m)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
