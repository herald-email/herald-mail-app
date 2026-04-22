package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/ai"
)

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
	if m.classifier == nil {
		if m.demoMode {
			return ""
		}
		return lipgloss.NewStyle().Foreground(defaultTheme.DimFg).Render("AI: off")
	}
	status := m.schedulerStatus()
	label := string(status.DisplayKind())
	if label == "" {
		label = "idle"
	}
	chip := "AI: " + label
	if queued := status.DisplayQueuedCount(); queued > 0 && label != "idle" && label != "unavailable" {
		chip += fmt.Sprintf(" (+%d)", queued)
	}
	style := lipgloss.NewStyle().Foreground(defaultTheme.InfoFg)
	switch label {
	case "idle":
		style = style.Foreground(defaultTheme.DimFg)
	case "deferred":
		style = style.Foreground(defaultTheme.DemoFg)
	case "unavailable":
		style = style.Foreground(defaultTheme.ConfirmFg)
	}
	return style.Render(chip)
}

func (m *Model) renderTabBar() string {
	inactive := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(defaultTheme.TabInactiveFg)
	active := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(defaultTheme.TabActiveFg).
		Background(defaultTheme.TabActiveBg).
		Bold(true)

	tab := func(n int, label string) string {
		if m.activeTab == n {
			return active.Render(label)
		}
		return inactive.Render(label)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		tab(tabTimeline, "1  Timeline"),
		tab(tabCompose, "2  Compose"),
		tab(tabCleanup, "3  Cleanup"),
		tab(tabContacts, "4  Contacts"),
	)
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
		return lipgloss.NewStyle().
			Foreground(defaultTheme.ConfirmFg).
			Background(defaultTheme.ConfirmBg).
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
		return lipgloss.NewStyle().
			Foreground(defaultTheme.ConfirmFg).
			Background(defaultTheme.UnsubBg).
			Width(w).
			Padding(0, 1).
			Render(safeChromeLine(line, w-2))
	}

	// 3-way unsubscribe choice prompt overrides normal status bar
	if m.unsubConfirmMode {
		w := m.windowWidth
		if w <= 0 {
			w = 80
		}
		line := fmt.Sprintf("  Unsubscribe from %s:  [h] Hard unsubscribe  │  [s] Soft unsubscribe (auto-move)  │  [Esc] Cancel", m.unsubConfirmSender)
		return lipgloss.NewStyle().
			Foreground(defaultTheme.ConfirmFg).
			Background(defaultTheme.UnsubBg).
			Width(w).
			Padding(0, 1).
			Render(safeChromeLine(line, w-2))
	}

	filterPrefix := m.timelineFilterPrefix()

	// Folder breadcrumb
	folderParts := strings.Split(m.currentFolder, "/")
	breadcrumb := strings.Join(folderParts, " › ")

	var parts []string
	if msg := strings.TrimSpace(m.statusMessage); msg != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(defaultTheme.InfoFg).Render(msg))
	}
	parts = append(parts, breadcrumb)
	if chip := m.renderAIStatusChip(); chip != "" {
		parts = append(parts, chip)
	}

	// Folder counts
	if st, ok := m.folderStatus[m.currentFolder]; ok {
		parts = append(parts, fmt.Sprintf("%d unread / %d total", st.Unseen, st.Total))
	}

	// Mode (cleanup tab only)
	if m.activeTab == tabCleanup {
		if m.groupByDomain {
			parts = append(parts, "Domain mode")
		} else {
			parts = append(parts, "Sender mode")
		}
	}

	// Selection state is cleanup-local and should not leak into other tabs.
	if m.activeTab == tabCleanup && len(m.selectedRows) > 0 {
		label := "senders"
		if m.groupByDomain {
			label = "domains"
		}
		if len(m.selectedRows) == 1 {
			label = strings.TrimSuffix(label, "s")
		}
		parts = append(parts, fmt.Sprintf("%d %s selected", len(m.selectedRows), label))
	} else if m.activeTab == tabCleanup && len(m.selectedMessages) > 0 {
		// Count how many distinct sender/domain keys have selected messages
		keySet := map[string]bool{}
		for key, emails := range m.emailsBySender {
			for _, e := range emails {
				if m.selectedMessages[e.MessageID] {
					keySet[key] = true
					break
				}
			}
		}
		groupLabel := "sender"
		if m.groupByDomain {
			groupLabel = "domain"
		}
		if len(keySet) > 1 {
			parts = append(parts, fmt.Sprintf("%d messages from %d %ss selected", len(m.selectedMessages), len(keySet), groupLabel))
		} else {
			parts = append(parts, fmt.Sprintf("%d messages selected", len(m.selectedMessages)))
		}
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

	if m.activeTab != tabTimeline && m.stats != nil {
		total := 0
		for _, s := range m.stats {
			total += s.TotalEmails
		}
		parts = append(parts, fmt.Sprintf("%d senders  %d emails", len(m.stats), total))
	}

	// Classification progress
	if m.classifying {
		parts = append(parts, fmt.Sprintf("Tagging… %d/%d", m.classifyDone, m.classifyTotal))
	}

	// Background embedding progress
	if m.embeddingTotal > 0 && m.embeddingDone < m.embeddingTotal {
		parts = append(parts, fmt.Sprintf("⬡ embedding %d/%d", m.embeddingDone, m.embeddingTotal))
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
		parts = append(parts, lipgloss.NewStyle().Foreground(defaultTheme.DemoFg).Bold(true).Render("[DEMO]"))
	}

	// Dry-run mode indicator
	if m.dryRun {
		parts = append(parts, lipgloss.NewStyle().Foreground(defaultTheme.DryRunFg).Bold(true).Render("[DRY RUN]"))
	}

	// Logs indicator
	if m.showLogs {
		parts = append(parts, "Logs ON")
	}

	// Sidebar auto-hidden indicator
	if m.sidebarTooWide {
		parts = append(parts, "sidebar hidden (too narrow — widen terminal or press f)")
	}

	line := filterPrefix + strings.Join(parts, "  │  ")
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	return lipgloss.NewStyle().
		Foreground(defaultTheme.StatusFg).
		Background(defaultTheme.StatusBg).
		Width(w).
		Padding(0, 1).
		Render(truncateVisual(line, w-2))
}

