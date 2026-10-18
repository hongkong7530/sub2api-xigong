package admin

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type GroupCategoryHandler struct {
	svc *service.GroupCategoryService
}

func NewGroupCategoryHandler(svc *service.GroupCategoryService) *GroupCategoryHandler {
	return &GroupCategoryHandler{svc: svc}
}

type groupCategoryRequest struct {
	Name string `json:"name"`
}

type groupCategorySortRequest struct {
	Items []service.GroupSortOrderUpdate `json:"items"`
}

func (h *GroupCategoryHandler) List(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured"))
		return
	}
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *GroupCategoryHandler) Create(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured"))
		return
	}
	var req groupCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.svc.Create(c.Request.Context(), req.Name)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *GroupCategoryHandler) Update(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured"))
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid category ID")
		return
	}
	var req groupCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.svc.Update(c.Request.Context(), id, req.Name)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *GroupCategoryHandler) Delete(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured"))
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid category ID")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "category deleted"})
}

func (h *GroupCategoryHandler) UpdateSortOrder(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("GROUP_CATEGORY_UNAVAILABLE", "group category service is not configured"))
		return
	}
	var req groupCategorySortRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.svc.UpdateSortOrders(c.Request.Context(), req.Items); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "sort order updated"})
}
