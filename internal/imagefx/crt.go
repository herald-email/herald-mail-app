// Package imagefx implements bounded, deterministic local preview effects.
package imagefx

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"strings"
)

type Style string

const (
	Auto             Style = "auto"
	Original         Style = "original"
	Amber            Style = "amber-crt"
	BurntAmber       Style = "burnt-amber-crt"
	VaultGreen       Style = "vault-green-crt"
	DarkOlive        Style = "dark-olive-crt"
	Gloom            Style = "gloom-crt"
	OldColorTV       Style = "old-color-tv"
	WornVHS          Style = "worn-vhs"
	HauntedBroadcast Style = "haunted-broadcast"
	MaxInputBytes          = 5 * 1024 * 1024
	MaxPixels              = 16 * 1000 * 1000
	MaxWidth               = 960
	MaxHeight              = 540
)

func Parse(value string) (Style, error) {
	s := Style(strings.ToLower(strings.TrimSpace(value)))
	if s == "" {
		s = Auto
	}
	switch s {
	case Auto, Original, Amber, BurntAmber, VaultGreen, DarkOlive, Gloom, OldColorTV, WornVHS, HauntedBroadcast:
		return s, nil
	}
	return "", fmt.Errorf("invalid image style %q (valid: auto, original, amber-crt, burnt-amber-crt, vault-green-crt, dark-olive-crt, gloom-crt, old-color-tv, worn-vhs, haunted-broadcast)", value)
}
func Resolve(style Style, theme string) Style {
	if style == "" || style == Auto {
		switch theme {
		case string(Amber):
			return Amber
		case string(BurntAmber):
			return BurntAmber
		case string(VaultGreen):
			return VaultGreen
		case string(DarkOlive):
			return DarkOlive
		}
		return Original
	}
	return style
}

// Transform never mutates source bytes. Call outside the UI/render goroutine.
// Only one frame is decoded for animated formats; unsupported input is an error
// so callers can preserve the normal original-image fallback.
func Transform(data []byte, style Style) ([]byte, error) {
	if style == Original {
		return data, nil
	}
	if style != Amber && style != BurntAmber && style != VaultGreen && style != DarkOlive && style != Gloom && style != OldColorTV && style != WornVHS && style != HauntedBroadcast {
		return nil, fmt.Errorf("unresolved image style %q", style)
	}
	if len(data) > MaxInputBytes {
		return nil, fmt.Errorf("image exceeds input limit")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxPixels/cfg.Height {
		return nil, fmt.Errorf("image exceeds decoded pixel limit")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	if w > MaxWidth {
		h = max(1, h*MaxWidth/w)
		w = MaxWidth
	}
	if h > MaxHeight {
		w = max(1, w*MaxHeight/h)
		h = MaxHeight
	}
	// Normalize once with optimized stdlib raster conversion, then sample raw
	// pixels. Avoid per-pixel color interface allocations for JPEG/YCbCr input.
	b := src.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	scaled := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*cfg.Height/h)*rgba.Stride + (x*cfg.Width/w)*4
			j := y*scaled.Stride + x*4
			a := uint32(rgba.Pix[i+3])
			for c := 0; c < 3; c++ {
				if a > 0 {
					scaled.Pix[j+c] = uint8(uint32(rgba.Pix[i+c]) * 255 / a)
				}
			}
			scaled.Pix[j+3] = uint8(a)
		}
	}
	out := filter(scaled, style)
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func noise(x, y int) int {
	n := uint32(x)*374761393 + uint32(y)*668265263 + 42
	n = (n ^ (n >> 13)) * 1274126177
	return int((n^(n>>16))%9) - 4
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func filter(src *image.NRGBA, style Style) *image.NRGBA {
	if style == OldColorTV || style == WornVHS || style == HauntedBroadcast {
		return filterAnalog(src, style)
	}
	dst := image.NewNRGBA(src.Bounds())
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	black, ink := [3]int{10, 5, 1}, [3]int{220, 131, 36}
	switch style {
	case BurntAmber:
		black, ink = [3]int{9, 3, 1}, [3]int{207, 101, 29}
	case VaultGreen:
		black, ink = [3]int{3, 8, 3}, [3]int{112, 208, 89}
	case DarkOlive:
		black, ink = [3]int{5, 8, 6}, [3]int{147, 180, 129}
	}
	for y := 0; y < h; y++ {
		shift := noise(13, y/5) / 2
		band := y % 103
		if band < 5 {
			shift += 8 - band
		}
		if y%173 > 168 {
			shift -= 5
		}
		for x := 0; x < w; x++ {
			sx := max(0, min(w-1, x+shift))
			i := y*src.Stride + sx*4
			j := y*dst.Stride + x*4
			l := (54*int(src.Pix[i]) + 183*int(src.Pix[i+1]) + 19*int(src.Pix[i+2])) >> 8
			ghost := y*src.Stride + max(0, min(w-1, sx-5))*4
			gl := (54*int(src.Pix[ghost]) + 183*int(src.Pix[ghost+1]) + 19*int(src.Pix[ghost+2])) >> 8
			l = (l*9 + gl) / 10
			colorLuma := max(1, l)
			l = max(0, l-12)*85/100 + noise(x, y)*5 + noise(x/2, y/2)*3
			if y%3 == 2 {
				l = l * 70 / 100
			}
			if band < 3 {
				l = l*78/100 + noise(x, y)*4
			}
			edge := 256 - 28*abs(2*x-w)/max(1, w) - 20*abs(2*y-h)/max(1, h)
			l = max(0, min(255, l*edge/256))
			// Preserve the destination alpha mask: tracking must not smear transparency
			// into surrounding text or change the silhouette of transparent artwork.
			for c := 0; c < 3; c++ {
				if style == Gloom {
					// Scale all source channels by the same exposure to retain hues.
					channel := (9*int(src.Pix[i+c]) + int(src.Pix[ghost+c])) / 10
					dst.Pix[j+c] = uint8(min(255, channel*l*72/(colorLuma*100)))
				} else {
					dst.Pix[j+c] = uint8(black[c] + l*ink[c]/255)
				}
			}
			dst.Pix[j+3] = src.Pix[y*src.Stride+x*4+3]
		}
	}
	return dst
}
