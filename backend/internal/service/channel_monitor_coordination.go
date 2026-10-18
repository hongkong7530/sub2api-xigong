package service

import (
	"context"
	"database/sql"
	"fmt"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Manual and scheduled probes share a lock; only scheduled work consults the idle gate.
func (s *ChannelMonitorService) SetCoordinationDB(db *sql.DB) { s.coordinationDB = db }

func (s *ChannelMonitorService) coordinatedCheck(ctx context.Context, id int64, scheduled bool) ([]*CheckResult, error) {
	if s.coordinationDB != nil {
		release, acquired, err := tryAcquireDBAdvisoryLockWithError(ctx, s.coordinationDB, hashAdvisoryLockID(fmt.Sprintf("channel-monitor:%d", id)))
		if err != nil { return nil, err }
		if !acquired {
			if scheduled { return nil, nil }
			return nil, infraerrors.Conflict("MONITOR_CHECK_IN_PROGRESS", "a monitor check is already running")
		}
		defer release()
	}
	if scheduled {
		due, err := s.ScheduledCheckDue(ctx, id)
		if err != nil || !due { return nil, err }
	}
	return s.runCheck(ctx, id)
}

func (s *ChannelMonitorService) RunCheck(ctx context.Context, id int64) ([]*CheckResult, error) {
	return s.coordinatedCheck(ctx, id, false)
}

func (s *ChannelMonitorService) RunScheduledCheck(ctx context.Context, id int64) ([]*CheckResult, error) {
	return s.coordinatedCheck(ctx, id, true)
}
