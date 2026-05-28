package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/ai"
)

const titleTabGap = " "

func (m *Model) topSyncStripSegments() (string, string) {
	message := strings.TrimSpace(m.progressInfo.Message)
	if message == "" {
		return "Live sync in progress", "waiting for IMAP updates"
	}

	if strings.HasPrefix(message, "Opening ") {
		folder := strings.TrimSuffix(strings.TrimPrefix(message, "Opening "), "...")
		if folder == "" {
			folder = displayFolderName(m.currentFolder)
		}
		return "IMAP connected", fmt.Sprintf("opening %s — another mail client may be busy", folder)
	}

	if strings.HasPrefix(message, "Checking sync state in ") {
		rest := strings.TrimSuffix(strings.TrimPrefix(message, "Checking sync state in "), "...")
		folder := rest
		detail := "reading mailbox state"
		if idx := strings.Index(rest, " ("); idx >= 0 {
			folder = rest[:idx]
			detail = strings.Trim(rest[idx+1:], "()")
		}
		if folder == "" {
			folder = displayFolderName(m.currentFolder)
		}
		return fmt.Sprintf("Syncing %s", folder), detail
	}

	if strings.HasPrefix(message, "Checking for new mail in ") {
		folder := strings.TrimSuffix(strings.TrimPrefix(message, "Checking for new mail in "), "...")
		if folder == "" {
			folder = displayFolderName(m.currentFolder)
		}
		return fmt.Sprintf("Syncing %s", folder), "comparing cache with live mailbox"
	}

	if strings.HasPrefix(message, "Refreshing ") {
		folder := strings.TrimSuffix(strings.TrimPrefix(message, "Refreshing "), " from the server...")
		if folder == "" {
			folder = displayFolderName(m.currentFolder)
		}
		return fmt.Sprintf("Refreshing %s", folder), "rebuilding local view from the server"
	}

	if strings.HasPrefix(message, "Fetching ") && strings.Contains(message, " new emails for ") {
		rest := strings.TrimSuffix(strings.TrimPrefix(message, "Fetching "), "...")
		parts := strings.SplitN(rest, " new emails for ", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("Syncing %s", parts[1]), fmt.Sprintf("preparing %s new rows", parts[0])
		}
	}

	if strings.HasPrefix(message, "Fetched ") && m.progressInfo.Total > 0 {
		return fmt.Sprintf("Syncing %s", displayFolderName(m.currentFolder)), fmt.Sprintf("%d/%d new rows cached", m.progressInfo.Current, m.progressInfo.Total)
	}

	if strings.HasPrefix(message, "No new mail in ") {
		return fmt.Sprintf("%s is current", displayFolderName(m.currentFolder)), "nothing new to load"
	}

	if strings.HasPrefix(message, "Generating statistics") {
		return fmt.Sprintf("Finalizing %s", displayFolderName(m.currentFolder)), "refreshing sender stats and counts"
	}

	if strings.HasPrefix(message, "Found ") && strings.Contains(message, " senders") {
		if !m.syncCountsSettled {
			return fmt.Sprintf("Refreshing %s", displayFolderName(m.currentFolder)), strings.ToLower(message)
		}
		return fmt.Sprintf("%s synced", displayFolderName(m.currentFolder)), strings.ToLower(message)
	}

	return "Live sync in progress", message
}

func (m *Model) schedulerStatus() ai.SchedulerStatus {
	if m.classifier == nil {
		return ai.SchedulerStatus{}
	}
	reporter, ok := m.classifier.(ai.StatusReporter)
	if !ok {
		return ai.SchedulerStatus{}
	}
	return reporter.AIStatus()
}

func (m *Model) renderAIStatusChip() string {
	if m.aiModelWarning != nil && m.aiModelWarning.Err() != nil {
		return m.theme.Severity.Error.Style().Render(fmt.Sprintf("%-10s", "AI down"))
	}
	if m.classifier == nil {
		if m.demoMode {
			return ""
		}
		return m.theme.Text.Dim.Style().Render("AI: off")
	}
	status := m.schedulerStatus()
	label := "idle"
	switch status.DisplayKind() {
	case ai.TaskKindEmbedding:
		label = "embed"
	case ai.TaskKindClassification:
		label = "tag"
	case ai.TaskKindQuickReply:
		label = "reply"
	case ai.TaskKindSemanticSearch:
		label = "search"
	case ai.TaskKindChat:
		label = "chat"
	case "deferred":
		label = "defer"
	case "unavailable":
		label = "down"
	}
	chip := fmt.Sprintf("%-10s", "AI "+label)
	style := m.theme.Severity.Info.Style()
	switch label {
	case "idle":
		style = m.theme.Text.Dim.Style()
	case "defer":
		style = m.theme.Badges.Demo.Style()
	case "down":
		style = m.theme.Severity.Error.Style()
	}
	return style.Render(chip)
}

