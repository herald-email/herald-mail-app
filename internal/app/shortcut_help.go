package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type shortcutHelpSection struct {
	Title   string
	Entries []shortcutHelpEntry
}

type shortcutHelpEntry struct {
	Key  string
	Desc string
}

type shortcutHelpLayout struct {
	panelWidth  int
	panelHeight int
	styleWidth  int
	contentW    int
	visibleRows int
}

const (
	shortcutHelpMaxWidth  = 88
	shortcutHelpMaxHeight = 24
)

func (m *Model) handleShortcutHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	rawKey := msg.String()
	key := shortcutKey(msg)
	if m.showHelp {
		switch key {
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit, true
		case "?", "esc", "q":
			m.showHelp = false
			m.helpScrollOffset = 0
			return m, nil, true
		case "up", "k":
			if m.helpScrollOffset > 0 {
				m.helpScrollOffset--
			}
			return m, nil, true
		case "down", "j":
			m.helpScrollOffset++
			return m, nil, true
		case "pgup":
			m.helpScrollOffset -= m.shortcutHelpPageStep()
			if m.helpScrollOffset < 0 {
				m.helpScrollOffset = 0
			}
			return m, nil, true
		case "pgdown":
			m.helpScrollOffset += m.shortcutHelpPageStep()
			return m, nil, true
		case "home":
			m.helpScrollOffset = 0
			return m, nil, true
		case "end":
			m.helpScrollOffset = 1 << 30
			return m, nil, true
		}
		return m, nil, true
	}

	if rawKey != key && key == "?" && m.shortcutAliasBelongsToTextInput() {
		return m, nil, false
	}
	if key == "?" && !m.questionMarkBelongsToTextInput() {
		m.showHelp = true
		m.helpScrollOffset = 0
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) shortcutAliasBelongsToTextInput() bool {
	if m.questionMarkBelongsToTextInput() {
		return true
	}
	if m.activeTab != tabCompose {
		return false
	}
	if m.attachmentInputActive {
		return true
	}
	if m.composeAIPanel && m.composeAIInput.Focused() {
		return true
	}
	if m.composeField == composeFieldOriginalMessage || m.composeField == composeFieldForwardedAttachments {
		return false
	}
	return true
}

func (m *Model) questionMarkBelongsToTextInput() bool {
	return (m.activeTab == tabTimeline &&
		m.timeline.searchMode &&
		m.timeline.searchFocus == timelineSearchFocusInput) ||
		(m.activeTab == tabContacts && m.contactSearchMode != "")
}

func (m *Model) shouldAdvertiseShortcutHelp() bool {
	return !m.showHelp && !m.questionMarkBelongsToTextInput()
}

func (m *Model) shortcutHelpPageStep() int {
	layout := m.shortcutHelpLayout()
	if layout.visibleRows > 0 {
		return layout.visibleRows
	}
	return 6
}

func (m *Model) shortcutHelpLayout() shortcutHelpLayout {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	h := m.windowHeight
	if h <= 0 {
		h = 24
	}

	panelW := shortcutHelpMaxWidth
	if maxW := w - 4; maxW < panelW {
		panelW = maxW
	}
	if panelW < 30 {
		panelW = w
	}
	if panelW < 30 {
		panelW = 30
	}

	panelH := shortcutHelpMaxHeight
	if maxH := h - 4; maxH < panelH {
		panelH = maxH
	}
	if panelH < 10 {
		panelH = h
	}
	if panelH < 6 {
		panelH = 6
	}

	styleW := panelW - 2
	if styleW < 28 {
		styleW = 28
	}
	contentW := styleW - 4
	if contentW < 20 {
		contentW = 20
	}
	visibleRows := panelH - 5
	if visibleRows < 1 {
		visibleRows = 1
	}

	return shortcutHelpLayout{
		panelWidth:  panelW,
		panelHeight: panelH,
		styleWidth:  styleW,
		contentW:    contentW,
		visibleRows: visibleRows,
	}
}

func (m *Model) renderShortcutHelpView() string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	h := m.windowHeight
	if h <= 0 {
		h = 24
	}

	backdrop := m.renderShortcutHelpBackdropView()
	panel := m.renderShortcutHelpPanel()
	return overlayCentered(backdrop, panel, w, h)
}

func (m *Model) renderSettingsOverlayView() string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	h := m.windowHeight
	if h <= 0 {
		h = 24
	}

	backdrop := m.renderShortcutHelpBackdropView()
	panel := m.settingsPanel.renderPanel()
	return overlayCentered(backdrop, panel, w, h)
}

func (m *Model) renderShortcutHelpBackdropView() string {
	if m.loading && !m.hasVisibleStartupData() {
		return m.renderLoadingView()
	}
	if m.timeline.fullScreen {
		return m.renderFullScreenEmail()
	}
	if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupFullScreen {
		return m.renderCleanupPreview()
	}
	return m.renderMainView()
}

