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
	shortcutHelpMaxHeight = 25
)

func (m *Model) handleShortcutHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	rawKey := msg.String()
	key := shortcutKey(msg)
	if m.showHelp {
		if m.helpSearchActive {
			switch key {
			case "ctrl+c":
				m.cleanup()
				return m, tea.Quit, true
			case "esc":
				m.helpSearchActive = false
				m.helpSearch = ""
				m.helpScrollOffset = 0
				return m, nil, true
			case "enter":
				m.helpSearchActive = false
				return m, nil, true
			case "backspace", "ctrl+h":
				if len(m.helpSearch) > 0 {
					runes := []rune(m.helpSearch)
					m.helpSearch = string(runes[:len(runes)-1])
					m.helpScrollOffset = 0
				}
				return m, nil, true
			}
			if msg.Text != "" && msg.Mod == 0 {
				m.helpSearch += msg.Text
				m.helpScrollOffset = 0
			}
			return m, nil, true
		}
		switch key {
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit, true
		case "?", "esc", "q":
			m.showHelp = false
			m.helpScrollOffset = 0
			m.helpSearchActive = false
			m.helpSearch = ""
			return m, nil, true
		case "/":
			m.helpSearchActive = true
			m.helpSearch = ""
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
		m.helpSearchActive = false
		m.helpSearch = ""
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
	if m.showChat && m.focusedPanel == panelChat {
		return true
	}
	if (m.activeTab == tabTimeline &&
		m.timeline.searchMode &&
		m.timeline.searchFocus == timelineSearchFocusInput) ||
		(m.activeTab == tabContacts && m.contactSearchMode != "") {
		return true
	}
	if m.activeTab == tabCalendar {
		if m.calendarEdit.Active {
			return true
		}
		if !m.calendarDetailOpen && (m.calendarView == calendarViewSearch || m.calendarView == calendarViewCrossSearch) {
			return true
		}
	}
	if m.activeTab == tabCompose {
		return m.composeQuestionMarkBelongsToTextInput()
	}
	return false
}

func (m *Model) composeQuestionMarkBelongsToTextInput() bool {
	if m.attachmentInputActive {
		return true
	}
	if m.composeAIPanel && (m.composeAIInput.Focused() || m.composeAIResponse.Focused()) {
		return true
	}
	switch m.composeField {
	case composeFieldTo, composeFieldCC, composeFieldBCC, composeFieldSubject, composeFieldBody:
		return true
	default:
		return false
	}
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
	visibleRows := panelH - 6
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

	backdrop := m.cachedSettingsBackdropView()
	panel := m.settingsPanel.renderPanel()
	return overlayCentered(backdrop, panel, w, h)
}

func (m *Model) cachedSettingsBackdropView() string {
	key := m.settingsBackdropKey()
	if m.settingsBackdrop.Valid && m.settingsBackdrop.Key == key && m.settingsBackdrop.Content != "" {
		return m.settingsBackdrop.Content
	}
	content := m.renderShortcutHelpBackdropView()
	m.settingsBackdrop = settingsBackdropCache{
		Valid:   true,
		Key:     key,
		Content: content,
	}
	return content
}

func (m *Model) invalidateSettingsBackdrop() {
	m.settingsBackdrop = settingsBackdropCache{}
}

func (m *Model) settingsBackdropKey() string {
	return fmt.Sprintf(
		"%dx%d|tab:%d|logs:%t|full:%t|loading:%t|events:%d|filters:%d",
		m.windowWidth,
		m.windowHeight,
		m.activeTab,
		m.showLogs,
		m.timeline.fullScreen,
		m.loading && !m.hasVisibleStartupData(),
		m.calendarEventsVersion,
		m.calendarFiltersVersion,
	)
}

func (m *Model) renderCompactOverlayView(panel string) string {
	w, h := m.compactOverlayViewportSize()
	backdrop := m.renderShortcutHelpBackdropView()
	return overlayCentered(backdrop, panel, w, h)
}

func (m *Model) compactOverlayViewportSize() (int, int) {
	w := m.windowWidth
	if w <= 0 {
		w = 120
	}
	h := m.windowHeight
	if h <= 0 {
		h = 40
	}
	return w, h
}

func (m *Model) renderShortcutHelpBackdropView() string {
	if m.loading && !m.hasVisibleStartupData() {
		return m.renderLoadingView()
	}
	if m.timeline.fullScreen {
		return m.renderFullScreenEmail()
	}
	return m.renderMainView()
}

func (m *Model) renderShortcutHelpPanel() string {
	layout := m.shortcutHelpLayout()
	title := fmt.Sprintf("Shortcut Help - %s (Profile: %s)", m.shortcutHelpContextTitle(), m.keyboardProfileLabel())
	lines := m.shortcutHelpLines(layout.contentW)
	m.helpScrollOffset = clampShortcutHelpOffset(m.helpScrollOffset, len(lines), layout.visibleRows)
	end := m.helpScrollOffset + layout.visibleRows
	if end > len(lines) {
		end = len(lines)
	}

	bodyLines := append([]string{}, lines[m.helpScrollOffset:end]...)
	for len(bodyLines) < layout.visibleRows {
		bodyLines = append(bodyLines, "")
	}

	scroll := m.shortcutHelpHintText(len(lines), layout.visibleRows, m.helpScrollOffset)

	headerStyle := lipgloss.NewStyle().Foreground(m.theme.Severity.Info.ForegroundColor()).Bold(true)
	footerStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
	content := []string{
		headerStyle.Render(ansi.Truncate(title, layout.contentW, "")),
		strings.Repeat("─", layout.contentW),
	}
	content = append(content, bodyLines...)
	content = append(content, strings.Repeat("─", layout.contentW))
	content = append(content, footerStyle.Render(ansi.Truncate(scroll, layout.contentW, "")))

	return lipgloss.NewStyle().
		Width(layout.styleWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Focus.PanelBorderFocused.ForegroundColor()).
		Padding(0, 1).
		Render(strings.Join(content, "\n"))
}

func (m *Model) shortcutHelpHintBarText() string {
	layout := m.shortcutHelpLayout()
	lines := m.shortcutHelpLines(layout.contentW)
	offset := clampShortcutHelpOffset(m.helpScrollOffset, len(lines), layout.visibleRows)
	return m.shortcutHelpHintText(len(lines), layout.visibleRows, offset)
}

func (m *Model) shortcutHelpHintText(totalRows, visibleRows, offset int) string {
	segments := m.shortcutHelpHintSegments(totalRows, visibleRows, offset)
	return joinHintSegments(segments...)
}

func (m *Model) shortcutHelpHintSegments(totalRows, visibleRows, offset int) []string {
	if m.helpSearchActive {
		leader := "type: search"
		if m.helpSearch != "" {
			leader = fmt.Sprintf("/%s", m.helpSearch)
		}
		return []string{leader, "backspace: delete", "enter: done", "esc: clear", "ctrl+c: quit"}
	}

	if query := strings.TrimSpace(m.helpSearch); query != "" {
		segments := []string{fmt.Sprintf("filter: %s", query), "/ edit", "Esc/?/q close", "ctrl+c: quit"}
		if totalRows > visibleRows {
			segments = append([]string{fmt.Sprintf("j/k scroll %d/%d", offset+1, totalRows), "pgup/pgdn: page", "home/end: jump"}, segments...)
		}
		return segments
	}

	segments := []string{"/ search", "Esc/?/q close", "ctrl+c: quit"}
	if totalRows > visibleRows {
		segments = append([]string{fmt.Sprintf("j/k scroll %d/%d", offset+1, totalRows), "pgup/pgdn: page", "home/end: jump"}, segments...)
	}
	return segments
}

func clampShortcutHelpOffset(offset, totalRows, visibleRows int) int {
	maxOffset := totalRows - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	if offset < 0 {
		return 0
	}
	return offset
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
	sections := m.shortcutHelpFilteredSections()
	lines := make([]string, 0, 40)
	for si, section := range sections {
		if si > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.Chrome.TitleBar.ForegroundColor()).Bold(true).Render(section.Title))
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

func (m *Model) shortcutHelpFilteredSections() []shortcutHelpSection {
	sections := m.shortcutHelpSections()
	query := strings.ToLower(strings.TrimSpace(m.helpSearch))
	if query == "" {
		return sections
	}
	filtered := make([]shortcutHelpSection, 0, len(sections))
	for _, section := range sections {
		entries := make([]shortcutHelpEntry, 0, len(section.Entries))
		for _, entry := range section.Entries {
			haystack := strings.ToLower(section.Title + " " + entry.Key + " " + entry.Desc)
			if strings.Contains(haystack, query) {
				entries = append(entries, entry)
			}
		}
		if len(entries) > 0 {
			section.Entries = entries
			filtered = append(filtered, section)
		}
	}
	if len(filtered) == 0 {
		return []shortcutHelpSection{{Title: "No Matches", Entries: []shortcutHelpEntry{{Key: query, Desc: "No shortcut entries match this search."}}}}
	}
	return filtered
}

func (m *Model) keyboardProfileLabel() string {
	profile := keyboardProfileDefault
	if m != nil && m.keyboard != nil {
		profile = m.keyboard.Profile()
	} else if m != nil && m.cfg != nil && strings.TrimSpace(m.cfg.Keyboard.Profile) != "" {
		profile = strings.TrimSpace(m.cfg.Keyboard.Profile)
	}
	switch strings.ToLower(profile) {
	case keyboardProfileVim:
		return "Vim"
	case keyboardProfileEmacs:
		return "Emacs"
	case keyboardProfileCustom:
		return "Custom"
	default:
		return "Default"
	}
}

func (m *Model) shortcutHelpSections() []shortcutHelpSection {
	globalEntries := []shortcutHelpEntry{
		m.commandHelpEntry(keyboardScopeGlobal, CommandHelpOpen, "open or close this shortcut help"),
		m.commandHelpEntry(keyboardScopeGlobal, CommandHelpSearch, "search this help overlay while it is open"),
		{m.primaryTabHelpKey(), m.primaryTabHelpDescription()},
	}
	if m.hasMultipleAccounts() {
		globalEntries = append(globalEntries, m.commandHelpEntry(keyboardScopeGlobal, CommandAccountSwitcher, "open account switcher"))
	}
	globalEntries = append(globalEntries,
		shortcutHelpEntry{m.commandHelpKey(keyboardScopeGlobal, CommandSidebarToggle) + " / " + m.commandHelpKey(keyboardScopeGlobal, CommandLogsToggle), "toggle sidebar or logs"},
		m.commandHelpEntry(keyboardScopeGlobal, CommandChatToggle, "toggle the AI chat panel"),
		m.commandHelpEntry(keyboardScopeGlobal, CommandAppSettings, "open settings"),
		m.commandHelpEntry(keyboardScopeGlobal, CommandAppRefresh, "refresh the current folder outside text-entry fields"),
		m.commandHelpEntry(keyboardScopeGlobal, CommandAppQuit, "quit Herald"),
	)
	sections := []shortcutHelpSection{{
		Title:   "Global",
		Entries: globalEntries,
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
			{"L / Esc", "close logs"},
			{"q", "quit when help is closed"},
		}})
	case m.focusedPanel == panelChat && m.showChat:
		sections = append(sections, shortcutHelpSection{"Chat", []shortcutHelpEntry{
			{"Enter", "send the current chat message"},
			{"Up / Down", "scroll the chat transcript by one line"},
			{"PgUp / PgDn", "scroll the chat transcript by one page"},
			{"Home / End", "jump to the oldest or newest chat transcript line"},
			{"/clear / /clean", "reset the chat conversation"},
			{"Esc", "close the chat drawer"},
			{"Tab / Shift+Tab", "move focus between visible panels"},
		}})
	case m.activeTab == tabCompose:
		sections = append(sections, m.composeShortcutHelpSection())
	case m.activeTab == tabContacts:
		sections = append(sections, m.contactsShortcutHelpSection())
	default:
		sections = append(sections, m.timelineShortcutHelpSection())
	}
	if m.usesDefaultKeyboardProfile() {
		sections = append(sections, m.defaultLegacyShortcutHelpSection())
	}
	return sections
}

