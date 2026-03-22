package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/ai"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// --- Thread grouping types ---

// threadGroup holds all emails that share the same normalised subject.
type threadGroup struct {
	normalizedSubject string
	emails            []*models.EmailData // newest first (inherited from sorted input)
}

// timelineRowKind distinguishes collapsed thread headers from individual email rows.
type timelineRowKind int

const (
	rowKindThread timelineRowKind = iota // collapsed thread header (>1 email, not expanded)
	rowKindEmail                         // individual email row
)

// timelineRowRef maps a table-cursor position to a thread group and email.
type timelineRowRef struct {
	kind     timelineRowKind
	group    *threadGroup
	emailIdx int // index into group.emails; meaningful only for rowKindEmail
}

// --- Folder tree types ---

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

// deleteSelected deletes the selected senders or individual messages via queue.
// All model state is read and mutations performed here (on the Update goroutine)
// before a background goroutine is launched, avoiding data races.
func (m *Model) deleteSelected() tea.Cmd {
	type deleteTarget struct {
		messageID string
		sender    string
		isDomain  bool
		folder    string
	}

	folder := m.currentFolder
	var targets []deleteTarget

	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			// Delete all selected messages (across all senders)
			for messageID := range m.selectedMessages {
				targets = append(targets, deleteTarget{messageID: messageID, folder: folder})
			}
			m.selectedMessages = make(map[string]bool) // safe: still on Update goroutine
		} else {
			// Delete current message
			cursor := m.detailsTable.Cursor()
			if cursor < len(m.detailsEmails) {
				email := m.detailsEmails[cursor]
				targets = append(targets, deleteTarget{messageID: email.MessageID, folder: folder})
			}
		}
	} else if len(m.selectedRows) > 0 {
		// Delete multiple selected senders (or domains in domain mode)
		for cursor := range m.selectedRows {
			sender, ok := m.rowToSender[cursor]
			if !ok || sender == "" {
				logger.Warn("No sender mapping found for row %d", cursor)
				continue
			}
			targets = append(targets, deleteTarget{sender: sender, isDomain: m.groupByDomain, folder: folder})
		}
		m.selectedRows = make(map[int]bool) // safe: still on Update goroutine
	} else {
		// Delete current sender using row mapping (or domain in domain mode)
		cursor := m.summaryTable.Cursor()
		sender, ok := m.rowToSender[cursor]
		if ok && sender != "" {
			targets = append(targets, deleteTarget{sender: sender, isDomain: m.groupByDomain, folder: folder})
		}
	}

	if len(targets) == 0 {
		return nil
	}

	// Send deletion requests to the queue from a goroutine so we don't block
	// the Update loop. targets is a local copy; no model state is accessed.
	ch := m.deletionRequestCh
	go func() {
		for _, t := range targets {
			ch <- models.DeletionRequest{
				MessageID: t.messageID,
				Sender:    t.sender,
				IsDomain:  t.isDomain,
				Folder:    t.folder,
			}
		}
	}()

	// Set pending counters
	m.deletionsPending = len(targets)
	m.deletionsTotal = len(targets)
	logger.Info("Queued %d deletion(s)", len(targets))

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
	// Do not close deletionRequestCh: the goroutine spawned by deleteSelected
	// may still be sending to it, and closing a channel while a sender is
	// active causes a panic. The deletion worker goroutine will be terminated
	// when the process exits.
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
	chatExtra := 0
	if m.showChat {
		chatExtra = chatPanelWidth + 2 + 2 // content + border + gap
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
	cleanupOverhead := 28 + sidebarExtra + chatExtra

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

	// --- Timeline tab: single full-width table (or split with preview) ---
	// Fixed cols: Date(16) + Size(7) + Att(3) + Tag(4) = 30; numCols=6; overhead=6*2+2=14
	const timelineFixedCols = 30
	const timelineNumCols = 6

	// Reserve half the available width for the email preview panel when one is open.
	availableForTimeline := width - sidebarExtra - chatExtra
	previewWidth := 0
	if m.selectedTimelineEmail != nil {
		previewWidth = availableForTimeline / 2
		if previewWidth < 40 {
			previewWidth = 40
		}
	}
	m.emailPreviewWidth = previewWidth

	previewBorder := 0
	if previewWidth > 0 {
		previewBorder = 1 // renderEmailPreview uses BorderLeft (+1 width)
	}
	timelineOverhead := timelineFixedCols + timelineNumCols*2 + 2 + sidebarExtra + chatExtra + previewWidth + previewBorder
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
		{Title: "Tag", Width: 4},
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

// normalizeSubject strips common reply/forward prefixes (case-insensitive) so that
// "Re: Re: Hello" and "Fwd: Hello" both map to "hello".
func normalizeSubject(s string) string {
	prefixes := []string{"re:", "fwd:", "fw:", "aw:", "tr:"}
	s = strings.TrimSpace(strings.ToLower(s))
	for {
		changed := false
		for _, p := range prefixes {
			if strings.HasPrefix(s, p) {
				s = strings.TrimSpace(s[len(p):])
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return s
}

// buildThreadGroups groups emails by normalised subject.
// emails must already be sorted newest-first; group order is determined by each
// group's most-recent email, so groups are also implicitly newest-first.
func buildThreadGroups(emails []*models.EmailData) []threadGroup {
	type entry struct {
		ns  string
		idx int // index into groups slice
	}
	var groups []threadGroup
	seen := make(map[string]int) // normalised subject → index in groups

	for _, e := range emails {
		ns := normalizeSubject(e.Subject)
		if idx, ok := seen[ns]; ok {
			groups[idx].emails = append(groups[idx].emails, e)
		} else {
			seen[ns] = len(groups)
			groups = append(groups, threadGroup{
				normalizedSubject: ns,
				emails:            []*models.EmailData{e},
			})
		}
	}
	return groups
}

// updateTimelineTable rebuilds the timeline table rows from m.timelineEmails,
// grouping them into collapsed threads where appropriate.
func (m *Model) updateTimelineTable() {
	maxSubj := m.timelineSubjectWidth
	if maxSubj <= 0 {
		maxSubj = 40
	}
	maxSend := m.timelineSenderWidth
	if maxSend <= 0 {
		maxSend = 20
	}

	trunc := func(s string, n int) string {
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		return string(r[:n-3]) + "..."
	}

	emailRow := func(email *models.EmailData, senderPrefix string) table.Row {
		dateStr := "N/A"
		if !email.Date.IsZero() {
			dateStr = email.Date.Format("06-01-02 15:04")
		}
		subject := sanitizeText(email.Subject)
		if subject == "" {
			subject = "(no subject)"
		}
		sender := senderPrefix + sanitizeText(email.Sender)
		att := "N"
		if email.HasAttachments {
			att = "Y"
		}
		tag := ""
		if m.classifications != nil {
			tag = m.classifications[email.MessageID]
		}
		return table.Row{
			trunc(sender, maxSend),
			trunc(subject, maxSubj),
			dateStr,
			fmt.Sprintf("%.1f", float64(email.Size)/1024),
			att,
			tag,
		}
	}

	// Build thread groups from the full email list
	m.threadGroups = buildThreadGroups(m.timelineEmails)
	m.threadRowMap = m.threadRowMap[:0]

	var rows []table.Row
	for gi := range m.threadGroups {
		g := &m.threadGroups[gi]
		expanded := m.expandedThreads[g.normalizedSubject]

		if len(g.emails) == 1 {
			// Single-email thread: show as a plain row
			rows = append(rows, emailRow(g.emails[0], ""))
			m.threadRowMap = append(m.threadRowMap, timelineRowRef{
				kind: rowKindEmail, group: g, emailIdx: 0,
			})
			continue
		}

		if !expanded {
			// Collapsed thread header: newest email's sender, subject with [N] prefix
			newest := g.emails[0]
			dateStr := "N/A"
			if !newest.Date.IsZero() {
				dateStr = newest.Date.Format("06-01-02 15:04")
			}
			subject := sanitizeText(newest.Subject)
			if subject == "" {
				subject = "(no subject)"
			}
			totalSize := 0
			anyAtt := false
			for _, e := range g.emails {
				totalSize += e.Size
				if e.HasAttachments {
					anyAtt = true
				}
			}
			att := "N"
			if anyAtt {
				att = "Y"
			}
			tag := ""
			if m.classifications != nil {
				tag = m.classifications[newest.MessageID]
			}
			threadSubj := fmt.Sprintf("[%d] %s", len(g.emails), subject)
			rows = append(rows, table.Row{
				trunc(sanitizeText(newest.Sender), maxSend),
				trunc(threadSubj, maxSubj),
				dateStr,
				fmt.Sprintf("%.1f", float64(totalSize)/1024),
				att,
				tag,
			})
			m.threadRowMap = append(m.threadRowMap, timelineRowRef{
				kind: rowKindThread, group: g,
			})
		} else {
			// Expanded: show each email with an indent prefix on all but the first
			for ei, email := range g.emails {
				prefix := ""
				if ei > 0 {
					prefix = "  ↳ "
				}
				rows = append(rows, emailRow(email, prefix))
				m.threadRowMap = append(m.threadRowMap, timelineRowRef{
					kind: rowKindEmail, group: g, emailIdx: ei,
				})
			}
		}
	}

	m.timelineTable.SetRows(rows)
}

// renderTimelineView renders the timeline tab content.
// When an email is selected, it splits into a list on the left and preview on the right.
func (m *Model) renderTimelineView() string {
	tableView := m.baseStyle.Render(m.timelineTable.View())

	var mainContent string
	if m.selectedTimelineEmail != nil {
		previewPanel := m.renderEmailPreview()
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, tableView, previewPanel)
	} else {
		mainContent = tableView
	}

	if m.showSidebar {
		sidebarView := m.baseStyle.Render(m.renderSidebar())
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, "  ", mainContent)
	}
	return mainContent
}

// renderEmailPreview renders the right-hand email body preview panel.
func (m *Model) renderEmailPreview() string {
	w := m.emailPreviewWidth
	if w <= 0 {
		w = 40
	}
	innerW := w - 4 // left border + padding

	var sb strings.Builder

	// Header block
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	email := m.selectedTimelineEmail
	sb.WriteString(dimStyle.Render("From: "+truncate(email.Sender, innerW-6)) + "\n")
	sb.WriteString(dimStyle.Render("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04")) + "\n")
	sb.WriteString(dimStyle.Render("Subj: "+truncate(email.Subject, innerW-6)) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	if m.emailBodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.emailBody != nil {
		// Show inline image descriptors (raw escape sequences corrupt the TUI renderer)
		for _, img := range m.emailBody.InlineImages {
			label := fmt.Sprintf("[image  %s  %d KB]", img.MIMEType, len(img.Data)/1024)
			sb.WriteString(dimStyle.Render(label) + "\n")
		}

		// Plain-text body
		body := m.emailBody.TextPlain
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		// Trim trailing whitespace and wrap to panel width
		body = strings.TrimRight(body, "\r\n\t ")
		sb.WriteString(strings.Join(wrapText(body, innerW), "\n"))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		PaddingLeft(1)

	return panelStyle.Render(sb.String())
}

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// loadEmailBodyCmd returns a tea.Cmd that fetches the email body in the background.
func (m *Model) loadEmailBodyCmd(folder string, uid uint32) tea.Cmd {
	return func() tea.Msg {
		body, err := m.backend.FetchEmailBody(folder, uid)
		return EmailBodyMsg{Body: body, Err: err}
	}
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
	)
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

	// Classification progress
	if m.classifying {
		parts = append(parts, fmt.Sprintf("Tagging… %d/%d", m.classifyDone, m.classifyTotal))
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
	if m.focusedPanel == panelChat && m.showChat {
		hints = "enter: send  │  esc/tab: close chat  │  q: quit"
	} else if m.showLogs {
		hints = "l: close logs  │  ↑/k ↓/j: scroll  │  q: quit"
	} else if m.activeTab == tabCompose {
		hints = "1/2/3: tabs  │  tab: next field  │  ctrl+s: send  │  ctrl+p: preview  │  c: chat  │  q: quit"
	} else if m.activeTab == tabTimeline {
		if m.selectedTimelineEmail != nil {
			hints = "esc: close preview  │  ↑/k ↓/j: navigate  │  R: reply  │  q: quit"
		} else {
			hints = "1/2/3: tabs  │  ↑/k ↓/j: navigate  │  enter: open  │  R: reply  │  a: AI tag  │  f: sidebar  │  c: chat  │  l: logs  │  q: quit"
		}
	} else {
		switch m.focusedPanel {
		case panelSidebar:
			hints = "1/2/3: tabs  │  tab: next panel  │  ↑/k ↓/j: nav  │  space: expand  │  enter: open  │  f: hide  │  c: chat  │  q: quit"
		case panelDetails:
			hints = "1/2/3: tabs  │  tab: next panel  │  ↑/k ↓/j: nav  │  space: select  │  D: delete  │  c: chat  │  l: logs  │  q: quit"
		default: // panelSummary
			hints = "1/2/3: tabs  │  tab: panel  │  enter: details  │  space: select  │  D: delete  │  d: domain  │  r: refresh  │  f: sidebar  │  c: chat  │  q: quit"
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
	if m.showChat {
		panels = append(panels, panelChat)
	}
	for i, p := range panels {
		if p == m.focusedPanel {
			next := panels[(i+1)%len(panels)]
			if next == panelChat {
				m.focusedPanel = panelChat
				m.chatInput.Focus()
				m.summaryTable.Blur()
				m.detailsTable.Blur()
			} else {
				m.chatInput.Blur()
				m.setFocusedPanel(next)
			}
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

// chatPanelWidth is the fixed display width of the chat panel content (excluding border)
const chatPanelWidth = 36

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

// startClassification starts background AI classification for unclassified emails.
// It closes the captured classifyCh when done so any outstanding
// listenForClassification cmd unblocks and returns ClassifyDoneMsg.
func (m *Model) startClassification() tea.Cmd {
	folder := m.currentFolder
	ch := m.classifyCh // capture the current channel
	return func() tea.Msg {
		defer close(ch) // unblock the listener when we're done
		ids, err := m.backend.GetUnclassifiedIDs(folder)
		if err != nil || len(ids) == 0 {
			return ClassifyDoneMsg{}
		}
		total := len(ids)
		for i, id := range ids {
			email, err := m.backend.GetEmailByID(id)
			if err != nil {
				continue
			}
			cat, err := m.classifier.Classify(email.Sender, email.Subject)
			if err != nil {
				logger.Warn("Classification failed for %s: %v", id, err)
				continue
			}
			_ = m.backend.SetClassification(id, cat)
			ch <- ClassifyProgressMsg{
				MessageID: id,
				Category:  cat,
				Done:      i + 1,
				Total:     total,
			}
		}
		return ClassifyDoneMsg{}
	}
}

// listenForClassification waits for the next classification result.
// Returns ClassifyDoneMsg when the channel is closed (classification finished).
func (m *Model) listenForClassification() tea.Cmd {
	ch := m.classifyCh // capture so it survives a channel replacement
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return ClassifyDoneMsg{} // channel closed — classification is done
		}
		return msg
	}
}

// loadClassifications fetches existing AI tags from cache for the current folder
func (m *Model) loadClassifications() {
	tags, err := m.backend.GetClassifications(m.currentFolder)
	if err != nil {
		logger.Warn("Failed to load classifications: %v", err)
		return
	}
	for id, cat := range tags {
		m.classifications[id] = cat
	}
}

// handleComposeKey handles all key input when on the compose tab
func (m *Model) handleComposeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "1":
		m.activeTab = tabTimeline
		m.timelineTable.Focus()
		m.timelineTable.SetStyles(m.activeTableStyle)
		m.summaryTable.Blur()
		m.detailsTable.Blur()
		m.composeBody.Blur()
		return m, m.loadTimelineEmails()
	case "2":
		return m, nil // already on compose
	case "3":
		m.activeTab = tabCleanup
		m.timelineTable.Blur()
		m.timelineTable.SetStyles(m.inactiveTableStyle)
		m.composeBody.Blur()
		m.setFocusedPanel(m.focusedPanel)
		return m, nil
	case "tab":
		m.cycleComposeField()
		return m, nil
	case "ctrl+s":
		return m, m.sendCompose()
	case "ctrl+p":
		m.composePreview = !m.composePreview
		return m, nil
	case "esc":
		m.composeStatus = ""
		return m, nil
	}
	// Forward all other keys to the focused field
	var cmd tea.Cmd
	switch m.composeField {
	case 0:
		m.composeTo, cmd = m.composeTo.Update(msg)
	case 1:
		m.composeSubject, cmd = m.composeSubject.Update(msg)
	case 2:
		m.composeBody, cmd = m.composeBody.Update(msg)
	}
	return m, cmd
}

// cycleComposeField advances focus to the next compose input field
func (m *Model) cycleComposeField() {
	m.composeField = (m.composeField + 1) % 3
	switch m.composeField {
	case 0:
		m.composeTo.Focus()
		m.composeSubject.Blur()
		m.composeBody.Blur()
	case 1:
		m.composeTo.Blur()
		m.composeSubject.Focus()
		m.composeBody.Blur()
	case 2:
		m.composeTo.Blur()
		m.composeSubject.Blur()
		m.composeBody.Focus()
	}
}

// sendCompose sends the composed message via SMTP
func (m *Model) sendCompose() tea.Cmd {
	from := m.fromAddress
	to := m.composeTo.Value()
	subject := m.composeSubject.Value()
	body := m.composeBody.Value()
	return func() tea.Msg {
		if m.mailer == nil {
			return ComposeStatusMsg{Message: "Error: SMTP not configured", Err: fmt.Errorf("smtp not configured")}
		}
		if to == "" {
			return ComposeStatusMsg{Message: "Error: To field is empty"}
		}
		if subject == "" {
			return ComposeStatusMsg{Message: "Error: Subject is empty"}
		}
		err := m.mailer.Send(from, to, subject, body)
		if err != nil {
			return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", err), Err: err}
		}
		return ComposeStatusMsg{Message: "Message sent!"}
	}
}

// renderComposeView renders the compose tab content
func (m *Model) renderComposeView() string {
	var sb strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(10)
	activeFieldStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("57"))
	inactiveFieldStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	// To field
	toStyle := inactiveFieldStyle
	if m.composeField == 0 {
		toStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("To:"),
		toStyle.Render(m.composeTo.View()),
	) + "\n")

	// Subject field
	subStyle := inactiveFieldStyle
	if m.composeField == 1 {
		subStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("Subject:"),
		subStyle.Render(m.composeSubject.View()),
	) + "\n")

	// Divider
	divWidth := m.windowWidth - 4
	if divWidth < 10 {
		divWidth = 10
	}
	sb.WriteString(strings.Repeat("─", divWidth) + "\n")

	// Body / Preview
	if m.composePreview {
		previewLabel := lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Render("  Preview (Ctrl+P to edit)  ")
		sb.WriteString(previewLabel + "\n")
		body := m.composeBody.Value()
		if body == "" {
			body = "_empty body_"
		}
		if rendered, err := glamour.Render(body, "dark"); err == nil {
			sb.WriteString(rendered)
		} else {
			sb.WriteString(body + "\n")
		}
	} else {
		bodyStyle := inactiveFieldStyle
		if m.composeField == 2 {
			bodyStyle = activeFieldStyle
		}
		sb.WriteString(bodyStyle.Render(m.composeBody.View()) + "\n")
	}

	// Status message
	if m.composeStatus != "" {
		color := "86"
		if strings.HasPrefix(m.composeStatus, "Error") || strings.HasPrefix(m.composeStatus, "Send failed") {
			color = "196"
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(m.composeStatus) + "\n")
	}

	return sb.String()
}

// submitChat sends the current chat input to Ollama with email context
func (m *Model) submitChat() tea.Cmd {
	question := strings.TrimSpace(m.chatInput.Value())
	if question == "" {
		return nil
	}
	m.chatInput.SetValue("")
	m.chatWaiting = true

	// Append user message to history
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{
		Role:    "user",
		Content: question,
	})

	// Build system prompt with email context
	var ctx strings.Builder
	ctx.WriteString(fmt.Sprintf("You are an email assistant. The user is viewing folder: %s.\n", m.currentFolder))
	if st, ok := m.folderStatus[m.currentFolder]; ok {
		ctx.WriteString(fmt.Sprintf("Folder has %d total emails, %d unread.\n", st.Total, st.Unseen))
	}
	if len(m.timelineEmails) > 0 {
		ctx.WriteString("Recent emails (newest first):\n")
		limit := 20
		if len(m.timelineEmails) < limit {
			limit = len(m.timelineEmails)
		}
		for _, e := range m.timelineEmails[:limit] {
			ctx.WriteString(fmt.Sprintf("  - From: %s | Subject: %s | Date: %s\n",
				e.Sender, e.Subject, e.Date.Format("2006-01-02")))
		}
	}

	systemMsg := ai.ChatMessage{Role: "system", Content: ctx.String()}
	messages := append([]ai.ChatMessage{systemMsg}, m.chatMessages...)

	classifier := m.classifier
	return func() tea.Msg {
		if classifier == nil {
			return ChatResponseMsg{Err: fmt.Errorf("AI not configured")}
		}
		reply, err := classifier.Chat(messages)
		return ChatResponseMsg{Content: reply, Err: err}
	}
}

