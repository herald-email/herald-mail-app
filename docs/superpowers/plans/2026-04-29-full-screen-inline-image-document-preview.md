# Full-Screen Inline Image Document Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a full-screen email preview document stream where text and inline raster images scroll together, with HTML `cid:` images rendered near their authored positions and safe fallbacks for unsupported terminals.

**Architecture:** Add an app-local preview document layer that converts `models.EmailBody` into ordered text/image/link/placeholder blocks, then lay those blocks out into a viewport under the existing pinned preview header. Move graphics-mode selection and image row accounting behind a small renderer API so iTerm2 works now and Kitty/Sixel can be added without changing document semantics.

**Tech Stack:** Go 1.25, Bubble Tea, Lipgloss, `golang.org/x/net/html`, existing `internal/render` link wrapping, existing iTerm2 OSC 1337 renderer, tmux/manual terminal QA.

---

## File Structure

- Create: `internal/app/preview_document.go`
  - Owns preview block types, HTML/plain-text document construction, CID normalization, orphan inline image fallback placement, and remote-image markdown-link blocks.
- Create: `internal/app/preview_document_test.go`
  - Unit tests for HTML `cid:` placement, missing CIDs, remote image links, plain-text fallback, and orphan MIME images.
- Create: `internal/app/preview_image_renderer.go`
  - Owns preview graphics mode selection, image sizing, row accounting, and rendering one image block as iTerm2 raster, local OSC 8 link, placeholder, or off.
- Create: `internal/app/preview_image_renderer_test.go`
  - Unit tests for mode selection, iTerm2 row accounting, hidden trailing newline prevention, empty/oversized image fallback, and deterministic image sizing.
- Create: `internal/app/preview_viewport.go`
  - Owns laying out preview document blocks into logical row spans and rendering the visible full-screen slice for Timeline/Cleanup.
- Create: `internal/app/preview_viewport_test.go`
  - Unit tests for scroll clamping, row budget, image/text ordering, and rendered height.
- Modify: `internal/app/layout.go`
  - Add small cache fields to `TimelineState` for full-screen document layouts.
- Modify: `internal/app/app.go`
  - Add small cache fields to `Model` for Cleanup full-screen document layouts.
- Modify: `internal/app/email_preview.go`
  - Replace full-screen Timeline and Cleanup pre-body image rendering with the preview viewport renderer. Keep split preview compact.
- Modify: `internal/app/image_preview_server.go`
  - Add `ContentID` metadata to local preview links so document-image blocks can map links back to their source image.
- Modify: `internal/app/timeline.go`, `internal/app/key_routing.go`, `internal/app/mouse.go`
  - Clear preview document caches wherever body/wrapped-line caches and image preview links are currently cleared.
- Modify: `internal/demo/fixtures.go`
  - Give the Creative Commons image sampler real HTML with `cid:` image placement while retaining readable plain-text fallback.
- Modify: `internal/demo/fixtures_test.go`, `internal/backend/demo_behavior_test.go`
  - Assert the demo sampler has HTML CID references as well as four inline images.
- Modify: `internal/app/image_preview_test.go`, `internal/app/email_link_rendering_test.go`, `internal/app/layout_regression_test.go`
  - Adjust or add integration tests for the new document stream while preserving split preview behavior and safe link rendering.
- Modify: `TUI_TESTPLAN.md`
  - Expand TC-23A with app-scroll, native scrollback, real-terminal raster screenshots, and protocol-mode evidence.
- Modify: `TUI_TESTING.md`
  - Document that tmux verifies layout/escape output but cannot prove actual raster placement.
- Create: `reports/TEST_REPORT_2026-04-29_full-screen-inline-image-document-preview.md`
  - Record automated tests plus tmux/real-terminal/SSH/MCP surface verification once implementation is complete.

---

### Task 1: Preview Document Builder

**Files:**
- Create: `internal/app/preview_document.go`
- Create: `internal/app/preview_document_test.go`

- [ ] **Step 1: Write failing document builder tests**

Add `internal/app/preview_document_test.go`:

```go
package app

import (
	"strings"
	"testing"

	"mail-processor/internal/models"
)

func blockTexts(blocks []previewDocumentBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, block.Text)
	}
	return out
}

func TestBuildPreviewDocument_HTMLCIDImagesStayInAuthoredOrder(t *testing.T) {
	body := &models.EmailBody{
		TextPlain: "Plain fallback should not win",
		TextHTML: `<html><body>
			<h1>Gallery</h1>
			<p>Before badge.</p>
			<img alt="Badge" src="cid:badge">
			<p>Between images.</p>
			<img alt="Bee" src="cid:bee@local">
			<p>After bee.</p>
		</body></html>`,
		InlineImages: []models.InlineImage{
			{ContentID: "badge", MIMEType: "image/png", Data: []byte("badge")},
			{ContentID: "bee@local", MIMEType: "image/jpeg", Data: []byte("bee")},
		},
	}

	doc := buildPreviewDocument(body, nil)

	if got, want := len(doc.Blocks), 5; got != want {
		t.Fatalf("block count = %d, want %d: %#v", got, want, doc.Blocks)
	}
	if doc.Blocks[0].Kind != previewBlockText || !strings.Contains(doc.Blocks[0].Text, "# Gallery") {
		t.Fatalf("first block should be heading text, got %#v", doc.Blocks[0])
	}
	if doc.Blocks[1].Kind != previewBlockText || !strings.Contains(doc.Blocks[1].Text, "Before badge") {
		t.Fatalf("second block should be text before badge, got %#v", doc.Blocks[1])
	}
	if doc.Blocks[2].Kind != previewBlockInlineImage || doc.Blocks[2].Image.ContentID != "badge" {
		t.Fatalf("third block should be badge image, got %#v", doc.Blocks[2])
	}
	if doc.Blocks[3].Kind != previewBlockText || !strings.Contains(doc.Blocks[3].Text, "Between images") {
		t.Fatalf("fourth block should be text between images, got %#v", doc.Blocks[3])
	}
	if doc.Blocks[4].Kind != previewBlockInlineImage || doc.Blocks[4].Image.ContentID != "bee@local" {
		t.Fatalf("fifth block should be bee image, got %#v", doc.Blocks[4])
	}
}

func TestBuildPreviewDocument_RemoteImagesRemainLinksAndMissingCIDIsPlaceholder(t *testing.T) {
	body := &models.EmailBody{
		TextHTML: `<p>Logo below</p>
			<img alt="Remote logo" src="https://example.test/logo.png">
			<img alt="Missing" src="cid:not-found">`,
	}

	doc := buildPreviewDocument(body, nil)
	plain := strings.Join(blockTexts(doc.Blocks), "\n")

	if !strings.Contains(plain, "Logo below") {
		t.Fatalf("expected surrounding text, got %#v", doc.Blocks)
	}
	if !strings.Contains(plain, "![Remote logo](https://example.test/logo.png)") {
		t.Fatalf("expected remote image markdown link, got %#v", doc.Blocks)
	}
	if !strings.Contains(plain, "[missing inline image: not-found]") {
		t.Fatalf("expected missing CID placeholder, got %#v", doc.Blocks)
	}
}

func TestBuildPreviewDocument_OrphanInlineImagesRenderOnceAfterPlainText(t *testing.T) {
	body := &models.EmailBody{
		TextPlain: "Intro paragraph.\n\nAttribution list.",
		InlineImages: []models.InlineImage{
			{ContentID: "chart", MIMEType: "image/png", Data: []byte("chart")},
		},
	}

	doc := buildPreviewDocument(body, nil)

	if got, want := len(doc.Blocks), 3; got != want {
		t.Fatalf("block count = %d, want %d: %#v", got, want, doc.Blocks)
	}
	if doc.Blocks[0].Kind != previewBlockText || !strings.Contains(doc.Blocks[0].Text, "Intro paragraph") {
		t.Fatalf("first block should be plain text, got %#v", doc.Blocks[0])
	}
	if doc.Blocks[1].Kind != previewBlockText || !strings.Contains(doc.Blocks[1].Text, "Inline images") {
		t.Fatalf("second block should label orphan image section, got %#v", doc.Blocks[1])
	}
	if doc.Blocks[2].Kind != previewBlockInlineImage || doc.Blocks[2].Image.ContentID != "chart" {
		t.Fatalf("third block should be orphan image, got %#v", doc.Blocks[2])
	}
}

func TestBuildPreviewDocument_HTMLPlacedImageIsNotRepeatedAsOrphan(t *testing.T) {
	body := &models.EmailBody{
		TextHTML: `<p>Before</p><img src="cid:chart"><p>After</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "chart", MIMEType: "image/png", Data: []byte("chart")},
		},
	}

	doc := buildPreviewDocument(body, nil)
	imageCount := 0
	for _, block := range doc.Blocks {
		if block.Kind == previewBlockInlineImage {
			imageCount++
		}
	}
	if imageCount != 1 {
		t.Fatalf("placed image should render once, got %d image blocks: %#v", imageCount, doc.Blocks)
	}
}
```

- [ ] **Step 2: Run document builder tests and verify they fail**

Run:

```bash
go test ./internal/app -run 'TestBuildPreviewDocument' -count=1
```

Expected: FAIL with compile errors for `previewDocumentBlock`, `previewBlockText`, `previewBlockInlineImage`, and `buildPreviewDocument`.

- [ ] **Step 3: Implement the document builder**

Create `internal/app/preview_document.go`:

```go
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