// renderKeyHints renders the context-sensitive key hint line
func (m *Model) renderKeyHints() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	chrome := m.chromeState(plan)
	var hints string
	timelineHints, hasTimelineHints := m.timelineKeyHints(chrome)
	if m.pendingDeleteConfirm || m.pendingUnsubscribe {
		hints = "[y] confirm  │  [n/Esc] cancel"
	} else if hasTimelineHints {
		hints = timelineHints
	} else if chrome.FocusedPanel == panelChat && chrome.ShowChat {
		hints = "enter: send  │  esc/tab: close chat  │  q: quit"
	} else if chrome.ShowLogs {
		hints = "l: close logs  │  ↑/k ↓/j: scroll  │  q: quit"
	} else if m.activeTab == tabCompose {
		hints = "1/2/3/4: tabs  │  tab: next field  │  ctrl+s: send  │  ctrl+p: preview  │  ctrl+a: attach  │  ctrl+g: AI  │  r: refresh  │  c: chat  │  q: quit"
	} else if m.activeTab == tabContacts {
		if m.contactSearchMode == "keyword" {
			hints = fmt.Sprintf("/ %s  │  esc: clear search  │  q: quit", m.contactSearch)
		} else if m.contactSearchMode == "semantic" {
			hints = fmt.Sprintf("? %s  │  esc: clear search  │  q: quit", m.contactSearch)
		} else if m.contactFocusPanel == 1 {
			hints = "1/2/3/4: tabs  │  tab: list panel  │  ↑/k ↓/j: nav emails  │  e: enrich  │  enter: open email  │  q: quit"
		} else {
			hints = "1/2/3/4: tabs  │  tab: detail panel  │  ↑/k ↓/j: nav  │  enter: detail  │  /: search  │  ?: semantic  │  e: enrich  │  esc: clear  │  q: quit"
		}
	} else {
		switch chrome.FocusedPanel {
		case panelSidebar:
			hints = "1/2/3/4: tabs  │  tab: next panel  │  ↑/k ↓/j: nav  │  space: expand  │  enter: open  │  r: refresh  │  a: AI tag  │  f: hide  │  c: chat  │  q: quit"
		case panelDetails:
			if m.showCleanupPreview {
				hints = "↑/k ↓/j: scroll preview  │  enter: scroll down  │  z: full-screen  │  esc: close preview  │  tab: next panel  │  D: delete  │  A: re-classify  │  q: quit"
			} else {
				hints = "1/2/3/4: tabs  │  tab: next panel  │  ↑/k ↓/j: nav  │  enter: preview  │  space: select  │  D: delete  │  e: archive  │  r: refresh  │  a: AI tag  │  c: chat  │  l: logs  │  q: quit"
			}
		default: // panelSummary
			hints = "1/2/3/4: tabs  │  tab: panel  │  enter: details  │  space: select  │  D: delete  │  e: archive  │  d: domain  │  r: refresh  │  a: AI tag  │  W: rule  │  C: cleanup  │  P: prompt  │  f: sidebar  │  c: chat  │  q: quit"
		}
	}
	// Truncate to prevent wrapping that pushes the header off-screen.
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	return lipgloss.NewStyle().
		Foreground(defaultTheme.HintFg).
		Render(truncateVisual(hints, w))
}

