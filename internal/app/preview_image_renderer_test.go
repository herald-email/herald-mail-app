package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/models"
)

func tinyPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestDetectPreviewImageMode(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if got := detectPreviewImageMode(previewImageModeAuto, true, false); got != previewImageModeIterm2 {
		t.Fatalf("iTerm auto mode = %q, want %q", got, previewImageModeIterm2)
	}
	if got := detectPreviewImageMode(previewImageModeAuto, true, true); got != previewImageModePlaceholder {
		t.Fatalf("ssh auto mode = %q, want placeholder", got)
	}
	if got := detectPreviewImageMode(previewImageModeLinks, true, false); got != previewImageModeLinks {
		t.Fatalf("forced links mode = %q, want links", got)
	}
}

func TestDetectPreviewImageModeKittyAndGhostty(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want previewImageMode
	}{
		{
			name: "kitty window id",
			env:  map[string]string{"KITTY_WINDOW_ID": "12"},
			want: previewImageModeKitty,
		},
		{
			name: "kitty term",
			env:  map[string]string{"TERM": "xterm-kitty"},
			want: previewImageModeKitty,
		},
		{
			name: "ghostty term",
			env:  map[string]string{"TERM": "xterm-ghostty"},
			want: previewImageModeKitty,
		},
		{
			name: "ghostty term program",
			env:  map[string]string{"TERM_PROGRAM": "Ghostty"},
			want: previewImageModeKitty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearTerminalImageEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			if got := detectPreviewImageMode(previewImageModeAuto, true, false); got != tt.want {
				t.Fatalf("auto mode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectPreviewImageModeSSHAndForcedModes(t *testing.T) {
	clearTerminalImageEnv(t)
	t.Setenv("KITTY_WINDOW_ID", "12")
	if got := detectPreviewImageMode(previewImageModeAuto, true, true); got != previewImageModePlaceholder {
		t.Fatalf("ssh auto mode = %q, want placeholder", got)
	}
	if got := detectPreviewImageMode(previewImageModeKitty, false, true); got != previewImageModeKitty {
		t.Fatalf("forced kitty over ssh = %q, want kitty", got)
	}
	if got := detectPreviewImageMode(previewImageModeIterm2, false, false); got != previewImageModeIterm2 {
		t.Fatalf("forced iterm2 without iTerm env = %q, want iterm2", got)
	}
}

func TestDetectPreviewImageModeFallsBackToLinksLocally(t *testing.T) {
	clearTerminalImageEnv(t)
	if got := detectPreviewImageMode(previewImageModeAuto, true, false); got != previewImageModeLinks {
		t.Fatalf("local fallback mode = %q, want links", got)
	}
}

func TestParsePreviewImageMode(t *testing.T) {
	for _, value := range []string{"auto", "iterm2", "kitty", "links", "placeholder", "off"} {
		if _, err := ParsePreviewImageMode(value); err != nil {
			t.Fatalf("ParsePreviewImageMode(%q) unexpected error: %v", value, err)
		}
	}
	if _, err := ParsePreviewImageMode("sixel"); err == nil {
		t.Fatal("ParsePreviewImageMode(\"sixel\") returned nil error, want invalid mode error")
	}
}

func TestModelPreviewImageModeOverrideForcesKitty(t *testing.T) {
	clearTerminalImageEnv(t)
	m := New(&stubBackend{}, nil, "", nil, false)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(false)
	m.SetPreviewImageMode(PreviewImageModeKitty)

	if got := m.currentPreviewImageMode(); got != previewImageModeKitty {
		t.Fatalf("current preview mode = %q, want kitty", got)
	}
	if !m.fullScreenImagesAvailable() {
		t.Fatal("forced kitty mode should make full-screen image viewing available")
	}
}

func clearTerminalImageEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"TERM_PROGRAM", "TERM", "KITTY_WINDOW_ID"} {
		t.Setenv(key, "")
	}
}

func TestPreviewImageCellSizeAvoidsUpscalingTinyImages(t *testing.T) {
	img := models.InlineImage{ContentID: "small", MIMEType: "image/png", Data: tinyPNG(t, 32, 16)}
	size := previewImageCellSize(img, 120, 20)
	if size.Width > 8 {
		t.Fatalf("tiny image width = %d cells, want no large upscale", size.Width)
	}
	if size.Rows > 3 {
		t.Fatalf("tiny image rows = %d, want no large upscale", size.Rows)
	}
}

func TestPreviewImageCellSizeBoundsLargeImages(t *testing.T) {
	img := models.InlineImage{ContentID: "large", MIMEType: "image/png", Data: tinyPNG(t, 1200, 800)}
	size := previewImageCellSize(img, 80, 18)
	if size.Width > 78 {
		t.Fatalf("large image width = %d cells, want <= 78", size.Width)
	}
	if size.Rows > 18 {
		t.Fatalf("large image rows = %d, want <= 18", size.Rows)
	}
	if size.Rows < 8 {
		t.Fatalf("large image rows = %d, want useful preview height", size.Rows)
	}
}

func TestIterm2PreviewRendererReportsRowsWithoutTrailingNewline(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	img := models.InlineImage{ContentID: "photo", MIMEType: "image/png", Data: tinyPNG(t, 120, 80)}
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeIterm2,
		Image:         img,
		InnerWidth:    100,
		AvailableRows: 12,
	})

	if rendered.Rows <= 0 || rendered.Rows > 12 {
		t.Fatalf("rows = %d, want 1..12", rendered.Rows)
	}
	if !strings.Contains(rendered.Content, "\x1b]1337;File=") {
		t.Fatalf("expected iTerm2 image escape, got %q", rendered.Content)
	}
	if strings.HasSuffix(rendered.Content, "\n") {
		t.Fatalf("image renderer content should not hide trailing newline: %q", rendered.Content)
	}
}

