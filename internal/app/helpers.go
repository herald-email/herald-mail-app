package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// openBrowserFn launches the system browser for the given URL. It is a
// package-level variable so tests can substitute a no-op to avoid spawning a
// real browser process.
var openBrowserFn = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

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
	engine.DryRun = m.dryRun
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

		senderColW := m.summaryTable.Columns()[1].Width
		if senderColW <= 0 {
			senderColW = 33
		}
		sender := styledSender(item.sender, senderColW)
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

// styledSender renders a sender string with the display name in bright white and
// the <email> part in dim gray, making the two visually distinct in table columns.
// The plain-text content is truncated to maxWidth before styling to avoid
// confusing runewidth-based layout in the table renderer.
func styledSender(raw string, maxWidth int) string {
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	emailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	// Try to split "Display Name <email@domain>" format.
	if lt := strings.Index(raw, " <"); lt > 0 && strings.HasSuffix(raw, ">") {
		name := sanitizeText(raw[:lt])
		email := raw[lt+1:] // keeps the "<...>" wrapper
		combined := name + " " + email
		if len([]rune(combined)) > maxWidth {
			combined = string([]rune(combined)[:maxWidth-1]) + "…"
		}
		// Re-locate the " <" boundary in the (possibly truncated) string.
		if lt2 := strings.Index(combined, " <"); lt2 > 0 {
			return nameStyle.Render(combined[:lt2]) + " " + emailStyle.Render(combined[lt2+1:])
		}
		// If "<" was truncated away, render everything as name.
		return nameStyle.Render(combined)
	}

	// Fallback: no "Name <email>" structure — render as plain name.
	plain := sanitizeText(raw)
	if len([]rune(plain)) > maxWidth {
		plain = string([]rune(plain)[:maxWidth-1]) + "…"
	}
	return nameStyle.Render(plain)
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
	if m.showCleanupPreview && m.cleanupFullScreen {
		// Full-screen: preview uses the entire terminal width
		m.cleanupPreviewWidth = width
	} else if m.showCleanupPreview {
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

		// Progressive column hiding: when there isn't enough space for the
		// Sender/Domain and Subject variable columns, hide lower-priority
		// fixed columns first.  Hide order (lowest priority first):
		//   1. Attach (6)  2. Att (3)  3. Date Range (20)  4. Avg KB (7)
		const minVariable = 24 // minimum combined sender + subject width

		attachW := 6
		dateRangeW := 20
		avgKBW := 7
		detailsAttW := 3

		sumFixed := summaryFixedCols   // 41
		detFixed := detailsFixedCols   // 29
		sumNCols := summaryNumCols     // 6
		detNCols := detailsNumCols     // 5

		calcVariable := func() int {
			overhead := sumNCols*2 + detNCols*2 + 4 + 2 + sidebarExtra + chatExtra
			v := width - overhead - sumFixed - detFixed
			if v < 0 {
				return 0
			}
			return v
		}

		if calcVariable() < minVariable && attachW > 0 {
			sumFixed -= 6
			sumNCols--
			attachW = 0
		}
		if calcVariable() < minVariable && detailsAttW > 0 {
			detFixed -= 3
			detNCols--
			detailsAttW = 0
		}
		if calcVariable() < minVariable && dateRangeW > 0 {
			sumFixed -= 20
			sumNCols--
			dateRangeW = 0
		}
		if calcVariable() < minVariable && avgKBW > 0 {
			sumFixed -= 7
			sumNCols--
			avgKBW = 0
		}

		cleanupVariable := calcVariable()
		senderWidth := cleanupVariable * 40 / 100
		subjectWidth := cleanupVariable - senderWidth
		if cleanupVariable >= minVariable {
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
			{Title: "Avg KB", Width: avgKBW},
			{Title: "Attach", Width: attachW},
			{Title: "Date Range", Width: dateRangeW},
		})
		m.summaryTable.SetWidth(sumFixed + senderWidth + sumNCols*2)

		m.subjectColWidth = subjectWidth
		m.detailsTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: subjectWidth},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: detailsAttW},
		})
		m.detailsTable.SetWidth(detFixed + subjectWidth + detNCols*2)
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
	m.composeCC.Width = composeInputWidth
	m.composeBCC.Width = composeInputWidth
	m.composeSubject.Width = composeInputWidth
	// Body: no label, borders add 2
	composeBodyWidth := width - chatExtra - 2
	if composeBodyWidth < 10 {
		composeBodyWidth = 10
	}
	// Reserve rows for: To(3) + CC(3) + BCC(3) + Subject(3) + divider(1) + status(1) + body border top/bot(2) = 16
	composeBodyHeight := tableHeight - 16
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
		starDot := " "
		if email.IsStarred {
			starDot = "★"
		}
		indicatorWidth := len([]rune(unreadDot)) + len([]rune(starDot)) + len([]rune(senderPrefix))
		senderAvail := maxSend - indicatorWidth
		if senderAvail < 1 {
			senderAvail = 1
		}
		sender := unreadDot + starDot + senderPrefix + styledSender(email.Sender, senderAvail)
		att := "N"
		if email.HasAttachments {
			att = "Y"
		}
		tag := ""
		if m.classifications != nil {
			tag = m.classifications[email.MessageID]
		}
		return table.Row{
			sender,
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

	// Sort starred threads to the top, preserving date order within each bucket.
	sort.SliceStable(m.threadGroups, func(i, j int) bool {
		iStarred := len(m.threadGroups[i].emails) > 0 && m.threadGroups[i].emails[0].IsStarred
		jStarred := len(m.threadGroups[j].emails) > 0 && m.threadGroups[j].emails[0].IsStarred
		return iStarred && !jStarred
	})

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
			// Build sender cell with the same indicators as single-email rows
			// so columns stay aligned across all timeline rows.
			unreadDot := " "
			if !newest.IsRead {
				unreadDot = "●"
			}
			starDot := " "
			if newest.IsStarred {
				starDot = "★"
			}
			indicatorWidth := len([]rune(unreadDot)) + len([]rune(starDot))
			senderAvail := maxSend - indicatorWidth
			if senderAvail < 1 {
				senderAvail = 1
			}
			threadSender := unreadDot + starDot + styledSender(newest.Sender, senderAvail)
			rows = append(rows, table.Row{
				threadSender,
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
		tableView = m.baseStyle.Render(renderStyledTableView(&m.timelineTable, 0))
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
		body := linkifyURLs(stripInvisibleChars(m.emailBody.TextPlain))
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

		// Pad short content so the indicator always sits at the bottom of the panel.
		shownLines := end - m.bodyScrollOffset
		for i := shownLines; i < visibleLines; i++ {
			sb.WriteString("\n")
		}

		// Scroll indicator (pinned to bottom)
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

		body := linkifyURLs(stripInvisibleChars(m.emailBody.TextPlain))
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

		// Pad short content so the indicator always sits at the bottom of the panel.
		visibleLines := end - m.bodyScrollOffset
		for i := visibleLines; i < maxBodyLines; i++ {
			sb.WriteString("\n")
		}

		// Scroll indicator (pinned to bottom)
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
	m.composeField = 4
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
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
		body := linkifyURLs(stripInvisibleChars(m.cleanupEmailBody.TextPlain))
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

		// Pad short content so the indicator always sits at the bottom of the panel.
		visibleLines := end - m.cleanupBodyScrollOffset
		for i := visibleLines; i < maxBodyLines; i++ {
			sb.WriteString("\n")
		}

		escHint := "Esc: close"
		zHint := "z: full-screen"
		if m.cleanupFullScreen {
			escHint = "Esc: close preview"
			zHint = "z: exit full-screen"
		}
		if totalLines > maxBodyLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.cleanupBodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" D: delete  e: archive  %s  ↑↓ j/k  line %d/%d  %d%%  │  %s", zHint, m.cleanupBodyScrollOffset+1, totalLines, pct, escHint)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		} else {
			indicator := fmt.Sprintf(" D: delete  e: archive  %s  ↑↓ j/k  │  %s", zHint, escHint)
			sb.WriteString("\n" + dimStyle.Render(indicator))
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

	// Dry-run mode indicator
	if m.dryRun {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render("[DRY RUN]"))
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
			hints = "tab/shift+tab: panels  │  ↑/k ↓/j: navigate  │  enter: open  │  esc: close  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  A: re-classify  │  q: quit"
		} else {
			hints = "1/2/3/4: tabs  │  ↑/k ↓/j: navigate  │  enter: open  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  /: search  │  a: AI tag  │  A: re-classify  │  f: sidebar  │  q: quit"
		}
	} else {
		switch m.focusedPanel {
		case panelSidebar:
			hints = "1/2/3/4: tabs  │  tab: next panel  │  ↑/k ↓/j: nav  │  space: expand  │  enter: open  │  r: refresh  │  a: AI tag  │  f: hide  │  c: chat  │  q: quit"
		case panelDetails:
			if m.showCleanupPreview {
				hints = "↑/k ↓/j: scroll preview  │  enter: scroll down  │  esc: close preview  │  tab: next panel  │  D: delete  │  A: re-classify  │  q: quit"
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

// reclassifyEmailCmd re-classifies a single email and stores the result.
func (m *Model) reclassifyEmailCmd(email *models.EmailData) tea.Cmd {
	classifier := m.classifier // snapshot before goroutine
	b := m.backend
	messageID := email.MessageID
	sender := email.Sender
	subject := email.Subject
	return func() tea.Msg {
		if classifier == nil {
			return ReclassifyResultMsg{Err: errors.New("no AI classifier configured")}
		}
		cat, err := classifier.Classify(sender, subject)
		if err != nil {
			return ReclassifyResultMsg{MessageID: messageID, Err: err}
		}
		if setErr := b.SetClassification(messageID, cat); setErr != nil {
			return ReclassifyResultMsg{MessageID: messageID, Err: setErr}
		}
		return ReclassifyResultMsg{MessageID: messageID, Category: cat}
	}
}

// autoClassifyEmailCmd classifies a newly arrived email in the background and
// returns AutoClassifyResultMsg. Unlike reclassifyEmailCmd, it is a fire-and-
// forget background op triggered automatically on email arrival — no visible
// status update is set on success.
func (m *Model) autoClassifyEmailCmd(email *models.EmailData) tea.Cmd {
	classifier := m.classifier // snapshot
	b := m.backend
	messageID := email.MessageID
	sender := email.Sender
	subject := email.Subject
	return func() tea.Msg {
		cat, err := classifier.Classify(sender, subject)
		if err != nil {
			return AutoClassifyResultMsg{MessageID: messageID, Err: err}
		}
		_ = b.SetClassification(messageID, cat)
		return AutoClassifyResultMsg{MessageID: messageID, Category: string(cat)}
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

	// When AI panel prompt is focused, route keystrokes to it
	if m.composeAIPanel && m.composeAIInput.Focused() {
		if msg.String() == "enter" {
			instruction := strings.TrimSpace(m.composeAIInput.Value())
			if instruction == "" {
				return m, nil
			}
			m.composeAILoading = true
			m.composeAIInput.SetValue("")
			return m, m.aiAssistCmd(instruction)
		}
		var cmd tea.Cmd
		m.composeAIInput, cmd = m.composeAIInput.Update(msg)
		return m, cmd
	}

	// When AI response textarea is focused, route to it
	if m.composeAIPanel && m.composeAIResponse.Focused() {
		var cmd tea.Cmd
		m.composeAIResponse, cmd = m.composeAIResponse.Update(msg)
		return m, cmd
	}

	// When AI panel is open, number keys 1-5 trigger quick actions
	if m.composeAIPanel && !m.composeAIInput.Focused() {
		actions := map[string]string{
			"1": "Improve the clarity and professionalism of this email",
			"2": "Shorten this email to be more concise",
			"3": "Lengthen this email with more detail",
			"4": "Rewrite this email in a formal tone",
			"5": "Rewrite this email in a casual, friendly tone",
		}
		if instruction, ok := actions[msg.String()]; ok {
			if m.composeBody.Value() == "" {
				m.composeStatus = "Write something first"
				return m, nil
			}
			m.composeAILoading = true
			return m, m.aiAssistCmd(instruction)
		}
	}

	// Autocomplete dropdown interactions take priority over normal field navigation
	if len(m.suggestions) > 0 {
		switch msg.String() {
		case "up":
			if m.suggestionIdx > 0 {
				m.suggestionIdx--
			}
			return m, nil
		case "down":
			if m.suggestionIdx < len(m.suggestions)-1 {
				m.suggestionIdx++
			}
			return m, nil
		case "enter", "tab":
			// Accept selected suggestion
			if m.suggestionIdx >= 0 && m.suggestionIdx < len(m.suggestions) {
				c := m.suggestions[m.suggestionIdx]
				label := c.DisplayName
				if label == "" {
					label = c.Email
				} else {
					label = fmt.Sprintf("%s <%s>", label, c.Email)
				}
				m.acceptSuggestion(label)
			}
			m.suggestions = nil
			m.suggestionIdx = -1
			return m, nil
		case "esc":
			m.suggestions = nil
			m.suggestionIdx = -1
			return m, nil
		}
		// Any other key: dismiss dropdown and fall through to normal key handling
		m.suggestions = nil
		m.suggestionIdx = -1
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
	case "4":
		m.activeTab = tabContacts
		m.contactFocusPanel = 0
		m.composeBody.Blur()
		return m, m.loadContacts()
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
	case "ctrl+g":
		if m.classifier == nil {
			m.composeStatus = "No AI backend configured"
			return m, nil
		}
		m.composeAIPanel = !m.composeAIPanel
		if m.composeAIPanel {
			m.composeAIInput.Focus()
		} else {
			m.composeAIInput.Blur()
			m.composeAIResponse.Blur()
		}
		return m, nil
	case "ctrl+j":
		if m.classifier == nil {
			m.composeStatus = "No AI backend configured"
			return m, nil
		}
		if m.composeBody.Value() == "" && m.replyContextEmail == nil {
			m.composeStatus = "Write something first"
			return m, nil
		}
		m.composeAILoading = true
		return m, m.aiSubjectCmd()
	case "ctrl+enter":
		if m.composeAIPanel && m.composeAIResponse.Value() != "" {
			m.composeBody.SetValue(m.composeAIResponse.Value())
			m.composeAIPanel = false
			m.composeAIDiff = ""
			m.composeAIResponse.SetValue("")
			m.composeAIInput.Blur()
			m.composeAIResponse.Blur()
			m.composeBody.Focus()
			m.composeField = 4
		}
		return m, nil
	case "tab":
		// If a subject hint is pending, Tab accepts it
		if m.composeAISubjectHint != "" {
			m.composeSubject.SetValue(m.composeAISubjectHint)
			m.composeAISubjectHint = ""
			return m, nil
		}
		m.cycleComposeField()
		return m, nil
	case "esc":
		// Dismiss subject hint
		if m.composeAISubjectHint != "" {
			m.composeAISubjectHint = ""
			return m, nil
		}
		// Close AI panel
		if m.composeAIPanel {
			m.composeAIPanel = false
			m.composeAIDiff = ""
			m.composeAIInput.Blur()
			m.composeAIResponse.Blur()
			return m, nil
		}
		m.composeStatus = ""
		return m, nil
	}
	// Forward all other keys to the focused field
	var cmd tea.Cmd
	switch m.composeField {
	case 0:
		m.composeTo, cmd = m.composeTo.Update(msg)
		return m, tea.Batch(cmd, m.searchContactsCmd(currentComposeToken(m.composeTo.Value())))
	case 1:
		m.composeCC, cmd = m.composeCC.Update(msg)
		return m, tea.Batch(cmd, m.searchContactsCmd(currentComposeToken(m.composeCC.Value())))
	case 2:
		m.composeBCC, cmd = m.composeBCC.Update(msg)
		return m, tea.Batch(cmd, m.searchContactsCmd(currentComposeToken(m.composeBCC.Value())))
	case 3:
		m.composeSubject, cmd = m.composeSubject.Update(msg)
	case 4:
		m.composeBody, cmd = m.composeBody.Update(msg)
	}
	return m, cmd
}

// renderSuggestionDropdown renders the autocomplete dropdown list.
// Returns an empty string when there are no suggestions.
func (m *Model) renderSuggestionDropdown() string {
	if len(m.suggestions) == 0 {
		return ""
	}
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("57")).
		Foreground(lipgloss.Color("255"))
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	maxW := m.windowWidth - 6
	if maxW < 20 {
		maxW = 20
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("57")).
		Padding(0, 1).
		MaxWidth(maxW)

	var rows []string
	for i, c := range m.suggestions {
		label := c.DisplayName
		if label == "" {
			label = c.Email
		} else {
			label = fmt.Sprintf("%s <%s>", label, c.Email)
		}
		if i == m.suggestionIdx {
			rows = append(rows, selectedStyle.Render(label))
		} else {
			rows = append(rows, normalStyle.Render(label))
		}
	}
	return boxStyle.Render(strings.Join(rows, "\n"))
}

// acceptSuggestion replaces the current token in the active address field
// with the accepted label (DisplayName <email>), followed by ", ".
func (m *Model) acceptSuggestion(label string) {
	replaceToken := func(existing, replacement string) string {
		if i := strings.LastIndex(existing, ","); i >= 0 {
			return existing[:i+1] + " " + replacement + ", "
		}
		return replacement + ", "
	}

	switch m.composeField {
	case 0:
		m.composeTo.SetValue(replaceToken(m.composeTo.Value(), label))
		m.composeTo.CursorEnd()
	case 1:
		m.composeCC.SetValue(replaceToken(m.composeCC.Value(), label))
		m.composeCC.CursorEnd()
	case 2:
		m.composeBCC.SetValue(replaceToken(m.composeBCC.Value(), label))
		m.composeBCC.CursorEnd()
	}
}

// cycleComposeField advances focus to the next compose input field.
// Order: To(0) → CC(1) → BCC(2) → Subject(3) → Body(4) → wrap.
func (m *Model) cycleComposeField() {
	m.composeField = (m.composeField + 1) % 5
	// Clear autocomplete when moving away from address fields (0–2)
	if m.composeField > 2 {
		m.suggestions = nil
		m.suggestionIdx = -1
	}
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	switch m.composeField {
	case 0:
		m.composeTo.Focus()
	case 1:
		m.composeCC.Focus()
	case 2:
		m.composeBCC.Focus()
	case 3:
		m.composeSubject.Focus()
	case 4:
		m.composeBody.Focus()
	}
}

// sendCompose sends the composed message via SMTP as multipart/alternative
// (HTML + plain-text fallback). The body textarea is treated as Markdown.
// Any staged attachments are sent as multipart/mixed parts.
// Local inline images referenced as ![alt](~/path) or ![alt](/path) are
// embedded as multipart/related parts with cid: references.
func (m *Model) sendCompose() tea.Cmd {
	mailer := m.mailer // snapshot before goroutine to avoid data races
	from := m.fromAddress
	to := m.composeTo.Value()
	cc := m.composeCC.Value()
	bcc := m.composeBCC.Value()
	subject := m.composeSubject.Value()
	markdownBody := m.composeBody.Value()
	attachments := m.composeAttachments // snapshot; cleared on success in Update()
	return func() tea.Msg {
		if mailer == nil {
			return ComposeStatusMsg{Message: "Error: SMTP not configured", Err: fmt.Errorf("smtp not configured")}
		}
		if to == "" {
			return ComposeStatusMsg{Message: "Error: To field is empty"}
		}
		if subject == "" {
			return ComposeStatusMsg{Message: "Error: Subject is empty"}
		}
		htmlBody, inlines, inlineErr := appsmtp.BuildInlineImages(markdownBody)
		if inlineErr != nil {
			// Log and fall back to plain markdown conversion without inline images.
			logger.Warn("inline image embedding failed: %v", inlineErr)
			htmlBody, _ = appsmtp.MarkdownToHTMLAndPlain(markdownBody)
			inlines = nil
		}
		_, plainText := appsmtp.MarkdownToHTMLAndPlain(markdownBody)
		err := mailer.SendWithInlineImages(from, to, subject, plainText, htmlBody, cc, bcc, attachments, inlines)
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

	// CC field
	ccStyle := inactiveFieldStyle
	if m.composeField == 1 {
		ccStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("CC:"),
		ccStyle.Render(m.composeCC.View()),
	) + "\n")

	// BCC field
	bccStyle := inactiveFieldStyle
	if m.composeField == 2 {
		bccStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("BCC:"),
		bccStyle.Render(m.composeBCC.View()),
	) + "\n")

	// Autocomplete dropdown (shown when address field has suggestions)
	if drop := m.renderSuggestionDropdown(); drop != "" {
		sb.WriteString(drop + "\n")
	}

	// Subject field
	subStyle := inactiveFieldStyle
	if m.composeField == 3 {
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

	// Subject hint (shown below divider when a suggestion is pending)
	if m.composeAISubjectHint != "" {
		hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		hintText := m.composeAISubjectHint
		if len(hintText) > divWidth-30 && divWidth > 35 {
			hintText = hintText[:divWidth-30] + "…"
		}
		hint := hintStyle.Render("✨ "+hintText) +
			"  " + dimStyle.Render("Tab: accept  Esc: dismiss")
		sb.WriteString(hint + "\n")
	}

	// Body + optional AI panel
	bodyAreaWidth := m.windowWidth - 4
	if bodyAreaWidth < 10 {
		bodyAreaWidth = 10
	}
	if m.composeAIPanel {
		// Split: panel takes ~40%, body takes the rest (min panel width = 36)
		panelWidth := 40
		if bodyAreaWidth < 80 {
			panelWidth = bodyAreaWidth / 2
		}
		bodyWidth := bodyAreaWidth - panelWidth - 3 // 3 for gap/border
		if bodyWidth < 20 {
			bodyWidth = 20
		}

		var bodyPane string
		if m.composePreview {
			previewLabel := lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Render("  Preview (Ctrl+P to edit)  ")
			body := m.composeBody.Value()
			if body == "" {
				body = "_empty body_"
			}
			rendered := body
			if r, err := glamour.Render(body, "dark"); err == nil {
				rendered = r
			}
			bodyPane = previewLabel + "\n" + lipgloss.NewStyle().Width(bodyWidth).Render(rendered)
		} else {
			bodyStyle := inactiveFieldStyle.Width(bodyWidth)
			if m.composeField == 4 {
				bodyStyle = activeFieldStyle.Width(bodyWidth)
			}
			m.composeBody.SetWidth(bodyWidth)
			bodyPane = bodyStyle.Render(m.composeBody.View())
		}
		panelPane := m.renderAIPanel(panelWidth)
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, bodyPane, "  ", panelPane) + "\n")
	} else {
		// Normal full-width body / preview
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
			if m.composeField == 4 {
				bodyStyle = activeFieldStyle
			}
			sb.WriteString(bodyStyle.Render(m.composeBody.View()) + "\n")
		}
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
		// Parse angle-bracket-delimited URIs: <https://...>, <http://...>, <mailto:...>
		var httpsURL, httpURL, mailtoAddr string
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if len(part) >= 2 && part[0] == '<' && part[len(part)-1] == '>' {
				part = part[1 : len(part)-1]
			}
			if strings.HasPrefix(part, "https://") && httpsURL == "" {
				httpsURL = part
			} else if strings.HasPrefix(part, "http://") && httpURL == "" {
				httpURL = part
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
		// Browser fallback: open HTTP/HTTPS URL in the system browser
		webURL := httpsURL
		if webURL == "" {
			webURL = httpURL
		}
		if webURL != "" {
			if err := openBrowserFn(webURL); err == nil {
				return UnsubscribeResultMsg{Method: "browser-opened", URL: webURL}
			}
			// fall through to clipboard on exec error
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

// toggleStarCmd toggles the \Flagged IMAP flag and returns a StarResultMsg.
func (m *Model) toggleStarCmd(email *models.EmailData) tea.Cmd {
	b := m.backend
	messageID := email.MessageID
	folder := email.Folder
	starred := !email.IsStarred
	return func() tea.Msg {
		var err error
		if starred {
			err = b.MarkStarred(messageID, folder)
		} else {
			err = b.UnmarkStarred(messageID, folder)
		}
		if err != nil {
			return StarResultMsg{MessageID: messageID, Err: err}
		}
		return StarResultMsg{MessageID: messageID, Starred: starred}
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

// urlRe matches http/https URLs.
var urlRe = regexp.MustCompile(`https?://[^\s<>\[\](){}"'` + "`" + `]+`)

// linkifyURLs replaces raw URLs with OSC 8 terminal hyperlinks.
// The visible text is a shortened version (domain + truncated path);
// the full URL is embedded in the escape sequence so terminals can open it on click.
func linkifyURLs(text string) string {
	return urlRe.ReplaceAllStringFunc(text, func(raw string) string {
		// Trim trailing punctuation that's likely not part of the URL
		trimmed := strings.TrimRight(raw, ".,;:!?)")
		label := shortenURL(trimmed)
		// OSC 8: \033]8;;URL\033\\ LABEL \033]8;;\033\\
		return "\033]8;;" + trimmed + "\033\\" + label + "\033]8;;\033\\"
	})
}

// shortenURL produces a human-readable label like "example.com/path…"
func shortenURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		if len(raw) > 40 {
			return raw[:37] + "..."
		}
		return raw
	}
	host := parsed.Hostname()
	path := parsed.Path
	if q := parsed.RawQuery; q != "" {
		path += "?" + q
	}
	if path == "" || path == "/" {
		return host
	}
	// Show domain + beginning of path, cap at 50 visible chars
	full := host + path
	if len(full) > 50 {
		return full[:47] + "..."
	}
	return full
}

// wrapText wraps text to fit within width visible columns.
// Uses ansi.StringWidth so ANSI escape sequences (OSC 8 hyperlinks, SGR
// styling) are not counted toward visible width.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	runes := []rune(text)
	for len(runes) > 0 {
		if ansi.StringWidth(string(runes)) <= width {
			lines = append(lines, string(runes))
			break
		}
		// Walk forward counting visible width to find the cut point.
		visW := 0
		cut := 0
		for cut < len(runes) {
			// Check for escape sequence start (ESC)
			if runes[cut] == '\033' {
				// Skip the entire escape sequence without adding to visW.
				seqEnd := skipEscapeSeq(runes, cut)
				cut = seqEnd
				continue
			}
			rw := ansi.StringWidth(string(runes[cut : cut+1]))
			if visW+rw > width {
				break
			}
			visW += rw
			cut++
		}
		// Try to break at the last space within the visible width.
		bestCut := cut
		for bestCut > 0 && runes[bestCut-1] != ' ' {
			bestCut--
		}
		if bestCut == 0 {
			bestCut = cut
		}
		if bestCut == 0 && cut == 0 {
			// Safety: avoid infinite loop on zero-width content
			bestCut = 1
		}
		lines = append(lines, string(runes[:bestCut]))
		rest := runes[bestCut:]
		for len(rest) > 0 && rest[0] == ' ' {
			rest = rest[1:]
		}
		runes = rest
	}
	return lines
}

// skipEscapeSeq advances past an escape sequence starting at runes[pos].
// Handles OSC (ESC ]) and CSI (ESC [) sequences.
func skipEscapeSeq(runes []rune, pos int) int {
	if pos >= len(runes) || runes[pos] != '\033' {
		return pos + 1
	}
	pos++ // skip ESC
	if pos >= len(runes) {
		return pos
	}
	switch runes[pos] {
	case ']': // OSC sequence — terminated by ST (ESC \) or BEL (\a)
		pos++
		for pos < len(runes) {
			if runes[pos] == '\a' {
				return pos + 1
			}
			if runes[pos] == '\033' && pos+1 < len(runes) && runes[pos+1] == '\\' {
				return pos + 2
			}
			pos++
		}
	case '[': // CSI sequence — terminated by 0x40-0x7E
		pos++
		for pos < len(runes) {
			if runes[pos] >= 0x40 && runes[pos] <= 0x7E {
				return pos + 1
			}
			pos++
		}
	default:
		// Unknown sequence type, just skip the ESC
	}
	return pos
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
		emails, err := m.backend.GetContactEmails(contact.Email, 20)
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
		// Close inline email preview first, then detail, then search
		if m.contactPreviewEmail != nil {
			m.contactPreviewEmail = nil
			m.contactPreviewBody = nil
			m.contactPreviewLoading = false
			return m, nil
		}
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
			// Open selected email inline in the contact detail panel
			if len(m.contactDetailEmails) > 0 && m.contactDetailIdx < len(m.contactDetailEmails) {
				email := m.contactDetailEmails[m.contactDetailIdx]
				m.contactPreviewEmail = email
				m.contactPreviewBody = nil
				m.contactPreviewLoading = true
				return m, m.loadEmailBodyCmd(email.Folder, email.UID)
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
	// Each panel's outer visual width = Width(w) + 2 border chars; separator = 2 chars.
	// Total = (leftW+2) + 2 + (rightW+2) = leftW+rightW+6, so rightW = width-leftW-6.
	rightW := width - leftW - 6
	if rightW < 10 {
		rightW = 10
	}

	// height is the full terminal height. renderMainView adds chrome around us:
	// header(1) + tab bar(1) + blank(1) + "\n" after content(1) + status bar(1) + key hints(1) = 6.
	// Each panel also adds 2 border lines (top + bottom), so total deduction = 8.
	contentH := height - 8
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

			// Inner content width = Width(leftW) - PaddingLeft(1) = leftW-1.
			innerW := leftW - 1

			// Progressive column layout based on available width:
			// Wide (>=60): Name | Email | Company | Count
			// Medium (>=35): Name | Email | Count
			// Narrow (<35): Name | Count
			countW := 4
			showEmail := innerW >= 35
			showCompany := innerW >= 60

			displayName := c.DisplayName
			if displayName == "" {
				displayName = c.Email
			}
			countStr := fmt.Sprintf("%d", c.EmailCount)

			var line string
			if showCompany {
				companyW := 14
				separators := 6 // 3 x "  "
				nameW := (innerW - separators - countW - companyW) * 55 / 100
				if nameW < 8 {
					nameW = 8
				}
				emailW := innerW - separators - countW - companyW - nameW
				if emailW < 6 {
					emailW = 6
				}
				dn := ansi.Truncate(displayName, nameW, "…")
				em := ansi.Truncate(c.Email, emailW, "…")
				co := ""
				if c.Company != "" {
					co = ansi.Truncate(c.Company, companyW, "…")
				}
				dnPad := strings.Repeat(" ", nameW-ansi.StringWidth(dn))
				emPad := strings.Repeat(" ", emailW-ansi.StringWidth(em))
				coPad := strings.Repeat(" ", companyW-ansi.StringWidth(co))
				if i == m.contactsIdx {
					bg := activeColor
					ns := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(bg).Bold(true)
					es := lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Background(bg)
					cs := lipgloss.NewStyle().Foreground(lipgloss.Color("223")).Background(bg)
					ks := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(bg).Bold(true)
					bs := lipgloss.NewStyle().Background(bg)
					line = ns.Render(dn) + bs.Render(dnPad+"  ") + es.Render(em) + bs.Render(emPad+"  ") + cs.Render(co) + bs.Render(coPad+"  ") + ks.Render(countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
					es := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
					cs := lipgloss.NewStyle().Foreground(lipgloss.Color("249"))
					ks := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
					line = ns.Render(dn) + dnPad + "  " + es.Render(em) + emPad + "  " + cs.Render(co) + coPad + "  " + ks.Render(countStr)
				}
			} else if showEmail {
				separators := 4 // 2 x "  "
				nameW := (innerW - separators - countW) * 45 / 100
				if nameW < 8 {
					nameW = 8
				}
				emailW := innerW - separators - countW - nameW
				if emailW < 6 {
					emailW = 6
				}
				dn := ansi.Truncate(displayName, nameW, "…")
				em := ansi.Truncate(c.Email, emailW, "…")
				dnPad := strings.Repeat(" ", nameW-ansi.StringWidth(dn))
				emPad := strings.Repeat(" ", emailW-ansi.StringWidth(em))
				if i == m.contactsIdx {
					bg := activeColor
					ns := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(bg).Bold(true)
					es := lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Background(bg)
					ks := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(bg).Bold(true)
					bs := lipgloss.NewStyle().Background(bg)
					line = ns.Render(dn) + bs.Render(dnPad+"  ") + es.Render(em) + bs.Render(emPad+"  ") + ks.Render(countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
					es := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
					ks := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
					line = ns.Render(dn) + dnPad + "  " + es.Render(em) + emPad + "  " + ks.Render(countStr)
				}
			} else {
				// Narrow: just name + count
				nameW := innerW - 2 - countW
				if nameW < 4 {
					nameW = 4
				}
				dn := ansi.Truncate(displayName, nameW, "…")
				dnPad := strings.Repeat(" ", nameW-ansi.StringWidth(dn))
				if i == m.contactsIdx {
					bg := activeColor
					s := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(bg).Bold(true)
					line = s.Render(dn+dnPad+"  "+countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
					ks := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
					line = ns.Render(dn) + dnPad + "  " + ks.Render(countStr)
				}
			}
			leftSb.WriteString(line + "\n")
		}
	}

	leftPanel := makePanel(leftBorderColor, leftW).Render(leftSb.String())

	// --- Right panel: contact detail ---
	var rightSb strings.Builder

	if m.contactPreviewEmail != nil {
		// Inline email preview within the Contacts tab
		email := m.contactPreviewEmail
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		boldStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
		rightSb.WriteString(boldStyle.Render("From: "+sanitizeText(email.Sender)) + "\n")
		rightSb.WriteString(dimStyle.Render("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04")) + "\n")
		rightSb.WriteString(boldStyle.Render("Subj: "+sanitizeText(email.Subject)) + "\n")
		rightSb.WriteString(strings.Repeat("─", rightW-1) + "\n")
		if m.contactPreviewLoading {
			rightSb.WriteString(dimStyle.Render("Loading…"))
		} else if m.contactPreviewBody != nil {
			body := linkifyURLs(stripInvisibleChars(m.contactPreviewBody.TextPlain))
			if body == "" {
				body = "(No text content)"
			}
			innerW := rightW - 1
			if innerW < 10 {
				innerW = 10
			}
			// Render markdown via glamour for HTML-converted content (same as Timeline)
			var lines []string
			if m.contactPreviewBody.IsFromHTML {
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						lines = strings.Split(rendered, "\n")
					} else {
						lines = wrapLines(body, innerW)
					}
				} else {
					lines = wrapLines(body, innerW)
				}
			} else {
				lines = wrapLines(body, innerW)
			}
			maxLines := contentH - 6 // header(4) + hint(1) + margin
			if maxLines < 1 {
				maxLines = 1
			}
			if len(lines) > maxLines {
				lines = lines[:maxLines]
			}
			rightSb.WriteString(strings.Join(lines, "\n"))
			// Pad to push hint to bottom
			for i := len(lines); i < maxLines; i++ {
				rightSb.WriteString("\n")
			}
		}
		rightSb.WriteString("\n" + dimStyle.Render(" Esc: back to contact"))
	} else if m.contactDetail == nil {
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
			// Inner content = Width(rightW) - PaddingLeft(1) = rightW-1.
			// Line = "  "(2) + subj(maxSubjW) + "  "(2) + date(10) = maxSubjW+14.
			// To fit: maxSubjW+14 <= rightW-1 → maxSubjW = rightW-15.
			maxSubjW := rightW - 15
			if maxSubjW < 10 {
				maxSubjW = 10
			}
			for i, e := range m.contactDetailEmails {
				subj := ansi.Truncate(e.Subject, maxSubjW, "…")
				subjPad := strings.Repeat(" ", maxSubjW-ansi.StringWidth(subj))
				line := "  " + subj + subjPad + "  " + e.Date.Format("2006-01-02")
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

// --- Draft auto-save helpers ---

// draftSaveTick returns a Cmd that fires DraftSaveTickMsg after 30 seconds.
func draftSaveTick() tea.Cmd {
	return tea.Tick(30*time.Second, func(_ time.Time) tea.Msg {
		return DraftSaveTickMsg{}
	})
}

// currentComposeToken returns the text after the last comma in s, trimmed.
// This is the fragment being typed for autocomplete in a comma-separated
// address field.
func currentComposeToken(s string) string {
	if i := strings.LastIndex(s, ","); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s)
}

// searchContactsCmd queries SearchContacts with token and returns a
// ContactSuggestionsMsg. Clears suggestions when token is shorter than 2 chars.
func (m *Model) searchContactsCmd(token string) tea.Cmd {
	if len(token) < 2 {
		return func() tea.Msg { return ContactSuggestionsMsg{} }
	}
	backend := m.backend
	return func() tea.Msg {
		contacts, err := backend.SearchContacts(token)
		if err != nil || len(contacts) == 0 {
			return ContactSuggestionsMsg{}
		}
		if len(contacts) > 5 {
			contacts = contacts[:5]
		}
		return ContactSuggestionsMsg{Contacts: contacts}
	}
}

// composeHasContent returns true if any compose field is non-empty.
func composeHasContent(m *Model) bool {
	return m.composeTo.Value() != "" || m.composeSubject.Value() != "" || m.composeBody.Value() != ""
}

// saveDraftCmd saves the current compose content as a draft.
// Snapshots the values before the goroutine to prevent data races.
func (m *Model) saveDraftCmd() tea.Cmd {
	backend := m.backend
	to := m.composeTo.Value()
	cc := m.composeCC.Value()
	bcc := m.composeBCC.Value()
	subject := m.composeSubject.Value()
	body := m.composeBody.Value()
	return func() tea.Msg {
		uid, folder, err := backend.SaveDraft(to, cc, bcc, subject, body)
		return DraftSavedMsg{UID: uid, Folder: folder, Err: err}
	}
}

// deleteDraftCmd deletes the draft with the given UID from the given folder.
func (m *Model) deleteDraftCmd(uid uint32, folder string) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		err := backend.DeleteDraft(uid, folder)
		return DraftDeletedMsg{Err: err}
	}
}

// tokenizeWords splits s into a slice of word and non-word tokens,
// preserving whitespace and punctuation as separate tokens.
// "Hello, world" → ["Hello", ",", " ", "world"]
func tokenizeWords(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(r))
		} else if r == ',' || r == '.' || r == '!' || r == '?' || r == ';' || r == ':' || r == '"' || r == '\'' || r == '(' || r == ')' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// lcsTokens returns the longest common subsequence of token slices a and b.
func lcsTokens(a, b []string) []string {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	result := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, a[i-1])
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
		result[l], result[r] = result[r], result[l]
	}
	return result
}

