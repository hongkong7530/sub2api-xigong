package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/cluster"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strconv"
	"strings"
)

type SiteContentHandler struct {
	svc        *service.SiteContentService
	redis      *redis.Client
	runtime    *cluster.Runtime
	settings   *service.SettingService
	plaza      *service.ModelPlazaService
	timezone   string
	mediaSlots chan struct{}
}

func NewSiteContentHandler(svc *service.SiteContentService, rdb *redis.Client, runtime *cluster.Runtime, settings *service.SettingService, cfg *config.Config, plaza *service.ModelPlazaService) *SiteContentHandler {
	return &SiteContentHandler{svc: svc, redis: rdb, runtime: runtime, settings: settings, plaza: plaza, timezone: cfg.Timezone, mediaSlots: make(chan struct{}, 2)}
}
func contentReply(c *gin.Context, data any, err error) {
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}
func contentBind(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := c.ShouldBindJSON(v); err != nil {
		response.BadRequest(c, "Invalid content payload")
		return false
	}
	return true
}
func contentID(c *gin.Context) int64 {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		response.BadRequest(c, "Invalid content ID")
		return 0
	}
	return id
}
func (h *SiteContentHandler) Appearance(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	a, e := h.svc.Appearance(c.Request.Context())
	contentReply(c, a, e)
}
func (h *SiteContentHandler) SaveAppearance(c *gin.Context) {
	var a service.SiteAppearance
	if !contentBind(c, &a) {
		return
	}
	err := h.svc.SaveAppearance(c.Request.Context(), &a)
	contentReply(c, a, err)
}
func (h *SiteContentHandler) AdminDocuments(c *gin.Context) {
	page, ok := contentPage(c)
	if !ok {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 300 {
		response.BadRequest(c, "Search query too long")
		return
	}
	items, more, e := h.svc.Repo.Documents(c.Request.Context(), false, q, page)
	contentReply(c, gin.H{"documents": items, "has_more": more, "page": page}, e)
}
func (h *SiteContentHandler) SaveDocument(c *gin.Context) {
	var input struct {
		Slug    string               `json:"slug"`
		Draft   service.DocumentBody `json:"draft"`
		Version int                  `json:"version"`
	}
	if !contentBind(c, &input) {
		return
	}
	d := service.SiteDocument{Slug: input.Slug, Draft: input.Draft, Version: input.Version}
	if c.Param("id") != "" {
		d.ID = contentID(c)
		if d.ID == 0 {
			return
		}
	}
	if e := h.svc.SaveDocument(c.Request.Context(), &d); e != nil {
		contentReply(c, nil, e)
		return
	}
	saved, e := h.svc.Repo.Document(c.Request.Context(), d.ID)
	contentReply(c, saved, e)
}
func (h *SiteContentHandler) DocumentAction(c *gin.Context) {
	id := contentID(c)
	if id == 0 {
		return
	}
	var req struct {
		Version int `json:"version"`
	}
	if !contentBind(c, &req) {
		return
	}
	var e error
	switch c.Param("action") {
	case "publish":
		e = h.svc.Repo.PublishDocument(c.Request.Context(), id, req.Version, true)
	case "unpublish":
		e = h.svc.Repo.PublishDocument(c.Request.Context(), id, req.Version, false)
	case "delete":
		e = h.svc.Repo.DeleteDocument(c.Request.Context(), id, req.Version)
	default:
		response.NotFound(c, "Unknown action")
		return
	}
	contentReply(c, gin.H{"ok": true}, e)
}
func (h *SiteContentHandler) Categories(c *gin.Context) {
	items, e := h.svc.Repo.Categories(c.Request.Context())
	contentReply(c, items, e)
}
func (h *SiteContentHandler) SaveCategory(c *gin.Context) {
	var item service.SiteCategory
	if !contentBind(c, &item) {
		return
	}
	item.ID = 0
	if c.Param("id") != "" {
		item.ID = contentID(c)
		if item.ID == 0 {
			return
		}
	}
	e := h.svc.SaveCategory(c.Request.Context(), &item)
	contentReply(c, item, e)
}
func (h *SiteContentHandler) DeleteCategory(c *gin.Context) {
	id := contentID(c)
	if id == 0 {
		return
	}
	var req struct {
		Version int `json:"version"`
	}
	if !contentBind(c, &req) {
		return
	}
	contentReply(c, gin.H{"ok": true}, h.svc.Repo.DeleteCategory(c.Request.Context(), id, req.Version))
}
func (h *SiteContentHandler) AdminPartners(c *gin.Context) {
	items, e := h.svc.Repo.Partners(c.Request.Context(), false)
	contentReply(c, items, e)
}
func (h *SiteContentHandler) SavePartner(c *gin.Context) {
	var item service.SitePartner
	if !contentBind(c, &item) {
		return
	}
	item.ID = 0
	if c.Param("id") != "" {
		item.ID = contentID(c)
		if item.ID == 0 {
			return
		}
	}
	e := h.svc.SavePartner(c.Request.Context(), &item)
	contentReply(c, item, e)
}
func (h *SiteContentHandler) DeletePartner(c *gin.Context) {
	id := contentID(c)
	if id == 0 {
		return
	}
	var req struct {
		Version int `json:"version"`
	}
	if !contentBind(c, &req) {
		return
	}
	contentReply(c, gin.H{"ok": true}, h.svc.Repo.DeletePartner(c.Request.Context(), id, req.Version))
}
func (h *SiteContentHandler) PublicPartners(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	a, e := h.svc.Appearance(c.Request.Context())
	if e != nil {
		contentReply(c, nil, e)
		return
	}
	if !a.PartnersEnabled {
		response.NotFound(c, "Partners disabled")
		return
	}
	items, e := h.svc.Repo.Partners(c.Request.Context(), true)
	if e != nil {
		contentReply(c, nil, e)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, p := range items {
		out = append(out, gin.H{"id": p.ID, "name": p.Name, "description": p.Description, "url": p.URL, "icon": p.Icon, "thumbnail": p.Thumbnail, "tag": p.Tag, "position": p.Position})
	}
	contentReply(c, out, nil)
}
func (h *SiteContentHandler) PublicDocuments(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	a, e := h.svc.Appearance(c.Request.Context())
	if e != nil {
		contentReply(c, nil, e)
		return
	}
	if !a.DocsEnabled {
		response.NotFound(c, "Documentation disabled")
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 300 {
		response.BadRequest(c, "Search query too long")
		return
	}
	page, ok := contentPage(c)
	if !ok {
		return
	}
	items, more, e := h.svc.Repo.Documents(c.Request.Context(), true, q, page)
	if e != nil {
		contentReply(c, nil, e)
		return
	}
	out := make([]gin.H, 0, len(items))
	used := map[int64]bool{}
	for _, d := range items {
		if d.Published == nil {
			continue
		}
		b := *d.Published
		used[b.CategoryID] = true
		out = append(out, gin.H{"id": d.ID, "slug": d.Slug, "title": b.Title, "summary": b.Summary, "category_id": b.CategoryID, "position": b.Position, "updated_at": d.PublishedAt})
	}
	categories, e := h.svc.Repo.Categories(c.Request.Context())
	if e != nil {
		contentReply(c, nil, e)
		return
	}
	publicCategories := make([]gin.H, 0)
	for _, cat := range categories {
		if used[cat.ID] {
			publicCategories = append(publicCategories, gin.H{"id": cat.ID, "name": cat.Name, "position": cat.Position})
		}
	}
	response.Success(c, gin.H{"documents": out, "categories": publicCategories, "has_more": more, "page": page})
}
func (h *SiteContentHandler) PublicDocument(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	a, e := h.svc.Appearance(c.Request.Context())
	if e != nil {
		contentReply(c, nil, e)
		return
	}
	if !a.DocsEnabled {
		response.NotFound(c, "Documentation disabled")
		return
	}
	d, e := h.svc.Repo.PublishedDocument(c.Request.Context(), c.Param("slug"))
	if e != nil {
		contentReply(c, nil, e)
		return
	}
	b := d.Published
	response.Success(c, gin.H{"id": d.ID, "slug": d.Slug, "title": b.Title, "summary": b.Summary, "content": b.Content, "category_id": b.CategoryID, "position": b.Position, "updated_at": d.PublishedAt})
}

func contentPage(c *gin.Context) (int, bool) {
	page, e := strconv.Atoi(c.DefaultQuery("page", "1"))
	if e != nil || page < 1 || page > 100000 {
		response.BadRequest(c, "Invalid page")
		return 0, false
	}
	return page, true
}
func (h *SiteContentHandler) AdminDocument(c *gin.Context) {
	id := contentID(c)
	if id == 0 {
		return
	}
	d, e := h.svc.Repo.Document(c.Request.Context(), id)
	contentReply(c, d, e)
}
