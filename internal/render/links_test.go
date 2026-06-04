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

func TestOSC8TargetAtVisibleColumn(t *testing.T) {
	line := "see " + TerminalHyperlink("example", "https://example.com/path") + " today"

	for _, col := range []int{4, 7, 10} {
		got, ok := OSC8TargetAtVisibleColumn(line, col)
		if !ok || got != "https://example.com/path" {
			t.Fatalf("column %d target=%q ok=%v, want example target", col, got, ok)
		}
	}
	for _, col := range []int{0, 3, 11, 12} {
		if got, ok := OSC8TargetAtVisibleColumn(line, col); ok {
			t.Fatalf("column %d unexpectedly hit target %q", col, got)
		}
	}
}

func TestOSC8TargetAtVisibleColumnHandlesWideRunes(t *testing.T) {
	line := "a " + TerminalHyperlink("界", "https://wide.example") + " z"

	for _, col := range []int{2, 3} {
		got, ok := OSC8TargetAtVisibleColumn(line, col)
		if !ok || got != "https://wide.example" {
			t.Fatalf("column %d target=%q ok=%v, want wide target", col, got, ok)
		}
	}
	if got, ok := OSC8TargetAtVisibleColumn(line, 4); ok {
		t.Fatalf("column after wide rune unexpectedly hit target %q", got)
	}
}

func TestOSC8LinkRangeAtVisibleColumn(t *testing.T) {
	line := "see " + TerminalHyperlink("example", "https://example.com/path") + " today"

	link, ok := OSC8LinkRangeAtVisibleColumn(line, 7)
	if !ok {
		t.Fatal("expected link range at visible column")
	}
	if link.Target != "https://example.com/path" {
		t.Fatalf("target=%q, want example target", link.Target)
	}
	if link.StartColumn != 4 || link.EndColumn != 11 {
		t.Fatalf("range=%d..%d, want 4..11", link.StartColumn, link.EndColumn)
	}
}

func TestOSC8LinkRangeAtVisibleColumnHandlesWideRunes(t *testing.T) {
	line := "a " + TerminalHyperlink("界", "https://wide.example") + " z"

	link, ok := OSC8LinkRangeAtVisibleColumn(line, 3)
	if !ok {
		t.Fatal("expected wide-rune link range at visible column")
	}
	if link.Target != "https://wide.example" {
		t.Fatalf("target=%q, want wide target", link.Target)
	}
	if link.StartColumn != 2 || link.EndColumn != 4 {
		t.Fatalf("range=%d..%d, want 2..4", link.StartColumn, link.EndColumn)
	}
	if _, ok := OSC8LinkRangeAtVisibleColumn(line, 4); ok {
		t.Fatal("column after wide-rune link should not be in range")
	}
}

func TestOSC8HoverHighlightPreservesWidthTargetAndANSI(t *testing.T) {
	line := "see " + TerminalHyperlink("\033[3mexample\033[23m", "https://example.com/path") + " today"
	link, ok := OSC8LinkRangeAtVisibleColumn(line, 7)
	if !ok {
		t.Fatal("expected link range at visible column")
	}

	highlighted := HighlightOSC8LinkRange(line, link)
	if ansi.StringWidth(highlighted) != ansi.StringWidth(line) {
		t.Fatalf("highlight changed visible width: got %d want %d", ansi.StringWidth(highlighted), ansi.StringWidth(line))
	}
	if !strings.Contains(highlighted, "\033[1;4;7m") || !strings.Contains(highlighted, "\033[22;24;27m") {
		t.Fatalf("highlight should add width-preserving SGR styling, got %q", highlighted)
	}
	if !strings.Contains(highlighted, "\033[3m") || !strings.Contains(highlighted, "\033[23m") {
		t.Fatalf("highlight should preserve embedded ANSI styling, got %q", highlighted)
	}
	for _, col := range []int{4, 7, 10} {
		got, ok := OSC8TargetAtVisibleColumn(highlighted, col)
		if !ok || got != "https://example.com/path" {
			t.Fatalf("column %d target=%q ok=%v after highlight, want example target", col, got, ok)
		}
	}
}

