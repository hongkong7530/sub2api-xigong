package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type profitHandlerRepo struct {
	user   int64
	actor  int64
	group  int64
	input  service.ProfitCostInput
	writes int
	err    error
}

func (r *profitHandlerRepo) Snapshot(context.Context, bool) (*service.ProfitSnapshot, error) {
	return &service.ProfitSnapshot{Scope: "public_groups", Groups: []service.ProfitGroup{}}, r.err
}
func (r *profitHandlerRepo) Contribution(_ context.Context, id int64) (*service.ProfitContribution, error) {
	r.user = id
	return &service.ProfitContribution{}, r.err
}
func (r *profitHandlerRepo) SetCost(_ context.Context, group, actor int64, input service.ProfitCostInput) (*service.ProfitCostVersion, error) {
	r.group = group
	r.actor = actor
	r.input = input
	r.writes++
	return &service.ProfitCostVersion{Version: input.Version + 1, Multiplier: input.Multiplier}, r.err
}
func (r *profitHandlerRepo) Entries(context.Context, service.ProfitEntryFilter) (*service.ProfitEntryPage, error) {
	return &service.ProfitEntryPage{Items: []service.ProfitEntry{}}, r.err
}
func (r *profitHandlerRepo) Aggregate(context.Context, int) (int, error) { return 0, nil }
func profitTestRouter(repo *profitHandlerRepo, user int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if user > 0 {
		r.Use(func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: user})
			c.Next()
		})
	}
	h := NewSiteProfitHandler(service.NewSiteProfitService(repo))
	r.GET("/me", h.Contribution)
	r.PUT("/groups/:id/cost", h.SetCost)
	r.GET("/disclosure", h.Disclosure)
	r.GET("/entries", h.Entries)
	return r
}
func TestSiteProfitPersonalUsesOnlyAuthenticatedIdentity(t *testing.T) {
	repo := &profitHandlerRepo{}
	r := profitTestRouter(repo, 7)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/me?user_id=999", nil))
	require.Equal(t, 200, w.Code)
	require.EqualValues(t, 7, repo.user)
	w = httptest.NewRecorder()
	profitTestRouter(repo, 0).ServeHTTP(w, httptest.NewRequest("GET", "/me", nil))
	require.Equal(t, 401, w.Code)
}
func TestSiteProfitCostInputAndConflict(t *testing.T) {
	repo := &profitHandlerRepo{}
	r := profitTestRouter(repo, 7)
	for _, body := range []string{`{"version":0}`, `{"multiplier":"0.2"}`, `{"multiplier":0.2,"version":0}`, `{"multiplier":"-1","version":0}`, `{"multiplier":"1e-3","version":0}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/groups/2/cost", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, 400, w.Code, body)
	}
	require.Zero(t, repo.writes)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/groups/2/cost", strings.NewReader(`{"multiplier":"0","version":0}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.EqualValues(t, 7, repo.actor)
	require.Equal(t, "0.00000000", *repo.input.Multiplier)
	repo.err = service.ErrProfitConflict
	w = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", "/groups/2/cost", strings.NewReader(`{"multiplier":null,"version":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 409, w.Code)
}
func TestSiteProfitConditionalDisclosureAndFilterValidation(t *testing.T) {
	r := profitTestRouter(&profitHandlerRepo{}, 7)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/disclosure", nil))
	require.Equal(t, 200, w.Code)
	require.NotEmpty(t, w.Header().Get("ETag"))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	req := httptest.NewRequest("GET", "/disclosure", nil)
	req.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotModified, w.Code)
	for _, query := range []string{"?page=0", "?group_id=-1", "?from=no", "?from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z"} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/entries"+query, nil))
		require.Equal(t, 400, w.Code)
	}
}
