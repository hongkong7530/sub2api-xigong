package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Uses the caller's native SQL transaction; the Ent quota repository must not open a second one.
func incrementClusterPlatformQuota(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) error {
	charge := cmd.PlatformQuota
	if charge == nil || charge.Amount <= 0 {
		return nil
	}
	if charge.Platform == "" {
		return fmt.Errorf("platform quota charge has no platform")
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('sub2api.quota_writer','settlement-v1',true)`); err != nil {
		return err
	}
	now := time.Now().UTC()
	day, week := timezone.StartOfDay(now), timezone.StartOfWeek(now)
	_, err := tx.ExecContext(ctx, `UPDATE user_platform_quotas SET
 daily_usage_usd=CASE WHEN daily_window_start IS NULL OR daily_window_start<$4 THEN $3 ELSE daily_usage_usd+$3 END,
 weekly_usage_usd=CASE WHEN weekly_window_start IS NULL OR weekly_window_start<$5 THEN $3 ELSE weekly_usage_usd+$3 END,
 monthly_usage_usd=CASE WHEN monthly_window_start IS NULL OR monthly_window_start<=($6::timestamptz-INTERVAL '720 hours') THEN $3 ELSE monthly_usage_usd+$3 END,
 daily_window_start=CASE WHEN daily_window_start IS NULL OR daily_window_start<$4 THEN $4 ELSE daily_window_start END,
 weekly_window_start=CASE WHEN weekly_window_start IS NULL OR weekly_window_start<$5 THEN $5 ELSE weekly_window_start END,
 monthly_window_start=CASE WHEN monthly_window_start IS NULL OR monthly_window_start<=($6::timestamptz-INTERVAL '720 hours') THEN $6 ELSE monthly_window_start END,
 updated_at=NOW()
 WHERE user_id=$1 AND platform=$2 AND deleted_at IS NULL
 AND (daily_limit_usd IS NOT NULL OR weekly_limit_usd IS NOT NULL OR monthly_limit_usd IS NOT NULL)`, cmd.UserID, charge.Platform, charge.Amount, day, week, now)
	return err
}
