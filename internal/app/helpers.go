package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/ai"
	"mail-processor/internal/backend"
	"mail-processor/internal/contacts"
	imapClient "mail-processor/internal/imap"
	"mail-processor/internal/iterm2"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	"mail-processor/internal/rules"
	appsmtp "mail-processor/internal/smtp"
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

// describeImagesCmd returns one tea.Cmd per inline image that asks the vision model for a
// one-sentence description. Each command resolves to an ImageDescMsg.
func describeImagesCmd(classifier ai.AIClient, images []models.InlineImage) []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(images))
	for _, img := range images {
		img := img // capture loop variable
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			desc, err := classifier.DescribeImage(ctx, img.Data, img.MIMEType)
			return ImageDescMsg{ContentID: img.ContentID, Description: desc, Err: err}
		})
	}
	return cmds
}

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

// listenForValidIDs waits for the background reconciliation to send the live
// valid-ID set, then delivers it as ValidIDsMsg so all views can re-filter.
func (m *Model) listenForValidIDs() tea.Cmd {
	ch := m.validIDsCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ids, ok := <-ch
		if !ok {
			return nil
		}
		return ValidIDsMsg{ValidIDs: ids}
	}
}

// listenForDeletionResults listens for deletion results from the worker
func (m *Model) listenForDeletionResults() tea.Cmd {
	return func() tea.Msg {
		return <-m.deletionResultCh
	}
}

// ruleWorker processes emails through the rule engine serially.
func (m *Model) ruleWorker() {
	engine := rules.New(m.backend, m.backend, m.classifier)
	for req := range m.ruleRequestCh {
		fired, err := engine.EvaluateEmail(req.Email, req.Category)
		select {
		case m.ruleResultCh <- models.RuleResult{
			MessageID:  req.Email.MessageID,
			FiredCount: fired,
			Err:        err,
		}:
		default:
			// result dropped if channel full — rule fired but UI won't see the count
		}
	}
}

