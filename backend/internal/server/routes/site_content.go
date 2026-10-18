package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterSiteContentRoutes(v1 *gin.RouterGroup, h *handler.Handlers, optional middleware.OptionalJWTAuthMiddleware, settings *service.SettingService, limiter *middleware.PanelRateLimiter) {
	content := v1.Group("/site-content")
	content.Use(limiter.PublicIP(), gin.HandlerFunc(optional), middleware.BackendModeUserGuard(settings))
	content.GET("/appearance", h.SiteContent.Appearance)
	content.GET("/stats", h.SiteContent.SiteStats)
	content.GET("/channel-monitors", h.ChannelMonitor.PublicList)
	content.GET("/quote", h.SiteContent.Quote)
	content.POST("/presence", h.SiteContent.Presence)
	content.GET("/assets/:asset", h.SiteContent.Asset)
	content.GET("/partners", h.SiteContent.PublicPartners)
	content.GET("/docs", h.SiteContent.PublicDocuments)
	content.GET("/docs/:slug", h.SiteContent.PublicDocument)
}

// Mounted only below the existing admin auth, audit, rate and compliance guards.
func registerSiteContentAdminRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	c := admin.Group("/site-content")
	c.GET("/appearance", h.SiteContent.Appearance)
	c.POST("/assets", h.SiteContent.UploadAsset)
	c.POST("/partners/resolve", h.SiteContent.ResolvePartner)
	c.PUT("/appearance", h.SiteContent.SaveAppearance)
	c.GET("/docs", h.SiteContent.AdminDocuments)
	c.GET("/docs/:id", h.SiteContent.AdminDocument)
	c.POST("/docs", h.SiteContent.SaveDocument)
	c.PUT("/docs/:id", h.SiteContent.SaveDocument)
	c.POST("/docs/:id/:action", h.SiteContent.DocumentAction)
	c.GET("/categories", h.SiteContent.Categories)
	c.POST("/categories", h.SiteContent.SaveCategory)
	c.PUT("/categories/:id", h.SiteContent.SaveCategory)
	c.DELETE("/categories/:id", h.SiteContent.DeleteCategory)
	c.GET("/partners", h.SiteContent.AdminPartners)
	c.POST("/partners", h.SiteContent.SavePartner)
	c.PUT("/partners/:id", h.SiteContent.SavePartner)
	c.DELETE("/partners/:id", h.SiteContent.DeletePartner)
}
