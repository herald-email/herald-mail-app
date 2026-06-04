package app

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/iterm2"
	"github.com/herald-email/herald-mail-app/internal/kittyimg"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/render"
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

type previewNativeImageSource struct {
	Image models.InlineImage
	Width int
	Rows  int

	decodeAttempted bool
	decodedImage    image.Image
	decodeErr       error
	clippedRenders  map[previewNativeImageClipKey]string
}

type previewNativeImageClipKey struct {
	Mode        previewImageMode
	Width       int
	TotalRows   int
	ClipTopRows int
	VisibleRows int
}

type previewImageRenderResult struct {
	Content              string
	Rows                 int
	Width                int
	TerminalConsumesRows bool
	NativeImage          *previewNativeImageSource
}

var decodePreviewNativeImage = image.Decode

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
		rendered := strings.TrimRight(iterm2.RenderInlineImageOnly(req.Image.Data, size.Width, size.Rows), "\n")
		if rendered == "" {
			return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
		}
		return previewImageRenderResult{
			Content:              rendered,
			Rows:                 size.Rows,
			Width:                size.Width,
			TerminalConsumesRows: true,
			NativeImage:          &previewNativeImageSource{Image: req.Image, Width: size.Width, Rows: size.Rows},
		}
	case previewImageModeKitty:
		if len(req.Image.Data) > maxPreviewImageBytes {
			return oneLinePreviewImageFallback(fmt.Sprintf("[image too large to render inline: %s]", req.Image.MIMEType), req.InnerWidth)
		}
		size := previewImageCellSizeForMode(mode, req.Image, req.InnerWidth, req.AvailableRows)
		rendered, err := kittyimg.RenderInline(req.Image.Data, size.Width, size.Rows)
		if err != nil || rendered == "" {
			return oneLinePreviewImageFallback(previewImagePlaceholderText(req), req.InnerWidth)
		}
		return previewImageRenderResult{
			Content:     strings.TrimRight(rendered, "\n"),
			Rows:        size.Rows,
			Width:       size.Width,
			NativeImage: &previewNativeImageSource{Image: req.Image, Width: size.Width, Rows: size.Rows},
		}
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

func renderClippedPreviewNativeImage(source *previewNativeImageSource, mode previewImageMode, clipTopRows, visibleRows int) (string, bool) {
	if source == nil || visibleRows < 1 || source.Rows < 1 || source.Width < 1 || len(source.Image.Data) == 0 {
		return "", false
	}
	if clipTopRows < 0 {
		clipTopRows = 0
	}
	if clipTopRows >= source.Rows {
		return "", false
	}
	if clipTopRows+visibleRows > source.Rows {
		visibleRows = source.Rows - clipTopRows
	}
	if visibleRows < 1 {
		return "", false
	}

	key := previewNativeImageClipKey{
		Mode:        mode,
		Width:       source.Width,
		TotalRows:   source.Rows,
		ClipTopRows: clipTopRows,
		VisibleRows: visibleRows,
	}
	if source.clippedRenders != nil {
		if rendered, ok := source.clippedRenders[key]; ok {
			return rendered, rendered != ""
		}
	}

	data, pixelWidth, pixelHeight, err := source.cropPreviewImageRows(clipTopRows, visibleRows)
	if err != nil || len(data) == 0 {
		return "", false
	}

	rendered := ""
	switch mode {
	case previewImageModeIterm2:
		rendered = strings.TrimRight(iterm2.RenderInlineImageOnly(data, source.Width, visibleRows), "\n")
	case previewImageModeKitty:
		renderedKitty, err := kittyimg.RenderInlinePNG(data, pixelWidth, pixelHeight, source.Width, visibleRows)
		if err != nil {
			return "", false
		}
		rendered = strings.TrimRight(renderedKitty, "\n")
	default:
		return "", false
	}
	if rendered == "" {
		return "", false
	}
	if source.clippedRenders == nil {
		source.clippedRenders = make(map[previewNativeImageClipKey]string)
	}
	source.clippedRenders[key] = rendered
	return rendered, true
}

func cropPreviewImageRows(imageData []byte, clipTopRows, visibleRows, totalRows int) ([]byte, error) {
	img, _, err := decodePreviewNativeImage(bytes.NewReader(imageData))
	if err != nil {
		return nil, err
	}
	data, _, _, err := cropPreviewImageRowsFromImage(img, clipTopRows, visibleRows, totalRows)
	return data, err
}

func (source *previewNativeImageSource) cropPreviewImageRows(clipTopRows, visibleRows int) ([]byte, int, int, error) {
	img, err := source.decodedPreviewImage()
	if err != nil {
		return nil, 0, 0, err
	}
	return cropPreviewImageRowsFromImage(img, clipTopRows, visibleRows, source.Rows)
}

func (source *previewNativeImageSource) decodedPreviewImage() (image.Image, error) {
	if source == nil || len(source.Image.Data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}
	if source.decodeAttempted {
		return source.decodedImage, source.decodeErr
	}
	source.decodeAttempted = true
	source.decodedImage, _, source.decodeErr = decodePreviewNativeImage(bytes.NewReader(source.Image.Data))
	return source.decodedImage, source.decodeErr
}

func cropPreviewImageRowsFromImage(img image.Image, clipTopRows, visibleRows, totalRows int) ([]byte, int, int, error) {
	if totalRows < 1 || visibleRows < 1 {
		return nil, 0, 0, fmt.Errorf("invalid crop rows")
	}
	if img == nil {
		return nil, 0, 0, fmt.Errorf("missing image")
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, 0, 0, fmt.Errorf("invalid image dimensions")
	}

	if clipTopRows < 0 {
		clipTopRows = 0
	}
	if clipTopRows > totalRows {
		clipTopRows = totalRows
	}
	if clipTopRows+visibleRows > totalRows {
		visibleRows = totalRows - clipTopRows
	}
	if visibleRows < 1 {
		visibleRows = 1
	}

	y0 := bounds.Min.Y + clipTopRows*height/totalRows
	y1 := bounds.Min.Y + (clipTopRows+visibleRows)*height/totalRows
	if y1 <= y0 {
		y1 = y0 + 1
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	if y0 >= y1 {
		return nil, 0, 0, fmt.Errorf("empty image crop")
	}

	cropped := image.NewRGBA(image.Rect(0, 0, width, y1-y0))
	draw.Draw(cropped, cropped.Bounds(), img, image.Point{X: bounds.Min.X, Y: y0}, draw.Src)

	var out bytes.Buffer
	if err := png.Encode(&out, cropped); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), width, y1 - y0, nil
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