func (m *Model) renderShortcutHelpPanel() string {
	layout := m.shortcutHelpLayout()
	title := "Shortcut Help - " + m.shortcutHelpContextTitle()
	lines := m.shortcutHelpLines(layout.contentW)
	maxOffset := len(lines) - layout.visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.helpScrollOffset > maxOffset {
		m.helpScrollOffset = maxOffset
	}
	if m.helpScrollOffset < 0 {
		m.helpScrollOffset = 0
	}
	end := m.helpScrollOffset + layout.visibleRows
	if end > len(lines) {
		end = len(lines)
	}

	bodyLines := append([]string{}, lines[m.helpScrollOffset:end]...)
	for len(bodyLines) < layout.visibleRows {
		bodyLines = append(bodyLines, "")
	}

	scroll := "Esc/?/q close"
	if len(lines) > layout.visibleRows {
		scroll = fmt.Sprintf("j/k scroll  %d/%d  Esc/?/q close", m.helpScrollOffset+1, len(lines))
	}

	headerStyle := lipgloss.NewStyle().Foreground(defaultTheme.Severity.Info.ForegroundColor()).Bold(true)
	footerStyle := lipgloss.NewStyle().Foreground(defaultTheme.Text.Dim.ForegroundColor())
	content := []string{
		headerStyle.Render(ansi.Truncate(title, layout.contentW, "")),
		strings.Repeat("─", layout.contentW),
	}
	content = append(content, bodyLines...)
	content = append(content, footerStyle.Render(ansi.Truncate(scroll, layout.contentW, "")))

	return lipgloss.NewStyle().
		Width(layout.styleWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(defaultTheme.Focus.PanelBorderFocused.ForegroundColor()).
		Padding(0, 1).
		Render(strings.Join(content, "\n"))
}

func overlayCentered(backdrop, overlay string, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	backdropLines := splitViewportLines(backdrop, height)
	overlayLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")
	if len(overlayLines) == 0 {
		return strings.Join(backdropLines, "\n")
	}

	overlayW := 0
	for _, line := range overlayLines {
		if w := ansi.StringWidth(line); w > overlayW {
			overlayW = w
		}
	}
	if overlayW > width {
		overlayW = width
	}
	overlayH := len(overlayLines)
	if overlayH > height {
		overlayH = height
		overlayLines = overlayLines[:height]
	}

	startX := (width - overlayW) / 2
	if startX < 0 {
		startX = 0
	}
	startY := (height - overlayH) / 2
	if startY < 0 {
		startY = 0
	}

	for i, overlayLine := range overlayLines {
		y := startY + i
		if y < 0 || y >= len(backdropLines) {
			continue
		}
		line := backdropLines[y]
		mid := padANSIToWidth(ansi.Cut(overlayLine, 0, overlayW), overlayW)
		left := padANSIToWidth(ansi.Cut(line, 0, startX), startX)
		right := ansi.Cut(line, startX+overlayW, width)
		backdropLines[y] = ansi.Cut(left+mid+right, 0, width)
	}

	return strings.Join(backdropLines, "\n")
}

func splitViewportLines(view string, height int) []string {
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func padANSIToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if current := ansi.StringWidth(s); current < width {
		return s + strings.Repeat(" ", width-current)
	}
	return ansi.Cut(s, 0, width)
}

func (m *Model) shortcutHelpContextTitle() string {
	switch {
	case m.pendingDeleteConfirm:
		if m.pendingArchive {
			return "Archive Confirmation"
		}
		return "Delete Confirmation"
	case m.pendingUnsubscribe:
		return "Unsubscribe Confirmation"
	case m.showLogs:
		return "Logs"
	case m.focusedPanel == panelChat && m.showChat:
		return "Chat"
	case m.activeTab == tabCompose:
		return "Compose"
	case m.activeTab == tabContacts:
		if m.contactPreviewEmail != nil {
			return "Contacts Preview"
		}
		if m.contactFocusPanel == 1 {
			return "Contacts Detail"
		}
		return "Contacts"
	case m.activeTab == tabCleanup:
		if m.showCleanupPreview {
			return "Cleanup Preview"
		}
		if m.focusedPanel == panelDetails {
			return "Cleanup Details"
		}
		return "Cleanup Summary"
	default:
		if m.timeline.fullScreen {
			return "Timeline Full-Screen Preview"
		}
		if m.timeline.quickReplyOpen {
			return "Timeline Quick Replies"
		}
		if m.timeline.searchMode {
			return "Timeline Search"
		}
		if m.timeline.selectedEmail != nil && m.focusedPanel == panelPreview {
			return "Timeline Preview"
		}
		return "Timeline"
	}
}

func (m *Model) shortcutHelpLines(width int) []string {
	sections := m.shortcutHelpSections()
	lines := make([]string, 0, 40)
	for si, section := range sections {
		if si > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(defaultTheme.Chrome.TitleBar.ForegroundColor()).Bold(true).Render(section.Title))
		for _, entry := range section.Entries {
			key := ansi.Truncate(entry.Key, 16, "")
			descWidth := width - 20
			if descWidth < 12 {
				descWidth = 12
			}
			descLines := wrapLines(entry.Desc, descWidth)
			if len(descLines) == 0 {
				descLines = []string{""}
			}
			for i, desc := range descLines {
				prefix := ""
				if i == 0 {
					prefix = fmt.Sprintf("  %-16s  ", key)
				} else {
					prefix = strings.Repeat(" ", 20)
				}
				lines = append(lines, prefix+ansi.Truncate(desc, descWidth, ""))
			}
		}
	}
	return lines
}

