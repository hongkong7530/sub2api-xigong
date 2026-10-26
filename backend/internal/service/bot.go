package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const BotSignatureWindow = 5 * time.Minute
const BotResponseLimit = 2 * 1024 * 1024

var (
	ErrBotUnauthorized  = infraerrors.Unauthorized("BOT_UNAUTHORIZED", "Invalid bot authorization or signature")
	ErrBotForbidden     = infraerrors.Forbidden("BOT_FORBIDDEN", "Bot identity or data scope is not authorized")
	ErrBotInvalid       = infraerrors.BadRequest("BOT_INVALID", "Invalid bot request or installation policy")
	ErrBotUnavailable   = infraerrors.New(503, "BOT_UNAVAILABLE", "Bot service temporarily unavailable")
	ErrBotReplay        = infraerrors.Conflict("BOT_REPLAY", "Bot request has already been used")
	ErrBotRateLimit     = infraerrors.New(429, "BOT_RATE_LIMIT", "Bot request rate exceeded")
	ErrBotUnsupported   = infraerrors.New(501, "BOT_UNSUPPORTED", "This query is not supported by this bot API")
	botIDPattern        = regexp.MustCompile(`^[0-9a-f]{32}$`)
	botNoncePattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	botSignaturePattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// BotIdentity binds the entire signed event tuple; independent allowlists would permit unintended combinations.
type BotIdentity struct {
	User         string `json:"platform_user"`
	Session      string `json:"session_id"`
	Conversation string `json:"conversation_id"`
}

type BotPolicy struct {
	Name       string        `json:"name"`
	Platform   string        `json:"platform"`
	Identities []BotIdentity `json:"identities"`
	// Empty scopes deny access. AllData must be an explicit administrator decision, never an empty-list default.
	AllData           bool      `json:"all_data"`
	UserIDs           []int64   `json:"user_ids"`
	GroupIDs          []int64   `json:"group_ids"`
	ExpiresAt         time.Time `json:"expires_at"`
	RequestsPerMinute int       `json:"requests_per_minute"`
}

type BotInstallation struct {
	ID string `json:"installation_id"`
	BotPolicy
	Scope             string     `json:"scope"`
	CreatedAt         time.Time  `json:"created_at"`
	RevokedAt         *time.Time `json:"revoked_at"`
	TokenHash         string     `json:"-"`
	SigningCiphertext string     `json:"-"`
}

type BotCredentials struct {
	Installation       *BotInstallation `json:"installation"`
	BearerToken        string           `json:"bearer_token"`
	InstallationID     string           `json:"installation_id"`
	InstallationSecret string           `json:"installation_secret"`
}

type BotRepository interface {
	Create(context.Context, *BotInstallation) error
	Get(context.Context, string) (*BotInstallation, error)
	List(context.Context) ([]BotInstallation, error)
	Revoke(context.Context, string) error
	Query(context.Context, string, *BotInstallation, BotQuery) (json.RawMessage, error)
}

type BotRequestGuard interface {
	Consume(context.Context, string, string, time.Duration, int) (bool, bool, error)
}

type BotService struct {
	repo      BotRepository
	guard     BotRequestGuard
	encryptor SecretEncryptor
	now       func() time.Time
}

func NewBotService(repo BotRepository, guard BotRequestGuard, encryptor SecretEncryptor) *BotService {
	return &BotService{repo: repo, guard: guard, encryptor: encryptor, now: time.Now}
}

func botValidIdentity(s string) bool {
	return s != "" && s == strings.TrimSpace(s) && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 256 && !strings.ContainsAny(s, "\r\n\x00")
}

func (s *BotService) Create(ctx context.Context, p BotPolicy) (*BotCredentials, error) {
	now := s.now()
	if strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 100 || !botValidIdentity(p.Platform) || len(p.Identities) == 0 || len(p.Identities) > 100 || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(366*24*time.Hour)) {
		return nil, ErrBotInvalid
	}
	if p.RequestsPerMinute == 0 {
		p.RequestsPerMinute = 60
	}
	if p.RequestsPerMinute < 1 || p.RequestsPerMinute > 600 {
		return nil, ErrBotInvalid
	}
	if p.AllData && (len(p.UserIDs) > 0 || len(p.GroupIDs) > 0) || !p.AllData && len(p.UserIDs) == 0 && len(p.GroupIDs) == 0 {
		return nil, ErrBotInvalid
	}
	for _, ids := range [][]int64{p.UserIDs, p.GroupIDs} {
		if len(ids) > 1000 {
			return nil, ErrBotInvalid
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if id <= 0 || seen[id] {
				return nil, ErrBotInvalid
			}
			seen[id] = true
		}
	}
	seen := map[BotIdentity]bool{}
	for _, i := range p.Identities {
		if !botValidIdentity(i.User) || !botValidIdentity(i.Session) || !botValidIdentity(i.Conversation) || seen[i] {
			return nil, ErrBotInvalid
		}
		seen[i] = true
	}
	random := make([]byte, 80)
	if _, e := rand.Read(random); e != nil {
		return nil, ErrBotUnavailable
	}
	id := hex.EncodeToString(random[:16])
	token := "astrbot_" + id + "_" + hex.EncodeToString(random[16:48])
	secret := hex.EncodeToString(random[48:])
	encrypted, e := s.encryptor.Encrypt(id + ":" + secret)
	if e != nil {
		return nil, ErrBotUnavailable
	}
	hash := sha256.Sum256([]byte(token))
	v := &BotInstallation{ID: id, BotPolicy: p, Scope: "bot:read", CreatedAt: now.UTC(), TokenHash: hex.EncodeToString(hash[:]), SigningCiphertext: encrypted}
	if e = s.repo.Create(ctx, v); e != nil {
		return nil, ErrBotUnavailable
	}
	return &BotCredentials{Installation: v, BearerToken: token, InstallationID: id, InstallationSecret: secret}, nil
}

