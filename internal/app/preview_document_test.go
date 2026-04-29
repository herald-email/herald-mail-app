package app

import (
	"strings"
	"testing"

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
	plain := strings.Join(blockTexts(doc.Blocks), "\n")

	if !strings.Contains(plain, "Before") || !strings.Contains(plain, "After") {
		t.Fatalf("expected surrounding text, got %#v", doc.Blocks)
	}
	if !strings.Contains(plain, "![Logo](https://example.test/logo.png)") {
		t.Fatalf("expected linked remote image to remain visible, got %#v", doc.Blocks)
	}
	if strings.Contains(plain, "[](https://example.test)") || strings.Contains(plain, "[https://example.test](https://example.test)") {
		t.Fatalf("expected image link instead of empty/plain anchor link, got %#v", doc.Blocks)
	}
}
