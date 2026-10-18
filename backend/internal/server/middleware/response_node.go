package middleware

import "github.com/gin-gonic/gin"

// ResponseNode identifies the application answering this response, not the public
// reverse proxy or the worker that may answer a different gateway request.
func ResponseNode(nodeID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Sub2api-App-Node", nodeID)
		c.Next()
	}
}
