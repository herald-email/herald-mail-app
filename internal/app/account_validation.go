package app

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *Model) renderAccountValidationOverlayView() string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	h := m.windowHeight
	if h <= 0 {
		h = 24
	}
	return overlayCentered(m.renderShortcutHelpBackdropView(), m.renderAccountValidationPanel(), w, h)
}

func (m *Model) renderAccountValidationPanel() string {
	state := m.accountValidation
	if state == nil {
		return ""
	}
	title := "Validating account"
	footer := "Please wait..."
	if !state.Checking {
		title = "Account settings not saved"
		footer = "Enter/Esc/q: close"
	}
	message := strings.TrimSpace(state.Message)
	if message == "" && state.Checking {
		message = "Checking IMAP and SMTP before saving account settings..."
	}
	if message == "" {
		message = "Settings were not saved."
	}

	width := m.windowWidth - 10
	if width > 72 {
		width = 72
	}
	if width < 34 {
		width = 34
	}
	innerW := width - 4
	lines := []string{
		m.theme.Setup.Title.Style().Render(title),
		strings.Repeat("-", innerW),
	}
	lines = append(lines, wrapLines(message, innerW)...)
	lines = append(lines, strings.Repeat("-", innerW), m.theme.Text.Dim.Style().Render(footer))

	border := m.theme.Setup.Border.ForegroundColor()
	if !state.Checking {
		border = m.theme.Severity.Error.ForegroundColor()
	}
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func (m *Model) renderAIModelValidationOverlayView() string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	h := m.windowHeight
	if h <= 0 {
		h = 24
	}
	return overlayCentered(m.renderShortcutHelpBackdropView(), m.renderAIModelValidationPanel(), w, h)
}

func (m *Model) renderAIModelValidationPanel() string {
	state := m.aiModelValidation
	if state == nil {
		return ""
	}
	title := "Validating AI setup"
	footer := "Please wait..."
	if !state.Checking {
		title = "AI settings not saved"
		footer = "Enter/Esc/q: close"
	}
	message := strings.TrimSpace(state.Message)
	if message == "" && state.Checking {
		message = "Checking local Ollama models before saving AI settings..."
	}
	if message == "" {
		message = "AI settings were not saved."
	}

	width := m.windowWidth - 10
	if width > 72 {
		width = 72
	}
	if width < 34 {
		width = 34
	}
	innerW := width - 4
	lines := []string{
		m.theme.Setup.Title.Style().Render(title),
		strings.Repeat("-", innerW),
	}
	lines = append(lines, wrapLines(message, innerW)...)
	lines = append(lines, strings.Repeat("-", innerW), m.theme.Text.Dim.Style().Render(footer))

	border := m.theme.Setup.Border.ForegroundColor()
	if !state.Checking {
		border = m.theme.Severity.Error.ForegroundColor()
	}
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}