func TestForcedIterm2PreviewRendererDoesNotRequireItermEnv(t *testing.T) {
	clearTerminalImageEnv(t)
	img := models.InlineImage{ContentID: "photo", MIMEType: "image/png", Data: tinyPNG(t, 120, 80)}
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeIterm2,
		Image:         img,
		InnerWidth:    100,
		AvailableRows: 12,
	})

	if !strings.Contains(rendered.Content, "\x1b]1337;File=") {
		t.Fatalf("forced iTerm2 should emit OSC 1337 without iTerm env, got %q", rendered.Content)
	}
}

func TestKittyPreviewRendererReportsRowsWithoutTrailingNewline(t *testing.T) {
	img := models.InlineImage{ContentID: "photo", MIMEType: "image/png", Data: tinyPNG(t, 120, 80)}
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeKitty,
		Image:         img,
		InnerWidth:    100,
		AvailableRows: 12,
	})

	if rendered.Rows <= 0 || rendered.Rows > 12 {
		t.Fatalf("rows = %d, want 1..12", rendered.Rows)
	}
	if !strings.Contains(rendered.Content, "\x1b_G") {
		t.Fatalf("expected Kitty image escape, got %q", rendered.Content)
	}
	if strings.HasSuffix(rendered.Content, "\n") {
		t.Fatalf("image renderer content should not hide trailing newline: %q", rendered.Content)
	}
}

func TestPreviewRowsPreserveKittyEscapeContent(t *testing.T) {
	img := models.InlineImage{ContentID: "photo", MIMEType: "image/png", Data: tinyPNG(t, 120, 80)}
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeKitty,
		Image:         img,
		InnerWidth:    100,
		AvailableRows: 12,
	})

	rows := previewRowsFromRenderedImage(rendered, 100)
	if len(rows) == 0 || !strings.Contains(rows[0].Content, "\x1b_G") {
		t.Fatalf("expected first preview row to preserve Kitty escape, got %#v", rows)
	}
}

func TestPreviewImageRendererFallbacks(t *testing.T) {
	empty := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeIterm2,
		Image:         models.InlineImage{ContentID: "empty", MIMEType: "image/png"},
		InnerWidth:    80,
		AvailableRows: 6,
	})
	if empty.Rows != 1 || !strings.Contains(empty.Content, "image unavailable") {
		t.Fatalf("empty image fallback = %#v", empty)
	}

	off := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeOff,
		Image:         models.InlineImage{ContentID: "off", MIMEType: "image/png", Data: []byte("x")},
		InnerWidth:    80,
		AvailableRows: 6,
	})
	if off.Rows != 0 || off.Content != "" {
		t.Fatalf("off mode = %#v, want empty block", off)
	}
}

func TestPreviewImageModeLinksPreservesOpenImageLabelWithLongAlt(t *testing.T) {
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeLinks,
		Image:         models.InlineImage{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		Alt:           strings.Repeat("very long alternate text ", 8),
		InnerWidth:    40,
		AvailableRows: 5,
		LinkLabel:     "open image 1",
		LinkURL:       "http://127.0.0.1:12345/image/logo",
	})

	plain := ansi.Strip(rendered.Content)
	if rendered.Rows != 1 {
		t.Fatalf("rows = %d, want 1", rendered.Rows)
	}
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("local image link label should remain visible, got %q", plain)
	}
	if !strings.Contains(rendered.Content, "\x1b]8;;http://127.0.0.1:12345/image/logo") {
		t.Fatalf("expected OSC8 local image target, got raw %q", rendered.Content)
	}
}

func TestPreviewImageModeLinksPreservesOpenImageLabelForLargeImage(t *testing.T) {
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeLinks,
		Image:         models.InlineImage{ContentID: "large", MIMEType: "image/jpeg", Data: make([]byte, maxPreviewImageBytes+1)},
		InnerWidth:    80,
		AvailableRows: 5,
		LinkLabel:     "open image 1",
		LinkURL:       "http://127.0.0.1:12345/image/large",
	})

	plain := ansi.Strip(rendered.Content)
	if rendered.Rows != 1 {
		t.Fatalf("rows = %d, want 1", rendered.Rows)
	}
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("large local image link label should remain visible, got %q", plain)
	}
	if strings.Contains(plain, "too large to render inline") {
		t.Fatalf("link mode should not suppress local open link for large images, got %q", plain)
	}
	if !strings.Contains(rendered.Content, "\x1b]8;;http://127.0.0.1:12345/image/large") {
		t.Fatalf("expected OSC8 local image target, got raw %q", rendered.Content)
	}
}
