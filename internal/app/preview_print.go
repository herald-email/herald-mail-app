package app

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/printing"
)

type previewPrintSurface int

const (
	previewPrintSurfaceTimeline previewPrintSurface = iota
	previewPrintSurfaceContacts
)

func (m *Model) SetPreviewPrinter(printer printing.Printer) {
	if printer == nil {
		printer = printing.UnsupportedPrinter{Reason: "printing disabled"}
	}
	m.previewPrinter = printer
}

func (m *Model) previewPrinterUnsupported() bool {
	if m.previewPrinter == nil {
		return true
	}
	if _, ok := m.previewPrinter.(printing.UnsupportedPrinter); ok {
		return true
	}
	if _, ok := m.previewPrinter.(*printing.UnsupportedPrinter); ok {
		return true
	}
	return false
}

func (m *Model) PreviewPrintingUnsupportedForTest() bool {
	return m.previewPrinterUnsupported()
}

func (m *Model) openPreviewPrintChooser(surface previewPrintSurface) (tea.Model, tea.Cmd, bool) {
	req, ok := m.previewPrintRequest(surface, printing.ModeOriginalVisual, printing.ThemeOriginal)
	if !ok {
		m.statusMessage = "Print unavailable until preview finishes loading"
		return m, nil, true
	}
	if m.previewPrintBusy {
		m.statusMessage = "Print already in progress"
		return m, nil, true
	}
	if m.previewPrinterUnsupported() {
		result, err := m.previewPrinter.Print(context.Background(), req)
		m.statusMessage = previewPrintStatusMessage(result, err)
		return m, nil, true
	}
	m.previewPrintChooser = true
	m.previewPrintSurface = surface
	m.previewPrintRemote = false
	m.previewPrintPending = false
	m.statusMessage = ""
	return m, nil, true
}

func (m *Model) openTimelinePrintChooserOrLoad() (tea.Model, tea.Cmd, bool) {
	if _, ok := m.previewPrintRequest(previewPrintSurfaceTimeline, printing.ModeOriginalVisual, printing.ThemeOriginal); ok {
		return m.openPreviewPrintChooser(previewPrintSurfaceTimeline)
	}
	if m.previewPrintBusy {
		m.statusMessage = "Print already in progress"
		return m, nil, true
	}
	if m.previewPrinterUnsupported() {
		if m.previewPrinter == nil {
			m.previewPrinter = printing.UnsupportedPrinter{Reason: "printing disabled"}
		}
		result, err := m.previewPrinter.Print(context.Background(), printing.Request{})
		m.statusMessage = previewPrintStatusMessage(result, err)
		return m, nil, true
	}
	if m.timeline.bodyLoading && m.timeline.selectedEmail != nil {
		m.previewPrintPending = true
		m.statusMessage = "Loading preview for print..."
		return m, nil, true
	}
	email := m.timeline.selectedEmail
	if email == nil || !m.timelineBodyLoadedFor(email) {
		email = m.currentTimelinePreviewTarget()
	}
	if email == nil {
		m.statusMessage = "Print unavailable until preview finishes loading"
		return m, nil, true
	}
	m.previewPrintPending = true
	m.statusMessage = "Loading preview for print..."
	return m, m.openTimelineEmail(email), true
}

func (m *Model) handlePreviewPrintChooserKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.previewPrintChooser {
		return m, nil, false
	}
	switch shortcutKey(msg) {
	case "esc", "q":
		m.previewPrintChooser = false
		m.statusMessage = "Print canceled"
		return m, nil, true
	case "i":
		m.previewPrintRemote = !m.previewPrintRemote
		return m, nil, true
	case "1":
		return m, m.startPreviewPrint(printing.ModeOriginalVisual, printing.ThemeOriginal), true
	default:
		if option, ok := printing.MarkdownThemeByKey(shortcutKey(msg)); ok {
			return m, m.startPreviewPrint(printing.ModeRenderedMarkdown, option.ID), true
		}
		return m, nil, true
	}
}

func (m *Model) startPreviewPrint(mode printing.Mode, theme printing.Theme) tea.Cmd {
	req, ok := m.previewPrintRequest(m.previewPrintSurface, mode, theme)
	m.previewPrintChooser = false
	if !ok {
		m.statusMessage = "Print unavailable until preview finishes loading"
		return nil
	}
	if m.previewPrinter == nil {
		m.previewPrinter = printing.UnsupportedPrinter{Reason: "printing disabled"}
	}
	m.previewPrintBusy = true
	printer := m.previewPrinter
	return func() tea.Msg {
		result, err := printer.Print(context.Background(), req)
		return PreviewPrintMsg{Result: result, Err: err}
	}
}

func (m *Model) previewPrintRequest(surface previewPrintSurface, mode printing.Mode, theme printing.Theme) (printing.Request, bool) {
	var email *models.EmailData
	var body *models.EmailBody
	switch surface {
	case previewPrintSurfaceContacts:
		if m.contactPreviewLoading {
			return printing.Request{}, false
		}
		email = m.contactPreviewEmail
		body = m.contactPreviewBody
	default:
		email = m.timeline.selectedEmail
		body = m.timeline.body
		if m.timeline.bodyLoading {
			return printing.Request{}, false
		}
		if email != nil && m.timeline.bodyMessageID != "" && m.timeline.bodyMessageID != email.MessageID {
			return printing.Request{}, false
		}
	}
	if email == nil || body == nil {
		return printing.Request{}, false
	}
	return printing.Request{
		Email:             email,
		Body:              body,
		Mode:              mode,
		Theme:             theme,
		Title:             email.Subject,
		AllowRemoteImages: m.previewPrintRemote,
	}, true
}

