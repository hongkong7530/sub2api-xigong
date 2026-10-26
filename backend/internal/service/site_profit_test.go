package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSiteProfitCostDecimalContract(t *testing.T) {
	for _, tc := range []struct {
		input   string
		want    string
		invalid bool
	}{{"0", "0.00000000", false}, {"0.2", "0.20000000", false}, {"10000", "10000.00000000", false}, {"0.12345678", "0.12345678", false}, {"-1", "", true}, {"NaN", "", true}, {"Infinity", "", true}, {"1e-2", "", true}, {"10000.1", "", true}, {".000000001", "", true}, {"", "", true}} {
		t.Run(tc.input, func(t *testing.T) {
			v, e := NormalizeProfitCost(&tc.input)
			if tc.invalid {
				require.ErrorIs(t, e, ErrProfitInvalid)
			} else {
				require.NoError(t, e)
				require.Equal(t, tc.want, *v)
			}
		})
	}
	v, e := NormalizeProfitCost(nil)
	require.NoError(t, e)
	require.Nil(t, v)
}
func TestSiteProfitWeightedMarginAndUnknown(t *testing.T) {
	a := ProfitMetric{Requests: 1, KnownRequests: 1, Revenue: "0.3", KnownRevenue: "0.3", Cost: "0.2", Profit: "0.1"}
	a.Complete()
	require.Equal(t, "0.33333333", *a.Margin)
	b := ProfitMetric{Requests: 1, KnownRequests: 1, Revenue: "0.7", KnownRevenue: "0.7", Cost: "0.6", Profit: "0.1"}
	sum := AddProfitMetric(a, b)
	require.Equal(t, "0.20000000", *sum.Margin)
	require.Equal(t, "0.20000000", sum.Profit)
	u := ProfitMetric{Requests: 3, KnownRequests: 0, Revenue: "100", KnownRevenue: "0", Cost: "0", Profit: "0"}
	sum = AddProfitMetric(sum, u)
	require.Equal(t, "0.20000000", *sum.Margin)
	require.Equal(t, "0.40000000", *sum.Coverage)
	noRevenue := ProfitMetric{Requests: 1, KnownRequests: 1, Revenue: "0", KnownRevenue: "0", Cost: "1", Profit: "-1"}
	noRevenue.Complete()
	require.Nil(t, noRevenue.Margin)
	r := ProfitReferenceMargin(sum.Margin)
	require.Equal(t, "-0.02000000", *r)
	negative := "-0.2"
	require.Equal(t, "0.02000000", *ProfitReferenceMargin(&negative))
	require.Nil(t, ProfitReferenceMargin(nil))
	require.Equal(t, "0.20000000", sum.Profit)
}

type profitRepoStub struct {
	mu        sync.Mutex
	calls     int
	value     *ProfitSnapshot
	err       error
	seenUser  int64
	costInput ProfitCostInput
	entered   chan struct{}
	release   chan struct{}
}

func (s *profitRepoStub) Snapshot(ctx context.Context, _ bool) (*ProfitSnapshot, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.value, s.err
}
func (s *profitRepoStub) Contribution(_ context.Context, id int64) (*ProfitContribution, error) {
	s.seenUser = id
	return &ProfitContribution{}, s.err
}
func (s *profitRepoStub) SetCost(_ context.Context, _, _ int64, in ProfitCostInput) (*ProfitCostVersion, error) {
	s.costInput = in
	return &ProfitCostVersion{Version: in.Version + 1, Multiplier: in.Multiplier}, s.err
}
func (s *profitRepoStub) Entries(context.Context, ProfitEntryFilter) (*ProfitEntryPage, error) {
	return &ProfitEntryPage{Items: []ProfitEntry{}}, s.err
}
func (s *profitRepoStub) Aggregate(context.Context, int) (int, error) { return 0, s.err }
func TestSiteProfitCacheIsBoundedAndFailureNotCached(t *testing.T) {
	r := &profitRepoStub{value: &ProfitSnapshot{AsOf: nil, Scope: "public_groups"}}
	s := NewSiteProfitService(r)
	a, e := s.Snapshot(context.Background(), false)
	require.NoError(t, e)
	b, e := s.Snapshot(context.Background(), false)
	require.NoError(t, e)
	require.Same(t, a, b)
	require.Equal(t, 1, r.calls)
	_, e = s.Snapshot(context.Background(), true)
	require.NoError(t, e)
	require.Equal(t, 2, r.calls)
	value := "0.2"
	_, e = s.SetCost(context.Background(), 1, 2, ProfitCostInput{Version: 0, Multiplier: &value})
	require.NoError(t, e)
	require.Equal(t, "0.20000000", *r.costInput.Multiplier)
	r.err = errors.New("db unavailable")
	_, e = s.Snapshot(context.Background(), false)
	require.Error(t, e)
	require.Equal(t, 3, r.calls)
	_, e = s.Snapshot(context.Background(), false)
	require.Error(t, e)
	require.Equal(t, 4, r.calls)
}
func TestSiteProfitCanceledViewerCannotCancelSharedRead(t *testing.T) {
	r := &profitRepoStub{value: &ProfitSnapshot{}, entered: make(chan struct{}, 1), release: make(chan struct{})}
	s := NewSiteProfitService(r)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, e := s.Snapshot(ctx, false); result <- e }()
	<-r.entered
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	close(r.release)
	time.Sleep(10 * time.Millisecond)
	_, e := s.Snapshot(context.Background(), false)
	require.NoError(t, e)
	require.Equal(t, 1, r.calls)
}
func TestSiteProfitPublicDTOHasNoCostConfiguration(t *testing.T) {
	raw, e := json.Marshal(ProfitGroup{ID: 1, Name: "public"})
	require.NoError(t, e)
	require.NotContains(t, string(raw), "cost_multiplier")
	require.NotContains(t, string(raw), "cost_version")
	require.NotContains(t, string(raw), "user_id")
}
