package middleware

import (
 "bytes"
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/Wei-Shaw/sub2api/internal/service"
 "github.com/gin-gonic/gin"
 "github.com/stretchr/testify/require"
)

func TestBotInstallationAuditOmitsAcceptedAndRejectedBodies(t *testing.T) {
 gin.SetMode(gin.TestMode)
 repository := &auditCaptureRepository{}
 audit := service.NewAuditLogService(repository, nil)
 audit.Start()
 router := gin.New()
 router.Use(gin.HandlerFunc(NewAuditLogMiddleware(audit)))
 router.POST("/api/v1/admin/bot/installations", func(c *gin.Context) {
  if c.GetHeader("X-Test-Reject") == "true" {
   c.Status(http.StatusBadRequest)
   return
  }
  c.Status(http.StatusCreated)
 })
 for _, rejected := range []bool{false, true} {
  body := `{"name":"audit-canary-free-text","identities":[{"platform_user":"audit-canary-user","session_id":"audit-canary-session","conversation_id":"audit-canary-conversation"}]}`
  if rejected { body = `"audit-canary-invalid-string"` }
  req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bot/installations", bytes.NewBufferString(body))
  req.Header.Set("Content-Type", "application/json")
  if rejected { req.Header.Set("X-Test-Reject", "true") }
  router.ServeHTTP(httptest.NewRecorder(), req)
 }
 audit.Stop()
 repository.mu.Lock()
 defer repository.mu.Unlock()
 require.Len(t, repository.logs, 2)
 for _, entry := range repository.logs {
  require.Equal(t, "<credential-bearing body omitted>", entry.RequestBody)
  require.NotContains(t, entry.RequestBody, "audit-canary")
 }
 require.Equal(t, http.StatusCreated, repository.logs[0].StatusCode)
 require.Equal(t, http.StatusBadRequest, repository.logs[1].StatusCode)
}
