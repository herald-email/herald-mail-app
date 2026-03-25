package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/ai"
	"mail-processor/internal/backend"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	appsmtp "mail-processor/internal/smtp"
)

// Panel focus constants
const (
	panelSidebar  = 0
	panelSummary  = 1
	panelDetails  = 2
	panelChat     = 3
	panelTimeline = 4
	panelPreview  = 5
)

// Tab constants
const (
	tabTimeline = 0
	tabCompose  = 1
	tabCleanup  = 2
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

// TimelineLoadedMsg carries emails sorted by date for the timeline tab
type TimelineLoadedMsg struct {
	Emails []*models.EmailData
}

// ComposeStatusMsg carries the result of a send attempt
type ComposeStatusMsg struct {
	Message string
	Err     error
}

// ClassifyProgressMsg carries a single classification result
type ClassifyProgressMsg struct {
	MessageID string
	Category  string
	Done      int
	Total     int
}

// ClassifyDoneMsg signals classification is complete
type ClassifyDoneMsg struct{}

// EmailBodyMsg carries the result of fetching an email body from IMAP
type EmailBodyMsg struct {
	Body *models.EmailBody
	Err  error
}

// ChatResponseMsg carries an Ollama chat reply
type ChatResponseMsg struct {
	Content string
	Err     error
}

// SearchResultMsg carries search results back to the UI
type SearchResultMsg struct {
	Emails []*models.EmailData
	Query  string
	Source string // "local", "fts", "imap", "semantic"
}

// NewEmailsMsg signals new emails arrived via IDLE/polling
type NewEmailsMsg struct {
	Emails []*models.EmailData
	Folder string
}

// EmailExpungedMsg signals an email was deleted on the server
type EmailExpungedMsg struct {
	MessageID string
	Folder    string
}

// SyncTickMsg drives the sync countdown timer
type SyncTickMsg struct{}

// EmbeddingProgressMsg reports background embedding progress
type EmbeddingProgressMsg struct {
	Remaining int
}

// EmbeddingDoneMsg signals background embedding finished
type EmbeddingDoneMsg struct{}

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

	// Classification channel (buffered; one result per email)
	classifyCh chan ClassifyProgressMsg

	// Data
	stats          map[string]*models.SenderStats
	emailsBySender map[string][]*models.EmailData

	// Tables
	summaryTable  table.Model
	detailsTable  table.Model
	timelineTable table.Model
	logViewer     *LogViewer

	// Display options
	groupByDomain        bool
	currentFolder        string
	selectedSender       string
	selectedRows         map[int]bool        // Selected rows in summary table
	selectedMessages     map[string]bool     // Selected messages by MessageID (across all senders)
	rowToSender          map[int]string      // Maps row index to original sender (before sanitization)
	detailsEmails        []*models.EmailData // Current emails shown in details table
	timelineEmails       []*models.EmailData // All emails sorted by date for timeline tab
	timelineSenderWidth  int
	timelineSubjectWidth int
	sidebarTooWide       bool // set by layout when sidebar + terminal width leaves < 16 variable cols

	// Thread grouping (timeline tab)
	threadGroups    []threadGroup
	threadRowMap    []timelineRowRef // maps table cursor index → row descriptor
	expandedThreads map[string]bool  // normalised subject → expanded?

	// Tabs
	activeTab int // tabCleanup, tabTimeline, or tabCompose

	// Email body preview (timeline tab)
	selectedTimelineEmail *models.EmailData
	emailBody             *models.EmailBody
	emailBodyLoading      bool
	emailPreviewWidth     int // computed in updateTableDimensions
	// Cached wrapped body lines — invalidated when body or panel width changes.
	bodyWrappedLines  []string
	bodyWrappedWidth  int
	bodyScrollOffset  int // first visible line in preview body

	// Chat panel
	showChat          bool
	chatMessages      []ai.ChatMessage // conversation history
	chatWrappedLines  [][]string       // cached wrapText output per message; nil = invalid
	chatWrappedWidth  int              // width at which chatWrappedLines was built
	chatInput         textinput.Model
	chatWaiting       bool // waiting for Ollama response

	// AI classification
	classifier      *ai.Classifier
	classifying     bool
	classifications map[string]string // messageID → category
	classifyTotal   int
	classifyDone    int

	// Compose
	mailer          *appsmtp.Client
	fromAddress     string
	composeTo       textinput.Model
	composeSubject  textinput.Model
	composeBody     textarea.Model
	composeField    int    // 0=To, 1=Subject, 2=Body
	composeStatus   string // last send result message
	composePreview  bool   // show glamour markdown preview

	// Sidebar
	folders       []string
	folderTree    []*folderNode
	folderStatus  map[string]models.FolderStatus
	showSidebar   bool
	sidebarCursor int
	focusedPanel  int // panelSidebar, panelSummary, panelDetails

	// Deletion confirmation
	pendingDeleteConfirm bool
	pendingDeleteDesc    string
	pendingDeleteAction  func() tea.Cmd
	pendingArchive       bool // true = archive, false = delete

	// Search
	searchMode          bool
	searchInput         textinput.Model
	searchResults       []*models.EmailData // nil = not in search mode
	timelineEmailsCache []*models.EmailData // full list before search

	// IMAP IDLE / background sync
	syncStatusMode  string // "idle", "polling", "off"
	syncCountdown   int    // seconds until next poll

	// Background embedding
	embeddingPending int

	// Styles
	baseStyle          lipgloss.Style
	headerStyle        lipgloss.Style
	loadingStyle       lipgloss.Style
	progressStyle      lipgloss.Style
	activeTableStyle   table.Styles
	inactiveTableStyle table.Styles
}

