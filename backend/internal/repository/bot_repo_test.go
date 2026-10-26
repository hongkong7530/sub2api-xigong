//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBotRedisReplayAcrossInstancesAndRateLimit(t *testing.T) {
	m := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: m.Addr()})
	defer client.Close()
	a, b := NewBotRequestGuard(client), NewBotRequestGuard(client)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for n := 0; n < 16; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			guard := a
			if n%2 == 1 {
				guard = b
			}
			fresh, limited, e := guard.Consume(context.Background(), "fixture", "same-nonce", 10*time.Minute, 2)
			require.NoError(t, e)
			require.False(t, limited)
			if fresh {
				accepted.Add(1)
			}
		}(n)
	}
	wg.Wait()
	require.EqualValues(t, 1, accepted.Load())
	fresh, limited, e := b.Consume(context.Background(), "fixture", "second-nonce", 10*time.Minute, 2)
	require.NoError(t, e)
	require.True(t, fresh)
	require.False(t, limited)
	fresh, limited, e = a.Consume(context.Background(), "fixture", "third-nonce", 10*time.Minute, 2)
	require.NoError(t, e)
	require.True(t, fresh)
	require.True(t, limited)
	m.FastForward(6 * time.Minute)
	fresh, _, e = b.Consume(context.Background(), "fixture", "same-nonce", 10*time.Minute, 2)
	require.NoError(t, e)
	require.False(t, fresh)
	m.FastForward(5 * time.Minute)
	fresh, _, e = b.Consume(context.Background(), "fixture", "same-nonce", 10*time.Minute, 2)
	require.NoError(t, e)
	require.True(t, fresh)
	m.Close()
	_, _, e = a.Consume(context.Background(), "fixture", "offline", time.Minute, 2)
	require.Error(t, e)
}
func TestBotDataScopesAreAppliedBeforeAggregation(t *testing.T) {
	v := &service.BotInstallation{BotPolicy: service.BotPolicy{UserIDs: []int64{9}, GroupIDs: []int64{7}}}
	q := service.BotQuery{Values: map[string][]string{"user_id": {"9"}, "group_id": {"7"}}}
	f := botDataFilter(v, q, "p.user_id", "p.group_id", "p.platform")
	require.Contains(t, f.where, "p.user_id=ANY($1::bigint[])")
	require.Contains(t, f.where, "p.group_id=ANY($2::bigint[])")
	require.Contains(t, f.where, "p.user_id=$3")
	require.Contains(t, f.where, "p.group_id=$4")
	f = botDataFilter(v, q, "", "g.id", "g.platform")
	require.True(t, strings.HasPrefix(f.where, "false"))
	v.UserIDs = nil
	v.GroupIDs = nil
	f = botDataFilter(v, q, "p.user_id", "p.group_id", "p.platform")
	require.True(t, strings.HasPrefix(f.where, "false"))
}
func TestBotRepositoryPolicyRoundTripAndRevoke(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewBotRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	p := service.BotPolicy{Name: "fixture", Platform: "napcat_qq", Identities: []service.BotIdentity{{User: "u", Session: "s", Conversation: "c"}}, GroupIDs: []int64{7}, ExpiresAt: now.Add(time.Hour), RequestsPerMinute: 60}
	raw, e := json.Marshal(p)
	require.NoError(t, e)
	m.ExpectQuery("SELECT .* FROM bot_installations WHERE id").WithArgs("fixture").WillReturnRows(sqlmock.NewRows([]string{"id", "policy", "token_hash", "signing_ciphertext", "scope", "expires_at", "created_at", "revoked_at"}).AddRow("fixture", raw, "hash", "cipher", "bot:read", now.Add(time.Hour), now, nil))
	v, e := r.Get(context.Background(), "fixture")
	require.NoError(t, e)
	require.Equal(t, []int64{7}, v.GroupIDs)
	require.Equal(t, "bot:read", v.Scope)
	m.ExpectExec("UPDATE bot_installations SET revoked_at=COALESCE").WithArgs("fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, r.Revoke(context.Background(), "fixture"))
	require.NoError(t, m.ExpectationsWereMet())
}
func TestBotUsageReadHasNoCredentialOrPayloadProjection(t *testing.T) {
	db, m, e := sqlmock.New()
	require.NoError(t, e)
	defer db.Close()
	r := NewBotRepository(db)
	m.ExpectBegin()
	m.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery(`SELECT jsonb_build_object\('id',u.id,.*u.user_id=ANY\(\$1::bigint\[\]\).*u.group_id=ANY\(\$2::bigint\[\]\).*LIMIT \$5 OFFSET \$6`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 51, 0).WillReturnRows(sqlmock.NewRows([]string{"item"}).AddRow(`{"id":1,"upstream_model_mismatch":null}`))
	m.ExpectCommit()
	result, e := r.Query(context.Background(), "model-audit", &service.BotInstallation{BotPolicy: service.BotPolicy{UserIDs: []int64{9}, GroupIDs: []int64{7}}}, service.BotQuery{Page: 1, PageSize: 50, Start: time.Now().Add(-time.Hour), End: time.Now(), Values: map[string][]string{}})
	require.NoError(t, e)
	require.Contains(t, string(result), `"history_complete":false`)
	require.NotContains(t, string(result), "credentials")
	require.NoError(t, m.ExpectationsWereMet())
}