func (m *Model) renderTitleBar(width int) string {
	if width <= 0 {
		width = 80
	}
	title := m.headerStyle.Render("Herald")
	prefix := title + titleTabGap
	if account := m.activeAccountLabel(); account != "" {
		prefix += m.theme.Chrome.TabInactive.Style().Padding(0, 1).Render(account) + titleTabGap
	}
	line := truncateVisual(prefix+m.renderTabBar(), width)
	if missing := width - ansi.StringWidth(line); missing > 0 {
		line += strings.Repeat(" ", missing)
	}
	return line
}

func (m *Model) titleTabStartX() int {
	start := ansi.StringWidth(m.headerStyle.Render("Herald")) + ansi.StringWidth(titleTabGap)
	if account := m.activeAccountLabel(); account != "" {
		start += ansi.StringWidth(m.theme.Chrome.TabInactive.Style().Padding(0, 1).Render(account)) + ansi.StringWidth(titleTabGap)
	}
	return start
}

func (m *Model) renderTabBar() string {
	inactive := m.theme.Chrome.TabInactive.Style().Padding(0, 2)
	active := m.theme.Chrome.TabActive.Style().Padding(0, 2)

	tab := func(item tabNavigationItem) string {
		label := m.tabBarLabel(item)
		if m.activeTab == item.tab {
			return active.Render(label)
		}
		return inactive.Render(label)
	}
	tabs := m.visibleTopLevelTabNavigation()
	rendered := make([]string, 0, len(tabs))
	for _, item := range tabs {
		rendered = append(rendered, tab(item))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m *Model) renderTopSyncStrip() string {
	if !m.hasTopSyncStrip() {
		return ""
	}

	w := m.windowWidth
	if w <= 0 {
		w = 80
	}

	title, detail := m.topSyncStripSegments()
	line := fmt.Sprintf(" %s  │  %s", title, detail)

	return m.theme.Chrome.TopSyncStrip.Style().
		Width(w).
		Padding(0, 1).
		Render(safeChromeLine(line, w-2))
}

// renderStatusBar renders the persistent bottom status bar
func (m *Model) renderStatusBar() string {
	// Deletion/archive confirmation prompt overrides everything
	if m.pendingDeleteConfirm {
		w := m.windowWidth
		if w <= 0 {
			w = 80
		}
		line := fmt.Sprintf("  %s  [y] confirm  [n/Esc] cancel", m.pendingDeleteDesc)
		return m.theme.Severity.Destructive.Style().
			Width(w).
			Padding(0, 1).
			Render(safeChromeLine(line, w-2))
	}
	// Unsubscribe confirmation prompt overrides everything
	if m.pendingUnsubscribe {
		w := m.windowWidth
		if w <= 0 {
			w = 80
		}
		line := fmt.Sprintf("  %s  [y] confirm  [n/Esc] cancel", m.pendingUnsubscribeDesc)
		return m.theme.Severity.Caution.Style().
			Width(w).
			Padding(0, 1).
			Render(safeChromeLine(line, w-2))
	}

	filterPrefix := m.timelineFilterPrefix()
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	compactChrome := w <= 80

	// Folder breadcrumb
	folderParts := strings.Split(displayFolderName(m.currentFolder), "/")
	breadcrumb := strings.Join(folderParts, " › ")
	statusRole := m.theme.Chrome.StatusBar

	var parts []string
	if msg := strings.TrimSpace(m.statusMessageForActiveTab()); msg != "" {
		parts = append(parts, chromeBarPart(statusRole, m.theme.Severity.Info.Style().Render(msg)))
	}
	if account := m.activeAccountLabel(); account != "" {
		parts = append(parts, account)
	}
	parts = append(parts, breadcrumb)
	if chip := m.renderAIStatusChip(); chip != "" {
		parts = append(parts, chromeBarPart(statusRole, chip))
	}
	if m.classifying {
		parts = append(parts, fmt.Sprintf("tag %d/%d", m.classifyDone, m.classifyTotal))
	}
	if m.embeddingTotal > 0 && m.embeddingDone < m.embeddingTotal {
		parts = append(parts, fmt.Sprintf("embed %d/%d", m.embeddingDone, m.embeddingTotal))
	}
	if m.loading && strings.TrimSpace(m.progressInfo.Message) != "" {
		parts = append(parts, m.progressInfo.Message)
	}

	// Folder counts
	if st, ok := m.folderStatus[m.currentFolder]; ok {
		parts = append(parts, formatFolderCountsStatus(st.Unseen, st.Total, m.loading && !m.syncCountsSettled, compactChrome))
	}

	// Deletion progress
	if m.deleting {
		completed := m.deletionsTotal - m.deletionsPending
		status := "Deleting"
		if m.connectionLost {
			status = "Deleting (reconnecting…)"
		}
		if m.deletionProgress.Sender != "" {
			parts = append(parts, fmt.Sprintf("%s %s  %d/%d", status, m.deletionProgress.Sender, completed, m.deletionsTotal))
		} else {
			parts = append(parts, fmt.Sprintf("%s…  %d/%d", status, completed, m.deletionsTotal))
		}
	}

	parts = m.appendTimelineStatusParts(parts)

	// Sync status
	switch m.syncStatusMode {
	case "idle":
		parts = append(parts, "↻ live")
	case "polling":
		if m.syncCountdown > 0 {
			parts = append(parts, fmt.Sprintf("↻ %ds", m.syncCountdown))
		}
	}

	// Demo mode indicator
	if m.demoMode {
		parts = append(parts, chromeBarPart(statusRole, m.theme.Badges.Demo.Style().Render("[DEMO]")))
	}

	// Dry-run mode indicator
	if m.dryRun {
		parts = append(parts, chromeBarPart(statusRole, m.theme.Badges.DryRun.Style().Render("[DRY RUN]")))
	}

	if m.vimFieldProfileActive() && m.activeTab == tabCompose {
		mode := strings.ToUpper(m.fieldKeyMode)
		if mode == "" {
			mode = "NORMAL"
		}
		parts = append(parts, mode)
		parts = append(parts, "Keys: "+m.keyboardProfileLabel())
	} else if m.timeline.visualMode {
		parts = append(parts, "VISUAL")
	} else if profile := m.keyboardProfileLabel(); profile != "Default" {
		parts = append(parts, "Keys: "+profile)
	}

	// Logs indicator
	if m.showLogs {
		parts = append(parts, "Logs ON")
	}

	// Sidebar auto-hidden indicator
	if m.sidebarTooWide {
		parts = append(parts, sidebarHiddenStatusNotice(compactChrome))
	}

	line := filterPrefix + strings.Join(parts, "  │  ")
	return m.theme.Chrome.StatusBar.Style().
		Width(w).
		Padding(0, 1).
		Render(truncateVisual(line, w-2))
}

func chromeBarPart(barRole ThemeStyle, rendered string) string {
	if barRole.Reverse || barRole.Background != nil {
		return ansi.Strip(rendered)
	}
	return rendered
}

func formatFolderCountsStatus(unseen, total int, unsettled, compact bool) string {
	if compact {
		if unsettled {
			return fmt.Sprintf("%du/%dt…", unseen, total)
		}
		return fmt.Sprintf("%du/%dt", unseen, total)
	}
	if unsettled {
		return fmt.Sprintf("%d unread / %d total…", unseen, total)
	}
	return fmt.Sprintf("%d unread / %d total", unseen, total)
}

func sidebarHiddenStatusNotice(compact bool) string {
	if compact {
		return "sidebar hidden"
	}
	return "sidebar hidden (too narrow - widen terminal or press B)"
}

func wrapChromeSegments(text string, width, maxLines int) []string {
	if width <= 0 {
		width = 1
	}
	if maxLines < 1 {
		maxLines = 1
	}
	parts := strings.Split(text, "  │  ")
	if len(parts) == 0 {
		return []string{""}
	}

	lines := []string{}
	current := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		candidate := part
		if current != "" {
			candidate = current + "  │  " + part
		}
		if ansi.StringWidth(candidate) <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
			if len(lines) == maxLines {
				lines[len(lines)-1] = truncateVisual(lines[len(lines)-1], width)
				return lines
			}
		}
		current = truncateVisual(part, width)
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		lines = append(lines, truncateVisual(text, width))
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[len(lines)-1] = truncateVisual(lines[len(lines)-1], width)
	}
	return lines
}