// listenForRuleResult waits for a single result from the rule engine worker.
func (m *Model) listenForRuleResult() tea.Cmd {
	return func() tea.Msg {
		result := <-m.ruleResultCh
		return RuleResultMsg{Result: result}
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
		subject = truncate(subject, maxLen)

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
	return m.queueRequests(false)
}

// archiveSelected archives the selected senders or individual messages via queue.
func (m *Model) archiveSelected() tea.Cmd {
	return m.queueRequests(true)
}

// queueRequests builds deletion/archive requests and sends them to the worker.
func (m *Model) queueRequests(isArchive bool) tea.Cmd {
	type deleteTarget struct {
		messageID string
		sender    string
		isDomain  bool
		folder    string
	}

	folder := m.currentFolder
	var targets []deleteTarget

	// Timeline tab: delete/archive current email
	if m.activeTab == tabTimeline {
		cursor := m.timelineTable.Cursor()
		if cursor < len(m.threadRowMap) {
			ref := m.threadRowMap[cursor]
			var email *models.EmailData
			if ref.kind == rowKindThread {
				email = ref.group.emails[0]
			} else {
				email = ref.group.emails[ref.emailIdx]
			}
			if email != nil {
				targets = append(targets, deleteTarget{messageID: email.MessageID, folder: email.Folder})
			}
		}
		if len(targets) == 0 {
			return nil
		}
		ch := m.deletionRequestCh
		go func() {
			for _, t := range targets {
				ch <- models.DeletionRequest{
					MessageID: t.messageID,
					Folder:    t.folder,
					IsArchive: isArchive,
				}
			}
		}()
		m.deletionsPending = len(targets)
		m.deletionsTotal = len(targets)
		return m.listenForDeletionResults()
	}

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
				IsArchive: isArchive,
			}
		}
	}()

	// Set pending counters
	m.deletionsPending = len(targets)
	m.deletionsTotal = len(targets)
	logger.Info("Queued %d deletion(s) isArchive=%v", len(targets), isArchive)

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

		// Perform deletion or archive based on what's provided
		if req.MessageID != "" {
			if req.IsArchive {
				logger.Info("Archiving message: %s", req.MessageID)
				result.Error = m.backend.ArchiveEmail(req.MessageID, req.Folder)
			} else {
				logger.Info("Deleting message: %s", req.MessageID)
				result.Error = m.backend.DeleteEmail(req.MessageID, req.Folder)
			}
			result.DeletedCount = 1
		} else if req.Sender != "" {
			if req.IsArchive {
				logger.Info("Archiving all messages from sender: %s", req.Sender)
				result.Error = m.backend.ArchiveSenderEmails(req.Sender, req.Folder)
			} else if req.IsDomain {
				logger.Info("Deleting all messages from domain: %s", req.Sender)
				result.Error = m.backend.DeleteDomainEmails(req.Sender, req.Folder)
			} else {
				logger.Info("Deleting all messages from sender: %s", req.Sender)
				result.Error = m.backend.DeleteSenderEmails(req.Sender, req.Folder)
			}
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

	// Determine how much space the sidebar would consume.
	// If the sidebar leaves fewer than 16 variable cols for the timeline, treat it
	// as hidden in the layout (sidebarTooWide=true). The user's showSidebar preference
	// is preserved — pressing f will toggle it back when the terminal is widened.
	sidebarWouldConsume := sidebarContentWidth + 2 + 2 // content + border + gap
	const minTimelineVariableCols = 16
	m.sidebarTooWide = m.showSidebar &&
		(width-sidebarWouldConsume) < (minTermWidth+minTimelineVariableCols)

	sidebarExtra := 0
	if m.showSidebar && !m.sidebarTooWide {
		sidebarExtra = sidebarWouldConsume
	}
	chatExtra := 0
	if m.showChat {
		chatExtra = chatPanelWidth + 2 + 2 // content + border + gap
	}

	// --- Cleanup tab: two (or three when preview open) side-by-side tables ---
	// Fixed (non-resizable) column widths:
	//   Summary: checkmark(2) + count(6) + avgkb(7) + attach(6) + daterange(20) = 41
	//   Details: checkmark(2) + date(16) + size(8) + att(3) = 29
	const summaryFixedCols = 41
	const summaryNumCols = 6
	const detailsFixedCols = 29
	const detailsNumCols = 5

	// When cleanup preview is open, compute a 3-column layout (25%/25%/50%).
	// Sidebar is hidden while preview is open.
	availForCleanup := width - chatExtra
	if m.showCleanupPreview {
		// Hide sidebar while preview is shown
		availForCleanup = width - chatExtra
		// Preview occupies ~50% of the available width
		cleanupPreviewW := availForCleanup / 2
		if cleanupPreviewW < 25 {
			cleanupPreviewW = 25
		}
		m.cleanupPreviewWidth = cleanupPreviewW

		// The two tables share the remaining ~50%
		// Total rendering overhead for two tables: summaryNumCols*2 + detailsNumCols*2 + 4 borders + 2 gaps = 28
		// Plus 1 border for the preview panel + 2 gaps = 3 extra
		tablesAvail := availForCleanup - cleanupPreviewW - 3
		if tablesAvail < 0 {
			tablesAvail = 0
		}
		// Split tables evenly: each gets half
		tableHalf := tablesAvail / 2
		// Fixed overhead for each table: summaryFixed + numCols*2 or detailsFixed + numCols*2
		perTableOverhead := summaryFixedCols + summaryNumCols*2
		senderW := tableHalf - perTableOverhead
		if senderW < 4 {
			senderW = 4
		}
		perTableOverhead2 := detailsFixedCols + detailsNumCols*2
		subjectW := tablesAvail - tableHalf - perTableOverhead2
		if subjectW < 4 {
			subjectW = 4
		}

		m.summaryTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Sender/Domain", Width: senderW},
			{Title: "Count", Width: 6},
			{Title: "Avg KB", Width: 7},
			{Title: "Attach", Width: 6},
			{Title: "Date Range", Width: 20},
		})
		m.summaryTable.SetWidth(summaryFixedCols + senderW + summaryNumCols*2)

		m.subjectColWidth = subjectW
		m.detailsTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: subjectW},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: 3},
		})
		m.detailsTable.SetWidth(detailsFixedCols + subjectW + detailsNumCols*2)
	} else {
		m.cleanupPreviewWidth = 0

		// Total rendering overhead for two tables:
		//   summaryNumCols*2 + detailsNumCols*2 + 4 borders + 2 gap = 12+10+4+2 = 28
		cleanupOverhead := 28 + sidebarExtra + chatExtra

		cleanupVariable := width - cleanupOverhead - summaryFixedCols - detailsFixedCols
		if cleanupVariable < 0 {
			cleanupVariable = 0
		}
		senderWidth := cleanupVariable * 40 / 100
		subjectWidth := cleanupVariable - senderWidth
		if cleanupVariable >= 24 {
			if senderWidth < 12 {
				senderWidth = 12
			}
			if subjectWidth < 12 {
				subjectWidth = 12
			}
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
	}

	m.summaryTable.SetHeight(tableHeight)
	m.detailsTable.SetHeight(tableHeight)

	// --- Timeline tab: single full-width table (or split with preview) ---
	// Fixed cols: Date(16) + Size(7) + Att(3) + Tag(4) = 30; numCols=6; overhead=6*2+2=14
	const timelineFixedCols = 30
	const timelineNumCols = 6

	// Reserve roughly half the available width for the email preview panel.
	// The preview is only shown when there is enough room: the table's fixed
	// overhead (timelineFixedCols + separators + baseStyle border = 44) plus the
	// preview's border (1) must still leave at least one column for variable content.
	// Minimum useful preview width is 25 cols.
	const timelineTableFixedOverhead = timelineFixedCols + timelineNumCols*2 + 2 // = 44
	const minPreviewWidth = 25
	availableForTimeline := width - sidebarExtra - chatExtra
	previewWidth := 0
	if m.selectedTimelineEmail != nil {
		// Cap preview so the table always has at least 0 variable cols (no overflow).
		// previewBorder(1) is included in the cap.
		maxPreview := availableForTimeline - timelineTableFixedOverhead - 1
		if maxPreview >= minPreviewWidth {
			previewWidth = availableForTimeline / 2
			if previewWidth < minPreviewWidth {
				previewWidth = minPreviewWidth
			}
			if previewWidth > maxPreview {
				previewWidth = maxPreview
			}
		}
		// If maxPreview < minPreviewWidth there is not enough room; preview stays 0.
	}
	m.emailPreviewWidth = previewWidth

	previewBorder := 0
	if previewWidth > 0 {
		previewBorder = 1 // renderEmailPreview uses BorderLeft (+1 width)
	}
	timelineOverhead := timelineTableFixedOverhead + sidebarExtra + chatExtra + previewWidth + previewBorder
	timelineVariable := width - timelineOverhead
	if timelineVariable < 0 {
		timelineVariable = 0
	}
	tSenderWidth := timelineVariable * 30 / 100
	tSubjectWidth := timelineVariable - tSenderWidth
	// Enforce display minimums only when the budget permits; never cause overflow.
	if timelineVariable >= 24 {
		if tSenderWidth < 10 {
			tSenderWidth = 10
		}
		if tSubjectWidth < 14 {
			tSubjectWidth = 14
		}
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

	// Update compose field widths (B4/B5)
	// Label is 10 wide, borders add 2, so input width = total - chatExtra - 12
	composeInputWidth := width - chatExtra - 12
	if composeInputWidth < 10 {
		composeInputWidth = 10
	}
	m.composeTo.Width = composeInputWidth
	m.composeSubject.Width = composeInputWidth
	// Body: no label, borders add 2
	composeBodyWidth := width - chatExtra - 2
	if composeBodyWidth < 10 {
		composeBodyWidth = 10
	}
	// Reserve rows for: To(3) + Subject(3) + divider(1) + status(1) + body border top/bot(2) = 10
	composeBodyHeight := tableHeight - 10
	if composeBodyHeight < 3 {
		composeBodyHeight = 3
	}
	m.composeBody.SetWidth(composeBodyWidth)
	m.composeBody.SetHeight(composeBodyHeight)
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
	var groups []threadGroup
	seen := make(map[string]int) // (normalised subject + "\n" + sender) → index in groups

	for _, e := range emails {
		ns := normalizeSubject(e.Subject)
		if ns == "" {
			// Empty subjects are never grouped; each stands alone.
			groups = append(groups, threadGroup{
				normalizedSubject: ns,
				emails:            []*models.EmailData{e},
			})
			continue
		}
		key := ns + "\n" + strings.ToLower(e.Sender)
		if idx, ok := seen[key]; ok {
			groups[idx].emails = append(groups[idx].emails, e)
		} else {
			seen[key] = len(groups)
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
		if n <= 0 {
			return ""
		}
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		if n <= 3 {
			return string(r[:n])
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
		// Prepend similarity score badge for semantic search results
		if m.semanticScores != nil {
			if score, ok := m.semanticScores[email.MessageID]; ok {
				pct := int(score * 100)
				subject = fmt.Sprintf("[%d%%] %s", pct, subject)
			}
		}
		unreadDot := " "
		if !email.IsRead {
			unreadDot = "●"
		}
		sender := unreadDot + senderPrefix + sanitizeText(email.Sender)
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

	// Priority: chat filter > search results > full list
	var displayEmails []*models.EmailData
	switch {
	case m.chatFilterMode:
		displayEmails = m.chatFilteredEmails
	case m.searchMode && m.searchResults != nil:
		displayEmails = m.searchResults
	default:
		displayEmails = m.timelineEmails
	}

	// Build thread groups from the full email list
	m.threadGroups = buildThreadGroups(displayEmails)
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
	var tableView string
	if m.timelineEmails != nil && len(m.timelineEmails) == 0 {
		tableView = m.emptyStateView("No emails in this folder  •  press r to refresh")
	} else {
		tableView = m.baseStyle.Render(m.timelineTable.View())
	}

	var mainContent string
	if m.selectedTimelineEmail != nil {
		previewPanel := m.renderEmailPreview()
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, tableView, previewPanel)
	} else {
		mainContent = tableView
	}

	if m.showSidebar && !m.sidebarTooWide {
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

	// Focus-aware colors: brighter when preview panel has focus
	borderColor := "238"
	headerColor := "245"
	if m.focusedPanel == panelPreview {
		borderColor = "39"
		headerColor = "255"
	}

	// Header block
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(headerColor))
	email := m.selectedTimelineEmail
	sb.WriteString(headerStyle.Render("From: "+truncate(email.Sender, innerW-6)) + "\n")
	sb.WriteString(headerStyle.Render("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04")) + "\n")
	sb.WriteString(headerStyle.Render("Subj: "+truncate(email.Subject, innerW-6)) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	panelHeight := m.windowHeight - 6
	if panelHeight < 5 {
		panelHeight = 5
	}
	// Header block is 4 rows (From + Date + Subj + separator).
	// Reserve 1 row for scroll indicator → maxBodyLines = panelHeight - 4 - 1.
	maxBodyLines := panelHeight - 5
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}

	// Reserve rows for the quick reply picker overlay when open.
	pickerLines := 0
	if m.quickReplyOpen {
		pickerLines = 2 + len(m.quickReplies) // divider + header + items
		if len(m.quickReplies) == 0 {
			pickerLines = 3
		}
		if pickerLines > 12 {
			pickerLines = 12
		}
		maxBodyLines -= pickerLines
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}
	}

	dimStyle := headerStyle
	if m.emailBodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.emailBody != nil {
		// Show inline image descriptors (raw escape sequences corrupt the TUI renderer)
		imageLines := 0
		for _, img := range m.emailBody.InlineImages {
			var label string
			if iterm2.IsSupported() {
				// Render image inline via iTerm2 protocol
				rendered := iterm2.Render(img.Data, innerW)
				if rendered != "" {
					sb.WriteString(rendered)
					imageLines++
					continue
				}
			}
			if desc, ok := m.inlineImageDescs[img.ContentID]; ok {
				label = fmt.Sprintf("[Image: %s]", desc)
			} else if m.classifier != nil && m.classifier.HasVisionModel() {
				label = fmt.Sprintf("[image  %s  %d KB  — describing…]", img.MIMEType, len(img.Data)/1024)
			} else {
				label = fmt.Sprintf("[image: %s]", img.MIMEType)
			}
			label = truncate(label, innerW)
			sb.WriteString(dimStyle.Render(label) + "\n")
			imageLines++
		}

		// Show downloadable attachments
		attachStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
		selectedAttachStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
		for i, att := range m.emailBody.Attachments {
			sizeStr := fmt.Sprintf("%.1f KB", float64(att.Size)/1024)
			if att.Size >= 1024*1024 {
				sizeStr = fmt.Sprintf("%.1f MB", float64(att.Size)/(1024*1024))
			}
			label := fmt.Sprintf("[attach] %s  %s  %s", att.Filename, att.MIMEType, sizeStr)
			label = truncate(label, innerW)
			if i == m.selectedAttachment {
				sb.WriteString(selectedAttachStyle.Render(label) + "\n")
			} else {
				sb.WriteString(attachStyle.Render(label) + "\n")
			}
			imageLines++
		}

		// Save-path prompt
		if m.attachmentSavePrompt {
			promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
			sb.WriteString(promptStyle.Render("Save to: ") + m.attachmentSaveInput.View() + "\n")
			imageLines++
		}

		// Body — wrap/render once and cache; re-render only if panel width changed
		body := stripInvisibleChars(m.emailBody.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		if m.bodyWrappedLines == nil || m.bodyWrappedWidth != innerW {
			if m.emailBody.IsFromHTML {
				// Render markdown (converted from HTML) via glamour at panel width
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						// Clamp any lines exceeding innerW (e.g. long URLs that
						// glamour couldn't break at a word boundary).
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						m.bodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.bodyWrappedLines = wrapLines(body, innerW)
					}
				} else {
					m.bodyWrappedLines = wrapLines(body, innerW)
				}
			} else {
				m.bodyWrappedLines = wrapLines(body, innerW)
			}
			m.bodyWrappedWidth = innerW
		}

		// Clamp scroll offset
		visibleLines := maxBodyLines - imageLines
		if visibleLines < 1 {
			visibleLines = 1
		}
		totalLines := len(m.bodyWrappedLines)
		maxOffset := totalLines - visibleLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.bodyScrollOffset > maxOffset {
			m.bodyScrollOffset = maxOffset
		}

		end := m.bodyScrollOffset + visibleLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(renderBodyLines(m.bodyWrappedLines, m.bodyScrollOffset, end,
			m.visualMode, m.visualStart, m.visualEnd))

		// Scroll indicator
		if totalLines > visibleLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%", m.bodyScrollOffset+1, totalLines, pct)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		}
	}

	// Quick reply picker overlay at the bottom of the preview panel.
	if m.quickReplyOpen {
		sb.WriteString(m.renderQuickReplyPicker(innerW))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w).
		Height(panelHeight).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingLeft(1)

	return panelStyle.Render(sb.String())
}

// renderBodyLines joins lines[start:end] into a string, applying a purple
// highlight to lines within [visualStart, visualEnd] when visualMode is true.
func renderBodyLines(lines []string, start, end int, visualMode bool, visualStart, visualEnd int) string {
	if !visualMode {
		return strings.Join(lines[start:end], "\n")
	}
	highlightStyle := lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("229"))
	lo, hi := visualStart, visualEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	var sb strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			sb.WriteByte('\n')
		}
		if i >= lo && i <= hi {
			sb.WriteString(highlightStyle.Render(lines[i]))
		} else {
			sb.WriteString(lines[i])
		}
	}
	return sb.String()
}