func buildPreviewDocument(body *models.EmailBody, descs map[string]string) previewDocument {
	if body == nil {
		return previewDocument{Blocks: []previewDocumentBlock{{Kind: previewBlockText, Text: "(No content)"}}}
	}

	imagesByCID := mapInlineImagesByCID(body.InlineImages)
	placed := map[string]bool{}
	var doc previewDocument

	if strings.TrimSpace(body.TextHTML) != "" {
		if blocks, ok := buildPreviewDocumentFromHTML(body.TextHTML, imagesByCID, placed); ok {
			doc.Blocks = append(doc.Blocks, blocks...)
		}
	}
	if len(doc.Blocks) == 0 {
		text := strings.TrimSpace(stripInvisibleChars(body.TextPlain))
		if text == "" {
			text = "(No plain text - HTML only)"
		}
		doc.Blocks = append(doc.Blocks, previewDocumentBlock{Kind: previewBlockText, Text: text})
	}

	orphanBlocks := orphanInlineImageBlocks(body.InlineImages, placed)
	if len(orphanBlocks) > 0 {
		doc.Blocks = append(doc.Blocks, previewDocumentBlock{Kind: previewBlockText, Text: "Inline images:"})
		doc.Blocks = append(doc.Blocks, orphanBlocks...)
	}

	return doc
}

func mapInlineImagesByCID(images []models.InlineImage) map[string]models.InlineImage {
	out := make(map[string]models.InlineImage, len(images))
	for _, img := range images {
		key := normalizeContentID(img.ContentID)
		if key != "" {
			out[key] = img
		}
	}
	return out
}

func normalizeContentID(cid string) string {
	cid = strings.TrimSpace(cid)
	cid = strings.TrimPrefix(cid, "cid:")
	cid = strings.Trim(cid, "<>")
	if decoded, err := url.QueryUnescape(cid); err == nil {
		cid = decoded
	}
	return strings.ToLower(strings.TrimSpace(cid))
}