func TestRenderEmailBodyLines_MarkdownLinksUseAnchorText(t *testing.T) {
	longURL := "https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?utm_source=email&o=eyJmaXJzdF9uYW1lIjoiUm93YW4iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldSbFpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY"
	sanitizedURL := "https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?o=eyJmaXJzdF9uYW1lIjoiUm93YW4iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldSbFpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY"
	lines := RenderEmailBodyLines("Welcome\n\n[Display in your browser]("+longURL+")\n\nHi Rowan", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if !strings.Contains(visible, "Display in your browser") {
		t.Fatalf("expected anchor text to be visible, got:\n%s", visible)
	}
	if strings.Contains(visible, "taskpad.mail.example") || strings.Contains(visible, "eyJmaXJ") {
		t.Fatalf("expected long URL to be hidden from visible text, got:\n%s", visible)
	}
	if strings.Contains(rendered, "utm_source=email") {
		t.Fatalf("expected tracker param to be stripped from OSC 8 target, got raw:\n%q", rendered)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+sanitizedURL+"\x1b\\") {
		t.Fatalf("expected sanitized URL to remain in OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_LabelFollowedByBracketedURLBecomesOneLink(t *testing.T) {
	longURL := "https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?o=eyJmaXJzdF9uYW1lIjoiUm93YW4iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldSbFpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY"
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
	logoURL := "https://taskpad.mail.example/_next/static/media/taskpad-logo.0-dsvhpw__1x7.png?utm_medium=email&id=42#logo"
	sanitizedURL := "https://taskpad.mail.example/_next/static/media/taskpad-logo.0-dsvhpw__1x7.png?id=42#logo"
	lines := RenderEmailBodyLines("![Taskpad logo]("+logoURL+")", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if strings.TrimSpace(visible) != "Taskpad logo" {
		t.Fatalf("expected image alt text as visible link label, got %q", visible)
	}
	if strings.Contains(visible, "taskpad-logo") {
		t.Fatalf("expected image URL to be hidden from visible text, got %q", visible)
	}
	if strings.Contains(rendered, "utm_medium=email") {
		t.Fatalf("expected image tracker param to be stripped from OSC 8 target, got raw:\n%q", rendered)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+sanitizedURL+"\x1b\\") {
		t.Fatalf("expected sanitized logo URL to remain in OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_KeepsPunctuationTightAfterInlineFormatting(t *testing.T) {
	lines := RenderEmailBodyLines("**Budget alert** for *Project Orion*.", 80)
	visible := ansi.Strip(strings.Join(lines, "\n"))
	if visible != "Budget alert for Project Orion." {
		t.Fatalf("expected punctuation to stay attached, got %q", visible)
	}
}

func TestRenderEmailBodyLines_UsesGlamourStyledMarkdown(t *testing.T) {
	lines := RenderEmailBodyLines("# HTML preview quality\n\n**Budget alert** for *Project Orion*.", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if !strings.Contains(visible, "HTML preview quality") {
		t.Fatalf("expected heading text to render, got:\n%s", visible)
	}
	if strings.Contains(visible, "# HTML preview quality") || strings.Contains(visible, "**Budget alert**") || strings.Contains(visible, "*Project Orion*") {
		t.Fatalf("expected markdown markers to be rendered away, got:\n%s", visible)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected Glamour ANSI styling, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_DoesNotForceDarkThemeBodyForeground(t *testing.T) {
	lines := RenderEmailBodyLines("Herald is a terminal email client.", 80)
	rendered := strings.Join(lines, "\n")

	if strings.Contains(rendered, "\x1b[38;5;") {
		t.Fatalf("plain body rendering should inherit the app theme foreground, got hard-coded xterm color in:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_PreservesSignatureDelimiterBlock(t *testing.T) {
	lines := RenderEmailBodyLines("-- \nCheers, \nAnton", 80)
	visible := ansi.Strip(strings.Join(lines, "\n"))
	want := "-- \nCheers, \nAnton"

	if visible != want {
		t.Fatalf("rendered signature block = %q, want %q", visible, want)
	}
	if lines[0] != "-- " {
		t.Fatalf("signature delimiter line = %q, want trailing space preserved", lines[0])
	}
}

func TestRenderEmailBodyLines_PreservesSignatureBlockAfterBody(t *testing.T) {
	lines := RenderEmailBodyLines("Hello Anton,\n\n-- \nCheers,\nAnton", 80)
	visible := ansi.Strip(strings.Join(lines, "\n"))

	if !strings.Contains(visible, "\n\n-- \nCheers,\nAnton") {
		t.Fatalf("expected signature block to stay multiline after body, got:\n%q", visible)
	}
}

func TestRenderEmailBodyLines_NakedLongURLUsesShortLabel(t *testing.T) {
	longURL := "https://example.com/path/to/a/very/long/resource/name/that/would/otherwise/wrap/badly?utm_source=email&token=abcdefghijklmnopqrstuvwxyz0123456789"
	sanitizedURL := "https://example.com/path/to/a/very/long/resource/name/that/would/otherwise/wrap/badly?token=abcdefghijklmnopqrstuvwxyz0123456789"
	lines := RenderEmailBodyLines("Open "+longURL+" today", 80)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if !strings.Contains(visible, "example.com/path/to/a/very/long/resource/name") {
		t.Fatalf("expected shortened domain/path label, got:\n%s", visible)
	}
	if strings.Contains(visible, "abcdefghijklmnopqrstuvwxyz0123456789") || strings.Contains(visible, "utm_source=email") {
		t.Fatalf("expected long query string to be hidden from visible text, got:\n%s", visible)
	}
	if strings.Contains(rendered, "utm_source=email") {
		t.Fatalf("expected tracker param to be stripped from OSC 8 target, got raw:\n%q", rendered)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+sanitizedURL+"\x1b\\") {
		t.Fatalf("expected sanitized naked URL to remain in OSC 8 target, got raw:\n%q", rendered)
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

func TestRenderEmailBodyLines_PreservesLongLinkTargetWhenWrapped(t *testing.T) {
	longURL := "https://example.com/path?token=abcdefghijklmnopqrstuvwxyz0123456789"
	body := "Before [Open dashboard](" + longURL + ") after"
	lines := RenderEmailBodyLines(body, 24)
	rendered := strings.Join(lines, "\n")
	visible := ansi.Strip(rendered)

	if !strings.Contains(visible, "Open dashboard") {
		t.Fatalf("expected link label to remain visible, got:\n%s", visible)
	}
	if strings.Contains(visible, "abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Fatalf("expected URL token to stay hidden from visible text, got:\n%s", visible)
	}
	if !strings.Contains(rendered, "\x1b]8;;"+longURL+"\x1b\\") {
		t.Fatalf("expected original URL to remain as OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_DuplicateVisibleLabelDoesNotStealLink(t *testing.T) {
	linkURL := "https://commons.wikimedia.org/wiki/File:Changing_Landscape.jpg"
	body := "Changing Landscape: 960px JPEG thumbnail. Source: [Changing Landscape](" + linkURL + ")"
	lines := RenderEmailBodyLines(body, 120)
	rendered := strings.Join(lines, "\n")
	sourceIndex := strings.Index(rendered, "Source:")
	targetIndex := strings.LastIndex(rendered, "\x1b]8;;"+linkURL+"\x1b\\")

	if sourceIndex < 0 {
		t.Fatalf("expected rendered text to contain Source:, got raw:\n%q", rendered)
	}
	if targetIndex < sourceIndex {
		t.Fatalf("expected source label to receive OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestRenderEmailBodyLines_DoesNotEmitOversizedGlamourLines(t *testing.T) {
	body := "This demo email includes four embedded inline images with different dimensions so you can test Herald's split preview hint, full-screen image rendering, and non-iTerm local image fallback links without fetching media at runtime."
	const width = 218
	lines := RenderEmailBodyLines(body, width)
	visible := ansi.Strip(strings.Join(lines, "\n"))
	visibleFlow := strings.Join(strings.Fields(visible), " ")

	if !strings.Contains(visibleFlow, "at runtime") {
		t.Fatalf("expected wrapped text to preserve phrase %q, got:\n%s", "at runtime", visible)
	}
	for i, line := range lines {
		if strings.TrimSpace(ansi.Strip(line)) == "at" {
			t.Fatalf("line %d split a short phrase awkwardly:\n%s", i, visible)
		}
		if visibleWidth := ansi.StringWidth(line); visibleWidth > width {
			t.Fatalf("line %d visible width=%d exceeds width %d: %q", i, visibleWidth, width, line)
		}
	}
}

func TestRenderEmailBodyLines_ReflowsPaddedPlaintextFallback(t *testing.T) {
	body := strings.Join([]string{
		"        KRYSTAL BARTLETT HAS INVITED",
		"        YOU TO ATTEND: DOORDASH",
		"        ROUND 1 - ZOOM INTERVIEW",
		"        ANTON GOLUBTSOV HI ANTON I'M",
		"        FOLLOWING UP HERE TO REQUEST",
		"        YOUR AVAILABILITY FOR THE",
		"        TECHNICAL INTERVIEWS THE",
		"        NEXT STEPS IN THE PROCESS",
		"        WILL BE A 1-HOUR TECHNICAL",
		"        CODING CHALLENGE IN ADDITION",
		"        TO A 45-MIN PROJECT DEEP",
		"        DIVE.",
	}, "\n")

	lines := RenderEmailBodyLines(body, 88)
	visible := ansi.Strip(strings.Join(lines, "\n"))
	visibleFlow := strings.Join(strings.Fields(visible), " ")

	if !strings.Contains(visibleFlow, "KRYSTAL BARTLETT HAS INVITED YOU TO ATTEND: DOORDASH ROUND 1 - ZOOM INTERVIEW") {
		t.Fatalf("expected padded fallback prose to be joined into readable flow, got:\n%s", visible)
	}
	if got := nonEmptyLineCount(visible); got > 5 {
		t.Fatalf("expected hard-wrapped fallback to reflow into a few lines, got %d lines:\n%s", got, visible)
	}
	for i, line := range strings.Split(visible, "\n") {
		if strings.HasPrefix(line, "        ") {
			t.Fatalf("line %d retained sender padding:\n%s", i, visible)
		}
	}
}

func TestReflowPaddedPlaintextFallbackKeepsSentenceBreaks(t *testing.T) {
	body := strings.Join([]string{
		"        THIS NOTICE IS NOT PROOF OF",
		"        PERMISSION TO TRAVEL TO THE UK.",
		"        YOU HAVE BEEN GRANTED ENTRY",
		"        CLEARANCE TO THE UK AS VISIT",
		"        FROM 5 JUNE 2026 UNTIL 5",
		"        DECEMBER 2026.",
		"        IT IS IMPORTANT TO CHECK THAT",
		"        YOUR DETAILS ABOVE AND ON YOUR",
		"        EVISA ARE CORRECT BEFORE YOU",
		"        TRAVEL.",
	}, "\n")

	got := reflowPaddedPlaintextFallback(body)

	for _, want := range []string{
		"THIS NOTICE IS NOT PROOF OF PERMISSION TO TRAVEL TO THE UK.",
		"YOU HAVE BEEN GRANTED ENTRY CLEARANCE TO THE UK AS VISIT FROM 5 JUNE 2026 UNTIL 5 DECEMBER 2026.",
		"IT IS IMPORTANT TO CHECK THAT YOUR DETAILS ABOVE AND ON YOUR EVISA ARE CORRECT BEFORE YOU TRAVEL.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected reflowed body to contain %q, got:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "UK.\n\nYOU HAVE") || !strings.Contains(got, "2026.\n\nIT IS IMPORTANT") {
		t.Fatalf("expected sentence boundaries to remain paragraph breaks, got:\n%s", got)
	}
}

func TestRenderEmailBodyLines_PreservesIntentionalCodeBlock(t *testing.T) {
	body := "Config example:\n\n    smtp:\n      host: 127.0.0.1\n      port: 1025\n\nDone."

	lines := RenderEmailBodyLines(body, 80)
	visible := ansi.Strip(strings.Join(lines, "\n"))

	if !strings.Contains(visible, "\nsmtp:\n") && !strings.Contains(visible, "\n  smtp:\n") {
		t.Fatalf("expected indented code-like block to remain intact, got:\n%s", visible)
	}
	if !strings.Contains(visible, "host: 127.0.0.1\n") || !strings.Contains(visible, "port: 1025") {
		t.Fatalf("expected config lines to remain separate, got:\n%s", visible)
	}
}

func nonEmptyLineCount(s string) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
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

func TestSanitizePreviewURLTargetStripsTrackersOnly(t *testing.T) {
	raw := "https://example.com/path?utm_source=email&id=42&token=abc#frag"
	want := "https://example.com/path?id=42&token=abc#frag"
	if got := SanitizePreviewURLTarget(raw); got != want {
		t.Fatalf("SanitizePreviewURLTarget(%q) = %q, want %q", raw, got, want)
	}

	invalid := "https://%"
	if got := SanitizePreviewURLTarget(invalid); got != invalid {
		t.Fatalf("invalid URL should be unchanged, got %q", got)
	}
}
