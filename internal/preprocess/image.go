package preprocess

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log/slog"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxImageEdge     = 1600
	jpegQuality      = 85
	maxImageBytes    = 1_500_000
	maxDecodePixels  = 20_000_000
)

func CompactImage(data []byte) ([]byte, string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, "", fmt.Errorf("decode image: invalid dimensions")
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxDecodePixels {
		return nil, "", fmt.Errorf("decode image: dimensions too large")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := 1.0
	if w > maxImageEdge || h > maxImageEdge {
		sw := float64(maxImageEdge) / float64(w)
		sh := float64(maxImageEdge) / float64(h)
		if sw < sh {
			scale = sw
		} else {
			scale = sh
		}
	}
	nw := int(float64(w) * scale)
	nh := int(float64(h) * scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, "", err
	}
	compacted := out.Bytes()
	slog.Debug("compact image", "in_bytes", len(data), "out_bytes", len(compacted), "width", nw, "height", nh)
	if len(compacted) > maxImageBytes {
		return nil, "", fmt.Errorf("image still too large after compact (%d bytes)", len(compacted))
	}
	return compacted, "image/jpeg", nil
}
