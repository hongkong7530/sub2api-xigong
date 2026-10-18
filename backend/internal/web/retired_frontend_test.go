//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredFrontendPaths(t *testing.T) {
	for _, path := range []string{"/admin/devin", "/admin/devin/", "/admin/devin/requests", "/studio", "/studio/", "/studio/settings", "/image-studio", "/image-studio/", "/image-studio/assets/index.js"} {
		assert.True(t, isRetiredFrontendPath(path), path)
	}
	for _, path := range []string{"/", "/dashboard", "/batch-image", "/v1/images/generations", "/images/generations", "/image-studio-backup", "/studio-other"} {
		assert.False(t, isRetiredFrontendPath(path), path)
	}
}

func TestRetiredFrontendServing(t *testing.T) {
	server, err := NewFrontendServer(&mockSettingsProvider{settings: map[string]string{"site_name": "Test"}})
	require.NoError(t, err)
	require.False(t, server.fileExists("image-studio/index.html"), "retired resources must not remain embedded")
	// Even an old resource tree must not revive the app through static serving.
	server.distFS = fstest.MapFS{"image-studio/index.html": &fstest.MapFile{Data: []byte("retired app")}}
	for name, middleware := range map[string]gin.HandlerFunc{"settings": server.Middleware(), "legacy": ServeEmbeddedFrontend()} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware)
			for _, path := range []string{"/studio", "/studio/", "/image-studio", "/image-studio/", "/image-studio/index.html", "/image-studio/assets/missing.js"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				assert.Equal(t, http.StatusNotFound, w.Code, path)
				assert.Empty(t, w.Body.String(), path)
			}
		})
	}
}

func TestImagesAPIsBypassEmbeddedFrontend(t *testing.T) {
	server, err := NewFrontendServer(&mockSettingsProvider{settings: map[string]string{"site_name": "Test"}})
	require.NoError(t, err)
	for name, middleware := range map[string]gin.HandlerFunc{"settings": server.Middleware(), "legacy": ServeEmbeddedFrontend()} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware)
			paths := []string{"/v1/images/generations", "/v1/images/edits", "/images/generations"}
			for _, path := range paths {
				router.POST(path, func(c *gin.Context) { c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"}) })
			}
			for _, path := range paths {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
				assert.Equal(t, http.StatusUnauthorized, w.Code, path)
				assert.Contains(t, w.Header().Get("Content-Type"), "application/json", path)
			}
		})
	}
}
