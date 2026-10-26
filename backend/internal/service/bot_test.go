//go:build unit

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type botTestRepo struct {
	v    *BotInstallation
	fail bool
}

func (r *botTestRepo) Create(_ context.Context, v *BotInstallation) error { r.v = v; return nil }
func (r *botTestRepo) Get(context.Context, string) (*BotInstallation, error) {
	if r.fail {
		return nil, errors.New("db unavailable")
	}
	return r.v, nil
}
func (r *botTestRepo) List(context.Context) ([]BotInstallation, error) {
	return []BotInstallation{*r.v}, nil
}
func (r *botTestRepo) Revoke(context.Context, string) error {
	now := time.Now()
	r.v.RevokedAt = &now
	return nil
}
func (r *botTestRepo) Query(context.Context, string, *BotInstallation, BotQuery) (json.RawMessage, error) {
	return json.RawMessage(`{"items":[]}`), nil
}

type botTestCrypto struct{}

func (botTestCrypto) Encrypt(s string) (string, error) { return "encrypted:" + s, nil }
func (botTestCrypto) Decrypt(s string) (string, error) {
	return strings.TrimPrefix(s, "encrypted:"), nil
}

type botTestGuard struct {
	mu   sync.Mutex
	used map[string]bool
	ttl  time.Duration
	fail bool
}

func (g *botTestGuard) Consume(_ context.Context, id, nonce string, ttl time.Duration, _ int) (bool, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ttl = ttl
	if g.fail {
		return false, false, errors.New("redis unavailable")
	}
	key := id + ":" + nonce
	if g.used[key] {
		return false, false, nil
	}
	g.used[key] = true
	return true, false, nil
}

