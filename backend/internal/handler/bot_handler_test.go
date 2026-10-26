//go:build unit

package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type botHTTPRepo struct {
	v       *service.BotInstallation
	queried int
}

func (r *botHTTPRepo) Create(_ context.Context, v *service.BotInstallation) error {
	r.v = v
	return nil
}
func (r *botHTTPRepo) Get(context.Context, string) (*service.BotInstallation, error) { return r.v, nil }
func (r *botHTTPRepo) List(context.Context) ([]service.BotInstallation, error) {
	return []service.BotInstallation{*r.v}, nil
}
func (r *botHTTPRepo) Revoke(context.Context, string) error {
	now := time.Now()
	r.v.RevokedAt = &now
	return nil
}
func (r *botHTTPRepo) Query(context.Context, string, *service.BotInstallation, service.BotQuery) (json.RawMessage, error) {
	r.queried++
	return json.RawMessage(`{"currency":"USD","requests":3}`), nil
}

type botHTTPCrypto struct{}

func (botHTTPCrypto) Encrypt(s string) (string, error) { return s, nil }
func (botHTTPCrypto) Decrypt(s string) (string, error) { return s, nil }

type botHTTPGuard struct{ used map[string]bool }

func (g *botHTTPGuard) Consume(_ context.Context, _ string, n string, _ time.Duration, _ int) (bool, bool, error) {
	if g.used[n] {
		return false, false, nil
	}
	g.used[n] = true
	return true, false, nil
}
func TestBotHTTPReadOnlyAndProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &botHTTPRepo{}
	svc := service.NewBotService(repo, &botHTTPGuard{used: map[string]bool{}}, botHTTPCrypto{})
	creds, e := svc.Create(context.Background(), service.BotPolicy{Name: "http fixture", Platform: "napcat_qq", Identities: []service.BotIdentity{{User: "123456789", Session: "private", Conversation: "private"}}, GroupIDs: []int64{7}, ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, e)
	h := NewBotHandler(svc, nil)
	router := gin.New()
	router.Any("/api/v1/bot/*path", h.Read)
	signed := func(method, path, nonce string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		stamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		i := creds.Installation.Identities[0]
		r.Header.Set("Authorization", "Bearer "+creds.BearerToken)
		r.Header.Set("X-AstrBot-Timestamp", stamp)
		r.Header.Set("X-AstrBot-Nonce", nonce)
		r.Header.Set("X-AstrBot-Platform-User", i.User)
		r.Header.Set("X-AstrBot-Session-ID", i.Session)
		r.Header.Set("X-AstrBot-Conversation-ID", i.Conversation)
		m := hmac.New(sha256.New, []byte(creds.InstallationSecret))
		_, _ = m.Write([]byte(service.BotCanonical(method, r.URL.RequestURI(), stamp, nonce, i, "")))
		r.Header.Set("X-AstrBot-Signature", hex.EncodeToString(m.Sum(nil)))
		return r
	}
	run := func(r *http.Request, want int) string {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, want, w.Code, w.Body.String())
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		return w.Body.String()
	}
	raw := run(signed("GET", "/api/v1/bot/costs", "nonce-000000000001"), 200)
	require.Contains(t, raw, `"code":0`)
	require.Contains(t, raw, `"requests":3`)
	run(signed("GET", "/api/v1/bot/costs", "nonce-000000000001"), 409)
	run(signed("GET", "/api/v1/bot/costs?group_id=8", "nonce-000000000002"), 403)
	for n, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		run(signed(method, "/api/v1/bot/operations/execute", fmt.Sprintf("nonce-0000000000%02d", n+10)), 403)
	}
	run(signed("GET", "/api/v1/bot/unknown", "nonce-000000000020"), 501)
	run(httptest.NewRequest("GET", "/api/v1/bot/costs", nil), 401)
	req := signed("GET", "/api/v1/bot/costs", "nonce-000000000030")
	req.Body = http.NoBody
	req.ContentLength = 2
	run(req, 403)
	req = signed("GET", "/api/v1/bot/costs", "nonce-000000000031")
	req.Body = ioBody(`{}`)
	run(req, 400)
	require.Equal(t, 1, repo.queried)
}
func ioBody(s string) readCloser { return readCloser{strings.NewReader(s)} }

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }
