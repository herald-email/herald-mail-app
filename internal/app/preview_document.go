package app

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"mail-processor/internal/models"
)

type previewDocumentBlockKind int

const (
	previewBlockText previewDocumentBlockKind = iota
	previewBlockInlineImage
)

type previewDocumentBlock struct {
	Kind      previewDocumentBlockKind
	Text      string
	Image     models.InlineImage
	ContentID string
	Alt       string
}

type previewDocument struct {
	Blocks []previewDocumentBlock
}

func buildPreviewDocument(body *models.EmailBody, imageDescriptions map[string]string) previewDocument {
	if body == nil {
		return previewDocument{Blocks: []previewDocumentBlock{{Kind: previewBlockText, Text: "(No content)"}}}
	}

	imagesByCID := mapInlineImagesByCID(body.InlineImages)
	placed := make(map[string]bool)
	blocks, ok := buildPreviewDocumentFromHTML(body.TextHTML, imagesByCID, placed)
	if !ok {
		text := strings.TrimSpace(stripInvisibleChars(body.TextPlain))
		if text == "" {
			text = "(No plain text - HTML only)"
		}
		blocks = append(blocks, previewDocumentBlock{Kind: previewBlockText, Text: text})
	}

	orphanBlocks := orphanInlineImageBlocks(body.InlineImages, placed)
	for i := range orphanBlocks {
		if orphanBlocks[i].Kind != previewBlockInlineImage {
			continue
		}
		if imageDescriptions == nil {
			continue
		}
		if desc := imageDescriptions[normalizeContentID(orphanBlocks[i].ContentID)]; desc != "" {
			orphanBlocks[i].Alt = desc
		}
	}
	blocks = append(blocks, orphanBlocks...)

	return previewDocument{Blocks: blocks}
}

func mapInlineImagesByCID(images []models.InlineImage) map[string]models.InlineImage {
	out := make(map[string]models.InlineImage, len(images))
	for _, image := range images {
		cid := normalizeContentID(image.ContentID)
		if cid != "" {
			out[cid] = image
		}
	}
	return out
}

func normalizeContentID(cid string) string {
	cid = strings.TrimSpace(cid)
	cid = strings.TrimPrefix(strings.ToLower(cid), "cid:")
	cid = strings.Trim(cid, "<>")
	if unescaped, err := url.PathUnescape(cid); err == nil {
		cid = unescaped
	}
	return strings.ToLower(strings.TrimSpace(strings.Trim(cid, "<>")))
}

func inlineImageDocumentKey(image models.InlineImage, index int) string {
	if cid := normalizeContentID(image.ContentID); cid != "" {
		return cid
	}
	return fmt.Sprintf("__inline_image_%d", index)
}