func buildPreviewDocumentFromHTML(htmlText string, imagesByCID map[string]models.InlineImage, placed map[string]bool) ([]previewDocumentBlock, bool) {
	root, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return nil, false
	}

	var blocks []previewDocumentBlock
	var text strings.Builder

	flushText := func() {
		clean := strings.TrimSpace(text.String())
		for strings.Contains(clean, "\n\n\n") {
			clean = strings.ReplaceAll(clean, "\n\n\n", "\n\n")
		}
		if clean != "" {
			blocks = append(blocks, previewDocumentBlock{Kind: previewBlockText, Text: clean})
		}
		text.Reset()
	}
	addBreak := func(n int) {
		if text.Len() == 0 {
			return
		}
		if n == 1 {
			text.WriteByte('\n')
			return
		}
		text.WriteString("\n\n")
	}
	writeWords := func(s string) {
		s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\u00a0", " ")), " ")
		if s == "" {
			return
		}
		current := text.String()
		if len(current) > 0 {
			last := current[len(current)-1]
			first := s[0]
			if last != ' ' && last != '\n' && first != '.' && first != ',' && first != ':' && first != ';' && first != '!' && first != '?' {
				text.WriteByte(' ')
			}
		}
		text.WriteString(s)
	}

	var collectText func(*html.Node) string
	collectText = func(n *html.Node) string {
		var sb strings.Builder
		var walk func(*html.Node)
		walk = func(node *html.Node) {
			if node.Type == html.TextNode {
				value := strings.Join(strings.Fields(strings.ReplaceAll(node.Data, "\u00a0", " ")), " ")
				if value != "" {
					if sb.Len() > 0 {
						sb.WriteByte(' ')
					}
					sb.WriteString(value)
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(n)
		return sb.String()
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			writeWords(n.Data)
			return
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			switch tag {
			case "style", "script", "head":
				return
			case "br":
				addBreak(1)
				return
			case "h1":
				flushText()
				text.WriteString("# ")
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				flushText()
				return
			case "h2":
				flushText()
				text.WriteString("## ")
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				flushText()
				return
			case "h3", "h4", "h5", "h6":
				flushText()
				text.WriteString("### ")
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				flushText()
				return
			case "p", "div", "section", "article", "blockquote":
				flushText()
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				flushText()
				return
			case "ul", "ol":
				flushText()
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				flushText()
				return
			case "li":
				flushText()
				text.WriteString("- ")
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				flushText()
				return
			case "a":
				href := attrValue(n, "href")
				label := collectText(n)
				if label != "" && href != "" && href != "#" && !strings.HasPrefix(strings.ToLower(href), "javascript") {
					writeWords(fmt.Sprintf("[%s](%s)", escapeMarkdownLabel(label), href))
				} else {
					writeWords(label)
				}
				return
			case "img":
				flushText()
				src := strings.TrimSpace(attrValue(n, "src"))
				alt := strings.TrimSpace(attrValue(n, "alt"))
				if alt == "" {
					alt = strings.TrimSpace(attrValue(n, "title"))
				}
				if alt == "" {
					alt = "image"
				}
				lowerSrc := strings.ToLower(src)
				if strings.HasPrefix(lowerSrc, "cid:") {
					key := normalizeContentID(src)
					if img, ok := imagesByCID[key]; ok {
						placed[key] = true
						blocks = append(blocks, previewDocumentBlock{Kind: previewBlockInlineImage, Image: img, ContentID: img.ContentID, Alt: alt})
					} else {
						blocks = append(blocks, previewDocumentBlock{Kind: previewBlockText, Text: fmt.Sprintf("[missing inline image: %s]", strings.TrimPrefix(src, "cid:"))})
					}
					return
				}
				if strings.HasPrefix(lowerSrc, "http://") || strings.HasPrefix(lowerSrc, "https://") {
					blocks = append(blocks, previewDocumentBlock{Kind: previewBlockText, Text: fmt.Sprintf("![%s](%s)", escapeMarkdownLabel(alt), src)})
				}
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(root)
	flushText()
	return blocks, len(blocks) > 0
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
	var out []previewDocumentBlock
	for _, img := range images {
		key := normalizeContentID(img.ContentID)
		if key != "" && placed[key] {
			continue
		}
		out = append(out, previewDocumentBlock{Kind: previewBlockInlineImage, Image: img, ContentID: img.ContentID, Alt: "inline image"})
	}
	return out
}
```

- [ ] **Step 4: Run document builder tests and verify they pass**

Run:

```bash
go test ./internal/app -run 'TestBuildPreviewDocument' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit document builder**

Run:

```bash
git add internal/app/preview_document.go internal/app/preview_document_test.go
git commit -m "feat: build preview document blocks"
```

Expected: commit succeeds.

---

### Task 2: Graphics Mode And Image Renderer

**Files:**
- Create: `internal/app/preview_image_renderer.go`
- Create: `internal/app/preview_image_renderer_test.go`
- Modify: `internal/app/image_preview_render.go`

- [ ] **Step 1: Write failing image renderer tests**

Add `internal/app/preview_image_renderer_test.go`:

```go
package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"mail-processor/internal/models"
)

func tinyPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestDetectPreviewImageMode(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if got := detectPreviewImageMode(previewImageModeAuto, true, false); got != previewImageModeIterm2 {
		t.Fatalf("iTerm auto mode = %q, want %q", got, previewImageModeIterm2)
	}
	if got := detectPreviewImageMode(previewImageModeAuto, true, true); got != previewImageModePlaceholder {
		t.Fatalf("ssh auto mode = %q, want placeholder", got)
	}
	if got := detectPreviewImageMode(previewImageModeLinks, true, false); got != previewImageModeLinks {
		t.Fatalf("forced links mode = %q, want links", got)
	}
}

func TestPreviewImageCellSizeAvoidsUpscalingTinyImages(t *testing.T) {
	img := models.InlineImage{ContentID: "small", MIMEType: "image/png", Data: tinyPNG(t, 32, 16)}
	size := previewImageCellSize(img, 120, 20)
	if size.Width > 8 {
		t.Fatalf("tiny image width = %d cells, want no large upscale", size.Width)
	}
	if size.Rows > 3 {
		t.Fatalf("tiny image rows = %d, want no large upscale", size.Rows)
	}
}

func TestPreviewImageCellSizeBoundsLargeImages(t *testing.T) {
	img := models.InlineImage{ContentID: "large", MIMEType: "image/png", Data: tinyPNG(t, 1200, 800)}
	size := previewImageCellSize(img, 80, 18)
	if size.Width > 78 {
		t.Fatalf("large image width = %d cells, want <= 78", size.Width)
	}
	if size.Rows > 18 {
		t.Fatalf("large image rows = %d, want <= 18", size.Rows)
	}
	if size.Rows < 8 {
		t.Fatalf("large image rows = %d, want useful preview height", size.Rows)
	}
}

func TestIterm2PreviewRendererReportsRowsWithoutTrailingNewline(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	img := models.InlineImage{ContentID: "photo", MIMEType: "image/png", Data: tinyPNG(t, 120, 80)}
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeIterm2,
		Image:         img,
		InnerWidth:    100,
		AvailableRows: 12,
	})

	if rendered.Rows <= 0 || rendered.Rows > 12 {
		t.Fatalf("rows = %d, want 1..12", rendered.Rows)
	}
	if !strings.Contains(rendered.Content, "\x1b]1337;File=") {
		t.Fatalf("expected iTerm2 image escape, got %q", rendered.Content)
	}
	if strings.HasSuffix(rendered.Content, "\n") {
		t.Fatalf("image renderer content should not hide trailing newline: %q", rendered.Content)
	}
}

func TestPreviewImageRendererFallbacks(t *testing.T) {
	empty := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeIterm2,
		Image:         models.InlineImage{ContentID: "empty", MIMEType: "image/png"},
		InnerWidth:    80,
		AvailableRows: 6,
	})
	if empty.Rows != 1 || !strings.Contains(empty.Content, "image unavailable") {
		t.Fatalf("empty image fallback = %#v", empty)
	}

	off := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeOff,
		Image:         models.InlineImage{ContentID: "off", MIMEType: "image/png", Data: []byte("x")},
		InnerWidth:    80,
		AvailableRows: 6,
	})
	if off.Rows != 0 || off.Content != "" {
		t.Fatalf("off mode = %#v, want empty block", off)
	}
}
```

- [ ] **Step 2: Run image renderer tests and verify they fail**

Run:

```bash
go test ./internal/app -run 'TestDetectPreviewImageMode|TestPreviewImageCellSize|TestIterm2PreviewRenderer|TestPreviewImageRendererFallbacks' -count=1
```

Expected: FAIL with compile errors for `previewImageModeAuto`, `detectPreviewImageMode`, `previewImageCellSize`, `renderPreviewImageBlock`, and related types.

- [ ] **Step 3: Implement image renderer mode, sizing, and row accounting**

Create `internal/app/preview_image_renderer.go`:

```go
package app

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"mail-processor/internal/iterm2"
	"mail-processor/internal/models"
)

type previewImageMode string

const (
	previewImageModeAuto        previewImageMode = "auto"
	previewImageModeIterm2      previewImageMode = "iterm2"
	previewImageModeLinks       previewImageMode = "links"
	previewImageModePlaceholder previewImageMode = "placeholder"
	previewImageModeOff         previewImageMode = "off"
)

type previewImageSize struct {
	Width int
	Rows  int
}

type previewImageRenderRequest struct {
	Mode          previewImageMode
	Image         models.InlineImage
	Alt           string
	Description   string
	InnerWidth    int
	AvailableRows int
}

type previewImageRenderResult struct {
	Content string
	Rows    int
}

func detectPreviewImageMode(requested previewImageMode, localLinks bool, sshMode bool) previewImageMode {
	switch requested {
	case previewImageModeIterm2, previewImageModeLinks, previewImageModePlaceholder, previewImageModeOff:
		return requested
	}
	if sshMode {
		return previewImageModePlaceholder
	}
	if iterm2.IsSupported() {
		return previewImageModeIterm2
	}
	if localLinks {
		return previewImageModeLinks
	}
	return previewImageModePlaceholder
}

func previewImageCellSize(img models.InlineImage, innerW, availableRows int) previewImageSize {
	maxWidth := innerW - 2
	if maxWidth < 1 {
		maxWidth = 1
	}
	if availableRows < 1 {
		availableRows = 1
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		rows := clampInt(availableRows/2, 1, minInt(maxPreviewImageRows, availableRows))
		return previewImageSize{Width: maxWidth, Rows: rows}
	}

	widthCells := (cfg.Width + 7) / 8
	if widthCells < 1 {
		widthCells = 1
	}
	if widthCells > maxWidth {
		widthCells = maxWidth
	}

	rowCells := (cfg.Height + 15) / 16
	if rowCells < 1 {
		rowCells = 1
	}
	maxRows := minInt(availableRows, 18)
	if rowCells > maxRows {
		rowCells = maxRows
	}
	return previewImageSize{Width: widthCells, Rows: rowCells}
}

func renderPreviewImageBlock(req previewImageRenderRequest) previewImageRenderResult {
	if req.Mode == previewImageModeOff {
		return previewImageRenderResult{}
	}
	if req.AvailableRows <= 0 {
		return previewImageRenderResult{}
	}
	if len(req.Image.Data) == 0 {
		return oneLinePreviewImageFallback(fmt.Sprintf("[image unavailable: %s]", req.Image.ContentID), req.InnerWidth)
	}
	if len(req.Image.Data) > maxPreviewImageBytes {
		return oneLinePreviewImageFallback(fmt.Sprintf("[image too large to render inline: %s  %d KB]", req.Image.MIMEType, len(req.Image.Data)/1024), req.InnerWidth)
	}

	switch req.Mode {
	case previewImageModeIterm2:
		size := previewImageCellSize(req.Image, req.InnerWidth, req.AvailableRows)
		content := strings.TrimRight(iterm2.Render(req.Image.Data, size.Width, size.Rows), "\n")
		if content == "" {
			return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
		}
		return previewImageRenderResult{Content: content, Rows: size.Rows}
	case previewImageModePlaceholder, previewImageModeLinks:
		return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
	default:
		return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
	}
}

func previewImagePlaceholderText(req previewImageRenderRequest) string {
	if desc := strings.TrimSpace(req.Description); desc != "" {
		return "[Image: " + desc + "]"
	}
	if alt := strings.TrimSpace(req.Alt); alt != "" && alt != "image" && alt != "inline image" {
		return fmt.Sprintf("[image: %s  %s  %d KB]", alt, req.Image.MIMEType, len(req.Image.Data)/1024)
	}
	return fmt.Sprintf("[image: %s  %d KB]", req.Image.MIMEType, len(req.Image.Data)/1024)
}

func oneLinePreviewImageFallback(text string, innerW int) previewImageRenderResult {
	return previewImageRenderResult{Content: truncateVisual(text, innerW), Rows: 1}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

Modify `internal/app/image_preview_render.go` only after the new renderer exists:

```go
func renderIterm2PreviewImages(images []models.InlineImage, descs map[string]string, innerW, availableRows int) (string, int) {
	var sb strings.Builder
	used := 0
	for i, img := range images {
		if used >= availableRows {
			break
		}
		if used > 0 {
			sb.WriteByte('\n')
			used++
		}
		desc := ""
		if descs != nil {
			desc = descs[img.ContentID]
		}
		rendered := renderPreviewImageBlock(previewImageRenderRequest{
			Mode:          previewImageModeIterm2,
			Image:         img,
			Description:   desc,
			InnerWidth:    innerW,
			AvailableRows: availableRows - used,
		})
		if rendered.Rows == 0 {
			continue
		}
		sb.WriteString(rendered.Content)
		used += rendered.Rows
	}
	return sb.String(), clampInt(used, 0, availableRows)
}
```

- [ ] **Step 4: Run image renderer tests and existing image tests**

Run:

```bash
go test ./internal/app -run 'TestDetectPreviewImageMode|TestPreviewImageCellSize|TestIterm2PreviewRenderer|TestPreviewImageRendererFallbacks|TestItermPreviewImagesDoNotExceedAvailableRows|TestTimelineFullScreen_ItermRendersBoundedInlineImage' -count=1
```

Expected: PASS. If `TestTimelineFullScreen_ItermRendersBoundedInlineImage` still expects exact `width=76` and `height=8`, update that assertion to verify bounded `width=` and `height=` values are present and `height` is no more than available rows, because the new sizing avoids upscaling small images.

- [ ] **Step 5: Commit image renderer**

Run:

```bash
git add internal/app/preview_image_renderer.go internal/app/preview_image_renderer_test.go internal/app/image_preview_render.go internal/app/image_preview_test.go
git commit -m "feat: add preview image renderer modes"
```

Expected: commit succeeds.

---

### Task 3: Preview Viewport Layout

**Files:**
- Create: `internal/app/preview_viewport.go`
- Create: `internal/app/preview_viewport_test.go`

- [ ] **Step 1: Write failing viewport tests**

Add `internal/app/preview_viewport_test.go`:

```go
package app

import (
	"strings"
	"testing"

	"mail-processor/internal/models"
)

func TestLayoutPreviewDocumentOrdersTextAndImages(t *testing.T) {
	doc := previewDocument{Blocks: []previewDocumentBlock{
		{Kind: previewBlockText, Text: "Before image"},
		{Kind: previewBlockInlineImage, Image: models.InlineImage{ContentID: "logo", MIMEType: "image/png", Data: []byte("png")}, Alt: "Logo"},
		{Kind: previewBlockText, Text: "After image"},
	}}

	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    80,
		AvailableRows: 10,
		ImageMode:     previewImageModePlaceholder,
	})

	if layout.TotalRows < 3 {
		t.Fatalf("total rows = %d, want text/image/text rows", layout.TotalRows)
	}
	rendered := renderPreviewDocumentViewport(layout, 0, 10)
	plain := stripANSI(rendered.Content)
	before := strings.Index(plain, "Before image")
	image := strings.Index(plain, "Logo")
	after := strings.Index(plain, "After image")
	if before == -1 || image == -1 || after == -1 || !(before < image && image < after) {
		t.Fatalf("unexpected order:\n%s", plain)
	}
}

func TestRenderPreviewDocumentViewportClampsRows(t *testing.T) {
	doc := previewDocument{Blocks: []previewDocumentBlock{
		{Kind: previewBlockText, Text: strings.Repeat("line\n", 50)},
	}}
	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    20,
		AvailableRows: 8,
		ImageMode:     previewImageModePlaceholder,
	})

	rendered := renderPreviewDocumentViewport(layout, 0, 8)
	if rendered.Rows != 8 {
		t.Fatalf("rendered rows = %d, want 8", rendered.Rows)
	}
	if got := len(strings.Split(rendered.Content, "\n")); got != 8 {
		t.Fatalf("content line count = %d, want 8:\n%s", got, rendered.Content)
	}
}

func TestClampPreviewScrollOffsetUsesDocumentRows(t *testing.T) {
	if got := clampPreviewScrollOffset(99, 20, 6); got != 14 {
		t.Fatalf("clamped offset = %d, want 14", got)
	}
	if got := clampPreviewScrollOffset(-3, 20, 6); got != 0 {
		t.Fatalf("negative offset = %d, want 0", got)
	}
	if got := clampPreviewScrollOffset(10, 4, 6); got != 0 {
		t.Fatalf("short document offset = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run viewport tests and verify they fail**

Run:

```bash
go test ./internal/app -run 'TestLayoutPreviewDocument|TestRenderPreviewDocumentViewport|TestClampPreviewScrollOffset' -count=1
```

Expected: FAIL with compile errors for `layoutPreviewDocument`, `previewLayoutOptions`, `renderPreviewDocumentViewport`, and `clampPreviewScrollOffset`.

- [ ] **Step 3: Implement layout and viewport rendering**

Create `internal/app/preview_viewport.go`:

```go
package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type previewLayoutOptions struct {
	InnerWidth    int
	AvailableRows int
	ImageMode     previewImageMode
	Descriptions  map[string]string
}

type previewRenderedRow struct {
	Content string
}

type previewDocumentLayout struct {
	Rows      []previewRenderedRow
	TotalRows int
}

type previewViewportRender struct {
	Content string
	Rows    int
}

func layoutPreviewDocument(doc previewDocument, opts previewLayoutOptions) previewDocumentLayout {
	if opts.InnerWidth < 1 {
		opts.InnerWidth = 1
	}
	if opts.AvailableRows < 1 {
		opts.AvailableRows = 1
	}

	var rows []previewRenderedRow
	for blockIndex, block := range doc.Blocks {
		if blockIndex > 0 && len(rows) > 0 {
			rows = append(rows, previewRenderedRow{})
		}
		switch block.Kind {
		case previewBlockInlineImage:
			desc := ""
			if opts.Descriptions != nil {
				desc = opts.Descriptions[block.Image.ContentID]
			}
			rendered := renderPreviewImageBlock(previewImageRenderRequest{
				Mode:          opts.ImageMode,
				Image:         block.Image,
				Alt:           block.Alt,
				Description:   desc,
				InnerWidth:    opts.InnerWidth,
				AvailableRows: opts.AvailableRows,
			})
			if rendered.Rows == 0 || rendered.Content == "" {
				continue
			}
			contentRows := strings.Split(rendered.Content, "\n")
			for _, line := range contentRows {
				rows = append(rows, previewRenderedRow{Content: ansi.Truncate(line, opts.InnerWidth, "")})
			}
			for len(contentRows) < rendered.Rows {
				rows = append(rows, previewRenderedRow{})
				contentRows = append(contentRows, "")
			}
		default:
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			for _, line := range renderEmailBodyLines(text, opts.InnerWidth) {
				rows = append(rows, previewRenderedRow{Content: ansi.Truncate(line, opts.InnerWidth, "")})
			}
		}
	}
	if len(rows) == 0 {
		rows = append(rows, previewRenderedRow{Content: "(No content)"})
	}
	return previewDocumentLayout{Rows: rows, TotalRows: len(rows)}
}

func clampPreviewScrollOffset(offset, totalRows, visibleRows int) int {
	if offset < 0 {
		return 0
	}
	maxOffset := totalRows - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func renderPreviewDocumentViewport(layout previewDocumentLayout, offset, visibleRows int) previewViewportRender {
	if visibleRows < 1 {
		visibleRows = 1
	}
	offset = clampPreviewScrollOffset(offset, layout.TotalRows, visibleRows)
	end := offset + visibleRows
	if end > layout.TotalRows {
		end = layout.TotalRows
	}
	lines := make([]string, 0, visibleRows)
	for i := offset; i < end; i++ {
		lines = append(lines, layout.Rows[i].Content)
	}
	for len(lines) < visibleRows {
		lines = append(lines, "")
	}
	return previewViewportRender{Content: strings.Join(lines, "\n"), Rows: len(lines)}
}
```

- [ ] **Step 4: Run viewport tests**

Run:

```bash
go test ./internal/app -run 'TestLayoutPreviewDocument|TestRenderPreviewDocumentViewport|TestClampPreviewScrollOffset' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit viewport layout**

Run:

```bash
git add internal/app/preview_viewport.go internal/app/preview_viewport_test.go
git commit -m "feat: lay out preview document viewports"
```

Expected: commit succeeds.

---

### Task 4: Timeline Full-Screen Integration

**Files:**
- Modify: `internal/app/layout.go`
- Modify: `internal/app/email_preview.go`
- Modify: `internal/app/timeline.go`
- Modify: `internal/app/key_routing.go`
- Modify: `internal/app/mouse.go`
- Modify: `internal/app/image_preview_test.go`
- Modify: `internal/app/layout_regression_test.go`

- [ ] **Step 1: Write failing Timeline full-screen integration tests**

Append to `internal/app/image_preview_test.go`:

```go
func TestTimelineFullScreen_RendersCIDImageInDocumentOrder(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextPlain: "Plain fallback",
		TextHTML: `<p>Before local image.</p><img alt="Local Logo" src="cid:logo"><p>After local image.</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true

	rendered := stripANSI(m.renderFullScreenEmail())
	before := strings.Index(rendered, "Before local image")
	image := strings.Index(rendered, "Local Logo")
	after := strings.Index(rendered, "After local image")
	if before == -1 || image == -1 || after == -1 || !(before < image && image < after) {
		t.Fatalf("CID image should be in document order, got:\n%s", rendered)
	}
}

func TestTimelineFullScreen_DocumentRowsDriveScrollIndicator(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 80, 18)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>` + strings.Repeat("body line<br>", 30) + `</p><img alt="Chart" src="cid:chart"><p>tail</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "chart", MIMEType: "image/png", Data: []byte("chart-bytes")},
		},
	}
	m.timeline.fullScreen = true
	m.timeline.bodyScrollOffset = 999

	rendered := m.renderFullScreenEmail()
	assertFitsHeight(t, 18, rendered)
	if !strings.Contains(stripANSI(rendered), "z/esc: exit full-screen") {
		t.Fatalf("expected pinned full-screen hint, got:\n%s", stripANSI(rendered))
	}
}
```

- [ ] **Step 2: Run Timeline integration tests and verify they fail**

Run:

```bash
go test ./internal/app -run 'TestTimelineFullScreen_RendersCIDImageInDocumentOrder|TestTimelineFullScreen_DocumentRowsDriveScrollIndicator' -count=1
```

Expected: first test FAILS because current full-screen renders all images before text. The row test FAILS if the old text-only scroll accounting still allows the full-screen render to exceed the terminal budget.

- [ ] **Step 3: Add cache fields and clear helper**

Modify `internal/app/layout.go`:

```go
type TimelineState struct {
	// existing fields...
	bodyWrappedLines []string
	bodyWrappedWidth int
	bodyScrollOffset int
	previewDocLayout *previewDocumentLayout
	previewDocWidth  int
	previewDocMode   previewImageMode
	previewDocMessageID string
	// remaining fields...
}
```

Modify `internal/app/app.go` near cleanup preview fields:

```go
	cleanupBodyWrappedLines  []string
	cleanupBodyWrappedWidth  int
	cleanupPreviewDocLayout  *previewDocumentLayout
	cleanupPreviewDocWidth   int
	cleanupPreviewDocMode    previewImageMode
	cleanupPreviewDocMessageID string
```

Create helper functions in `internal/app/email_preview.go` near `clampInt`:

```go
func (m *Model) clearTimelinePreviewDocumentCache() {
	m.timeline.previewDocLayout = nil
	m.timeline.previewDocWidth = 0
	m.timeline.previewDocMode = ""
	m.timeline.previewDocMessageID = ""
}

func (m *Model) clearCleanupPreviewDocumentCache() {
	m.cleanupPreviewDocLayout = nil
	m.cleanupPreviewDocWidth = 0
	m.cleanupPreviewDocMode = ""
	m.cleanupPreviewDocMessageID = ""
}
```

- [ ] **Step 4: Build Timeline full-screen document layout**

Add to `internal/app/email_preview.go` near `renderFullScreenEmail`:

```go
func (m *Model) currentPreviewImageMode() previewImageMode {
	return detectPreviewImageMode(previewImageModeAuto, m.localImageLinks, m.sshMode)
}

func (m *Model) timelinePreviewDocumentLayout(innerW, availableRows int) previewDocumentLayout {
	mode := m.currentPreviewImageMode()
	messageID := m.timeline.bodyMessageID
	if m.timeline.previewDocLayout != nil &&
		m.timeline.previewDocWidth == innerW &&
		m.timeline.previewDocMode == mode &&
		m.timeline.previewDocMessageID == messageID {
		return *m.timeline.previewDocLayout
	}
	doc := buildPreviewDocument(m.timeline.body, m.timeline.inlineImageDescs)
	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    innerW,
		AvailableRows: availableRows,
		ImageMode:     mode,
		Descriptions:  m.timeline.inlineImageDescs,
	})
	m.timeline.previewDocLayout = &layout
	m.timeline.previewDocWidth = innerW
	m.timeline.previewDocMode = mode
	m.timeline.previewDocMessageID = messageID
	return layout
}
```

- [ ] **Step 5: Replace Timeline full-screen body rendering**

In `renderFullScreenEmail`, replace the current image-block-plus-`bodyWrappedLines` section from `imageBlock, imageRows := ...` through the scroll indicator with this structure:

```go
		layout := m.timelinePreviewDocumentLayout(innerW, maxBodyLines)
		m.timeline.bodyScrollOffset = clampPreviewScrollOffset(m.timeline.bodyScrollOffset, layout.TotalRows, maxBodyLines)
		viewport := renderPreviewDocumentViewport(layout, m.timeline.bodyScrollOffset, maxBodyLines)
		sb.WriteString(viewport.Content)

		if layout.TotalRows > maxBodyLines {
			maxOffset := layout.TotalRows - maxBodyLines
			pct := 0
			if maxOffset > 0 {
				pct = m.timeline.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%  │  z/esc: exit full-screen", m.timeline.bodyScrollOffset+1, layout.TotalRows, pct)
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
		} else {
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(" z/esc: exit full-screen", innerW)))
		}
```

Keep `Loading…`, quick reply picker reservation, header rendering, and bottom hint behavior intact.

- [ ] **Step 6: Clear Timeline document cache at existing invalidation points**

At every site already setting `m.timeline.bodyWrappedLines = nil` because the body, width, message, visual content, or full-screen state changed, add:

```go
m.clearTimelinePreviewDocumentCache()
```

Required sites from current code:
- `internal/app/email_preview.go` in `maybeUpdatePreview`
- `internal/app/email_preview.go` in `clearTimelinePreview`
- `internal/app/timeline.go` in `EmailBodyMsg` handling
- `internal/app/timeline.go` in `ImageDescMsg` handling
- `internal/app/timeline.go` in full-screen `z` toggle
- `internal/app/timeline.go` in `clearTimelineFullScreen`
- `internal/app/mouse.go` if revoking image previews after mode changes affects full-screen rendered links

- [ ] **Step 7: Run Timeline tests**

Run:

```bash
go test ./internal/app -run 'TestTimelineFullScreen_RendersCIDImageInDocumentOrder|TestTimelineFullScreen_DocumentRowsDriveScrollIndicator|TestTimelineFullScreen_ItermRendersBoundedInlineImage|TestSplitPreviewDoesNotPromiseFullscreenImageRenderingWhenNoViewerAvailable' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Timeline integration**

Run:

```bash
git add internal/app/layout.go internal/app/app.go internal/app/email_preview.go internal/app/timeline.go internal/app/key_routing.go internal/app/mouse.go internal/app/image_preview_test.go internal/app/layout_regression_test.go
git commit -m "feat: render timeline preview as document stream"
```

Expected: commit succeeds.

---

### Task 5: Local Link Renderer Integration

**Files:**
- Modify: `internal/app/preview_image_renderer.go`
- Modify: `internal/app/preview_viewport.go`
- Modify: `internal/app/email_preview.go`
- Modify: `internal/app/image_preview_server.go`
- Modify: `internal/app/image_preview_test.go`

- [ ] **Step 1: Write failing local link viewport test**

Append to `internal/app/image_preview_test.go`:

```go
func TestTimelineFullScreen_DocumentImageUsesLocalOpenLinkWhenNotIterm(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>Before</p><img alt="Logo" src="cid:logo"><p>After</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true

	rendered := m.renderFullScreenEmail()
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected local open image link, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;http://127.0.0.1:") {
		t.Fatalf("expected OSC8 localhost target, got raw:\n%q", rendered)
	}
}
```

- [ ] **Step 2: Run local link test and verify it fails if links are not wired into document renderer**

Run:

```bash
go test ./internal/app -run 'TestTimelineFullScreen_DocumentImageUsesLocalOpenLinkWhenNotIterm' -count=1
```

Expected: FAIL if Task 4 used placeholders for `previewImageModeLinks`.

- [ ] **Step 3: Add model-aware local link rendering to document layout**

Update `previewImageRenderRequest` in `internal/app/preview_image_renderer.go`:

```go
type previewImageRenderRequest struct {
	Mode          previewImageMode
	Image         models.InlineImage
	Alt           string
	Description   string
	InnerWidth    int
	AvailableRows int
	LinkLabel     string
	LinkURL       string
}
```

Update `renderPreviewImageBlock` link branch:

```go
	case previewImageModeLinks:
		if req.LinkURL != "" && req.LinkLabel != "" {
			label := render.TerminalHyperlink("["+req.LinkLabel+"]", req.LinkURL)
			meta := fmt.Sprintf(" %s  %d KB", req.Image.MIMEType, len(req.Image.Data)/1024)
			return previewImageRenderResult{Content: truncateVisual(label+meta, req.InnerWidth), Rows: 1}
		}
		return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
```

Add the missing import:

```go
	"mail-processor/internal/render"
```

- [ ] **Step 4: Pass registered links from `Model` into layout options**

Update `imagePreviewLink` and `RegisterSet` in `internal/app/image_preview_server.go`:

```go
type imagePreviewLink struct {
	Label     string
	URL       string
	MIMEType  string
	Size      int
	ContentID string
}
```

In the `s.links = append(s.links, imagePreviewLink{...})` block, add:

```go
			ContentID: img.ContentID,
```

Extend `previewLayoutOptions` in `internal/app/preview_viewport.go`:

```go
type previewLayoutOptions struct {
	InnerWidth    int
	AvailableRows int
	ImageMode     previewImageMode
	Descriptions  map[string]string
	ImageLinks    map[string]imagePreviewLink
}
```

When rendering an image block, look up link data:

```go
			link := opts.ImageLinks[normalizeContentID(block.Image.ContentID)]
			rendered := renderPreviewImageBlock(previewImageRenderRequest{
				Mode:          opts.ImageMode,
				Image:         block.Image,
				Alt:           block.Alt,
				Description:   desc,
				InnerWidth:    opts.InnerWidth,
				AvailableRows: opts.AvailableRows,
				LinkLabel:     link.Label,
				LinkURL:       link.URL,
			})
```

In `internal/app/email_preview.go`, add:

```go
func (m *Model) localImageLinkMap(scopeKey string, images []models.InlineImage, mode previewImageMode) map[string]imagePreviewLink {
	if mode != previewImageModeLinks || len(images) == 0 {
		return nil
	}
	if m.imagePreviewLinks == nil {
		m.imagePreviewLinks = newImagePreviewServer()
	}
	links, err := m.imagePreviewLinks.RegisterSet(scopeKey, images)
	if err != nil {
		logger.Warn("local image preview links unavailable: %v", err)
		return nil
	}
	out := make(map[string]imagePreviewLink, len(links))
	for _, link := range links {
		out[normalizeContentID(link.ContentID)] = link
	}
	return out
}
```

Add this import to `internal/app/email_preview.go`:

```go
	"mail-processor/internal/logger"
```

Use it in `timelinePreviewDocumentLayout`:

```go
	doc := buildPreviewDocument(m.timeline.body, m.timeline.inlineImageDescs)
	scopeKey := "timeline:" + m.timeline.bodyMessageID
	imageLinks := m.localImageLinkMap(scopeKey, m.timeline.body.InlineImages, mode)
	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    innerW,
		AvailableRows: availableRows,
		ImageMode:     mode,
		Descriptions:  m.timeline.inlineImageDescs,
		ImageLinks:    imageLinks,
	})
```

- [ ] **Step 5: Run local link and image tests**

Run:

```bash
go test ./internal/app -run 'TestTimelineFullScreen_DocumentImageUsesLocalOpenLinkWhenNotIterm|TestTimelineFullScreen_NonItermShowsLocalImageLinks|TestLocalImageLinksUseMetadataFromLinkedImage|TestImagePreviewServerServesRegisteredImagesOnly' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit local link integration**

Run:

```bash
git add internal/app/preview_image_renderer.go internal/app/preview_viewport.go internal/app/email_preview.go internal/app/image_preview_test.go
git commit -m "feat: expose local links in document preview"
```

Expected: commit succeeds.

---

### Task 6: Cleanup Full-Screen Integration

**Files:**
- Modify: `internal/app/email_preview.go`
- Modify: `internal/app/key_routing.go`
- Modify: `internal/app/cleanup_preview_test.go`
- Modify: `internal/app/image_preview_test.go`

- [ ] **Step 1: Write failing Cleanup document-order test**

Append to `internal/app/image_preview_test.go`:

```go
func TestCleanupFullScreen_RendersCIDImageInDocumentOrder(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewWidth = 100
	m.cleanupPreviewEmail = testImageEmail()
	m.cleanupEmailBody = &models.EmailBody{
		TextHTML: `<p>Before cleanup image.</p><img alt="Cleanup Logo" src="cid:logo"><p>After cleanup image.</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}

	rendered := stripANSI(m.renderCleanupPreview())
	before := strings.Index(rendered, "Before cleanup image")
	image := strings.Index(rendered, "open image")
	after := strings.Index(rendered, "After cleanup image")
	if before == -1 || image == -1 || after == -1 || !(before < image && image < after) {
		t.Fatalf("cleanup CID image should be in document order, got:\n%s", rendered)
	}
}
```

- [ ] **Step 2: Run Cleanup test and verify it fails**

Run:

```bash
go test ./internal/app -run 'TestCleanupFullScreen_RendersCIDImageInDocumentOrder' -count=1
```

Expected: FAIL because current Cleanup full-screen still renders image block before text.

- [ ] **Step 3: Add Cleanup document layout helper**

Add to `internal/app/email_preview.go` near the Timeline helper:

```go
func (m *Model) cleanupPreviewDocumentLayout(innerW, availableRows int) previewDocumentLayout {
	mode := m.currentPreviewImageMode()
	messageID := ""
	if m.cleanupPreviewEmail != nil {
		messageID = m.cleanupPreviewEmail.MessageID
	}
	if m.cleanupPreviewDocLayout != nil &&
		m.cleanupPreviewDocWidth == innerW &&
		m.cleanupPreviewDocMode == mode &&
		m.cleanupPreviewDocMessageID == messageID {
		return *m.cleanupPreviewDocLayout
	}
	doc := buildPreviewDocument(m.cleanupEmailBody, nil)
	scopeKey := "cleanup"
	if messageID != "" {
		scopeKey += ":" + messageID
	}
	imageLinks := map[string]imagePreviewLink(nil)
	if m.cleanupEmailBody != nil {
		imageLinks = m.localImageLinkMap(scopeKey, m.cleanupEmailBody.InlineImages, mode)
	}
	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    innerW,
		AvailableRows: availableRows,
		ImageMode:     mode,
		ImageLinks:    imageLinks,
	})
	m.cleanupPreviewDocLayout = &layout
	m.cleanupPreviewDocWidth = innerW
	m.cleanupPreviewDocMode = mode
	m.cleanupPreviewDocMessageID = messageID
	return layout
}
```

- [ ] **Step 4: Replace Cleanup full-screen body rendering**

In `renderCleanupPreview`, keep split preview behavior unchanged. Inside `if m.cleanupFullScreen { ... }`, stop calling `renderInlineImagesForPreview`; instead use:

```go
		if m.cleanupFullScreen {
			layout := m.cleanupPreviewDocumentLayout(innerW, maxBodyLines)
			m.cleanupBodyScrollOffset = clampPreviewScrollOffset(m.cleanupBodyScrollOffset, layout.TotalRows, maxBodyLines)
			viewport := renderPreviewDocumentViewport(layout, m.cleanupBodyScrollOffset, maxBodyLines)
			sb.WriteString(viewport.Content)

			escHint := "Esc: close preview"
			zHint := "z: exit full-screen"
			actionHint := "D: delete  e: archive"
			if layout.TotalRows > maxBodyLines {
				maxOffset := layout.TotalRows - maxBodyLines
				pct := 0
				if maxOffset > 0 {
					pct = m.cleanupBodyScrollOffset * 100 / maxOffset
				}
				indicator := fmt.Sprintf(" %s  %s  ↑↓ j/k  line %d/%d  %d%%  │  %s", actionHint, zHint, m.cleanupBodyScrollOffset+1, layout.TotalRows, pct, escHint)
				sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
			} else {
				indicator := fmt.Sprintf(" %s  %s  ↑↓ j/k  │  %s", actionHint, zHint, escHint)
				sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
			}
		} else {
			// keep existing split-preview compact hint and text wrapping path here
		}
