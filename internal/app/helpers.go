package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// folderNode represents one node in the folder tree
type folderNode struct {
	name     string
	fullPath string // IMAP path; empty for synthetic parent nodes
	children []*folderNode
	expanded bool
}

// sidebarItem is a flattened entry used for navigation
type sidebarItem struct {
	node  *folderNode
	depth int
}

// commonFolderPriority defines the sort order for well-known top-level folders
var commonFolderPriority = map[string]int{
	"INBOX":    0,
	"Sent":     1,
	"Drafts":   2,
	"Archive":  3,
	"Spam":     4,
	"Trash":    5,
	"Starred":  6,
	"All Mail": 7,
}

// buildFolderTree parses a flat IMAP folder list into a tree.
// Common folders (INBOX, Sent, …) are sorted to the top.
func buildFolderTree(folders []string) []*folderNode {
	sorted := make([]string, len(folders))
	copy(sorted, folders)
	sort.Strings(sorted)

	nodeMap := map[string]*folderNode{}
	var roots []*folderNode

	var getOrCreate func(path string) *folderNode
	getOrCreate = func(path string) *folderNode {
		if n, ok := nodeMap[path]; ok {
			return n
		}
		parts := strings.Split(path, "/")
		n := &folderNode{name: parts[len(parts)-1], expanded: true}
		nodeMap[path] = n
		if len(parts) == 1 {
			roots = append(roots, n)
		} else {
			parent := getOrCreate(strings.Join(parts[:len(parts)-1], "/"))
			parent.children = append(parent.children, n)
		}
		return n
	}

	for _, folder := range sorted {
		n := getOrCreate(folder)
		n.fullPath = folder
	}

	// Sort root nodes: common folders first (by priority), then alphabetical
	sort.SliceStable(roots, func(i, j int) bool {
		pi, oki := commonFolderPriority[roots[i].name]
		pj, okj := commonFolderPriority[roots[j].name]
		if oki && okj {
			return pi < pj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return roots[i].name < roots[j].name
	})

	return roots
}

// flattenTree returns all currently visible nodes in display order
func flattenTree(roots []*folderNode) []sidebarItem {
	var items []sidebarItem
	var walk func(nodes []*folderNode, depth int)
	walk = func(nodes []*folderNode, depth int) {
		for _, node := range nodes {
			items = append(items, sidebarItem{node, depth})
			if node.expanded && len(node.children) > 0 {
				walk(node.children, depth+1)
			}
		}
	}
	walk(roots, 0)
	return items
}

type tickMsg struct{}

// tickSpinner returns a command to tick the spinner
func (m *Model) tickSpinner() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// listenForProgress listens for progress updates from the IMAP client
func (m *Model) listenForProgress() tea.Cmd {
	return func() tea.Msg {
		info := <-m.progressCh
		return LoadingMsg{Info: info}
	}
}

// listenForDeletionResults listens for deletion results from the worker
func (m *Model) listenForDeletionResults() tea.Cmd {
	return func() tea.Msg {
		return <-m.deletionResultCh
	}
}

// startLoading kicks off the backend's load sequence for the current folder.
func (m *Model) startLoading() tea.Cmd {
	return func() tea.Msg {
		m.backend.Load(m.currentFolder)
		return nil
	}
}

// updateSummaryTable updates the summary table with current data
func (m *Model) updateSummaryTable() {
	if m.stats == nil {
		return
	}

	// Sort senders by total emails
	type senderStat struct {
		sender string
		stats  *models.SenderStats
	}

	var sortedStats []senderStat
	for sender, stats := range m.stats {
		sortedStats = append(sortedStats, senderStat{sender, stats})
	}

	// Sort by email count (descending), then by sender name (ascending) for stable order
	sort.Slice(sortedStats, func(i, j int) bool {
		if sortedStats[i].stats.TotalEmails == sortedStats[j].stats.TotalEmails {
			return sortedStats[i].sender < sortedStats[j].sender
		}
		return sortedStats[i].stats.TotalEmails > sortedStats[j].stats.TotalEmails
	})

	// Build table rows and mapping
	var rows []table.Row
	m.rowToSender = make(map[int]string) // Clear and rebuild mapping
	for i, item := range sortedStats {
		// Store original sender for deletion
		m.rowToSender[i] = item.sender

		// Sanitize for display
		sender := sanitizeText(item.sender)
		stats := item.stats

		// Format date range
		dateRange := "N/A"
		if !stats.FirstEmail.IsZero() && !stats.LastEmail.IsZero() {
			if stats.FirstEmail.Year() == stats.LastEmail.Year() {
				dateRange = fmt.Sprintf("%s - %s",
					stats.FirstEmail.Format("Jan"),
					stats.LastEmail.Format("Jan 2006"))
			} else {
				dateRange = fmt.Sprintf("%s - %s",
					stats.FirstEmail.Format("Jan 2006"),
					stats.LastEmail.Format("Jan 2006"))
			}
		}

		// Add selection indicator in first column
		checkmark := " "
		if m.selectedRows[i] {
			checkmark = "✓"
		}

		row := table.Row{
			checkmark,
			sender,
			fmt.Sprintf("%d", stats.TotalEmails),
			fmt.Sprintf("%.1f", stats.AvgSize/1024),
			fmt.Sprintf("%d", stats.WithAttachments),
			dateRange,
		}
		rows = append(rows, row)
	}

	m.summaryTable.SetRows(rows)
}

// updateDetailsTable updates the details table for the selected sender
func (m *Model) updateDetailsTable() {
	// Get original sender from row mapping; fall back to row 0 if cursor is out of range
	// (happens when switching to a folder that has fewer rows than the previous one)
	cursor := m.summaryTable.Cursor()
	sender, ok := m.rowToSender[cursor]
	if !ok || sender == "" {
		sender, ok = m.rowToSender[0]
	}
	if !ok || sender == "" {
		m.detailsTable.SetRows([]table.Row{})
		return
	}

	m.selectedSender = sender

	// Get emails for this sender
	emails, err := m.backend.GetEmailsBySender(m.currentFolder)
	if err != nil {
		logger.Warn("Failed to get emails for sender %s: %v", sender, err)
		m.detailsTable.SetRows([]table.Row{})
		return
	}

	senderEmails := emails[sender]
	if len(senderEmails) == 0 {
		logger.Debug("No emails found for sender: %s", sender)
		m.detailsTable.SetRows([]table.Row{})
		return
	}

	// Sort emails by date (newest first)
	sort.Slice(senderEmails, func(i, j int) bool {
		return senderEmails[i].Date.After(senderEmails[j].Date)
	})

	// Store emails for deletion
	m.detailsEmails = senderEmails

	// Debug: log selected messages
	logger.Debug("updateDetailsTable: %d messages shown, %d selected globally", len(senderEmails), len(m.selectedMessages))

	// Build table rows
	var rows []table.Row
	for _, email := range senderEmails {
		dateStr := "N/A"
		if !email.Date.IsZero() {
			dateStr = email.Date.Format("06-01-02 15:04")
		}

		subject := sanitizeText(email.Subject)
		if subject == "" {
			subject = "No Subject"
		}
		maxLen := m.subjectColWidth
		if maxLen <= 0 {
			maxLen = 32
		}
		if len(subject) > maxLen {
			subject = subject[:maxLen-3] + "..."
		}

		attachments := "N"
		if email.HasAttachments {
			attachments = "Y"
		}

		// Add selection checkmark (based on message ID, not row index)
		checkmark := " "
		if email.MessageID != "" && m.selectedMessages[email.MessageID] {
			checkmark = "✓"
		}

		row := table.Row{
			checkmark,
			dateStr,
			subject,
			fmt.Sprintf("%.1f", float64(email.Size)/1024),
			attachments,
		}
		rows = append(rows, row)
	}

	m.detailsTable.SetRows(rows)
}

// toggleDomainMode switches between domain and email grouping
func (m *Model) toggleDomainMode() {
	m.groupByDomain = !m.groupByDomain
	m.backend.SetGroupByDomain(m.groupByDomain)

	logger.Info("Toggling domain mode to: %v", m.groupByDomain)

	// Reload statistics with new grouping mode
	stats, err := m.backend.GetSenderStatistics(m.currentFolder)
	if err != nil {
		logger.Error("Failed to reload statistics after toggling domain mode: %v", err)
		return
	}

	// Update stats and refresh tables
	m.stats = stats
	m.selectedRows = make(map[int]bool) // Clear selections as row indices will change
	m.updateSummaryTable()
	m.updateDetailsTable()
}

// toggleSelection toggles selection of the current row in active table
func (m *Model) toggleSelection() {
	if m.summaryTable.Focused() {
		cursor := m.summaryTable.Cursor()
		if m.selectedRows[cursor] {
			delete(m.selectedRows, cursor)
		} else {
			m.selectedRows[cursor] = true
		}
		// Refresh the table to show/hide checkmarks
		m.updateSummaryTable()
	} else if m.detailsTable.Focused() {
		cursor := m.detailsTable.Cursor()
		if cursor < len(m.detailsEmails) {
			messageID := m.detailsEmails[cursor].MessageID
			if messageID == "" {
				logger.Warn("Cannot select message with empty MessageID")
				return
			}
			if m.selectedMessages[messageID] {
				logger.Debug("Deselecting message: %s", messageID)
				delete(m.selectedMessages, messageID)
			} else {
				logger.Debug("Selecting message: %s", messageID)
				m.selectedMessages[messageID] = true
			}
			logger.Debug("Total selected messages: %d", len(m.selectedMessages))
			// Refresh the table to show/hide checkmarks
			m.updateDetailsTable()
		}
	}
}

// deleteSelected deletes the selected senders or individual messages via queue
func (m *Model) deleteSelected() tea.Cmd {
	// Calculate deletion count before launching the goroutine; the goroutine
	// clears m.selectedMessages and m.selectedRows, so reading them afterward
	// would be a data race.
	deletionCount := 0
	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			deletionCount = len(m.selectedMessages)
		} else {
			deletionCount = 1
		}
	} else if len(m.selectedRows) > 0 {
		deletionCount = len(m.selectedRows)
	} else {
		deletionCount = 1
	}

	// Send deletion requests to the queue
	go func() {
		// Check if details table is focused - delete individual messages
		if m.detailsTable.Focused() {
			if len(m.selectedMessages) > 0 {
				// Delete all selected messages (across all senders)
				for messageID := range m.selectedMessages {
					m.deletionRequestCh <- models.DeletionRequest{
						MessageID: messageID,
						Folder:    m.currentFolder,
					}
				}
				m.selectedMessages = make(map[string]bool)
			} else {
				// Delete current message
				cursor := m.detailsTable.Cursor()
				if cursor < len(m.detailsEmails) {
					email := m.detailsEmails[cursor]
					m.deletionRequestCh <- models.DeletionRequest{
						MessageID: email.MessageID,
						Folder:    m.currentFolder,
					}
				}
			}
		} else if len(m.selectedRows) > 0 {
			// Delete multiple selected senders (or domains in domain mode)
			for cursor := range m.selectedRows {
				// Get original sender from mapping (before sanitization)
				sender, ok := m.rowToSender[cursor]
				if !ok || sender == "" {
					logger.Warn("No sender mapping found for row %d", cursor)
					continue
				}

				m.deletionRequestCh <- models.DeletionRequest{
					Sender:   sender,
					IsDomain: m.groupByDomain,
					Folder:   m.currentFolder,
				}
			}
			m.selectedRows = make(map[int]bool)
		} else {
			// Delete current sender using row mapping (or domain in domain mode)
			cursor := m.summaryTable.Cursor()
			sender, ok := m.rowToSender[cursor]
			if ok && sender != "" {
				m.deletionRequestCh <- models.DeletionRequest{
					Sender:   sender,
					IsDomain: m.groupByDomain,
					Folder:   m.currentFolder,
				}
			}
		}
	}()

	// Set pending counters
	m.deletionsPending = deletionCount
	m.deletionsTotal = deletionCount
	logger.Info("Queued %d deletion(s)", deletionCount)

	// Start listening for deletion results
	return m.listenForDeletionResults()
}

// deletionWorker processes deletion requests from the queue
func (m *Model) deletionWorker() {
	for req := range m.deletionRequestCh {
		result := models.DeletionResult{
			MessageID: req.MessageID,
			Sender:    req.Sender,
			Folder:    req.Folder,
		}

		// Perform deletion based on what's provided
		if req.MessageID != "" {
			// Delete individual message
			logger.Info("Deleting message: %s", req.MessageID)
			err := m.backend.DeleteEmail(req.MessageID, req.Folder)
			result.Error = err
			result.DeletedCount = 1
		} else if req.Sender != "" {
			if req.IsDomain {
				// Delete all messages from domain
				logger.Info("Deleting all messages from domain: %s", req.Sender)
				err := m.backend.DeleteDomainEmails(req.Sender, req.Folder)
				result.Error = err
			} else {
				// Delete all messages from sender
				logger.Info("Deleting all messages from sender: %s", req.Sender)
				err := m.backend.DeleteSenderEmails(req.Sender, req.Folder)
				result.Error = err
			}
			// We don't know the count here, would need to update Delete*Emails to return it
		}

		// Send result back
		if req.Response != nil {
			req.Response <- result
		}

		// Also send to result channel for UI updates
		m.deletionResultCh <- result
	}
}

// cleanup cleans up resources
func (m *Model) cleanup() {
	if m.deletionRequestCh != nil {
		close(m.deletionRequestCh)
	}
	if m.backend != nil {
		m.backend.Close()
	}
}

