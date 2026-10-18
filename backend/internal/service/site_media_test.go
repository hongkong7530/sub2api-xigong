package service

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type mediaTestRepo struct {
	SiteContentRepository
	body  []byte
	mime  string
	calls int
}

func (r *mediaTestRepo) SaveSiteAsset(_ context.Context, _ string, mime string, b []byte) error {
	r.body = append([]byte{}, b...)
	r.mime = mime
	r.calls++
	return nil
}
func TestSiteMediaReencodesAndRejectsActivePayloads(t *testing.T) {
	repo := &mediaTestRepo{}
	s := NewSiteContentService(repo, nil)
	for _, data := range [][]byte{[]byte(`<svg onload="alert(1)"/>`), []byte(`<html>hi</html>`), make([]byte, SiteAssetMaxBytes+1)} {
		_, err := s.SaveAsset(context.Background(), data)
		require.ErrorIs(t, err, ErrContentInvalid)
	}
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 20, 10))))
	url, err := s.SaveAsset(context.Background(), b.Bytes())
	require.NoError(t, err)
	require.Regexp(t, `^/api/v1/site-content/assets/[0-9a-f]{64}$`, url)
	require.Equal(t, "image/jpeg", repo.mime)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(repo.body))
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 20, cfg.Width)
	require.Equal(t, 1, repo.calls)
}
func TestSiteMetadataRejectsUnsafeURLTargets(t *testing.T) {
	for _, url := range []string{"http://example.com", "https://localhost/", "https://127.0.0.1/", "https://169.254.169.254/", "https://[::1]/", "https://224.0.0.1/", "https://user:pass@example.com/", "https://example.com:8443/", "https://example.com\\@127.0.0.1/"} {
		_, err := sitePublicURL(url)
		require.Error(t, err, url)
	}
	_, err := sitePublicURL("https://example.com/path?q=1#title")
	require.NoError(t, err)
	err = siteHTTPClient().CheckRedirect(nil, make([]*http.Request, 4))
	require.Error(t, err)
}
func TestSiteWallpaperDeviceFieldsPreserveAndClear(t *testing.T) {
	settings := &appearanceSettings{}
	repo := &appearanceRepository{settings: settings}
	s := NewSiteContentService(repo, settings)
	a := DefaultSiteAppearance()
	desktop, mobile := "https://example.com/wide.jpg", "https://example.com/tall.jpg"
	x := 0.0
	a.Wallpapers[0].DesktopURL = &desktop
	a.Wallpapers[0].MobileURL = &mobile
	a.Wallpapers[0].MobileFocusX = &x
	require.NoError(t, s.SaveAppearance(context.Background(), &a))
	legacy := DefaultSiteAppearance()
	legacy.Version = a.Version
	require.NoError(t, s.SaveAppearance(context.Background(), &legacy))
	require.Equal(t, mobile, *legacy.Wallpapers[0].MobileURL)
	require.Zero(t, *legacy.Wallpapers[0].MobileFocusX)
	empty := ""
	legacy.Wallpapers[0].MobileURL = &empty
	require.NoError(t, s.SaveAppearance(context.Background(), &legacy))
	got, err := s.Appearance(context.Background())
	require.NoError(t, err)
	require.Equal(t, "", *got.Wallpapers[0].MobileURL)
}
