package service

import "testing"

func TestSiteContentURLs(t *testing.T) {
	for _, v := range []string{"javascript:alert(1)", "//evil.example/x", "http://example.com", "https://u:p@example.com", "/\\evil.example/x", "data:image/svg+xml,x"} {
		if ValidSiteURL(v, true) {
			t.Fatalf("unsafe URL accepted: %s", v)
		}
	}
	for _, v := range []string{"/experience/image.webp", "https://example.com/logo.webp"} {
		if !ValidSiteURL(v, true) {
			t.Fatalf("safe URL rejected: %s", v)
		}
	}
}
func TestSiteContentDefaultAppearance(t *testing.T) {
	a := DefaultSiteAppearance()
	if a.HomeMode != "immersive" || len(a.Wallpapers) != 6 || a.Interval != 5 {
		t.Fatal(a)
	}
}
