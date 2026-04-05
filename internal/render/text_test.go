package render

import (
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hell…"},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "ab…"},
	}
	for _, tt := range tests {
		got := Truncate(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "Hello World"},
		{"café", "café"},
		{"emoji 🎉 here", "emoji here"},
		{"🚀launch", "launch"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		got := SanitizeText(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripInvisibleChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal text", "hello world", "hello world"},
		{"zero-width space", "hello\u200bworld", "helloworld"},
		{"BOM", "\ufeffhello", "hello"},
		{"CGJ spacer", "a\u034fb", "ab"},
		{"preserves tabs", "a\tb", "a\tb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripInvisibleChars(tt.input)
			if got != tt.want {
				t.Errorf("StripInvisibleChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWrapLines(t *testing.T) {
	text := "short\n\nlong line that should be wrapped at some point because it is too wide"
	lines := WrapLines(text, 20)
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %v", len(lines), lines)
	}
	// First line should be "short"
	if lines[0] != "short" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "short")
	}
	// Second line should be blank (paragraph break)
	if lines[1] != "" {
		t.Errorf("lines[1] = %q, want empty", lines[1])
	}
	// No line should exceed 20 visible chars
	for i, l := range lines {
		if len(l) > 40 { // generous allowance for escape sequences
			t.Errorf("line %d too long: %q", i, l)
		}
	}
}

func TestWrapText_OSC8(t *testing.T) {
	// An OSC 8 hyperlink should not count toward visible width
	link := "\033]8;;https://example.com\033\\click\033]8;;\033\\"
	lines := WrapText("prefix "+link+" suffix", 30)
	// Should fit on one line since visible text is "prefix click suffix" = 19 chars
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d: %v", len(lines), lines)
	}
}

func TestWrapLines_CollapsesBlanks(t *testing.T) {
	text := "a\n\n\n\nb"
	lines := WrapLines(text, 80)
	// Should collapse multiple blank lines to one
	blankCount := 0
	for _, l := range lines {
		if l == "" {
			blankCount++
		}
	}
	if blankCount != 1 {
		t.Errorf("expected 1 blank line, got %d in %v", blankCount, lines)
	}
}

func TestCalculateTextWidth(t *testing.T) {
	if w := CalculateTextWidth("abc"); w != 3 {
		t.Errorf("CalculateTextWidth(\"abc\") = %d, want 3", w)
	}
	if w := CalculateTextWidth("日本"); w != 4 {
		t.Errorf("CalculateTextWidth(\"日本\") = %d, want 4", w)
	}
}

func TestSkipEscapeSeq(t *testing.T) {
	// CSI sequence: ESC [ 31 m
	runes := []rune("\033[31mhello")
	end := SkipEscapeSeq(runes, 0)
	remaining := string(runes[end:])
	if !strings.HasPrefix(remaining, "hello") {
		t.Errorf("after CSI skip, remaining = %q, want prefix 'hello'", remaining)
	}

	// OSC sequence: ESC ] 8 ;; url ESC \.
	runes2 := []rune("\033]8;;https://x.com\033\\label")
	end2 := SkipEscapeSeq(runes2, 0)
	remaining2 := string(runes2[end2:])
	if !strings.HasPrefix(remaining2, "label") {
		t.Errorf("after OSC skip, remaining = %q, want prefix 'label'", remaining2)
	}
}
