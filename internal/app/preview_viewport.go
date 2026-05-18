package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/herald-email/herald-mail-app/internal/kittyimg"
)

type previewLayoutOptions struct {
	InnerWidth    int
	AvailableRows int
	ImageMode     previewImageMode
	Descriptions  map[string]string
	ImageLinks    map[string]imagePreviewLink
}

type previewRenderedRow struct {
	Content          string
	TerminalConsumed bool
}

type previewDocumentLayout struct {
	ImageMode previewImageMode
	Rows      []previewRenderedRow
	TotalRows int
}

type previewNativeOverlay struct {
	Row     int
	Mode    previewImageMode
	Content string
}

type previewViewportRender struct {
	Content        string
	Rows           int
	NativeOverlays []previewNativeOverlay
}

func layoutPreviewDocument(doc previewDocument, opts previewLayoutOptions) previewDocumentLayout {
	if opts.InnerWidth < 1 {
		opts.InnerWidth = 1
	}
	if opts.AvailableRows < 1 {
		opts.AvailableRows = 1
	}

	var rows []previewRenderedRow
	for _, block := range doc.Blocks {
		startLen := len(rows)
		blockRows := layoutPreviewDocumentBlock(block, opts)
		if len(blockRows) == 0 {
			continue
		}
		if startLen > 0 {
			rows = append(rows, previewRenderedRow{})
		}
		rows = append(rows, blockRows...)
	}

	if len(rows) == 0 {
		rows = append(rows, previewRenderedRow{Content: "(No content)"})
	}
	return previewDocumentLayout{ImageMode: opts.ImageMode, Rows: rows, TotalRows: len(rows)}
}

func layoutPreviewDocumentBlock(block previewDocumentBlock, opts previewLayoutOptions) []previewRenderedRow {
	switch block.Kind {
	case previewBlockInlineImage:
		return layoutPreviewImageBlock(block, opts)
	default:
		return layoutPreviewTextBlock(block.Text, opts.InnerWidth)
	}
}

func layoutPreviewTextBlock(text string, innerWidth int) []previewRenderedRow {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	lines := renderEmailBodyLines(text, innerWidth)
	rows := make([]previewRenderedRow, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, previewRenderedRow{Content: ansi.Truncate(line, innerWidth, "")})
	}
	return rows
}

func layoutPreviewImageBlock(block previewDocumentBlock, opts previewLayoutOptions) []previewRenderedRow {
	key := normalizeContentID(block.ContentID)
	if key == "" {
		key = normalizeContentID(block.Image.ContentID)
	}
	desc := ""
	if opts.Descriptions != nil {
		desc = opts.Descriptions[key]
	}
	link := opts.ImageLinks[key]
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
	if rendered.Rows == 0 || rendered.Content == "" {
		return nil
	}

	return previewRowsFromRenderedImage(rendered, opts.InnerWidth)
}

func previewRowsFromRenderedImage(rendered previewImageRenderResult, innerWidth int) []previewRenderedRow {
	if rendered.TerminalConsumesRows {
		rows := []previewRenderedRow{{Content: rendered.Content}}
		for len(rows) < rendered.Rows {
			rows = append(rows, previewRenderedRow{TerminalConsumed: true})
		}
		return rows
	}

	contentLines := strings.Split(rendered.Content, "\n")
	rows := make([]previewRenderedRow, 0, rendered.Rows)
	for i, line := range contentLines {
		if i >= rendered.Rows {
			break
		}
		rows = append(rows, previewRenderedRow{Content: ansi.Truncate(line, innerWidth, "")})
	}
	for len(rows) < rendered.Rows {
		rows = append(rows, previewRenderedRow{})
	}
	return rows
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
	return renderPreviewDocumentViewportWithVisual(layout, offset, visibleRows, false, 0, 0)
}

func renderPreviewDocumentViewportWithVisual(layout previewDocumentLayout, offset, visibleRows int, visualMode bool, visualStart, visualEnd int) previewViewportRender {
	return renderPreviewDocumentViewportWithTheme(defaultTheme, layout, offset, visibleRows, visualMode, visualStart, visualEnd)
}

func renderPreviewDocumentViewportWithTheme(theme Theme, layout previewDocumentLayout, offset, visibleRows int, visualMode bool, visualStart, visualEnd int) previewViewportRender {
	if visibleRows < 1 {
		visibleRows = 1
	}

	offset = clampPreviewScrollOffset(offset, layout.TotalRows, visibleRows)
	end := offset + visibleRows
	if end > layout.TotalRows {
		end = layout.TotalRows
	}

	lines := make([]string, 0, visibleRows)
	nativeOverlays := make([]previewNativeOverlay, 0)
	lo, hi := visualStart, visualEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	highlightStyle := theme.Focus.VisualSelection.Style()
	for i := offset; i < end && i < len(layout.Rows); i++ {
		row := layout.Rows[i]
		viewportRow := len(lines)
		if row.TerminalConsumed {
			lines = append(lines, "")
			continue
		}
		content := row.Content
		if isNativePreviewImageContent(layout.ImageMode, content) {
			nativeOverlays = append(nativeOverlays, previewNativeOverlay{
				Row:     viewportRow,
				Mode:    layout.ImageMode,
				Content: content,
			})
			content = ""
		} else if visualMode && i >= lo && i <= hi {
			content = highlightStyle.Render(content)
		}
		lines = append(lines, content)
	}
	for len(lines) < visibleRows {
		lines = append(lines, "")
	}
	content := strings.Join(lines, "\n")
	return previewViewportRender{Content: content, Rows: visibleRows, NativeOverlays: nativeOverlays}
}

func isNativePreviewImageContent(mode previewImageMode, content string) bool {
	switch mode {
	case previewImageModeIterm2:
		return strings.Contains(content, "\x1b]1337;File=")
	case previewImageModeKitty:
		return strings.Contains(content, "\x1b_G")
	default:
		return false
	}
}

func renderNativeImageOverlayTail(overlays []previewNativeOverlay, originRow, originCol int) string {
	if len(overlays) == 0 {
		return ""
	}
	if originRow < 1 {
		originRow = 1
	}
	if originCol < 1 {
		originCol = 1
	}

	var b strings.Builder
	clearKitty := false
	for _, overlay := range overlays {
		if overlay.Mode == previewImageModeKitty {
			clearKitty = true
			break
		}
	}
	if clearKitty {
		b.WriteString(kittyimg.DeleteVisiblePlacements())
	}

	for _, overlay := range overlays {
		if overlay.Content == "" {
			continue
		}
		b.WriteString("\x1b7")
		b.WriteString(fmt.Sprintf("\x1b[%d;%dH", originRow+overlay.Row, originCol))
		b.WriteString(overlay.Content)
		b.WriteString("\x1b8")
	}
	return b.String()
}

func appendNativeImageOverlayTail(content, tail string) string {
	if tail == "" {
		return content
	}
	content = strings.TrimRight(content, "\n")
	return content + "\n" + tail
}

func appendNativeImageOverlayTailWithinRows(content, tail string, rows int) string {
	if tail == "" {
		return content
	}
	if rows < 1 {
		return appendNativeImageOverlayTail(content, tail)
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) >= rows {
		lines[rows-1] = tail
		return strings.Join(lines[:rows], "\n")
	}
	return strings.Join(append(lines, tail), "\n")
}