```

Make the split-preview branch keep the existing `cleanupBodyWrappedLines` code.

- [ ] **Step 5: Clear Cleanup document cache at invalidation points**

Add `m.clearCleanupPreviewDocumentCache()` wherever Cleanup body state changes:
- `CleanupEmailBodyMsg` handling in `internal/app/app.go`
- `handleEscKey` cleanup preview close path in `internal/app/key_routing.go`
- cleanup full-screen `z` toggle path
- cleanup preview email changes
- `SetLocalImageLinksEnabled(false)`

- [ ] **Step 6: Run Cleanup tests**

Run:

```bash
go test ./internal/app -run 'TestCleanupFullScreen_RendersCIDImageInDocumentOrder|TestCleanupFullScreen_NonItermShowsLocalImageLinks|TestCleanupFullScreen|TestCleanupEscInFullScreen|TestCleanupEscClosesPreview' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Cleanup integration**

Run:

```bash
git add internal/app/email_preview.go internal/app/key_routing.go internal/app/cleanup_preview_test.go internal/app/image_preview_test.go
git commit -m "feat: render cleanup preview as document stream"
```

Expected: commit succeeds.

---

### Task 7: Demo Fixture HTML/CID Placement

**Files:**
- Modify: `internal/demo/fixtures.go`
- Modify: `internal/demo/fixtures_test.go`
- Modify: `internal/backend/demo_behavior_test.go`

