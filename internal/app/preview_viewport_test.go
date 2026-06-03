package app

import (
	"bytes"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/herald-email/herald-mail-app/internal/kittyimg"
	"github.com/herald-email/herald-mail-app/internal/models"
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

	rendered := renderPreviewDocumentViewportWithThemeAndSafety(defaultTheme, layout, 0, 3, false, 0, 0, true)
	tail := renderNativeImageOverlayTail(rendered.NativeOverlays, 1, 1)
	if !strings.HasPrefix(tail, kittyimg.DeleteVisiblePlacements()) {
		t.Fatalf("Kitty overlay tail should clear previous placements before redraw, got %q", tail[:min(len(tail), 80)])
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

func TestRenderPreviewDocumentViewportReservesIterm2ConsumedPhysicalRows(t *testing.T) {
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
	if strings.Contains(rendered.Content, "\x1b]1337;File=") {
		t.Fatalf("iTerm2 native escape should not be embedded inline, got %q", rendered.Content)
	}
	if strings.Count(rendered.Content, "\n") != 4 {
		t.Fatalf("iTerm2 terminal-consumed rows should be blank reserved rows, got %q", rendered.Content)
	}
	if !strings.Contains(rendered.Content, "before") || !strings.Contains(rendered.Content, "after") {
		t.Fatalf("viewport lost surrounding text: %q", rendered.Content)
	}
	if len(rendered.NativeOverlays) != 1 {
		t.Fatalf("native overlays = %d, want 1", len(rendered.NativeOverlays))
	}
}

func TestRenderPreviewDocumentViewportSuppressesNativeOverlayWhenImageWouldOverflow(t *testing.T) {
	layout := previewDocumentLayout{
		ImageMode: previewImageModeIterm2,
		Rows: []previewRenderedRow{
			{Content: "line 1"},
			{Content: "line 2"},
			{Content: "\x1b]1337;File=inline=1;width=64;height=3:payload\a"},
			{TerminalConsumed: true},
			{TerminalConsumed: true},
			{Content: "after"},
		},
		TotalRows: 6,
	}

	rendered := renderPreviewDocumentViewport(layout, 0, 4)

	if len(rendered.NativeOverlays) != 0 {
		t.Fatalf("native overlays = %d, want 0 when the image rows would overflow the viewport", len(rendered.NativeOverlays))
	}
	if strings.Contains(rendered.Content, "\x1b]1337;File=") {
		t.Fatalf("overflowing image escape should not be emitted inline: %q", rendered.Content)
	}
}

func TestRenderPreviewDocumentViewportClipsNativeOverlayAtViewportBottom(t *testing.T) {
	layout := layoutPreviewDocument(previewDocument{Blocks: []previewDocumentBlock{
		{Kind: previewBlockInlineImage, Image: models.InlineImage{ContentID: "landscape", MIMEType: "image/png", Data: tinyPNG(t, 960, 540)}, Alt: "Landscape"},
	}}, previewLayoutOptions{
		InnerWidth:    160,
		AvailableRows: 18,
		ImageMode:     previewImageModeIterm2,
	})

	rendered := renderPreviewDocumentViewport(layout, 0, 5)

	if len(rendered.NativeOverlays) != 1 {
		t.Fatalf("native overlays = %d, want one clipped bottom slice", len(rendered.NativeOverlays))
	}
	if rendered.NativeOverlays[0].Row != 0 || rendered.NativeOverlays[0].Rows != 5 {
		t.Fatalf("overlay geometry = row %d rows %d, want row 0 rows 5", rendered.NativeOverlays[0].Row, rendered.NativeOverlays[0].Rows)
	}
	tail := renderNativeImageOverlayTail(rendered.NativeOverlays, 1, 1)
	if height := itermImageDimension(t, tail, "height"); height != 5 {
		t.Fatalf("clipped image height = %d, want 5; raw=%q", height, tail)
	}
}

func TestRenderPreviewDocumentViewportClipsNativeOverlayAtViewportTop(t *testing.T) {
	layout := layoutPreviewDocument(previewDocument{Blocks: []previewDocumentBlock{
		{Kind: previewBlockInlineImage, Image: models.InlineImage{ContentID: "landscape", MIMEType: "image/png", Data: tinyPNG(t, 960, 540)}, Alt: "Landscape"},
	}}, previewLayoutOptions{
		InnerWidth:    160,
		AvailableRows: 18,
		ImageMode:     previewImageModeIterm2,
	})

	rendered := renderPreviewDocumentViewport(layout, 4, 5)

	if len(rendered.NativeOverlays) != 1 {
		t.Fatalf("native overlays = %d, want one clipped top slice", len(rendered.NativeOverlays))
	}
	if rendered.NativeOverlays[0].Row != 0 || rendered.NativeOverlays[0].Rows != 5 {
		t.Fatalf("overlay geometry = row %d rows %d, want row 0 rows 5", rendered.NativeOverlays[0].Row, rendered.NativeOverlays[0].Rows)
	}
	if rendered.NativeOverlays[0].ClipTopRows != 4 {
		t.Fatalf("clip top rows = %d, want 4", rendered.NativeOverlays[0].ClipTopRows)
	}
	tail := renderNativeImageOverlayTail(rendered.NativeOverlays, 1, 1)
	if height := itermImageDimension(t, tail, "height"); height != 5 {
		t.Fatalf("clipped image height = %d, want 5; raw=%q", height, tail)
	}
}

func TestRenderPreviewDocumentViewportSuppressesNativeOverlayAtViewportBottom(t *testing.T) {
	layout := previewDocumentLayout{
		ImageMode: previewImageModeIterm2,
		Rows: []previewRenderedRow{
			{Content: "\x1b]1337;File=inline=1;width=64;height=3:payload\a"},
			{TerminalConsumed: true},
			{TerminalConsumed: true},
		},
		TotalRows: 3,
	}

	rendered := renderPreviewDocumentViewportWithThemeAndSafety(defaultTheme, layout, 0, 3, false, 0, 0, true)

	if len(rendered.NativeOverlays) != 0 {
		t.Fatalf("native overlays = %d, want 0 when no safety row remains below the image", len(rendered.NativeOverlays))
	}
	if strings.Contains(rendered.Content, "\x1b]1337;File=") {
		t.Fatalf("bottom-aligned image escape should not be emitted inline: %q", rendered.Content)
	}
}

func TestFilterNativeOverlaysWithinBottomRowUsesRenderedImageHeight(t *testing.T) {
	overlays := []previewNativeOverlay{
		{Row: 5, Rows: 4, Mode: previewImageModeIterm2, Content: "\x1b]1337;File=inline=1;width=64;height=4:fits\a"},
		{Row: 8, Rows: 6, Mode: previewImageModeIterm2, Content: "\x1b]1337;File=inline=1;width=64;height=6:overlaps\a"},
	}

	filtered := filterNativeOverlaysWithinBottomRow(overlays, 10, 18)

	if len(filtered) != 1 {
		t.Fatalf("filtered overlays = %d, want only the fitting image", len(filtered))
	}
	if filtered[0].Content != overlays[0].Content {
		t.Fatalf("kept overlay = %q, want first fitting overlay", filtered[0].Content)
	}
}

func TestFilterNativeOverlaysWithinBottomRowClipsOverflowingBottomSlice(t *testing.T) {
	source := &previewNativeImageSource{
		Image: models.InlineImage{ContentID: "landscape", MIMEType: "image/png", Data: tinyPNG(t, 960, 540)},
		Width: 64,
		Rows:  8,
	}
	overlay := previewNativeOverlay{
		Row:     4,
		Rows:    8,
		Mode:    previewImageModeIterm2,
		Content: "\x1b]1337;File=inline=1;width=64;height=8:full\a",
		Source:  source,
	}

	filtered := filterNativeOverlaysWithinBottomRow([]previewNativeOverlay{overlay}, 10, 16)

	if len(filtered) != 1 {
		t.Fatalf("filtered overlays = %d, want clipped overlay", len(filtered))
	}
	if filtered[0].Rows != 3 {
		t.Fatalf("clipped rows = %d, want 3", filtered[0].Rows)
	}
	tail := renderNativeImageOverlayTail(filtered, 10, 1)
	if height := itermImageDimension(t, tail, "height"); height != 3 {
		t.Fatalf("clipped panel image height = %d, want 3; raw=%q", height, tail)
	}
}

func TestRenderPreviewDocumentViewportPreservesNativeImageEscapesThroughV2Renderer(t *testing.T) {
	layout := previewDocumentLayout{
		ImageMode: previewImageModeIterm2,
		Rows: []previewRenderedRow{
			{Content: "before"},
			{Content: "\x1b]1337;File=inline=1;width=12;height=2:payload\a"},
			{TerminalConsumed: true},
			{Content: "after"},
		},
		TotalRows: 4,
	}

	rendered := renderPreviewDocumentViewport(layout, 0, 4)
	if strings.Contains(rendered.Content, "\x1b]1337;File=") {
		t.Fatalf("native image escape should be carried by overlay tail, not inline content: %q", rendered.Content)
	}
	if len(rendered.NativeOverlays) != 1 {
		t.Fatalf("native overlays = %d, want 1", len(rendered.NativeOverlays))
	}
	if rendered.NativeOverlays[0].Row != 1 {
		t.Fatalf("native overlay row = %d, want 1", rendered.NativeOverlays[0].Row)
	}

	terminalBytes := renderStyledStringThroughV2TestRenderer(rendered.Content + renderNativeImageOverlayTail(rendered.NativeOverlays, 1, 1))
	if !strings.Contains(terminalBytes, "\x1b]1337;File=") {
		t.Fatalf("v2 renderer output lost native image escape:\n%q", terminalBytes)
	}
}

func TestAppendNativeImageOverlayTailKeepsTailInsideFullHeightView(t *testing.T) {
	fullHeightContent := strings.TrimSuffix(strings.Repeat("x\n", 20), "\n")
	tail := renderNativeImageOverlayTail([]previewNativeOverlay{{
		Row:     1,
		Mode:    previewImageModeIterm2,
		Content: "\x1b]1337;File=inline=1;width=12;height=2:payload\a",
	}}, 1, 1)

	terminalBytes := renderStyledStringThroughV2TestRenderer(appendNativeImageOverlayTailWithinRows(fullHeightContent+"\n", tail, 20))
	if !strings.Contains(terminalBytes, "\x1b]1337;File=") {
		t.Fatalf("v2 renderer clipped native image tail after full-height content:\n%q", terminalBytes)
	}
}

func renderStyledStringThroughV2TestRenderer(content string) string {
	return renderStyledStringThroughSizedV2TestRenderer(content, 120, 20)
}

func renderStyledStringThroughSizedV2TestRenderer(content string, width, height int) string {
	var out bytes.Buffer
	renderer := uv.NewTerminalRenderer(&out, nil)
	screen := uv.NewScreenBuffer(width, height)
	uv.NewStyledString(content).Draw(screen, screen.Bounds())
	renderer.Render(screen.RenderBuffer)
	_ = renderer.Flush()
	return out.String()
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
