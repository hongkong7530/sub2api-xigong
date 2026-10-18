package service

import (
	"context"
	"html"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	ChannelGroupReportCategoryUnreachable    = "unreachable"
	ChannelGroupReportCategorySlowFirstToken = "slow_first_token"
	ChannelGroupReportCategoryUsageDoubt     = "usage_doubt"
	ChannelGroupReportCategoryOther          = "other"

	channelGroupReportMaxBodyRunes   = 2000
	channelGroupReportCooldown       = 15 * time.Minute
	channelGroupReportAdminListLimit = 20
)

var channelGroupReportCategories = map[string]struct{}{
	ChannelGroupReportCategoryUnreachable:    {},
	ChannelGroupReportCategorySlowFirstToken: {},
	ChannelGroupReportCategoryUsageDoubt:     {},
	ChannelGroupReportCategoryOther:          {},
}

// ChannelGroupReport is a user-submitted monitor-group report.
type ChannelGroupReport struct {
	ID        int64
	UserID    int64
	GroupID   int64
	Category  string
	Body      string
	CreatedAt time.Time
	Username  string
	Email     string
	Deleted   bool
}

// ChannelGroupReportRepository stores channel group reports.
type ChannelGroupReportRepository interface {
	Create(ctx context.Context, report *ChannelGroupReport) error
	LatestByUserAndGroup(ctx context.Context, userID, groupID int64) (*ChannelGroupReport, error)
	ListByGroup(ctx context.Context, groupID int64, limit int) ([]*ChannelGroupReport, error)
	HasByUserAndGroup(ctx context.Context, userID, groupID int64) (bool, error)
	ListSubmittedGroupIDs(ctx context.Context, userID int64) ([]int64, error)
}

type channelGroupReportNotifier interface {
	Send(ctx context.Context, input NotificationEmailSendInput) error
}

type channelGroupReportUserStore interface {
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error)
}

type channelGroupReportGroupStore interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
}

type channelGroupReportMonitorStore interface {
	ListBoundGroupIDs(ctx context.Context) ([]int64, error)
}

// ChannelGroupReportService creates and lists group reports.
type ChannelGroupReportService struct {
	repo     ChannelGroupReportRepository
	users    channelGroupReportUserStore
	groups   channelGroupReportGroupStore
	monitors channelGroupReportMonitorStore
	settings channelGroupReportSettings
	notifier channelGroupReportNotifier
	now      func() time.Time
}

type channelGroupReportSettings interface {
	GetValue(ctx context.Context, key string) (string, error)
}

func NewChannelGroupReportService(
	repo ChannelGroupReportRepository,
	users UserRepository,
	groups GroupRepository,
	monitors ChannelMonitorRepository,
	settings *SettingService,
	notifier *NotificationEmailService,
) *ChannelGroupReportService {
	return &ChannelGroupReportService{
		repo:     repo,
		users:    users,
		groups:   groups,
		monitors: monitors,
		settings: settings,
		notifier: notifier,
		now:      time.Now,
	}
}

func NormalizeChannelGroupReportCategory(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	_, ok := channelGroupReportCategories[value]
	return value, ok
}

func sanitizeReportBodyHTML(raw string) string {
	escaped := html.EscapeString(raw)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

func (s *ChannelGroupReportService) Create(ctx context.Context, userID, groupID int64, category, body string) (*ChannelGroupReport, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("GROUP_REPORT_UNAVAILABLE", "group report service is not configured")
	}
	normalized, ok := NormalizeChannelGroupReportCategory(category)
	if !ok {
		return nil, infraerrors.BadRequest("INVALID_REPORT_CATEGORY", "invalid report category")
	}
	trimmed := strings.TrimSpace(body)
	if utf8.RuneCountInString(trimmed) > channelGroupReportMaxBodyRunes {
		return nil, infraerrors.BadRequest("REPORT_BODY_TOO_LONG", "report body exceeds 2000 characters")
	}
	if normalized == ChannelGroupReportCategoryOther && trimmed == "" {
		return nil, infraerrors.BadRequest("REPORT_BODY_REQUIRED", "report body is required for other")
	}
	if s.groups != nil {
		if _, err := s.groups.GetByID(ctx, groupID); err != nil {
			return nil, err
		}
	}
	if s.monitors != nil {
		bound, err := s.monitors.ListBoundGroupIDs(ctx)
		if err != nil {
			return nil, err
		}
		found := false
		for _, id := range bound {
			if id == groupID {
				found = true
				break
			}
		}
		if !found {
			return nil, infraerrors.Forbidden("GROUP_NOT_MONITORED", "group is not bound to a channel monitor")
		}
	}

	latest, err := s.repo.LatestByUserAndGroup(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if latest != nil && now.Sub(latest.CreatedAt) < channelGroupReportCooldown {
		return nil, infraerrors.TooManyRequests("GROUP_REPORT_RATE_LIMITED", "please wait before submitting another report for this group")
	}

	user, err := s.lookupUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	groupName := ""
	if s.groups != nil {
		if group, gerr := s.groups.GetByID(ctx, groupID); gerr == nil && group != nil {
			groupName = group.Name
		}
	}

	report := &ChannelGroupReport{
		UserID:    userID,
		GroupID:   groupID,
		Category:  normalized,
		Body:      trimmed,
		CreatedAt: now,
	}
	if user != nil {
		report.Username = user.Username
		report.Email = user.Email
	}
	if err := s.repo.Create(ctx, report); err != nil {
		return nil, err
	}
	if err := s.notifyAdmins(ctx, report, groupName, user); err != nil {
		slog.Error("channel group report email failed after persist", "group_id", groupID, "user_id", userID, "error", err)
	}
	return report, nil
}

func (s *ChannelGroupReportService) ListByGroup(ctx context.Context, groupID int64) ([]*ChannelGroupReport, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("GROUP_REPORT_UNAVAILABLE", "group report service is not configured")
	}
	items, err := s.repo.ListByGroup(ctx, groupID, channelGroupReportAdminListLimit)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		user, uerr := s.lookupUserIncludeDeleted(ctx, item.UserID)
		if uerr != nil || user == nil {
			item.Deleted = true
			continue
		}
		item.Username = user.Username
		item.Email = user.Email
		item.Deleted = user.DeletedAt != nil
	}
	return items, nil
}

