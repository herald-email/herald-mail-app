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

// startLoading kicks off the backend's load sequence for INBOX.
func (m *Model) startLoading() tea.Cmd {
	return func() tea.Msg {
		m.backend.Load("INBOX")
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
	// Get original sender from row mapping (before sanitization)
	cursor := m.summaryTable.Cursor()
	sender, ok := m.rowToSender[cursor]
	if !ok || sender == "" {
		logger.Debug("No sender mapping found for cursor %d", cursor)
		m.detailsTable.SetRows([]table.Row{})
		return
	}

	m.selectedSender = sender

	// Get emails for this sender
	emails, err := m.backend.GetEmailsBySender("INBOX")
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
	stats, err := m.backend.GetSenderStatistics("INBOX")
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
	// Count how many deletions we're queueing
	deletionCount := 0

	// Send deletion requests to the queue
	go func() {
		// Check if details table is focused - delete individual messages
		if m.detailsTable.Focused() {
			if len(m.selectedMessages) > 0 {
				// Delete all selected messages (across all senders)
				for messageID := range m.selectedMessages {
					m.deletionRequestCh <- models.DeletionRequest{
						MessageID: messageID,
						Folder:    "INBOX",
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
						Folder:    "INBOX",
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
					Folder:   "INBOX",
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
					Folder:   "INBOX",
				}
			}
		}
	}()

	// Calculate deletion count before goroutine clears the maps
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

	// Fixed (non-resizable) column widths:
	//   Summary: checkmark(2) + count(6) + avgkb(7) + attach(6) + daterange(20) = 41
	//   Details: checkmark(2) + date(16) + size(8) + att(3) = 29
	const summaryFixedCols = 41
	const summaryNumCols = 6
	const detailsFixedCols = 29
	const detailsNumCols = 5

	// Total rendering overhead:
	//   bubbles/table default cell style Padding(0,1) adds 2 chars per cell
	//   baseStyle NormalBorder adds 2 chars (left+right) per table = 4 total
	//   2-space gap between tables
	// overhead = summaryNumCols*2 + detailsNumCols*2 + 4 + 2 = 12 + 10 + 4 + 2 = 28
	const overhead = 28

	// Space remaining for the two variable-width columns (Sender/Domain and Subject)
	variable := width - overhead - summaryFixedCols - detailsFixedCols
	if variable < 24 {
		variable = 24
	}

	// 40% to sender column, 60% to subject column
	senderWidth := variable * 40 / 100
	if senderWidth < 12 {
		senderWidth = 12
	}
	subjectWidth := variable - senderWidth
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

	// Height: full terminal minus chrome (header 2, status 2, help 2, padding 1 = 7)
	tableHeight := height - 7
	if tableHeight < 5 {
		tableHeight = 5
	}
	m.summaryTable.SetHeight(tableHeight)
	m.detailsTable.SetHeight(tableHeight)

	// Update log viewer to match
	logWidth := width - 4
	if logWidth < 20 {
		logWidth = 20
	}
	m.logViewer.SetSize(logWidth, tableHeight)
}

// handleNavigation handles up/down navigation for the focused table
func (m *Model) handleNavigation(direction int) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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