// getPhaseIcon returns an icon for the current phase
func getPhaseIcon(phase string) string {
	switch phase {
	case "scanning":
		return "🔍"
	case "processing":
		return "📧"
	case "complete":
		return "✅"
	default:
		return "⚙️"
	}
}

// calculateTextWidth estimates the visual width of text with emojis
func calculateTextWidth(text string) int {
	width := 0
	for _, r := range text {
		// Emojis and wide characters typically take 2 spaces
		if r > 127 {
			width += 2
		} else {
			width += 1
		}
	}
	return width
}

// renderProgressBar creates a visual progress bar
func (m *Model) renderProgressBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := (percent * width) / 100
	empty := width - filled

	// Create the bar with filled and empty segments
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	// Style the progress bar
	progressBarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).  // Green for filled
		Background(lipgloss.Color("235")). // Dark background
		Padding(0, 1).
		Margin(0, 2)

	return progressBarStyle.Render(fmt.Sprintf("[%s] %d%%", bar, percent))
}

// sanitizeText removes emoji and symbols while preserving all language text
func sanitizeText(text string) string {
	var result strings.Builder
	for _, r := range text {
		// Keep letters, numbers, punctuation, and spaces from any language
		// Remove only emoji, symbols, and other graphical characters
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsPunct(r) || unicode.IsSpace(r) {
			result.WriteRune(r)
		} else {
			// Replace emoji/symbols with space
			result.WriteRune(' ')
		}
	}
	// Clean up multiple consecutive spaces
	cleaned := strings.Join(strings.Fields(result.String()), " ")
	return cleaned
}