func (m *Model) shortcutHelpSections() []shortcutHelpSection {
	sections := []shortcutHelpSection{{
		Title: "Global",
		Entries: []shortcutHelpEntry{
			{"?", "open or close this shortcut help"},
			{"F1-F3", "switch tabs"},
			{"Alt+L / Alt+C", "open logs or chat from text-entry contexts"},
			{"Alt+F / Alt+R", "toggle sidebar or refresh from text-entry contexts"},
			{"Ctrl+C", "quit Herald"},
		},
	}}

	switch {
	case m.pendingDeleteConfirm:
		sections = append(sections, shortcutHelpSection{"Confirmation", []shortcutHelpEntry{
			{"y", "confirm the pending delete or archive action"},
			{"n / Esc", "cancel without changing mail"},
		}})
	case m.pendingUnsubscribe:
		sections = append(sections, shortcutHelpSection{"Confirmation", []shortcutHelpEntry{
			{"y", "confirm unsubscribe"},
			{"n / Esc", "cancel without changing subscription state"},
		}})
	case m.showLogs:
		sections = append(sections, shortcutHelpSection{"Logs", []shortcutHelpEntry{
			{"j/k or arrows", "scroll log output"},
			{"l / Alt+L / Esc", "close logs"},
			{"q", "quit when help is closed"},
		}})
	case m.focusedPanel == panelChat && m.showChat:
		sections = append(sections, shortcutHelpSection{"Chat", []shortcutHelpEntry{
			{"Enter", "send the current chat message"},
			{"Esc / Tab", "close chat or return focus to the main pane"},
		}})
	case m.activeTab == tabCompose:
		sections = append(sections, m.composeShortcutHelpSection())
	case m.activeTab == tabContacts:
		sections = append(sections, m.contactsShortcutHelpSection())
	case m.activeTab == tabCleanup:
		sections = append(sections, m.cleanupShortcutHelpSection())
	default:
		sections = append(sections, m.timelineShortcutHelpSection())
	}
	return sections
}

func (m *Model) composeShortcutHelpSection() shortcutHelpSection {
	entries := []shortcutHelpEntry{
		{"Tab", "move to the next Compose field"},
		{"Ctrl+S", "send the current message"},
		{"Ctrl+P", "toggle Markdown preview"},
		{"Ctrl+A", "attach a file"},
		{"Ctrl+G", "open or close the AI writing assistant"},
		{"Ctrl+J", "suggest a subject from the draft"},
		{"Esc", "dismiss Compose status, subject suggestion, or AI panel"},
	}
	if m.composePreserved != nil {
		entries = append(entries, shortcutHelpEntry{"Ctrl+O", "cycle preservation mode for the original reply or forward context"})
		entries = append(entries, shortcutHelpEntry{"j / k", "scroll the focused original-message context"})
		if m.hasForwardedAttachments() {
			entries = append(entries, shortcutHelpEntry{"x", "toggle whether the selected forwarded attachment is included"})
		}
	}
	return shortcutHelpSection{"Compose", entries}
}

func (m *Model) contactsShortcutHelpSection() shortcutHelpSection {
	if m.contactPreviewEmail != nil {
		return shortcutHelpSection{"Contacts Preview", []shortcutHelpEntry{
			{"Esc", "back to the selected contact"},
			{"Tab", "switch contact panes when preview is closed"},
		}}
	}
	return shortcutHelpSection{"Contacts", []shortcutHelpEntry{
		{"j/k or arrows", "navigate contacts or contact emails"},
		{"Enter", "open contact detail or selected email preview"},
		{"Tab", "switch between list and detail panes"},
		{"/", "start contact search; type ? query for semantic search"},
		{"e", "enrich the selected contact"},
		{"Esc", "clear search or return to the contacts list"},
	}}
}

