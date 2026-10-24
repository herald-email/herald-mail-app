package imagefx

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestTransformDeterministicBoundedAndOriginalPreserved(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1200, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			src.SetNRGBA(x, y, color.NRGBA{180, 190, 210, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, src)
	input := append([]byte(nil), buf.Bytes()...)
	for _, style := range []Style{Amber, BurntAmber} {
		a, e := Transform(input, style)
		if e != nil {
			t.Fatal(e)
		}
		b, e := Transform(input, style)
		if e != nil || !bytes.Equal(a, b) {
			t.Fatal("unstable output", e)
		}
		im, _, e := image.Decode(bytes.NewReader(a))
		if e != nil {
			t.Fatal(e)
		}
		if im.Bounds().Dx() > MaxWidth || im.Bounds().Dy() > MaxHeight {
			t.Fatal(im.Bounds())
		}
		r, g, bv, _ := im.At(200, 200).RGBA()
		if !(r > g && g > bv) {
			t.Fatal("not amber")
		}
	}
	if !bytes.Equal(input, buf.Bytes()) {
		t.Fatal("mutated source")
	}
	got, e := Transform(input, Original)
	if e != nil || !bytes.Equal(got, input) {
		t.Fatal("original changed")
	}
}
func TestFilterPreservesAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for i := range src.Pix {
		src.Pix[i] = uint8(i)
	}
	got := filter(src, Amber)
	for i := 3; i < len(src.Pix); i += 4 {
		if got.Pix[i] != src.Pix[i] {
			t.Fatal("alpha mask changed")
		}
	}
}
func TestTransformRejectsBadAndOversizedInput(t *testing.T) {
	for _, data := range [][]byte{[]byte("not an image"), make([]byte, MaxInputBytes+1), []byte("P6\n20000 20000\n255\n")} {
		if _, e := Transform(data, Amber); e == nil {
			t.Fatal("accepted invalid input")
		}
	}
}
func TestParseAndResolve(t *testing.T) {
	for _, s := range []string{"", "auto", "original", " Amber-CRT ", "burnt-amber-crt"} {
		if _, e := Parse(s); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := Parse("bad"); e == nil {
		t.Fatal("accepted bad style")
	}
	if Resolve(Auto, "amber-crt") != Amber || Resolve(Original, "amber-crt") != Original || Resolve(Auto, "forest-crt") != Original {
		t.Fatal("resolution")
	}
}
func BenchmarkTransformAmber(b *testing.B) {
	data, e := os.ReadFile("../demo/assets/changing-landscape-960px.jpg")
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := Transform(data, Amber); e != nil {
			b.Fatal(e)
		}
	}
}

func TestTransformRejectsDecodedPixelBudgetBeforeDecode(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	binary.BigEndian.PutUint32(data[16:20], 20000)
	binary.BigEndian.PutUint32(data[20:24], 20000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatal("fixture header invalid", err)
	}
	if _, err := Transform(data, Amber); err == nil || err.Error() != "image exceeds decoded pixel limit" {
		t.Fatal("pixel guard not used", err)
	}
}

func TestGreenCRTStyles(t *testing.T) {
	data, err := os.ReadFile("../demo/assets/changing-landscape-960px.jpg")
	if err != nil {
		t.Fatal(err)
	}
	var results [][]byte
	for _, style := range []Style{VaultGreen, DarkOlive} {
		parsed, err := Parse(string(style))
		if err != nil || parsed != style {
			t.Fatal("parse", style, err)
		}
		if Resolve(Auto, string(style)) != style || Resolve(Original, string(style)) != Original {
			t.Fatal("resolution", style)
		}
		first, err := Transform(data, style)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Transform(data, style)
		if err != nil || !bytes.Equal(first, second) {
			t.Fatal("unstable", style)
		}
		decoded, _, err := image.Decode(bytes.NewReader(first))
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, _ := decoded.At(400, 150).RGBA()
		if g <= r || g <= b {
			t.Fatal("not green phosphor", style, r, g, b)
		}
		results = append(results, first)
	}
	if bytes.Equal(results[0], results[1]) {
		t.Fatal("green palettes must differ")
	}
}
