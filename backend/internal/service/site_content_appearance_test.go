package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

type appearanceSettings struct {
	SettingRepository
	raw string
}

func (s *appearanceSettings) GetValue(context.Context, string) (string, error) {
	if s.raw == "" {
		return "", ErrSettingNotFound
	}
	return s.raw, nil
}

type appearanceRepository struct {
	SiteContentRepository
	settings *appearanceSettings
	version  int
}

func (r *appearanceRepository) CompareAndSwapAppearance(_ context.Context, expected int, raw string) error {
	if expected != r.version {
		return ErrContentConflict
	}
	r.version++
	r.settings.raw = raw
	return nil
}

func TestSiteAppearanceDefaultsAndPartialJSON(t *testing.T) {
	for _, raw := range []string{`{"home_mode":"immersive"}`, `{"console":null}`, `{"console":{"blur":0,"flow_enabled":false}}`} {
		s := NewSiteContentService(nil, &appearanceSettings{raw: raw})
		a, err := s.Appearance(context.Background())
		if err != nil || a.Console == nil || !a.Console.Enabled || a.Console.SurfaceOpacity != .72 {
			t.Fatalf("defaults: %+v %v", a.Console, err)
		}
		if raw == `{"console":{"blur":0,"flow_enabled":false}}` && (a.Console.Blur != 0 || a.Console.FlowEnabled) {
			t.Fatal("explicit zero/false lost")
		}
	}
}

func TestSiteAppearanceConsoleBounds(t *testing.T) {
	cases := []func(*SiteConsoleAppearance){
		func(a *SiteConsoleAppearance) { a.SurfaceOpacity = .54 }, func(a *SiteConsoleAppearance) { a.Blur = 25 },
		func(a *SiteConsoleAppearance) { a.FocusX = -1 }, func(a *SiteConsoleAppearance) { a.BackgroundDim = .66 },
		func(a *SiteConsoleAppearance) { a.Blur = math.NaN() }, func(a *SiteConsoleAppearance) { a.FocusY = math.Inf(1) },
		func(a *SiteConsoleAppearance) { a.WallpaperID = "scene-7" }, func(a *SiteConsoleAppearance) { a.SurfaceMode = "unknown" },
		func(a *SiteConsoleAppearance) { a.BackgroundMode = "custom" }, func(a *SiteConsoleAppearance) { a.BackgroundURL = "//evil.example/x" },
		func(a *SiteConsoleAppearance) { a.BackgroundURL = "https://user:pass@example.com/x" }, func(a *SiteConsoleAppearance) { a.BackgroundURL = "javascript:alert(1)" },
	}
	for i, change := range cases {
		a := DefaultSiteAppearance()
		change(a.Console)
		if err := (&SiteContentService{}).SaveAppearance(context.Background(), &a); err != ErrContentInvalid {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestSiteAppearanceSaveRoundTripAndConflict(t *testing.T) {
	settings := &appearanceSettings{}
	repo := &appearanceRepository{settings: settings}
	s := NewSiteContentService(repo, settings)
	a := DefaultSiteAppearance()
	a.Console.BackgroundMode = "custom"
	a.Console.BackgroundURL = "https://example.com/scene.webp"
	a.Console.SurfaceOpacity = .62
	if err := s.SaveAppearance(context.Background(), &a); err != nil {
		t.Fatal(err)
	}
	read, err := s.Appearance(context.Background())
	if err != nil || read.Console.SurfaceOpacity != .62 || read.Version != 1 {
		t.Fatalf("roundtrip: %+v %v", read, err)
	}
	stale := DefaultSiteAppearance()
	if err := s.SaveAppearance(context.Background(), &stale); err != ErrContentConflict || stale.Version != 0 {
		t.Fatalf("conflict: %v version=%d", err, stale.Version)
	}
	legacy := read
	legacy.Console = nil
	if err := s.SaveAppearance(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	var persisted SiteAppearance
	if err := json.Unmarshal([]byte(settings.raw), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Console.BackgroundURL != "https://example.com/scene.webp" || persisted.Version != 2 {
		t.Fatal("legacy client reset console")
	}
}