// renderFullScreenEmail renders the email preview filling the entire terminal.
// All chrome (tab bar, sidebar, timeline, status bar, key hints) is hidden.
func (m *Model) renderFullScreenEmail() string {
	innerW := m.windowWidth - 2
	if innerW < 10 {
		innerW = 10
	}

	var sb strings.Builder

	headerColor := "255"
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(headerColor))

	email := m.selectedTimelineEmail
	sb.WriteString(headerStyle.Render("From: "+truncate(email.Sender, innerW-6)) + "\n")
	sb.WriteString(headerStyle.Render("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04")) + "\n")
	sb.WriteString(headerStyle.Render("Subj: "+truncate(email.Subject, innerW-6)) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	// Reserve 1 row at the bottom for the scroll indicator.
	// Also reserve rows for the quick reply picker overlay when open.
	maxBodyLines := m.windowHeight - 5 // 4 header rows + 1 scroll indicator
	if m.quickReplyOpen {
		pickerRows := 2 + len(m.quickReplies)
		if len(m.quickReplies) == 0 {
			pickerRows = 3
		}
		if pickerRows > 12 {
			pickerRows = 12
		}
		maxBodyLines -= pickerRows
	}
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	if m.emailBodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.emailBody != nil {
		// Show inline images — same 3-path logic as split view
		imageLines := 0
		for _, img := range m.emailBody.InlineImages {
			if iterm2.IsSupported() {
				rendered := iterm2.Render(img.Data, innerW)
				if rendered != "" {
					sb.WriteString(rendered)
					imageLines++
					continue
				}
			}
			var label string
			if desc, ok := m.inlineImageDescs[img.ContentID]; ok {
				label = fmt.Sprintf("[Image: %s]", desc)
			} else if m.classifier != nil && m.classifier.HasVisionModel() {
				label = fmt.Sprintf("[image  %s  %d KB  — describing…]", img.MIMEType, len(img.Data)/1024)
			} else {
				label = fmt.Sprintf("[image: %s]", img.MIMEType)
			}
			label = truncate(label, innerW)
			sb.WriteString(dimStyle.Render(label) + "\n")
			imageLines++
		}
		// Reserve lines used by image labels so body scroll accounting is correct
		maxBodyLines -= imageLines
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}

		body := stripInvisibleChars(m.emailBody.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		// Re-wrap if width changed (full-screen uses different innerW than split view)
		if m.bodyWrappedLines == nil || m.bodyWrappedWidth != innerW {
			if m.emailBody.IsFromHTML {
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						m.bodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.bodyWrappedLines = wrapLines(body, innerW)
					}
				} else {
					m.bodyWrappedLines = wrapLines(body, innerW)
				}
			} else {
				m.bodyWrappedLines = wrapLines(body, innerW)
			}
			m.bodyWrappedWidth = innerW
		}

		totalLines := len(m.bodyWrappedLines)
		maxOffset := totalLines - maxBodyLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.bodyScrollOffset > maxOffset {
			m.bodyScrollOffset = maxOffset
		}

		end := m.bodyScrollOffset + maxBodyLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(renderBodyLines(m.bodyWrappedLines, m.bodyScrollOffset, end,
			m.visualMode, m.visualStart, m.visualEnd))

		// Scroll indicator
		if totalLines > maxBodyLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%  │  z/esc: exit full-screen", m.bodyScrollOffset+1, totalLines, pct)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		} else {
			sb.WriteString("\n" + dimStyle.Render(" z/esc: exit full-screen"))
		}
	}

	// Quick reply picker overlay at the bottom of full-screen view.
	if m.quickReplyOpen {
		sb.WriteString(m.renderQuickReplyPicker(innerW))
	}

	return lipgloss.NewStyle().
		Width(m.windowWidth).
		Height(m.windowHeight).
		Render(sb.String())
}

// emptyStateView returns a placeholder string the same height as the content
// area, with msg centred vertically. Used when a table has no rows to display.
func (m *Model) emptyStateView(msg string) string {
	h := m.windowHeight - 6
	if h < 5 {
		h = 5
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	mid := h / 2
	var sb strings.Builder
	for i := 0; i < mid; i++ {
		sb.WriteString("\n")
	}
	sb.WriteString(dim.Render(msg))
	return sb.String()
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
// Retries up to 2 times on error to handle transient ProtonMail Bridge failures.
func (m *Model) loadEmailBodyCmd(folder string, uid uint32) tea.Cmd {
	return func() tea.Msg {
		var (
			body *models.EmailBody
			err  error
		)
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			body, err = m.backend.FetchEmailBody(folder, uid)
			if err == nil {
				break
			}
			logger.Warn("FetchEmailBody attempt %d failed: %v", attempt+1, err)
		}
		return EmailBodyMsg{Body: body, Err: err}
	}
}

// fetchCleanupBodyCmd returns a tea.Cmd that fetches an email body for the Cleanup tab preview.
func fetchCleanupBodyCmd(b backend.Backend, email *models.EmailData) tea.Cmd {
	return func() tea.Msg {
		body, err := b.FetchEmailBody(email.Folder, email.UID)
		return CleanupEmailBodyMsg{Body: body, Err: err}
	}
}

// buildCannedReplies returns 5 pre-written reply templates for the given sender.
func buildCannedReplies(senderName string) []string {
	firstName := senderName
	// Extract display name from "Name <email>" format
	if idx := strings.Index(senderName, "<"); idx > 0 {
		firstName = strings.TrimSpace(senderName[:idx])
	}
	// Use first word only
	if parts := strings.Fields(firstName); len(parts) > 0 {
		firstName = parts[0]
	}
	// Strip surrounding quotes if any
	firstName = strings.Trim(firstName, `"'`)
	if firstName == "" {
		firstName = "there"
	}
	return []string{
		"No thanks.",
		"Thank you for reaching out.",
		fmt.Sprintf("Thank you, %s.", firstName),
		"Copy that.",
		"I'll get back to you.",
	}
}

// generateQuickRepliesCmd returns a tea.Cmd that asks Ollama for AI reply suggestions.
func generateQuickRepliesCmd(classifier ai.AIClient, sender, subject, bodyPreview string) tea.Cmd {
	return func() tea.Msg {
		replies, err := classifier.GenerateQuickReplies(sender, subject, bodyPreview)
		return QuickRepliesMsg{Replies: replies, Err: err}
	}
}

// openQuickReply pre-fills the Compose tab with the selected reply template and switches to it.
func (m *Model) openQuickReply(template string) (tea.Model, tea.Cmd) {
	m.quickReplyOpen = false
	if m.selectedTimelineEmail == nil {
		return m, nil
	}
	email := m.selectedTimelineEmail
	m.activeTab = tabCompose
	m.composeTo.SetValue(email.Sender)
	subject := email.Subject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	m.composeSubject.SetValue(subject)
	m.composeBody.SetValue(template)
	m.composeField = 2
	m.composeTo.Blur()
	m.composeSubject.Blur()
	m.composeBody.Focus()
	return m, nil
}

// renderQuickReplyPicker renders the quick reply picker overlay appended to the preview panel.
func (m *Model) renderQuickReplyPicker(width int) string {
	if width < 20 {
		width = 20
	}
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(width)
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(width)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	aiLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	var sb strings.Builder
	sb.WriteString(strings.Repeat("─", width) + "\n")

	header := "Quick Reply — ↑↓ navigate  Enter: compose  Esc: close"
	if !m.quickRepliesReady {
		header = "Quick Reply — generating suggestions…"
	}
	sb.WriteString(headerStyle.Render(truncate(header, width)) + "\n")

	cannedCount := 5 // first 5 are always canned
	for i, reply := range m.quickReplies {
		num := fmt.Sprintf("%d. ", i+1)
		var label string
		if i >= cannedCount {
			label = num + aiLabelStyle.Render("[AI] ") + truncate(reply, width-len(num)-5)
		} else {
			label = num + truncate(reply, width-len(num))
		}
		if i == m.quickReplyIdx {
			sb.WriteString(selectedStyle.Render(label) + "\n")
		} else {
			sb.WriteString(normalStyle.Render(label) + "\n")
		}
	}

	if len(m.quickReplies) == 0 && m.quickRepliesReady {
		sb.WriteString(dimStyle.Render("  No suggestions available") + "\n")
	}

	return sb.String()
}

// renderCleanupPreview renders the right-hand email body preview panel for the Cleanup tab.
func (m *Model) renderCleanupPreview() string {
	w := m.cleanupPreviewWidth
	if w <= 0 {
		w = 40
	}
	innerW := w - 4 // left border + padding

	var sb strings.Builder

	borderColor := "238"
	headerColor := "245"

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(headerColor))
	dimStyle := headerStyle

	if m.cleanupPreviewEmail != nil {
		email := m.cleanupPreviewEmail
		sb.WriteString(headerStyle.Render("From: "+truncate(email.Sender, innerW-6)) + "\n")
		sb.WriteString(headerStyle.Render("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04")) + "\n")
		sb.WriteString(headerStyle.Render("Subj: "+truncate(email.Subject, innerW-6)) + "\n")
		sb.WriteString(strings.Repeat("─", innerW) + "\n")
	}

	panelHeight := m.windowHeight - 6
	if panelHeight < 5 {
		panelHeight = 5
	}
	// Header block is 4 rows; reserve 1 for scroll indicator → maxBodyLines = panelHeight - 4 - 1
	maxBodyLines := panelHeight - 5
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}

	if m.cleanupBodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.cleanupEmailBody != nil {
		body := stripInvisibleChars(m.cleanupEmailBody.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		if m.cleanupBodyWrappedLines == nil || m.cleanupBodyWrappedWidth != innerW {
			if m.cleanupEmailBody.IsFromHTML {
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						m.cleanupBodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.cleanupBodyWrappedLines = wrapLines(body, innerW)
					}
				} else {
					m.cleanupBodyWrappedLines = wrapLines(body, innerW)
				}
			} else {
				m.cleanupBodyWrappedLines = wrapLines(body, innerW)
			}
			m.cleanupBodyWrappedWidth = innerW
		}

		totalLines := len(m.cleanupBodyWrappedLines)
		maxOffset := totalLines - maxBodyLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.cleanupBodyScrollOffset > maxOffset {
			m.cleanupBodyScrollOffset = maxOffset
		}

		end := m.cleanupBodyScrollOffset + maxBodyLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(strings.Join(m.cleanupBodyWrappedLines[m.cleanupBodyScrollOffset:end], "\n"))

		if totalLines > maxBodyLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.cleanupBodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%  │  esc: close", m.cleanupBodyScrollOffset+1, totalLines, pct)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		} else {
			sb.WriteString("\n" + dimStyle.Render(" esc: close preview"))
		}
	} else {
		sb.WriteString(dimStyle.Render("(No content)"))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w).
		Height(panelHeight).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingLeft(1)

	return panelStyle.Render(sb.String())
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
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("160")).
			Width(w).
			Padding(0, 1).
			Render(line)
	}
	// Unsubscribe confirmation prompt overrides everything
	if m.pendingUnsubscribe {
		w := m.windowWidth
		if w <= 0 {
			w = 80
		}
		line := fmt.Sprintf("  %s  [y] confirm  [n/Esc] cancel", m.pendingUnsubscribeDesc)
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("202")).
			Width(w).
			Padding(0, 1).
			Render(line)
	}

	// 3-way unsubscribe choice prompt overrides normal status bar
	if m.unsubConfirmMode {
		w := m.windowWidth
		if w <= 0 {
			w = 80
		}
		line := fmt.Sprintf("  Unsubscribe from %s:  [h] Hard unsubscribe  │  [s] Soft unsubscribe (auto-move)  │  [Esc] Cancel", m.unsubConfirmSender)
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("202")).
			Width(w).
			Padding(0, 1).
			Render(line)
	}

	// Chat filter indicator
	var filterPrefix string
	if m.chatFilterMode && m.activeTab == tabTimeline {
		filterLabel := m.chatFilterLabel
		if filterLabel == "" {
			filterLabel = "filtered"
		}
		filterPrefix = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
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

	// Demo mode indicator
	if m.demoMode {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true).Render("[DEMO]"))
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
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("237")).
		Width(w).
		Padding(0, 1).
		Render(line)
}