func newBotFixture(t *testing.T) (*BotService, *botTestRepo, *botTestGuard, *BotCredentials) {
	t.Helper()
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	repo := &botTestRepo{}
	guard := &botTestGuard{used: map[string]bool{}}
	svc := NewBotService(repo, guard, botTestCrypto{})
	svc.now = func() time.Time { return now }
	v, e := svc.Create(context.Background(), BotPolicy{Name: "fixture", Platform: "napcat_qq", Identities: []BotIdentity{{"123456789", "private", "private"}}, GroupIDs: []int64{7}, ExpiresAt: now.Add(time.Hour)})
	require.NoError(t, e)
	return svc, repo, guard, v
}
func botSigned(v *BotCredentials, target string, at time.Time, nonce string) *http.Request {
	r := httptest.NewRequest("GET", target, nil)
	i := v.Installation.Identities[0]
	stamp := at.UTC().Format("2006-01-02T15:04:05Z")
	r.Header.Set("Authorization", "Bearer "+v.BearerToken)
	r.Header.Set("X-AstrBot-Timestamp", stamp)
	r.Header.Set("X-AstrBot-Nonce", nonce)
	r.Header.Set("X-AstrBot-Platform-User", i.User)
	r.Header.Set("X-AstrBot-Session-ID", i.Session)
	r.Header.Set("X-AstrBot-Conversation-ID", i.Conversation)
	m := hmac.New(sha256.New, []byte(v.InstallationSecret))
	_, _ = m.Write([]byte(BotCanonical("GET", r.URL.RequestURI(), stamp, nonce, i, "")))
	r.Header.Set("X-AstrBot-Signature", hex.EncodeToString(m.Sum(nil)))
	return r
}
func TestBotCanonicalCompatibility(t *testing.T) {
	i := BotIdentity{"123456789", "example-private-session", "example-private-session"}
	canonical := BotCanonical(" GET ", "/api/v1/bot/costs", "2026-10-20T00:00:00Z", "example-nonce-001", i, "")
	require.True(t, strings.HasSuffix(canonical, "\n"))
	require.Equal(t, 7, strings.Count(canonical, "\n"))
	m := hmac.New(sha256.New, []byte("example-signing-secret-not-a-credential"))
	_, _ = m.Write([]byte(canonical))
	require.Equal(t, "a47cf5d545d836d84d5fc2cb566b9d58b5cacb1646f8c370766238c89ba2df06", hex.EncodeToString(m.Sum(nil)))
}
func TestBotAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		change func(*http.Request, *BotService, *botTestRepo, *botTestGuard)
		want   error
	}{
		{"valid", func(*http.Request, *BotService, *botTestRepo, *botTestGuard) {}, nil},
		{"wrong_token", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Set("Authorization", r.Header.Get("Authorization")+"x")
		}, ErrBotUnauthorized},
		{"admin_key", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Set("Authorization", "Bearer admin-not-bot")
		}, ErrBotUnauthorized},
		{"bad_signature", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Set("X-AstrBot-Signature", strings.Repeat("0", 64))
		}, ErrBotUnauthorized},
		{"query_order", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.URL.RawQuery = "page=1&search=a+b"
			r.RequestURI = r.URL.RequestURI()
		}, ErrBotUnauthorized},
		{"query_encoding", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.URL.RawQuery = "search=a%20b&page=1"
			r.RequestURI = r.URL.RequestURI()
		}, ErrBotUnauthorized},
		{"changed_user", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Set("X-AstrBot-Platform-User", "other")
		}, ErrBotUnauthorized},
		{"changed_session", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Set("X-AstrBot-Session-ID", "other")
		}, ErrBotUnauthorized},
		{"changed_conversation", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Set("X-AstrBot-Conversation-ID", "other")
		}, ErrBotUnauthorized},
		{"duplicate_header", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Add("X-AstrBot-Nonce", "new-nonce-000000000")
		}, ErrBotUnauthorized},
		{"identity_newline", func(r *http.Request, _ *BotService, _ *botTestRepo, _ *botTestGuard) {
			r.Header.Set("X-AstrBot-Session-ID", "private\n")
		}, ErrBotUnauthorized},
		{"expired_stamp", func(_ *http.Request, s *BotService, _ *botTestRepo, _ *botTestGuard) {
			s.now = func() time.Time { return time.Date(2026, 10, 20, 0, 6, 0, 0, time.UTC) }
		}, ErrBotUnauthorized},
		{"revoked", func(_ *http.Request, s *BotService, r *botTestRepo, _ *botTestGuard) {
			v := s.now()
			r.v.RevokedAt = &v
		}, ErrBotUnauthorized},
		{"expired_token", func(_ *http.Request, s *BotService, r *botTestRepo, _ *botTestGuard) { r.v.ExpiresAt = s.now() }, ErrBotUnauthorized},
		{"write_scope", func(_ *http.Request, _ *BotService, r *botTestRepo, _ *botTestGuard) { r.v.Scope = "bot:write" }, ErrBotUnauthorized},
		{"wrong_allowed_identity", func(_ *http.Request, _ *BotService, r *botTestRepo, _ *botTestGuard) {
			r.v.Identities[0].Conversation = "other"
		}, ErrBotForbidden},
		{"db_failure", func(_ *http.Request, _ *BotService, r *botTestRepo, _ *botTestGuard) { r.fail = true }, ErrBotUnavailable},
		{"redis_failure", func(_ *http.Request, _ *BotService, _ *botTestRepo, g *botTestGuard) { g.fail = true }, ErrBotUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, repo, g, v := newBotFixture(t)
			r := botSigned(v, "/api/v1/bot/logs?search=a+b&page=1", s.now(), "nonce-000000000001")
			tt.change(r, s, repo, g)
			_, e := s.Authenticate(context.Background(), r)
			require.ErrorIs(t, e, tt.want)
		})
	}
}
func TestBotReplayAndFutureTimestamp(t *testing.T) {
	s, repo, g, v := newBotFixture(t)
	r := botSigned(v, "/api/v1/bot/costs", s.now().Add(5*time.Minute), "nonce-000000000001")
	_, e := s.Authenticate(context.Background(), r)
	require.NoError(t, e)
	require.Greater(t, g.ttl, 9*time.Minute)
	second := NewBotService(repo, g, botTestCrypto{})
	second.now = s.now
	_, e = second.Authenticate(context.Background(), r)
	require.ErrorIs(t, e, ErrBotReplay)
	require.NotEqual(t, v.BearerToken, repo.v.TokenHash)
	require.NotContains(t, repo.v.SigningCiphertext, v.BearerToken)
	raw, e := json.Marshal(repo.v)
	require.NoError(t, e)
	require.NotContains(t, string(raw), v.InstallationSecret)
	require.NotContains(t, string(raw), v.BearerToken)
}
func TestBotPolicyAndQueryScope(t *testing.T) {
	s, _, _, v := newBotFixture(t)
	base := v.Installation.BotPolicy
	for _, mutate := range []func(*BotPolicy){func(p *BotPolicy) { p.GroupIDs = nil }, func(p *BotPolicy) { p.AllData = true }, func(p *BotPolicy) { p.GroupIDs = []int64{-1} }, func(p *BotPolicy) { p.Identities = nil }, func(p *BotPolicy) { p.ExpiresAt = s.now() }, func(p *BotPolicy) { p.RequestsPerMinute = 601 }} {
		p := base
		mutate(&p)
		_, e := s.Create(context.Background(), p)
		require.ErrorIs(t, e, ErrBotInvalid)
	}
	for _, tc := range []struct {
		route, query string
		want         error
	}{
		{"costs", "group_id=7", nil}, {"costs", "group_id=8", ErrBotForbidden}, {"costs", "user_id=9", nil},
		{"logs", "page=1&page=2", ErrBotInvalid}, {"logs", "search=ignored", ErrBotInvalid}, {"logs", "page_size=1001", ErrBotInvalid},
		{"logs", "status=failed", ErrBotUnsupported}, {"logs", "status=success", ErrBotUnsupported}, {"logs", "log_source=system", ErrBotUnsupported},
		{"model-audit", "mismatch=true", nil}, {"model-audit", "mismatch=yes", ErrBotInvalid},
		{"costs", "start=2025-01-01T00%3A00%3A00Z&end=2026-10-20T00%3A00%3A00Z", ErrBotInvalid},
		{"operations/execute", "", ErrBotUnsupported},
	} {
		t.Run(tc.route+tc.query, func(t *testing.T) {
			_, e := ParseBotQuery(tc.route, tc.query, s.now(), v.Installation)
			require.ErrorIs(t, e, tc.want)
		})
	}
	v.Installation.UserIDs = []int64{9}
	_, e := ParseBotQuery("accounts/status", "", s.now(), v.Installation)
	require.ErrorIs(t, e, ErrBotForbidden)
}
