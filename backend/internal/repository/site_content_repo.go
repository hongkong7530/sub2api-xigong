package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"strconv"
	"strings"
	"time"
)

type siteContentRepository struct{ db *sql.DB }

func NewSiteContentRepository(db *sql.DB) service.SiteContentRepository {
	return &siteContentRepository{db}
}
func contentError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrContentMissing
	}
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return service.ErrContentConflict
	}
	return err
}
func scanDocument(row interface{ Scan(...any) error }) (service.SiteDocument, error) {
	var d service.SiteDocument
	var draft, published []byte
	err := row.Scan(&d.ID, &d.Slug, &draft, &published, &d.Version, &d.UpdatedAt, &d.PublishedAt)
	if err != nil {
		return d, contentError(err)
	}
	if err = json.Unmarshal(draft, &d.Draft); err != nil {
		return d, err
	}
	if len(published) > 0 {
		err = json.Unmarshal(published, &d.Published)
	}
	return d, err
}
func (r *siteContentRepository) PublishedDocument(ctx context.Context, slug string) (*service.SiteDocument, error) {
	d, e := scanDocument(r.db.QueryRowContext(ctx, "SELECT id,slug,'{}'::jsonb,published,version,updated_at,published_at FROM site_documents WHERE slug=$1 AND published IS NOT NULL", slug))
	return &d, e
}

const documentColumns = "id,slug,draft,published,version,updated_at,published_at"

