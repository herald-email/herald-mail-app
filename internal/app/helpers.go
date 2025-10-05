package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

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
			defer func() {
				logger.Debug("Closing IMAP connection...")
				m.imapClient.Close()
			}()

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

	sort.Slice(sortedStats, func(i, j int) bool {
		return sortedStats[i].stats.TotalEmails > sortedStats[j].stats.TotalEmails
	})

	// Build table rows
	var rows []table.Row
	for _, item := range sortedStats {
		sender := item.sender
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

		row := table.Row{
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
	if m.summaryTable.Cursor() >= len(m.summaryTable.Rows()) {
		return
	}

	selectedRow := m.summaryTable.SelectedRow()
	if len(selectedRow) == 0 {
		return
	}

	sender := selectedRow[0]
	m.selectedSender = sender

	// Get emails for this sender
	emails, err := m.imapClient.GetEmailsBySender("INBOX")
	if err != nil {
		return
	}

	senderEmails := emails[sender]
	if len(senderEmails) == 0 {
		return
	}

	// Sort emails by date (newest first)
	sort.Slice(senderEmails, func(i, j int) bool {
		return senderEmails[i].Date.After(senderEmails[j].Date)
	})

	// Build table rows
	var rows []table.Row
	for _, email := range senderEmails {
		dateStr := "N/A"
		if !email.Date.IsZero() {
			dateStr = email.Date.Format("06-01-02 15:04")
		}

		subject := email.Subject
		if subject == "" {
			subject = "No Subject"
		}
		if len(subject) > 35 {
			subject = subject[:32] + "..."
		}

		attachments := "N"
		if email.HasAttachments {
			attachments = "Y"
		}

		row := table.Row{
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

// toggleSelection toggles selection of the current row
func (m *Model) toggleSelection() {
	cursor := m.summaryTable.Cursor()
	if m.selectedRows[cursor] {
		delete(m.selectedRows, cursor)
	} else {
		m.selectedRows[cursor] = true
	}
}

// deleteSelected deletes the selected senders or current sender
func (m *Model) deleteSelected() tea.Cmd {
	return func() tea.Msg {
		if len(m.selectedRows) > 0 {
			// Delete multiple selected senders
			rows := m.summaryTable.Rows()
			for cursor := range m.selectedRows {
				if cursor < len(rows) {
					sender := rows[cursor][0]
					if err := m.imapClient.DeleteSenderEmails(sender, "INBOX"); err != nil {
						// Log error but continue
						continue
					}
				}
			}
			m.selectedRows = make(map[int]bool)
		} else {
			// Delete current sender
			if m.summaryTable.Cursor() < len(m.summaryTable.Rows()) {
				selectedRow := m.summaryTable.SelectedRow()
				if len(selectedRow) > 0 {
					sender := selectedRow[0]
					if err := m.imapClient.DeleteSenderEmails(sender, "INBOX"); err != nil {
						return LoadCompleteMsg{Error: err}
					}
				}
			}
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