package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type opsClusterConfigRequest struct {
	NodeName              string  `json:"node_name"`
	Enabled               bool    `json:"enabled"`
	CPULimitPercent       float64 `json:"cpu_limit_percent"`
	MinAvailableMemoryMB  int64   `json:"min_available_memory_mb"`
	DesiredWeight         int     `json:"desired_weight"`
	RecoveryStableSeconds int     `json:"recovery_stable_seconds"`
	ExpectedRevision      int64   `json:"expected_revision"`
}

type opsClusterReapplyRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

func (h *OpsHandler) GetCluster(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	start, end, err := parseOpsTimeRange(c, "5m")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	snapshot, err := h.opsService.GetClusterSnapshot(c.Request.Context(), start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}

func (h *OpsHandler) GetClusterLive(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	snapshot, err := h.opsService.GetClusterLiveSnapshot(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}

func (h *OpsHandler) GetClusterNode(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	start, end, err := parseOpsTimeRange(c, "5m")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	node, err := h.opsService.GetClusterNode(c.Request.Context(), strings.TrimSpace(c.Param("node_id")), start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, node)
}

func (h *OpsHandler) UpdateClusterNodeConfig(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	var request opsClusterConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("OPS_CLUSTER_CONFIG_INVALID", err.Error()))
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	operatorID := subject.UserID
	updated, err := h.opsService.UpdateClusterNodeConfig(c.Request.Context(), strings.TrimSpace(c.Param("node_id")), service.OpsClusterNodeConfigUpdate{
		NodeName: request.NodeName, Enabled: request.Enabled, CPULimitPercent: request.CPULimitPercent,
		MinAvailableMemoryMB: request.MinAvailableMemoryMB, DesiredWeight: request.DesiredWeight,
		RecoveryStableSeconds: request.RecoveryStableSeconds, ExpectedRevision: request.ExpectedRevision,
		OperatorUserID: &operatorID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

func (h *OpsHandler) ReapplyClusterNodeConfig(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	var request opsClusterReapplyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("OPS_CLUSTER_REAPPLY_INVALID", err.Error()))
		return
	}
	if request.ExpectedRevision < 1 {
		if raw := strings.TrimSpace(c.Query("expected_revision")); raw != "" {
			request.ExpectedRevision, _ = strconv.ParseInt(raw, 10, 64)
		}
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	operatorID := subject.UserID
	updated, err := h.opsService.ReapplyClusterNodeConfig(c.Request.Context(), strings.TrimSpace(c.Param("node_id")), request.ExpectedRevision, &operatorID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"config": updated, "queued_at": time.Now().UTC()})
}