// wordDiff computes a word-level diff between original and revised and returns
// a lipgloss-styled string. Deleted tokens appear red with strikethrough,
// added tokens appear green, unchanged tokens are unstyled.
func wordDiff(original, revised string) string {
	delStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Strikethrough(true).
		Background(lipgloss.Color("52"))
	addStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("46")).
		Background(lipgloss.Color("22"))

	origTokens := tokenizeWords(original)
	revTokens := tokenizeWords(revised)
	common := lcsTokens(origTokens, revTokens)

	var sb strings.Builder
	i, j, k := 0, 0, 0
	for k < len(common) {
		for i < len(origTokens) && origTokens[i] != common[k] {
			sb.WriteString(delStyle.Render(origTokens[i]))
			i++
		}
		for j < len(revTokens) && revTokens[j] != common[k] {
			sb.WriteString(addStyle.Render(revTokens[j]))
			j++
		}
		sb.WriteString(common[k])
		i++
		j++
		k++
	}
	for i < len(origTokens) {
		sb.WriteString(delStyle.Render(origTokens[i]))
		i++
	}
	for j < len(revTokens) {
		sb.WriteString(addStyle.Render(revTokens[j]))
		j++
	}
	return sb.String()
}

// aiAssistCmd fires an AI body-rewrite request with the given instruction.
// If m.composeAIThread is true and m.replyContextEmail is non-nil, the
// original email's sender and subject are included as context.
func (m *Model) aiAssistCmd(instruction string) tea.Cmd {
	classifier := m.classifier
	draft := m.composeBody.Value()
	threadCtx := m.composeAIThread
	replyEmail := m.replyContextEmail

	return func() tea.Msg {
		if classifier == nil {
			return AIAssistMsg{Err: fmt.Errorf("no AI backend configured")}
		}
		if strings.TrimSpace(draft) == "" {
			return AIAssistMsg{Err: fmt.Errorf("draft is empty")}
		}

		var contextParts []string
		if threadCtx && replyEmail != nil {
			contextParts = append(contextParts,
				fmt.Sprintf("This email is a reply to:\nFrom: %s\nSubject: %s",
					replyEmail.Sender, replyEmail.Subject))
		}
		contextParts = append(contextParts, "Current draft:\n"+draft)
		context := strings.Join(contextParts, "\n\n")

		messages := []ai.ChatMessage{
			{
				Role: "system",
				Content: "You are an expert email writing assistant. " +
					"Rewrite the email body according to the user's instruction. " +
					"Return only the rewritten body text, no explanations or preamble.",
			},
			{
				Role:    "user",
				Content: instruction + "\n\n" + context,
			},
		}
		result, err := classifier.Chat(messages)
		if err != nil {
			return AIAssistMsg{Err: err}
		}
		return AIAssistMsg{Result: strings.TrimSpace(result)}
	}
}

