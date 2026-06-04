package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type previewClipboardPayload struct {
	Plain     string
	HTML      string
	Image     *models.InlineImage
	ImageName string
}

type PreviewClipboardMsg struct {
	Summary string
	Err     error
}

type previewClipboardWriter interface {
	WritePreviewClipboard(previewClipboardPayload) (string, error)
}

type systemPreviewClipboardWriter struct{}

func (systemPreviewClipboardWriter) WritePreviewClipboard(payload previewClipboardPayload) (string, error) {
	return writeSystemPreviewClipboard(payload)
}

var activePreviewClipboardWriter previewClipboardWriter = systemPreviewClipboardWriter{}

func copyPreviewPayloadToClipboard(payload previewClipboardPayload) tea.Cmd {
	return func() tea.Msg {
		summary, err := activePreviewClipboardWriter.WritePreviewClipboard(payload)
		if summary == "" {
			switch {
			case payload.Image != nil:
				summary = "Image copied"
			case payload.HTML != "":
				summary = "Rich text copied"
			default:
				summary = "Text copied"
			}
		}
		if err != nil {
			return PreviewClipboardMsg{Err: fmt.Errorf("clipboard copy failed: %w", err)}
		}
		return PreviewClipboardMsg{Summary: summary}
	}
}
