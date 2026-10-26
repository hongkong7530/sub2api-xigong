package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ratemetric"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestLoggerClockBeginsBeforeAuthAndIgnoresClientTiming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger())
	var clock ratemetric.Clock
	r.Use(func(c *gin.Context) {
		var ok bool
		clock, ok = ratemetric.RequestClock(c.Request.Context())
		require.True(t, ok)
		require.False(t, clock.Start.IsZero())
		time.Sleep(time.Millisecond)
		c.Next()
	})
	r.GET("/v1/models", func(c *gin.Context) {
		o := ratemetric.ForRequest(c.Request.Context(), nil, time.Now())
		require.NotNil(t, o.RequestDurationMs)
		require.GreaterOrEqual(t, *o.RequestDurationMs, 1)
		c.Status(200)
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Request-Started-At", "1970-01-01T00:00:00Z")
	before := time.Now()
	r.ServeHTTP(httptest.NewRecorder(), req)
	require.False(t, clock.Start.Before(before))
}
func TestRequestLoggerWebSocketClockNeverBecomesConnectionThroughput(t *testing.T) {
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/responses", func(c *gin.Context) {
		clock, ok := ratemetric.RequestClock(c.Request.Context())
		require.True(t, ok)
		require.True(t, clock.WebSocket)
		require.Nil(t, ratemetric.ForRequest(c.Request.Context(), nil, time.Now().Add(time.Hour)))
		c.Status(200)
	})
	req := httptest.NewRequest(http.MethodGet, "/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(httptest.NewRecorder(), req)
}
