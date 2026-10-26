package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSiteProfitAdminRoutesStayBehindAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	auth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(401)
		} else {
			c.AbortWithStatus(403)
		}
	})
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	step := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(r.Group("/api/v1"), h, auth, audit, step, nil, nil)
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/admin/profit/overview"}, {"GET", "/api/v1/admin/profit/groups"}, {"GET", "/api/v1/admin/profit/entries"}, {"PUT", "/api/v1/admin/profit/groups/1/cost"}} {
		for _, authenticated := range []bool{false, true} {
			req := httptest.NewRequest(route.method, route.path, nil)
			want := http.StatusUnauthorized
			if authenticated {
				req.Header.Set("Authorization", "Bearer local-user-fixture")
				want = http.StatusForbidden
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, want, w.Code, route.path)
		}
	}
}
func TestSiteProfitUserRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := middleware.JWTAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(401) })
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	RegisterUserRoutes(r.Group("/api/v1"), &handler.Handlers{}, auth, audit, nil, nil)
	for _, path := range []string{"/api/v1/profit/disclosure", "/api/v1/user/profit-contribution"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 401, w.Code, path)
	}
}
