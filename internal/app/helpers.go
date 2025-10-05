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

// startLoading starts the data loading process
func (m *Model) startLoading() tea.Cmd {
	return func() tea.Msg {
		go func() {
			logger.Info("Starting data loading process...")
			
			// Send initial progress
			logger.Debug("Sending connecting progress...")
			m.progressCh <- models.ProgressInfo{
				Phase:   "connecting", 
				Message: "Connecting to IMAP server...",
			}
			logger.Debug("Connecting progress sent")
			time.Sleep(200 * time.Millisecond) // Allow UI to catch up
			
			// Connect to IMAP
			logger.Info("Connecting to IMAP server...")
			if err := m.imapClient.Connect(); err != nil {
				logger.Error("Failed to connect to IMAP: %v", err)
				// Send error through progress channel instead
				return
			}
			// Keep connection open for future operations (don't close after loading)
			logger.Debug("IMAP connection established and will remain open")

			// Process emails first (this will send its own progress updates)
			logger.Info("Processing emails...")
			logger.Debug("Starting ProcessEmails...")
			if err := m.imapClient.ProcessEmails("INBOX"); err != nil {
				logger.Error("Failed to process emails: %v", err)
				return
			}
			logger.Debug("ProcessEmails completed")

			// Clean up cache after processing (remove emails that no longer exist on server)
			logger.Info("Cleaning up stale cache entries...")
			m.progressCh <- models.ProgressInfo{
				Phase:   "cleanup",
				Message: "Cleaning up cache...",
			}
			if err := m.imapClient.CleanupCache("INBOX"); err != nil {
				logger.Warn("Cache cleanup failed (non-critical): %v", err)
				// Don't return - this is not critical
			}
			logger.Debug("Cache cleanup completed")

			// Get statistics
			m.progressCh <- models.ProgressInfo{
				Phase:   "finalizing",
				Message: "Generating statistics...",
			}
			logger.Info("Generating statistics...")
			stats, err := m.imapClient.GetSenderStatistics("INBOX")
			if err != nil {
				logger.Error("Failed to get statistics: %v", err)
				return
			}

			// Send completion through progress channel
			m.progressCh <- models.ProgressInfo{
				Phase:   "complete",
				Message: fmt.Sprintf("Found %d senders", len(stats)),
			}
			logger.Info("Data loading completed successfully. Found %d senders", len(stats))
		}()
		
		// Return immediately - progress will come through the channel
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
	emails, err := m.imapClient.GetEmailsBySender("INBOX")
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
		if len(subject) > 32 {
			subject = subject[:29] + "..."
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
	m.imapClient.SetGroupByDomain(m.groupByDomain)
	
	// Reload statistics
	m.loading = true
	m.startTime = time.Now()
	
	// Note: In a real implementation, you'd want to reload the data
	// For now, we'll just update the display
	m.loading = false
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

// deleteSelected deletes the selected senders or individual messages
func (m *Model) deleteSelected() tea.Cmd {
	return func() tea.Msg {
		var deletedCount int

		// Check if details table is focused - delete individual messages
		if m.detailsTable.Focused() {
			if len(m.selectedMessages) > 0 {
				// Delete all selected messages (across all senders)
				for messageID := range m.selectedMessages {
					// Find the email by message ID to get subject for logging
					var email *models.EmailData

					// Search through all senders' emails
					allEmails, err := m.imapClient.GetEmailsBySender("INBOX")
					if err != nil {
						logger.Error("Failed to get emails: %v", err)
						continue
					}

					found := false
					for _, emails := range allEmails {
						for _, e := range emails {
							if e.MessageID == messageID {
								email = e
								found = true
								break
							}
						}
						if found {
							break
						}
					}

					if !found || email == nil {
						logger.Warn("Message not found: %s", messageID)
						continue
					}

					logger.Info("Deleting individual message: %s (ID: %s)", email.Subject, email.MessageID)
					if err := m.imapClient.DeleteEmail(email.MessageID, "INBOX"); err != nil {
						logger.Error("Failed to delete message: %v", err)
						continue
					}
					deletedCount++
				}
				m.selectedMessages = make(map[string]bool)
			} else {
				// Delete current message
				cursor := m.detailsTable.Cursor()
				if cursor < len(m.detailsEmails) {
					email := m.detailsEmails[cursor]
					logger.Info("Deleting individual message: %s (ID: %s)", email.Subject, email.MessageID)
					if err := m.imapClient.DeleteEmail(email.MessageID, "INBOX"); err != nil {
						logger.Error("Failed to delete message: %v", err)
						return LoadCompleteMsg{Error: err}
					}
					deletedCount = 1
				}
			}
		} else if len(m.selectedRows) > 0 {
			// Delete multiple selected senders
			for cursor := range m.selectedRows {
				// Get original sender from mapping (before sanitization)
				sender, ok := m.rowToSender[cursor]
				if !ok || sender == "" {
					logger.Warn("No sender mapping found for row %d", cursor)
					continue
				}

				logger.Info("Deleting emails from: %s", sender)
				if err := m.imapClient.DeleteSenderEmails(sender, "INBOX"); err != nil {
					logger.Error("Failed to delete emails from %s: %v", sender, err)
					continue
				}
				deletedCount++
			}
			m.selectedRows = make(map[int]bool)
		} else {
			// Delete current sender using row mapping
			cursor := m.summaryTable.Cursor()
			logger.Info("Deletion requested: cursor=%d, total mappings=%d", cursor, len(m.rowToSender))

			// Debug: print all mappings
			for i, s := range m.rowToSender {
				logger.Debug("Row %d -> Sender '%s'", i, s)
			}

			sender, ok := m.rowToSender[cursor]
			if !ok || sender == "" {
				logger.Warn("No sender selected for deletion at cursor %d", cursor)
				return LoadCompleteMsg{Error: fmt.Errorf("no sender selected")}
			}

			logger.Info("Deleting emails from: '%s' (length=%d bytes)", sender, len(sender))
			if err := m.imapClient.DeleteSenderEmails(sender, "INBOX"); err != nil {
				logger.Error("Failed to delete emails: %v", err)
				return LoadCompleteMsg{Error: err}
			}
			deletedCount = 1
		}

		if deletedCount > 0 {
			logger.Info("Deleted emails from %d sender(s), reloading...", deletedCount)
		}

		// Reload data
		stats, err := m.imapClient.GetSenderStatistics("INBOX")
		if err != nil {
			return LoadCompleteMsg{Error: err}
		}

		return LoadCompleteMsg{Stats: stats}
	}
}

// cleanup cleans up resources
func (m *Model) cleanup() {
	if m.imapClient != nil {
		m.imapClient.Close()
	}
	if m.cache != nil {
		m.cache.Close()
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