// renderKeyHints renders the context-sensitive key hint line
func (m *Model) renderKeyHints() string {
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
	} else if m.focusedPanel == panelChat && m.showChat {
		hints = "enter: send  │  esc/tab: close chat  │  q: quit"
	} else if m.showLogs {
		hints = "l: close logs  │  ↑/k ↓/j: scroll  │  q: quit"
	} else if m.activeTab == tabCompose {
		hints = "1/2/3/4: tabs  │  tab: next field  │  ctrl+s: send  │  ctrl+p: preview  │  ctrl+a: attach  │  r: refresh  │  c: chat  │  q: quit"
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
		if m.focusedPanel == panelPreview {
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
			hints = "tab/shift+tab: panels  │  ↑/k ↓/j: navigate  │  enter: open  │  esc: close  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  q: quit"
		} else {
			hints = "1/2/3/4: tabs  │  ↑/k ↓/j: navigate  │  enter: open  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  /: search  │  a: AI tag  │  f: sidebar  │  q: quit"
		}
	} else {
		switch m.focusedPanel {
		case panelSidebar:
			hints = "1/2/3/4: tabs  │  tab: next panel  │  ↑/k ↓/j: nav  │  space: expand  │  enter: open  │  r: refresh  │  a: AI tag  │  f: hide  │  c: chat  │  q: quit"
		case panelDetails:
			if m.showCleanupPreview {
				hints = "↑/k ↓/j: scroll preview  │  enter: scroll down  │  esc: close preview  │  tab: next panel  │  D: delete  │  q: quit"
			} else {
				hints = "1/2/3/4: tabs  │  tab: next panel  │  ↑/k ↓/j: nav  │  enter: preview  │  space: select  │  D: delete  │  e: archive  │  r: refresh  │  a: AI tag  │  c: chat  │  l: logs  │  q: quit"
			}
		default: // panelSummary
			hints = "1/2/3/4: tabs  │  tab: panel  │  enter: details  │  space: select  │  D: delete  │  e: archive  │  d: domain  │  r: refresh  │  a: AI tag  │  W: create rule  │  C: auto-cleanup rules  │  P: new prompt  │  f: sidebar  │  c: chat  │  q: quit"
		}
	}
	// Override hints when quick reply picker is open.
	if m.quickReplyOpen {
		hints = "↑/k ↓/j: navigate replies  │  enter: compose  │  1-8: select  │  esc: close picker  │  q: quit"
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render(hints)
}

