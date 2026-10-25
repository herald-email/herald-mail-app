package imagefx

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"
)

func TestAnalogStylesPreserveSourceAndRemainDistinct(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1000, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 1000; x++ {
			i := y*src.Stride + x*4
			src.Pix[i] = uint8(x)
			src.Pix[i+1] = uint8(y)
			src.Pix[i+2] = uint8(x + y)
			src.Pix[i+3] = uint8(x * y)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), buf.Bytes()...)
	var results [][]byte
	for _, style := range []Style{OldColorTV, WornVHS, HauntedBroadcast} {
		t.Run(string(style), func(t *testing.T) {
			parsed, err := Parse(string(style))
			if err != nil || parsed != style || Resolve(parsed, "amber-crt") != style {
				t.Fatal("selection", err)
			}
			out, err := Transform(original, style)
			if err != nil {
				t.Fatal(err)
			}
			again, err := Transform(original, style)
			if err != nil || !bytes.Equal(out, again) {
				t.Fatal("not deterministic", err)
			}
			decoded, _, err := image.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds().Dx() > MaxWidth || decoded.Bounds().Dy() > MaxHeight {
				t.Fatal("unbounded output")
			}
			filtered := filter(src, style)
			for i := 3; i < len(src.Pix); i += 4 {
				if src.Pix[i] != filtered.Pix[i] {
					t.Fatal("alpha changed")
				}
			}
			for _, prior := range results {
				if bytes.Equal(prior, out) {
					t.Fatal("styles identical")
				}
			}
			results = append(results, out)
		})
	}
	if !bytes.Equal(original, buf.Bytes()) {
		t.Fatal("source modified")
	}
}

func TestAnalogExposureAndTracking(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 120, 150))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i] = 180
		src.Pix[i+1] = 120
		src.Pix[i+2] = 60
		src.Pix[i+3] = 255
	}
	sums := map[Style]int{}
	for _, style := range []Style{OldColorTV, WornVHS, HauntedBroadcast} {
		out := filter(src, style)
		for i := 0; i < len(out.Pix); i += 4 {
			sums[style] += int(out.Pix[i]) + int(out.Pix[i+1]) + int(out.Pix[i+2])
		}
		r, g, b := out.Pix[60*out.Stride+60*4], out.Pix[60*out.Stride+60*4+1], out.Pix[60*out.Stride+60*4+2]
		if !(r > g && g > b) {
			t.Fatal("source color lost", style)
		}
	}
	if sums[HauntedBroadcast] >= sums[OldColorTV]*70/100 || sums[HauntedBroadcast] >= sums[WornVHS]*70/100 {
		t.Fatal("haunted should be substantially darker")
	}
}

func BenchmarkTransformAnalog(b *testing.B) {
	data, err := os.ReadFile("../demo/assets/changing-landscape-960px.jpg")
	if err != nil {
		b.Fatal(err)
	}
	for _, style := range []Style{OldColorTV, WornVHS, HauntedBroadcast} {
		b.Run(string(style), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Transform(data, style); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
