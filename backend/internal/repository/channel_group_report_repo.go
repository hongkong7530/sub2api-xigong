package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type channelGroupReportRepository struct {
	db *sql.DB
}

func NewChannelGroupReportRepository(db *sql.DB) service.ChannelGroupReportRepository {
	return &channelGroupReportRepository{db: db}
}

func (r *channelGroupReportRepository) Create(ctx context.Context, report *service.ChannelGroupReport) error {
	if report == nil {
		return fmt.Errorf("report is nil")
	}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO channel_group_reports (user_id, group_id, category, body, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id`,
		report.UserID, report.GroupID, report.Category, report.Body, report.CreatedAt,
	).Scan(&report.ID)
	if err != nil {
		return fmt.Errorf("create channel group report: %w", err)
	}
	return nil
}

func (r *channelGroupReportRepository) LatestByUserAndGroup(ctx context.Context, userID, groupID int64) (*service.ChannelGroupReport, error) {
	item, err := r.scanOne(ctx, `
SELECT id, user_id, group_id, category, body, created_at
FROM channel_group_reports
WHERE user_id = $1 AND group_id = $2
ORDER BY created_at DESC
LIMIT 1`, userID, groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *channelGroupReportRepository) ListByGroup(ctx context.Context, groupID int64, limit int) ([]*service.ChannelGroupReport, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, user_id, group_id, category, body, created_at
FROM channel_group_reports
WHERE group_id = $1
ORDER BY created_at DESC
LIMIT $2`, groupID, limit)
	if err != nil {
		return nil, fmt.Errorf("list channel group reports: %w", err)
	}
	defer rows.Close()
	out := make([]*service.ChannelGroupReport, 0)
	for rows.Next() {
		item, scanErr := scanChannelGroupReport(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *channelGroupReportRepository) HasByUserAndGroup(ctx context.Context, userID, groupID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM channel_group_reports WHERE user_id = $1 AND group_id = $2
)`, userID, groupID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has channel group report: %w", err)
	}
	return exists, nil
}

func (r *channelGroupReportRepository) ListSubmittedGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT DISTINCT group_id FROM channel_group_reports WHERE user_id = $1 ORDER BY group_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list submitted group ids: %w", err)
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type reportScanner interface {
	Scan(dest ...any) error
}

func (r *channelGroupReportRepository) scanOne(ctx context.Context, q string, args ...any) (*service.ChannelGroupReport, error) {
	return scanChannelGroupReport(r.db.QueryRowContext(ctx, q, args...))
}

func scanChannelGroupReport(row reportScanner) (*service.ChannelGroupReport, error) {
	item := &service.ChannelGroupReport{}
	if err := row.Scan(&item.ID, &item.UserID, &item.GroupID, &item.Category, &item.Body, &item.CreatedAt); err != nil {
		return nil, err
	}
	return item, nil
}
