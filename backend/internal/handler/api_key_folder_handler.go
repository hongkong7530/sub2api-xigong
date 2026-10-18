package handler

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type APIKeyFolderHandler struct {
	svc *service.APIKeyFolderService
}

func NewAPIKeyFolderHandler(svc *service.APIKeyFolderService) *APIKeyFolderHandler {
	return &APIKeyFolderHandler{svc: svc}
}

type apiKeyFolderRequest struct {
	Name string `json:"name"`
}

func (h *APIKeyFolderHandler) List(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured"))
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	items, err := h.svc.List(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *APIKeyFolderHandler) Create(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured"))
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req apiKeyFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.svc.Create(c.Request.Context(), subject.UserID, req.Name)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *APIKeyFolderHandler) Update(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured"))
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid folder ID")
		return
	}
	var req apiKeyFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.svc.Update(c.Request.Context(), subject.UserID, id, req.Name)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *APIKeyFolderHandler) Delete(c *gin.Context) {
	if h == nil || h.svc == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("API_KEY_FOLDER_UNAVAILABLE", "api key folder service is not configured"))
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid folder ID")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "folder deleted"})
}
