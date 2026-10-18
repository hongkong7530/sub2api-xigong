package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSiteContentRejectsOversizedPositions(t *testing.T) {
	s := &SiteContentService{}
	require.ErrorIs(t, s.SaveDocument(context.Background(), &SiteDocument{Slug: "guide", Draft: DocumentBody{Title: "Guide", Content: "body", Position: 2147483648}}), ErrContentInvalid)
	require.ErrorIs(t, s.SaveCategory(context.Background(), &SiteCategory{Name: "Guide", Position: 2147483648}), ErrContentInvalid)
	require.ErrorIs(t, s.SavePartner(context.Background(), &SitePartner{Name: "Site", URL: "https://example.com", Position: 2147483648}), ErrContentInvalid)
}
func TestSiteContentRejectsAppearanceVersionOverflow(t *testing.T) {
	a := DefaultSiteAppearance()
	a.Version = 2147483647
	require.ErrorIs(t, (&SiteContentService{}).SaveAppearance(context.Background(), &a), ErrContentInvalid)
}
