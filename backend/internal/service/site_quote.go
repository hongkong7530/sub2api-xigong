package service

import (
	"context"
	"encoding/json"
	"strings"
)

// 只向固定公开语句服务取文案；不转发登录身份、Cookie或请求头。
func (s *SiteContentService) FetchQuote(ctx context.Context) (map[string]string, error) {
	client := siteHTTPClient()
	defer client.CloseIdleConnections()
	body, _, err := siteDownload(ctx, client, "https://international.v1.hitokoto.cn/?c=a&c=d&c=i&max_length=60", 32768)
	if err != nil {
		return nil, err
	}
	var q struct {
		Text   string `json:"hitokoto"`
		From   string `json:"from"`
		Author string `json:"from_who"`
	}
	if err = json.Unmarshal(body, &q); err != nil || strings.TrimSpace(q.Text) == "" || len([]rune(q.Text)) > 100 {
		return nil, ErrContentInvalid
	}
	return map[string]string{"zh": q.Text, "en": "", "source": strings.Trim(strings.TrimSpace(q.Author)+" · "+strings.TrimSpace(q.From), " ·")}, nil
}
