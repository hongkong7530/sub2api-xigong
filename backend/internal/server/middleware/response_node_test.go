package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponseNodeIsServerOwnedAndExposedForErrors(t *testing.T) {
	r := gin.New()
	r.Use(ResponseNode("xigong3"), CORS(config.CORSConfig{AllowedOrigins: []string{"https://example.test"}}))
	r.GET("/error", func(c *gin.Context) { c.AbortWithStatus(http.StatusServiceUnavailable) })
	req := httptest.NewRequest("GET", "/error", nil)
	req.Header.Set("X-Sub2api-App-Node", "spoofed")
	req.Header.Set("Origin", "https://example.test")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "xigong3", rec.Header().Get("X-Sub2api-App-Node"))
	require.Contains(t, rec.Header().Get("Access-Control-Expose-Headers"), "X-Sub2api-App-Node")
}
