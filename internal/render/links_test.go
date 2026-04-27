package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestShortenURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com", "example.com"},
		{"https://example.com/", "example.com"},
		{"https://example.com/path/to/page", "example.com/path/to/page"},
		{"https://example.com/very/long/path/that/exceeds/fifty/characters/total/definitely", "example.com/very/long/path/that/exceeds/fifty/c..."},
		{"not a url at all", "not a url at all"},
	}
	for _, tt := range tests {
		got := ShortenURL(tt.input)
		if got != tt.want {
			t.Errorf("ShortenURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLinkifyURLs(t *testing.T) {
	text := "visit https://example.com/page today"
	result := LinkifyURLs(text)
	// Should contain OSC 8 escape sequence
	if !strings.Contains(result, "\033]8;;") {
		t.Errorf("LinkifyURLs did not produce OSC 8 links: %q", result)
	}
	// Should contain the shortened label
	if !strings.Contains(result, "example.com/page") {
		t.Errorf("LinkifyURLs missing shortened label in: %q", result)
	}
	// Should still contain the surrounding text
	if !strings.Contains(result, "visit ") || !strings.Contains(result, " today") {
		t.Errorf("LinkifyURLs lost surrounding text: %q", result)
	}
}

func TestLinkifyWrappedLines(t *testing.T) {
	lines := []string{"no url here", "see https://example.com for details"}
	result := LinkifyWrappedLines(lines)
	if result[0] != lines[0] {
		t.Errorf("line without URL should be unchanged: got %q", result[0])
	}
	if !strings.Contains(result[1], "\033]8;;") {
		t.Errorf("line with URL should be linkified: got %q", result[1])
	}
}

func TestRenderEmailBodyLines_MarkdownLinksUseAnchorText(t *testing.T) {
	longURL := "https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?o=eyJmaXJzdF9uYW1lIjoiQW50b24iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldReVpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY"
	lines := RenderEmailBodyLines("Welcome\n\n[Display in your browser]("+longURL+")\n\nHi Anton", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if !strings.Contains(visible, "Display in your browser") {
		t.Fatalf("expected anchor text to be visible, got:\n%s", visible)
	}
	if strings.Contains(visible, "taskpad.mail.example") || strings.Contains(visible, "eyJmaXJ") {
		t.Fatalf("expected long URL to be hidden from visible text, got:\n%s", visible)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+longURL+"\x1b\\") {
		t.Fatalf("expected full URL to remain in OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_LabelFollowedByBracketedURLBecomesOneLink(t *testing.T) {
	longURL := "https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?o=eyJmaXJzdF9uYW1lIjoiQW50b24iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldReVpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY"
	lines := RenderEmailBodyLines("Display in your browser\n["+longURL+"]", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if strings.TrimSpace(visible) != "Display in your browser" {
		t.Fatalf("expected label+bracketed URL to render as one visible label, got:\n%s", visible)
	}
	if strings.Contains(visible, "taskpad.mail.example") || strings.Contains(visible, "3TyPkavY9d1vRxPgY") {
		t.Fatalf("expected bracketed URL to be hidden from visible text, got:\n%s", visible)
	}
	if strings.Contains(visible, "[") || strings.Contains(visible, "]") {
		t.Fatalf("expected markdown brackets to be hidden, got:\n%s", visible)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+longURL+"\x1b\\") {
		t.Fatalf("expected full URL to remain in OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_ImageLinksUseAltText(t *testing.T) {
	logoURL := "https://taskpad.mail.example/_next/static/media/taskpad-logo.0-dsvhpw__1x7.png"
	lines := RenderEmailBodyLines("![Taskpad logo]("+logoURL+")", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if strings.TrimSpace(visible) != "Taskpad logo" {
		t.Fatalf("expected image alt text as visible link label, got %q", visible)
	}
	if strings.Contains(visible, "taskpad-logo") {
		t.Fatalf("expected image URL to be hidden from visible text, got %q", visible)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+logoURL+"\x1b\\") {
		t.Fatalf("expected logo URL to remain in OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_NakedLongURLUsesShortLabel(t *testing.T) {
	longURL := "https://example.com/path/to/a/very/long/resource/name/that/would/otherwise/wrap/badly?utm_source=email&token=abcdefghijklmnopqrstuvwxyz0123456789"
	lines := RenderEmailBodyLines("Open "+longURL+" today", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if !strings.Contains(visible, "example.com/path/to/a/very/long/resource/name") {
		t.Fatalf("expected shortened domain/path label, got:\n%s", visible)
	}
	if strings.Contains(visible, "abcdefghijklmnopqrstuvwxyz0123456789") || strings.Contains(visible, "utm_source=email") {
		t.Fatalf("expected long query string to be hidden from visible text, got:\n%s", visible)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+longURL+"\x1b\\") {
		t.Fatalf("expected full naked URL to remain in OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_ClosesHyperlinksPerWrappedLine(t *testing.T) {
	longURL := "https://example.com/path?token=abcdefghijklmnopqrstuvwxyz0123456789"
	body := "Before [This is a deliberately long label that must not leave the terminal hyperlink open](" + longURL + ") after"
	lines := RenderEmailBodyLines(body, 24)

	for i, line := range lines {
		if strings.Contains(line, "\x1b]8;;https://") && !strings.Contains(line, "\x1b]8;;\x1b\\") {
			t.Fatalf("line %d opens an OSC 8 hyperlink without closing it: %q", i, line)
		}
		if visibleWidth := ansi.StringWidth(line); visibleWidth > 24 {
			t.Fatalf("line %d visible width=%d exceeds width 24: %q", i, visibleWidth, line)
		}
	}
}

func TestStripTrackers(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			"utm params removed",
			"https://example.com/page?utm_source=email&utm_medium=newsletter&id=42",
			"https://example.com/page?id=42",
		},
		{
			"all tracker params removed leaves clean URL",
			"https://example.com/page?utm_source=email&utm_campaign=test",
			"https://example.com/page",
		},
		{
			"no tracking params unchanged",
			"https://example.com/page?id=42&name=test",
			"https://example.com/page?id=42&name=test",
		},
		{
			"empty query string",
			"https://example.com/page",
			"https://example.com/page",
		},
		{
			"facebook click ID",
			"https://example.com/?fbclid=abc123&real=yes",
			"https://example.com/?real=yes",
		},
		{
			"mailchimp IDs",
			"https://example.com/?mc_cid=abc&mc_eid=def",
			"https://example.com/",
		},
		{
			"case insensitive match",
			"https://example.com/?UTM_SOURCE=email",
			"https://example.com/",
		},
		{
			"hubspot params",
			"https://example.com/path?_hsenc=abc&_hsmi=123&page=1",
			"https://example.com/path?page=1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripTrackers(tt.url)
			if got != tt.want {
				t.Errorf("StripTrackers(%q)\n  got  %q\n  want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestStripTrackersFromText(t *testing.T) {
	text := "Click here: https://example.com/page?utm_source=email&id=42 for more info"
	result := StripTrackersFromText(text)
	if strings.Contains(result, "utm_source") {
		t.Errorf("StripTrackersFromText should remove utm_source: %q", result)
	}
	if !strings.Contains(result, "id=42") {
		t.Errorf("StripTrackersFromText should preserve non-tracker params: %q", result)
	}
	if !strings.Contains(result, "Click here:") || !strings.Contains(result, "for more info") {
		t.Errorf("StripTrackersFromText should preserve surrounding text: %q", result)
	}
}
