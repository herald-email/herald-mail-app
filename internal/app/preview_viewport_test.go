package app

import (
	"strings"
	"testing"

	"mail-processor/internal/kittyimg"
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

func TestRenderPreviewDocumentViewportClearsKittyPlacements(t *testing.T) {
	layout := previewDocumentLayout{
		ImageMode: previewImageModeKitty,
		Rows: []previewRenderedRow{
			{Content: "first"},
			{Content: "\x1b_Ga=T,f=100,t=d,q=2,c=10,r=4;payload\x1b\\"},
			{Content: "third"},
		},
		TotalRows: 3,
	}

	rendered := renderPreviewDocumentViewport(layout, 0, 3)
	if !strings.HasPrefix(rendered.Content, kittyimg.DeleteVisiblePlacements()) {
		t.Fatalf("Kitty viewport should clear previous placements before redraw, got %q", rendered.Content[:min(len(rendered.Content), 80)])
	}
	if rendered.Rows != 3 {
		t.Fatalf("rendered rows = %d, want 3", rendered.Rows)
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

func TestPreviewRowsFromIterm2ImageMarksConsumedRows(t *testing.T) {
	rows := previewRowsFromRenderedImage(previewImageRenderResult{
		Content:              "\x1b]1337;File=inline=1;width=64;height=18:payload\a",
		Rows:                 4,
		TerminalConsumesRows: true,
	}, 80)

	if len(rows) != 4 {
		t.Fatalf("row count = %d, want 4", len(rows))
	}
	if rows[0].TerminalConsumed {
		t.Fatalf("first row must carry the OSC output")
	}
	for i := 1; i < len(rows); i++ {
		if !rows[i].TerminalConsumed {
			t.Fatalf("row %d should be terminal-consumed reservation: %#v", i, rows[i])
		}
	}
}

func TestPreviewRowsFromIterm2ControlBlockKeepsProtocolAtomic(t *testing.T) {
	rendered := previewImageRenderResult{
		Content:              "\r\x1b[2K\x1b[64C\n\r\x1b[2K\x1b[64C\n\r\x1b[2K\x1b[2A\x1b]1337;File=inline=1;width=64;height=3:payload\a",
		Rows:                 3,
		TerminalConsumesRows: true,
	}

	rows := previewRowsFromRenderedImage(rendered, 80)

	if len(rows) != 3 {
		t.Fatalf("row count = %d, want 3", len(rows))
	}
	if rows[0].TerminalConsumed {
		t.Fatal("first row must carry the complete iTerm2 control block")
	}
	if rows[0].Content != rendered.Content {
		t.Fatalf("iTerm2 control block was split or truncated:\ngot  %q\nwant %q", rows[0].Content, rendered.Content)
	}
	for i := 1; i < len(rows); i++ {
		if !rows[i].TerminalConsumed {
			t.Fatalf("row %d should be terminal-consumed reservation: %#v", i, rows[i])
		}
	}
}

func TestRenderPreviewDocumentViewportSkipsIterm2ConsumedPhysicalRows(t *testing.T) {
	layout := previewDocumentLayout{
		ImageMode: previewImageModeIterm2,
		Rows: []previewRenderedRow{
			{Content: "before"},
			{Content: "\x1b]1337;File=inline=1;width=64;height=3:payload\a"},
			{TerminalConsumed: true},
			{TerminalConsumed: true},
			{Content: "after"},
		},
		TotalRows: 5,
	}

	rendered := renderPreviewDocumentViewport(layout, 0, 5)

	if rendered.Rows != 5 {
		t.Fatalf("rendered rows = %d, want logical viewport rows 5", rendered.Rows)
	}
	if strings.Count(rendered.Content, "\n") != 2 {
		t.Fatalf("iTerm2 terminal-consumed rows should not print duplicate blank lines, got %q", rendered.Content)
	}
	if !strings.Contains(rendered.Content, "before") || !strings.Contains(rendered.Content, "after") {
		t.Fatalf("viewport lost surrounding text: %q", rendered.Content)
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