// setFocusedPanel updates focus state and table styles for the given panel
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
	if m.activeTab == tabTimeline {
		if m.showSidebar {
			panels = append(panels, panelSidebar)
		}
		panels = append(panels, panelTimeline)
		if m.selectedTimelineEmail != nil {
			panels = append(panels, panelPreview)
		}
		if m.showChat {
			panels = append(panels, panelChat)
		}
	} else {
		// Cleanup / other tabs
		if m.showSidebar {
			panels = append(panels, panelSidebar)
		}
		panels = append(panels, panelSummary, panelDetails)
		if m.showChat {
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
	m.focusedPanel = panelSummary
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

	// Limit rendered rows to tableHeight to prevent overflow at small terminal heights
	maxRows := m.windowHeight - 6
	if maxRows < 5 {
		maxRows = 5
	}
	startIdx := 0
	if len(items) > maxRows {
		startIdx = m.sidebarCursor - maxRows + 1
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx+maxRows > len(items) {
			startIdx = len(items) - maxRows
		}
	}

	for i, item := range items {
		if i < startIdx || i >= startIdx+maxRows {
			continue
		}
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

		prefixLen := len([]rune(indent)) + 2 // icon is 2 display cells
		available := sidebarContentWidth - prefixLen - len([]rune(countSuffix))
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
	return strings.TrimSuffix(sb.String(), "\n")
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
	// Attachment path input intercepts all keys while active
	if m.attachmentInputActive {
		switch msg.String() {
		case "enter":
			path := expandTilde(m.attachmentPathInput.Value())
			m.attachmentInputActive = false
			m.attachmentPathInput.SetValue("")
			m.attachmentPathInput.Blur()
			return m, addAttachmentCmd(path)
		case "esc":
			m.attachmentInputActive = false
			m.attachmentPathInput.SetValue("")
			m.attachmentPathInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.attachmentPathInput, cmd = m.attachmentPathInput.Update(msg)
			return m, cmd
		}
	}

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
	case "ctrl+a":
		m.attachmentInputActive = true
		m.attachmentPathInput.SetValue("")
		m.attachmentPathInput.Focus()
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

// sendCompose sends the composed message via SMTP as multipart/alternative
// (HTML + plain-text fallback). The body textarea is treated as Markdown.
// Any staged attachments are sent as multipart/mixed parts.
func (m *Model) sendCompose() tea.Cmd {
	from := m.fromAddress
	to := m.composeTo.Value()
	subject := m.composeSubject.Value()
	markdownBody := m.composeBody.Value()
	attachments := m.composeAttachments // snapshot; cleared on success in Update()
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
		htmlBody, plainText := appsmtp.MarkdownToHTMLAndPlain(markdownBody)
		err := m.mailer.SendWithAttachments(from, to, subject, plainText, htmlBody, attachments)
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

	// Attachment path input prompt
	if m.attachmentInputActive {
		promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
		sb.WriteString(promptStyle.Render("Attach file: ") + m.attachmentPathInput.View() + "\n")
	}

	// Staged attachments list
	for _, att := range m.composeAttachments {
		sizeStr := fmt.Sprintf("%.1f KB", float64(att.Size)/1024)
		if att.Size >= 1024*1024 {
			sizeStr = fmt.Sprintf("%.1f MB", float64(att.Size)/(1024*1024))
		}
		warnIcon := ""
		if att.Size > 10*1024*1024 {
			warnIcon = " ⚠ (>10 MB)"
		}
		label := fmt.Sprintf("  [attach] %s  (%s)%s", att.Filename, sizeStr, warnIcon)
		attachColor := "111"
		if att.Data == nil {
			attachColor = "196"
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(attachColor)).Render(label) + "\n")
	}

	// Status message
	if m.composeStatus != "" {
		color := "86"
		if strings.HasPrefix(m.composeStatus, "Error") || strings.HasPrefix(m.composeStatus, "Send failed") || strings.HasPrefix(m.composeStatus, "Attach error") {
			color = "196"
		} else if strings.HasPrefix(m.composeStatus, "Warning") {
			color = "214"
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(m.composeStatus) + "\n")
	}

	return sb.String()
}

const maxToolRounds = 5

// submitChat sends the current chat input to the AI backend with email context,
// using a multi-turn tool-calling loop when supported.
func (m *Model) submitChat() tea.Cmd {
	question := strings.TrimSpace(m.chatInput.Value())
	if question == "" {
		return nil
	}
	m.chatInput.SetValue("")
	m.chatWaiting = true

	currentFolder := m.currentFolder // snapshot before goroutine

	// Append user message to history
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{
		Role:    "user",
		Content: question,
	})
	m.chatWrappedLines = nil // invalidate wrap cache

	// Build system prompt with email context
	var ctx strings.Builder
	ctx.WriteString(fmt.Sprintf("You are an email assistant. The user is viewing folder: %s.\n", currentFolder))
	if st, ok := m.folderStatus[currentFolder]; ok {
		ctx.WriteString(fmt.Sprintf("Folder has %d total emails, %d unread.\n", st.Total, st.Unseen))
	}
	if len(m.timelineEmails) > 0 {
		ctx.WriteString("Recent emails (newest first):\n")
		limit := 20
		if len(m.timelineEmails) < limit {
			limit = len(m.timelineEmails)
		}
		for _, e := range m.timelineEmails[:limit] {
			ctx.WriteString(fmt.Sprintf("  - [%s] From: %s | Subject: %s | Date: %s\n",
				e.MessageID, e.Sender, e.Subject, e.Date.Format("2006-01-02")))
		}
	}
	ctx.WriteString("\nIf the user asks to show, filter, or find specific emails, include a <filter> block at the end of your response:\n")
	ctx.WriteString("<filter>{\"ids\": [\"<message-id-1>\", \"<message-id-2>\"], \"label\": \"short description\"}</filter>\n")
	ctx.WriteString("Only include a <filter> block when the user is explicitly asking to filter or navigate to specific emails.\n")

	systemMsg := ai.ChatMessage{Role: "system", Content: ctx.String()}
	messages := append([]ai.ChatMessage{systemMsg}, m.chatMessages...)

	classifier := m.classifier
	tools, dispatch := m.chatToolRegistryWithFolder(currentFolder)

	return func() tea.Msg {
		if classifier == nil {
			return ChatResponseMsg{Err: fmt.Errorf("AI not configured")}
		}

		for round := 0; round < maxToolRounds; round++ {
			response, calls, err := classifier.ChatWithTools(messages, tools)
			if err != nil {
				if errors.Is(err, ai.ErrToolsNotSupported) {
					// Fall back to plain Chat()
					reply, err2 := classifier.Chat(messages)
					if err2 != nil {
						return ChatResponseMsg{Err: err2}
					}
					return ChatResponseMsg{Content: reply}
				}
				return ChatResponseMsg{Err: err}
			}

			if len(calls) == 0 {
				// Final text response
				return ChatResponseMsg{Content: response}
			}

			// Append assistant turn with all tool calls (once per round)
			messages = append(messages, ai.ChatMessage{
				Role:      "assistant",
				ToolCalls: calls,
			})

			// Execute tool calls and append one result message per call
			for _, call := range calls {
				result, dispErr := dispatch(call.Name, call.Arguments)
				if dispErr != nil {
					result = "Error: " + dispErr.Error()
				}
				messages = append(messages, ai.ChatMessage{
					Role:       "tool",
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Content:    result,
				})
			}
		}

		return ChatResponseMsg{Content: "Max tool rounds reached without a final response."}
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

	// Rebuild wrap cache if stale
	if m.chatWrappedLines == nil || m.chatWrappedWidth != w {
		m.chatWrappedLines = make([][]string, len(m.chatMessages))
		for i, msg := range m.chatMessages {
			prefix := "AI: "
			if msg.Role == "user" {
				prefix = "You: "
			}
			m.chatWrappedLines[i] = wrapText(prefix+msg.Content, w)
		}
		m.chatWrappedWidth = w
	}

	// Collect rendered message lines (newest-last)
	var msgLines []string
	for i, msg := range m.chatMessages {
		style := aiStyle
		if msg.Role == "user" {
			style = userStyle
		}
		for _, line := range m.chatWrappedLines[i] {
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

// wrapLines splits text on newlines first, then word-wraps each paragraph to
// fit within width runes. Consecutive blank lines are collapsed to one blank
// line, so over-spaced HTML-converted bodies look reasonable.
// expandTilde replaces a leading "~/" with the user's home directory.
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// saveAttachmentCmd returns a tea.Cmd that writes attachment data to destPath.
func saveAttachmentCmd(b backend.Backend, att *models.Attachment, destPath string) tea.Cmd {
	return func() tea.Msg {
		if err := b.SaveAttachment(att, destPath); err != nil {
			return AttachmentSavedMsg{Err: err}
		}
		return AttachmentSavedMsg{Filename: att.Filename, Path: destPath}
	}
}

// addAttachmentCmd reads a file from path and returns an AttachmentAddedMsg.
func addAttachmentCmd(path string) tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(path)
		if err != nil {
			return AttachmentAddedMsg{Err: fmt.Errorf("cannot read %s: %w", path, err)}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return AttachmentAddedMsg{Err: err}
		}
		return AttachmentAddedMsg{
			Attachment: models.ComposeAttachment{
				Path:     path,
				Filename: filepath.Base(path),
				Size:     info.Size(),
				Data:     data,
			},
		}
	}
}

// unsubscribeCmd attempts to unsubscribe via List-Unsubscribe header.
// RFC 8058: if List-Unsubscribe-Post is "List-Unsubscribe=One-Click" and an https URL exists,
// it does an HTTP POST. Otherwise it copies the URL or mailto address to the clipboard.
func unsubscribeCmd(body *models.EmailBody) tea.Cmd {
	return func() tea.Msg {
		raw := body.ListUnsubscribe
		if raw == "" {
			return UnsubscribeResultMsg{Err: fmt.Errorf("no List-Unsubscribe header")}
		}
		// Parse angle-bracket-delimited URIs: <https://...>, <mailto:...>
		var httpsURL, mailtoAddr string
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if len(part) >= 2 && part[0] == '<' && part[len(part)-1] == '>' {
				part = part[1 : len(part)-1]
			}
			if strings.HasPrefix(part, "https://") && httpsURL == "" {
				httpsURL = part
			} else if strings.HasPrefix(part, "mailto:") && mailtoAddr == "" {
				mailtoAddr = part
			}
		}
		// One-click POST (RFC 8058)
		if body.ListUnsubscribePost == "List-Unsubscribe=One-Click" && httpsURL != "" {
			resp, err := http.Post(httpsURL, "application/x-www-form-urlencoded",
				strings.NewReader("List-Unsubscribe=One-Click"))
			if err != nil {
				return UnsubscribeResultMsg{Err: err}
			}
			resp.Body.Close()
			return UnsubscribeResultMsg{Method: "one-click", URL: httpsURL}
		}
		// Copy HTTPS URL to clipboard
		if httpsURL != "" {
			cmd := exec.Command("pbcopy")
			if runtime.GOOS == "linux" {
				if os.Getenv("WAYLAND_DISPLAY") != "" {
					cmd = exec.Command("wl-copy")
				} else {
					cmd = exec.Command("xclip", "-sel", "clip")
				}
			}
			cmd.Stdin = strings.NewReader(httpsURL)
			_ = cmd.Run()
			return UnsubscribeResultMsg{Method: "url-copied", URL: httpsURL}
		}
		// Copy mailto address to clipboard
		if mailtoAddr != "" {
			cmd := exec.Command("pbcopy")
			if runtime.GOOS == "linux" {
				if os.Getenv("WAYLAND_DISPLAY") != "" {
					cmd = exec.Command("wl-copy")
				} else {
					cmd = exec.Command("xclip", "-sel", "clip")
				}
			}
			cmd.Stdin = strings.NewReader(mailtoAddr)
			_ = cmd.Run()
			return UnsubscribeResultMsg{Method: "mailto-copied", URL: mailtoAddr}
		}
		return UnsubscribeResultMsg{Err: fmt.Errorf("no usable unsubscribe URI found")}
	}
}

// createSoftUnsubscribeRuleCmd creates a rule that auto-moves emails from sender
// to "Disabled Subscriptions", providing a non-destructive alternative to hard deletion.
func createSoftUnsubscribeRuleCmd(b backend.Backend, sender string) tea.Cmd {
	return func() tea.Msg {
		rule := &models.Rule{
			Name:         "Soft unsub: " + sender,
			Enabled:      true,
			TriggerType:  models.TriggerSender,
			TriggerValue: sender,
			Actions: []models.RuleAction{{
				Type:       models.ActionMove,
				DestFolder: "Disabled Subscriptions",
			}},
		}
		err := b.SaveRule(rule)
		return SoftUnsubResultMsg{Sender: sender, Err: err}
	}
}

// markReadCmd fires and forgets — marks the email as read on IMAP and in cache.
func markReadCmd(b backend.Backend, messageID, folder string) tea.Cmd {
	return func() tea.Msg {
		if err := b.MarkRead(messageID, folder); err != nil {
			logger.Warn("markReadCmd failed for %s: %v", messageID, err)
		}
		return nil
	}
}

// cacheUnsubscribeHeadersCmd stores List-Unsubscribe headers in the cache.
func cacheUnsubscribeHeadersCmd(b backend.Backend, messageID, listUnsub, listUnsubPost string) tea.Cmd {
	return func() tea.Msg {
		if err := b.UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost); err != nil {
			logger.Warn("cacheUnsubscribeHeadersCmd failed for %s: %v", messageID, err)
		}
		return nil
	}
}

// copyToClipboard returns a tea.Cmd that writes text to the system clipboard.
// Tries pbcopy (macOS), wl-copy (Wayland), then xclip (X11). Failures are
// logged and silently dropped so the TUI keeps running.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		default:
			if os.Getenv("WAYLAND_DISPLAY") != "" {
				cmd = exec.Command("wl-copy")
			} else {
				cmd = exec.Command("xclip", "-sel", "clip")
			}
		}
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			logger.Warn("clipboard copy failed: %v", err)
		}
		return nil
	}
}