// aiSubjectCmd fires an AI subject-suggestion request using the current
// draft body and, if available, the thread context.
func (m *Model) aiSubjectCmd() tea.Cmd {
	classifier := m.classifier
	draft := m.composeBody.Value()
	threadCtx := m.composeAIThread
	replyEmail := m.replyContextEmail

	return func() tea.Msg {
		if classifier == nil {
			return AISubjectMsg{Err: fmt.Errorf("no AI backend configured")}
		}

		var contextParts []string
		if threadCtx && replyEmail != nil {
			contextParts = append(contextParts,
				fmt.Sprintf("Original email subject: %s\nFrom: %s",
					replyEmail.Subject, replyEmail.Sender))
		}
		if strings.TrimSpace(draft) != "" {
			contextParts = append(contextParts, "Email body:\n"+draft)
		}
		if len(contextParts) == 0 {
			return AISubjectMsg{Err: fmt.Errorf("nothing to base a subject on")}
		}

		messages := []ai.ChatMessage{
			{
				Role: "system",
				Content: "You are an email writing assistant. " +
					"Suggest a concise, specific email subject line (maximum 10 words). " +
					"Return only the subject line text, no quotes, no explanation.",
			},
			{
				Role:    "user",
				Content: strings.Join(contextParts, "\n\n"),
			},
		}
		result, err := classifier.Chat(messages)
		if err != nil {
			return AISubjectMsg{Err: err}
		}
		return AISubjectMsg{Subject: strings.TrimSpace(result)}
	}
}

