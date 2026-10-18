package service

import (
	"context"
	"database/sql"
	"fmt"
	"maps"

	"github.com/Wei-Shaw/sub2api/internal/cluster"
)

type clusterPricingKey struct{}

type clusterPricingSnapshot struct {
	channels *channelCache
	billing  *BillingService
	rates    map[int64]float64
}

// FreezeClusterPricing binds immutable published channel/catalog snapshots before dispatch.
// It does not copy request content or credentials and is carried into the settlement worker.
func (s *APIKeyService) FreezeClusterPricing(ctx context.Context, key *APIKey) (context.Context, error) {
	if !s.cfg.ClusterEnabled() {
		return ctx, nil
	}
	if existing, _ := ctx.Value(clusterPricingKey{}).(*clusterPricingSnapshot); existing != nil {
		return ctx, nil
	}
	if s.clusterChannels == nil || s.clusterBilling == nil {
		return nil, fmt.Errorf("cluster pricing sources are not wired")
	}
	rates, err := s.GetUserGroupRates(ctx, key.UserID)
	if err != nil {
		return nil, err
	}
	snapshot := &clusterPricingSnapshot{rates: maps.Clone(rates)}
	if s.clusterChannels != nil {
		cache, err := s.clusterChannels.authoritativePricingSnapshot(ctx)
		if err != nil {
			return nil, err
		}
		snapshot.channels = cache
	}
	if s.clusterBilling != nil {
		original := s.clusterBilling
		copy := &BillingService{cfg: original.cfg, fallbackPrices: original.fallbackPrices}
		if pricing := original.pricingService; pricing != nil {
			pricing.mu.RLock()
			copy.pricingService = &PricingService{cfg: pricing.cfg, pricingData: pricing.pricingData, lastUpdated: pricing.lastUpdated, localHash: pricing.localHash}
			pricing.mu.RUnlock()
		}
		snapshot.billing = copy
	}
	return context.WithValue(ctx, clusterPricingKey{}, snapshot), nil
}
func CopyClusterPricing(parent, base context.Context) context.Context {
	if parent != nil {
		if snapshot, ok := parent.Value(clusterPricingKey{}).(*clusterPricingSnapshot); ok {
			base = context.WithValue(base, clusterPricingKey{}, snapshot)
		}
	}
	return cluster.CopyExecution(parent, base)
}
func clusterPricing(ctx context.Context) *clusterPricingSnapshot {
	if ctx == nil {
		return nil
	}
	snapshot, _ := ctx.Value(clusterPricingKey{}).(*clusterPricingSnapshot)
	return snapshot
}
func (s *APIKeyService) SetClusterPricingSources(channels *ChannelService, billing *BillingService, db *sql.DB) {
	s.clusterChannels = channels
	s.clusterBilling = billing
	if channels != nil {
		channels.clusterDB = db
	}
}

func WithClusterTurnSettlement(ctx context.Context, _ int) context.Context {
	if e := cluster.ExecutionFrom(ctx); e != nil {
		snapshot := e.Snapshot()
		return cluster.WithSettlementID(cluster.WithExecution(ctx, snapshot), snapshot.SettlementID())
	}
	return ctx
}

func (s *APIKeyService) RefreshClusterTurnPricing(ctx context.Context, key *APIKey) (context.Context, error) {
	return s.FreezeClusterPricing(context.WithValue(ctx, clusterPricingKey{}, (*clusterPricingSnapshot)(nil)), key)
}

func (s *ChannelService) authoritativePricingSnapshot(ctx context.Context) (*channelCache, error) {
	if s.clusterDB == nil { return nil, fmt.Errorf("authoritative pricing connection unavailable") }
	var revision int64
	if err := s.clusterDB.QueryRowContext(ctx, `SELECT revision FROM cluster_pricing_revision WHERE id=1`).Scan(&revision); err != nil { return nil, err }
	s.clusterSnapshotMu.Lock()
	if s.clusterSnapshot != nil && s.clusterRevision == revision { snapshot:=s.clusterSnapshot; s.clusterSnapshotMu.Unlock(); return snapshot,nil }
	s.clusterSnapshotMu.Unlock()
	value,err,_:=s.cacheSF.Do(fmt.Sprintf("cluster-pricing:%d",revision),func()(any,error){
		s.clusterSnapshotMu.Lock()
		if s.clusterSnapshot!=nil&&s.clusterRevision==revision{snapshot:=s.clusterSnapshot;s.clusterSnapshotMu.Unlock();return snapshot,nil}
		s.clusterSnapshotMu.Unlock()
		channels,err:=s.buildCache(context.WithValue(ctx,clusterPricingKey{},(*clusterPricingSnapshot)(nil)));if err!=nil{return nil,err}
		var after int64
		if err:=s.clusterDB.QueryRowContext(ctx,`SELECT revision FROM cluster_pricing_revision WHERE id=1`).Scan(&after);err!=nil{return nil,err}
		if after!=revision{return nil,fmt.Errorf("pricing changed while taking snapshot; retry admission")}
		s.clusterSnapshotMu.Lock()
		if s.clusterRevision<=revision{s.clusterSnapshot=channels;s.clusterRevision=revision}
		s.clusterSnapshotMu.Unlock()
		return channels,nil
	})
	if err!=nil{return nil,err}
	snapshot,ok:=value.(*channelCache);if !ok||snapshot==nil{return nil,fmt.Errorf("invalid authoritative pricing snapshot")}
	return snapshot,nil
}
