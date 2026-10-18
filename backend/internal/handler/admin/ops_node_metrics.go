package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// v3 owns membership; legacy routing rows must not hide a machine or supply a
// different machine's connection limits. Missing telemetry remains explicit.
func (h *OpsHandler) attachV3NodeMetrics(c *gin.Context, overview *service.OpsDashboardOverview) {
	if overview == nil || h.cluster == nil || !h.cluster.Enabled() {
		return
	}
	views, err := h.cluster.NodeViews(c.Request.Context())
	if err != nil {
		return
	}
	metrics := make(map[string]*service.OpsSystemMetricsSnapshot)
	for _, item := range overview.SystemMetricsNodes {
		if item != nil {
			metrics[item.NodeID] = item
		}
	}
	out := make([]*service.OpsSystemMetricsSnapshot, 0, len(views))
	for _, view := range views {
		node, ok := view["config"].(cluster.Node)
		if !ok || node.State == "retired" {
			continue
		}
		item := &service.OpsSystemMetricsSnapshot{NodeID: node.ID, NodeName: node.Name}
		if sample := metrics[node.ID]; sample != nil {
			*item = *sample
		}
		budget := node.DBConnections
		item.DBConfiguredBudget = &budget
		item.DBMaxOpenConns, item.RedisPoolSize = nil, nil
		status, _ := view["status"].(*cluster.Snapshot)
		fresh, _ := view["fresh"].(bool)
		if fresh && status != nil {
			item.DBMaxOpenConns = status.DBPoolMax
			item.DBConnActive = &status.DBConnections
			item.DBConnIdle = status.DBPoolIdle
			item.RedisPoolSize = status.RedisPoolMax
			item.RedisConnTotal = status.RedisPoolTotal
			item.RedisConnIdle = status.RedisPoolIdle
		}
		out = append(out, item)
		if item.NodeID == overview.ResponseNodeID {
			overview.SystemMetrics = item
		}
	}
	overview.SystemMetricsNodes = out
}