// updateTableDimensions recalculates table and column sizes based on terminal dimensions
func (m *Model) updateTableDimensions(width, height int) {
	if width == 0 {
		return
	}
	m.windowWidth = width
	m.windowHeight = height

	// Chrome: header(1) + tabbar+blank(2) + content + blank(1) + statusbar(1) + keyhints(1) = 6
	tableHeight := height - 6
	if tableHeight < 5 {
		tableHeight = 5
	}

	sidebarExtra := 0
	if m.showSidebar {
		sidebarExtra = sidebarContentWidth + 2 + 2 // content + border + gap
	}

	// --- Cleanup tab: two side-by-side tables ---
	// Fixed (non-resizable) column widths:
	//   Summary: checkmark(2) + count(6) + avgkb(7) + attach(6) + daterange(20) = 41
	//   Details: checkmark(2) + date(16) + size(8) + att(3) = 29
	const summaryFixedCols = 41
	const summaryNumCols = 6
	const detailsFixedCols = 29
	const detailsNumCols = 5

	// Total rendering overhead for two tables:
	//   summaryNumCols*2 + detailsNumCols*2 + 4 borders + 2 gap = 12+10+4+2 = 28
	cleanupOverhead := 28 + sidebarExtra

	cleanupVariable := width - cleanupOverhead - summaryFixedCols - detailsFixedCols
	if cleanupVariable < 24 {
		cleanupVariable = 24
	}
	senderWidth := cleanupVariable * 40 / 100
	if senderWidth < 12 {
		senderWidth = 12
	}
	subjectWidth := cleanupVariable - senderWidth
	if subjectWidth < 12 {
		subjectWidth = 12
	}

	m.summaryTable.SetColumns([]table.Column{
		{Title: "✓", Width: 2},
		{Title: "Sender/Domain", Width: senderWidth},
		{Title: "Count", Width: 6},
		{Title: "Avg KB", Width: 7},
		{Title: "Attach", Width: 6},
		{Title: "Date Range", Width: 20},
	})
	m.summaryTable.SetWidth(summaryFixedCols + senderWidth + summaryNumCols*2)

	m.subjectColWidth = subjectWidth
	m.detailsTable.SetColumns([]table.Column{
		{Title: "✓", Width: 2},
		{Title: "Date", Width: 16},
		{Title: "Subject", Width: subjectWidth},
		{Title: "Size", Width: 8},
		{Title: "Att", Width: 3},
	})
	m.detailsTable.SetWidth(detailsFixedCols + subjectWidth + detailsNumCols*2)

	m.summaryTable.SetHeight(tableHeight)
	m.detailsTable.SetHeight(tableHeight)

	// --- Timeline tab: single full-width table ---
	// Fixed cols: Date(16) + Size(7) + Att(3) = 26; numCols=5; overhead=5*2+2=12
	const timelineFixedCols = 26
	const timelineNumCols = 5
	timelineOverhead := timelineFixedCols + timelineNumCols*2 + 2 + sidebarExtra
	timelineVariable := width - timelineOverhead
	if timelineVariable < 24 {
		timelineVariable = 24
	}
	tSenderWidth := timelineVariable * 30 / 100
	if tSenderWidth < 10 {
		tSenderWidth = 10
	}
	tSubjectWidth := timelineVariable - tSenderWidth
	if tSubjectWidth < 14 {
		tSubjectWidth = 14
	}
	m.timelineSenderWidth = tSenderWidth
	m.timelineSubjectWidth = tSubjectWidth
	m.timelineTable.SetColumns([]table.Column{
		{Title: "Sender", Width: tSenderWidth},
		{Title: "Subject", Width: tSubjectWidth},
		{Title: "Date", Width: 16},
		{Title: "Size KB", Width: 7},
		{Title: "Att", Width: 3},
	})
	m.timelineTable.SetWidth(timelineFixedCols + tSenderWidth + tSubjectWidth + timelineNumCols*2)
	m.timelineTable.SetHeight(tableHeight)

	// Update log viewer to match
	logWidth := width - 4
	if logWidth < 20 {
		logWidth = 20
	}
	m.logViewer.SetSize(logWidth, tableHeight)
}