func (s *ChannelGroupReportService) Submitted(ctx context.Context, userID, groupID int64) (bool, error) {
	if s == nil || s.repo == nil {
		return false, nil
	}
	return s.repo.HasByUserAndGroup(ctx, userID, groupID)
}

func (s *ChannelGroupReportService) SubmittedGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.ListSubmittedGroupIDs(ctx, userID)
}

func (s *ChannelGroupReportService) lookupUser(ctx context.Context, userID int64) (*User, error) {
	if s.users == nil {
		return &User{ID: userID}, nil
	}
	return s.users.GetByID(ctx, userID)
}

func (s *ChannelGroupReportService) lookupUserIncludeDeleted(ctx context.Context, userID int64) (*User, error) {
	if s.users == nil {
		return nil, nil
	}
	if user, err := s.users.GetByIDIncludeDeleted(ctx, userID); err == nil {
		return user, nil
	}
	return s.users.GetByID(ctx, userID)
}

func (s *ChannelGroupReportService) notifyAdmins(ctx context.Context, report *ChannelGroupReport, groupName string, user *User) error {
	if s.notifier == nil {
		return infraerrors.InternalServer("GROUP_REPORT_EMAIL_UNAVAILABLE", "notification email is not configured")
	}
	recipients, err := s.reportRecipients(ctx)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return infraerrors.InternalServer("GROUP_REPORT_NO_RECIPIENTS", "no admin recipients configured")
	}
	username := ""
	email := ""
	if user != nil {
		username = user.Username
		email = user.Email
	}
	submittedAt := report.CreatedAt.UTC().Format(time.RFC3339)
	vars := map[string]string{
		"group_name":   groupName,
		"category":     report.Category,
		"username":     username,
		"email":        email,
		"submitted_at": submittedAt,
	}
	raw := map[string]string{"body_html": sanitizeReportBodyHTML(report.Body)}
	var firstErr error
	for _, recipient := range recipients {
		sendErr := s.notifier.Send(ctx, NotificationEmailSendInput{
			Event:            NotificationEmailEventMonitorGroupReport,
			RecipientEmail:   recipient,
			RecipientName:    recipient,
			SourceType:       "channel_group_report",
			SourceID:         strings.TrimSpace(groupName) + ":" + submittedAt + ":" + recipient,
			Variables:        vars,
			RawHTMLVariables: raw,
		})
		if sendErr != nil && firstErr == nil {
			firstErr = sendErr
		}
	}
	return firstErr
}

func (s *ChannelGroupReportService) reportRecipients(ctx context.Context) ([]string, error) {
	if s.settings != nil {
		raw, err := s.settings.GetValue(ctx, SettingKeyAccountQuotaNotifyEmails)
		if err == nil && strings.TrimSpace(raw) != "" && raw != "[]" {
			if emails := filterVerifiedEmails(ParseNotifyEmails(raw)); len(emails) > 0 {
				return emails, nil
			}
		}
	}
	if s.users == nil {
		return nil, nil
	}
	noSubs := false
	users, _, err := s.users.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 200}, UserListFilters{
		Role:                 RoleAdmin,
		Status:               StatusActive,
		IncludeSubscriptions: &noSubs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(users))
	seen := map[string]bool{}
	for _, user := range users {
		email := strings.ToLower(strings.TrimSpace(user.Email))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		out = append(out, user.Email)
	}
	return out, nil
}
