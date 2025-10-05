package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/cache"
	"mail-processor/internal/config"
	"mail-processor/internal/imap"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// LoadingMsg represents a loading state update
type LoadingMsg struct {
	Info models.ProgressInfo
}

// LoadCompleteMsg indicates loading is complete
type LoadCompleteMsg struct {
	Stats map[string]*models.SenderStats
	Error error
}

// Model represents the main application state
type Model struct {
	cfg        *config.Config
	imapClient *imap.Client
	cache      *cache.Cache
	progressCh chan models.ProgressInfo

	// UI State
	loading        bool
	deleting       bool
	loadingSpinner int
	startTime      time.Time
	progressInfo   models.ProgressInfo
	showLogs       bool
	windowHeight   int

	// Data
	stats          map[string]*models.SenderStats
	emailsBySender map[string][]*models.EmailData

	// Tables
	summaryTable table.Model
	detailsTable table.Model
	logViewer    *LogViewer

	// Display options
	groupByDomain      bool
	selectedSender     string
	selectedRows       map[int]bool              // Selected rows in summary table
	selectedMessages   map[string]bool           // Selected messages by MessageID (across all senders)
	rowToSender        map[int]string            // Maps row index to original sender (before sanitization)
	detailsEmails      []*models.EmailData       // Current emails shown in details table

	// Styles
	baseStyle          lipgloss.Style
	headerStyle        lipgloss.Style
	loadingStyle       lipgloss.Style
	progressStyle      lipgloss.Style
	activeTableStyle   table.Styles
	inactiveTableStyle table.Styles
}

// New creates a new application model
func New(cfg *config.Config) *Model {
	logger.Info("Creating new application model")

	// Create cache
	logger.Debug("Initializing cache database...")
	cache, err := cache.New("email_cache.db")
	if err != nil {
		logger.Error("Failed to create cache: %v", err)
		panic(fmt.Sprintf("Failed to create cache: %v", err))
	}
	logger.Info("Cache database initialized successfully")

	// Create progress channel
	progressCh := make(chan models.ProgressInfo, 100)

	// Create IMAP client
	imapClient := imap.New(cfg, cache, progressCh)

	// Setup styles
	baseStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	loadingStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235")).
		Padding(1, 3).
		Margin(1, 0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		Align(lipgloss.Center)

	progressStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Margin(0, 2)

	// Create tables optimized for side-by-side display
	// Summary table: ~82 chars total (left side) - added selection column
	summaryTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Sender/Domain", Width: 33},
			{Title: "Count", Width: 6},
			{Title: "Avg KB", Width: 7},
			{Title: "Attach", Width: 6},
			{Title: "Date Range", Width: 20},
		}),
		table.WithFocused(true),
		table.WithHeight(11),
	)

	// Details table: ~69 chars total (right side) - added selection column
	detailsTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: 32},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: 3},
		}),
		table.WithHeight(11),
	)

	// Create active table style (blue highlight)
	activeStyle := table.DefaultStyles()
	activeStyle.Header = activeStyle.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	activeStyle.Selected = activeStyle.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)

	// Create inactive table style (gray highlight)
	inactiveStyle := table.DefaultStyles()
	inactiveStyle.Header = inactiveStyle.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	inactiveStyle.Selected = inactiveStyle.Selected.
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color("238")).
		Bold(false)

	summaryTable.SetStyles(activeStyle)
	detailsTable.SetStyles(inactiveStyle)

	// Create log viewer
	logViewer := NewLogViewer(80, 15)

	// Set up log callback to capture logs in TUI
	logger.SetLogCallback(func(level, message string) {
		logViewer.AddLog(level, message)
	})

	return &Model{
		cfg:                cfg,
		imapClient:         imapClient,
		cache:              cache,
		progressCh:         progressCh,
		loading:            true,
		startTime:          time.Now(),
		selectedRows:       make(map[int]bool),
		selectedMessages:   make(map[string]bool),
		rowToSender:        make(map[int]string),
		summaryTable:       summaryTable,
		detailsTable:       detailsTable,
		logViewer:          logViewer,
		baseStyle:          baseStyle,
		headerStyle:        headerStyle,
		loadingStyle:       loadingStyle,
		progressStyle:      progressStyle,
		activeTableStyle:   activeStyle,
		inactiveTableStyle: inactiveStyle,
	}
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.startLoading(),
		m.tickSpinner(),
		m.listenForProgress(),
	)
}