func (s *BotService) List(ctx context.Context) ([]BotInstallation, error) { return s.repo.List(ctx) }
func (s *BotService) Revoke(ctx context.Context, id string) error {
	if !botIDPattern.MatchString(id) {
		return ErrBotInvalid
	}
	return s.repo.Revoke(ctx, id)
}

// BotCanonical signs the exact escaped external path and raw query, never a decoded/reordered reconstruction.
func BotCanonical(method, path, timestamp, nonce string, identity BotIdentity, body string) string {
	return strings.Join([]string{strings.ToUpper(strings.TrimSpace(method)), path, timestamp, nonce, strings.TrimSpace(identity.User), strings.TrimSpace(identity.Session), strings.TrimSpace(identity.Conversation), body}, "\n")
}

func (s *BotService) Authenticate(ctx context.Context, r *http.Request) (*BotInstallation, error) {
	one := func(key string) string {
		v := r.Header.Values(key)
		if len(v) != 1 {
			return ""
		}
		return v[0]
	}
	authorization := one("Authorization")
	parts := strings.Split(authorization, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, ErrBotUnauthorized
	}
	token := parts[1]
	if len(token) != 105 || !strings.HasPrefix(token, "astrbot_") || token[40] != '_' {
		return nil, ErrBotUnauthorized
	}
	id := token[8:40]
	if !botIDPattern.MatchString(id) {
		return nil, ErrBotUnauthorized
	}
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return nil, ErrBotUnavailable
	}
	now := s.now()
	if v == nil || v.RevokedAt != nil || !v.ExpiresAt.After(now) || v.Scope != "bot:read" {
		return nil, ErrBotUnauthorized
	}
	actual := sha256.Sum256([]byte(token))
	expected, e := hex.DecodeString(v.TokenHash)
	if e != nil || !hmac.Equal(actual[:], expected) {
		return nil, ErrBotUnauthorized
	}
	timestamp, nonce, signature := one("X-AstrBot-Timestamp"), one("X-AstrBot-Nonce"), one("X-AstrBot-Signature")
	stamp, e := time.Parse("2006-01-02T15:04:05Z", timestamp)
	if e != nil || stamp.Before(now.Add(-BotSignatureWindow)) || stamp.After(now.Add(BotSignatureWindow)) || !botNoncePattern.MatchString(nonce) || !botSignaturePattern.MatchString(signature) {
		return nil, ErrBotUnauthorized
	}
	i := BotIdentity{one("X-AstrBot-Platform-User"), one("X-AstrBot-Session-ID"), one("X-AstrBot-Conversation-ID")}
	if !botValidIdentity(i.User) || !botValidIdentity(i.Session) || !botValidIdentity(i.Conversation) {
		return nil, ErrBotUnauthorized
	}
	// The 0.1.2 client sends neither installation nor platform headers; token binding owns both.
	secret, e := s.encryptor.Decrypt(v.SigningCiphertext)
	if e != nil || !strings.HasPrefix(secret, id+":") {
		return nil, ErrBotUnavailable
	}
	secret = strings.TrimPrefix(secret, id+":")
	path := r.URL.EscapedPath()
	if r.URL.ForceQuery || r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	if r.RequestURI != "" && r.RequestURI != path {
		return nil, ErrBotUnauthorized
	}
	if r.Method != http.MethodGet || r.ContentLength > 0 || len(r.TransferEncoding) > 0 {
		return nil, ErrBotForbidden
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(BotCanonical(r.Method, path, timestamp, nonce, i, "")))
	supplied, _ := hex.DecodeString(signature)
	if !hmac.Equal(mac.Sum(nil), supplied) {
		return nil, ErrBotUnauthorized
	}
	if !slices.Contains(v.Identities, i) || v.Platform == "" {
		return nil, ErrBotForbidden
	}
	// Preserve the nonce until the signed timestamp can no longer be accepted, including future-clock skew.
	fresh, limited, e := s.guard.Consume(ctx, id, nonce, stamp.Add(BotSignatureWindow+time.Second).Sub(now), v.RequestsPerMinute)
	if e != nil {
		return nil, ErrBotUnavailable
	}
	if !fresh {
		return nil, ErrBotReplay
	}
	if limited {
		return nil, ErrBotRateLimit
	}
	return v, nil
}