// New creates a new application model backed by the given Backend.
func New(b backend.Backend, mailer *appsmtp.Client, fromAddress string, classifier *ai.Classifier) *Model {
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

	summaryTable.SetStyles(inactiveStyle)
	detailsTable.SetStyles(inactiveStyle)

	// Timeline table: full-width chronological email list
	timelineTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Sender", Width: 20},
			{Title: "Subject", Width: 40},
			{Title: "Date", Width: 16},
			{Title: "Size KB", Width: 7},
			{Title: "Att", Width: 3},
			{Title: "Tag", Width: 4},
		}),
		table.WithHeight(11),
	)
	timelineTable.SetStyles(activeStyle)
	timelineTable.Focus()

	// Create log viewer
	logViewer := NewLogViewer(140, 15)

	// Set up log callback to capture logs in TUI
	logger.SetLogCallback(func(level, message string) {
		logViewer.AddLog(level, message)
	})

	// Chat input
	chatInput := textinput.New()
	chatInput.Placeholder = "Ask about your emails..."
	chatInput.CharLimit = 512

	// Compose inputs
	composeTo := textinput.New()
	composeTo.Placeholder = "recipient@example.com"
	composeTo.CharLimit = 256
	composeTo.Focus()

	composeSubject := textinput.New()
	composeSubject.Placeholder = "Subject"
	composeSubject.CharLimit = 512

	composeBody := textarea.New()
	composeBody.Placeholder = "Write your message here (Markdown supported)..."
	composeBody.SetWidth(80)
	composeBody.SetHeight(15)
	composeBody.CharLimit = 0 // unlimited

	// Create deletion channels
	deletionRequestCh := make(chan models.DeletionRequest, 10)
	deletionResultCh := make(chan models.DeletionResult, 10)

	searchInput := textinput.New()
	searchInput.Placeholder = "Search emails... (/b body  /* all folders  ? semantic)"
	searchInput.CharLimit = 200

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
		focusedPanel:       panelTimeline,
		selectedRows:       make(map[int]bool),
		selectedMessages:   make(map[string]bool),
		rowToSender:        make(map[int]string),
		summaryTable:       summaryTable,
		detailsTable:       detailsTable,
		timelineTable:      timelineTable,
		logViewer:          logViewer,
		chatInput:          chatInput,
		classifier:         classifier,
		classifications:    make(map[string]string),
		expandedThreads:    make(map[string]bool),
		classifyCh:         make(chan ClassifyProgressMsg, 50),
		mailer:             mailer,
		fromAddress:        fromAddress,
		composeTo:          composeTo,
		composeSubject:     composeSubject,
		composeBody:        composeBody,
		baseStyle:          baseStyle,
		headerStyle:        headerStyle,
		loadingStyle:       loadingStyle,
		progressStyle:      progressStyle,
		activeTableStyle:   activeStyle,
		inactiveTableStyle: inactiveStyle,
		deletionRequestCh:  deletionRequestCh,
		deletionResultCh:   deletionResultCh,
		searchInput:        searchInput,
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

	case TimelineLoadedMsg:
		m.timelineEmails = msg.Emails
		m.updateTimelineTable()
		return m, nil

	case ComposeStatusMsg:
		m.composeStatus = msg.Message
		if msg.Err == nil && msg.Message != "" {
			// Clear the compose fields on success
			m.composeTo.SetValue("")
			m.composeSubject.SetValue("")
			m.composeBody.SetValue("")
		}
		return m, nil

	case ClassifyProgressMsg:
		m.classifyDone = msg.Done
		m.classifyTotal = msg.Total
		if m.classifications == nil {
			m.classifications = make(map[string]string)
		}
		m.classifications[msg.MessageID] = msg.Category
		// Refresh tables to show updated tags
		m.updateTimelineTable()
		return m, m.listenForClassification()

	case ClassifyDoneMsg:
		m.classifying = false
		m.updateTimelineTable()
		m.updateSummaryTable()
		logger.Info("Classification complete: %d emails tagged", m.classifyDone)
		return m, nil

	case EmailBodyMsg:
		m.emailBodyLoading = false
		if msg.Err != nil {
			logger.Warn("Failed to fetch email body: %v", msg.Err)
			m.emailBody = &models.EmailBody{TextPlain: "(Failed to load body)"}
		} else {
			m.emailBody = msg.Body
			// Cache body text for FTS and embedding (fire-and-forget)
			if msg.Body != nil && msg.Body.TextPlain != "" && m.selectedTimelineEmail != nil {
				msgID := m.selectedTimelineEmail.MessageID
				bodyText := msg.Body.TextPlain
				go func() {
					if err := m.backend.CacheBodyText(msgID, bodyText); err != nil {
						logger.Warn("Failed to cache body text: %v", err)
					}
				}()
			}
		}
		m.bodyWrappedLines = nil // invalidate wrap cache
		return m, nil

	case ChatResponseMsg:
		m.chatWaiting = false
		content := msg.Content
		if msg.Err != nil {
			content = "Error: " + msg.Err.Error()
		}
		m.chatMessages = append(m.chatMessages, ai.ChatMessage{
			Role:    "assistant",
			Content: content,
		})
		m.chatWrappedLines = nil // invalidate wrap cache
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
			m.loadClassifications()
			m.updateSummaryTable()
			m.summaryTable.GotoTop()
			m.updateDetailsTable()
			listFoldersCmd := func() tea.Msg {
				folders, err := m.backend.ListFolders()
				if err != nil {
					logger.Warn("Failed to list folders: %v", err)
					return FoldersLoadedMsg{Folders: []string{"INBOX"}}
				}
				return FoldersLoadedMsg{Folders: folders}
			}
			// Start background polling (default 60s interval)
			pollCmd := m.startPolling(60)
			// Always load timeline since it's the default startup tab
			return m, tea.Batch(listFoldersCmd, m.loadTimelineEmails(), pollCmd)
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
			// Also refresh timeline and sidebar folder counts
			folders := m.folders
			refreshCounts := func() tea.Msg {
				status, err := m.backend.GetFolderStatus(folders)
				if err != nil {
					logger.Warn("Failed to refresh folder status: %v", err)
					return FolderStatusMsg{Status: map[string]models.FolderStatus{}}
				}
				return FolderStatusMsg{Status: status}
			}
			return m, tea.Batch(m.loadTimelineEmails(), refreshCounts)
		}

		// Continue listening for more results
		return m, m.listenForDeletionResults()

	case SearchResultMsg:
		if msg.Query == "" {
			// Empty query = clear search
			m.searchResults = nil
			if m.timelineEmailsCache != nil {
				m.timelineEmails = m.timelineEmailsCache
				m.timelineEmailsCache = nil
			}
			m.updateTimelineTable()
		} else {
			m.searchResults = msg.Emails
			m.updateTimelineTable()
		}
		return m, nil

	case NewEmailsMsg:
		if msg.Folder == m.currentFolder {
			// Prepend new emails to visible list
			m.timelineEmails = append(msg.Emails, m.timelineEmails...)
			if m.timelineEmailsCache != nil {
				m.timelineEmailsCache = append(msg.Emails, m.timelineEmailsCache...)
			}
			m.updateTimelineTable()
		}
		return m, m.listenForNewEmails()

	case EmailExpungedMsg:
		if msg.Folder == m.currentFolder {
			filtered := m.timelineEmails[:0]
			for _, e := range m.timelineEmails {
				if e.MessageID != msg.MessageID {
					filtered = append(filtered, e)
				}
			}
			m.timelineEmails = filtered
			if m.timelineEmailsCache != nil {
				filtered2 := m.timelineEmailsCache[:0]
				for _, e := range m.timelineEmailsCache {
					if e.MessageID != msg.MessageID {
						filtered2 = append(filtered2, e)
					}
				}
				m.timelineEmailsCache = filtered2
			}
			m.updateTimelineTable()
		}
		return m, m.listenForExpunged()

	case SyncTickMsg:
		if m.syncCountdown > 0 {
			m.syncCountdown--
		}
		return m, m.tickSyncCountdown()

	case EmbeddingProgressMsg:
		m.embeddingPending = msg.Remaining
		if msg.Remaining > 0 {
			return m, m.runEmbeddingBatch()
		}
		return m, nil

	case EmbeddingDoneMsg:
		m.embeddingPending = 0
		return m, nil

	case tea.WindowSizeMsg:
		m.updateTableDimensions(msg.Width, msg.Height)
		m.chatWrappedLines = nil // invalidate on resize
		return m, nil

	case tickMsg:
		if m.loading {
			m.loadingSpinner = (m.loadingSpinner + 1) % len(spinnerChars)
			return m, m.tickSpinner()
		}
		return m, nil
	}

	// Route messages to chat input when chat panel is focused
	if m.focusedPanel == panelChat && m.showChat {
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Route all messages to compose inputs when on compose tab
	if m.activeTab == tabCompose {
		var cmd tea.Cmd
		switch m.composeField {
		case 0:
			m.composeTo, cmd = m.composeTo.Update(msg)
		case 1:
			m.composeSubject, cmd = m.composeSubject.Update(msg)
		case 2:
			m.composeBody, cmd = m.composeBody.Update(msg)
		}
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Update tables and log viewer
	var cmd tea.Cmd
	m.summaryTable, cmd = m.summaryTable.Update(msg)
	cmds = append(cmds, cmd)

	m.detailsTable, cmd = m.detailsTable.Update(msg)
	cmds = append(cmds, cmd)

	m.timelineTable, cmd = m.timelineTable.Update(msg)
	cmds = append(cmds, cmd)

	// Update log viewer
	_, cmd = m.logViewer.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
const minTermWidth = 60
const minTermHeight = 15

func (m *Model) View() string {
	if m.windowWidth > 0 && m.windowWidth < minTermWidth {
		return fmt.Sprintf("\n  Terminal too narrow (%d cols). Please resize to at least %d columns.", m.windowWidth, minTermWidth)
	}
	if m.windowHeight > 0 && m.windowHeight < minTermHeight {
		return fmt.Sprintf("\n  Terminal too short (%d rows). Please resize to at least %d rows.", m.windowHeight, minTermHeight)
	}
	if m.loading {
		return m.renderLoadingView()
	}
	return m.renderMainView()
}

// handleKeyMsg handles keyboard input
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Deletion/archive confirmation prompt intercepts all keys
	if m.pendingDeleteConfirm {
		switch msg.String() {
		case "y", "Y":
			m.pendingDeleteConfirm = false
			action := m.pendingDeleteAction
			m.pendingDeleteAction = nil
			m.pendingDeleteDesc = ""
			if action != nil {
				return m, action()
			}
		case "n", "N", "esc":
			m.pendingDeleteConfirm = false
			m.pendingDeleteAction = nil
			m.pendingDeleteDesc = ""
		}
		return m, nil
	}

	// Search mode intercepts input when active
	if m.searchMode && m.activeTab == tabTimeline {
		switch msg.String() {
		case "esc":
			m.searchMode = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			m.searchResults = nil
			if m.timelineEmailsCache != nil {
				m.timelineEmails = m.timelineEmailsCache
				m.timelineEmailsCache = nil
			}
			m.updateTimelineTable()
			return m, nil
		case "ctrl+s":
			if q := m.searchInput.Value(); q != "" {
				return m, m.saveCurrentSearch(q)
			}
		case "ctrl+i":
			return m, m.performIMAPSearch(m.searchInput.Value())
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, tea.Batch(cmd, m.performSearch(m.searchInput.Value()))
		}
		return m, nil
	}

	// Global quit always works
	if msg.String() == "q" || msg.String() == "ctrl+c" {
		m.cleanup()
		return m, tea.Quit
	}

	// Chat panel intercepts Enter/Esc when focused
	if m.focusedPanel == panelChat && m.showChat {
		switch msg.String() {
		case "enter":
			if !m.chatWaiting {
				return m, m.submitChat()
			}
		case "esc", "tab":
			m.chatInput.Blur()
			m.setFocusedPanel(panelSummary)
		}
		return m, nil
	}

	// Compose tab gets its own key handler
	if m.activeTab == tabCompose {
		return m.handleComposeKey(msg)
	}

	switch msg.String() {
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

	case "1":
		if !m.loading && m.activeTab != tabTimeline {
			m.activeTab = tabTimeline
			m.setFocusedPanel(panelTimeline)
			return m, m.loadTimelineEmails()
		}
		return m, nil

	case "2":
		if !m.loading && m.activeTab != tabCompose {
			m.activeTab = tabCompose
			m.timelineTable.Blur()
			m.summaryTable.Blur()
			m.detailsTable.Blur()
			m.composeField = 0
			m.composeTo.Focus()
			m.composeSubject.Blur()
			m.composeBody.Blur()
		}
		return m, nil

	case "3":
		if !m.loading && m.activeTab != tabCleanup {
			m.activeTab = tabCleanup
			m.setFocusedPanel(panelSummary)
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
		if !m.loading && !m.deleting && !m.pendingDeleteConfirm {
			desc := m.buildDeleteDesc()
			if desc != "" {
				m.pendingDeleteConfirm = true
				m.pendingDeleteDesc = desc
				m.pendingArchive = false
				m.pendingDeleteAction = func() tea.Cmd {
					m.deleting = true
					return m.deleteSelected()
				}
			}
		}
		return m, nil

	case "e":
		if !m.loading && !m.deleting && !m.pendingDeleteConfirm {
			desc := m.buildArchiveDesc()
			if desc != "" {
				m.pendingDeleteConfirm = true
				m.pendingDeleteDesc = desc
				m.pendingArchive = true
				m.pendingDeleteAction = func() tea.Cmd {
					m.deleting = true
					return m.archiveSelected()
				}
			}
		}
		return m, nil

	case "F":
		if !m.loading && m.activeTab == tabTimeline {
			cursor := m.timelineTable.Cursor()
			if cursor < len(m.threadRowMap) {
				ref := m.threadRowMap[cursor]
				var email *models.EmailData
				if ref.kind == rowKindThread {
					email = ref.group.emails[0]
				} else {
					email = ref.group.emails[ref.emailIdx]
				}
				subject := email.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "fwd:") {
					subject = "Fwd: " + subject
				}
				fwdBody := fmt.Sprintf("\n\n--- Forwarded message ---\nFrom: %s\nDate: %s\nSubject: %s\n\n",
					email.Sender, email.Date.Format("Mon, 02 Jan 2006 15:04"), email.Subject)
				if m.emailBody != nil && m.selectedTimelineEmail != nil &&
					m.selectedTimelineEmail.MessageID == email.MessageID {
					fwdBody += m.emailBody.TextPlain
				}
				m.activeTab = tabCompose
				m.composeTo.SetValue("")
				m.composeSubject.SetValue(subject)
				m.composeBody.SetValue(fwdBody)
				m.composeField = 0
				m.composeTo.Focus()
				m.composeSubject.Blur()
				m.composeBody.Blur()
			}
		}
		return m, nil

	case "/":
		if !m.loading && m.activeTab == tabTimeline && !m.searchMode {
			m.searchMode = true
			if m.timelineEmailsCache == nil {
				m.timelineEmailsCache = m.timelineEmails
			}
			m.searchInput.SetValue("")
			m.searchInput.Focus()
		}
		return m, nil

	case "enter":
		if !m.loading {
			if m.focusedPanel == panelSidebar {
				m.selectSidebarFolder()
				return m, tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForProgress())
			} else if m.activeTab == tabTimeline {
				cursor := m.timelineTable.Cursor()
				if cursor < len(m.threadRowMap) {
					ref := m.threadRowMap[cursor]
					if ref.kind == rowKindThread {
						key := ref.group.normalizedSubject
						savedCursor := m.timelineTable.Cursor()
						m.expandedThreads[key] = !m.expandedThreads[key]
						m.updateTimelineTable()
						m.timelineTable.SetCursor(savedCursor)
						return m, nil
					}
					// First row of an expanded thread: collapse on Enter instead of opening preview
					if ref.emailIdx == 0 && len(ref.group.emails) > 1 && m.expandedThreads[ref.group.normalizedSubject] {
						savedCursor := m.timelineTable.Cursor()
						m.expandedThreads[ref.group.normalizedSubject] = false
						m.updateTimelineTable()
						m.timelineTable.SetCursor(savedCursor)
						return m, nil
					}
					email := ref.group.emails[ref.emailIdx]
					m.selectedTimelineEmail = email
					m.emailBody = nil
					m.emailBodyLoading = true
					m.bodyScrollOffset = 0
					m.updateTableDimensions(m.windowWidth, m.windowHeight)
					return m, m.loadEmailBodyCmd(email.Folder, email.UID)
				}
			} else if m.focusedPanel == panelSummary {
				m.updateDetailsTable()
			}
		}
		return m, nil

	case "esc":
		if m.activeTab == tabTimeline && m.selectedTimelineEmail != nil {
			m.selectedTimelineEmail = nil
			m.emailBody = nil
			m.emailBodyLoading = false
			m.bodyWrappedLines = nil
			m.bodyScrollOffset = 0
			m.setFocusedPanel(panelTimeline)
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
		}
		return m, nil

	case "tab":
		if !m.loading {
			m.cyclePanel(true)
		}
		return m, nil

	case "shift+tab":
		if !m.loading {
			m.cyclePanel(false)
		}
		return m, nil

	case "l", "L":
		if !m.loading {
			m.showLogs = !m.showLogs
		}
		return m, nil

	case "c":
		// Toggle chat panel
		if !m.loading {
			m.showChat = !m.showChat
			if m.windowWidth > 0 {
				m.updateTableDimensions(m.windowWidth, m.windowHeight)
			}
			if m.showChat {
				m.focusedPanel = panelChat
				m.chatInput.Focus()
				m.summaryTable.Blur()
				m.detailsTable.Blur()
			} else {
				m.chatInput.Blur()
				m.setFocusedPanel(panelSummary)
			}
		}
		return m, nil

	case "a":
		// Trigger AI classification for current folder.
		// A fresh channel is allocated on each run so startClassification can
		// safely close it when done (unblocking listenForClassification) without
		// risking a send-on-closed-channel on any subsequent 'a' press.
		if !m.loading && !m.classifying && m.classifier != nil {
			m.classifying = true
			m.classifyDone = 0
			m.classifyTotal = 0
			m.classifyCh = make(chan ClassifyProgressMsg, 50)
			return m, tea.Batch(m.startClassification(), m.listenForClassification())
		}
		return m, nil

	case "R":
		// Reply: open compose pre-filled from selected timeline email
		if !m.loading && m.activeTab == tabTimeline {
			cursor := m.timelineTable.Cursor()
			if cursor < len(m.threadRowMap) {
				ref := m.threadRowMap[cursor]
				var email *models.EmailData
				if ref.kind == rowKindThread {
					email = ref.group.emails[0]
				} else {
					email = ref.group.emails[ref.emailIdx]
				}
				m.activeTab = tabCompose
				m.composeTo.SetValue(email.Sender)
				subject := email.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}
				m.composeSubject.SetValue(subject)
				m.composeField = 2
				m.composeTo.Blur()
				m.composeSubject.Blur()
				m.composeBody.Focus()
			}
		}
		return m, nil

	case "up", "k":
		if !m.loading {
			if m.activeTab == tabTimeline {
				if m.focusedPanel == panelPreview {
					if m.bodyScrollOffset > 0 {
						m.bodyScrollOffset--
					}
				} else if m.focusedPanel == panelSidebar {
					return m.handleNavigation(-1)
				} else {
					var cmd tea.Cmd
					m.timelineTable, cmd = m.timelineTable.Update(tea.KeyMsg{Type: tea.KeyUp})
					return m, cmd
				}
			} else {
				return m.handleNavigation(-1)
			}
		}
		return m, nil

	case "down", "j":
		if !m.loading {
			if m.activeTab == tabTimeline {
				if m.focusedPanel == panelPreview {
					m.bodyScrollOffset++
				} else if m.focusedPanel == panelSidebar {
					return m.handleNavigation(1)
				} else {
					var cmd tea.Cmd
					m.timelineTable, cmd = m.timelineTable.Update(tea.KeyMsg{Type: tea.KeyDown})
					return m, cmd
				}
			} else {
				return m.handleNavigation(1)
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
	content.WriteString(m.headerStyle.Render("ProtonMail Analyzer") + "\n")

	// Tab bar
	content.WriteString(m.renderTabBar() + "\n\n")

	// Content area
	var mainContent string
	if m.showLogs {
		mainContent = m.baseStyle.Render(m.logViewer.View())
	} else if m.activeTab == tabTimeline {
		mainContent = m.renderTimelineView()
	} else if m.activeTab == tabCompose {
		mainContent = m.renderComposeView()
	} else {
		// Cleanup tab
		var summaryView string
		if m.stats != nil && len(m.stats) == 0 {
			summaryView = m.emptyStateView("No emails in this folder  •  press r to refresh")
		} else {
			summaryView = m.baseStyle.Render(m.summaryTable.View())
		}
		detailsView := m.baseStyle.Render(m.detailsTable.View())
		if m.showSidebar && !m.sidebarTooWide {
			sidebarView := m.baseStyle.Render(m.renderSidebar())
			mainContent = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, "  ", summaryView, "  ", detailsView)
		} else {
			mainContent = lipgloss.JoinHorizontal(lipgloss.Top, summaryView, "  ", detailsView)
		}
	}
	if m.showChat {
		chatView := m.baseStyle.Render(m.renderChatPanel())
		content.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, mainContent, "  ", chatView) + "\n")
	} else {
		content.WriteString(mainContent + "\n")
	}

	// Status bar + key hints (persistent bottom bar)
	content.WriteString(m.renderStatusBar() + "\n")
	content.WriteString(m.renderKeyHints())

	return content.String()
}

// Helper functions and other methods continue in next part due to length...