// loadTimelineEmails returns a Cmd that fetches all emails sorted by date
func (m *Model) loadTimelineEmails() tea.Cmd {
	folder := m.currentFolder
	return func() tea.Msg {
		emails, err := m.backend.GetTimelineEmails(folder)
		if err != nil {
			logger.Error("Failed to load timeline emails: %v", err)
			return TimelineLoadedMsg{Emails: nil}
		}
		return TimelineLoadedMsg{Emails: emails}
	}
}

// updateTimelineTable rebuilds the timeline table rows from m.timelineEmails
func (m *Model) updateTimelineTable() {
	var rows []table.Row
	for _, email := range m.timelineEmails {
		dateStr := "N/A"
		if !email.Date.IsZero() {
			dateStr = email.Date.Format("06-01-02 15:04")
		}
		subject := sanitizeText(email.Subject)
		if subject == "" {
			subject = "(no subject)"
		}
		maxSubj := m.timelineSubjectWidth
		if maxSubj <= 0 {
			maxSubj = 40
		}
		if len([]rune(subject)) > maxSubj {
			subject = string([]rune(subject)[:maxSubj-3]) + "..."
		}
		sender := sanitizeText(email.Sender)
		maxSend := m.timelineSenderWidth
		if maxSend <= 0 {
			maxSend = 20
		}
		if len([]rune(sender)) > maxSend {
			sender = string([]rune(sender)[:maxSend-3]) + "..."
		}
		att := "N"
		if email.HasAttachments {
			att = "Y"
		}
		rows = append(rows, table.Row{
			sender,
			subject,
			dateStr,
			fmt.Sprintf("%.1f", float64(email.Size)/1024),
			att,
		})
	}
	m.timelineTable.SetRows(rows)
}

