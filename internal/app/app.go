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
	cfg            *config.Config
	imapClient     *imap.Client
	cache          *cache.Cache
	
	// UI State
	loading        bool
	loadingSpinner int
	startTime      time.Time
	progressInfo   models.ProgressInfo
	
	// Data
	stats          map[string]*models.SenderStats
	emailsBySender map[string][]*models.EmailData
	
	// Tables
	summaryTable   table.Model
	detailsTable   table.Model
	
	// Display options
	groupByDomain  bool
	selectedSender string
	selectedRows   map[int]bool
	
	// Styles
	baseStyle      lipgloss.Style
	headerStyle    lipgloss.Style
	loadingStyle   lipgloss.Style
	progressStyle  lipgloss.Style
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
		Padding(1, 2).
		Margin(1, 0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86"))

	progressStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Margin(0, 2)

	// Create tables
	summaryTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Domain/Sender", Width: 40},
			{Title: "Total Emails", Width: 12},
			{Title: "Avg Size (KB)", Width: 12},
			{Title: "With Attachments", Width: 15},
			{Title: "Date Range", Width: 20},
		}),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	detailsTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Date", Width: 20},
			{Title: "Subject", Width: 40},
			{Title: "Size", Width: 10},
			{Title: "Attachments", Width: 12},
		}),
		table.WithHeight(15),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	summaryTable.SetStyles(s)
	detailsTable.SetStyles(s)

	return &Model{
		cfg:            cfg,
		imapClient:     imapClient,
		cache:          cache,
		loading:        true,
		startTime:      time.Now(),
		selectedRows:   make(map[int]bool),
		summaryTable:   summaryTable,
		detailsTable:   detailsTable,
		baseStyle:      baseStyle,
		headerStyle:    headerStyle,
		loadingStyle:   loadingStyle,
		progressStyle:  progressStyle,
	}
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.startLoading(),
		m.tickSpinner(),
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
		return m, nil
		
	case LoadCompleteMsg:
		m.loading = false
		if msg.Error != nil {
			logger.Error("Error loading data: %v", msg.Error)
			return m, tea.Quit
		}
		m.stats = msg.Stats
		m.updateSummaryTable()
		return m, nil
		
	case tea.WindowSizeMsg:
		m.summaryTable.SetWidth(msg.Width / 2)
		m.detailsTable.SetWidth(msg.Width / 2)
		return m, nil
		
	case tickMsg:
		if m.loading {
			m.loadingSpinner = (m.loadingSpinner + 1) % len(spinnerChars)
			return m, m.tickSpinner()
		}
		return m, nil
	}

	// Update tables
	var cmd tea.Cmd
	m.summaryTable, cmd = m.summaryTable.Update(msg)
	cmds = append(cmds, cmd)
	
	m.detailsTable, cmd = m.detailsTable.Update(msg)
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
			return m, tea.Batch(m.startLoading(), m.tickSpinner())
		}
		return m, nil
		
	case " ":
		if !m.loading && m.summaryTable.Focused() {
			m.toggleSelection()
		}
		return m, nil
		
	case "D":
		if !m.loading {
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
				m.detailsTable.Focus()
			} else {
				m.detailsTable.Blur()
				m.summaryTable.Focus()
			}
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
	
	// Loading banner
	banner := fmt.Sprintf("%s %s", getPhaseIcon(m.progressInfo.Phase), m.progressInfo.Message)
	content.WriteString(m.loadingStyle.Render(banner) + "\n")
	
	// Progress info
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
			remaining := float64(m.progressInfo.Total - m.progressInfo.Current) * avgTime
			progressText += fmt.Sprintf(" | ETA: %.0fs", remaining)
		}
	}
	
	content.WriteString(m.progressStyle.Render(progressText) + "\n\n")
	
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
	header := fmt.Sprintf("📧 ProtonMail Analyzer - %s Mode", mode)
	content.WriteString(m.headerStyle.Render(header) + "\n\n")
	
	// Status
	status := fmt.Sprintf("Ready | %d senders", len(m.stats))
	if len(m.selectedRows) > 0 {
		status += fmt.Sprintf(" | %d selected", len(m.selectedRows))
	}
	content.WriteString(status + "\n\n")
	
	// Tables side by side
	summaryView := m.baseStyle.Render(m.summaryTable.View())
	detailsView := m.baseStyle.Render(m.detailsTable.View())
	
	tablesView := lipgloss.JoinHorizontal(lipgloss.Top, summaryView, " ", detailsView)
	content.WriteString(tablesView + "\n\n")
	
	// Help
	help := "q: quit | d: toggle domain mode | r: refresh | space: select | D: delete | enter: show details | tab: switch table"
	content.WriteString(help)
	
	return content.String()
}

// Helper functions and other methods continue in next part due to length...