// Update implements tea.Model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case LoadingMsg:
		m.progressInfo = msg.Info
		// Check if this is completion
		if msg.Info.Phase == "complete" {
			// Get final statistics
			stats, err := m.imapClient.GetSenderStatistics("INBOX")
			if err != nil {
				logger.Error("Failed to get final statistics: %v", err)
				return m, tea.Quit
			}
			m.loading = false
			m.stats = stats
			m.updateSummaryTable()
			m.updateDetailsTable() // Show details for first sender
			return m, nil
		}
		// Continue listening for more progress updates
		return m, m.listenForProgress()

	case LoadCompleteMsg:
		m.loading = false
		m.deleting = false
		if msg.Error != nil {
			logger.Error("Error loading data: %v", msg.Error)
			return m, tea.Quit
		}
		m.stats = msg.Stats
		m.updateSummaryTable()
		m.updateDetailsTable() // Show details for first sender
		return m, nil

	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height

		// Use fixed widths based on actual column sizes instead of 50/50 split
		// Summary table: 35+6+7+6+20 = 74 + borders/padding = ~80
		// Details table: 16+35+8+3 = 62 + borders/padding = ~68
		m.summaryTable.SetWidth(80)
		m.detailsTable.SetWidth(68)

		// Set table height to 90% of window height
		// Account for header (3 lines), status (2 lines), help (2 lines) = ~7 lines overhead
		tableHeight := int(float64(msg.Height) * 0.9) - 7
		if tableHeight < 5 {
			tableHeight = 5 // Minimum height
		}
		m.summaryTable.SetHeight(tableHeight)
		m.detailsTable.SetHeight(tableHeight)

		return m, nil

	case tickMsg:
		if m.loading {
			m.loadingSpinner = (m.loadingSpinner + 1) % len(spinnerChars)
			return m, m.tickSpinner()
		}
		return m, nil
	}

	// Update tables and log viewer
	var cmd tea.Cmd
	m.summaryTable, cmd = m.summaryTable.Update(msg)
	cmds = append(cmds, cmd)

	m.detailsTable, cmd = m.detailsTable.Update(msg)
	cmds = append(cmds, cmd)

	// Update log viewer
	_, cmd = m.logViewer.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m *Model) View() string {
	if m.loading {
		return m.renderLoadingView()
	}
	return m.renderMainView()
}

// handleKeyMsg handles keyboard input
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.cleanup()
		return m, tea.Quit

	case "d":
		if !m.loading {
			m.toggleDomainMode()
		}
		return m, nil

	case "r":
		if !m.loading {
			m.loading = true
			m.startTime = time.Now()
			return m, tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForProgress())
		}
		return m, nil

	case " ":
		if !m.loading {
			m.toggleSelection()
		}
		return m, nil

	case "D":
		if !m.loading && !m.deleting {
			m.deleting = true
			return m, m.deleteSelected()
		}
		return m, nil

	case "enter":
		if !m.loading && m.summaryTable.Focused() {
			m.updateDetailsTable()
		}
		return m, nil

	case "tab":
		if !m.loading {
			if m.summaryTable.Focused() {
				m.summaryTable.Blur()
				m.summaryTable.SetStyles(m.inactiveTableStyle)
				m.detailsTable.Focus()
				m.detailsTable.SetStyles(m.activeTableStyle)
			} else {
				m.detailsTable.Blur()
				m.detailsTable.SetStyles(m.inactiveTableStyle)
				m.summaryTable.Focus()
				m.summaryTable.SetStyles(m.activeTableStyle)
			}
		}
		return m, nil

	case "l", "L":
		if !m.loading {
			m.showLogs = !m.showLogs
		}
		return m, nil

	case "up", "k":
		if !m.loading {
			return m.handleNavigation(-1)
		}
		return m, nil

	case "down", "j":
		if !m.loading {
			return m.handleNavigation(1)
		}
		return m, nil
	}

	return m, nil
}

