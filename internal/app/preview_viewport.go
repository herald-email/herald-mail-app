package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/herald-email/herald-mail-app/internal/kittyimg"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/render"
)

type previewLayoutOptions struct {
	InnerWidth    int
	AvailableRows int
	ImageMode     previewImageMode
	Descriptions  map[string]string
	ImageLinks    map[string]imagePreviewLink
	RemoteImages  map[string]previewRemoteImageState
}

type previewRenderedRow struct {
	Content          string
	TerminalConsumed bool
	NativeImageRows  int
	NativeImage      *previewNativeImageSource
}

type previewDocumentLayout struct {
	ImageMode previewImageMode
	Rows      []previewRenderedRow
	TotalRows int
}

type previewNativeOverlay struct {
	Row         int
	Rows        int
	Mode        previewImageMode
	Content     string
	ClipTopRows int
	Source      *previewNativeImageSource
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
	case previewBlockRemoteImage:
		return layoutPreviewRemoteImageBlock(block, opts)
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

func layoutPreviewRemoteImageBlock(block previewDocumentBlock, opts previewLayoutOptions) []previewRenderedRow {
	key := block.Remote.Key
	if key == "" {
		key = remoteImageDocumentKey(block.Remote.URL)
	}
	state := opts.RemoteImages[key]
	if len(state.Image.Data) == 0 {
		return []previewRenderedRow{{Content: remoteImagePlaceholderLine(block.Remote, state, opts.InnerWidth)}}
	}
	image := state.Image
	if image.ContentID == "" {
		image.ContentID = key
	}
	if image.MIMEType == "" {
		image.MIMEType = "image"
	}
	link := opts.ImageLinks[normalizeContentID(key)]
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          opts.ImageMode,
		Image:         image,
		Alt:           block.Remote.Alt,
		InnerWidth:    opts.InnerWidth,
		AvailableRows: opts.AvailableRows,
		LinkLabel:     link.Label,
		LinkURL:       link.URL,
	})
	if rendered.Rows == 0 || rendered.Content == "" {
		return []previewRenderedRow{{Content: remoteImagePlaceholderLine(block.Remote, state, opts.InnerWidth)}}
	}
	return previewRowsFromRenderedImage(rendered, opts.InnerWidth)
}

func remoteImagePlaceholderLine(remote previewRemoteImage, state previewRemoteImageState, innerWidth int) string {
	label := remoteImageDisplayLabel(remote)
	text := fmt.Sprintf("image: %s (press o to reveal)", label)
	if state.Loading {
		text = fmt.Sprintf("image: %s (loading...)", label)
	} else if state.Err != "" {
		text = fmt.Sprintf("image: %s (reveal failed; press o to retry)", label)
	}
	if remote.URL != "" {
		text = render.TerminalHyperlink(text, render.SanitizePreviewURLTarget(remote.URL))
	}
	return truncateVisual(text, innerWidth)
}