func (m *Model) composeShortcutHelpSection() shortcutHelpSection {
	entries := []shortcutHelpEntry{
		{"Tab", "move to the next Compose field"},
		{"Ctrl+Enter", "send the current message"},
		{"Ctrl+S", "send fallback for terminals that do not report Ctrl+Enter"},
		{"Ctrl+P", "toggle Markdown preview"},
		{"Ctrl+A", "attach a file"},
		{"Ctrl+X", "open the draft body in an external editor"},
		{"Ctrl+Alt+C/B", "show and focus CC or BCC"},
		{"Ctrl+K", "focus the inline AI instruction field"},
		{"Ctrl+J", "suggest a subject from the draft"},
		{"Esc", "dismiss Compose status, subject suggestion, or AI bar"},
	}
	if m.composeAIPanel {
		entries = append(entries,
			shortcutHelpEntry{"Ctrl+T", "open the Translate dropdown"},
			shortcutHelpEntry{"Ctrl+Y", "open the Style dropdown"},
			shortcutHelpEntry{"Ctrl+F", "fix typos in the current draft"},
			shortcutHelpEntry{"Ctrl+N", "shorten the current draft"},
			shortcutHelpEntry{"Ctrl+E", "expand the current draft"},
			shortcutHelpEntry{"Ctrl+Z", "undo the last accepted AI rewrite"},
			shortcutHelpEntry{"Ctrl+Enter", "accept the editable AI suggestion"},
			shortcutHelpEntry{"Tab", "toggle Original/Suggestion while reviewing AI output"},
		)
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
		entries := []shortcutHelpEntry{
			{"Mouse drag", "select visible header, address, and body text"},
			{"y", "copy the active preview text selection"},
			{"m", "release or restore terminal mouse capture"},
			{"Esc", "back to the selected contact"},
			{"Tab", "switch contact panes when preview is closed"},
		}
		if m.contactsPrintablePreviewLoaded() {
			if !m.previewPrinterUnsupported() {
				entries = append(entries, m.commandHelpEntry("contacts", CommandPreviewPrint, "print the loaded preview"))
			}
		}
		return shortcutHelpSection{"Contacts Preview", entries}
	}
	if m.contactFocusPanel == 1 && m.contactDetail != nil {
		return shortcutHelpSection{"Contacts Detail", []shortcutHelpEntry{
			{"Mouse drag", "select visible contact detail or recent-email text"},
			{"y", "copy the active detail text selection"},
			{"m", "release or restore terminal mouse capture"},
			{"Enter", "open selected email preview"},
			{"Tab", "switch between list and detail panes"},
			{"Esc", "return to the contacts list"},
		}}
	}
	return shortcutHelpSection{"Contacts", []shortcutHelpEntry{
		{"h/j/k/l or arrows", "navigate contacts or contact emails"},
		{"Enter", "open contact detail or selected email preview"},
		{"Tab", "switch between list and detail panes"},
		m.commandHelpEntry("contacts", CommandHelpSearch, "start contact search; type ? query for semantic search"),
		{"e", "enrich the selected contact"},
		{"Esc", "clear search or return to the contacts list"},
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
				{"h / Left arrow", "return focus to the Timeline list without closing preview"},
				{"[ / ]", "navigate attachments when preview focus has attachments"},
				{"l / Right / ]", "open preview, or focus an already-open preview from list focus"},
				m.commandHelpEntry("timeline", CommandComposeNew, "open a blank Compose screen"),
				{"U", "mark unread"},
				{"E / Ctrl+S", "edit draft in Compose or send draft after confirmation"},
				m.commandHelpEntry("timeline", CommandMailDeleteConfirm, "discard draft after confirmation"),
				m.commandHelpEntry("timeline", CommandMailDeleteImmediate, "delete draft immediately"),
				{"z", "toggle full-screen preview"},
				{"v / y / Y", "visual selection and copy"},
				{"Esc", "close preview"},
			}}
		}
		entries := []shortcutHelpEntry{
			{"j/k or arrows", "scroll preview"},
			{"Tab", "switch between list and preview"},
			{"h / Left arrow", "return focus to the Timeline list without closing preview"},
			{"[ / ]", "navigate attachments when preview focus has attachments"},
			{"l / Right / ]", "open preview, or focus an already-open preview from list focus"},
			m.commandHelpEntry("timeline", CommandComposeNew, "open a blank Compose screen"),
			m.commandHelpEntry("timeline", CommandTimelineGroupCycle, "cycle Timeline grouping"),
			m.commandHelpEntry("timeline", CommandTimelineSortCycle, "cycle Timeline sorting"),
			{"U", "mark unread"},
			m.commandHelpEntry("timeline", CommandMailReplyAll, "reply all"),
			m.commandHelpEntry("timeline", CommandMailReplySender, "reply sender"),
			m.commandHelpEntry("timeline", CommandMailForward, "forward"),
			m.commandHelpEntry("timeline", CommandMailDeleteConfirm, "delete after confirmation"),
			m.commandHelpEntry("timeline", CommandMailDeleteImmediate, "delete immediately"),
			m.commandHelpEntry("timeline", CommandMailArchiveCurrent, "archive immediately"),
			{"* / " + m.commandHelpKey("timeline", CommandMailReclassify), "star or re-classify"},
			{"u / " + m.commandHelpKey("timeline", CommandMailHideFuture), "unsubscribe when available or hide future mail"},
			m.commandHelpEntry("timeline", CommandPreviewRevealRemoteImages, "reveal linked remote images in this message"),
			{"z", "toggle full-screen preview"},
			{"v / y / Y", "visual selection and copy"},
			{"Esc", "close preview"},
		}
		if m.timelinePrintablePreviewLoaded() {
			if !m.previewPrinterUnsupported() {
				entries = append(entries, m.commandHelpEntry("timeline", CommandPreviewPrint, "print the loaded preview"))
			}
		}
		return shortcutHelpSection{"Timeline Preview", entries}
	}
	return shortcutHelpSection{"Timeline", []shortcutHelpEntry{
		{"arrows; h/j/k/l legacy", "navigate messages and threads"},
		{"l / Right / ]", "preview the highlighted message, or focus an already-open preview"},
		{"h / Left / [", "fold an expanded thread, or close preview and focus folders"},
		m.commandHelpEntry("timeline", CommandComposeNew, "open a blank Compose screen"),
		m.commandHelpEntry("timeline", CommandTimelineGroupCycle, "cycle Timeline grouping"),
		m.commandHelpEntry("timeline", CommandTimelineSortCycle, "cycle Timeline sorting"),
		m.commandHelpEntry("timeline", CommandMailReplyAll, "reply all to highlighted non-draft email"),
		m.commandHelpEntry("timeline", CommandMailReplySender, "reply sender to highlighted non-draft email"),
		m.commandHelpEntry("timeline", CommandMailForward, "forward highlighted non-draft email"),
		m.commandHelpEntry("timeline", CommandMailDeleteConfirm, "delete highlighted or selected mail after confirmation"),
		m.commandHelpEntry("timeline", CommandMailDeleteImmediate, "delete highlighted or selected mail immediately"),
		m.commandHelpEntry("timeline", CommandMailArchiveCurrent, "archive highlighted or selected mail immediately"),
		{"Enter", "expand a thread or open an email preview"},
		{"Space", "select highlighted messages"},
		{"Shift+Up / Shift+Down", "toggle Timeline selection when supported"},
		{"V then j/k", "fallback toggle selection without shifted arrows"},
		{"Esc", "clear selected Timeline messages after local preview/search state is closed"},
		{"U", "mark unread"},
		{"E / Ctrl+S", "edit or send highlighted draft"},
		{"* / " + m.commandHelpKey("timeline", CommandMailReclassify), "star or re-classify"},
		m.commandHelpEntry("timeline", CommandHelpSearch, "start Timeline search; type ? query for semantic search"),
		{"Ctrl+D / Ctrl+U", "half-page down or up"},
		{"F6 / Shift+F6", "switch visible panels; Tab and Shift+Tab remain aliases"},
	}}
}

func (m *Model) defaultLegacyShortcutHelpSection() shortcutHelpSection {
	return shortcutHelpSection{"Default Legacy Aliases", []shortcutHelpEntry{
		{"c", "legacy alias for Ctrl+N new message"},
		{"r / R", "single-letter aliases for reply sender and reply all"},
		{"f / F", "legacy alias for Ctrl+F forward in Timeline"},
		{"d / Backspace", "legacy aliases for Delete confirmed delete"},
		{"D / Shift+Backspace", "legacy aliases for Shift+Delete immediate delete"},
		{"e / E", "secondary aliases for a archive when not editing a draft"},
		{"Tab / Shift+Tab", "legacy aliases for F6 / Shift+F6 pane focus"},
		{"h/j/k/l", "legacy navigation aliases; Vim keeps them as primaries"},
	}}
}