func wrapLines(text string, width int) []string {
	// Normalize CRLF and strip trailing whitespace
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n\t ")

	var result []string
	consecutiveBlanks := 0
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimRight(para, " \t")
		if para == "" {
			consecutiveBlanks++
			if consecutiveBlanks <= 1 {
				result = append(result, "")
			}
			continue
		}
		consecutiveBlanks = 0
		result = append(result, wrapText(para, width)...)
	}
	return result
}

// wrapText wraps text to fit within width runes.
// stripInvisibleChars removes zero-width and formatting Unicode characters that
// appear as visible noise in terminal output (U+200B, U+FEFF, U+034F, etc.).
// Regular whitespace (space, tab, newline) is preserved.
func stripInvisibleChars(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == ' ':
			b.WriteRune(r) // preserve normal whitespace
		case unicode.Is(unicode.Cf, r): // format characters (zero-width, BOM, etc.)
			// skip
		case r == '\u034f': // COMBINING GRAPHEME JOINER — used as invisible spacer in HTML email
			// skip: xterm.js and some terminal renderers give it nonzero width,
			// causing lines of "͏ ͏ ͏ ..." to overflow the preview panel.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

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

// --- Deletion/archive confirmation description builders ---

// buildDeleteDesc builds a human-readable description for the deletion confirmation prompt.
func (m *Model) buildDeleteDesc() string {
	if m.activeTab == tabTimeline {
		cursor := m.timelineTable.Cursor()
		if cursor < len(m.threadRowMap) {
			ref := m.threadRowMap[cursor]
			var email *models.EmailData
			if ref.kind == rowKindThread {
				email = ref.group.emails[0]
			} else {
				email = ref.group.emails[ref.emailIdx]
			}
			if email != nil {
				subj := email.Subject
				if len(subj) > 50 {
					subj = subj[:47] + "..."
				}
				return fmt.Sprintf("Delete \"%s\"?", subj)
			}
		}
		return ""
	}
	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			return fmt.Sprintf("Delete %d selected message(s)?", len(m.selectedMessages))
		}
		cursor := m.detailsTable.Cursor()
		if cursor < len(m.detailsEmails) {
			return fmt.Sprintf("Delete message from %s?", m.detailsEmails[cursor].Sender)
		}
		return ""
	}
	if len(m.selectedRows) > 0 {
		return fmt.Sprintf("Delete emails from %d selected sender(s)?", len(m.selectedRows))
	}
	cursor := m.summaryTable.Cursor()
	if sender, ok := m.rowToSender[cursor]; ok && sender != "" {
		if m.groupByDomain {
			return fmt.Sprintf("Delete all emails from domain %s?", sender)
		}
		return fmt.Sprintf("Delete all emails from %s?", sender)
	}
	return ""
}

// buildArchiveDesc builds a human-readable description for the archive confirmation prompt.
func (m *Model) buildArchiveDesc() string {
	if m.activeTab == tabTimeline {
		cursor := m.timelineTable.Cursor()
		if cursor < len(m.threadRowMap) {
			ref := m.threadRowMap[cursor]
			var email *models.EmailData
			if ref.kind == rowKindThread {
				email = ref.group.emails[0]
			} else {
				email = ref.group.emails[ref.emailIdx]
			}
			if email != nil {
				subj := email.Subject
				if len(subj) > 50 {
					subj = subj[:47] + "..."
				}
				return fmt.Sprintf("Archive \"%s\"?", subj)
			}
		}
		return ""
	}
	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			return fmt.Sprintf("Archive %d selected message(s)?", len(m.selectedMessages))
		}
		cursor := m.detailsTable.Cursor()
		if cursor < len(m.detailsEmails) {
			return fmt.Sprintf("Archive message from %s?", m.detailsEmails[cursor].Sender)
		}
		return ""
	}
	if len(m.selectedRows) > 0 {
		return fmt.Sprintf("Archive emails from %d selected sender(s)?", len(m.selectedRows))
	}
	cursor := m.summaryTable.Cursor()
	if sender, ok := m.rowToSender[cursor]; ok && sender != "" {
		return fmt.Sprintf("Archive all emails from %s?", sender)
	}
	return ""
}

// --- Search helpers ---

// performSearch runs a local or semantic search and returns the result as a tea.Cmd.
func (m *Model) performSearch(query string) tea.Cmd {
	if query == "" {
		return func() tea.Msg { return SearchResultMsg{Query: ""} }
	}
	folder := m.currentFolder
	bodyMode := strings.HasPrefix(query, "/b ")
	crossFolder := strings.HasPrefix(query, "/*")
	semanticMode := strings.HasPrefix(query, "?")

	actualQuery := query
	switch {
	case bodyMode:
		actualQuery = strings.TrimPrefix(query, "/b ")
	case crossFolder:
		actualQuery = strings.TrimPrefix(strings.TrimPrefix(query, "/* "), "/*")
	case semanticMode:
		actualQuery = strings.TrimPrefix(query, "?")
	}
	actualQuery = strings.TrimSpace(actualQuery)
	if actualQuery == "" {
		return func() tea.Msg { return SearchResultMsg{Query: ""} }
	}

	classifier := m.classifier
	backend := m.backend
	return func() tea.Msg {
		var emails []*models.EmailData
		var scores map[string]float64
		var err error
		source := "local"
		switch {
		case semanticMode:
			source = "semantic"
			if classifier == nil {
				logger.Warn("Semantic search requires Ollama classifier — not configured")
				return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: source}
			}
			queryText := ai.BuildQueryText(actualQuery)
			vec, embedErr := classifier.Embed(queryText)
			if embedErr != nil {
				logger.Warn("Semantic search embed error: %v", embedErr)
				return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: source}
			}
			results, searchErr := backend.SearchSemanticChunked(folder, vec, 20, 0.3)
			if searchErr != nil {
				logger.Warn("semantic search: %v", searchErr)
				if strings.Contains(searchErr.Error(), "not supported") {
					return SearchResultMsg{Emails: nil, Err: fmt.Errorf("semantic search requires local backend")}
				}
				return SearchResultMsg{Emails: nil, Err: searchErr}
			}
			scores = make(map[string]float64, len(results))
			for _, r := range results {
				emails = append(emails, r.Email)
				scores[r.Email.MessageID] = r.Score
			}
		case bodyMode:
			emails, err = backend.SearchEmails(folder, actualQuery, true)
			source = "fts"
		case crossFolder:
			emails, err = backend.SearchEmailsCrossFolder(actualQuery)
			source = "cross"
		default:
			emails, err = backend.SearchEmails(folder, actualQuery, false)
		}
		if err != nil {
			logger.Warn("Search error: %v", err)
			return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: source}
		}
		if emails == nil {
			emails = []*models.EmailData{}
		}
		return SearchResultMsg{Emails: emails, Scores: scores, Query: query, Source: source}
	}
}

// performIMAPSearch performs a server-side IMAP search as a tea.Cmd.
func (m *Model) performIMAPSearch(query string) tea.Cmd {
	if query == "" {
		return nil
	}
	folder := m.currentFolder
	return func() tea.Msg {
		emails, err := m.backend.SearchEmailsIMAP(folder, query)
		if err != nil {
			logger.Warn("IMAP search error: %v", err)
			return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: "imap"}
		}
		return SearchResultMsg{Emails: emails, Query: query, Source: "imap"}
	}
}

// saveCurrentSearch persists the current search query with an auto-generated name.
func (m *Model) saveCurrentSearch(query string) tea.Cmd {
	folder := m.currentFolder
	name := query
	if len(name) > 30 {
		name = name[:27] + "..."
	}
	return func() tea.Msg {
		if err := m.backend.SaveSearch(name, query, folder); err != nil {
			logger.Warn("Failed to save search: %v", err)
		}
		return nil
	}
}

// updateTimelineTableFromSearch replaces the displayed emails with search results.
// Called from the SearchResultMsg handler when searchMode is active.
func (m *Model) updateTimelineTableFromSearch(emails []*models.EmailData) {
	if emails == nil {
		// Restore from cache
		if m.timelineEmailsCache != nil {
			m.timelineEmails = m.timelineEmailsCache
			m.timelineEmailsCache = nil
		}
	} else {
		m.timelineEmails = emails
	}
	m.updateTimelineTable()
}

// --- Background sync helpers ---

// listenForNewEmails returns a Cmd that blocks on the backend's new-emails channel.
func (m *Model) listenForNewEmails() tea.Cmd {
	ch := m.backend.NewEmailsCh()
	return func() tea.Msg {
		notif := <-ch
		return NewEmailsMsg{Emails: notif.Emails, Folder: notif.Folder}
	}
}

// listenForExpunged is a no-op stub (IMAP expunge notifications not yet implemented).
func (m *Model) listenForExpunged() tea.Cmd {
	return nil
}

// tickSyncCountdown drives the sync countdown ticker.
func (m *Model) tickSyncCountdown() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		return SyncTickMsg{}
	})
}

// startPolling starts background polling and the sync countdown timer.
func (m *Model) startPolling(interval int) tea.Cmd {
	m.syncStatusMode = "polling"
	m.syncCountdown = interval
	m.backend.StartPolling(m.currentFolder, interval)
	return tea.Batch(m.listenForNewEmails(), m.tickSyncCountdown())
}

// startSync tries IDLE first (if enabled in config), falling back to polling.
func (m *Model) startSync(folder string) tea.Cmd {
	// Stop any running sync before starting a new one.
	m.backend.StopIDLE()
	m.backend.StopPolling()

	if m.cfg.Sync.Idle {
		if err := m.backend.StartIDLE(folder); err == nil {
			m.syncStatusMode = "idle"
			return m.listenForNewEmails()
		} else if !errors.Is(err, imapClient.ErrIDLENotSupported) {
			logger.Warn("IDLE failed, falling back to polling: %v", err)
		}
	}
	return m.startPolling(m.cfg.Sync.Interval)
}