func escapeMarkdownLabel(label string) string {
	label = strings.ReplaceAll(label, `\`, `\\`)
	label = strings.ReplaceAll(label, `[`, `\[`)
	label = strings.ReplaceAll(label, `]`, `\]`)
	return label
}

func buildPreviewDocumentFromHTML(htmlText string, imagesByCID map[string]models.InlineImage, placed map[string]bool) ([]previewDocumentBlock, bool) {
	if strings.TrimSpace(htmlText) == "" {
		return nil, false
	}
	if imagesByCID == nil {
		imagesByCID = make(map[string]models.InlineImage)
	}
	if placed == nil {
		placed = make(map[string]bool)
	}

	root, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return nil, false
	}

	builder := previewHTMLBuilder{
		imagesByCID: imagesByCID,
		placed:      placed,
	}
	builder.walk(root)
	builder.flushText()
	return builder.blocks, len(builder.blocks) > 0
}

func attrValue(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func orphanInlineImageBlocks(images []models.InlineImage, placed map[string]bool) []previewDocumentBlock {
	if placed == nil {
		placed = make(map[string]bool)
	}
	var blocks []previewDocumentBlock
	for i, image := range images {
		key := inlineImageDocumentKey(image, i)
		if key == "" || placed[key] {
			continue
		}
		if len(blocks) == 0 {
			blocks = append(blocks, previewDocumentBlock{Kind: previewBlockText, Text: "Inline images:"})
		}
		if normalizeContentID(image.ContentID) == "" {
			image.ContentID = key
		}
		blocks = append(blocks, previewDocumentBlock{
			Kind:      previewBlockInlineImage,
			Image:     image,
			ContentID: key,
		})
		placed[key] = true
	}
	return blocks
}

type previewHTMLBuilder struct {
	blocks      []previewDocumentBlock
	text        strings.Builder
	imagesByCID map[string]models.InlineImage
	placed      map[string]bool
}

func (b *previewHTMLBuilder) walk(n *html.Node) {
	if n == nil {
		return
	}
	if n.Type == html.TextNode {
		b.writeText(n.Data)
		return
	}
	if n.Type != html.ElementNode {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			b.walk(child)
		}
		return
	}

	switch strings.ToLower(n.Data) {
	case "script", "style", "head":
		return
	case "br":
		b.writeText("\n")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		b.flushText()
		text := strings.TrimSpace(collapseWhitespace(nodeText(n)))
		if text != "" {
			level := headingLevel(n.Data)
			b.blocks = append(b.blocks, previewDocumentBlock{Kind: previewBlockText, Text: strings.Repeat("#", level) + " " + text})
		}
	case "p", "div", "section", "article", "blockquote", "ul", "ol", "li":
		b.flushText()
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			b.walk(child)
		}
		b.flushText()
	case "a":
		b.writeAnchor(n)
	case "img":
		b.flushText()
		b.writeImage(n)
	default:
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			b.walk(child)
		}
	}
}

func (b *previewHTMLBuilder) writeText(text string) {
	if strings.TrimSpace(text) == "" {
		b.text.WriteString(" ")
		return
	}
	b.text.WriteString(text)
	b.text.WriteString(" ")
}

func (b *previewHTMLBuilder) flushText() {
	text := strings.TrimSpace(stripInvisibleChars(collapseWhitespace(b.text.String())))
	b.text.Reset()
	if text != "" {
		b.blocks = append(b.blocks, previewDocumentBlock{Kind: previewBlockText, Text: text})
	}
}

func (b *previewHTMLBuilder) writeAnchor(n *html.Node) {
	if nodeContainsElement(n, "img") {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			b.walk(child)
		}
		return
	}

	href := strings.TrimSpace(attrValue(n, "href"))
	if href == "" || strings.HasPrefix(strings.ToLower(href), "javascript:") {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			b.walk(child)
		}
		return
	}
	label := strings.TrimSpace(collapseWhitespace(nodeText(n)))
	if label == "" {
		label = href
	}
	b.writeText("[" + escapeMarkdownLabel(label) + "](" + href + ")")
}

func (b *previewHTMLBuilder) writeImage(n *html.Node) {
	src := strings.TrimSpace(attrValue(n, "src"))
	alt := strings.TrimSpace(attrValue(n, "alt"))
	if src == "" {
		return
	}
	if strings.HasPrefix(strings.ToLower(src), "cid:") {
		cid := normalizeContentID(src)
		if image, ok := b.imagesByCID[cid]; ok {
			b.blocks = append(b.blocks, previewDocumentBlock{
				Kind:      previewBlockInlineImage,
				Image:     image,
				ContentID: cid,
				Alt:       alt,
			})
			b.placed[cid] = true
			return
		}
		b.blocks = append(b.blocks, previewDocumentBlock{Kind: previewBlockText, Text: "[missing inline image: " + cid + "]"})
		return
	}
	if isRemoteImageURL(src) {
		b.blocks = append(b.blocks, previewDocumentBlock{Kind: previewBlockText, Text: "![" + escapeMarkdownLabel(alt) + "](" + src + ")"})
	}
}

func headingLevel(tag string) int {
	switch strings.ToLower(tag) {
	case "h1":
		return 1
	case "h2":
		return 2
	default:
		return 3
	}
}

func nodeText(n *html.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "script", "style", "head":
			return ""
		}
	}
	var parts []string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if text := nodeText(child); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func nodeContainsElement(n *html.Node, tag string) bool {
	if n == nil {
		return false
	}
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, tag) {
		return true
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if nodeContainsElement(child, tag) {
			return true
		}
	}
	return false
}

func collapseWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func isRemoteImageURL(src string) bool {
	lower := strings.ToLower(src)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
