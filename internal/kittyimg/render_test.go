package kittyimg

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func TestIsSupportedDetectsKittyAndGhostty(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "kitty window id", env: map[string]string{"KITTY_WINDOW_ID": "42"}},
		{name: "kitty term", env: map[string]string{"TERM": "xterm-kitty"}},
		{name: "ghostty term", env: map[string]string{"TERM": "xterm-ghostty"}},
		{name: "ghostty term program", env: map[string]string{"TERM_PROGRAM": "Ghostty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			if !IsSupported() {
				t.Fatalf("IsSupported() = false with env %#v", tt.env)
			}
		})
	}
}

func TestRenderInlineTranscodesToPNGAndChunks(t *testing.T) {
	rendered, err := RenderInline(testJPEG(t, 320, 240), 40, 12)
	if err != nil {
		t.Fatalf("RenderInline returned error: %v", err)
	}
	if !strings.HasPrefix(rendered, "\x1b_G") {
		t.Fatalf("rendered output should start with Kitty escape, got %q", rendered[:min(len(rendered), 20)])
	}
	if !strings.Contains(rendered, "a=T") || !strings.Contains(rendered, "f=100") || !strings.Contains(rendered, "c=40") || !strings.Contains(rendered, "r=12") {
		t.Fatalf("rendered output missing Kitty controls: %q", rendered[:min(len(rendered), 160)])
	}
	if strings.HasSuffix(rendered, "\n") {
		t.Fatalf("rendered output should not end with newline: %q", rendered[len(rendered)-min(len(rendered), 20):])
	}
	if strings.Count(rendered, "\x1b_G") < 2 {
		t.Fatalf("expected large image to be chunked across multiple Kitty commands")
	}
}

func TestRenderUsesEnvironmentDetection(t *testing.T) {
	clearEnv(t)
	if got := Render(testJPEG(t, 12, 12), 4, 2); got != "" {
		t.Fatalf("Render without Kitty/Ghostty env = %q, want empty string", got)
	}
	t.Setenv("TERM", "xterm-ghostty")
	if got := Render(testJPEG(t, 12, 12), 4, 2); !strings.Contains(got, "\x1b_G") {
		t.Fatalf("Render with Ghostty env should emit Kitty escape, got %q", got)
	}
}

func TestDeleteVisiblePlacementsEmitsQuietDeleteCommand(t *testing.T) {
	rendered := DeleteVisiblePlacements()
	if rendered != "\x1b_Ga=d,d=A,q=2\x1b\\" {
		t.Fatalf("DeleteVisiblePlacements() = %q, want quiet visible-placement delete command", rendered)
	}
	if strings.HasSuffix(rendered, "\n") {
		t.Fatalf("delete command should not end with newline: %q", rendered)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"TERM_PROGRAM", "TERM", "KITTY_WINDOW_ID"} {
		t.Setenv(key, "")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
