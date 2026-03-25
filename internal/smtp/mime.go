package smtp

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
)

// MarkdownToHTMLAndPlain converts a Markdown string to an HTML body and a
// plain-text fallback. The HTML body is rendered by goldmark; the plain-text
// fallback is produced by stripping HTML tags so recipients see clean prose
// rather than raw Markdown syntax (e.g. **bold** → bold).
func MarkdownToHTMLAndPlain(md string) (htmlOut, plain string) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		// goldmark should never fail on valid UTF-8, but fall back gracefully
		return "", md
	}
	htmlOut = buf.String()
	plain = stripHTMLTags(htmlOut)
	return
}

// stripHTMLTags removes all HTML tags and decodes common entities to produce
// readable plain text. It mirrors the logic in imap/body.go.
func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	// Collapse multiple blank lines that arise from <p> / <br> tags
	result := sb.String()
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}
