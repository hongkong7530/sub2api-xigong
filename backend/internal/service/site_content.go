package service

import (
	"context"
	"encoding/json"
	"errors"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrContentMissing  = infraerrors.NotFound("CONTENT_NOT_FOUND", "Content not found")
	ErrContentConflict = infraerrors.Conflict("CONTENT_CONFLICT", "Content changed; reload before saving")
	ErrContentInvalid  = infraerrors.BadRequest("CONTENT_INVALID", "Invalid content or URL")
)

const SiteAppearanceKey = "site_experience_v1"

type DocumentBody struct {
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Content    string `json:"content"`
	CategoryID int64  `json:"category_id"`
	Position   int    `json:"position"`
}
type SiteDocument struct {
	ID          int64         `json:"id"`
	Slug        string        `json:"slug"`
	Draft       DocumentBody  `json:"draft"`
	Published   *DocumentBody `json:"published"`
	Version     int           `json:"version"`
	UpdatedAt   time.Time     `json:"updated_at"`
	PublishedAt *time.Time    `json:"published_at"`
}
type SiteCategory struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Version  int    `json:"version"`
}
type SitePartner struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`
	Thumbnail   string `json:"thumbnail"`
	Tag         string `json:"tag"`
	Position    int    `json:"position"`
	Enabled     bool   `json:"enabled"`
	Version     int    `json:"version"`
}
type SiteWallpaper struct {
	ID           string   `json:"id"`
	FocusX       float64  `json:"focus_x"`
	FocusY       float64  `json:"focus_y"`
	DesktopURL   *string  `json:"desktop_url,omitempty"`
	MobileURL    *string  `json:"mobile_url,omitempty"`
	MobileFocusX *float64 `json:"mobile_focus_x,omitempty"`
	MobileFocusY *float64 `json:"mobile_focus_y,omitempty"`
}
type SiteQuote struct {
	EN string `json:"en"`
	ZH string `json:"zh"`
}
type SiteAppearance struct {
	Version         int                    `json:"version"`
	HomeMode        string                 `json:"home_mode"`
	DocsEnabled     bool                   `json:"docs_enabled"`
	PartnersEnabled bool                   `json:"partners_enabled"`
	MotionEnabled   bool                   `json:"motion_enabled"`
	Intensity       float64                `json:"intensity"`
	Interval        int                    `json:"interval"`
	Phrases         []string               `json:"phrases"`
	Quotes          []SiteQuote            `json:"quotes"`
	Wallpapers      []SiteWallpaper        `json:"wallpapers"`
	Console         *SiteConsoleAppearance `json:"console"`
}

func DefaultSiteAppearance() SiteAppearance {
	return SiteAppearance{Console: DefaultSiteConsoleAppearance(), HomeMode: "immersive", DocsEnabled: true, PartnersEnabled: true, MotionEnabled: true, Intensity: 1, Interval: 5,
		Phrases:    []string{"xigong.cloud", "西宫我们爱你", "我最喜欢的中转站", "好像和西宫在一起"},
		Quotes:     []SiteQuote{{EN: "Stay curious.", ZH: "别被默认值定义。"}, {EN: "Make room for the unexpected.", ZH: "不必循规，灵感自有去处。"}},
		Wallpapers: []SiteWallpaper{{ID: "scene-1", FocusX: 70, FocusY: 45}, {ID: "scene-2", FocusX: 25, FocusY: 50}, {ID: "scene-3", FocusX: 45, FocusY: 45}, {ID: "scene-4", FocusX: 50, FocusY: 28}, {ID: "scene-5", FocusX: 25, FocusY: 45}, {ID: "scene-6", FocusX: 50, FocusY: 40}}}
}

type SiteContentRepository interface {
	Documents(context.Context, bool, string, int) ([]SiteDocument, bool, error)
	CompareAndSwapAppearance(context.Context, int, string) error
	Document(context.Context, int64) (*SiteDocument, error)
	PublishedDocument(context.Context, string) (*SiteDocument, error)
	SaveDocument(context.Context, *SiteDocument) error
	PublishDocument(context.Context, int64, int, bool) error
	DeleteDocument(context.Context, int64, int) error
	Categories(context.Context) ([]SiteCategory, error)
	SaveCategory(context.Context, *SiteCategory) error
	DeleteCategory(context.Context, int64, int) error
	Partners(context.Context, bool) ([]SitePartner, error)
	SavePartner(context.Context, *SitePartner) error
	DeletePartner(context.Context, int64, int) error
	SaveSiteAsset(context.Context, string, string, []byte) error
	SiteAsset(context.Context, string) (string, []byte, error)
	PublicSiteUsage(context.Context, time.Time, time.Time) (*SiteUsageSnapshot, error)
}
type SiteContentService struct {
	Repo     SiteContentRepository
	settings SettingRepository
}

func NewSiteContentService(repo SiteContentRepository, settings SettingRepository) *SiteContentService {
	return &SiteContentService{repo, settings}
}
func (s *SiteContentService) Appearance(ctx context.Context) (SiteAppearance, error) {
	a := DefaultSiteAppearance()
	raw, err := s.settings.GetValue(ctx, SiteAppearanceKey)
	if errors.Is(err, ErrSettingNotFound) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	err = json.Unmarshal([]byte(raw), &a)
	if a.Console == nil {
		a.Console = DefaultSiteConsoleAppearance()
	}
	return a, err
}
func (s *SiteContentService) SaveAppearance(ctx context.Context, a *SiteAppearance) error {
	if a.Version < 0 || a.Version >= 2147483647 || (a.HomeMode != "legacy" && a.HomeMode != "immersive") {
		return ErrContentInvalid
	}
	if a.Interval < 3 || a.Interval > 30 || a.Intensity < 0 || a.Intensity > 2 || len(a.Phrases) > 20 || len(a.Quotes) > 20 || len(a.Wallpapers) < 1 || len(a.Wallpapers) > 6 {
		return ErrContentInvalid
	}
	for _, p := range a.Phrases {
		if strings.TrimSpace(p) == "" || utf8.RuneCountInString(p) > 80 {
			return ErrContentInvalid
		}
	}
	for _, q := range a.Quotes {
		if len(q.EN) > 300 || len(q.ZH) > 300 {
			return ErrContentInvalid
		}
	}
	// 先拒绝非法输入；兼容合并仅在有效草稿上读取已有设置。
	if a.Console != nil && !a.Console.Valid() {
		return ErrContentInvalid
	}
	for _, w := range a.Wallpapers {
		if !regexp.MustCompile(`^scene-[1-6]$`).MatchString(w.ID) || !validWallpaper(&w) || w.FocusX < 0 || w.FocusX > 100 || w.FocusY < 0 || w.FocusY > 100 {
			return ErrContentInvalid
		}
	}
	previous, err := s.Appearance(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i := range a.Wallpapers {
		w := &a.Wallpapers[i]
		for _, old := range previous.Wallpapers {
			if old.ID == w.ID {
				if w.DesktopURL == nil {
					w.DesktopURL = old.DesktopURL
				}
				if w.MobileURL == nil {
					w.MobileURL = old.MobileURL
				}
				if w.MobileFocusX == nil {
					w.MobileFocusX = old.MobileFocusX
				}
				if w.MobileFocusY == nil {
					w.MobileFocusY = old.MobileFocusY
				}
				break
			}
		}
		if !validWallpaper(w) {
			return ErrContentInvalid
		}
		if !regexp.MustCompile(`^scene-[1-6]$`).MatchString(w.ID) || seen[w.ID] || w.FocusX < 0 || w.FocusX > 100 || w.FocusY < 0 || w.FocusY > 100 {
			return ErrContentInvalid
		}
		seen[w.ID] = true
	}
	// 旧客户端不认识控制台字段，省略它不意味着重置管理员已保存的外观。
	if a.Console == nil {
		previous, err := s.Appearance(ctx)
		if err != nil {
			return err
		}
		a.Console = previous.Console
	}
	if a.Console.MobileBackgroundURL == nil {
		a.Console.MobileBackgroundURL = previous.Console.MobileBackgroundURL
	}
	if a.Console.MobileFocusX == nil {
		a.Console.MobileFocusX = previous.Console.MobileFocusX
	}
	if a.Console.MobileFocusY == nil {
		a.Console.MobileFocusY = previous.Console.MobileFocusY
	}
	if !a.Console.Valid() {
		return ErrContentInvalid
	}
	expected := a.Version
	next := *a
	next.Version++
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := s.Repo.CompareAndSwapAppearance(ctx, expected, string(b)); err != nil {
		return err
	}
	a.Version = next.Version
	return nil
}
func ValidSiteURL(raw string, relative bool) bool {
	if len(raw) > 2048 || strings.ContainsAny(raw, "\\\r\n\t") {
		return false
	}
	if relative && strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		u, e := url.Parse(raw)
		return e == nil && u.Host == "" && u.Scheme == ""
	}
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
func (s *SiteContentService) SaveDocument(ctx context.Context, d *SiteDocument) error {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,95}$`).MatchString(d.Slug) || strings.TrimSpace(d.Draft.Title) == "" || utf8.RuneCountInString(d.Draft.Title) > 160 || len(d.Draft.Summary) > 2000 || strings.TrimSpace(d.Draft.Content) == "" || len(d.Draft.Content) > 200000 || d.Draft.CategoryID < 0 || (d.Draft.Position < 0 || d.Draft.Position > 2147483647) {
		return ErrContentInvalid
	}
	return s.Repo.SaveDocument(ctx, d)
}
func (s *SiteContentService) SaveCategory(ctx context.Context, c *SiteCategory) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || utf8.RuneCountInString(c.Name) > 80 || (c.Position < 0 || c.Position > 2147483647) {
		return ErrContentInvalid
	}
	return s.Repo.SaveCategory(ctx, c)
}
func (s *SiteContentService) SavePartner(ctx context.Context, p *SitePartner) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 100 || len(p.Description) > 2000 || len(p.Tag) > 120 || (p.Position < 0 || p.Position > 2147483647) || !ValidSiteURL(p.URL, false) || (p.Icon != "" && !ValidSiteURL(p.Icon, true)) || (p.Thumbnail != "" && !ValidSiteURL(p.Thumbnail, true)) {
		return ErrContentInvalid
	}
	return s.Repo.SavePartner(ctx, p)
}
