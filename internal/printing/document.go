package printing

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/models"
	emailrender "github.com/herald-email/herald-mail-app/internal/render"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
)

func BuildHTMLDocument(req Request) (string, error) {
	if req.Email == nil {
		return "", fmt.Errorf("print email metadata is missing")
	}
	if req.Body == nil {
		return "", errMissingPrintBody
	}

	req.Mode = normalizeMode(req.Mode)
	theme := printThemeForRequest(req)
	bodyHTML, err := printableBodyHTML(req)
	if err != nil {
		return "", err
	}
	bodyHTML = sanitizePrintableHTML(bodyHTML, req.Body, req.AllowRemoteImages)

	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\">")
	b.WriteString(printContentSecurityPolicy(req.AllowRemoteImages))
	b.WriteString("<title>")
	b.WriteString(escaped(requestTitle(req)))
	b.WriteString("</title>")
	b.WriteString(printStyles(theme))
	b.WriteString("</head><body data-print-theme=\"")
	b.WriteString(string(theme))
	b.WriteString("\"><article class=\"email-print\">")
	b.WriteString(printHeader(req))
	b.WriteString("<main class=\"message-body\">")
	b.WriteString(bodyHTML)
	b.WriteString("</main>")
	b.WriteString(printAttachments(req))
	b.WriteString("</article></body></html>")
	return b.String(), nil
}

func printContentSecurityPolicy(allowRemoteImages bool) string {
	imgSrc := "data:"
	if allowRemoteImages {
		imgSrc = "data: https: http:"
	}
	return `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ` + imgSrc + `; style-src 'unsafe-inline'; font-src data:;">`
}

func printableBodyHTML(req Request) (string, error) {
	switch req.Mode {
	case ModeRenderedMarkdown:
		md := emailrender.EmailBodyMarkdownWithCIDImages(req.Body)
		if strings.TrimSpace(md) == "" {
			md = "(No text content)"
		}
		return markdownToPrintHTML(md)
	case ModeOriginalVisual:
		fallthrough
	default:
		return appsmtp.PreparePreservedHTML(req.Body.TextHTML, req.Body.TextPlain, models.PreservationModeSafe), nil
	}
}

func markdownToPrintHTML(md string) (string, error) {
	var buf bytes.Buffer
	markdown := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRendererOptions(goldmarkhtml.WithHardWraps(), goldmarkhtml.WithUnsafe()),
	)
	if err := markdown.Convert([]byte(md), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func printHeader(req Request) string {
	rows := []struct {
		label string
		value string
	}{
		{"From", messageFrom(req)},
		{"To", fieldFromBody(req, "to")},
		{"Cc", fieldFromBody(req, "cc")},
		{"Date", messageDate(req)},
		{"Subject", messageSubject(req)},
	}

	var b strings.Builder
	b.WriteString("<header class=\"message-header\"><h1>")
	if subject := messageSubject(req); strings.TrimSpace(subject) != "" {
		b.WriteString(escaped(subject))
	} else {
		b.WriteString("Email")
	}
	b.WriteString("</h1><dl>")
	for _, row := range rows {
		if strings.TrimSpace(row.value) == "" {
			continue
		}
		b.WriteString("<div><dt>")
		b.WriteString(escaped(row.label))
		b.WriteString("</dt><dd>")
		b.WriteString(escaped(row.value))
		b.WriteString("</dd></div>")
	}
	b.WriteString("</dl></header>")
	return b.String()
}

func fieldFromBody(req Request, name string) string {
	if req.Body == nil {
		return ""
	}
	switch name {
	case "to":
		return strings.TrimSpace(req.Body.To)
	case "cc":
		return strings.TrimSpace(req.Body.CC)
	default:
		return ""
	}
}

func printAttachments(req Request) string {
	if req.Body == nil || len(req.Body.Attachments) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<section class=\"attachments\"><h2>Attachments</h2><ul>")
	for _, att := range req.Body.Attachments {
		b.WriteString("<li><span class=\"attachment-name\">")
		b.WriteString(escaped(att.Filename))
		b.WriteString("</span><span>")
		b.WriteString(escaped(att.MIMEType))
		b.WriteString("</span><span>")
		b.WriteString(formatBytes(att.Size))
		b.WriteString("</span></li>")
	}
	b.WriteString("</ul></section>")
	return b.String()
}

