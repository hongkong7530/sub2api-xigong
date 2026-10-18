package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// The v3 enforcement path never trusts a Redis sentinel or a delayed absolute snapshot.
func (s *BillingCacheService) checkAuthoritativePlatformQuota(ctx context.Context, userID int64, platform string) error {
	if platform == "" {
		return nil
	}
	if s.userPlatformQuotaRepo == nil {
		return ErrBillingServiceUnavailable
	}
	rec, err := s.userPlatformQuotaRepo.GetByUserPlatform(ctx, userID, platform)
	if err != nil {
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if rec == nil {
		return nil
	}
	now := time.Now()
	daily, weekly, monthly := rec.DailyUsageUSD, rec.WeeklyUsageUSD, rec.MonthlyUsageUSD
	if quotaWindowExpired(rec.DailyWindowStart, timezone.StartOfDay(now)) {
		daily = 0
	}
	if quotaWindowExpired(rec.WeeklyWindowStart, timezone.StartOfWeek(now)) {
		weekly = 0
	}
	if monthlyQuotaWindowExpired(rec.MonthlyWindowStart, now) {
		monthly = 0
	}
	if rec.DailyLimitUSD != nil && daily >= *rec.DailyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformDailyQuotaExhausted, nextDailyReset(now))
	}
	if rec.WeeklyLimitUSD != nil && weekly >= *rec.WeeklyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformWeeklyQuotaExhausted, nextWeeklyReset(now))
	}
	if rec.MonthlyLimitUSD != nil && monthly >= *rec.MonthlyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformMonthlyQuotaExhausted, nextMonthlyResetFrom(rec.MonthlyWindowStart, now))
	}
	return nil
}
