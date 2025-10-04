package app

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
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

// startLoading starts the data loading process
func (m *Model) startLoading() tea.Cmd {
	return func() tea.Msg {
		logger.Info("Starting data loading process...")
		
		// Connect to IMAP
		logger.Info("Connecting to IMAP server...")
		if err := m.imapClient.Connect(); err != nil {
			logger.Error("Failed to connect to IMAP: %v", err)
			return LoadCompleteMsg{Error: fmt.Errorf("failed to connect: %w", err)}
		}
		defer func() {
			logger.Debug("Closing IMAP connection...")
			m.imapClient.Close()
		}()

		// Clean up cache first
		logger.Info("Cleaning up cache...")
		if err := m.imapClient.CleanupCache("INBOX"); err != nil {
			logger.Error("Failed to cleanup cache: %v", err)
			return LoadCompleteMsg{Error: fmt.Errorf("failed to cleanup cache: %w", err)}
		}

		// Process emails
		logger.Info("Processing emails...")
		if err := m.imapClient.ProcessEmails("INBOX"); err != nil {
			logger.Error("Failed to process emails: %v", err)
			return LoadCompleteMsg{Error: fmt.Errorf("failed to process emails: %w", err)}
		}

		// Get statistics
		logger.Info("Generating statistics...")
		stats, err := m.imapClient.GetSenderStatistics("INBOX")
		if err != nil {
			logger.Error("Failed to get statistics: %v", err)
			return LoadCompleteMsg{Error: fmt.Errorf("failed to get statistics: %w", err)}
		}

		logger.Info("Data loading completed successfully. Found %d senders", len(stats))
		return LoadCompleteMsg{Stats: stats}
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
			dateStr = email.Date.Format("2006-01-02 15:04")
		}

		subject := email.Subject
		if subject == "" {
			subject = "No Subject"
		}
		if len(subject) > 40 {
			subject = subject[:37] + "..."
		}

		attachments := "No"
		if email.HasAttachments {
			attachments = "Yes"
		}

		row := table.Row{
			dateStr,
			subject,
			fmt.Sprintf("%.1f KB", float64(email.Size)/1024),
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