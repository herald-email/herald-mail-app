package app

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/iterm2"
	"mail-processor/internal/models"
	"mail-processor/internal/render"
)

const decodedPreviewImageMaxRows = 18

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
	LinkLabel     string
	LinkURL       string
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
	widthCap := innerW - 2
	if widthCap < 1 {
		widthCap = 1
	}
	rowCap := minInt(availableRows, decodedPreviewImageMaxRows)
	if rowCap < 1 {
		rowCap = 1
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		rows := availableRows / 2
		if rows < 1 {
			rows = 1
		}
		rows = minInt(rows, maxPreviewImageRows)
		rows = minInt(rows, rowCap)
		return previewImageSize{Width: widthCap, Rows: rows}
	}

	widthCells := (cfg.Width + 7) / 8
	rowCells := (cfg.Height + 15) / 16
	if widthCells < 1 {
		widthCells = 1
	}
	if rowCells < 1 {
		rowCells = 1
	}
	if widthCells > widthCap {
		widthCells = widthCap
	}
	if rowCells > rowCap {
		rowCells = rowCap
	}
	return previewImageSize{Width: widthCells, Rows: rowCells}
}

func renderPreviewImageBlock(req previewImageRenderRequest) previewImageRenderResult {
	mode := req.Mode
	if mode == "" || mode == previewImageModeAuto {
		mode = detectPreviewImageMode(previewImageModeAuto, false, false)
	}
	if mode == previewImageModeOff || req.AvailableRows <= 0 {
		return previewImageRenderResult{}
	}
	if len(req.Image.Data) == 0 {
		return oneLinePreviewImageFallback("[image unavailable: empty data]", req.InnerWidth)
	}

	switch mode {
	case previewImageModeIterm2:
		if len(req.Image.Data) > maxPreviewImageBytes {
			return oneLinePreviewImageFallback(fmt.Sprintf("[image too large to render inline: %s]", req.Image.MIMEType), req.InnerWidth)
		}
		size := previewImageCellSize(req.Image, req.InnerWidth, req.AvailableRows)
		rendered := strings.TrimRight(iterm2.Render(req.Image.Data, size.Width, size.Rows), "\n")
		if rendered == "" {
			return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
		}
		return previewImageRenderResult{Content: rendered, Rows: size.Rows}
	case previewImageModeLinks:
		if req.LinkURL != "" && req.LinkLabel != "" {
			label := render.TerminalHyperlink("["+req.LinkLabel+"]", req.LinkURL)
			meta := fmt.Sprintf(" %s  %d KB", req.Image.MIMEType, len(req.Image.Data)/1024)
			return previewImageRenderResult{Content: renderPreviewImageLinkLine(req, label+meta), Rows: 1}
		}
		return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
	case previewImageModePlaceholder:
		return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
	default:
		return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
	}
}

func renderPreviewImageLinkLine(req previewImageRenderRequest, linkText string) string {
	prefix := previewImageLinkPrefix(req)
	if prefix == "" {
		return truncateVisual(linkText, req.InnerWidth)
	}
	linkWidth := ansi.StringWidth(linkText)
	prefixWidth := req.InnerWidth - linkWidth
	if prefixWidth < 2 {
		return truncateVisual(linkText, req.InnerWidth)
	}
	return truncateVisual(prefix, prefixWidth) + truncateVisual(linkText, req.InnerWidth-prefixWidth)
}

func previewImageLinkPrefix(req previewImageRenderRequest) string {
	if desc := strings.TrimSpace(req.Description); desc != "" {
		return desc + " "
	}
	if alt := strings.TrimSpace(req.Alt); isUsefulPreviewAlt(alt) {
		return alt + " "
	}
	return ""
}

func previewImagePlaceholderText(req previewImageRenderRequest) string {
	if desc := strings.TrimSpace(req.Description); desc != "" {
		return fmt.Sprintf("[Image: %s]", desc)
	}
	if alt := strings.TrimSpace(req.Alt); isUsefulPreviewAlt(alt) {
		return fmt.Sprintf("[Image: %s]", alt)
	}
	mime := strings.TrimSpace(req.Image.MIMEType)
	if mime == "" {
		mime = "image"
	}
	return fmt.Sprintf("[image: %s  %d KB]", mime, len(req.Image.Data)/1024)
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

func isUsefulPreviewAlt(alt string) bool {
	if alt == "" {
		return false
	}
	switch strings.ToLower(alt) {
	case "image", "photo", "picture", "inline image", "attachment":
		return false
	default:
		return true
	}
}