func formatBytes(n int) string {
	if n < 0 {
		n = 0
	}
	const kib = 1024
	const mib = kib * 1024
	switch {
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func printStyles(theme Theme) string {
	return `<style>
html { color-scheme: light; }
body {
  --print-bg: #fff;
  --print-fg: #171717;
  --print-dim: #5f6368;
  --print-rule: #d8d8d8;
  --print-soft-rule: #eeeeee;
  --print-accent: #2f5d50;
  --print-code-bg: #f6f8fa;
  --print-max-width: 760px;
  --print-padding: 36px 40px;
  --print-body-font: 13px/1.45 -apple-system, BlinkMacSystemFont, "Helvetica Neue", Arial, sans-serif;
  --print-heading-font: -apple-system, BlinkMacSystemFont, "Helvetica Neue", Arial, sans-serif;
  --print-mono-font: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  margin: 0;
  color: var(--print-fg);
  background: var(--print-bg);
  font: var(--print-body-font);
}
` + selectedPrintThemeCSS(theme) + `
.email-print { max-width: var(--print-max-width); margin: 0 auto; padding: var(--print-padding); }
.message-header { border-bottom: 1px solid var(--print-rule); margin-bottom: 24px; padding-bottom: 16px; }
h1, h2, h3, h4, h5, h6 { font-family: var(--print-heading-font); }
h1 { margin: 0 0 14px; font-size: 22px; line-height: 1.25; font-weight: 650; }
h2 { margin: 24px 0 8px; font-size: 15px; }
dl { margin: 0; display: grid; grid-template-columns: max-content 1fr; gap: 6px 14px; }
dl div { display: contents; }
dt { color: var(--print-dim); font-weight: 650; }
dd { margin: 0; overflow-wrap: anywhere; }
.message-body { overflow-wrap: anywhere; }
.message-body img { max-width: min(100%, 560px); max-height: 520px; width: auto; height: auto; object-fit: contain; }
body:not([data-print-theme="original"]) .message-body img { display: block; margin: 0.6em 0 0.35em; }
pre, code { font-family: var(--print-mono-font); }
pre { white-space: pre-wrap; overflow-wrap: anywhere; background: var(--print-code-bg); padding: 10px 12px; }
code { background: var(--print-code-bg); padding: 0 3px; }
blockquote { border-left: 3px solid var(--print-rule); margin-left: 0; padding-left: 12px; color: var(--print-dim); }
.remote-image-placeholder { display: block; border: 1px dashed var(--print-rule); color: var(--print-dim); padding: 8px 10px; margin: 8px 0; background: var(--print-code-bg); }
.remote-image-placeholder strong { color: var(--print-fg); }
table { border-collapse: collapse; max-width: 100%; }
td, th { border: 1px solid var(--print-rule); padding: 4px 6px; vertical-align: top; }
th { background: var(--print-code-bg); font-weight: 650; }
a { color: var(--print-accent); }
.attachments { border-top: 1px solid var(--print-rule); margin-top: 24px; padding-top: 10px; }
.attachments ul { list-style: none; margin: 0; padding: 0; }
.attachments li { display: flex; gap: 12px; padding: 4px 0; border-bottom: 1px solid var(--print-soft-rule); }
.attachment-name { font-weight: 650; flex: 1; }
@media print { .email-print { padding: 0; } a { color: inherit; text-decoration: none; } }
</style>`
}

func selectedPrintThemeCSS(theme Theme) string {
	if theme == ThemeOriginal {
		return ""
	}
	switch normalizeTheme(theme) {
	case ThemeGitHub:
		return `body[data-print-theme="github"] {
  --print-fg: #24292f;
  --print-dim: #57606a;
  --print-rule: #d0d7de;
  --print-soft-rule: #d8dee4;
  --print-accent: #0969da;
  --print-code-bg: #f6f8fa;
  --print-max-width: 820px;
  --print-body-font: 13px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  --print-heading-font: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
}
body[data-print-theme="github"] .email-print { border: 1px solid #d0d7de; border-radius: 6px; }
body[data-print-theme="github"] h1 { border-bottom: 1px solid var(--print-rule); padding-bottom: 0.3em; }
body[data-print-theme="github"] h2 { border-bottom: 1px solid var(--print-soft-rule); padding-bottom: 0.25em; }`
	case ThemeManuscript:
		return `body[data-print-theme="manuscript"] {
  --print-fg: #1f1b16;
  --print-dim: #6a5d4f;
  --print-rule: #c9bda8;
  --print-soft-rule: #e8dfd2;
  --print-accent: #7b3f2a;
  --print-max-width: 680px;
  --print-padding: 42px 52px;
  --print-body-font: 14px/1.68 Georgia, "Times New Roman", serif;
  --print-heading-font: Georgia, "Times New Roman", serif;
}
body[data-print-theme="manuscript"] h1 { text-align: center; font-size: 24px; font-weight: 500; }
body[data-print-theme="manuscript"] .message-header { text-align: left; border-bottom-style: double; }
body[data-print-theme="manuscript"] .message-body p { margin: 0 0 0.9em; text-indent: 1.15em; }
body[data-print-theme="manuscript"] .message-body p:first-child { text-indent: 0; }`
	case ThemeAcademic:
		return `body[data-print-theme="academic"] {
  --print-fg: #161a19;
  --print-dim: #59635f;
  --print-rule: #b9c7c0;
  --print-soft-rule: #dde5e1;
  --print-accent: #2f5d50;
  --print-max-width: 720px;
  --print-padding: 40px 46px;
  --print-body-font: 13.5px/1.58 "Crimson Text", Georgia, "Times New Roman", serif;
  --print-heading-font: "Avenir Next", -apple-system, BlinkMacSystemFont, "Helvetica Neue", Arial, sans-serif;
}
body[data-print-theme="academic"] h1 { color: #183d34; font-size: 21px; letter-spacing: 0.02em; }
body[data-print-theme="academic"] .message-header { border-top: 4px double #2f5d50; padding-top: 14px; }
body[data-print-theme="academic"] blockquote { border-left: 4px solid #2f5d50; }
body[data-print-theme="academic"] table { font-size: 12.5px; }`
	case ThemeSwiss:
		fallthrough
	default:
		return `body[data-print-theme="swiss"] {
  --print-fg: #111;
  --print-dim: #555;
  --print-rule: #111;
  --print-soft-rule: #d7d7d7;
  --print-accent: #111;
  --print-body-font: 13px/1.5 -apple-system, BlinkMacSystemFont, "Helvetica Neue", Arial, sans-serif;
  --print-heading-font: "Helvetica Neue", Arial, sans-serif;
}
body[data-print-theme="swiss"] h1 { text-transform: uppercase; letter-spacing: 0; font-size: 20px; }
body[data-print-theme="swiss"] .message-header { border-bottom-width: 2px; }
body[data-print-theme="swiss"] h2 { text-transform: uppercase; letter-spacing: 0.03em; }`
	}
}

func sanitizePrintableHTML(fragment string, body *models.EmailBody, allowRemoteImages bool) string {
	return sanitizePrintableHTMLFragment(fragment, inlineImageDataURIs(body), allowRemoteImages)
}

func sanitizePrintableHTMLFragment(fragment string, images map[string]string, allowRemoteImages bool) string {
	doc, err := html.Parse(strings.NewReader("<!doctype html><html><body>" + fragment + "</body></html>"))
	if err != nil {
		return `<pre style="white-space:pre-wrap">` + escaped(fragment) + `</pre>`
	}
	body := findBody(doc)
	if body == nil {
		body = doc
	}
	sanitizeNode(body, images, allowRemoteImages)
	return renderChildren(body)
}

func findBody(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "body") {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findBody(c); found != nil {
			return found
		}
	}
	return nil
}

func sanitizeNode(n *html.Node, images map[string]string, allowRemoteImages bool) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if shouldDropPrintNode(c) {
			n.RemoveChild(c)
			c = next
			continue
		}
		if c.Type == html.ElementNode {
			if !allowRemoteImages {
				if placeholder := remoteImagePlaceholder(c); placeholder != nil {
					n.InsertBefore(placeholder, c)
					n.RemoveChild(c)
					c = next
					continue
				}
			}
			c.Attr = sanitizeAttrs(c.Data, c.Attr, images, allowRemoteImages)
		}
		sanitizeNode(c, images, allowRemoteImages)
		c = next
	}
}