type BotQuery struct {
	Start    time.Time
	End      time.Time
	Page     int
	PageSize int
	Values   url.Values
}

func (q BotQuery) ID(key string) int64 { v, _ := strconv.ParseInt(q.Values.Get(key), 10, 64); return v }

// ParseBotQuery accepts only filters that the selected route actually implements; silent filtering loss is unsafe.
func ParseBotQuery(route, raw string, now time.Time, v *BotInstallation) (BotQuery, error) {
	q := BotQuery{Page: 1, PageSize: 50, End: now.UTC(), Start: now.UTC().Add(-24 * time.Hour)}
	if len(raw) > 8192 {
		return q, ErrBotInvalid
	}
	values, e := url.ParseQuery(raw)
	if e != nil {
		return q, ErrBotInvalid
	}
	q.Values = values
	allowed := ""
	switch route {
	case "costs", "profit":
		allowed = "start,end,page,page_size,platform,group_id,user_id,type"
	case "consumption":
		allowed = "start,end,page,page_size,platform,group_id,user_id,account_id,channel_id,requested_model,upstream_model,upstream_response_model"
	case "logs", "model-audit":
		allowed = "start,end,page,page_size,platform,group_id,user_id,account_id,channel_id,requested_model,upstream_model,upstream_response_model,status,mismatch,log_source"
	case "accounts/status":
		allowed = "page,page_size,platform,group_id,account_id,type"
	case "channels/status":
		allowed = "page,page_size,platform,group_id,channel_id"
	case "models/prices":
		allowed = "page,page_size,platform,group_id"
	default:
		return q, ErrBotUnsupported
	}
	for key, vs := range values {
		if !slices.Contains(strings.Split(allowed, ","), key) || len(vs) != 1 || vs[0] == "" || !utf8.ValidString(vs[0]) || utf8.RuneCountInString(vs[0]) > 256 {
			return q, ErrBotInvalid
		}
		value := vs[0]
		switch key {
		case "page", "page_size":
			n, e := strconv.Atoi(value)
			max := 100000
			if key == "page_size" {
				max = 1000
			}
			if e != nil || n < 1 || n > max {
				return q, ErrBotInvalid
			}
			if key == "page" {
				q.Page = n
			} else {
				q.PageSize = n
			}
		case "group_id", "user_id", "account_id", "channel_id":
			n, e := strconv.ParseInt(value, 10, 64)
			if e != nil || n <= 0 {
				return q, ErrBotInvalid
			}
		case "start", "end":
			t, e := time.Parse(time.RFC3339, value)
			if e != nil {
				return q, ErrBotInvalid
			}
			if key == "start" {
				q.Start = t
			} else {
				q.End = t
			}
		case "status":
			if value != "all" && value != "success" && value != "failed" {
				return q, ErrBotInvalid
			}
			if value != "all" {
				return q, ErrBotUnsupported
			}
		case "mismatch":
			if value != "true" && value != "false" {
				return q, ErrBotInvalid
			}
		case "log_source":
			if value != "usage" {
				return q, ErrBotUnsupported
			}
		case "type":
			if route == "costs" || route == "profit" {
				if value != "balance" && value != "subscription" {
					return q, ErrBotInvalid
				}
			}
		}
	}
	if values.Get("start") == "" {
		q.Start = q.End.Add(-24 * time.Hour)
	}
	if !q.Start.Before(q.End) || q.End.Sub(q.Start) > 366*24*time.Hour {
		return q, ErrBotInvalid
	}
	if !v.AllData {
		if len(v.UserIDs) == 0 && len(v.GroupIDs) == 0 {
			return q, ErrBotForbidden
		}
		if id := q.ID("user_id"); id > 0 && len(v.UserIDs) > 0 && !slices.Contains(v.UserIDs, id) {
			return q, ErrBotForbidden
		}
		if id := q.ID("group_id"); id > 0 && len(v.GroupIDs) > 0 && !slices.Contains(v.GroupIDs, id) {
			return q, ErrBotForbidden
		}
		// Account/channel inventory and configured prices are group-level data, not per-user data.
		if len(v.UserIDs) > 0 && (route == "accounts/status" || route == "channels/status" || route == "models/prices") {
			return q, ErrBotForbidden
		}
	}
	return q, nil
}

func (s *BotService) Query(ctx context.Context, route, raw string, v *BotInstallation) (json.RawMessage, error) {
	q, e := ParseBotQuery(route, raw, s.now(), v)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, e := s.repo.Query(ctx, route, v, q)
	if e != nil {
		if errors.Is(e, ErrBotUnsupported) {
			return nil, e
		}
		return nil, ErrBotUnavailable
	}
	if len(result) > BotResponseLimit-128 {
		return nil, infraerrors.New(413, "BOT_RESPONSE_TOO_LARGE", "Reduce page_size or the requested range")
	}
	return result, nil
}
