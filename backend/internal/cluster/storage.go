package cluster

import (
	"context"
	"fmt"
	"time"
)

type StorageTable struct {
	Name           string `json:"name"`
	Bytes          int64  `json:"bytes"`
	EstimatedRows  int64  `json:"estimated_rows"`
	DeadRows       int64  `json:"dead_rows"`
	RetentionClass string `json:"retention_class"`
}
type StorageView struct {
	SampledAt       time.Time      `json:"sampled_at"`
	DatabaseBytes   int64          `json:"database_bytes"`
	Tables          []StorageTable `json:"tables"`
	LedgerDetached  bool           `json:"ledger_detached"`
	DeletionEnabled bool           `json:"deletion_enabled"`
}

func (s *Store) Storage(ctx context.Context) (*StorageView, error) {
	view := &StorageView{SampledAt: time.Now().UTC(), Tables: []StorageTable{}}
	if err := s.DB.QueryRowContext(ctx, `SELECT pg_database_size(current_database()),EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='billing_usage_entries'::regclass AND conname='billing_usage_entries_usage_log_id_fkey' AND confdeltype='n')`).Scan(&view.DatabaseBytes, &view.LedgerDetached); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT relname,pg_total_relation_size(relid),n_live_tup,n_dead_tup FROM pg_stat_user_tables WHERE schemaname='public' AND relname=ANY(ARRAY['usage_logs','ops_error_logs','ops_system_logs','billing_usage_entries','usage_billing_dedup','usage_billing_dedup_archive','cluster_settlement_events']) ORDER BY pg_total_relation_size(relid) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row StorageTable
		if err := rows.Scan(&row.Name, &row.Bytes, &row.EstimatedRows, &row.DeadRows); err != nil {
			return nil, err
		}
		row.RetentionClass = "durable"
		if row.Name == "usage_logs" {
			row.RetentionClass = "request_detail"
		}
		if row.Name == "ops_error_logs" || row.Name == "ops_system_logs" {
			row.RetentionClass = "diagnostic"
		}
		view.Tables = append(view.Tables, row)
	}
	return view, rows.Err()
}

type RetentionPreview struct {
	SampledAt                 time.Time `json:"sampled_at"`
	Cutoff                    time.Time `json:"cutoff"`
	Days                      int       `json:"days"`
	Table                     string    `json:"table"`
	Rows                      int64     `json:"rows"`
	EstimatedReusableBytes    int64     `json:"estimated_reusable_bytes"`
	FilesystemBytesGuaranteed int64     `json:"filesystem_bytes_guaranteed"`
	CanApply                  bool      `json:"can_apply"`
}

func (s *Store) PreviewRetention(ctx context.Context, table string, days int) (*RetentionPreview, error) {
	if days < 1 || days > 3650 {
		return nil, fmt.Errorf("retention must be between 1 and 3650 days")
	}
	if table != "usage_logs" && table != "ops_error_logs" && table != "ops_system_logs" {
		return nil, fmt.Errorf("table is not an approved retention target")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	preview := &RetentionPreview{SampledAt: time.Now().UTC(), Days: days, Table: table}
	preview.Cutoff = preview.SampledAt.AddDate(0, 0, -days)
	// Explicit preview only; never COUNT a large table on each live dashboard refresh.
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE created_at<$1`, preview.Cutoff).Scan(&preview.Rows); err != nil {
		return nil, err
	}
	var bytes, rows int64
	if err := s.DB.QueryRowContext(ctx, `SELECT pg_total_relation_size(relid),GREATEST(n_live_tup,1) FROM pg_stat_user_tables WHERE schemaname='public' AND relname=$1`, table).Scan(&bytes, &rows); err != nil {
		return nil, err
	}
	preview.EstimatedReusableBytes = min(bytes, bytes/rows*preview.Rows)
	if table == "usage_logs" {
		var safe bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='billing_usage_entries'::regclass AND conname='billing_usage_entries_usage_log_id_fkey' AND confdeltype='n')`).Scan(&safe); err != nil {
			return nil, err
		}
		preview.CanApply = safe
	} else {
		preview.CanApply = true
	}
	return preview, nil
}
