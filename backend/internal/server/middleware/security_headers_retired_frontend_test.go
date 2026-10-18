package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Retired app URLs must not retain the former same-origin iframe exception.
func TestSecurityHeadersRetiredFrontendFraming(t *testing.T) {
	for _, path := range []string{"/", "/dashboard", "/keys", "/studio", "/studio/", "/image-studio", "/image-studio/", "/image-studio/assets/index.js"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, path, nil)
			SecurityHeaders(config.CSPConfig{Enabled: true, Policy: config.DefaultCSPPolicy}, nil)(c)
			assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
			assert.Contains(t, w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
		})
	}
}