func previewDocumentRemoteImages(doc previewDocument) []previewRemoteImage {
	seen := make(map[string]bool)
	images := make([]previewRemoteImage, 0)
	for _, block := range doc.Blocks {
		if block.Kind != previewBlockRemoteImage || block.Remote.URL == "" {
			continue
		}
		key := block.Remote.Key
		if key == "" {
			key = remoteImageDocumentKey(block.Remote.URL)
			block.Remote.Key = key
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		images = append(images, block.Remote)
	}
	return images
}

func previewDocumentRenderableImages(doc previewDocument, remoteImages map[string]previewRemoteImageState) []models.InlineImage {
	images := make([]models.InlineImage, 0)
	seen := make(map[string]bool)
	for _, block := range doc.Blocks {
		switch block.Kind {
		case previewBlockInlineImage:
			image := block.Image
			key := normalizeContentID(block.ContentID)
			if key == "" {
				key = normalizeContentID(image.ContentID)
			}
			if key == "" {
				key = inlineImageDocumentKey(image, len(images))
			}
			if image.ContentID == "" {
				image.ContentID = key
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			images = append(images, image)
		case previewBlockRemoteImage:
			key := block.Remote.Key
			if key == "" {
				key = remoteImageDocumentKey(block.Remote.URL)
			}
			if seen[key] {
				continue
			}
			state := remoteImages[key]
			if len(state.Image.Data) == 0 {
				continue
			}
			image := state.Image
			if image.ContentID == "" {
				image.ContentID = key
			}
			seen[key] = true
			images = append(images, image)
		}
	}
	return images
}

func previewRowsFromRenderedImage(rendered previewImageRenderResult, innerWidth int) []previewRenderedRow {
	if rendered.TerminalConsumesRows {
		rows := []previewRenderedRow{{Content: rendered.Content, NativeImageRows: rendered.Rows, NativeImage: rendered.NativeImage}}
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
		row := previewRenderedRow{Content: ansi.Truncate(line, innerWidth, "")}
		if i == 0 {
			row.NativeImageRows = rendered.Rows
			row.NativeImage = rendered.NativeImage
		}
		rows = append(rows, row)
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
	return renderPreviewDocumentViewportWithThemeAndSafety(theme, layout, offset, visibleRows, visualMode, visualStart, visualEnd, false)
}

func renderPreviewDocumentViewportWithThemeAndSafety(theme Theme, layout previewDocumentLayout, offset, visibleRows int, visualMode bool, visualStart, visualEnd int, requireNativeImageSafetyRow bool) previewViewportRender {
	if visibleRows < 1 {
		visibleRows = 1
	}

	offset = clampPreviewScrollOffset(offset, layout.TotalRows, visibleRows)
	end := offset + visibleRows
	if end > layout.TotalRows {
		end = layout.TotalRows
	}

	lines := make([]string, 0, visibleRows)
	nativeOverlays := nativeOverlaysForViewport(layout, offset, end, requireNativeImageSafetyRow)
	lo, hi := visualStart, visualEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	highlightStyle := theme.Focus.VisualSelection.Style()
	for i := offset; i < end && i < len(layout.Rows); i++ {
		row := layout.Rows[i]
		if row.TerminalConsumed {
			lines = append(lines, "")
			continue
		}
		content := row.Content
		if isNativePreviewImageContent(layout.ImageMode, content) {
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

func nativeOverlaysForViewport(layout previewDocumentLayout, offset, end int, requireSafetyRow bool) []previewNativeOverlay {
	if end <= offset || len(layout.Rows) == 0 {
		return nil
	}
	overlays := make([]previewNativeOverlay, 0)
	for imageRow, row := range layout.Rows {
		if !isNativePreviewImageContent(layout.ImageMode, row.Content) {
			continue
		}
		imageRows := nativePreviewImageRows(layout.Rows, imageRow)
		imageEnd := imageRow + imageRows
		if imageEnd <= offset || imageRow >= end {
			continue
		}
		if requireSafetyRow && imageEnd >= end {
			continue
		}
		visibleStart := maxInt(imageRow, offset)
		visibleEnd := minInt(imageEnd, end)
		visibleRows := visibleEnd - visibleStart
		if visibleRows < 1 {
			continue
		}
		clipTopRows := visibleStart - imageRow
		content := row.Content
		if clipTopRows > 0 || visibleRows < imageRows {
			clipped, ok := renderClippedPreviewNativeImage(row.NativeImage, layout.ImageMode, clipTopRows, visibleRows)
			if !ok {
				continue
			}
			content = clipped
		}
		overlays = append(overlays, previewNativeOverlay{
			Row:         visibleStart - offset,
			Rows:        visibleRows,
			Mode:        layout.ImageMode,
			Content:     content,
			ClipTopRows: clipTopRows,
			Source:      row.NativeImage,
		})
	}
	return overlays
}

func nativePreviewImageFitsViewport(rows []previewRenderedRow, imageRow, viewportEnd int, requireSafetyRow bool) bool {
	if imageRow < 0 || imageRow >= len(rows) {
		return false
	}
	imageEnd := imageRow + nativePreviewImageRows(rows, imageRow)
	if requireSafetyRow {
		return imageEnd < viewportEnd
	}
	return imageEnd <= viewportEnd
}

func nativePreviewImageRows(rows []previewRenderedRow, imageRow int) int {
	if imageRow < 0 || imageRow >= len(rows) {
		return 1
	}
	if rows[imageRow].NativeImageRows > 0 {
		return rows[imageRow].NativeImageRows
	}
	imageEnd := imageRow + 1
	for imageEnd < len(rows) && rows[imageEnd].TerminalConsumed {
		imageEnd++
	}
	return maxInt(1, imageEnd-imageRow)
}

func filterNativeOverlaysWithinBottomRow(overlays []previewNativeOverlay, originRow, maxBottomRow int) []previewNativeOverlay {
	if len(overlays) == 0 || maxBottomRow < 1 {
		return overlays
	}
	filtered := overlays[:0]
	for _, overlay := range overlays {
		rows := overlay.Rows
		if rows < 1 {
			rows = 1
		}
		top := originRow + overlay.Row
		bottom := top + rows - 1
		if bottom <= maxBottomRow {
			filtered = append(filtered, overlay)
			continue
		}
		visibleRows := maxBottomRow - top + 1
		if visibleRows < 1 {
			continue
		}
		clipped, ok := renderClippedPreviewNativeImage(overlay.Source, overlay.Mode, overlay.ClipTopRows, visibleRows)
		if !ok {
			continue
		}
		overlay.Rows = visibleRows
		overlay.Content = clipped
		filtered = append(filtered, overlay)
	}
	return filtered
}

func previewLayoutPlainRows(layout previewDocumentLayout) []string {
	rows := make([]string, 0, len(layout.Rows))
	for _, row := range layout.Rows {
		rows = append(rows, ansi.Strip(row.Content))
	}
	return rows
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

func appendNativeImageOverlayTailToLastLine(content, tail string) string {
	if tail == "" {
		return content
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 0 {
		return tail
	}
	lines[len(lines)-1] += "\r" + tail
	return strings.Join(lines, "\n")
}