// renderAIPanel renders the compose AI assistant panel.
// Returns an empty string when composeAIPanel is false.
// width is the panel's character width.
func (m *Model) renderAIPanel(width int) string {
	if !m.composeAIPanel {
		return ""
	}
	if width < 20 {
		width = 20
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		Width(width)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(width)
	activeToggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("25")).
		Padding(0, 1)
	inactiveToggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)
	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Margin(0, 1, 0, 0)
	acceptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("28")).
		Padding(0, 1)
	discardStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	// Title
	sb.WriteString(titleStyle.Render("🤖 AI Assistant") + "\n")
	sb.WriteString(strings.Repeat("─", width) + "\n")

	// Context toggle — only shown when replying (replyContextEmail != nil)
	if m.replyContextEmail != nil {
		sb.WriteString(labelStyle.Render("Context:") + "\n")
		threadLabel := inactiveToggleStyle.Render("Thread")
		draftLabel := inactiveToggleStyle.Render("Draft only")
		if m.composeAIThread {
			threadLabel = activeToggleStyle.Render("● Thread")
		} else {
			draftLabel = activeToggleStyle.Render("● Draft only")
		}
		sb.WriteString(threadLabel + "  " + draftLabel + "\n\n")
	}

	// Quick action buttons
	actions := []string{"Improve", "Shorten", "Lengthen", "Formal", "Casual"}
	var actionRow strings.Builder
	for _, a := range actions {
		actionRow.WriteString(actionStyle.Render(a))
	}
	sb.WriteString(actionRow.String() + "\n\n")

	// Free-form prompt input
	sb.WriteString(labelStyle.Render("Custom prompt:") + "\n")
	m.composeAIInput.Width = width - 2
	sb.WriteString(m.composeAIInput.View() + "\n\n")

	// Loading spinner
	if m.composeAILoading {
		sb.WriteString(spinnerStyle.Render("⠋ Thinking…") + "\n")
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Render(sb.String())
	}

	// Diff view (if result available)
	if m.composeAIDiff != "" {
		sb.WriteString(labelStyle.Render("Changes:") + "\n")
		diffStyle := lipgloss.NewStyle().
			Width(width - 2).
			MaxWidth(width - 2)
		sb.WriteString(diffStyle.Render(m.composeAIDiff) + "\n\n")
	}

	// Editable response textarea
	if m.composeAIResponse.Value() != "" || m.composeAIDiff != "" {
		sb.WriteString(labelStyle.Render("Suggestion (edit freely):") + "\n")
		m.composeAIResponse.SetWidth(width - 2)
		m.composeAIResponse.SetHeight(8)
		sb.WriteString(m.composeAIResponse.View() + "\n\n")

		// Accept / Discard
		sb.WriteString(acceptStyle.Render("✓ Accept") + "  " + discardStyle.Render("Discard") + "\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
			"Ctrl+Enter: accept  Esc: discard") + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("86")).
		Width(width).
		Render(sb.String())
}
