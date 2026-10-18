package handler

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ChannelGroupReportHandler struct {
	svc *service.ChannelGroupReportService
}

func NewChannelGroupReportHandler(svc *service.ChannelGroupReportService) *ChannelGroupReportHandler {
	return &ChannelGroupReportHandler{svc: svc}
}

type channelGroupReportRequest struct {
	Category string `json:"category"`
	Body     string `json:"body"`
}

func (h *ChannelGroupReportHandler) Create(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("GROUP_REPORT_UNAVAILABLE", "group report service is not configured"))
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	groupID, err := strconv.ParseInt(c.Param("groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	var req channelGroupReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	report, err := h.svc.Create(c.Request.Context(), subject.UserID, groupID, req.Category, req.Body)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"id":         report.ID,
		"group_id":   report.GroupID,
		"category":   report.Category,
		"created_at": report.CreatedAt,
		"submitted":  true,
	})
}

func (h *ChannelGroupReportHandler) Submitted(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.Success(c, gin.H{"group_ids": []int64{}})
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	ids, err := h.svc.SubmittedGroupIDs(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if ids == nil {
		ids = []int64{}
	}
	response.Success(c, gin.H{"group_ids": ids})
}
