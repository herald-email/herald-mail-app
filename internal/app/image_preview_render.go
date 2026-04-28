package app

import (
	"fmt"
	"strings"

	"mail-processor/internal/iterm2"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	"mail-processor/internal/render"
)

const (
	maxPreviewImageCount = 4
	maxPreviewImageBytes = 5 * 1024 * 1024
	maxPreviewImageRows  = 8
)

func (m *Model) fullScreenImagesAvailable() bool {
	return iterm2.IsSupported() || m.localImageLinks
}

func splitInlineImageHint(count int, available bool) string {
	if count == 0 {
		return ""
	}
	if available {
		return fmt.Sprintf("[%d image(s) - press z for full-screen to view]", count)
	}
	return fmt.Sprintf("[%d image(s) - full-screen image viewing unavailable here]", count)
}

func (m *Model) renderInlineImagesForPreview(scopeKey string, images []models.InlineImage, descs map[string]string, innerW, availableRows int) (string, int) {
	if len(images) == 0 || availableRows <= 0 {
		return "", 0
	}
	displayImages := boundedPreviewImages(images)
	if len(displayImages) == 0 {
		return "", 0
	}

	if iterm2.IsSupported() {
		return renderIterm2PreviewImages(displayImages, descs, innerW, availableRows)
	}
	if m.localImageLinks {
		return m.renderLocalImageLinks(scopeKey, displayImages, descs, innerW, availableRows)
	}
	return renderImagePlaceholders(displayImages, descs, innerW, availableRows)
}

func boundedPreviewImages(images []models.InlineImage) []models.InlineImage {
	limit := len(images)
	if limit > maxPreviewImageCount {
		limit = maxPreviewImageCount
	}
	out := make([]models.InlineImage, 0, limit)
	for _, img := range images {
		if len(out) >= limit {
			break
		}
		out = append(out, img)
	}
	return out
}

func renderIterm2PreviewImages(images []models.InlineImage, descs map[string]string, innerW, availableRows int) (string, int) {
	width := innerW - 2
	if width < 10 {
		width = innerW
	}
	if width < 1 {
		width = 1
	}
	rows := previewImageRows(availableRows, len(images))
	var sb strings.Builder
	used := 0
	for i, img := range images {
		if used >= availableRows {
			break
		}
		if used > 0 {
			sb.WriteByte('\n')
		}
		if len(img.Data) == 0 {
			sb.WriteString(truncateVisual(fmt.Sprintf("[image %d unavailable: empty data]", i+1), innerW))
			used++
			continue
		}
		if len(img.Data) > maxPreviewImageBytes {
			sb.WriteString(truncateVisual(fmt.Sprintf("[image %d too large to render inline: %s]", i+1, img.MIMEType), innerW))
			used++
			continue
		}
		rendered := strings.TrimRight(iterm2.Render(img.Data, width, rows), "\n")
		if rendered == "" {
			sb.WriteString(imagePlaceholderLine(i+1, img, descs, innerW))
			used++
			continue
		}
		sb.WriteString(rendered)
		used += rows
	}
	return sb.String(), clampInt(used, 0, availableRows)
}

func (m *Model) renderLocalImageLinks(scopeKey string, images []models.InlineImage, descs map[string]string, innerW, availableRows int) (string, int) {
	if m.imagePreviewLinks == nil {
		m.imagePreviewLinks = newImagePreviewServer()
	}
	links, err := m.imagePreviewLinks.RegisterSet(scopeKey, images)
	if err != nil || len(links) == 0 {
		logger.Warn("local image preview links unavailable: %v", err)
		return renderImagePlaceholders(images, descs, innerW, availableRows)
	}
	var lines []string
	for _, link := range links {
		if len(lines) >= availableRows {
			break
		}
		label := render.TerminalHyperlink("["+link.Label+"]", link.URL)
		meta := fmt.Sprintf(" %s  %d KB", link.MIMEType, link.Size/1024)
		lines = append(lines, truncateVisual(label+meta, innerW))
	}
	return strings.Join(lines, "\n"), len(lines)
}

func renderImagePlaceholders(images []models.InlineImage, descs map[string]string, innerW, availableRows int) (string, int) {
	var lines []string
	for i, img := range images {
		if len(lines) >= availableRows {
			break
		}
		lines = append(lines, imagePlaceholderLine(i+1, img, descs, innerW))
	}
	return strings.Join(lines, "\n"), len(lines)
}

func imagePlaceholderLine(index int, img models.InlineImage, descs map[string]string, innerW int) string {
	if descs != nil {
		if desc := strings.TrimSpace(descs[img.ContentID]); desc != "" {
			return truncateVisual(fmt.Sprintf("[Image: %s]", desc), innerW)
		}
	}
	return truncateVisual(fmt.Sprintf("[image %d: %s  %d KB]", index, img.MIMEType, len(img.Data)/1024), innerW)
}

func previewImageRows(availableRows, imageCount int) int {
	if imageCount < 1 {
		imageCount = 1
	}
	total := availableRows / 2
	if total < 1 {
		total = 1
	}
	if total > maxPreviewImageRows {
		total = maxPreviewImageRows
	}
	rows := total / imageCount
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *Model) revokeImagePreviews() {
	if m.imagePreviewLinks != nil {
		m.imagePreviewLinks.RevokeAll()
	}
}
