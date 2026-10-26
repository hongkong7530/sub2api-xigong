//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func relayVideoRedirect(location string) *http.Response {
	return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader(""))}
}

func TestGrokMediaCustomCDNUsesAuthenticatedRelayThenCredentialFreeRedirect(t *testing.T) {
	cdn := "https://cdn.example.test/signed%2Fvideo.mp4?token=fixture%2Bsignature&part=1"
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"status":"done","video":{"url":"https://cdn.example.test/untrusted-hint.mp4"}}`),
		relayVideoRedirect(cdn),
		{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Range": []string{"bytes 0-12/100"}}, Body: io.NopCloser(strings.NewReader("video-payload"))},
	}}
	account := grokMediaContentTestAccount()
	account.Credentials[credKeyHeaderOverrideEnabled] = true
	account.Credentials[credKeyHeaderOverrides] = map[string]any{"user-agent": "private-agent", "x-api-key": "private-extra"}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{"Range": "bytes=0-12"})
	_, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "video-payload", recorder.Body.String())
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "https://relay.example/v1/videos/task-1/content", upstream.requests[1].URL.String())
	require.Equal(t, "Bearer upstream-key", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, cdn, upstream.requests[2].URL.String(), "signed path and query remain byte-for-byte")
	require.Empty(t, upstream.requests[2].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[2].Header.Get("X-Api-Key"))
	require.Empty(t, upstream.requests[2].Header.Get("User-Agent"))
	require.Equal(t, "bytes=0-12", upstream.requests[2].Header.Get("Range"))
	require.True(t, HTTPUpstreamPublicHostsOnly(upstream.requests[2].Context()))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.requests[2].Context()))
}

func TestGrokMediaRelayRejectsUnsafeOrRepeatedRedirects(t *testing.T) {
	for _, location := range []string{"http://cdn.example.test/video.mp4", "https://127.0.0.1/video.mp4", "https://169.254.169.254/latest", "https://10.0.0.1/video.mp4", "https://localhost/video.mp4", "https://key@cdn.example.test/video.mp4", "https://cdn.example.test:444/video.mp4", "https://cdn.example.test/video.mp4#fragment", ""} {
		t.Run(location, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{grokMediaContentStatusResponse(`{"status":"done","video":{"url":"https://cdn.example.test/video.mp4"}}`), relayVideoRedirect(location)}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, _ := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
			_, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
			require.Error(t, err)
			require.Len(t, upstream.requests, 2, "unsafe redirect is never fetched")
			require.False(t, IsResponseCommitted(c))
		})
	}
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{grokMediaContentStatusResponse(`{"status":"done"}`), relayVideoRedirect("https://cdn.example.test/video.mp4"), relayVideoRedirect("https://another.example.test/video.mp4")}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, _ := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
	_, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.ErrorContains(t, err, "redirect is not allowed")
	require.Len(t, upstream.requests, 3)
}

func TestGrokMediaCDNFailureDoesNotDisableAPIAccountOrExposeChallenge(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"status":"done"}`), relayVideoRedirect("https://cdn.example.test/video.mp4"),
		{StatusCode: http.StatusForbidden, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("<html>Attention Required private-detail</html>"))},
	}}
	// No account services are provisioned: a CDN response must not invoke account penalties.
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
	_, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.ErrorContains(t, err, "download unavailable")
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), "video_content_unavailable")
	require.NotContains(t, recorder.Body.String(), "private-detail")
}

func TestGrokMediaContentPreservesNonzeroRangeBytes(t *testing.T) {
	for _, payload := range []string{`{"compressed":"bytes"}`, "<!DOCTYPE html><html>compressed bytes</html>", "   \x00\x01video"} {
		t.Run(payload, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
				grokMediaContentStatusResponse(`{"status":"done"}`),
				{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Range": []string{"bytes 1024-2047/4096"}}, Body: io.NopCloser(strings.NewReader(payload))},
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", map[string]string{"Range": "bytes=1024-2047"})
			_, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
			require.NoError(t, err)
			require.Equal(t, http.StatusPartialContent, recorder.Code)
			require.Equal(t, payload, recorder.Body.String())
			require.Equal(t, "bytes 1024-2047/4096", recorder.Header().Get("Content-Range"))
		})
	}
}

func TestGrokMediaContentRejectsJSONDisguisedAsVideo(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"status":"done"}`),
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"download_failed"}}`))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
	_, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.ErrorContains(t, err, "non-video")
	require.False(t, IsResponseCommitted(c))
	require.Empty(t, recorder.Body.String())
}

func TestGrokMediaContentRejectsHTMLSuccessBeforeCommitting(t *testing.T) {
	for _, mime := range []string{"text/html", "application/json", "video/mp4", "application/octet-stream", ""} {
		t.Run(mime, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{grokMediaContentStatusResponse(`{"status":"done"}`), {StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{mime}}, Body: io.NopCloser(strings.NewReader("<!DOCTYPE html><html>Attention Required</html>"))}}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			c, recorder := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
			_, err := svc.ForwardGrokMedia(context.Background(), c, grokMediaContentTestAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
			require.ErrorContains(t, err, "non-video")
			require.False(t, IsResponseCommitted(c))
			require.Empty(t, recorder.Body.String())
		})
	}
}