// --- Embedding helpers ---

// embedChunksForEmail strips, chunks, and embeds an email body for semantic search.
// Uses nomic-embed-text's search_document: prefix for asymmetric retrieval.
// Returns nil if classifier is nil, body is empty, or all embeddings fail.
func embedChunksForEmail(email *models.EmailData, bodyText string, classifier ai.AIClient) []models.EmbeddingChunk {
	if classifier == nil || email == nil || bodyText == "" {
		return nil
	}
	cleaned := ai.StripQuotedText(bodyText)
	if cleaned == "" {
		cleaned = email.Subject // fallback: at least embed the subject
	}
	rawChunks := ai.ChunkText(cleaned, 800, 200, 10)
	if len(rawChunks) == 0 {
		return nil
	}
	date := email.Date.Format("2006-01-02")
	var result []models.EmbeddingChunk
	for i, chunk := range rawChunks {
		doc := ai.BuildDocumentChunk(email.Sender, date, email.Subject, chunk)
		vec, err := classifier.Embed(doc)
		if err != nil {
			logger.Debug("embed chunk %d for %s: %v", i, email.MessageID, err)
			continue
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(doc)))
		result = append(result, models.EmbeddingChunk{
			MessageID:   email.MessageID,
			ChunkIndex:  i,
			Embedding:   vec,
			ContentHash: hash,
		})
	}
	return result
}

// runEmbeddingBatch processes one batch of emails for semantic search embedding.
// Pass 1 embeds emails with cached body text.
// Pass 2 lazily fetches bodies for emails not yet cached (rate-limited to 5 per call).
func (m *Model) runEmbeddingBatch() tea.Cmd {
	folder := m.currentFolder
	return func() tea.Msg {
		// Pass 1: embed emails that already have body_text in cache
		if ids, err := m.backend.GetUnembeddedIDsWithBody(folder); err == nil && len(ids) > 0 {
			if len(ids) > 20 {
				ids = ids[:20]
			}
			for _, id := range ids {
				email, err := m.backend.GetEmailByID(id)
				if err != nil || email == nil {
					continue
				}
				bodyText, err := m.backend.GetBodyText(id)
				if err != nil || bodyText == "" {
					continue
				}
				chunks := embedChunksForEmail(email, bodyText, m.classifier)
				if len(chunks) > 0 {
					if err := m.backend.StoreEmbeddingChunks(id, chunks); err != nil {
						logger.Warn("StoreEmbeddingChunks %s: %v", id, err)
					}
				}
			}
		}

		// Pass 2: lazily fetch bodies for emails with neither body_text nor chunks
		if uncached, err := m.backend.GetUncachedBodyIDs(folder, 5); err == nil {
			for _, id := range uncached {
				email, err := m.backend.GetEmailByID(id)
				if err != nil || email == nil {
					continue
				}
				body, err := m.backend.FetchAndCacheBody(id)
				if err != nil || body == nil || body.TextPlain == "" {
					continue
				}
				chunks := embedChunksForEmail(email, body.TextPlain, m.classifier)
				if len(chunks) > 0 {
					if err := m.backend.StoreEmbeddingChunks(id, chunks); err != nil {
						logger.Warn("StoreEmbeddingChunks (lazy) %s: %v", id, err)
					}
				}
			}
		}

		done, total, _ := m.backend.GetEmbeddingProgress(folder)
		if total > 0 && done >= total {
			return EmbeddingDoneMsg{}
		}
		return EmbeddingProgressMsg{Done: done, Total: total}
	}
}

// runContactEnrichment fetches up to 5 unenriched contacts (email_count >= 3),
// calls Ollama to extract company + topics, stores the results, then embeds each
// enriched contact and stores the embedding. Returns ContactEnrichedMsg.
// This is a no-op (returns Count: 0) when no contacts need enrichment.
func (m *Model) runContactEnrichment() tea.Cmd {
	return func() tea.Msg {
		if m.classifier == nil {
			return ContactEnrichedMsg{Count: 0}
		}
		contacts, err := m.backend.GetContactsToEnrich(3, 5)
		if err != nil {
			logger.Warn("runContactEnrichment: GetContactsToEnrich: %v", err)
			return ContactEnrichedMsg{Count: 0}
		}
		if len(contacts) == 0 {
			return nil
		}

		enriched := 0
		for _, contact := range contacts {
			// Fetch recent email subjects for this contact
			subjects, err := m.backend.GetRecentSubjectsByContact(contact.Email, 10)
			if err != nil {
				logger.Warn("runContactEnrichment: GetRecentSubjectsByContact %s: %v", contact.Email, err)
				continue
			}

			// Ask Ollama to extract company and topics
			company, topics, err := m.classifier.EnrichContact(contact.Email, subjects)
			if err != nil {
				logger.Warn("runContactEnrichment: EnrichContact %s: %v", contact.Email, err)
				continue
			}

			// Store enrichment result (even if company and topics are empty — marks as processed)
			if err := m.backend.UpdateContactEnrichment(contact.Email, company, topics); err != nil {
				logger.Warn("runContactEnrichment: UpdateContactEnrichment %s: %v", contact.Email, err)
				continue
			}

			// Build embedding text and embed
			displayName := contact.DisplayName
			if displayName == "" {
				displayName = contact.Email
			}
			topicsStr := strings.Join(topics, ", ")
			embText := displayName + " " + contact.Email
			if company != "" {
				embText += " from " + company
			}
			if topicsStr != "" {
				embText += ", topics: " + topicsStr
			}

			vec, embErr := m.classifier.Embed(embText)
			if embErr != nil {
				logger.Warn("runContactEnrichment: Embed %s: %v", contact.Email, embErr)
				// Enrichment still counts even if embedding fails
			} else {
				if storeErr := m.backend.UpdateContactEmbedding(contact.Email, vec); storeErr != nil {
					logger.Warn("runContactEnrichment: UpdateContactEmbedding %s: %v", contact.Email, storeErr)
				}
			}

			enriched++
		}

		return ContactEnrichedMsg{Count: enriched}
	}
}

// --- Contacts tab ---

// loadContacts returns a Cmd that fetches all contacts from the backend.
func (m *Model) loadContacts() tea.Cmd {
	return func() tea.Msg {
		contacts, err := m.backend.ListContacts(200, "last_seen")
		if err != nil {
			logger.Warn("loadContacts: %v", err)
			return ContactsLoadedMsg{}
		}
		return ContactsLoadedMsg{Contacts: contacts}
	}
}

// loadContactDetail returns a Cmd that fetches recent emails for the given contact.
func (m *Model) loadContactDetail(contact models.ContactData) tea.Cmd {
	return func() tea.Msg {
		emails, err := m.backend.GetContactEmails(contact.Email, 5)
		if err != nil {
			logger.Warn("loadContactDetail: %v", err)
			return ContactDetailLoadedMsg{}
		}
		return ContactDetailLoadedMsg{Emails: emails}
	}
}

// applyContactSearch filters contactsFiltered based on the current search query and mode.
func (m *Model) applyContactSearch() {
	if m.contactSearch == "" {
		m.contactsFiltered = m.contactsList
		m.contactsIdx = 0
		return
	}
	if m.contactSearchMode == "keyword" {
		q := strings.ToLower(m.contactSearch)
		var out []models.ContactData
		for _, c := range m.contactsList {
			if strings.Contains(strings.ToLower(c.DisplayName), q) ||
				strings.Contains(strings.ToLower(c.Email), q) ||
				strings.Contains(strings.ToLower(c.Company), q) {
				out = append(out, c)
			}
		}
		m.contactsFiltered = out
	} else {
		// semantic: search via backend; fall back to keyword if classifier unavailable
		var out []models.ContactData
		if m.classifier != nil {
			vec, embErr := m.classifier.Embed(m.contactSearch)
			if embErr == nil {
				results, err := m.backend.SearchContactsSemantic(vec, 50, 0.3)
				if err != nil {
					logger.Warn("applyContactSearch semantic: %v", err)
				} else {
					for _, r := range results {
						out = append(out, r.Contact)
					}
				}
			}
		}
		if len(out) == 0 {
			q := strings.ToLower(m.contactSearch)
			for _, c := range m.contactsList {
				if strings.Contains(strings.ToLower(c.DisplayName), q) ||
					strings.Contains(strings.ToLower(c.Email), q) ||
					strings.Contains(strings.ToLower(c.Company), q) {
					out = append(out, c)
				}
			}
		}
		m.contactsFiltered = out
	}
	m.contactsIdx = 0
}

// runSingleContactEnrichment enriches one specific contact by email address.
func (m *Model) runSingleContactEnrichment(contact models.ContactData) tea.Cmd {
	return func() tea.Msg {
		if m.classifier == nil {
			return ContactEnrichedMsg{Count: 0}
		}
		subjects, err := m.backend.GetRecentSubjectsByContact(contact.Email, 10)
		if err != nil {
			logger.Warn("runSingleContactEnrichment: GetRecentSubjectsByContact %s: %v", contact.Email, err)
			return ContactEnrichedMsg{Count: 0}
		}
		company, topics, err := m.classifier.EnrichContact(contact.Email, subjects)
		if err != nil {
			logger.Warn("runSingleContactEnrichment: EnrichContact %s: %v", contact.Email, err)
			return ContactEnrichedMsg{Count: 0}
		}
		if err := m.backend.UpdateContactEnrichment(contact.Email, company, topics); err != nil {
			logger.Warn("runSingleContactEnrichment: UpdateContactEnrichment %s: %v", contact.Email, err)
			return ContactEnrichedMsg{Count: 0}
		}
		return ContactEnrichedMsg{Count: 1}
	}
}

