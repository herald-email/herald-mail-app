package imagefx

import "image"

// filterAnalog uses a fixed number of neighboring samples per pixel. The result
// is a static raster: no frames, time-dependent noise, or additional cache.
func filterAnalog(src *image.NRGBA, style Style) *image.NRGBA {
	dst := image.NewNRGBA(src.Bounds())
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	sample := func(x, y, c int) int { return int(src.Pix[y*src.Stride+max(0, min(w-1, x))*4+c]) }
	luma := func(x, y int) int { return (54*sample(x, y, 0) + 183*sample(x, y, 1) + 19*sample(x, y, 2)) >> 8 }
	for y := 0; y < h; y++ {
		shift := 0
		tracking := y >= h-max(2, h/30)
		interference := y%137 < 6
		if style == WornVHS {
			shift = noise(11, y/4) / 2
			if tracking {
				shift += noise(5, y) * 3
			}
		}
		if style == HauntedBroadcast {
			shift = noise(7, y/6)
			if interference {
				shift += 12 - y%137*2
			}
		}
		for x := 0; x < w; x++ {
			sx := x + shift
			j := y*dst.Stride + x*4
			edge := 256 - 18*abs(2*x-w)/max(1, w) - 16*abs(2*y-h)/max(1, h)
			lum := luma(sx, y)
			bleedLum := (luma(sx-4, y) + luma(sx-8, y)) / 2
			for c := 0; c < 3; c++ {
				v := sample(sx, y, c)
				switch style {
				case OldColorTV:
					v = (v*6 + sample(sx-1, y, c) + sample(sx+1, y, c)) / 8
					v = v*94/100 + noise(x, y)
					// A restrained aperture-grille texture retains full-color readability.
					if x%3 != c {
						v = v * 92 / 100
					}
					if y%3 == 2 {
						v = v * 86 / 100
					}
				case WornVHS:
					// Delay and soften chroma independently of the sharper luminance.
					chroma := (sample(sx-4, y, c)+sample(sx-8, y, c))/2 - bleedLum
					v = (lum+chroma*65/100)*82/100 + 9 + noise(x, y)*2
					if y%2 == 1 {
						v = v * 94 / 100
					}
					if tracking {
						v = v*65/100 + noise(x, y)*9 + 18
					}
				case HauntedBroadcast:
					v = (v*7 + sample(sx-12, y, c)*3) / 10
					v = max(0, v-13)*57/100 + noise(x, y)*3
					if y%3 == 2 {
						v = v * 72 / 100
					}
					if interference {
						v = v*65/100 + noise(x/2, y)*8
					}
				}
				dst.Pix[j+c] = uint8(max(0, min(255, v*edge/256)))
			}
			dst.Pix[j+3] = src.Pix[y*src.Stride+x*4+3]
		}
	}
	return dst
}
