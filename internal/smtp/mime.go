package smtp

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yuin/goldmark"
)

// InlineImage holds a resolved inline image to embed in the outgoing email.
type InlineImage struct {
	ContentID   string // e.g. "img001@herald"
	MIMEType    string // e.g. "image/png"
	Data        []byte
	AltText     string
	OriginalRef string // the original src path reference
}

// imgTagRe matches <img ...src="..."> tags to extract src attributes.
var imgTagRe = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)

// BuildInlineImages scans a markdown body for ![alt](localpath) references
// where localpath starts with / or ~/. It converts the markdown to HTML,
// reads each local image file, assigns a Content-ID, and substitutes
// src="path" with src="cid:ContentID" in the HTML.
//
// Returns:
//   - htmlBody: the HTML version with cid: src references substituted
//   - inlines: the list of resolved inline images
//   - err: first file read error encountered, if any
func BuildInlineImages(markdownBody string) (htmlBody string, inlines []InlineImage, err error) {
	htmlBody, _ = MarkdownToHTMLAndPlain(markdownBody)

	matches := imgTagRe.FindAllStringSubmatchIndex(htmlBody, -1)
	if len(matches) == 0 {
		return htmlBody, nil, nil
	}

	// Process matches in reverse order so indices remain valid after replacements.
	inlines = make([]InlineImage, 0)
	// We need a forward pass to assign deterministic Content-IDs, then replace.
	type replacement struct {
		src string
		cid string
	}
	var replacements []replacement
	imgIndex := 0

	for _, match := range matches {
		// match[2]:match[3] is the capture group — the src value
		src := htmlBody[match[2]:match[3]]

		// Only handle local paths (absolute or home-relative)
		if !strings.HasPrefix(src, "/") && !strings.HasPrefix(src, "~/") {
			replacements = append(replacements, replacement{src: src, cid: ""})
			continue
		}

		resolvedPath := src
		if strings.HasPrefix(src, "~/") {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return htmlBody, nil, fmt.Errorf("expand home dir: %w", homeErr)
			}
			resolvedPath = filepath.Join(homeDir, src[2:])
		}

		data, readErr := os.ReadFile(resolvedPath)
		if readErr != nil {
			return htmlBody, nil, fmt.Errorf("read inline image %q: %w", resolvedPath, readErr)
		}

		imgIndex++
		cid := fmt.Sprintf("img%03d@herald", imgIndex)
		mimeType := mimeTypeFromExt(filepath.Ext(resolvedPath))

		inlines = append(inlines, InlineImage{
			ContentID:   cid,
			MIMEType:    mimeType,
			Data:        data,
			OriginalRef: src,
		})
		replacements = append(replacements, replacement{src: src, cid: cid})
	}

	// Substitute src="path" with src="cid:..." in the HTML.
	for _, r := range replacements {
		if r.cid == "" {
			continue
		}
		htmlBody = strings.ReplaceAll(htmlBody,
			fmt.Sprintf(`src="%s"`, r.src),
			fmt.Sprintf(`src="cid:%s"`, r.cid),
		)
	}

	return htmlBody, inlines, nil
}

// mimeTypeFromExt returns the MIME type for common image extensions.
func mimeTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/octet-stream"
	}
}

// BuildDraftMessage builds a raw RFC 2822 message suitable for IMAP APPEND.
// Returns the full message bytes.
func BuildDraftMessage(from, to, subject, body string) ([]byte, error) {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	return []byte(msg.String()), nil
}

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
