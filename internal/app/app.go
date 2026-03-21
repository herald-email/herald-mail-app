package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/backend"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// Panel focus constants
const (
	panelSidebar = 0
	panelSummary = 1
	panelDetails = 2
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

// FoldersLoadedMsg carries the folder list fetched after connect
type FoldersLoadedMsg struct {
	Folders []string
}

// FolderStatusMsg carries MESSAGES/UNSEEN counts for all folders
type FolderStatusMsg struct {
	Status map[string]models.FolderStatus
}

// Model represents the main application state
type Model struct {
	backend    backend.Backend
	progressCh <-chan models.ProgressInfo

	// UI State
	loading          bool
	deleting         bool
	deletionProgress models.DeletionResult
	deletionsPending int // Number of deletions waiting to complete
	deletionsTotal   int // Total deletions in current batch
	loadingSpinner   int
	startTime        time.Time
	progressInfo     models.ProgressInfo
	showLogs         bool
	windowWidth      int
	windowHeight     int
	subjectColWidth  int

	// Deletion channels
	deletionRequestCh chan models.DeletionRequest
	deletionResultCh  chan models.DeletionResult

	// Data
	stats          map[string]*models.SenderStats
	emailsBySender map[string][]*models.EmailData

	// Tables
	summaryTable table.Model
	detailsTable table.Model
	logViewer    *LogViewer

	// Display options
	groupByDomain      bool
	currentFolder      string
	selectedSender     string
	selectedRows       map[int]bool              // Selected rows in summary table
	selectedMessages   map[string]bool           // Selected messages by MessageID (across all senders)
	rowToSender        map[int]string            // Maps row index to original sender (before sanitization)
	detailsEmails      []*models.EmailData       // Current emails shown in details table

	// Sidebar
	folders       []string
	folderTree    []*folderNode
	folderStatus  map[string]models.FolderStatus
	showSidebar   bool
	sidebarCursor int
	focusedPanel  int // panelSidebar, panelSummary, panelDetails

	// Styles
	baseStyle          lipgloss.Style
	headerStyle        lipgloss.Style
	loadingStyle       lipgloss.Style
	progressStyle      lipgloss.Style
	activeTableStyle   table.Styles
	inactiveTableStyle table.Styles
}

// New creates a new application model backed by the given Backend.
func New(b backend.Backend) *Model {
	logger.Info("Creating new application model")

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
	logViewer := NewLogViewer(140, 15)

	// Set up log callback to capture logs in TUI
	logger.SetLogCallback(func(level, message string) {
		logViewer.AddLog(level, message)
	})

	// Create deletion channels
	deletionRequestCh := make(chan models.DeletionRequest, 10)
	deletionResultCh := make(chan models.DeletionResult, 10)

	m := &Model{
		backend:            b,
		progressCh:         b.Progress(),
		loading:            true,
		startTime:          time.Now(),
		currentFolder:      "INBOX",
		folders:            []string{"INBOX"},
		folderTree:         buildFolderTree([]string{"INBOX"}),
		folderStatus:       make(map[string]models.FolderStatus),
		showSidebar:        true,
		focusedPanel:       panelSummary,
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
		deletionRequestCh:  deletionRequestCh,
		deletionResultCh:   deletionResultCh,
	}

	// Start deletion worker goroutine
	go m.deletionWorker()

	return m
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

	case FoldersLoadedMsg:
		m.folders = msg.Folders
		m.folderTree = buildFolderTree(msg.Folders)
		// Keep cursor on the active folder
		items := flattenTree(m.folderTree)
		m.sidebarCursor = 0
		for i, item := range items {
			if item.node.fullPath == m.currentFolder {
				m.sidebarCursor = i
				break
			}
		}
		// Only fetch counts on first load to avoid flickering on every folder switch
		if len(m.folderStatus) == 0 {
			folders := msg.Folders
			return m, func() tea.Msg {
				status, err := m.backend.GetFolderStatus(folders)
				if err != nil {
					logger.Warn("Failed to get folder status: %v", err)
					return FolderStatusMsg{Status: map[string]models.FolderStatus{}}
				}
				return FolderStatusMsg{Status: status}
			}
		}
		return m, nil

	case FolderStatusMsg:
		// Merge rather than replace so partial results don't wipe existing counts
		for folder, st := range msg.Status {
			m.folderStatus[folder] = st
		}
		return m, nil

	case LoadingMsg:
		m.progressInfo = msg.Info
		switch msg.Info.Phase {
		case "complete":
			stats, err := m.backend.GetSenderStatistics(m.currentFolder)
			if err != nil {
				logger.Error("Failed to get final statistics: %v", err)
				return m, tea.Quit
			}
			m.loading = false
			m.stats = stats
			m.updateSummaryTable()
			m.summaryTable.GotoTop()
			m.updateDetailsTable()
			return m, func() tea.Msg {
				folders, err := m.backend.ListFolders()
				if err != nil {
					logger.Warn("Failed to list folders: %v", err)
					return FoldersLoadedMsg{Folders: []string{"INBOX"}}
				}
				return FoldersLoadedMsg{Folders: folders}
			}
		case "error":
			// Stop loading; keep existing data so the user can still navigate
			logger.Error("Load error: %s", msg.Info.Message)
			m.loading = false
			if m.stats == nil {
				m.stats = map[string]*models.SenderStats{}
			}
			m.updateSummaryTable()
			m.updateDetailsTable()
			return m, nil
		default:
			return m, m.listenForProgress()
		}

	case LoadCompleteMsg:
		m.loading = false
		m.deleting = false
		if msg.Error != nil {
			logger.Error("Error loading data: %v", msg.Error)
			return m, tea.Quit
		}
		m.stats = msg.Stats
		m.updateSummaryTable()
		m.updateDetailsTable()
		return m, nil

	case models.DeletionResult:
		// Update deletion progress
		m.deletionProgress = msg
		m.deletionsPending--

		if msg.Error != nil {
			logger.Error("Deletion error: %v", msg.Error)
		} else {
			// Remove from local cache immediately for instant UI update
			if msg.Sender != "" {
				logger.Info("Deletion complete: %s", msg.Sender)
				// Remove sender from stats
				delete(m.stats, msg.Sender)
			} else if msg.MessageID != "" {
				logger.Info("Deletion complete: message %s", msg.MessageID)
				// Remove individual message from cache
				// We don't have sender info here, so we'll update on full reload
			}

			// Update UI immediately
			m.updateSummaryTable()
			m.updateDetailsTable()
		}

		// Check if all deletions are complete
		if m.deletionsPending <= 0 {
			logger.Info("All %d deletions complete, reloading data...", m.deletionsTotal)
			m.deleting = false
			m.deletionsPending = 0
			m.deletionsTotal = 0

			// Reload data after all deletions complete to sync with server
			stats, err := m.backend.GetSenderStatistics(m.currentFolder)
			if err != nil {
				logger.Error("Failed to reload after deletion: %v", err)
				return m, nil
			}
			m.stats = stats
			m.updateSummaryTable()
			m.updateDetailsTable()
			return m, nil
		}

		// Continue listening for more results
		return m, m.listenForDeletionResults()

	case tea.WindowSizeMsg:
		m.updateTableDimensions(msg.Width, msg.Height)
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

	case "f":
		if !m.loading {
			m.showSidebar = !m.showSidebar
			if m.windowWidth > 0 {
				m.updateTableDimensions(m.windowWidth, m.windowHeight)
			}
			// If sidebar was hidden and focus was on it, move to summary
			if !m.showSidebar && m.focusedPanel == panelSidebar {
				m.setFocusedPanel(panelSummary)
			}
		}
		return m, nil

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
			if m.focusedPanel == panelSidebar {
				m.toggleSidebarNode()
			} else {
				m.toggleSelection()
			}
		}
		return m, nil

	case "D":
		if !m.loading && !m.deleting {
			m.deleting = true
			return m, m.deleteSelected()
		}
		return m, nil

	case "enter":
		if !m.loading {
			if m.focusedPanel == panelSidebar {
				m.selectSidebarFolder()
				return m, tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForProgress())
			} else if m.focusedPanel == panelSummary {
				m.updateDetailsTable()
			}
		}
		return m, nil

	case "tab":
		if !m.loading {
			m.cyclePanel()
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
		completed := m.deletionsTotal - m.deletionsPending
		if m.deletionProgress.Sender != "" {
			status = fmt.Sprintf("Deleting: %s (%d/%d)", m.deletionProgress.Sender, completed, m.deletionsTotal)
		} else if m.deletionProgress.MessageID != "" {
			status = fmt.Sprintf("Deleting message (%d/%d)", completed, m.deletionsTotal)
		} else {
			status = fmt.Sprintf("Deleting... (%d/%d)", completed, m.deletionsTotal)
		}
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
		summaryView := m.baseStyle.Render(m.summaryTable.View())
		detailsView := m.baseStyle.Render(m.detailsTable.View())

		var tablesView string
		if m.showSidebar {
			sidebarView := m.baseStyle.Render(m.renderSidebar())
			tablesView = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, "  ", summaryView, "  ", detailsView)
		} else {
			tablesView = lipgloss.JoinHorizontal(lipgloss.Top, summaryView, "  ", detailsView)
		}
		content.WriteString(tablesView + "\n\n")

		help := "q: quit | f: sidebar | l: logs | d: domain | r: refresh | ↑/k ↓/j: nav | space: select | D: delete | enter: details/select | tab: switch panel"
		content.WriteString(help + "\n")
	}

	return content.String()
}

// Helper functions and other methods continue in next part due to length...