// renderTimelineView renders the timeline tab content
func (m *Model) renderTimelineView() string {
	timelineView := m.baseStyle.Render(m.timelineTable.View())
	if m.showSidebar {
		sidebarView := m.baseStyle.Render(m.renderSidebar())
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, "  ", timelineView)
	}
	return timelineView
}

// renderTabBar renders the tab navigation bar
func (m *Model) renderTabBar() string {
	inactive := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color("245"))
	active := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(true)

	var t1, t2 string
	if m.activeTab == tabCleanup {
		t1 = active.Render("1  Cleanup")
		t2 = inactive.Render("2  Timeline")
	} else {
		t1 = inactive.Render("1  Cleanup")
		t2 = active.Render("2  Timeline")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, t1, t2)
}

// renderStatusBar renders the persistent bottom status bar
func (m *Model) renderStatusBar() string {
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
		parts = append(parts, fmt.Sprintf("%d messages selected", len(m.selectedMessages)))
	}

	// Deletion progress
	if m.deleting {
		completed := m.deletionsTotal - m.deletionsPending
		if m.deletionProgress.Sender != "" {
			parts = append(parts, fmt.Sprintf("Deleting %s  %d/%d", m.deletionProgress.Sender, completed, m.deletionsTotal))
		} else {
			parts = append(parts, fmt.Sprintf("Deleting…  %d/%d", completed, m.deletionsTotal))
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

	// Logs indicator
	if m.showLogs {
		parts = append(parts, "Logs ON")
	}

	line := strings.Join(parts, "  │  ")
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("237")).
		Width(w).
		Padding(0, 1).
		Render(line)
}

// renderKeyHints renders the context-sensitive key hint line
func (m *Model) renderKeyHints() string {
	var hints string
	if m.showLogs {
		hints = "l: close logs  │  ↑/k ↓/j: scroll  │  q: quit"
	} else if m.activeTab == tabTimeline {
		hints = "1/2: switch tab  │  ↑/k ↓/j: navigate  │  f: sidebar  │  l: logs  │  q: quit"
	} else {
		switch m.focusedPanel {
		case panelSidebar:
			hints = "1/2: switch tab  │  tab: next panel  │  ↑/k ↓/j: nav  │  space: expand  │  enter: open folder  │  f: hide  │  q: quit"
		case panelDetails:
			hints = "1/2: switch tab  │  tab: next panel  │  ↑/k ↓/j: nav  │  space: select  │  D: delete  │  l: logs  │  q: quit"
		default: // panelSummary
			hints = "1/2: switch tab  │  tab: panel  │  enter: details  │  space: select  │  D: delete  │  d: domain  │  r: refresh  │  f: sidebar  │  l: logs  │  q: quit"
		}
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render(hints)
}

// setFocusedPanel updates focus state and table styles for the given panel
func (m *Model) setFocusedPanel(panel int) {
	m.focusedPanel = panel
	if panel == panelSummary {
		m.summaryTable.Focus()
		m.summaryTable.SetStyles(m.activeTableStyle)
		m.detailsTable.Blur()
		m.detailsTable.SetStyles(m.inactiveTableStyle)
	} else if panel == panelDetails {
		m.detailsTable.Focus()
		m.detailsTable.SetStyles(m.activeTableStyle)
		m.summaryTable.Blur()
		m.summaryTable.SetStyles(m.inactiveTableStyle)
	} else {
		// sidebar or any non-table panel
		m.summaryTable.Blur()
		m.summaryTable.SetStyles(m.inactiveTableStyle)
		m.detailsTable.Blur()
		m.detailsTable.SetStyles(m.inactiveTableStyle)
	}
}

// cyclePanel advances focus to the next panel in order
func (m *Model) cyclePanel() {
	panels := []int{panelSummary, panelDetails}
	if m.showSidebar {
		panels = []int{panelSidebar, panelSummary, panelDetails}
	}
	for i, p := range panels {
		if p == m.focusedPanel {
			m.setFocusedPanel(panels[(i+1)%len(panels)])
			return
		}
	}
	m.setFocusedPanel(panels[0])
}

// toggleSidebarNode expands/collapses the node at sidebarCursor
func (m *Model) toggleSidebarNode() {
	items := flattenTree(m.folderTree)
	if m.sidebarCursor >= len(items) {
		return
	}
	node := items[m.sidebarCursor].node
	if len(node.children) > 0 {
		node.expanded = !node.expanded
		// Clamp cursor if it fell outside the new visible range
		newLen := len(flattenTree(m.folderTree))
		if m.sidebarCursor >= newLen {
			m.sidebarCursor = newLen - 1
		}
	}
}

// selectSidebarFolder switches to the folder at sidebarCursor
func (m *Model) selectSidebarFolder() {
	items := flattenTree(m.folderTree)
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return
	}
	node := items[m.sidebarCursor].node
	if node.fullPath == "" {
		// Synthetic parent — toggle expand instead of navigating
		m.toggleSidebarNode()
		return
	}
	m.currentFolder = node.fullPath
	m.loading = true
	m.startTime = time.Now()
	m.stats = nil
	m.selectedRows = make(map[int]bool)
	m.selectedMessages = make(map[string]bool)
	logger.Info("Switching to folder: %s", m.currentFolder)
}

