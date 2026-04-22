package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

	// Chat filter indicator
	var filterPrefix string
	if m.chatFilterMode && m.activeTab == tabTimeline {
		filterLabel := m.chatFilterLabel
		if filterLabel == "" {
			filterLabel = "filtered"
		}
		filterPrefix = lipgloss.NewStyle().
			Foreground(defaultTheme.InfoFg).
			Bold(true).
			Render(fmt.Sprintf("⬡ filter: %s (%d emails)  ", filterLabel, len(m.chatFilteredEmails)))
	}

	// Folder breadcrumb
	folderParts := strings.Split(m.currentFolder, "/")
	breadcrumb := strings.Join(folderParts, " › ")

	parts := []string{breadcrumb}

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

	// Selection state
	if len(m.selectedRows) > 0 {
		parts = append(parts, fmt.Sprintf("%d senders selected", len(m.selectedRows)))
	} else if len(m.selectedMessages) > 0 {
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

	// Timeline email count
	if m.activeTab == tabTimeline {
		parts = append(parts, fmt.Sprintf("%d emails", len(m.timelineEmails)))
	} else if m.stats != nil {
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

	// Search result count
	if m.searchMode {
		if m.searchResults != nil {
			parts = append(parts, fmt.Sprintf("Search: %d results", len(m.searchResults)))
		} else {
			parts = append(parts, "Search")
		}
	}

	// Background embedding progress
	if m.embeddingTotal > 0 && m.embeddingDone < m.embeddingTotal {
		parts = append(parts, fmt.Sprintf("⬡ embedding %d/%d", m.embeddingDone, m.embeddingTotal))
	}

	// Quick reply hint / generating indicator
	if m.activeTab == tabTimeline && m.emailBody != nil {
		if !m.quickRepliesReady && m.classifier != nil {
			parts = append(parts, "⚡ generating replies…")
		} else if m.quickRepliesReady && !m.quickReplyOpen {
			parts = append(parts, "ctrl+q: quick reply")
		}
	}

	// Sync status
	switch m.syncStatusMode {
	case "idle":
		parts = append(parts, "↻ live")
	case "polling":
		if m.syncCountdown > 0 {
			parts = append(parts, fmt.Sprintf("↻ %ds", m.syncCountdown))
		}
	}

	// AI status indicator
	if m.classifier == nil && !m.demoMode {
		parts = append(parts, lipgloss.NewStyle().Foreground(defaultTheme.DimFg).Render("AI: off"))
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

	// Mouse select mode indicator
	if m.mouseMode {
		parts = append([]string{"[mouse] select mode — m: restore TUI"}, parts...)
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
	if m.pendingDeleteConfirm || m.pendingUnsubscribe {
		hints = "[y] confirm  │  [n/Esc] cancel"
	} else if m.searchMode && m.activeTab == tabTimeline {
		q := m.searchInput.View()
		hints = fmt.Sprintf("/ %s  │  esc: clear  │  ctrl+s: save  │  ctrl+i: server search", q)
		// When search returns no results and we're not already in cross-folder mode, suggest it
		query := m.searchInput.Value()
		if m.searchError != "" {
			hints = fmt.Sprintf("/ %s  │  Error: %s  │  esc: clear", q, m.searchError)
		} else if m.searchResults != nil && len(m.searchResults) == 0 && query != "" && !strings.HasPrefix(query, "/*") {
			hints = fmt.Sprintf("/ %s  │  No results in this folder — try: /* %s  │  esc: clear  │  ctrl+i: server search", q, query)
		}
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
	} else if m.chatFilterMode && m.activeTab == tabTimeline {
		hints = "esc: clear filter  │  1/2/3/4: tabs  │  ↑/k ↓/j: navigate  │  enter: open  │  q: quit"
	} else if m.activeTab == tabTimeline {
		if chrome.FocusedPanel == panelPreview {
			hasAttachments := m.emailBody != nil && len(m.emailBody.Attachments) > 0
			hasUnsub := m.emailBody != nil && m.emailBody.ListUnsubscribe != ""
			if m.visualMode {
				hints = "j/k: extend selection  │  y: copy selection  │  Y: copy all  │  esc: cancel visual"
			} else if hasAttachments && hasUnsub {
				hints = "tab/shift+tab: panels  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  s: save attachment  │  u: unsubscribe  │  esc: close  │  q: quit"
			} else if hasAttachments {
				hints = "tab/shift+tab: panels  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  s: save attachment  │  esc: close  │  q: quit"
			} else if hasUnsub {
				hints = "tab/shift+tab: panels  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  u: unsubscribe  │  esc: close  │  q: quit"
			} else {
				hints = "tab/shift+tab: panels  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  esc: close  │  q: quit"
			}
		} else if m.selectedTimelineEmail != nil {
			hints = "tab/shift+tab: panels  │  ↑/k ↓/j: navigate  │  enter: open  │  esc: close  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  A: re-classify  │  q: quit"
		} else {
			hints = "1/2/3/4: tabs  │  ↑/k ↓/j: navigate  │  enter: open  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  /: search  │  a: AI tag  │  A: re-classify  │  f: sidebar  │  q: quit"
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
	// Override hints when quick reply picker is open.
	if m.quickReplyOpen {
		hints = "↑/k ↓/j: navigate replies  │  enter: compose  │  1-8: select  │  esc: close picker  │  q: quit"
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
		if m.selectedTimelineEmail != nil {
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
