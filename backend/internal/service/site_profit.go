package service

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
)

var profitCostPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,8})?$`)

var ErrProfitInvalid = infraerrors.BadRequest("PROFIT_INVALID", "Invalid cost multiplier, version or filter")
var ErrProfitConflict = infraerrors.Conflict("PROFIT_CONFLICT", "Cost configuration changed; reload before saving")
var ErrProfitGroupMissing = infraerrors.NotFound("PROFIT_GROUP_NOT_FOUND", "Group not found")

// Money strings preserve the database's decimal scale. Ratios are fractions, not percentages.
type ProfitMetric struct {
	Requests      int64   `json:"requests"`
	KnownRequests int64   `json:"known_requests"`
	Revenue       string  `json:"revenue"`
	KnownRevenue  string  `json:"known_revenue"`
	Cost          string  `json:"cost,omitempty"`
	Profit        string  `json:"profit"`
	Margin        *string `json:"margin"`
	Coverage      *string `json:"coverage"`
}
type ProfitWindows struct {
	Total ProfitMetric `json:"total"`
	Today ProfitMetric `json:"today"`
	Week  ProfitMetric `json:"week"`
}
type ProfitGroup struct {
	ID              int64         `json:"id"`
	Name            string        `json:"name"`
	Platform        string        `json:"platform"`
	Status          string        `json:"status"`
	Rate            string        `json:"rate"`
	PeakEnabled     bool          `json:"peak_enabled"`
	PeakStart       string        `json:"peak_start"`
	PeakEnd         string        `json:"peak_end"`
	PeakMultiplier  string        `json:"peak_multiplier"`
	AtCost          bool          `json:"at_cost"`
	ReferenceMargin *string       `json:"reference_margin"`
	CostMultiplier  *string       `json:"cost_multiplier,omitempty"`
	CostVersion     int64         `json:"cost_version,omitempty"`
	EffectiveAt     *time.Time    `json:"effective_at,omitempty"`
	Metrics         ProfitWindows `json:"metrics"`
}
type ProfitSnapshot struct {
	Backlog      bool          `json:"backlog"`
	Scope        string        `json:"scope"`
	Timezone     string        `json:"timezone"`
	StartedAt    time.Time     `json:"started_at"`
	AsOf         *time.Time    `json:"as_of"`
	Stale        bool          `json:"stale"`
	Metrics      ProfitWindows `json:"metrics"`
	Balance      ProfitMetric  `json:"balance"`
	Subscription ProfitMetric  `json:"subscription"`
	Groups       []ProfitGroup `json:"groups"`
}
type ProfitContribution struct {
	StartedAt    time.Time    `json:"started_at"`
	AsOf         *time.Time   `json:"as_of"`
	Stale        bool         `json:"stale"`
	Total        ProfitMetric `json:"total"`
	Balance      ProfitMetric `json:"balance"`
	Subscription ProfitMetric `json:"subscription"`
}
type ProfitCostInput struct {
	Multiplier *string `json:"multiplier"`
	Version    int64   `json:"version"`
}
type ProfitCostVersion struct {
	Version     int64     `json:"version"`
	Multiplier  *string   `json:"multiplier"`
	EffectiveAt time.Time `json:"effective_at"`
}
type ProfitEntry struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	GroupID        int64     `json:"group_id"`
	GroupName      string    `json:"group_name"`
	Platform       string    `json:"platform"`
	BillingType    int16     `json:"billing_type"`
	SettledAt      time.Time `json:"settled_at"`
	Base           string    `json:"base"`
	Revenue        string    `json:"revenue"`
	Cost           *string   `json:"cost"`
	Profit         *string   `json:"profit"`
	CostMultiplier *string   `json:"cost_multiplier"`
}
type ProfitEntryFilter struct {
	Page     int
	GroupID  int64
	From, To *time.Time
}
type ProfitEntryPage struct {
	Items   []ProfitEntry `json:"items"`
	Page    int           `json:"page"`
	HasMore bool          `json:"has_more"`
}
type SiteProfitRepository interface {
	Snapshot(context.Context, bool) (*ProfitSnapshot, error)
	Contribution(context.Context, int64) (*ProfitContribution, error)
	SetCost(context.Context, int64, int64, ProfitCostInput) (*ProfitCostVersion, error)
	Entries(context.Context, ProfitEntryFilter) (*ProfitEntryPage, error)
	Aggregate(context.Context, int) (int, error)
}

func NormalizeProfitCost(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	if len(s) > 24 || !profitCostPattern.MatchString(s) {
		return nil, ErrProfitInvalid
	}
	d, e := decimal.NewFromString(s)
	if e != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(10000)) || !d.Equal(d.Round(8)) {
		return nil, ErrProfitInvalid
	}
	result := d.StringFixed(8)
	return &result, nil
}
func EmptyProfitMetric() ProfitMetric {
	return ProfitMetric{Revenue: "0.00000000", KnownRevenue: "0.00000000", Cost: "0.00000000", Profit: "0.00000000"}
}
func (m *ProfitMetric) Complete() {
	revenue, e := decimal.NewFromString(m.KnownRevenue)
	if e == nil && revenue.IsPositive() {
		p, err := decimal.NewFromString(m.Profit)
		if err == nil {
			s := p.Div(revenue).StringFixed(8)
			m.Margin = &s
		}
	}
	if m.Requests > 0 {
		s := decimal.NewFromInt(m.KnownRequests).Div(decimal.NewFromInt(m.Requests)).StringFixed(8)
		m.Coverage = &s
	}
}
func AddProfitMetric(a, b ProfitMetric) ProfitMetric {
	sum := func(x, y string) string {
		d, _ := decimal.NewFromString(x)
		e, _ := decimal.NewFromString(y)
		return d.Add(e).StringFixed(8)
	}
	out := ProfitMetric{Requests: a.Requests + b.Requests, KnownRequests: a.KnownRequests + b.KnownRequests, Revenue: sum(a.Revenue, b.Revenue), KnownRevenue: sum(a.KnownRevenue, b.KnownRevenue), Cost: sum(a.Cost, b.Cost), Profit: sum(a.Profit, b.Profit)}
	out.Complete()
	return out
}
func ProfitReferenceMargin(today *string) *string {
	if today == nil {
		return nil
	}
	d, e := decimal.NewFromString(*today)
	if e != nil {
		return nil
	}
	s := d.Mul(decimal.RequireFromString("-0.1")).StringFixed(8)
	return &s
}

type SiteProfitService struct {
	Repo       SiteProfitRepository
	mu         sync.Mutex
	disclosure *ProfitSnapshot
	expires    time.Time
	flights    singleflight.Group
}

func NewSiteProfitService(repo SiteProfitRepository) *SiteProfitService {
	return &SiteProfitService{Repo: repo}
}
func (s *SiteProfitService) Snapshot(ctx context.Context, admin bool) (*ProfitSnapshot, error) {
	if admin {
		return s.Repo.Snapshot(ctx, true)
	}
	s.mu.Lock()
	cached, valid := s.disclosure, time.Now().Before(s.expires)
	s.mu.Unlock()
	if valid && cached != nil {
		return cached, nil
	}
	ch := s.flights.DoChan("disclosure", func() (any, error) {
		// One caller leaving must not cancel the shared read; the read still has a bounded deadline.
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		v, e := s.Repo.Snapshot(c, false)
		if e == nil {
			s.mu.Lock()
			s.disclosure = v
			s.expires = time.Now().Add(5 * time.Second)
			s.mu.Unlock()
		}
		return v, e
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(*ProfitSnapshot), nil
	}
}
func (s *SiteProfitService) SetCost(ctx context.Context, group, actor int64, input ProfitCostInput) (*ProfitCostVersion, error) {
	if group <= 0 || actor <= 0 || input.Version < 0 {
		return nil, ErrProfitInvalid
	}
	v, e := NormalizeProfitCost(input.Multiplier)
	if e != nil {
		return nil, e
	}
	input.Multiplier = v
	result, e := s.Repo.SetCost(ctx, group, actor, input)
	if e == nil {
		s.mu.Lock()
		s.expires = time.Time{}
		s.mu.Unlock()
	}
	return result, e
}