// renderLoadingView renders the loading screen
func (m *Model) renderLoadingView() string {
	elapsed := time.Since(m.startTime)
	spinner := spinnerChars[m.loadingSpinner]

	var content strings.Builder

	// Header
	content.WriteString(m.headerStyle.Render("📧 ProtonMail Analyzer") + "\n\n")

	// Loading banner (manually pad to center with emoji compensation)
	icon := getPhaseIcon(m.progressInfo.Phase)
	message := m.progressInfo.Message
	if message == "" {
		message = "Starting up..."
	}
	banner := fmt.Sprintf(" %s %s ", icon, message) // Add extra space to compensate for emoji width
	content.WriteString(m.loadingStyle.Render(banner) + "\n")

	// Progress info with visual progress bar
	progressText := fmt.Sprintf("%s Elapsed: %.1fs", spinner, elapsed.Seconds())
	if m.progressInfo.Total > 0 {
		percent := 0
		if m.progressInfo.Current > 0 {
			percent = (m.progressInfo.Current * 100) / m.progressInfo.Total
		}
		progressText = fmt.Sprintf("Progress: %d/%d (%d%%) | Elapsed: %.1fs | %s",
			m.progressInfo.Current, m.progressInfo.Total, percent, elapsed.Seconds(), spinner)

		// Add ETA if processing
		if m.progressInfo.Current > 0 && m.progressInfo.Phase == "processing" {
			avgTime := elapsed.Seconds() / float64(m.progressInfo.Current)
			remaining := float64(m.progressInfo.Total-m.progressInfo.Current) * avgTime
			progressText += fmt.Sprintf(" | ETA: %.0fs", remaining)
		}

		// Add visual progress bar
		progressBar := m.renderProgressBar(percent, 50)
		content.WriteString(m.progressStyle.Render(progressText) + "\n")
		content.WriteString(progressBar + "\n\n")
	} else {
		content.WriteString(m.progressStyle.Render(progressText) + "\n\n")
	}

	// Instructions
	content.WriteString("Press 'q' to quit")

	return content.String()
}

// renderMainView renders the main application view
func (m *Model) renderMainView() string {
	var content strings.Builder

	// Header
	mode := "Email"
	if m.groupByDomain {
		mode = "Domain"
	}
	logIndicator := ""
	if m.showLogs {
		logIndicator = " | Logs: ON"
	}
	header := fmt.Sprintf("📧 ProtonMail Analyzer - %s Mode%s", mode, logIndicator)
	content.WriteString(m.headerStyle.Render(header) + "\n\n")

	// Status - calculate total emails from all senders
	totalEmails := 0
	for _, stats := range m.stats {
		totalEmails += stats.TotalEmails
	}
	status := "Ready"
	if m.deleting {
		status = "Deleting..."
	}
	status += fmt.Sprintf(" | %d senders | %d emails", len(m.stats), totalEmails)
	if len(m.selectedRows) > 0 {
		status += fmt.Sprintf(" | %d senders selected", len(m.selectedRows))
	}
	if len(m.selectedMessages) > 0 {
		status += fmt.Sprintf(" | %d messages selected", len(m.selectedMessages))
	}
	content.WriteString(status + "\n\n")

	if m.showLogs {
		// Show logs view
		logTitle := m.headerStyle.Render("📋 Real-time Logs")
		content.WriteString(logTitle + "\n")

		logView := m.baseStyle.Render(m.logViewer.View())
		content.WriteString(logView + "\n\n")

		// Help for log view
		help := "q: quit | l: toggle logs | r: refresh | d: toggle domain mode | ↑/k ↓/j: navigate"
		content.WriteString(help)
	} else {
		// Show tables side by side with minimal spacing
		summaryView := m.baseStyle.Render(m.summaryTable.View())
		detailsView := m.baseStyle.Render(m.detailsTable.View())

		// Use 2 spaces between tables instead of proportional spacing
		tablesView := lipgloss.JoinHorizontal(lipgloss.Top, summaryView, "  ", detailsView)
		content.WriteString(tablesView + "\n")

		// Add spacing before help text
		content.WriteString("\n")

		// Help for table view
		help := "q: quit | l: toggle logs | d: toggle domain mode | r: refresh | ↑/k ↓/j: navigate | space: select | D: delete | enter: show details | tab: switch table"
		content.WriteString(help)

		// Add bottom padding to prevent overlap
		content.WriteString("\n")
	}

	return content.String()
}

// Helper functions and other methods continue in next part due to length...
