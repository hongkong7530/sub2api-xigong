package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminJailbreakTemplateRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{Jailbreak: &admin.AdminJailbreakHandler{}}}
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) { c.Next() })
	audit := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	require.NotPanics(t, func() {
		RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, audit, stepUp, nil, nil)
		RegisterUserRoutes(router.Group("/api/v1"), handlers, servermiddleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }), audit, nil, nil)
	})
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /api/v1/admin/jailbreak/templates",
		"GET /api/v1/admin/api-keys/:id/jailbreak",
		"PATCH /api/v1/admin/api-keys/:id/jailbreak",
		"GET /api/v1/keys/:id/jailbreak",
		"PATCH /api/v1/keys/:id/jailbreak",
		"DELETE /api/v1/keys/:id/jailbreak",
		"GET /api/v1/jailbreak/templates",
		"POST /api/v1/admin/jailbreak/templates",
		"PUT /api/v1/admin/jailbreak/templates/:name",
		"DELETE /api/v1/admin/jailbreak/templates/:name",
	} {
		require.Truef(t, registered[expected], "missing route %s", expected)
	}
}