func (m *Model) setFocusedPanel(panel int) {
	m.focusedPanel = panel
	switch panel {
	case panelSummary:
		m.summaryTable.Focus()
		m.summaryTable.SetStyles(m.activeTableStyle)
		m.detailsTable.Blur()
		m.detailsTable.SetStyles(m.inactiveTableStyle)
		m.timelineTable.Blur()
		m.timelineTable.SetStyles(m.inactiveTableStyle)
		m.chatInput.Blur()
	case panelDetails:
		m.detailsTable.Focus()
		m.detailsTable.SetStyles(m.activeTableStyle)
		m.summaryTable.Blur()
		m.summaryTable.SetStyles(m.inactiveTableStyle)
		m.timelineTable.Blur()
		m.timelineTable.SetStyles(m.inactiveTableStyle)
		m.chatInput.Blur()
	case panelTimeline:
		m.timelineTable.Focus()
		m.timelineTable.SetStyles(m.activeTableStyle)
		m.summaryTable.Blur()
		m.summaryTable.SetStyles(m.inactiveTableStyle)
		m.detailsTable.Blur()
		m.detailsTable.SetStyles(m.inactiveTableStyle)
		m.chatInput.Blur()
	case panelChat:
		m.chatInput.Focus()
		m.timelineTable.Blur()
		m.timelineTable.SetStyles(m.inactiveTableStyle)
		m.summaryTable.Blur()
		m.summaryTable.SetStyles(m.inactiveTableStyle)
		m.detailsTable.Blur()
		m.detailsTable.SetStyles(m.inactiveTableStyle)
	default:
		// panelSidebar, panelPreview, or any other non-table panel
		m.summaryTable.Blur()
		m.summaryTable.SetStyles(m.inactiveTableStyle)
		m.detailsTable.Blur()
		m.detailsTable.SetStyles(m.inactiveTableStyle)
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
		// Cleanup / other tabs
		if plan.SidebarVisible {
			panels = append(panels, panelSidebar)
		}
		if !(m.activeTab == tabCleanup && m.showCleanupPreview && plan.Cleanup.SummaryWidth == 0) {
			panels = append(panels, panelSummary)
		}
		panels = append(panels, panelDetails)
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
