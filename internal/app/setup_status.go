package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/render"
)

// RenderSetupStatus renders non-form setup states, such as validation progress
// and validation failures, inside the same setup chrome as the first-run form.
func RenderSetupStatus(width, height int, title, body, footer string) string {
	if width > 0 && width < minTermWidth {
		return renderMinSizeMessage(width, height)
	}
	if height > 0 && height < minTermHeight {
		return renderMinSizeMessage(width, height)
	}

	boxWidth := wizardBoxWidthFor(width)
	contentWidth := boxWidth - 6
	if contentWidth < 20 {
		contentWidth = 20
	}

	var content []string
	if strings.TrimSpace(title) != "" {
		content = append(content, defaultTheme.Setup.SummaryLabel.Style().Render(title))
	}
	if strings.TrimSpace(body) != "" {
		if len(content) > 0 {
			content = append(content, "")
		}
		content = append(content, wrapSetupStatus(body, contentWidth)...)
	}
	if strings.TrimSpace(footer) != "" {
		if len(content) > 0 {
			content = append(content, "")
		}
		content = append(content, styleSetupStatusLines(wrapSetupStatus(footer, contentWidth))...)
	}
	if len(content) == 0 {
		content = append(content, " ")
	}

	setupTitle := defaultTheme.Setup.Title.Style().Render("Herald Setup")
	box := lipgloss.NewStyle().
		Width(boxWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(defaultTheme.Setup.Border.ForegroundColor()).
		Padding(1, 2)

	rendered := lipgloss.JoinVertical(lipgloss.Left,
		setupTitle,
		box.Render(strings.Join(content, "\n")),
	)
	rendered = strings.TrimRight(rendered, "\n")
	if width > 0 && height > 0 {
		return strings.TrimRight(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, rendered), "\n")
	}
	return rendered
}

func wrapSetupStatus(text string, width int) []string {
	lines := render.WrapLines(strings.TrimSpace(text), width)
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func styleSetupStatusLines(lines []string) []string {
	style := defaultTheme.Setup.SummaryBody.Style()
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, style.Render(line))
	}
	return out
}

func wizardBoxWidthFor(width int) int {
	if width <= 0 {
		return 88
	}
	w := width - 8
	if w > 88 {
		w = 88
	}
	if w < 56 {
		w = width
	}
	return w
}