// sidebarContentWidth is the fixed display width of sidebar content (excluding border)
const sidebarContentWidth = 26

// renderSidebar renders the folder tree sidebar content (without border)
func (m *Model) renderSidebar() string {
	items := flattenTree(m.folderTree)
	var sb strings.Builder

	for i, item := range items {
		indent := strings.Repeat("  ", item.depth)

		var icon string
		if len(item.node.children) > 0 {
			if item.node.expanded {
				icon = "▼ "
			} else {
				icon = "▶ "
			}
		} else {
			icon = "  "
		}

		// Build count suffix if status is available
		countSuffix := ""
		if item.node.fullPath != "" {
			if st, ok := m.folderStatus[item.node.fullPath]; ok {
				countSuffix = fmt.Sprintf(" %d/%d", st.Unseen, st.Total)
			}
		}

		prefixLen := len(indent) + 2 // icon is 2 display cells
		available := sidebarContentWidth - prefixLen - len(countSuffix)
		if available < 1 {
			available = 1
		}

		name := item.node.name
		runes := []rune(name)
		if len(runes) > available {
			if available > 3 {
				name = string(runes[:available-3]) + "..."
			} else {
				name = string(runes[:available])
			}
		}
		line := fmt.Sprintf("%s%s%-*s%s", indent, icon, available, name, countSuffix)

		if i == m.sidebarCursor {
			if m.focusedPanel == panelSidebar {
				line = lipgloss.NewStyle().
					Foreground(lipgloss.Color("229")).
					Background(lipgloss.Color("57")).
					Render(line)
			} else {
				line = lipgloss.NewStyle().
					Foreground(lipgloss.Color("250")).
					Background(lipgloss.Color("238")).
					Render(line)
			}
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// handleNavigation handles up/down navigation for the focused panel
func (m *Model) handleNavigation(direction int) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.focusedPanel == panelSidebar {
		max := len(flattenTree(m.folderTree)) - 1
		if max < 0 {
			max = 0
		}
		if direction > 0 {
			if m.sidebarCursor < max {
				m.sidebarCursor++
			}
		} else {
			if m.sidebarCursor > 0 {
				m.sidebarCursor--
			}
		}
		return m, nil
	}

	if m.summaryTable.Focused() {
		// Let the table handle navigation properly (including scrolling)
		if direction > 0 {
			m.summaryTable, cmd = m.summaryTable.Update(tea.KeyMsg{Type: tea.KeyDown})
		} else {
			m.summaryTable, cmd = m.summaryTable.Update(tea.KeyMsg{Type: tea.KeyUp})
		}
		// Auto-update details table on navigation
		m.updateDetailsTable()
	} else if m.detailsTable.Focused() {
		// Let the table handle navigation properly (including scrolling)
		if direction > 0 {
			m.detailsTable, cmd = m.detailsTable.Update(tea.KeyMsg{Type: tea.KeyDown})
		} else {
			m.detailsTable, cmd = m.detailsTable.Update(tea.KeyMsg{Type: tea.KeyUp})
		}
	}

	return m, cmd
}
