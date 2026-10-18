package service

import (
	"encoding/json"
	"math"
	"regexp"
)

// SiteConsoleAppearance 独立于首页轮播；业务文字不参与光学形变。
type SiteConsoleAppearance struct {
	Enabled             bool     `json:"enabled"`
	BackgroundMode      string   `json:"background_mode"`
	WallpaperID         string   `json:"wallpaper_id"`
	BackgroundURL       string   `json:"background_url"`
	MobileBackgroundURL *string  `json:"mobile_background_url,omitempty"`
	MobileFocusX        *float64 `json:"mobile_focus_x,omitempty"`
	MobileFocusY        *float64 `json:"mobile_focus_y,omitempty"`
	FocusX              float64  `json:"focus_x"`
	FocusY              float64  `json:"focus_y"`
	SurfaceMode         string   `json:"surface_mode"`
	SurfaceOpacity      float64  `json:"surface_opacity"`
	Blur                float64  `json:"blur"`
	BackgroundDim       float64  `json:"background_dim"`
	PointerReveal       bool     `json:"pointer_reveal"`
	ColorfulHeadings    bool     `json:"colorful_headings"`
	FlowEnabled         bool     `json:"flow_enabled"`
}

func DefaultSiteConsoleAppearance() *SiteConsoleAppearance {
	return &SiteConsoleAppearance{
		Enabled: true, BackgroundMode: "wallpaper", WallpaperID: "scene-1",
		FocusX: 70, FocusY: 45, SurfaceMode: "liquid", SurfaceOpacity: .72,
		Blur: 12, BackgroundDim: .18, PointerReveal: true, ColorfulHeadings: true, FlowEnabled: true,
	}
}

// 缺少字段的旧 JSON 补默认值，但显式 false 和 0 必须保留。
func (a *SiteConsoleAppearance) UnmarshalJSON(data []byte) error {
	type plain SiteConsoleAppearance
	value := plain(*DefaultSiteConsoleAppearance())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = SiteConsoleAppearance(value)
	return nil
}

var consoleWallpaperID = regexp.MustCompile(`^scene-[1-6]$`)

func (a *SiteConsoleAppearance) Valid() bool {
	between := func(value, min, max float64) bool {
		return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= min && value <= max
	}
	return a != nil &&
		(a.BackgroundMode == "wallpaper" || a.BackgroundMode == "custom" || a.BackgroundMode == "none") &&
		(a.SurfaceMode == "liquid" || a.SurfaceMode == "frosted") &&
		consoleWallpaperID.MatchString(a.WallpaperID) &&
		(a.BackgroundURL == "" || ValidSiteURL(a.BackgroundURL, true)) &&
		(a.MobileBackgroundURL == nil || *a.MobileBackgroundURL == "" || ValidSiteURL(*a.MobileBackgroundURL, true)) &&
		(a.MobileFocusX == nil || between(*a.MobileFocusX, 0, 100)) && (a.MobileFocusY == nil || between(*a.MobileFocusY, 0, 100)) &&
		(a.BackgroundMode != "custom" || a.BackgroundURL != "") &&
		between(a.FocusX, 0, 100) && between(a.FocusY, 0, 100) &&
		between(a.SurfaceOpacity, .55, .95) && between(a.Blur, 0, 24) && between(a.BackgroundDim, 0, .65)
}
