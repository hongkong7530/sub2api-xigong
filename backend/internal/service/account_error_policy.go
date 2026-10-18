package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// applyIgnoredAccountErrorRecovery removes automatic quarantine only. Explicit
// inactive accounts and manually paused schedulability remain under admin control.
func applyIgnoredAccountErrorRecovery(account *Account) {
	if !account.IgnoreAccountErrors() {
		return
	}
	if account.Status == StatusError {
		account.Status = StatusActive
		account.Schedulable = true
	}
	account.RateLimitedAt = nil
	account.RateLimitResetAt = nil
	account.OverloadUntil = nil
	if !IsAccountSchedulingThresholdReason(account.TempUnschedulableReason) {
		account.TempUnschedulableUntil = nil
	}
	if limits, ok := account.Extra[modelRateLimitsKey].(map[string]any); ok {
		kept := make(map[string]any)
		for key, value := range limits {
			if entry, ok := value.(map[string]any); ok {
				if reason, _ := entry["reason"].(string); IsAccountSchedulingThresholdReason(reason) {
					kept[key] = value
				}
			}
		}
		account.Extra[modelRateLimitsKey] = kept
	}
}

func recordIgnoredAccountErrorMessage(ctx context.Context, repo AccountRepository, account *Account, message string) {
	if repo == nil || account == nil {
		return
	}
	if err := repo.SetError(ctx, account.ID, message); err != nil {
		slog.Warn("ignored_account_error_record_failed", "account_id", account.ID, "error", err)
	}
}

// Record the error without changing HTTP responses, usage logs or retry budgets.
// The repository atomically keeps status and scheduling unchanged for opt-in accounts.
func (s *RateLimitService) recordIgnoredAccountError(ctx context.Context, account *Account, statusCode int, body []byte) {
	if s == nil || account == nil {
		return
	}
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	message = truncateForLog([]byte(fmt.Sprintf("Upstream HTTP %d: %s", statusCode, message)), 512)
	slog.Warn("account_error_quarantine_ignored", "account_id", account.ID, "status_code", statusCode)
	if s.accountRepo != nil {
		if err := s.accountRepo.SetError(ctx, account.ID, message); err != nil {
			slog.Warn("ignored_account_error_record_failed", "account_id", account.ID, "error", err)
		}
	}
}
