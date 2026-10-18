package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

// ClusterNodeGate applies to new HTTP work, including WebSocket upgrades.
// Protocol handlers still own admission of subsequent turns on an existing connection.
func ClusterNodeGate(runtime *cluster.Runtime, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.ClusterEnabled() {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		gateway := cluster.IsGatewayPath(path)
		// Processing ports must not bypass the authenticated node transport.
		if gateway && !cfg.RunsRole(config.RoleIngress) && c.Request.TLS == nil {
			host, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() || !cfg.RunsRole(config.RoleMaintenance) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}
		if gateway && !cfg.RunsRole(config.RoleWorker) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if !gateway {
			if path == "/health" {
				c.Next()
				return
			}
			if !cfg.RunsRole(config.RoleIngress) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.Next()
			return
		}
		if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") && !cfg.HasClusterCapability("websocket") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if (strings.Contains(path, "/live") || strings.Contains(path, "/realtime")) && !cfg.HasClusterCapability("live") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		// Polling already-created tasks is not a new execution and remains available while draining.
		polling := c.Request.Method == http.MethodGet && (strings.Contains(path, "/images/tasks/") || strings.Contains(path, "/contents/generations/tasks/"))
		if polling {
			c.Next()
			return
		}
		if runtime == nil || runtime.Gate == nil {
			c.AbortWithStatusJSON(503, gin.H{"error": gin.H{"code": "cluster_unavailable", "message": "Node admission is unavailable"}})
			return
		}
		if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			if !runtime.Gate.Ready() {
				c.AbortWithStatusJSON(503, gin.H{"error": gin.H{"code": "node_not_accepting", "message": "Node is not accepting new work"}})
				return
			}
		} else {
			release, err := runtime.Gate.Acquire(c.Request.Context(), time.Duration(cfg.Cluster.QueueTimeoutMS)*time.Millisecond)
			if err != nil {
				c.Header("Retry-After", "1")
				c.AbortWithStatusJSON(503, gin.H{"error": gin.H{"code": "node_not_accepting", "message": "Node is not accepting new work"}})
				return
			}
			defer release()
		}
		c.Request = c.Request.WithContext(cluster.WithRuntime(c.Request.Context(), runtime))
		c.Next()
		runtime.FinishExecution(c.Request.Context(), c.Writer.Status() >= 400)
	}
}
