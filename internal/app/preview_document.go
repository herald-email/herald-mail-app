package app

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"mail-processor/internal/models"
	emailrender "mail-processor/internal/render"
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
		text := strings.TrimSpace(stripInvisibleChars(emailrender.EmailBodyMarkdown(body)))
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
	builder.flushHTML()
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
	html        strings.Builder
	imagesByCID map[string]models.InlineImage
	placed      map[string]bool
}

func (b *previewHTMLBuilder) walk(n *html.Node) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "img") && isCIDImageNode(n) {
		b.flushHTML()
		b.writeImage(n)
		return
	}

	if n.Type == html.DocumentNode || nodeContainsCIDImage(n) {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			b.walk(child)
		}
		return
	}

	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "script", "style", "head":
			return
		}
	}

	b.writeHTMLNode(n)
	if n.Type == html.ElementNode && isPreviewMarkdownBlockElement(n.Data) {
		b.flushHTML()
	}
}

func (b *previewHTMLBuilder) writeHTMLNode(n *html.Node) {
	var buf bytes.Buffer
	if err := html.Render(&buf, n); err != nil {
		return
	}
	b.html.Write(buf.Bytes())
}

func (b *previewHTMLBuilder) flushHTML() {
	markdown := strings.TrimSpace(stripInvisibleChars(emailrender.HTMLToMarkdown(b.html.String())))
	b.html.Reset()
	if markdown != "" {
		b.blocks = append(b.blocks, previewDocumentBlock{Kind: previewBlockText, Text: markdown})
	}
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
}

func isCIDImageNode(n *html.Node) bool {
	return n != nil &&
		n.Type == html.ElementNode &&
		strings.EqualFold(n.Data, "img") &&
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(attrValue(n, "src"))), "cid:")
}

func nodeContainsCIDImage(n *html.Node) bool {
	if n == nil {
		return false
	}
	if isCIDImageNode(n) {
		return true
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if nodeContainsCIDImage(child) {
			return true
		}
	}
	return false
}

func isPreviewMarkdownBlockElement(tag string) bool {
	switch strings.ToLower(tag) {
	case "h1", "h2", "h3", "h4", "h5", "h6",
		"p", "div", "section", "article", "blockquote",
		"ul", "ol", "table", "tr":
		return true
	default:
		return false
	}
}
