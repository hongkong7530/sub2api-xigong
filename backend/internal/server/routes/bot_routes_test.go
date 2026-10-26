package routes

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBotInstallationManagementRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	auth := middleware.AdminAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(401) })
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	step := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(r.Group("/api/v1"), h, auth, audit, step, nil, nil)
	for _, method := range []string{"GET", "POST", "DELETE"} {
		path := "/api/v1/admin/bot/installations"
		if method == "DELETE" {
			path += "/fixture"
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer astrbot_fixture_not_admin")
		r.ServeHTTP(w, req)
		require.Equal(t, 401, w.Code)
	}
}