- [ ] **Step 1: Write failing demo fixture tests**

Append to `internal/demo/fixtures_test.go`:

```go
func TestCreativeCommonsSamplerIncludesHTMLCIDPlacement(t *testing.T) {
	box := NewMailboxFixture()
	var found *Message
	for i := range box.Messages {
		if box.Messages[i].Email.Subject == "Creative Commons image sampler for terminal previews" {
			found = &box.Messages[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected sampler fixture")
	}
	if !found.Body.IsFromHTML {
		t.Fatal("sampler should exercise HTML-derived preview behavior")
	}
	for _, cid := range []string{"cc-by-sa-badge", "color-chart-330px", "bee-on-sunflower-330px", "changing-landscape-960px"} {
		if !strings.Contains(found.Body.TextHTML, "cid:"+cid) {
			t.Fatalf("sampler HTML missing cid reference %q:\n%s", cid, found.Body.TextHTML)
		}
	}
}
```

Add the same behavior assertion to `internal/backend/demo_behavior_test.go` in the existing sampler test:

```go
	if body.TextHTML == "" || !strings.Contains(body.TextHTML, "cid:cc-by-sa-badge") {
		t.Fatalf("sampler should expose HTML CID placement, got HTML:\n%s", body.TextHTML)
	}
```

- [ ] **Step 2: Run demo tests and verify they fail**

