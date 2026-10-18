package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"math"
)

const SiteAssetMaxBytes = 8 << 20

func validWallpaper(w *SiteWallpaper) bool {
	for _, u := range []*string{w.DesktopURL, w.MobileURL} {
		if u != nil && *u != "" && !ValidSiteURL(*u, true) {
			return false
		}
	}
	for _, v := range []*float64{w.MobileFocusX, w.MobileFocusY} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 100) {
			return false
		}
	}
	return true
}

// 上传只接受可解码的有界光栅图片；统一重编码，不信任文件名或声明的MIME。
func (s *SiteContentService) SaveAsset(ctx context.Context, data []byte) (string, error) {
	if len(data) == 0 || len(data) > SiteAssetMaxBytes {
		return "", ErrContentInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return "", ErrContentInvalid
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrContentInvalid
	}
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Over)
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, dst, &jpeg.Options{Quality: 88}); err != nil {
		return "", err
	}
	if encoded.Len() > SiteAssetMaxBytes {
		return "", ErrContentInvalid
	}
	hash := sha256.Sum256(encoded.Bytes())
	id := hex.EncodeToString(hash[:])
	if err = s.Repo.SaveSiteAsset(ctx, id, "image/jpeg", encoded.Bytes()); err != nil {
		return "", err
	}
	return "/api/v1/site-content/assets/" + id, nil
}
