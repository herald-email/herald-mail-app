// Package render provides reusable text processing and terminal rendering
// utilities for email content. It has no dependency on the TUI app layer,
// so it can be used — and tested — independently in the TUI, MCP server,
// daemon, and SSH modes.
package render

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Truncate shortens s to at most n runes, appending "…" if truncated.
func Truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// SanitizeText removes emoji and symbols while preserving all language text.
func SanitizeText(text string) string {
	var result strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsPunct(r) || unicode.IsSpace(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(result.String()), " ")
}

// StripInvisibleChars removes zero-width and formatting Unicode characters
// (U+200B, U+FEFF, U+034F, etc.) that appear as visible noise in terminal
// output. Regular whitespace (space, tab, newline) is preserved.
func StripInvisibleChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == ' ':
			b.WriteRune(r)
		case unicode.Is(unicode.Cf, r):
			// skip format characters (zero-width, BOM, etc.)
		case r == '\u034f':
			// skip COMBINING GRAPHEME JOINER — used as invisible spacer in HTML email
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CalculateTextWidth estimates the visual width of text with emojis.
func CalculateTextWidth(text string) int {
	width := 0
	for _, r := range text {
		if r > 127 {
			width += 2
		} else {
			width += 1
		}
	}
	return width
}

// WrapLines splits text into paragraphs on newlines, collapses consecutive
// blank lines, and wraps each paragraph to fit within width visible columns.
func WrapLines(text string, width int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n\t ")

	var result []string
	consecutiveBlanks := 0
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimRight(para, " \t")
		if para == "" {
			consecutiveBlanks++
			if consecutiveBlanks <= 1 {
				result = append(result, "")
			}
			continue
		}
		consecutiveBlanks = 0
		result = append(result, WrapText(para, width)...)
	}
	return result
}

// WrapText wraps a single paragraph to fit within width visible columns.
// ANSI escape sequences (OSC 8 hyperlinks, SGR styling) are not counted
// toward visible width.
func WrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	runes := []rune(text)
	for len(runes) > 0 {
		if ansi.StringWidth(string(runes)) <= width {
			lines = append(lines, string(runes))
			break
		}
		visW := 0
		cut := 0
		for cut < len(runes) {
			if runes[cut] == '\033' {
				seqEnd := SkipEscapeSeq(runes, cut)
				cut = seqEnd
				continue
			}
			rw := ansi.StringWidth(string(runes[cut : cut+1]))
			if visW+rw > width {
				break
			}
			visW += rw
			cut++
		}
		bestCut := cut
		for bestCut > 0 && runes[bestCut-1] != ' ' {
			bestCut--
		}
		if bestCut == 0 {
			bestCut = cut
		}
		if bestCut == 0 && cut == 0 {
			bestCut = 1
		}
		lines = append(lines, string(runes[:bestCut]))
		rest := runes[bestCut:]
		for len(rest) > 0 && rest[0] == ' ' {
			rest = rest[1:]
		}
		runes = rest
	}
	return lines
}

// SkipEscapeSeq advances past an escape sequence starting at runes[pos].
// Handles OSC (ESC ]) and CSI (ESC [) sequences.
func SkipEscapeSeq(runes []rune, pos int) int {
	if pos >= len(runes) || runes[pos] != '\033' {
		return pos + 1
	}
	pos++ // skip ESC
	if pos >= len(runes) {
		return pos
	}
	switch runes[pos] {
	case ']': // OSC sequence — terminated by ST (ESC \) or BEL (\a)
		pos++
		for pos < len(runes) {
			if runes[pos] == '\a' {
				return pos + 1
			}
			if runes[pos] == '\033' && pos+1 < len(runes) && runes[pos+1] == '\\' {
				return pos + 2
			}
			pos++
		}
	case '[': // CSI sequence — terminated by a letter (A-Z, a-z)
		pos++
		for pos < len(runes) {
			if (runes[pos] >= 'A' && runes[pos] <= 'Z') || (runes[pos] >= 'a' && runes[pos] <= 'z') {
				return pos + 1
			}
			pos++
		}
	default:
		// Unknown sequence type, just skip the ESC
	}
	return pos
}