func (m *Model) importAppleContacts() tea.Cmd {
	return func() tea.Msg {
		addrs, err := contacts.ImportFromAppleContacts()
		if err != nil || len(addrs) == 0 {
			return AppleContactsImportedMsg{Count: 0}
		}
		if err := m.backend.UpsertContacts(addrs, "from"); err != nil {
			logger.Warn("Apple Contacts import: %v", err)
			return AppleContactsImportedMsg{Count: 0}
		}
		return AppleContactsImportedMsg{Count: len(addrs)}
	}
}

// handleContactsKey handles key events for the Contacts tab.
func (m *Model) handleContactsKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	key := msg.String()

	// In search mode route printable chars to the search buffer
	if m.contactSearchMode == "keyword" || m.contactSearchMode == "semantic" {
		switch key {
		case "esc":
			m.contactSearchMode = ""
			m.contactSearch = ""
			m.contactsFiltered = m.contactsList
			m.contactsIdx = 0
		case "backspace", "ctrl+h":
			runes := []rune(m.contactSearch)
			if len(runes) > 0 {
				m.contactSearch = string(runes[:len(runes)-1])
			}
			m.applyContactSearch()
		case "enter":
			m.contactSearchMode = "" // confirm; keep results
		default:
			if len(key) == 1 {
				m.contactSearch += key
				m.applyContactSearch()
			}
		}
		return m, nil
	}

	switch key {
	case "/":
		m.contactSearchMode = "keyword"
		m.contactSearch = ""
	case "?":
		m.contactSearchMode = "semantic"
		m.contactSearch = ""
	case "esc":
		m.contactSearchMode = ""
		m.contactSearch = ""
		m.contactsFiltered = m.contactsList
		m.contactsIdx = 0
		m.contactDetail = nil
		m.contactDetailEmails = nil
		m.contactFocusPanel = 0
	case "tab":
		if m.contactDetail != nil {
			m.contactFocusPanel = 1 - m.contactFocusPanel
		}
	case "j", "down":
		if m.contactFocusPanel == 0 {
			if m.contactsIdx < len(m.contactsFiltered)-1 {
				m.contactsIdx++
			}
		} else {
			if m.contactDetailIdx < len(m.contactDetailEmails)-1 {
				m.contactDetailIdx++
			}
		}
	case "k", "up":
		if m.contactFocusPanel == 0 {
			if m.contactsIdx > 0 {
				m.contactsIdx--
			}
		} else {
			if m.contactDetailIdx > 0 {
				m.contactDetailIdx--
			}
		}
	case "enter":
		if m.contactFocusPanel == 0 {
			if len(m.contactsFiltered) > 0 && m.contactsIdx < len(m.contactsFiltered) {
				c := m.contactsFiltered[m.contactsIdx]
				m.contactDetail = &c
				m.contactDetailEmails = nil
				m.contactDetailIdx = 0
				return m, m.loadContactDetail(c)
			}
		} else {
			// Open selected email in Timeline tab
			if len(m.contactDetailEmails) > 0 && m.contactDetailIdx < len(m.contactDetailEmails) {
				email := m.contactDetailEmails[m.contactDetailIdx]
				m.activeTab = tabTimeline
				m.contactFocusPanel = 0
				m.setFocusedPanel(panelTimeline)
				m.selectedTimelineEmail = email
				m.emailBody = nil
				m.emailBodyLoading = true
				return m, tea.Batch(
					m.loadTimelineEmails(),
					m.loadEmailBodyCmd(email.Folder, email.UID),
				)
			}
		}
	case "e":
		var target *models.ContactData
		if m.contactFocusPanel == 1 && m.contactDetail != nil {
			target = m.contactDetail
		} else if len(m.contactsFiltered) > 0 && m.contactsIdx < len(m.contactsFiltered) {
			c := m.contactsFiltered[m.contactsIdx]
			target = &c
		}
		if target != nil {
			return m, m.runSingleContactEnrichment(*target)
		}
		return m, nil
	}
	return m, nil
}

// renderContactsTab renders the two-panel Contacts tab (left: list, right: detail).
func (m *Model) renderContactsTab(width, height int) string {
	if width < 20 {
		return "Terminal too narrow"
	}

	leftW := width * 35 / 100
	if leftW < 20 {
		leftW = 20
	}
	rightW := width - leftW - 4
	if rightW < 10 {
		rightW = 10
	}

	contentH := height - 6
	if contentH < 5 {
		contentH = 5
	}

	activeColor := lipgloss.Color("57")
	inactiveColor := lipgloss.Color("240")

	leftBorderColor := inactiveColor
	if m.contactFocusPanel == 0 {
		leftBorderColor = activeColor
	}
	rightBorderColor := inactiveColor
	if m.contactFocusPanel == 1 {
		rightBorderColor = activeColor
	}

	makePanel := func(borderColor lipgloss.Color, w int) lipgloss.Style {
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Width(w).
			Height(contentH).
			PaddingLeft(1)
	}

	// --- Left panel: contact list ---
	var leftSb strings.Builder

	if m.contactSearchMode == "keyword" {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render(fmt.Sprintf("/ %s_", m.contactSearch)) + "\n")
	} else if m.contactSearchMode == "semantic" {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(fmt.Sprintf("? %s_", m.contactSearch)) + "\n")
	} else {
		leftSb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(
			fmt.Sprintf("Contacts (%d)", len(m.contactsFiltered))) + "\n")
	}

	if len(m.contactsFiltered) == 0 {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  No contacts"))
	} else {
		maxRows := contentH - 3
		if maxRows < 1 {
			maxRows = 1
		}
		start := 0
		if m.contactsIdx >= maxRows {
			start = m.contactsIdx - maxRows + 1
		}
		end := start + maxRows
		if end > len(m.contactsFiltered) {
			end = len(m.contactsFiltered)
		}
		for i := start; i < end; i++ {
			c := m.contactsFiltered[i]
			nameStr := c.DisplayName
			if nameStr == "" {
				nameStr = fmt.Sprintf("<%s>", c.Email)
			} else {
				nameStr = fmt.Sprintf("%s <%s>", c.DisplayName, c.Email)
			}
			maxNameW := leftW - 8
			if maxNameW < 8 {
				maxNameW = 8
			}
			runes := []rune(nameStr)
			if len(runes) > maxNameW {
				nameStr = string(runes[:maxNameW-1]) + "…"
			}
			company := ""
			if c.Company != "" {
				company = fmt.Sprintf("[%s] ", c.Company)
				cr := []rune(company)
				if len(cr) > 14 {
					company = string(cr[:13]) + "…] "
				}
			}
			line := fmt.Sprintf("%-*s  %s%d", maxNameW, nameStr, company, c.EmailCount)
			rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
			if i == m.contactsIdx {
				rowStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("229")).
					Background(activeColor).
					Bold(true)
			}
			leftSb.WriteString(rowStyle.Render(line) + "\n")
		}
	}

	leftPanel := makePanel(leftBorderColor, leftW).Render(leftSb.String())

	// --- Right panel: contact detail ---
	var rightSb strings.Builder

	if m.contactDetail == nil {
		rightSb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
			Render("  Select a contact and press Enter"))
	} else {
		c := m.contactDetail
		boldStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

		displayName := c.DisplayName
		if displayName == "" {
			displayName = c.Email
		}
		rightSb.WriteString(boldStyle.Render(displayName) + "\n")
		rightSb.WriteString(dimStyle.Render(c.Email) + "\n")
		if c.Company != "" {
			rightSb.WriteString(normalStyle.Render("Company: "+c.Company) + "\n")
		}
		if len(c.Topics) > 0 {
			rightSb.WriteString(normalStyle.Render("Topics: "+strings.Join(c.Topics, ", ")) + "\n")
		}

		firstStr := "—"
		lastStr := "—"
		if !c.FirstSeen.IsZero() {
			firstStr = c.FirstSeen.Format("2006-01-02")
		}
		if !c.LastSeen.IsZero() {
			lastStr = c.LastSeen.Format("2006-01-02")
		}
		stats := fmt.Sprintf("First seen: %s  Last seen: %s  Received: %d  Sent: %d",
			firstStr, lastStr, c.EmailCount, c.SentCount)
		rightSb.WriteString(dimStyle.Render(stats) + "\n")

		if c.EnrichedAt != nil {
			rightSb.WriteString(dimStyle.Render("Enriched: "+c.EnrichedAt.Format("2006-01-02")) + "\n")
		}

		rightSb.WriteString("\n")
		rightSb.WriteString(boldStyle.Render("Recent Emails") + "\n")

		if len(m.contactDetailEmails) == 0 {
			rightSb.WriteString(dimStyle.Render("  Loading…") + "\n")
		} else {
			maxSubjW := rightW - 14
			if maxSubjW < 10 {
				maxSubjW = 10
			}
			for i, e := range m.contactDetailEmails {
				subj := e.Subject
				sr := []rune(subj)
				if len(sr) > maxSubjW {
					subj = string(sr[:maxSubjW-1]) + "…"
				}
				line := fmt.Sprintf("  %-*s  %s", maxSubjW, subj, e.Date.Format("2006-01-02"))
				rowStyle := normalStyle
				if m.contactFocusPanel == 1 && i == m.contactDetailIdx {
					rowStyle = lipgloss.NewStyle().
						Foreground(lipgloss.Color("229")).
						Background(activeColor).
						Bold(true)
				}
				rightSb.WriteString(rowStyle.Render(line) + "\n")
			}
		}
	}

	rightPanel := makePanel(rightBorderColor, rightW).Render(rightSb.String())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)
}