func shouldDropPrintNode(n *html.Node) bool {
	if n.Type == html.CommentNode {
		return true
	}
	if n.Type != html.ElementNode {
		return false
	}
	switch strings.ToLower(n.Data) {
	case "script", "iframe", "object", "embed", "form", "input", "button", "textarea", "select", "option", "meta", "base", "link", "style":
		return true
	default:
		return false
	}
}

func sanitizeAttrs(tag string, attrs []html.Attribute, images map[string]string, allowRemoteImages bool) []html.Attribute {
	tag = strings.ToLower(tag)
	out := attrs[:0]
	for _, attr := range attrs {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		val := strings.TrimSpace(attr.Val)
		switch {
		case key == "":
			continue
		case strings.HasPrefix(key, "on"):
			continue
		case key == "srcdoc" || key == "srcset":
			continue
		case key == "style" && unsafeStyle(val):
			continue
		case key == "href":
			clean, ok := sanitizeHref(val)
			if !ok {
				continue
			}
			attr.Val = clean
		case key == "src" && tag == "img":
			clean, ok := sanitizeImageSrc(val, images, allowRemoteImages)
			if !ok {
				continue
			}
			attr.Val = clean
		case key == "background" || key == "poster" || key == "src":
			continue
		}
		attr.Key = key
		out = append(out, attr)
	}
	return out
}

