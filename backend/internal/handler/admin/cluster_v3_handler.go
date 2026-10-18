package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ProvideOpsHandler keeps the existing test constructor while wiring the optional runtime.
func ProvideOpsHandler(ops *service.OpsService, runtime *cluster.Runtime) *OpsHandler {
	h := NewOpsHandler(ops)
	h.cluster = runtime
	return h
}
func (h *OpsHandler) clusterEnabled(c *gin.Context) bool {
	if h.cluster == nil || !h.cluster.Enabled() {
		response.Error(c, 404, "Cluster v3 is not enabled")
		return false
	}
	return true
}
func (h *OpsHandler) ClusterV3Overview(c *gin.Context) {
	if !h.clusterEnabled(c) {
		return
	}
	nodes, err := h.cluster.NodeViews(c.Request.Context())
	if err != nil {
		clusterError(c, err)
		return
	}
	generation, held, err := h.cluster.Store.Generation(c.Request.Context())
	if err != nil {
		clusterError(c, err)
		return
	}
	capacity, err := h.cluster.Store.DatabaseCapacity(c.Request.Context())
	if err != nil {
		clusterError(c, err)
		return
	}
	response.Success(c, gin.H{"nodes": nodes, "response_node_id": service.LoadOpsNodeIdentity().ID, "database_capacity": capacity, "shadow": h.cluster.Shadow(), "data_authority": "single_primary", "control_url": h.cluster.ControlURL(), "redis_generation": generation, "redis_recovery_required": held})
}
func (h *OpsHandler) ClusterV3Register(c *gin.Context) {
	if !h.clusterEnabled(c) {
		return
	}
	if !h.cluster.IsController() {
		response.Error(c, 403, "This node does not own cluster changes")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	var node cluster.Node
	if c.ShouldBindJSON(&node) != nil {
		response.BadRequest(c, "Invalid node registration")
		return
	}
	token, err := h.cluster.Store.Register(c.Request.Context(), node, subject.UserID)
	if err != nil {
		clusterError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"node_id": node.ID, "join_token": token, "expires_in_seconds": 900})
}
func (h *OpsHandler) ClusterV3Change(c *gin.Context) {
	if !h.clusterEnabled(c) {
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	var change cluster.Change
	if c.ShouldBindJSON(&change) != nil {
		response.BadRequest(c, "Invalid node operation")
		return
	}
	op, err := h.cluster.Change(c.Request.Context(), c.Param("node_id"), change, subject.UserID)
	if err != nil {
		clusterError(c, err)
		return
	}
	response.Success(c, op)
}
func (h *OpsHandler) ClusterV3Operations(c *gin.Context) {
	if !h.clusterEnabled(c) {
		return
	}
	if key := c.Query("request_key"); key != "" {
		op, err := h.cluster.Store.OperationByRequestKey(c.Request.Context(), key)
		if err != nil {
			clusterError(c, err)
			return
		}
		response.Success(c, op)
		return
	}
	ops, err := h.cluster.Store.Operations(c.Request.Context())
	if err != nil {
		clusterError(c, err)
		return
	}
	response.Success(c, ops)
}
func clusterError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, cluster.ErrInvalid):
		response.Error(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, cluster.ErrConflict):
		response.Error(c, http.StatusConflict, "Node or operation revision conflict")
	case errors.Is(err, cluster.ErrForbidden):
		response.Error(c, http.StatusForbidden, "Node identity is not authorized")
	case errors.Is(err, cluster.ErrNotFound):
		response.Error(c, http.StatusNotFound, "Node not found")
	case errors.Is(err, cluster.ErrNotReady), errors.Is(err, cluster.ErrUnsafeExit):
		response.Error(c, http.StatusConflict, err.Error())
	default:
		// An unknown database/commit result is not proof that the operation was rejected.
		response.Error(c, http.StatusServiceUnavailable, "Cluster operation result is unconfirmed; query the request before retrying")
	}
}

func (h *OpsHandler) ClusterV3Storage(c *gin.Context) {
	if !h.clusterEnabled(c) {
		return
	}
	view, err := h.cluster.Store.Storage(c.Request.Context())
	if err != nil {
		clusterError(c, err)
		return
	}
	view.DeletionEnabled = h.cluster.RetentionApproved()
	response.Success(c, view)
}
func (h *OpsHandler) ClusterV3RetentionPreview(c *gin.Context) {
	if !h.clusterEnabled(c) {
		return
	}
	days, err := strconv.Atoi(c.Query("days"))
	if err != nil {
		response.BadRequest(c, "Invalid retention days")
		return
	}
	preview, err := h.cluster.Store.PreviewRetention(c.Request.Context(), c.Query("table"), days)
	if err != nil {
		clusterError(c, err)
		return
	}
	response.Success(c, preview)
}
func (h *OpsHandler) ClusterV3RedisRecovery(c *gin.Context) {
	if !h.clusterEnabled(c) {
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	var input struct {
		Generation string `json:"generation"`
		Confirm    bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&input) != nil || !input.Confirm {
		response.BadRequest(c, "Explicit recovery confirmation required")
		return
	}
	if err := h.cluster.AcknowledgeRedisRecovery(c.Request.Context(), input.Generation, subject.UserID); err != nil {
		clusterError(c, err)
		return
	}
	response.Success(c, gin.H{"acknowledged": true})
}
