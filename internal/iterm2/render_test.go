package iterm2

import (
	"strings"
	"testing"
)

func TestRenderInlineUsesExplicitCellDimensions(t *testing.T) {
	rendered := RenderInline([]byte("image-bytes"), 64, 18)

	for _, want := range []string{
		"\x1b]1337;File=",
		"inline=1",
		"preserveAspectRatio=0",
		"size=11",
		"width=64",
		"height=18",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered escape missing %q: %q", want, rendered)
		}
	}
}

func TestRenderInlineReservesRowsBeforeDrawingWholeImage(t *testing.T) {
	rendered := RenderInline([]byte("image-bytes"), 64, 18)

	if got := strings.Count(rendered, "\n"); got != 17 {
		t.Fatalf("RenderInline reservation newlines = %d, want 17 for an 18-row image: %q", got, rendered)
	}
	if !strings.Contains(rendered, "\r\x1b[2K") || !strings.Contains(rendered, "\r\x1b[64C\n") {
		t.Fatalf("RenderInline should clear each reserved row before moving down, got %q", rendered)
	}
	if got := strings.Count(rendered, "\x1b[2K"); got != 18 {
		t.Fatalf("RenderInline clear-line commands = %d, want 18 for an 18-row image: %q", got, rendered)
	}
	if !strings.Contains(rendered, "\x1b[17A\x1b]1337;File=") {
		t.Fatalf("RenderInline should move back to the reserved box top before OSC output, got %q", rendered)
	}
	if strings.HasSuffix(rendered, "\n") {
		t.Fatalf("RenderInline should not leave a trailing newline after the OSC draw, got %q", rendered)
	}
}

func TestRenderInlineInCellBoxClearsFullReservedWidth(t *testing.T) {
	rendered := RenderInlineInCellBox([]byte("image-bytes"), 64, 18, 100)

	clearRun := strings.Repeat(" ", 100)
	if !strings.Contains(rendered, "\r\x1b[2K"+clearRun+"\r\x1b[64C\n") {
		t.Fatalf("RenderInlineInCellBox should clear full reserved width before moving to image edge, got %q", rendered)
	}
	if strings.Contains(rendered, "width=100") {
		t.Fatalf("clear width must not change image width, got %q", rendered)
	}
	if !strings.Contains(rendered, "width=64") {
		t.Fatalf("image width should remain 64, got %q", rendered)
	}
}

func TestRenderRequiresItermEnvironmentButRenderInlineDoesNot(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	if got := Render([]byte("image-bytes"), 64, 18); got != "" {
		t.Fatalf("Render without iTerm env = %q, want empty string", got)
	}
	if got := RenderInline([]byte("image-bytes"), 64, 18); !strings.Contains(got, "\x1b]1337;File=") {
		t.Fatalf("RenderInline should support forced protocol mode, got %q", got)
	}
}