func renderChromeLines(lines []string, width int, role ThemeStyle) string {
	if width <= 0 {
		width = 80
	}
	style := role.Style().
		Width(width).
		Padding(0, 1)
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered = append(rendered, style.Render(safeChromeLine(line, width-2)))
	}
	return strings.Join(rendered, "\n")
}

func (m *Model) statusMessageForActiveTab() string {
	if m.activeTab == tabContacts && strings.TrimSpace(m.contactStatusMessage) != "" {
		return m.contactStatusMessage
	}
	return m.statusMessage
}

func (m *Model) renderStatusHintDivider() string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	return m.theme.Chrome.HintBar.Style().
		Width(w).
		Render(safeChromeLine(strings.Repeat("─", w), w))
}

// renderKeyHints renders the context-sensitive key hint line
func (m *Model) renderKeyHints() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	chrome := m.chromeState(plan)
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	return renderChromeLines(m.keyHintRows(w, chrome), w, m.theme.Chrome.HintBar)
}

func (m *Model) rawKeyHints(chrome ChromeState) string {
	return m.rawKeyHintsForWidth(m.windowWidth, chrome)
}

func (m *Model) keyHintRows(width int, chrome ChromeState) []string {
	if width <= 0 {
		width = 80
	}
	if m.showHelp {
		return wrapChromeSegments(m.shortcutHelpHintBarText(), width-2, 2)
	}
	if m.showAccountSwitcher {
		return wrapChromeSegments("up/down: move  │  enter: switch account  │  esc: close", width-2, 2)
	}
	hints := m.rawKeyHintsForWidth(width, chrome)
	if m.shouldAdvertiseShortcutHelp() {
		hints = joinHintSegments(m.commandHint(keyboardScopeGlobal, CommandHelpOpen, "help"), hints)
	}
	if layer := m.activeModifierHintLayer(); layer != modifierHintNone {
		hints = m.modifierHintText(layer, chrome, hints)
	}
	return wrapChromeSegments(hints, width-2, 2)
}

