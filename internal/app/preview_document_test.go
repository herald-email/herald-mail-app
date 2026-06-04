package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
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

	if got, want := len(doc.Blocks), 6; got != want {
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
	if doc.Blocks[5].Kind != previewBlockText || !strings.Contains(doc.Blocks[5].Text, "After bee") {
		t.Fatalf("sixth block should be text after bee, got %#v", doc.Blocks[5])
	}
}

func TestBuildPreviewDocument_RemoteImagesBecomeRevealBlocksAndMissingCIDIsPlaceholder(t *testing.T) {
	body := &models.EmailBody{
		TextHTML: `<p>Logo below</p>
            <img alt="Remote logo" title="Title logo" src="https://example.test/logo.png?utm_source=email&id=42">
            <img alt="Missing" src="cid:not-found">`,
	}

	doc := buildPreviewDocument(body, nil)
	plain := strings.Join(blockTexts(doc.Blocks), "\n")

	if !strings.Contains(plain, "Logo below") {
		t.Fatalf("expected surrounding text, got %#v", doc.Blocks)
	}
	if got, want := len(doc.Blocks), 3; got != want {
		t.Fatalf("block count = %d, want %d: %#v", got, want, doc.Blocks)
	}
	remote := doc.Blocks[1]
	if remote.Kind != previewBlockRemoteImage {
		t.Fatalf("second block should be remote image reveal block, got %#v", remote)
	}
	if remote.Remote.URL != "https://example.test/logo.png?id=42" || remote.Remote.Alt != "Remote logo" || remote.Remote.Title != "Title logo" {
		t.Fatalf("remote image metadata mismatch: %#v", remote.Remote)
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

func TestBuildPreviewDocument_OrphanInlineImageWithoutCIDStillRenders(t *testing.T) {
	body := &models.EmailBody{
		TextPlain: "Intro paragraph.",
		InlineImages: []models.InlineImage{
			{MIMEType: "image/png", Data: []byte("png")},
		},
	}

	doc := buildPreviewDocument(body, nil)

	if got, want := len(doc.Blocks), 3; got != want {
		t.Fatalf("block count = %d, want %d: %#v", got, want, doc.Blocks)
	}
	imageBlock := doc.Blocks[2]
	if imageBlock.Kind != previewBlockInlineImage {
		t.Fatalf("third block should be orphan image, got %#v", imageBlock)
	}
	if imageBlock.ContentID == "" || imageBlock.Image.ContentID == "" {
		t.Fatalf("no-CID orphan image should get a stable document key, got %#v", imageBlock)
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

func TestBuildPreviewDocument_AnchorWrappedCIDImageStaysInAuthoredOrder(t *testing.T) {
	body := &models.EmailBody{
		TextHTML: `<p>Before</p><a href="https://example.test"><img alt="Logo" src="cid:logo"></a><p>After</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("logo")},
		},
	}

	doc := buildPreviewDocument(body, nil)

	if got, want := len(doc.Blocks), 3; got != want {
		t.Fatalf("block count = %d, want %d: %#v", got, want, doc.Blocks)
	}
	if doc.Blocks[0].Kind != previewBlockText || !strings.Contains(doc.Blocks[0].Text, "Before") {
		t.Fatalf("first block should be text before image, got %#v", doc.Blocks[0])
	}
	if doc.Blocks[1].Kind != previewBlockInlineImage || doc.Blocks[1].Image.ContentID != "logo" {
		t.Fatalf("second block should be linked logo image, got %#v", doc.Blocks[1])
	}
	if doc.Blocks[2].Kind != previewBlockText || !strings.Contains(doc.Blocks[2].Text, "After") {
		t.Fatalf("third block should be text after image, got %#v", doc.Blocks[2])
	}

	imageCount := 0
	for _, block := range doc.Blocks {
		if block.Kind == previewBlockInlineImage {
			imageCount++
		}
	}
	if imageCount != 1 {
		t.Fatalf("linked image should render once, got %d image blocks: %#v", imageCount, doc.Blocks)
	}
}

func TestBuildPreviewDocument_AnchorWrappedRemoteImageRemainsVisible(t *testing.T) {
	body := &models.EmailBody{
		TextHTML: `<p>Before</p><a href="https://example.test"><img alt="Logo" src="https://example.test/logo.png"></a><p>After</p>`,
	}

	doc := buildPreviewDocument(body, nil)

	if got, want := len(doc.Blocks), 3; got != want {
		t.Fatalf("block count = %d, want %d: %#v", got, want, doc.Blocks)
	}
	if doc.Blocks[0].Kind != previewBlockText || !strings.Contains(doc.Blocks[0].Text, "Before") {
		t.Fatalf("expected text before remote image, got %#v", doc.Blocks)
	}
	if doc.Blocks[1].Kind != previewBlockRemoteImage {
		t.Fatalf("expected linked remote image reveal block, got %#v", doc.Blocks)
	}
	if doc.Blocks[1].Remote.URL != "https://example.test/logo.png" || doc.Blocks[1].Remote.Alt != "Logo" {
		t.Fatalf("remote image metadata mismatch: %#v", doc.Blocks[1].Remote)
	}
	if doc.Blocks[2].Kind != previewBlockText || !strings.Contains(doc.Blocks[2].Text, "After") {
		t.Fatalf("expected surrounding text, got %#v", doc.Blocks)
	}
	plain := strings.Join(blockTexts(doc.Blocks), "\n")
	if strings.Contains(plain, "[](https://example.test)") || strings.Contains(plain, "[https://example.test](https://example.test)") {
		t.Fatalf("expected image link instead of empty/plain anchor link, got %#v", doc.Blocks)
	}
}

func TestBuildPreviewDocument_RendersHTMLSignatureLinkWithoutMarkdownSyntax(t *testing.T) {
	body := &models.EmailBody{
		TextHTML: `<p>test<br>--<br>Cheers, Anton<br>Sent with Herald · <a href="https://herald-mail.app">herald-mail.app</a></p>`,
	}

	doc := buildPreviewDocument(body, nil)
	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    100,
		AvailableRows: 20,
		ImageMode:     previewImageModePlaceholder,
	})

	var renderedRows []string
	for _, row := range layout.Rows {
		renderedRows = append(renderedRows, row.Content)
	}
	rendered := strings.Join(renderedRows, "\n")
	visible := ansi.Strip(rendered)

	if strings.Contains(visible, "[herald-mail.app]") || strings.Contains(visible, "](https://herald-mail.app)") {
		t.Fatalf("expected received HTML signature link syntax to be rendered away, got:\n%s", visible)
	}
	if !strings.Contains(visible, "Sent with Herald · herald-mail.app") {
		t.Fatalf("expected received HTML signature link label to remain visible, got:\n%s", visible)
	}
	if !strings.Contains(rendered, "\x1b]8;;https://herald-mail.app\x1b\\") {
		t.Fatalf("expected received HTML signature link target to survive as OSC 8, got raw:\n%q", rendered)
	}
}