Run:

```bash
go test ./internal/demo ./internal/backend -run 'CreativeCommons|Sampler|Demo' -count=1
```

Expected: FAIL because sampler currently marks `IsFromHTML` but only has markdown/plain text and no `TextHTML`.

- [ ] **Step 3: Update Creative Commons sampler fixture**

Modify the sampler `add(29, ...)` call in `internal/demo/fixtures.go`. Keep the existing plain text as the body argument. Add a new option helper near `withHTML`:

```go
	withHTMLBody := func(html string) func(*Message) {
		return func(msg *Message) {
			msg.Body.IsFromHTML = true
			msg.Body.TextHTML = html
		}
	}
```

Pass this option to the sampler:

```go
withHTMLBody(`<html><body>
<h1>Creative Commons image sampler for terminal previews</h1>
<p>This demo email includes four embedded inline images with different dimensions so you can test Herald's split preview hint, full-screen image rendering, and non-iTerm local image fallback links without fetching media at runtime.</p>
<p><img alt="CC BY-SA badge" src="cid:cc-by-sa-badge"></p>
<p><img alt="Color chart" src="cid:color-chart-330px"></p>
<p><img alt="Bee on sunflower" src="cid:bee-on-sunflower-330px"></p>
<p><img alt="Changing landscape" src="cid:changing-landscape-960px"></p>
<h2>Embedded inline images</h2>
<ul>
<li>CC BY-SA badge: 46x21 PNG, CC0 1.0, by Heflox. Source: <a href="https://commons.wikimedia.org/wiki/File:CC-BY-SA.png">CC-BY-SA.png</a></li>
<li>Color chart: 330px PNG thumbnail, CC0 1.0, by Ccompagnon with a simplified revision by Iketsi. Source: <a href="https://commons.wikimedia.org/wiki/File:ColorChart.svg">ColorChart.svg</a></li>
<li>Bee on sunflower: 330px JPEG thumbnail, CC BY 4.0, by Mbrickn. Source: <a href="https://commons.wikimedia.org/wiki/File:Bee_on_Sunflower.jpg">Bee on Sunflower</a></li>
<li>Changing Landscape: 960px JPEG thumbnail, CC BY 4.0, by Mit.d.sheth. Source: <a href="https://commons.wikimedia.org/wiki/File:Changing_Landscape.jpg">Changing Landscape</a></li>
</ul>
<p>Remote image link, intentionally not fetched by Herald:</p>
<p><img alt="Remote Commons thumbnail" src="https://upload.wikimedia.org/wikipedia/commons/thumb/c/c0/ColorChart.svg/330px-ColorChart.svg.png"></p>
<p>Press z from the preview to open full-screen mode. In iTerm2, embedded images should render inline; in other local terminals, Herald should expose safe local open-image links for the embedded MIME bytes.</p>
</body></html>`)
```

Remove the older `withHTML` option from this fixture if both are present.

- [ ] **Step 4: Run demo tests**

Run:

```bash
go test ./internal/demo ./internal/backend -run 'CreativeCommons|Sampler|Demo' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit demo fixture**

Run:

```bash
git add internal/demo/fixtures.go internal/demo/fixtures_test.go internal/backend/demo_behavior_test.go
git commit -m "test: add cid placement to demo image sampler"
```

Expected: commit succeeds.

---

### Task 8: Test Protocol Documentation

**Files:**
- Modify: `TUI_TESTPLAN.md`
- Modify: `TUI_TESTING.md`

- [ ] **Step 1: Update TC-23A in `TUI_TESTPLAN.md`**

Replace the TC-23A steps and expectations with:

```markdown
**Steps:**
1. Open Timeline and search for `Creative Commons image sampler for terminal previews`.
2. Open the split preview and capture the image hint plus body links.
3. Press `z` to enter full-screen and capture the top of the document.
4. Scroll with app keys (`j`, `k`, `PgDn`, `PgUp`) until each inline image has appeared in the document flow.
5. In iTerm2/Kitty/Sixel raster mode, press `m` to release mouse capture, then use terminal-native scrollback to inspect whether image raster output displaced header/body text.
6. Repeat in a non-raster terminal, an iTerm2-compatible terminal if available, and SSH mode.
7. Run the standard resize cycle while full-screen preview is open.

