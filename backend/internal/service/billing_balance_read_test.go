package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type lightweightBalanceRepo struct {
	UserRepository
	balance      float64
	err          error
	balanceReads atomic.Int32
	fullReads    atomic.Int32
}

func (r *lightweightBalanceRepo) GetBalance(context.Context, int64) (float64, error) {
	r.balanceReads.Add(1)
	return r.balance, r.err
}
func (r *lightweightBalanceRepo) GetByID(context.Context, int64) (*User, error) {
	r.fullReads.Add(1)
	return nil, errors.New("full user lookup must not be used")
}

type legacyBalanceRepo struct {
	UserRepository
	reads int
}

func (r *legacyBalanceRepo) GetByID(context.Context, int64) (*User, error) {
	r.reads++
	return &User{ID: 42, Balance: 6.5}, nil
}

func TestBillingBalanceClusterReadsCurrentValueWithoutFullUserLookup(t *testing.T) {
	repo := &lightweightBalanceRepo{}
	s := &BillingCacheService{cfg: &config.Config{Cluster: config.ClusterConfig{Enabled: true}}, userRepo: repo}
	for _, expected := range []float64{4, 0, -0.5} {
		repo.balance = expected
		got, err := s.GetUserBalance(context.Background(), 42)
		require.NoError(t, err)
		require.Equal(t, expected, got)
	}
	require.EqualValues(t, 3, repo.balanceReads.Load())
	require.Zero(t, repo.fullReads.Load())
}
func TestBillingBalanceReadFailureDoesNotFallbackOrPermit(t *testing.T) {
	errDB := errors.New("database unavailable")
	repo := &lightweightBalanceRepo{balance: 100, err: errDB}
	s := &BillingCacheService{cfg: &config.Config{Cluster: config.ClusterConfig{Enabled: true}}, userRepo: repo}
	got, err := s.GetUserBalance(context.Background(), 42)
	require.ErrorIs(t, err, errDB)
	require.Zero(t, got)
	require.ErrorIs(t, s.checkBalanceEligibility(context.Background(), 42), ErrBillingServiceUnavailable)
	require.Zero(t, repo.fullReads.Load())
}
func TestBillingBalanceKeepsLegacyRepositoryCompatibility(t *testing.T) {
	repo := &legacyBalanceRepo{}
	s := &BillingCacheService{userRepo: repo}
	got, err := s.getUserBalanceFromDB(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 6.5, got)
	require.Equal(t, 1, repo.reads)
}
func TestBillingBalanceZeroAndNegativeStillRejected(t *testing.T) {
	for _, balance := range []float64{0, -0.01} {
		repo := &lightweightBalanceRepo{balance: balance}
		s := &BillingCacheService{cfg: &config.Config{Cluster: config.ClusterConfig{Enabled: true}}, userRepo: repo}
		require.ErrorIs(t, s.checkBalanceEligibility(context.Background(), 42), ErrInsufficientBalance)
		require.Zero(t, repo.fullReads.Load())
	}
}
