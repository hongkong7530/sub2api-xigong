package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type reportRepoStub struct {
	created   []*ChannelGroupReport
	latest    *ChannelGroupReport
	list      []*ChannelGroupReport
	has       bool
	createErr error
	latestErr error
}

func (r *reportRepoStub) Create(_ context.Context, report *ChannelGroupReport) error {
	if r.createErr != nil {
		return r.createErr
	}
	report.ID = int64(len(r.created) + 1)
	copied := *report
	r.created = append(r.created, &copied)
	r.latest = &copied
	r.has = true
	return nil
}

func (r *reportRepoStub) LatestByUserAndGroup(context.Context, int64, int64) (*ChannelGroupReport, error) {
	return r.latest, r.latestErr
}

func (r *reportRepoStub) ListByGroup(context.Context, int64, int) ([]*ChannelGroupReport, error) {
	return r.list, nil
}

func (r *reportRepoStub) HasByUserAndGroup(context.Context, int64, int64) (bool, error) {
	return r.has, nil
}

func (r *reportRepoStub) ListSubmittedGroupIDs(context.Context, int64) ([]int64, error) {
	if !r.has {
		return nil, nil
	}
	return []int64{42}, nil
}

type reportUserStub struct {
	users map[int64]*User
}

func (s *reportUserStub) GetByID(_ context.Context, id int64) (*User, error) {
	return s.users[id], nil
}

func (s *reportUserStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*User, error) {
	return s.users[id], nil
}

func (s *reportUserStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	out := make([]User, 0)
	for _, user := range s.users {
		if filters.Role != "" && user.Role != filters.Role {
			continue
		}
		if filters.Status != "" && user.Status != filters.Status {
			continue
		}
		out = append(out, *user)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out))}, nil
}

type reportGroupStub struct {
	group *Group
	err   error
}

func (s *reportGroupStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, s.err
}

type reportSettingStub struct {
	values map[string]string
}

func (s *reportSettingStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

type reportMailStub struct {
	inputs []NotificationEmailSendInput
	err    error
}

func (s *reportMailStub) Send(_ context.Context, input NotificationEmailSendInput) error {
	s.inputs = append(s.inputs, input)
	return s.err
}

func newReportService(repo *reportRepoStub, mail *reportMailStub) *ChannelGroupReportService {
	return &ChannelGroupReportService{
		repo: repo,
		users: &reportUserStub{users: map[int64]*User{
			8: {ID: 8, Username: "alice", Email: "alice@example.com", Role: RoleUser, Status: StatusActive},
			1: {ID: 1, Username: "admin", Email: "admin@example.com", Role: RoleAdmin, Status: StatusActive},
		}},
		groups:   &reportGroupStub{group: &Group{ID: 42, Name: "fast"}},
		monitors: &reportMonitorStub{ids: []int64{42}},
		settings: &reportSettingStub{values: map[string]string{}},
		notifier: mail,
		now:      func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) },
	}
}

type reportMonitorStub struct {
	ids []int64
}

func (s *reportMonitorStub) ListBoundGroupIDs(context.Context) ([]int64, error) {
	return s.ids, nil
}

func TestChannelGroupReportCreate_RateLimitedAndOtherRequiresBody(t *testing.T) {
	repo := &reportRepoStub{}
	mail := &reportMailStub{}
	svc := newReportService(repo, mail)

	_, err := svc.Create(context.Background(), 8, 42, "other", "   ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "required")

	created, err := svc.Create(context.Background(), 8, 42, "unreachable", "")
	require.NoError(t, err)
	require.Equal(t, "unreachable", created.Category)
	require.Len(t, mail.inputs, 1)
	require.Equal(t, NotificationEmailEventMonitorGroupReport, mail.inputs[0].Event)
	require.Equal(t, "admin@example.com", mail.inputs[0].RecipientEmail)

	_, err = svc.Create(context.Background(), 8, 42, "unreachable", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "wait")
}

func TestChannelGroupReportCreate_EmailFailureStillPersists(t *testing.T) {
	repo := &reportRepoStub{}
	mail := &reportMailStub{err: errMailDown{}}
	svc := newReportService(repo, mail)
	created, err := svc.Create(context.Background(), 8, 42, "usage_doubt", "looks off")
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Len(t, repo.created, 1)
}

type errMailDown struct{}

func (errMailDown) Error() string { return "smtp down" }

func TestChannelGroupReportCreate_RejectsUnboundGroup(t *testing.T) {
	repo := &reportRepoStub{}
	mail := &reportMailStub{}
	svc := newReportService(repo, mail)
	svc.monitors = &reportMonitorStub{ids: []int64{7}}
	_, err := svc.Create(context.Background(), 8, 42, "unreachable", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not bound")
	require.Empty(t, repo.created)
}

func TestSanitizeReportBodyHTMLEscapesMarkup(t *testing.T) {
	got := sanitizeReportBodyHTML("<script>alert(1)</script>\nnext")
	require.Equal(t, "&lt;script&gt;alert(1)&lt;/script&gt;<br>next", got)
}
