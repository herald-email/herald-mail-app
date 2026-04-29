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
	"mail-processor/internal/kittyimg"
	"mail-processor/internal/models"
	"mail-processor/internal/render"
)

const decodedPreviewImageMaxRows = 18

type PreviewImageMode string

type previewImageMode = PreviewImageMode

const (
	PreviewImageModeAuto        PreviewImageMode = "auto"
	PreviewImageModeIterm2      PreviewImageMode = "iterm2"
	PreviewImageModeKitty       PreviewImageMode = "kitty"
	PreviewImageModeLinks       PreviewImageMode = "links"
	PreviewImageModePlaceholder PreviewImageMode = "placeholder"
	PreviewImageModeOff         PreviewImageMode = "off"

	previewImageModeAuto        previewImageMode = PreviewImageModeAuto
	previewImageModeIterm2      previewImageMode = PreviewImageModeIterm2
	previewImageModeKitty       previewImageMode = PreviewImageModeKitty
	previewImageModeLinks       previewImageMode = PreviewImageModeLinks
	previewImageModePlaceholder previewImageMode = PreviewImageModePlaceholder
	previewImageModeOff         previewImageMode = PreviewImageModeOff
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
	Content              string
	Rows                 int
	TerminalConsumesRows bool
}

func detectPreviewImageMode(requested previewImageMode, localLinks bool, sshMode bool) previewImageMode {
	switch requested {
	case previewImageModeIterm2, previewImageModeKitty, previewImageModeLinks, previewImageModePlaceholder, previewImageModeOff:
		return requested
	}
	if sshMode {
		return previewImageModePlaceholder
	}
	if iterm2.IsSupported() {
		return previewImageModeIterm2
	}
	if kittyimg.IsSupported() {
		return previewImageModeKitty
	}
	if localLinks {
		return previewImageModeLinks
	}
	return previewImageModePlaceholder
}

func ParsePreviewImageMode(value string) (PreviewImageMode, error) {
	mode := PreviewImageMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case PreviewImageModeAuto, PreviewImageModeIterm2, PreviewImageModeKitty, PreviewImageModeLinks, PreviewImageModePlaceholder, PreviewImageModeOff:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid image protocol %q (valid: auto, iterm2, kitty, links, placeholder, off)", value)
	}
}

func previewImageCellSize(img models.InlineImage, innerW, availableRows int) previewImageSize {
	return previewImageCellSizeForMode(previewImageModeKitty, img, innerW, availableRows)
}

func previewImageCellSizeForMode(mode previewImageMode, img models.InlineImage, innerW, availableRows int) previewImageSize {
	if mode == previewImageModeIterm2 {
		return previewIterm2ImageCellSize(img, innerW, availableRows)
	}
	return previewAspectFitImageCellSize(img, innerW, availableRows, decodedPreviewImageMaxRows)
}

func previewAspectFitImageCellSize(img models.InlineImage, innerW, availableRows, maxRows int) previewImageSize {
	widthCap := innerW - 2
	if widthCap < 1 {
		widthCap = 1
	}
	rowCap := minInt(availableRows, maxRows)
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
	if widthCells > widthCap || rowCells > rowCap {
		if widthCap*rowCells <= rowCap*widthCells {
			rowCells = ceilDivInt(rowCells*widthCap, widthCells)
			widthCells = widthCap
		} else {
			widthCells = ceilDivInt(widthCells*rowCap, rowCells)
			rowCells = rowCap
		}
	}
	if widthCells > widthCap {
		widthCells = widthCap
	}
	if rowCells > rowCap {
		rowCells = rowCap
	}
	if widthCells < 1 {
		widthCells = 1
	}
	if rowCells < 1 {
		rowCells = 1
	}
	return previewImageSize{Width: widthCells, Rows: rowCells}
}

func previewIterm2ImageCellSize(img models.InlineImage, innerW, availableRows int) previewImageSize {
	size := previewAspectFitImageCellSize(img, innerW, availableRows, decodedPreviewImageMaxRows)
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
		return size
	}

	aspectMilli := cfg.Width * 1000 / cfg.Height
	switch {
	case cfg.Width <= 80 && cfg.Height <= 40:
		size.Width = minInt(size.Width, minInt(widthCap, 10))
		size.Rows = minInt(size.Rows, minInt(rowCap, 3))
	case aspectMilli >= 1500:
		size.Rows = minInt(rowCap, maxInt(6, minInt(rowCap, 18)))
		size.Width = minInt(widthCap, maxInt(24, minInt(ceilDivInt(size.Rows*16*cfg.Width, 8*cfg.Height), 72)))
	case aspectMilli <= 800:
		size.Rows = minInt(rowCap, maxInt(8, minInt(rowCap, 16)))
		size.Width = minInt(widthCap, maxInt(12, minInt(ceilDivInt(size.Rows*16*cfg.Width, 8*cfg.Height), 44)))
	default:
		size.Rows = minInt(rowCap, maxInt(8, minInt(rowCap, 14)))
		size.Width = minInt(widthCap, maxInt(18, minInt(ceilDivInt(size.Rows*16*cfg.Width, 8*cfg.Height), 56)))
	}
	if size.Width < 1 {
		size.Width = 1
	}
	if size.Rows < 1 {
		size.Rows = 1
	}
	return size
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
		size := previewImageCellSizeForMode(mode, req.Image, req.InnerWidth, req.AvailableRows)
		rendered := strings.TrimRight(iterm2.RenderInlineInCellBox(req.Image.Data, size.Width, size.Rows, req.InnerWidth), "\n")
		if rendered == "" {
			return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
		}
		return previewImageRenderResult{Content: rendered, Rows: size.Rows, TerminalConsumesRows: true}
	case previewImageModeKitty:
		if len(req.Image.Data) > maxPreviewImageBytes {
			return oneLinePreviewImageFallback(fmt.Sprintf("[image too large to render inline: %s]", req.Image.MIMEType), req.InnerWidth)
		}
		size := previewImageCellSizeForMode(mode, req.Image, req.InnerWidth, req.AvailableRows)
		rendered, err := kittyimg.RenderInline(req.Image.Data, size.Width, size.Rows)
		if err != nil || rendered == "" {
			return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
		}
		return previewImageRenderResult{Content: strings.TrimRight(rendered, "\n"), Rows: size.Rows}
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ceilDivInt(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
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