func previewPrintStatusMessage(result printing.Result, err error) string {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "Print canceled"
		}
		return "Print failed: " + boundedPreviewPrintStatus(err.Error())
	}
	message := strings.TrimSpace(result.Message)
	switch result.Status {
	case printing.StatusCanceled:
		if message == "" {
			message = "Print canceled"
		}
	case printing.StatusUnsupported:
		if message == "" {
			message = "Printing unsupported on this host"
		}
	default:
		if message == "" {
			message = "Print dialog opened"
		}
	}
	return boundedPreviewPrintStatus(message)
}

func boundedPreviewPrintStatus(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const limit = 220
	runes := []rune(s)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "..."
	}
	return s
}

func (m *Model) renderPreviewPrintChooserView() string {
	backdrop := m.renderMainView()
	if m.timeline.fullScreen {
		backdrop = m.renderFullScreenEmail()
	}
	w, h := m.windowWidth, m.windowHeight
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	layout := newCompactOverlayLayoutWithMax(w, h, 58, 15)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor())
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
	lines := m.previewPrintChooserLines(layout, titleStyle, dimStyle)
	content := strings.Join(lines, "\n")
	return overlayCentered(backdrop, renderCompactOverlayBox(content, layout), w, h)
}

func (m *Model) previewPrintChooserLines(layout compactOverlayLayout, titleStyle, dimStyle lipgloss.Style) []string {
	subject := strings.TrimSpace(m.previewPrintSubject())
	remoteToggle := m.previewPrintRemoteToggleLabel()
	if layout.contentHeight <= 8 {
		lines := []string{titleStyle.Render("Print Preview")}
		if subject != "" && layout.contentHeight >= 7 {
			lines = append(lines, dimStyle.Render(truncate(subject, layout.contentWidth)))
		}
		lines = append(lines,
			"",
			m.previewPrintOptionRow("1", "Original Visual"),
			m.previewPrintCompactThemeRow("2", "3"),
			m.previewPrintCompactThemeRow("4", "5"),
			remoteToggle,
			dimStyle.Render("Esc cancel"),
		)
		return lines
	}

	title := titleStyle.Render("Print Preview")
	if subject != "" {
		title += "\n" + dimStyle.Render(truncate(subject, layout.contentWidth))
	}
	lines := []string{
		title,
		"",
		m.previewPrintOptionRow("1", "Original Visual"),
	}
	for _, option := range printing.MarkdownThemes() {
		lines = append(lines, m.previewPrintOptionRow(option.Key, option.Name))
	}
	lines = append(lines,
		remoteToggle,
		"",
		dimStyle.Render("Esc cancel"),
	)
	return lines
}

func (m *Model) previewPrintRemoteToggleLabel() string {
	state := "[ OFF ]"
	stateStyle := m.theme.Overlay.PrintToggleOff.Style()
	if m.previewPrintRemote {
		state = "[ ON ]"
		stateStyle = m.theme.Overlay.PrintToggleOn.Style()
	}
	return m.previewPrintKeyBadge("i") + " Remote Images " + stateStyle.Render(state)
}

func (m *Model) previewPrintKeyBadge(key string) string {
	return m.theme.Overlay.PrintKeyBadge.Style().Render("[" + key + "]")
}

func (m *Model) previewPrintOptionRow(key, label string) string {
	return m.previewPrintKeyBadge(key) + " " + label
}

func (m *Model) previewPrintCompactThemeRow(leftKey, rightKey string) string {
	left, _ := printing.MarkdownThemeByKey(leftKey)
	right, _ := printing.MarkdownThemeByKey(rightKey)
	leftName := strings.TrimPrefix(left.Name, "Markdown ")
	rightName := strings.TrimPrefix(right.Name, "Markdown ")
	if leftName == "" {
		leftName = string(left.ID)
	}
	if rightName == "" {
		rightName = string(right.ID)
	}
	return m.previewPrintOptionRow(left.Key, leftName) + "   " + m.previewPrintOptionRow(right.Key, rightName)
}

func (m *Model) previewPrintSubject() string {
	switch m.previewPrintSurface {
	case previewPrintSurfaceContacts:
		if m.contactPreviewEmail != nil {
			return m.contactPreviewEmail.Subject
		}
	default:
		if m.timeline.selectedEmail != nil {
			return m.timeline.selectedEmail.Subject
		}
	}
	return ""
}

func (m *Model) timelinePrintablePreviewLoaded() bool {
	_, ok := m.previewPrintRequest(previewPrintSurfaceTimeline, printing.ModeOriginalVisual, printing.ThemeOriginal)
	return ok
}

func (m *Model) timelinePrintAvailableFromTimeline() bool {
	if m.previewPrinterUnsupported() {
		return false
	}
	if m.timelinePrintablePreviewLoaded() {
		return true
	}
	return m.currentTimelinePreviewTarget() != nil
}

func (m *Model) contactsPrintablePreviewLoaded() bool {
	_, ok := m.previewPrintRequest(previewPrintSurfaceContacts, printing.ModeOriginalVisual, printing.ThemeOriginal)
	return ok
}

func (m *Model) previewPrintHint(scope string) string {
	if m.previewPrinterUnsupported() {
		return ""
	}
	return m.commandHint(scope, CommandPreviewPrint, "print")
}
