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

func TestPreviewRowsFromRenderedImagePadsSinglePhysicalLineToReportedRows(t *testing.T) {
	rows := previewRowsFromRenderedImage(previewImageRenderResult{
		Content: "\x1b]1337;File=inline-image\a",
		Rows:    3,
	}, 80)

	if len(rows) != 3 {
		t.Fatalf("row count = %d, want 3", len(rows))
	}
	if rows[0].Content == "" {
		t.Fatalf("first row is empty, want rendered image content")
	}
	if rows[1].Content != "" || rows[2].Content != "" {
		t.Fatalf("padding rows = %q, %q; want empty rows", rows[1].Content, rows[2].Content)
	}
}

func TestPreviewRowsFromRenderedImageCapsPhysicalLinesToReportedRows(t *testing.T) {
	rows := previewRowsFromRenderedImage(previewImageRenderResult{
		Content: "[Image: first line\nsecond line]",
		Rows:    1,
	}, 80)

	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	if strings.Contains(rows[0].Content, "second line") {
		t.Fatalf("row content = %q, want only first physical line", rows[0].Content)
	}
}