func (m *Model) cleanupShortcutHelpSection() shortcutHelpSection {
	if m.showCleanupPreview {
		return shortcutHelpSection{"Cleanup Preview", []shortcutHelpEntry{
			{"j/k or arrows", "scroll preview"},
			{"Enter", "scroll down"},
			{"z", "toggle full-screen preview"},
			{"u", "unsubscribe when mailing-list headers are available"},
			{"h", "hide future mail from this sender"},
			{"D / e", "delete or archive this email"},
			{"A", "re-classify this email"},
			{"Esc", "close preview"},
		}}
	}
	if m.focusedPanel == panelDetails {
		return shortcutHelpSection{"Cleanup Details", []shortcutHelpEntry{
			{"j/k or arrows", "navigate emails for the selected sender or domain"},
			{"Enter", "open email preview"},
			{"Space", "select the highlighted message"},
			{"D / e", "delete or archive selected mail after confirmation"},
			{"Tab", "switch Cleanup panes"},
		}}
	}
	return shortcutHelpSection{"Cleanup Summary", []shortcutHelpEntry{
		{"j/k or arrows", "navigate sender or domain groups"},
		{"Enter", "load details for the highlighted group"},
		{"Space", "select the highlighted sender or domain"},
		{"d", "toggle sender/domain grouping"},
		{"W / C / P", "open rule, cleanup manager, or prompt editor overlays"},
		{"h", "hide future mail from this sender or domain"},
		{"D / e", "delete or archive selected mail after confirmation"},
	}}
}

func (m *Model) timelineShortcutHelpSection() shortcutHelpSection {
	if m.timeline.quickReplyOpen {
		return shortcutHelpSection{"Timeline Quick Replies", []shortcutHelpEntry{
			{"j/k or arrows", "navigate suggested replies"},
			{"1-8", "choose a reply by number"},
			{"Enter", "open selected reply in Compose"},
			{"Esc", "close quick replies"},
		}}
	}
	if m.timeline.searchMode {
		return shortcutHelpSection{"Timeline Search", []shortcutHelpEntry{
			{"Type", "search the current folder; start with ? query for semantic search"},
			{"Enter", "run search or move to results"},
			{"Ctrl+I", "run server search"},
			{"Esc", "back out of search"},
		}}
	}
	if m.timeline.selectedEmail != nil && (m.focusedPanel == panelPreview || m.timeline.fullScreen) {
		if m.timeline.selectedEmail.IsDraft {
			return shortcutHelpSection{"Timeline Draft", []shortcutHelpEntry{
				{"j/k or arrows", "scroll preview"},
				{"Tab", "switch between list and preview"},
				{"Left arrow", "return focus to the Timeline list without closing preview"},
				{"[ / ]", "navigate attachments when preview focus has attachments"},
				{"Right / ]", "open preview, or focus an already-open preview from list focus"},
				{"U", "mark unread"},
				{"E", "edit draft in Compose"},
				{"Ctrl+S", "send draft after confirmation"},
				{"D", "discard draft after confirmation"},
				{"z", "toggle full-screen preview"},
				{"v / y / Y", "visual selection and copy"},
				{"Esc", "close preview"},
			}}
		}
		return shortcutHelpSection{"Timeline Preview", []shortcutHelpEntry{
			{"j/k or arrows", "scroll preview"},
			{"Tab", "switch between list and preview"},
			{"Left arrow", "return focus to the Timeline list without closing preview"},
			{"[ / ]", "navigate attachments when preview focus has attachments"},
			{"Right / ]", "open preview, or focus an already-open preview from list focus"},
			{"U", "mark unread"},
			{"R / F", "reply or forward"},
			{"D / e", "delete or archive after confirmation"},
			{"* / A", "star or re-classify"},
			{"u / h", "unsubscribe when available or hide future mail"},
			{"z", "toggle full-screen preview"},
			{"v / y / Y", "visual selection and copy"},
			{"Esc", "close preview"},
		}}
	}
	return shortcutHelpSection{"Timeline", []shortcutHelpEntry{
		{"j/k or arrows", "navigate messages and threads"},
		{"Right / ]", "preview the highlighted message, or focus an already-open preview"},
		{"Left / [", "fold an expanded thread, or close preview and focus folders"},
		{"Enter", "expand a thread or open an email preview"},
		{"Space", "select highlighted messages"},
		{"U", "mark unread"},
		{"C", "open a blank Compose screen"},
		{"E / Ctrl+S", "edit or send highlighted draft"},
		{"R / F", "reply or forward highlighted non-draft email"},
		{"D / e", "delete or archive after confirmation"},
		{"* / A", "star or re-classify"},
		{"/", "start Timeline search; type ? query for semantic search"},
		{"Tab", "switch visible panels"},
	}}
}