func (m *Model) rawKeyHintsForWidth(w int, chrome ChromeState) string {
	if w <= 0 {
		w = 80
	}
	var hints string
	timelineHints, hasTimelineHints := m.timelineKeyHints(chrome)
	if m.showSettings && m.settingsPanel != nil {
		hints = m.settingsPanel.keyHints()
	} else if m.pendingDeleteConfirm || m.pendingUnsubscribe {
		hints = "[y] confirm  │  [n/Esc] cancel"
	} else if chrome.FocusedPanel == panelChat && chrome.ShowChat {
		hints = joinHintSegments("enter: send", "esc/tab: close chat", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
	} else if chrome.ShowLogs {
		hints = joinHintSegments(
			fmt.Sprintf("%s/esc: close logs", displayShortcutKey(m.commandKey(keyboardScopeGlobal, CommandLogsToggle), keyDisplayHint)),
			m.movementHint("timeline", "scroll"),
			m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"),
		)
	} else if hasTimelineHints {
		hints = timelineHints
	} else if m.activeTab == tabCompose {
		if m.composeAIReviewActive() {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: original/suggestion", "ctrl+enter: accept", "esc: discard", "ctrl+z: undo", "ctrl+alt+c/b: CC/BCC")
		} else if m.composeAIPanel {
			if m.classifier == nil {
				hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: next field", "ctrl+alt+c/b: CC/BCC", "ctrl+s: send", "ctrl+p: preview", "AI disabled", "esc: back", "ctrl+c: quit")
			} else {
				hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: next field", "ctrl+alt+c/b: CC/BCC", "ctrl+k: AI prompt", "ctrl+t: translate", "ctrl+y: style", "ctrl+f: fix", "ctrl+n/e: length", "ctrl+z: undo", "esc: AI/back")
			}
		} else {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: next field", "ctrl+alt+c/b: CC/BCC", "ctrl+s: send", "ctrl+p: preview", "ctrl+a: attach", "ctrl+k: AI prompt", "esc: back", "ctrl+c: quit")
		}
		if m.composePreserved != nil {
			hints = m.primaryTabShortcutHint() + "  │  tab: next field  │  ctrl+o: preserve mode  │  ctrl+s: send  │  ctrl+p: preview  │  esc: back  │  ctrl+c: quit"
			if m.composeField == composeFieldOriginalMessage {
				hints = m.primaryTabShortcutHint() + "  │  " + m.movementHint("timeline", "scroll original") + "  │  tab: next field  │  ctrl+o: preserve mode  │  ctrl+s: send  │  esc: back  │  ctrl+c: quit"
			}
			if m.hasForwardedAttachments() {
				hints = m.primaryTabShortcutHint() + "  │  tab: next field  │  ctrl+o: preserve mode  │  ctrl+s: send  │  ctrl+p: preview  │  x: toggle fwd attach  │  esc: back  │  ctrl+c: quit"
				if m.composeField == composeFieldOriginalMessage {
					hints = m.primaryTabShortcutHint() + "  │  " + m.movementHint("timeline", "scroll original") + "  │  tab: attachments  │  ctrl+o: preserve mode  │  ctrl+s: send  │  esc: back  │  ctrl+c: quit"
				}
			}
		}
	} else if m.activeTab == tabContacts {
		if m.contactSearchMode == "keyword" {
			hints = fmt.Sprintf("/ %s  │  esc: clear search  │  q: quit", m.contactSearch)
		} else if m.contactSearchMode == "semantic" {
			hints = fmt.Sprintf("? %s  │  esc: clear search  │  q: quit", m.contactSearch)
		} else if m.contactPreviewEmail != nil {
			hints = "tab: next panel  │  esc: back to contact  │  q: quit"
		} else if m.contactFocusPanel == 1 {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: list panel", m.movementHint("contacts", "nav emails"), "e: enrich", "enter: open email", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
		} else {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: detail panel", m.movementHint("contacts", "nav"), "enter: detail", m.commandHint("contacts", CommandHelpSearch, "search"), "?: semantic", "e: enrich", "esc: clear", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
		}
	} else if m.activeTab == tabCalendar {
		crossSearchHint := ""
		if m.crossSourceSearchAvailable() {
			crossSearchHint = "x: all search"
		}
		if m.calendarEdit.Active {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: next field", "backspace: edit", "ctrl+u: clear", "ctrl+s: save", "esc: cancel", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "cache-backed")
		} else if m.calendarMeetingPrepOpen {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "esc: event detail", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only cached")
		} else if m.calendarTravelBufferOpen {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "esc: event detail", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only cached")
		} else if m.calendarAISummaryOpen {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "esc: event detail", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only cached")
		} else if m.calendarDetailOpen {
			backLabel := "esc: agenda"
			if m.calendarView == calendarViewDay {
				backLabel = "esc: day"
			} else if m.calendarView == calendarViewWeek {
				backLabel = "esc: week"
			} else if m.calendarView == calendarViewThreeDay {
				backLabel = "esc: 3-day"
			} else if m.calendarView == calendarViewSearch {
				backLabel = "esc: search"
			}
			hints = joinHintSegments(m.primaryTabShortcutHint(), "p: prep", "b: buffer", "s: summary", "e: edit", "y/m/n: RSVP", "v: cycle", backLabel, m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "provider-backed")
		} else if m.calendarView == calendarViewSearch {
			query := strings.TrimSpace(m.calendarSearchQuery)
			if query == "" {
				query = "type query"
			} else {
				query = "/ " + query
			}
			hints = joinHintSegments(m.primaryTabShortcutHint(), query, m.movementHint("calendar", "results"), "backspace: edit", "enter: detail", "esc: clear search", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only")
		} else if m.calendarView == calendarViewCrossSearch {
			query := strings.TrimSpace(m.crossSourceSearchQuery)
			if query == "" {
				query = "type query"
			} else {
				query = "x " + query
			}
			hints = joinHintSegments(m.primaryTabShortcutHint(), query, m.movementHint("calendar", "results"), "backspace: edit", "enter: event detail", "esc: clear search", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only")
		} else if m.calendarView == calendarViewDay {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: rail/main/detail", m.movementHint("calendar", "events"), "ctrl+u/d: page", "h/l: day", "y/m/n: RSVP", "t: 3-day", "w: week", "a: agenda", "/: search", crossSearchHint, "enter: detail", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only list")
		} else if m.calendarView == calendarViewWeek {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: rail/main/detail", m.movementHint("calendar", "events"), "ctrl+u/d: page", "h/l: week", "y/m/n: RSVP", "t: 3-day", "d: day", "a: agenda", "/: search", crossSearchHint, "enter: detail", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only list")
		} else if m.calendarView == calendarViewThreeDay {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: rail/main/detail", m.movementHint("calendar", "events"), "ctrl+u/d: page", "h/l: 3-day", "y/m/n: RSVP", "w: week", "d: day", "a: agenda", "/: search", crossSearchHint, "enter: detail", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only list")
		} else {
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: rail/main/detail", m.movementHint("calendar", "events"), "ctrl+u/d: page", "h/l: day", "y/m/n: RSVP", "d: day", "w: week", "t: 3-day", "/: search", crossSearchHint, "enter: detail", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only list")
		}
	} else {
		switch chrome.FocusedPanel {
		case panelSidebar:
			hints = joinHintSegments(m.primaryTabShortcutHint(), "tab: next panel", m.movementHint("timeline", "nav"), "space: expand", "enter: open", m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandSidebarToggle, "hide"), m.commandHint(keyboardScopeGlobal, CommandChatToggle, "chat"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
		default:
			hints = joinHintSegments(m.primaryTabShortcutHint(), m.commandHint(keyboardScopeGlobal, CommandAppRefresh, "refresh"), m.commandHint(keyboardScopeGlobal, CommandSidebarToggle, "sidebar"), m.commandHint(keyboardScopeGlobal, CommandChatToggle, "chat"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
		}
	}
	return hints
}

func previewActionHintText(hasUnsubscribe bool) string {
	if hasUnsubscribe {
		return "H: hide future mail  │  u: unsubscribe"
	}
	return "H: hide future mail"
}

func (m *Model) previewActionHintText(scope string, hasUnsubscribe bool) string {
	hide := m.commandHint(scope, CommandMailHideFuture, "hide future mail")
	if hasUnsubscribe {
		return joinHintSegments(hide, "u: unsubscribe")
	}
	return hide
}

func (m *Model) setFocusedPanel(panel int) {
	m.focusedPanel = panel
	m.updateTableFocusStyles()
}

func (m *Model) updateTableFocusStyles() {
	switch m.focusedPanel {
	case panelTimeline:
		m.timelineTable.Focus()
		m.timelineTable.SetStyles(m.activeTableStyle)
		m.chatInput.Blur()
	case panelChat:
		m.chatInput.Focus()
		m.timelineTable.Blur()
		m.timelineTable.SetStyles(m.inactiveTableStyle)
	default:
		// panelSidebar, panelPreview, or any other non-table panel
		m.timelineTable.Blur()
		m.timelineTable.SetStyles(m.inactiveTableStyle)
		m.chatInput.Blur()
	}
}

// cyclePanel advances (forward=true) or retreats (forward=false) focus through visible panels
func (m *Model) cyclePanel(forward bool) {
	var panels []int
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	if m.activeTab == tabTimeline {
		if plan.SidebarVisible {
			panels = append(panels, panelSidebar)
		}
		panels = append(panels, panelTimeline)
		if m.timeline.selectedEmail != nil {
			panels = append(panels, panelPreview)
		}
		if plan.ChatVisible {
			panels = append(panels, panelChat)
		}
	} else {
		if plan.ChatVisible {
			panels = append(panels, panelChat)
		}
	}
	if len(panels) == 0 {
		return
	}
	// Find current index
	cur := 0
	for i, p := range panels {
		if p == m.focusedPanel {
			cur = i
			break
		}
	}
	// Step forward or backward with wrap
	var next int
	if forward {
		next = (cur + 1) % len(panels)
	} else {
		next = (cur - 1 + len(panels)) % len(panels)
	}
	m.setFocusedPanel(panels[next])
}
