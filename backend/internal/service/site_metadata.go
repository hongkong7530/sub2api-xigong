package service

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// 网页、跳转和二级图片均通过同一公网拨号策略；不沿用进程代理或用户凭据。
func sitePublicURL(raw string) (*url.URL, error) {
	if !ValidSiteURL(raw, false) {
		return nil, ErrContentInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Port() != "" && u.Port() != "443" || isBlockedHostname(u.Hostname()) {
		return nil, ErrContentInvalid
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (!ip.IsGlobalUnicast() || isPrivateIP(ip)) {
		return nil, ErrContentInvalid
	}
	u.Fragment = ""
	return u, nil
}
func siteDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" || isBlockedHostname(host) {
		return nil, ErrContentInvalid
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, ErrContentInvalid
	}
	for _, ip := range ips {
		if !ip.IP.IsGlobalUnicast() || isPrivateIP(ip.IP) {
			return nil, ErrContentInvalid
		}
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	for _, ip := range ips {
		conn, e := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if e == nil {
			return conn, nil
		}
		err = e
	}
	return nil, err
}
func siteHTTPClient() *http.Client {
	return &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{DialContext: siteDial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxIdleConns: 4}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return ErrContentInvalid
		}
		_, err := sitePublicURL(req.URL.String())
		return err
	}}
}
func siteDownload(ctx context.Context, client *http.Client, raw string, limit int64) ([]byte, *url.URL, error) {
	u, err := sitePublicURL(raw)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "Xigong-SitePreview/1.0")
	req.Header.Set("Accept", "text/html,image/png,image/jpeg;q=0.9,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, errors.New("site preview unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, nil, ErrContentInvalid
	}
	return data, resp.Request.URL, nil
}

type SitePartnerPreview struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	URL             string   `json:"url"`
	Icon            string   `json:"icon"`
	Thumbnail       string   `json:"thumbnail"`
	ThumbnailSource string   `json:"thumbnail_source"`
	Warnings        []string `json:"warnings"`
}

func (s *SiteContentService) ResolvePartner(ctx context.Context, raw string) (*SitePartnerPreview, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := siteHTTPClient()
	defer client.CloseIdleConnections()
	body, base, err := siteDownload(ctx, client, raw, 1<<20)
	if err != nil {
		return nil, ErrContentInvalid
	}
	result := &SitePartnerPreview{URL: base.String(), Warnings: []string{}, ThumbnailSource: "none"}
	icon, cover := "", ""
	z := html.NewTokenizer(strings.NewReader(string(body)))
	inTitle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		token := z.Token()
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			attrs := map[string]string{}
			for _, a := range token.Attr {
				attrs[strings.ToLower(a.Key)] = a.Val
			}
			switch token.Data {
			case "title":
				inTitle = true
			case "meta":
				key := strings.ToLower(attrs["property"])
				if key == "" {
					key = strings.ToLower(attrs["name"])
				}
				switch key {
				case "og:title":
					result.Name = attrs["content"]
				case "description", "og:description":
					if result.Description == "" {
						result.Description = attrs["content"]
					}
				case "og:image":
					cover = attrs["content"]
				}
			case "link":
				if strings.Contains(strings.ToLower(attrs["rel"]), "icon") && icon == "" {
					icon = attrs["href"]
				}
			}
		} else if tt == html.TextToken && inTitle && result.Name == "" {
			result.Name = token.Data
		} else if tt == html.EndTagToken && token.Data == "title" {
			inTitle = false
		}
	}
	clean := func(s string, n int) string {
		r := []rune(strings.Join(strings.Fields(s), " "))
		if len(r) > n {
			r = r[:n]
		}
		return string(r)
	}
	result.Name = clean(result.Name, 100)
	result.Description = clean(result.Description, 500)
	if result.Name == "" {
		result.Name = base.Hostname()
	}
	saveRemote := func(fetchCtx context.Context, fetchClient *http.Client, raw string) string {
		u, e := url.Parse(raw)
		if e != nil {
			return ""
		}
		b, _, e := siteDownload(fetchCtx, fetchClient, base.ResolveReference(u).String(), SiteAssetMaxBytes)
		if e != nil {
			return ""
		}
		saved, e := s.SaveAsset(fetchCtx, b)
		if e != nil {
			return ""
		}
		return saved
	}
	// 坏图标不能串行耗尽截图预算；两个有界分支各自写独立结果字段。
	var images sync.WaitGroup
	images.Add(1)
	go func() {
		defer images.Done()
		iconCtx, stop := context.WithTimeout(ctx, 6*time.Second)
		defer stop()
		if icon != "" {
			result.Icon = saveRemote(iconCtx, client, icon)
		}
		if result.Icon == "" {
			result.Icon = saveRemote(iconCtx, client, "https://www.google.com/s2/favicons?domain="+url.QueryEscape(base.Hostname())+"&sz=128")
		}
	}()
	screenshotClient := siteHTTPClient()
	screenshotClient.Timeout = 24 * time.Second
	screenshotClient.Transport.(*http.Transport).ResponseHeaderTimeout = 20 * time.Second
	defer screenshotClient.CloseIdleConnections()
	// 固定截图提供方只接收已验证的公开URL，不执行网页脚本于生产应用进程。
	if _, err = sitePublicURL(base.String()); err == nil {
		blocked, e := isPrivateOrLoopbackHost(ctx, base.Hostname())
		if e == nil && !blocked {
			result.Thumbnail = saveRemote(ctx, screenshotClient, "https://image.thum.io/get/width/800/crop/600/noanimate/"+base.String())
			if result.Thumbnail != "" {
				result.ThumbnailSource = "screenshot"
			}
		}
	}
	if result.Thumbnail == "" && cover != "" {
		result.Thumbnail = saveRemote(ctx, client, cover)
		if result.Thumbnail != "" {
			result.ThumbnailSource = "og_image"
		}
	}
	images.Wait()
	if result.Thumbnail == "" {
		result.Warnings = append(result.Warnings, "thumbnail_unavailable")
	}
	if result.Icon == "" {
		result.Warnings = append(result.Warnings, "icon_unavailable")
	}
	return result, nil
}