// renderChatPanel renders the chat panel content (without border)
func (m *Model) renderChatPanel() string {
	w := chatPanelWidth
	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		Width(w)
	sb.WriteString(titleStyle.Render("Chat") + "\n")
	sb.WriteString(strings.Repeat("─", w) + "\n")

	// Message history — show last messages that fit in height
	msgStyle := lipgloss.NewStyle().Width(w)
	userStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Width(w)
	aiStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("87")).Width(w)

	// Calculate how many lines we have for history
	// Total height = tableHeight; minus title(1) + divider(1) + divider2(1) + input(1) = 4
	historyLines := m.windowHeight - 6 - 4 // same tableHeight formula minus chat chrome
	if historyLines < 3 {
		historyLines = 3
	}

	// Collect rendered message lines (newest-last)
	var msgLines []string
	for _, msg := range m.chatMessages {
		prefix := "AI: "
		style := aiStyle
		if msg.Role == "user" {
			prefix = "You: "
			style = userStyle
		}
		// Wrap text
		text := prefix + msg.Content
		wrapped := wrapText(text, w)
		for _, line := range wrapped {
			msgLines = append(msgLines, style.Render(line))
		}
		msgLines = append(msgLines, "")
	}
	// Show only the last historyLines
	if len(msgLines) > historyLines {
		msgLines = msgLines[len(msgLines)-historyLines:]
	}
	// Pad to fill space
	for len(msgLines) < historyLines {
		msgLines = append([]string{msgStyle.Render("")}, msgLines...)
	}
	for _, line := range msgLines {
		sb.WriteString(line + "\n")
	}

	sb.WriteString(strings.Repeat("─", w) + "\n")

	// Input field
	if m.chatWaiting {
		waitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		sb.WriteString(waitStyle.Render("Thinking..."))
	} else {
		sb.WriteString(m.chatInput.View())
	}

	return sb.String()
}

// wrapText wraps text to fit within width runes.
// Uses rune-based indexing so multi-byte characters (CJK, accented, emoji)
// are never split mid-codepoint.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	runes := []rune(text)
	for len(runes) > 0 {
		if len(runes) <= width {
			lines = append(lines, string(runes))
			break
		}
		// Find last space within width
		cut := width
		for cut > 0 && runes[cut-1] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = width
		}
		lines = append(lines, string(runes[:cut]))
		// Trim leading spaces from the remainder
		rest := runes[cut:]
		for len(rest) > 0 && rest[0] == ' ' {
			rest = rest[1:]
		}
		runes = rest
	}
	return lines
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