// 列表仅投影元数据；正文详情单独读取，空搜索不扫描正文。
func (r *siteContentRepository) Documents(ctx context.Context, public bool, query string, page int) ([]service.SiteDocument, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	body, where := "draft", "TRUE"
	if public {
		body, where = "published", "published IS NOT NULL"
	}
	projection := "jsonb_build_object('title'," + body + "->>'title','summary'," + body + "->>'summary','category_id'," + body + "->'category_id','position'," + body + "->'position')"
	columns := "id,slug," + projection + ",CASE WHEN published IS NULL THEN NULL ELSE '{}'::jsonb END,version,updated_at,published_at"
	if public {
		columns = "id,slug,'{}'::jsonb," + projection + ",version,updated_at,published_at"
	}
	args := []any{}
	if query != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
		where += " AND concat_ws(' '," + body + "->>'title'," + body + "->>'summary'," + body + "->>'content') ILIKE $1"
		args = append(args, "%"+escaped+"%")
	}
	if page < 1 {
		page = 1
	}
	args = append(args, (page-1)*50)
	rows, err := r.db.QueryContext(ctx, "SELECT "+columns+" FROM site_documents WHERE "+where+" ORDER BY COALESCE(("+body+"->>'position')::bigint,0),id LIMIT 51 OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := make([]service.SiteDocument, 0, 51)
	for rows.Next() {
		d, e := scanDocument(rows)
		if e != nil {
			return nil, false, e
		}
		result = append(result, d)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(result) > 50
	if more {
		result = result[:50]
	}
	return result, more, nil
}
func (r *siteContentRepository) CompareAndSwapAppearance(ctx context.Context, version int, raw string) error {
	return changed(r.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) SELECT $1::varchar,$2::text,now() WHERE $3::int=0 OR EXISTS(SELECT 1 FROM settings WHERE key=$1::varchar) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now() WHERE COALESCE((settings.value::jsonb->>'version')::int,0)=$3`, service.SiteAppearanceKey, raw, version))
}
func (r *siteContentRepository) Document(ctx context.Context, id int64) (*service.SiteDocument, error) {
	d, e := scanDocument(r.db.QueryRowContext(ctx, "SELECT "+documentColumns+" FROM site_documents WHERE id=$1", id))
	return &d, e
}
func (r *siteContentRepository) SaveDocument(ctx context.Context, d *service.SiteDocument) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if d.Draft.CategoryID != 0 {
		var id int64
		if e := tx.QueryRowContext(ctx, "SELECT id FROM site_doc_categories WHERE id=$1 FOR KEY SHARE", d.Draft.CategoryID).Scan(&id); e != nil {
			return contentError(e)
		}
	}
	body, err := json.Marshal(d.Draft)
	if err != nil {
		return err
	}
	if d.ID == 0 {
		err = tx.QueryRowContext(ctx, "INSERT INTO site_documents(slug,draft,updated_at) VALUES($1,$2,now()) RETURNING id,version,updated_at", d.Slug, string(body)).Scan(&d.ID, &d.Version, &d.UpdatedAt)
	} else {
		// A stable slug prevents saved links from silently breaking; changing it requires a new document.
		err = tx.QueryRowContext(ctx, "UPDATE site_documents SET draft=$1,version=version+1,updated_at=now() WHERE id=$2 AND version=$3 AND slug=$4 RETURNING version,updated_at", string(body), d.ID, d.Version, d.Slug).Scan(&d.Version, &d.UpdatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrContentConflict
		}
	}
	if err != nil {
		return contentError(err)
	}
	return tx.Commit()
}
func changed(result sql.Result, err error) error {
	if err != nil {
		return contentError(err)
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return service.ErrContentConflict
	}
	return nil
}
func (r *siteContentRepository) PublishDocument(ctx context.Context, id int64, version int, publish bool) error {
	if publish {
		return changed(r.db.ExecContext(ctx, "UPDATE site_documents SET published=draft,published_at=now(),updated_at=now(),version=version+1 WHERE id=$1 AND version=$2", id, version))
	}
	return changed(r.db.ExecContext(ctx, "UPDATE site_documents SET published=NULL,published_at=NULL,updated_at=now(),version=version+1 WHERE id=$1 AND version=$2", id, version))
}
func (r *siteContentRepository) DeleteDocument(ctx context.Context, id int64, version int) error {
	return changed(r.db.ExecContext(ctx, "DELETE FROM site_documents WHERE id=$1 AND version=$2", id, version))
}
func (r *siteContentRepository) Categories(ctx context.Context) ([]service.SiteCategory, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, e := r.db.QueryContext(ctx, "SELECT id,name,position,version FROM site_doc_categories ORDER BY position,id LIMIT 201")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]service.SiteCategory, 0)
	for rows.Next() {
		var c service.SiteCategory
		if e := rows.Scan(&c.ID, &c.Name, &c.Position, &c.Version); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	if len(out) > 200 {
		return nil, service.ErrContentInvalid
	}
	return out, rows.Err()
}
func (r *siteContentRepository) SaveCategory(ctx context.Context, c *service.SiteCategory) error {
	if c.ID == 0 {
		tx, e := r.beginContentCreate(ctx, "site_doc_categories")
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = tx.QueryRowContext(ctx, "INSERT INTO site_doc_categories(name,position) VALUES($1,$2) RETURNING id,version", c.Name, c.Position).Scan(&c.ID, &c.Version); e != nil {
			return contentError(e)
		}
		return tx.Commit()
	}
	err := r.db.QueryRowContext(ctx, "UPDATE site_doc_categories SET name=$1,position=$2,version=version+1 WHERE id=$3 AND version=$4 RETURNING version", c.Name, c.Position, c.ID, c.Version).Scan(&c.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrContentConflict
	}
	return err
}
func (r *siteContentRepository) DeleteCategory(ctx context.Context, id int64, version int) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var locked int64
	if e = tx.QueryRowContext(ctx, "SELECT id FROM site_doc_categories WHERE id=$1 AND version=$2 FOR UPDATE", id, version).Scan(&locked); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return service.ErrContentConflict
		}
		return e
	}
	var used bool
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM site_documents WHERE (draft->>'category_id')::bigint=$1 OR (published->>'category_id')::bigint=$1)", id).Scan(&used); e != nil {
		return e
	}
	if used {
		return service.ErrContentConflict
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM site_doc_categories WHERE id=$1", id); e != nil {
		return e
	}
	return tx.Commit()
}
func (r *siteContentRepository) Partners(ctx context.Context, public bool) ([]service.SitePartner, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, e := r.db.QueryContext(ctx, "SELECT id,name,description,url,icon,tag,position,enabled,version,thumbnail FROM site_partners WHERE ($1=false OR enabled) ORDER BY position,id LIMIT 201", public)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]service.SitePartner, 0)
	for rows.Next() {
		var p service.SitePartner
		if e := rows.Scan(&p.ID, &p.Name, &p.Description, &p.URL, &p.Icon, &p.Tag, &p.Position, &p.Enabled, &p.Version, &p.Thumbnail); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	if len(out) > 200 {
		return nil, service.ErrContentInvalid
	}
	return out, rows.Err()
}
func (r *siteContentRepository) SavePartner(ctx context.Context, p *service.SitePartner) error {
	args := []any{p.Name, p.Description, p.URL, p.Icon, p.Tag, p.Position, p.Enabled, p.Thumbnail}
	query := "INSERT INTO site_partners(name,description,url,icon,tag,position,enabled,thumbnail) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,version"
	if p.ID != 0 {
		query = "UPDATE site_partners SET name=$1,description=$2,url=$3,icon=$4,tag=$5,position=$6,enabled=$7,thumbnail=$8,version=version+1 WHERE id=$9 AND version=$10 RETURNING id,version"
		args = append(args, p.ID, p.Version)
	}
	if p.ID == 0 {
		tx, e := r.beginContentCreate(ctx, "site_partners")
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = tx.QueryRowContext(ctx, query, args...).Scan(&p.ID, &p.Version); e != nil {
			return contentError(e)
		}
		return tx.Commit()
	}
	e := r.db.QueryRowContext(ctx, query, args...).Scan(&p.ID, &p.Version)
	if errors.Is(e, sql.ErrNoRows) {
		return service.ErrContentConflict
	}
	return contentError(e)
}
func (r *siteContentRepository) DeletePartner(ctx context.Context, id int64, version int) error {
	return changed(r.db.ExecContext(ctx, "DELETE FROM site_partners WHERE id=$1 AND version=$2", id, version))
}

// Only fixed internal table identifiers reach this bounded administrative creation path.
func (r *siteContentRepository) beginContentCreate(ctx context.Context, table string) (*sql.Tx, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, "LOCK TABLE "+table+" IN SHARE ROW EXCLUSIVE MODE"); e != nil {
		tx.Rollback()
		return nil, e
	}
	var count int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); e != nil {
		tx.Rollback()
		return nil, e
	}
	if count >= 200 {
		tx.Rollback()
		return nil, service.ErrContentInvalid
	}
	return tx, nil
}