func remoteImagePlaceholder(n *html.Node) *html.Node {
	if n == nil || n.Type != html.ElementNode || !strings.EqualFold(n.Data, "img") {
		return nil
	}
	raw := attrValue(n, "src")
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil
	}
	clean := emailrender.SanitizePreviewURLTarget(raw)
	label := strings.TrimSpace(attrValue(n, "alt"))
	if label == "" {
		label = strings.TrimSpace(attrValue(n, "title"))
	}
	if label == "" {
		label = "remote image"
	}

	span := &html.Node{
		Type: html.ElementNode,
		Data: "span",
		Attr: []html.Attribute{{Key: "class", Val: "remote-image-placeholder"}},
	}
	strong := &html.Node{Type: html.ElementNode, Data: "strong"}
	strong.AppendChild(&html.Node{Type: html.TextNode, Data: "Remote image blocked"})
	span.AppendChild(strong)
	span.AppendChild(&html.Node{Type: html.TextNode, Data: ": " + label + " ("})
	link := &html.Node{
		Type: html.ElementNode,
		Data: "a",
		Attr: []html.Attribute{{Key: "href", Val: clean}},
	}
	link.AppendChild(&html.Node{Type: html.TextNode, Data: "source"})
	span.AppendChild(link)
	span.AppendChild(&html.Node{Type: html.TextNode, Data: ")"})
	return span
}

func attrValue(n *html.Node, name string) string {
	if n == nil {
		return ""
	}
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, name) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func unsafeStyle(style string) bool {
	style = strings.ToLower(style)
	return strings.Contains(style, "url(") ||
		strings.Contains(style, "@import") ||
		strings.Contains(style, "expression(") ||
		strings.Contains(style, "behavior:")
}

func sanitizeHref(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "", "http", "https", "mailto", "tel":
		if scheme == "http" || scheme == "https" {
			return emailrender.SanitizePreviewURLTarget(raw), true
		}
		return raw, true
	default:
		return "", false
	}
}

func sanitizeImageSrc(raw string, images map[string]string, allowRemoteImages bool) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", false
	}
	if cid := normalizeCID(raw); cid != "" {
		dataURI := images[cid]
		return dataURI, dataURI != ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "data":
		return raw, strings.HasPrefix(strings.ToLower(raw), "data:image/")
	case "http", "https":
		if allowRemoteImages {
			return emailrender.SanitizePreviewURLTarget(raw), true
		}
		return "", false
	default:
		return "", false
	}
}

func normalizeCID(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "cid:") {
		return ""
	}
	raw = strings.TrimSpace(raw[4:])
	if unescaped, err := url.QueryUnescape(raw); err == nil {
		raw = unescaped
	}
	raw = strings.Trim(raw, "<>")
	return strings.ToLower(strings.TrimSpace(raw))
}

func inlineImageDataURIs(body *models.EmailBody) map[string]string {
	images := make(map[string]string)
	if body == nil {
		return images
	}
	for _, image := range body.InlineImages {
		if len(image.Data) == 0 {
			continue
		}
		cid := strings.ToLower(strings.Trim(strings.TrimSpace(image.ContentID), "<>"))
		if cid == "" {
			continue
		}
		images[cid] = dataURI(image.MIMEType, image.Data)
	}
	return images
}

func renderChildren(n *html.Node) string {
	var b bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return strings.TrimSpace(b.String())
}

func dataURI(mime string, data []byte) string {
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}