**Expect:**
- The Creative Commons sampler fixture exposes four embedded inline images with different dimensions and HTML `cid:` placement.
- Split preview stays compact and does not promise image viewing when no full-screen image path is available.
- Full-screen preview renders text and inline images as one scrollable document below the pinned header.
- Raster images appear near their authored positions and do not push the header/title out of the visible app viewport or terminal scrollback.
- iTerm2-compatible terminals render bounded inline images using the selected raster mode.
- Non-iTerm local TUI shows OSC 8 `open image` links to localhost-served MIME inline image bytes.
- SSH mode avoids misleading localhost links and shows bounded placeholders unless the original email contains remote image URLs.
- Remote HTML image URLs appear as readable OSC 8 links and Herald does not fetch them automatically.
- At `50x15`, the minimum-size guard appears and resizing back restores a clean full-screen preview.
- Test reports include terminal app/version, selected image protocol mode, screenshots for raster modes, and ANSI captures where possible.
```

- [ ] **Step 2: Add raster caveat to `TUI_TESTING.md`**

Add this paragraph near the tmux capture guidance:

```markdown
**Terminal raster image protocols.** tmux captures are still required for layout, key routing, fallback links, and escape-sequence checks, but tmux cannot prove actual raster placement for protocols such as iTerm2 OSC 1337, Kitty graphics, or Sixel. For changes that affect inline raster images, run the demo in a real compatible terminal as well, capture screenshots, record the terminal app/version and selected graphics mode, and verify native scrollback does not show images displacing pinned preview chrome.
```

- [ ] **Step 3: Review documentation diff**

Run:

```bash
git diff -- TUI_TESTPLAN.md TUI_TESTING.md
```

Expected: diff contains only the TC-23A/testing-protocol changes above.

- [ ] **Step 4: Commit test protocol docs**

Run:

```bash
git add TUI_TESTPLAN.md TUI_TESTING.md
git commit -m "docs: expand raster image preview testing"
```

Expected: commit succeeds.

---

### Task 9: Full Verification And Report

**Files:**
- Create: `reports/TEST_REPORT_2026-04-29_full-screen-inline-image-document-preview.md`

- [ ] **Step 1: Run focused Go tests**

Run:

```bash
go test ./internal/app ./internal/demo ./internal/backend -run 'PreviewDocument|PreviewImage|PreviewViewport|TimelineFullScreen|CleanupFullScreen|CreativeCommons|Sampler' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full Go test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Build TUI and run tmux smoke checks**

Run:

```bash
go build -o /tmp/herald-test .
tmux kill-session -t herald-image-doc 2>/dev/null || true
tmux new-session -d -s herald-image-doc -x 220 -y 50
tmux send-keys -t herald-image-doc '/tmp/herald-test -debug -demo' Enter
sleep 4
tmux send-keys -t herald-image-doc '/'
sleep 0.2
tmux send-keys -t herald-image-doc 'Creative Commons image sampler for terminal previews'
sleep 1
tmux send-keys -t herald-image-doc Enter
sleep 0.5
tmux send-keys -t herald-image-doc 'j'
sleep 0.2
tmux send-keys -t herald-image-doc Enter
sleep 1
tmux send-keys -t herald-image-doc 'z'
sleep 0.5
tmux capture-pane -t herald-image-doc -p -e > /tmp/herald-image-doc-fullscreen.ansi
tmux resize-window -t herald-image-doc -x 80 -y 24
sleep 0.3
tmux capture-pane -t herald-image-doc -p -e > /tmp/herald-image-doc-80x24.ansi
tmux resize-window -t herald-image-doc -x 50 -y 15
sleep 0.3
tmux capture-pane -t herald-image-doc -p -e > /tmp/herald-image-doc-50x15.ansi
tmux kill-session -t herald-image-doc
```

Expected: command sequence completes. The `50x15` capture shows the minimum-size guard. The `220x50` and `80x24` captures show clean full-screen preview text with either local `open image` links or placeholders depending on environment.

- [ ] **Step 4: Run real-terminal raster check**

In iTerm2 or another supported raster terminal, run:

```bash
make build
./bin/herald -debug -demo
```

Manual steps:
- Open Timeline.
- Search for `Creative Commons image sampler for terminal previews`.
- Open the message.
- Press `z`.
- Scroll through the full-screen preview with `j`, `k`, `PgDn`, and `PgUp`.
- Press `m` if terminal-native scrollback requires mouse capture release.
- Use native scrollback to inspect the top and image areas.

Expected:
- Header remains pinned in the app viewport.
- Text and images scroll as one document.
- Raster images appear near the HTML-authored positions.
- Native scrollback does not show raster images pushing the title/header out of order.

- [ ] **Step 5: Run SSH surface check**

Run server:

```bash
go build -o ./bin/herald-ssh-server ./cmd/herald-ssh-server
./bin/herald-ssh-server --demo
```

In another terminal:

```bash
ssh -p 2222 localhost
```

Expected: open the sampler in full-screen; SSH mode shows placeholders or remote links without misleading localhost image links.

- [ ] **Step 6: Run MCP surface check**

Run:

```bash
go build -o ./bin/herald-mcp-server ./cmd/herald-mcp-server
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/herald-mcp-server --demo
```

Expected: tools list succeeds. No preview renderer code is exercised by MCP, but the surface still builds after shared model/demo changes.

- [ ] **Step 7: Save test report**

Create `reports/TEST_REPORT_2026-04-29_full-screen-inline-image-document-preview.md` with:

```markdown
# Full-Screen Inline Image Document Preview Test Report

Date: 2026-04-29

## Summary

- Result: PASS or FAIL
- Change: Full-screen preview document stream with inline image placement and graphics-mode fallbacks.

## Automated Tests

- `go test ./internal/app ./internal/demo ./internal/backend -run 'PreviewDocument|PreviewImage|PreviewViewport|TimelineFullScreen|CleanupFullScreen|CreativeCommons|Sampler' -count=1`
  - Result:
- `go test ./...`
  - Result:

## TUI tmux Checks

- Binary:
- Sizes checked: `220x50`, `80x24`, `50x15`
- Captures:
  - `/tmp/herald-image-doc-fullscreen.ansi`
  - `/tmp/herald-image-doc-80x24.ansi`
  - `/tmp/herald-image-doc-50x15.ansi`
- Result:

## Real-Terminal Raster Check

- Terminal app/version:
- Selected graphics mode:
- Screenshot paths:
- Native scrollback result:

## SSH Surface

- Command:
- Result:

## MCP Surface

- Command:
- Result:

## Notes

- Any deviations, limitations, or follow-up work:
```

- [ ] **Step 8: Commit implementation report**

Run:

```bash
git add reports/TEST_REPORT_2026-04-29_full-screen-inline-image-document-preview.md
git commit -m "test: record inline image preview verification"
```

Expected: commit succeeds.

---

## Final Verification Before Handoff

- [ ] Run `git status --short` and confirm only intentional files are modified.
- [ ] Run `go test ./...` and confirm PASS.
- [ ] Confirm `reports/TEST_REPORT_2026-04-29_full-screen-inline-image-document-preview.md` exists and includes real-terminal raster evidence or clearly explains why the raster terminal was unavailable.
- [ ] Confirm TC-23A in `TUI_TESTPLAN.md` mentions native scrollback and terminal app/version evidence.
- [ ] Confirm full-screen Timeline and Cleanup previews still exit with `z` and `Esc`.
- [ ] Confirm split preview still shows only the compact inline-image hint.
