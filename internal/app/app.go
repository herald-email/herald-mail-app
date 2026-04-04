package app

import (
	"context"
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
	"mail-processor/internal/cleanup"
	"mail-processor/internal/config"
	"mail-processor/internal/iterm2"
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
	tabTimeline  = 0
	tabCompose   = 1
	tabCleanup   = 2
	tabContacts  = 3
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

// ReclassifyResultMsg carries the result of re-classifying a single email.
type ReclassifyResultMsg struct {
	MessageID string
	Category  string
	Err       error
}

// ValidIDsMsg is sent when background reconciliation has determined the live
// set of valid message IDs from the server. All views should re-filter.
type ValidIDsMsg struct {
	ValidIDs map[string]bool
}

// EmailBodyMsg carries the result of fetching an email body from IMAP
type EmailBodyMsg struct {
	Body *models.EmailBody
	Err  error
}

// QuickRepliesMsg is sent when AI quick reply generation completes.
type QuickRepliesMsg struct {
	Replies []string
	Err     error
}

// CleanupEmailBodyMsg carries the result of fetching an email body from the Cleanup tab
type CleanupEmailBodyMsg struct {
	Body *models.EmailBody
	Err  error
}

// ChatResponseMsg carries an Ollama chat reply
type ChatResponseMsg struct {
	Content string
	Err     error
}

// ChatFilterActivatedMsg is sent when the chat response contains a <filter> block.
type ChatFilterActivatedMsg struct {
	Emails []*models.EmailData
	Label  string
}

// SearchResultMsg carries search results back to the UI
type SearchResultMsg struct {
	Emails []*models.EmailData
	Scores map[string]float64 // messageID → similarity score; non-nil only for semantic results
	Query  string
	Source string // "local", "fts", "imap", "semantic"
	Err    error  // non-nil when the search failed with a user-visible error
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
	Done  int
	Total int
}

// EmbeddingDoneMsg signals background embedding finished
type EmbeddingDoneMsg struct{}

// AttachmentSavedMsg signals an attachment save completed
type AttachmentSavedMsg struct {
	Filename string
	Path     string
	Err      error
}

// AttachmentAddedMsg signals a compose attachment was loaded from disk
type AttachmentAddedMsg struct {
	Attachment models.ComposeAttachment
	Err        error
}

// UnsubscribeResultMsg carries the result of an unsubscribe attempt
type UnsubscribeResultMsg struct {
	Method string // "one-click", "browser-opened", "url-copied", "mailto-copied"
	URL    string
	Err    error
}

// RuleResultMsg carries the result of processing a single email through the rule engine
type RuleResultMsg struct{ Result models.RuleResult }

// AutoClassifyResultMsg carries the result of auto-classifying a newly arrived email.
type AutoClassifyResultMsg struct {
	MessageID string
	Category  string
	Err       error
}

// SoftUnsubResultMsg carries the result of a soft-unsubscribe attempt
type SoftUnsubResultMsg struct {
	Sender string
	Err    error
}

// ImageDescMsg carries an AI-generated description for a single inline image
type ImageDescMsg struct {
	ContentID   string
	Description string
	Err         error
}

// ContactEnrichedMsg signals that a batch of contacts was enriched with company/topics/embedding.
type ContactEnrichedMsg struct{ Count int }

// ContactsLoadedMsg carries the full contact list for the Contacts tab.
type ContactsLoadedMsg struct{ Contacts []models.ContactData }

// ContactDetailLoadedMsg carries recent emails for the selected contact detail panel.
type ContactDetailLoadedMsg struct{ Emails []*models.EmailData }

type AppleContactsImportedMsg struct{ Count int }

// StarResultMsg is returned after a star/unstar operation completes.
type StarResultMsg struct {
	MessageID string
	Starred   bool
	Err       error
}

// DraftSaveTickMsg fires every 30 seconds to trigger auto-save of compose draft.
type DraftSaveTickMsg struct{}

// DraftSavedMsg is returned after a SaveDraft call completes.
type DraftSavedMsg struct {
	UID    uint32
	Folder string
	Err    error
}

// DraftDeletedMsg is returned after a DeleteDraft call completes.
type DraftDeletedMsg struct {
	Err error
}

// ContactSuggestionsMsg carries autocomplete results for the compose address fields.
type ContactSuggestionsMsg struct {
	Contacts []models.ContactData
}

// AIAssistMsg carries the result of an AI body-rewrite request.
type AIAssistMsg struct {
	Result string
	Err    error
}

// AISubjectMsg carries the result of an AI subject-suggestion request.
type AISubjectMsg struct {
	Subject string
	Err     error
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

	// Rule engine channels
	ruleRequestCh  chan models.RuleRequest
	ruleResultCh   chan models.RuleResult
	rulesFiredCount int // total rules fired across session

	// Classification channel (buffered; one result per email)
	classifyCh chan ClassifyProgressMsg

	// validIDsCh receives the live valid-ID set from background reconciliation.
	validIDsCh <-chan map[string]bool

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
	inlineImageDescs      map[string]string // ContentID → AI description (vision fallback)
	emailPreviewWidth     int // computed in updateTableDimensions
	emailFullScreen       bool
	// Cached wrapped body lines — invalidated when body or panel width changes.
	bodyWrappedLines  []string
	bodyWrappedWidth  int
	bodyScrollOffset  int // first visible line in preview body

	// Quick replies
	quickReplies      []string // canned (first 5) + AI-generated (up to 3)
	quickRepliesReady bool     // true once AI has finished generating
	quickReplyOpen    bool     // picker overlay is visible
	quickReplyIdx     int      // currently highlighted item

	// Email body preview (cleanup tab)
	showCleanupPreview      bool
	cleanupPreviewEmail     *models.EmailData
	cleanupEmailBody        *models.EmailBody
	cleanupBodyLoading      bool
	cleanupBodyScrollOffset int
	cleanupBodyWrappedLines []string
	cleanupBodyWrappedWidth int
	cleanupPreviewWidth     int // computed in updateTableDimensions
	cleanupPreviewHadSidebar bool
	cleanupFullScreen         bool // true = preview takes entire screen
	cleanupPreviewDeleting    bool // true = deletion/archive was triggered from preview
	cleanupPreviewIsArchive   bool // true = the preview action was archive (not delete)

	// Attachment save prompt (receive side)
	selectedAttachment   int
	attachmentSavePrompt bool
	attachmentSaveInput  textinput.Model

	// Chat panel
	showChat          bool
	chatMessages      []ai.ChatMessage // conversation history
	chatWrappedLines  [][]string       // cached wrapText output per message; nil = invalid
	chatWrappedWidth  int              // width at which chatWrappedLines was built
	chatInput         textinput.Model
	chatWaiting       bool // waiting for Ollama response

	// Chat filter (timeline filtering driven by AI chat <filter> blocks)
	chatFilterMode     bool
	chatFilteredEmails []*models.EmailData
	chatFilterLabel    string

	// AI classification
	classifier      ai.AIClient
	classifying     bool
	classifications map[string]string // messageID → category
	classifyTotal   int
	classifyDone    int

	// Compose
	mailer               *appsmtp.Client
	fromAddress          string
	composeTo            textinput.Model
	composeCC            textinput.Model
	composeBCC           textinput.Model
	composeSubject       textinput.Model
	composeBody          textarea.Model
	composeField         int    // 0=To, 1=CC, 2=BCC, 3=Subject, 4=Body
	composeStatus        string // last send result message
	composePreview       bool   // show glamour markdown preview
	composeAttachments   []models.ComposeAttachment

	// Autocomplete (compose address fields)
	suggestions   []models.ContactData // current autocomplete candidates (empty = dropdown hidden)
	suggestionIdx int                  // selected row index (-1 = none selected)

	// Compose AI panel (Ctrl+G)
	composeAIPanel       bool
	composeAIInput       textinput.Model // free-form prompt
	composeAIResponse    textarea.Model  // editable AI rewrite
	composeAIDiff        string          // lipgloss-styled word diff (display only)
	composeAILoading     bool
	composeAIThread      bool              // true = include reply context in prompt
	composeAISubjectHint string            // pending subject suggestion ("" = none)
	replyContextEmail    *models.EmailData // set when reply is initiated; nil for new emails
	attachmentPathInput  textinput.Model
	attachmentInputActive bool

	// Draft auto-save state
	lastDraftUID    uint32 // UID of last auto-saved draft; 0 = not saved yet
	lastDraftFolder string // folder of last auto-saved draft
	draftSaving     bool   // true while a SaveDraft cmd is in flight (prevents concurrent saves)

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

	// Unsubscribe confirmation
	pendingUnsubscribe       bool
	pendingUnsubscribeDesc   string
	pendingUnsubscribeAction func() tea.Cmd

	// 3-way unsubscribe choice (Cleanup tab)
	unsubConfirmMode   bool
	unsubConfirmSender string

	// Search
	searchMode          bool
	searchInput         textinput.Model
	searchResults       []*models.EmailData // nil = not in search mode
	timelineEmailsCache []*models.EmailData // full list before search
	semanticScores      map[string]float64  // messageID → similarity score (populated during semantic search)
	searchError         string              // user-visible error from last search attempt

	// IMAP IDLE / background sync
	syncStatusMode  string // "idle", "polling", "off"
	syncCountdown   int    // seconds until next poll

	// Background embedding
	embeddingDone  int
	embeddingTotal int

	// Text selection (preview panel)
	mouseMode   bool
	visualMode  bool
	visualStart int // first selected line index in bodyWrappedLines
	visualEnd   int // last selected line index (inclusive)
	pendingY    bool // first 'y' of 'yy' sequence

	// Demo mode — set when DemoBackend is detected; shows [DEMO] in status bar
	demoMode bool

	// Dry-run mode — log rule/cleanup actions without executing destructive ones
	dryRun bool
	// Contacts tab
	contactsList       []models.ContactData
	contactsFiltered   []models.ContactData
	contactsIdx        int
	contactSearch      string
	contactSearchMode  string // "" | "keyword" | "semantic"
	contactDetail      *models.ContactData
	contactDetailEmails []*models.EmailData
	contactDetailIdx   int
	contactFocusPanel  int // 0 = list, 1 = detail

	// Config
	cfg        *config.Config
	configPath string

	// Settings panel overlay
	showSettings  bool
	settingsPanel *Settings

	// Rule editor overlay
	showRuleEditor bool
	ruleEditor     *RuleEditor

	// Cleanup manager overlay
	showCleanupMgr   bool
	cleanupManager   *CleanupManager
	cleanupScheduler *cleanup.Scheduler

	// Prompt editor overlay
	showPromptEditor bool
	promptEditor     *PromptEditor

	// OAuth wait overlay (shown after Gmail is chosen in the S-key settings panel)
	oauthWait *OAuthWaitModel

	// General status message (shown briefly after actions like settings save)
	statusMessage string

	// Styles
	baseStyle          lipgloss.Style
	headerStyle        lipgloss.Style
	loadingStyle       lipgloss.Style
	progressStyle      lipgloss.Style
	activeTableStyle   table.Styles
	inactiveTableStyle table.Styles
}

// New creates a new application model backed by the given Backend.
func New(b backend.Backend, mailer *appsmtp.Client, fromAddress string, classifier ai.AIClient, dryRun bool) *Model {
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

	composeCC := textinput.New()
	composeCC.Placeholder = "cc@example.com, ..."
	composeCC.CharLimit = 512

	composeBCC := textinput.New()
	composeBCC.Placeholder = "bcc@example.com, ..."
	composeBCC.CharLimit = 512

	composeSubject := textinput.New()
	composeSubject.Placeholder = "Subject"
	composeSubject.CharLimit = 512

	composeBody := textarea.New()
	composeBody.Placeholder = "Write your message here (Markdown supported)..."
	composeBody.SetWidth(80)
	composeBody.SetHeight(15)
	composeBody.CharLimit = 0 // unlimited

	composeAIInput := textinput.New()
	composeAIInput.Placeholder = "Ask AI anything about this email…"
	composeAIInput.CharLimit = 512

	composeAIResponse := textarea.New()
	composeAIResponse.Placeholder = "AI suggestion will appear here…"
	composeAIResponse.SetWidth(38)
	composeAIResponse.SetHeight(8)
	composeAIResponse.CharLimit = 0

	// Create deletion channels
	deletionRequestCh := make(chan models.DeletionRequest, 10)
	deletionResultCh := make(chan models.DeletionResult, 10)

	// Create rule engine channels
	ruleRequestCh := make(chan models.RuleRequest, 20)
	ruleResultCh := make(chan models.RuleResult, 50)

	searchInput := textinput.New()
	searchInput.Placeholder = "Search emails... (/b body  /* all folders  ? semantic)"
	searchInput.CharLimit = 200

	attachmentSaveInput := textinput.New()
	attachmentSaveInput.Placeholder = "~/Downloads/"
	attachmentSaveInput.CharLimit = 512

	attachmentPathInput := textinput.New()
	attachmentPathInput.Placeholder = "Path to file (e.g. ~/Documents/report.pdf)"
	attachmentPathInput.CharLimit = 512

	m := &Model{
		backend:            b,
		progressCh:         b.Progress(),
		loading:            true,
		dryRun:             dryRun,
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
		composeCC:          composeCC,
		composeBCC:         composeBCC,
		composeSubject:     composeSubject,
		composeBody:        composeBody,
		suggestionIdx:      -1,
		composeAIInput:     composeAIInput,
		composeAIResponse:  composeAIResponse,
		baseStyle:          baseStyle,
		headerStyle:        headerStyle,
		loadingStyle:       loadingStyle,
		progressStyle:      progressStyle,
		activeTableStyle:   activeStyle,
		inactiveTableStyle: inactiveStyle,
		deletionRequestCh:   deletionRequestCh,
		deletionResultCh:    deletionResultCh,
		ruleRequestCh:       ruleRequestCh,
		ruleResultCh:        ruleResultCh,
		searchInput:         searchInput,
		attachmentSaveInput: attachmentSaveInput,
		attachmentPathInput: attachmentPathInput,
	}

	// Detect demo mode via DemoBackendMarker interface
	if marker, ok := b.(interface{ IsDemo() bool }); ok && marker.IsDemo() {
		m.demoMode = true
	}

	// Start deletion worker goroutine
	go m.deletionWorker()

	// Start rule engine worker goroutine
	go m.ruleWorker()

	return m
}

// SetConfigPath stores the resolved config file path so settings saves can
// persist changes back to disk.
func (m *Model) SetConfigPath(path string) {
	m.configPath = path
}

// SetConfig stores the loaded config so the settings panel can pre-fill fields.
func (m *Model) SetConfig(cfg *config.Config) {
	m.cfg = cfg
}

// SetCleanupScheduler wires a cleanup Scheduler into the model so that
// CleanupRunNowMsg actually executes the rules.
func (m *Model) SetCleanupScheduler(s *cleanup.Scheduler) {
	m.cleanupScheduler = s
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.startLoading(),
		m.tickSpinner(),
		m.listenForProgress(),
		m.listenForRuleResult(),
		m.importAppleContacts(),
		draftSaveTick(),
	)
}

// Update implements tea.Model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle rule editor messages before forwarding, so we can close it cleanly.
	switch msg := msg.(type) {
	case RuleEditorDoneMsg:
		m.showRuleEditor = false
		m.ruleEditor = nil
		if msg.Rule != nil {
			if len(msg.Rule.Actions) == 0 {
				m.statusMessage = "Rule not saved: no actions selected"
				return m, nil
			}
			if err := m.backend.SaveRule(msg.Rule); err != nil {
				m.statusMessage = "Error saving rule: " + err.Error()
			} else {
				m.statusMessage = "Rule created: " + msg.Rule.Name
			}
		}
		return m, nil

	case RuleEditorCancelledMsg:
		m.showRuleEditor = false
		m.ruleEditor = nil
		return m, nil

	case PromptEditorDoneMsg:
		m.showPromptEditor = false
		m.promptEditor = nil
		if msg.Prompt != nil {
			if err := m.backend.SaveCustomPrompt(msg.Prompt); err != nil {
				m.statusMessage = "Error saving prompt: " + err.Error()
			} else {
				m.statusMessage = "Prompt saved: " + msg.Prompt.Name
			}
		}
		return m, nil

	case PromptEditorCancelledMsg:
		m.showPromptEditor = false
		m.promptEditor = nil
		return m, nil
	}

	// Handle settings panel messages before forwarding to the panel, so we can
	// close it cleanly when it emits a completion message.
	switch msg := msg.(type) {
	case SettingsSavedMsg:
		m.cfg = msg.Config
		m.statusMessage = "Settings saved."
		if m.configPath != "" {
			if err := m.cfg.Save(m.configPath); err != nil {
				m.statusMessage = fmt.Sprintf("Settings saved (config write failed: %v)", err)
			}
		}
		m.showSettings = false
		m.settingsPanel = nil
		return m, nil

	case SettingsCancelledMsg:
		m.showSettings = false
		m.settingsPanel = nil
		return m, nil

	case OAuthRequiredMsg:
		// Gmail chosen in the settings panel — launch the OAuth wait overlay.
		m.showSettings = false
		m.settingsPanel = nil
		oauthModel, err := NewOAuthWaitModel(msg.Email, msg.Config, m.configPath)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Failed to start OAuth flow: %v", err)
			return m, nil
		}
		m.oauthWait = oauthModel
		return m, m.oauthWait.Init()

	case OAuthDoneMsg:
		m.oauthWait = nil
		m.cfg = msg.Config
		m.statusMessage = "Gmail account authorized. Reconnecting…"
		// TODO: trigger backend reconnect when reconnect API is available
		return m, nil

	case OAuthErrorMsg:
		m.oauthWait = nil
		m.statusMessage = fmt.Sprintf("OAuth failed: %v", msg.Err)
		return m, nil
	}

	// Forward all messages to the OAuth wait overlay when active.
	if m.oauthWait != nil {
		newModel, cmd := m.oauthWait.Update(msg)
		m.oauthWait = newModel.(*OAuthWaitModel)
		return m, cmd
	}

	// Forward all messages to the settings panel when it is active (intercepts
	// key presses and window-size events so the panel handles them exclusively).
	if m.showSettings && m.settingsPanel != nil {
		newModel, cmd := m.settingsPanel.Update(msg)
		m.settingsPanel = newModel.(*Settings)
		return m, cmd
	}

	// Forward all messages to the rule editor when it is active.
	if m.showRuleEditor && m.ruleEditor != nil {
		var ruleCmd tea.Cmd
		m.ruleEditor, ruleCmd = m.ruleEditor.Update(msg)
		return m, ruleCmd
	}

	// Forward all messages to the prompt editor when it is active.
	if m.showPromptEditor && m.promptEditor != nil {
		var promptCmd tea.Cmd
		m.promptEditor, promptCmd = m.promptEditor.Update(msg)
		return m, promptCmd
	}

	// Handle cleanup manager messages
	switch msg.(type) {
	case CleanupManagerOpenMsg:
		m.cleanupManager = NewCleanupManager(m.backend, m.windowWidth, m.windowHeight)
		m.showCleanupMgr = true
		return m, m.cleanupManager.Init()

	case CleanupManagerCloseMsg:
		m.showCleanupMgr = false
		m.cleanupManager = nil
		return m, nil

	case CleanupRunNowMsg:
		m.showCleanupMgr = false
		m.cleanupManager = nil
		m.statusMessage = "Running cleanup rules..."
		if m.cleanupScheduler != nil {
			return m, m.cleanupScheduler.RunNow(context.Background())
		}
		return m, nil

	case cleanup.CleanupDoneMsg:
		doneMsg := msg.(cleanup.CleanupDoneMsg)
		total := 0
		for _, n := range doneMsg.Results {
			total += n
		}
		m.statusMessage = fmt.Sprintf("Cleanup complete: %d email(s) processed", total)
		return m, nil
	}

	// Forward all messages to the cleanup manager when it is active.
	if m.showCleanupMgr && m.cleanupManager != nil {
		var cleanupCmd tea.Cmd
		m.cleanupManager, cleanupCmd = m.cleanupManager.Update(msg)
		return m, cleanupCmd
	}

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
		// If we navigated here from Contacts, scroll the cursor to the selected email.
		if m.selectedTimelineEmail != nil {
			targetID := m.selectedTimelineEmail.MessageID
			for rowIdx, ref := range m.threadRowMap {
				if ref.kind == rowKindEmail &&
					ref.group != nil &&
					ref.emailIdx < len(ref.group.emails) &&
					ref.group.emails[ref.emailIdx].MessageID == targetID {
					m.timelineTable.SetCursor(rowIdx)
					break
				}
			}
		}
		if m.classifier != nil {
			return m, tea.Batch(m.runEmbeddingBatch(), m.runContactEnrichment())
		}
		return m, nil

	case ComposeStatusMsg:
		m.composeStatus = msg.Message
		if msg.Err == nil && msg.Message != "" {
			// Clear the compose fields on success
			m.composeTo.SetValue("")
			m.composeSubject.SetValue("")
			m.composeBody.SetValue("")
			m.composeAttachments = nil
			// Clear reply/AI context after successful send
			m.replyContextEmail = nil
			m.composeAIThread = false
			m.composeAIPanel = false
			m.composeAIDiff = ""
			m.composeAISubjectHint = ""
			m.composeAIResponse.SetValue("")
			m.composeAILoading = false
			// Delete the auto-saved draft (if any) since the email was sent
			if m.lastDraftUID != 0 {
				cmd := m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder)
				m.lastDraftUID = 0
				m.lastDraftFolder = ""
				return m, cmd
			}
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
		// Push newly classified email to rule engine (non-blocking)
		for _, e := range m.timelineEmails {
			if e.MessageID == msg.MessageID {
				select {
				case m.ruleRequestCh <- models.RuleRequest{Email: e, Category: msg.Category}:
				default:
				}
				break
			}
		}
		return m, m.listenForClassification()

	case ClassifyDoneMsg:
		m.classifying = false
		m.updateTimelineTable()
		m.updateSummaryTable()
		logger.Info("Classification complete: %d emails tagged", m.classifyDone)
		return m, nil

	case ReclassifyResultMsg:
		if msg.Err != nil {
			m.statusMessage = "Reclassify failed: " + msg.Err.Error()
			return m, nil
		}
		if m.classifications == nil {
			m.classifications = make(map[string]string)
		}
		m.classifications[msg.MessageID] = msg.Category
		m.statusMessage = "Reclassified: " + msg.Category
		m.updateTimelineTable()
		m.updateSummaryTable()
		return m, nil

	case AttachmentSavedMsg:
		if msg.Err != nil {
			m.composeStatus = fmt.Sprintf("Save failed: %v", msg.Err)
		} else {
			m.composeStatus = fmt.Sprintf("Saved: %s", msg.Path)
		}
		return m, nil

	case AttachmentAddedMsg:
		if msg.Err != nil {
			m.composeStatus = fmt.Sprintf("Attach error: %v", msg.Err)
		} else {
			if msg.Attachment.Size > 10*1024*1024 {
				m.composeStatus = fmt.Sprintf("Warning: %s is %.1f MB (>10 MB)", msg.Attachment.Filename, float64(msg.Attachment.Size)/(1024*1024))
			}
			m.composeAttachments = append(m.composeAttachments, msg.Attachment)
		}
		return m, nil

	case UnsubscribeResultMsg:
		if msg.Err != nil {
			m.composeStatus = fmt.Sprintf("Unsubscribe failed: %v", msg.Err)
		} else {
			switch msg.Method {
			case "one-click":
				m.composeStatus = "Unsubscribed (one-click POST sent)"
			case "browser-opened":
				m.composeStatus = "Opened unsubscribe link in browser"
			case "url-copied":
				m.composeStatus = fmt.Sprintf("Unsubscribe URL copied to clipboard: %s", msg.URL)
			case "mailto-copied":
				m.composeStatus = fmt.Sprintf("Unsubscribe address copied to clipboard: %s", msg.URL)
			}
			// Persist the unsubscribe record so future arrivals from this sender can be flagged.
			if m.selectedTimelineEmail != nil {
				sender := m.selectedTimelineEmail.Sender
				method := msg.Method
				url := msg.URL
				go func() {
					_ = m.backend.RecordUnsubscribe(sender, method, url)
				}()
			}
		}
		return m, nil

	case SoftUnsubResultMsg:
		m.unsubConfirmMode = false
		if msg.Err != nil {
			m.statusMessage = "Error creating soft unsubscribe rule: " + msg.Err.Error()
		} else {
			m.statusMessage = "Soft unsubscribe rule created for " + msg.Sender
		}
		return m, nil

	case StarResultMsg:
		if msg.Err != nil {
			m.statusMessage = "Star failed: " + msg.Err.Error()
		} else {
			for _, e := range m.timelineEmails {
				if e.MessageID == msg.MessageID {
					e.IsStarred = msg.Starred
					break
				}
			}
			m.updateTimelineTable()
			if msg.Starred {
				m.statusMessage = "★ Starred"
			} else {
				m.statusMessage = "☆ Unstarred"
			}
		}
		return m, nil

	case DraftSaveTickMsg:
		cmds := []tea.Cmd{draftSaveTick()} // always reschedule
		if m.activeTab == tabCompose && composeHasContent(m) && !m.draftSaving {
			m.draftSaving = true
			if m.lastDraftUID != 0 {
				cmds = append(cmds, m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder))
				m.lastDraftUID = 0
				m.lastDraftFolder = ""
			}
			cmds = append(cmds, m.saveDraftCmd())
		}
		return m, tea.Batch(cmds...)

	case DraftSavedMsg:
		m.draftSaving = false
		if msg.Err != nil {
			logger.Warn("auto-save draft failed: %v", msg.Err)
		} else {
			m.lastDraftUID = msg.UID
			m.lastDraftFolder = msg.Folder
			m.statusMessage = "Draft saved"
		}
		return m, nil

	case DraftDeletedMsg:
		if msg.Err != nil {
			logger.Warn("delete old draft failed: %v", msg.Err)
		}
		return m, nil

	case ContactSuggestionsMsg:
		m.suggestions = msg.Contacts
		if len(m.suggestions) == 0 {
			m.suggestionIdx = -1
		} else {
			m.suggestionIdx = 0
		}
		return m, nil

	case AIAssistMsg:
		m.composeAILoading = false
		if msg.Err != nil {
			m.composeStatus = fmt.Sprintf("AI error: %v", msg.Err)
			return m, nil
		}
		original := m.composeBody.Value()
		m.composeAIDiff = wordDiff(original, msg.Result)
		m.composeAIResponse.SetValue(msg.Result)
		return m, nil

	case AISubjectMsg:
		m.composeAILoading = false
		if msg.Err != nil {
			m.composeStatus = fmt.Sprintf("AI error: %v", msg.Err)
			return m, nil
		}
		m.composeAISubjectHint = strings.TrimSpace(msg.Subject)
		return m, nil

	case ValidIDsMsg:
		// Background reconciliation has produced the live valid-ID set.
		// The backend's filterByValidIDs now applies automatically; reload all views.
		if stats, err := m.backend.GetSenderStatistics(m.currentFolder); err == nil {
			m.stats = stats
		}
		m.loadClassifications()
		m.updateSummaryTable()
		m.updateDetailsTable()
		return m, m.loadTimelineEmails()

	case EmailBodyMsg:
		m.emailBodyLoading = false
		m.selectedAttachment = 0 // reset attachment cursor for new email
		// Reset quick reply state for the new email
		m.quickReplies = nil
		m.quickRepliesReady = false
		m.quickReplyOpen = false
		m.quickReplyIdx = 0
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
			// Build canned replies and kick off AI generation
			if m.selectedTimelineEmail != nil {
				email := m.selectedTimelineEmail
				m.quickReplies = buildCannedReplies(email.Sender)
				// Mark as read and cache unsubscribe headers (fire-and-forget commands)
				body := msg.Body
				var cmds []tea.Cmd
				if !email.IsRead {
					email.IsRead = true // optimistic update in memory
					cmds = append(cmds, markReadCmd(m.backend, email.MessageID, email.Folder))
				}
				if body != nil && (body.ListUnsubscribe != "" || body.ListUnsubscribePost != "") {
					cmds = append(cmds, cacheUnsubscribeHeadersCmd(m.backend, email.MessageID, body.ListUnsubscribe, body.ListUnsubscribePost))
				}
				// Only fetch AI image descriptions when the terminal can't render them natively
				if body != nil && len(body.InlineImages) > 0 && m.classifier != nil && m.classifier.HasVisionModel() && !iterm2.IsSupported() {
					cmds = append(cmds, describeImagesCmd(m.classifier, body.InlineImages)...)
				}
				// Kick off AI quick reply generation
				if m.classifier != nil && body != nil && body.TextPlain != "" {
					bodyPreview := body.TextPlain
					if len([]rune(bodyPreview)) > 500 {
						bodyPreview = string([]rune(bodyPreview)[:500])
					}
					cmds = append(cmds, generateQuickRepliesCmd(m.classifier, email.Sender, email.Subject, bodyPreview))
				} else {
					// No classifier or no body text — mark as ready immediately with just canned replies
					m.quickRepliesReady = true
				}
				if len(cmds) > 0 {
					m.bodyWrappedLines = nil
					return m, tea.Batch(cmds...)
				}
			} else {
				m.quickRepliesReady = true
			}
		}
		m.bodyWrappedLines = nil // invalidate wrap cache
		return m, nil

	case QuickRepliesMsg:
		if msg.Err != nil {
			logger.Warn("Quick reply generation failed: %v", msg.Err)
		} else if len(msg.Replies) > 0 {
			// Append AI suggestions after canned replies
			m.quickReplies = append(m.quickReplies, msg.Replies...)
		}
		m.quickRepliesReady = true
		return m, nil

	case CleanupEmailBodyMsg:
		m.cleanupBodyLoading = false
		if msg.Err != nil {
			logger.Warn("Failed to fetch cleanup email body: %v", msg.Err)
			m.cleanupEmailBody = &models.EmailBody{TextPlain: "(Failed to load body)"}
		} else {
			m.cleanupEmailBody = msg.Body
			m.cleanupBodyWrappedLines = nil // force rewrap on next render
		}
		return m, nil

	case ImageDescMsg:
		if msg.Err == nil && msg.Description != "" {
			if m.inlineImageDescs == nil {
				m.inlineImageDescs = make(map[string]string)
			}
			m.inlineImageDescs[msg.ContentID] = msg.Description
		}
		return m, nil

	case ChatResponseMsg:
		m.chatWaiting = false
		content := msg.Content
		if msg.Err != nil {
			content = "Error: " + msg.Err.Error()
		}
		// Check for a <filter> block before stripping it for display.
		var filterCmd tea.Cmd
		if ids, label, found := parseChatFilter(content); found {
			// Build matched email list by MessageID.
			idSet := make(map[string]bool, len(ids))
			for _, id := range ids {
				idSet[id] = true
			}
			var matched []*models.EmailData
			for _, e := range m.timelineEmails {
				if idSet[e.MessageID] {
					matched = append(matched, e)
				}
			}
			if len(matched) > 0 {
				activateLabel := label
				filterCmd = func() tea.Msg {
					return ChatFilterActivatedMsg{Emails: matched, Label: activateLabel}
				}
			}
			// Strip filter block for clean display.
			content = stripChatFilter(content)
		}
		m.chatMessages = append(m.chatMessages, ai.ChatMessage{
			Role:    "assistant",
			Content: content,
		})
		m.chatWrappedLines = nil // invalidate wrap cache
		if filterCmd != nil {
			return m, filterCmd
		}
		return m, nil

	case ChatFilterActivatedMsg:
		m.chatFilterMode = true
		m.chatFilteredEmails = msg.Emails
		m.chatFilterLabel = msg.Label
		m.activeTab = tabTimeline
		m.updateTimelineTable()
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
			// Start background sync: IDLE if supported, else fall back to polling.
			syncCmd := m.startSync(m.currentFolder)
			// Subscribe to background reconciliation now that Load() has set the channel.
			m.validIDsCh = m.backend.ValidIDsCh()
			// Always load timeline since it's the default startup tab
			return m, tea.Batch(listFoldersCmd, m.loadTimelineEmails(), syncCmd, m.listenForValidIDs())
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
			if m.cleanupPreviewDeleting {
				m.cleanupPreviewDeleting = false
				m.cleanupPreviewIsArchive = false
			}
		} else {
			// Handle cleanup preview deletion: close preview and remove from details list
			if m.cleanupPreviewDeleting && msg.MessageID != "" &&
				m.cleanupPreviewEmail != nil && msg.MessageID == m.cleanupPreviewEmail.MessageID {
				toast := "Deleted"
				if m.cleanupPreviewIsArchive {
					toast = "Archived"
				}
				m.statusMessage = toast
				// Remove from detailsEmails slice
				filtered := m.detailsEmails[:0]
				for _, e := range m.detailsEmails {
					if e.MessageID != msg.MessageID {
						filtered = append(filtered, e)
					}
				}
				m.detailsEmails = filtered
				// Close the preview
				m.showCleanupPreview = false
				m.cleanupPreviewEmail = nil
				m.cleanupEmailBody = nil
				m.cleanupBodyLoading = false
				m.cleanupBodyScrollOffset = 0
				m.cleanupBodyWrappedLines = nil
				m.cleanupFullScreen = false
				m.cleanupPreviewDeleting = false
				m.cleanupPreviewIsArchive = false
				m.showSidebar = m.cleanupPreviewHadSidebar
				m.updateTableDimensions(m.windowWidth, m.windowHeight)
			}

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

	case RuleResultMsg:
		m.rulesFiredCount += msg.Result.FiredCount
		if msg.Result.Err != nil {
			logger.Warn("rule worker error: %v", msg.Result.Err)
		}
		return m, m.listenForRuleResult()

	case SearchResultMsg:
		if msg.Err != nil {
			m.searchError = msg.Err.Error()
			return m, nil
		}
		m.searchError = ""
		if msg.Query == "" {
			// Empty query = clear search
			m.searchResults = nil
			m.semanticScores = nil
			if m.timelineEmailsCache != nil {
				m.timelineEmails = m.timelineEmailsCache
				m.timelineEmailsCache = nil
			}
			m.updateTimelineTable()
		} else {
			m.searchResults = msg.Emails
			m.semanticScores = msg.Scores
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
		var cmds []tea.Cmd
		cmds = append(cmds, m.listenForNewEmails())
		for _, email := range msg.Emails {
			if m.classifier != nil && m.classifications[email.MessageID] == "" {
				// Auto-classify; rules will be triggered after classification completes.
				cmds = append(cmds, m.autoClassifyEmailCmd(email))
			} else if cat := m.classifications[email.MessageID]; cat != "" {
				// Already classified — send directly to rule engine (non-blocking).
				select {
				case m.ruleRequestCh <- models.RuleRequest{Email: email, Category: cat}:
				default:
				}
			}
			// Warn if this sender was previously unsubscribed from.
			if ok, _ := m.backend.IsUnsubscribedSender(email.Sender); ok {
				m.statusMessage = fmt.Sprintf("⚠ Email from unsubscribed sender: %s", email.Sender)
			}
		}
		return m, tea.Batch(cmds...)

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

	case AutoClassifyResultMsg:
		if msg.Err == nil {
			if m.classifications == nil {
				m.classifications = make(map[string]string)
			}
			m.classifications[msg.MessageID] = msg.Category
			m.updateTimelineTable()
		}
		// Send to rule engine regardless; use empty category on error so rules
		// that don't require a category can still fire.
		cat := msg.Category
		for _, e := range m.timelineEmails {
			if e.MessageID == msg.MessageID {
				select {
				case m.ruleRequestCh <- models.RuleRequest{Email: e, Category: cat}:
				default:
				}
				break
			}
		}
		return m, nil

	case SyncTickMsg:
		if m.syncCountdown > 0 {
			m.syncCountdown--
		}
		return m, m.tickSyncCountdown()

	case EmbeddingProgressMsg:
		m.embeddingDone = msg.Done
		m.embeddingTotal = msg.Total
		if msg.Done < msg.Total {
			return m, m.runEmbeddingBatch()
		}
		return m, nil

	case EmbeddingDoneMsg:
		m.embeddingDone = 0
		m.embeddingTotal = 0
		return m, nil

	case ContactEnrichedMsg:
		if msg.Count > 0 {
			m.statusMessage = fmt.Sprintf("Enriched %d contacts", msg.Count)
			return m, m.runContactEnrichment()
		}
		return m, nil

	case ContactsLoadedMsg:
		m.contactsList = msg.Contacts
		m.contactsFiltered = msg.Contacts
		m.contactsIdx = 0
		return m, nil

	case ContactDetailLoadedMsg:
		m.contactDetailEmails = msg.Emails
		m.contactDetailIdx = 0
		return m, nil

	case AppleContactsImportedMsg:
		if msg.Count > 0 {
			logger.Debug("Imported %d contacts from Apple Contacts", msg.Count)
		}
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
			m.composeCC, cmd = m.composeCC.Update(msg)
		case 2:
			m.composeBCC, cmd = m.composeBCC.Update(msg)
		case 3:
			m.composeSubject, cmd = m.composeSubject.Update(msg)
		case 4:
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
	// OAuth wait overlay takes over the entire screen when active.
	if m.oauthWait != nil {
		return m.oauthWait.View()
	}
	// Settings overlay takes over the entire screen when active.
	if m.showSettings && m.settingsPanel != nil {
		return m.settingsPanel.View()
	}
	// Rule editor overlay takes over the entire screen when active.
	if m.showRuleEditor && m.ruleEditor != nil {
		return m.ruleEditor.View()
	}
	// Prompt editor overlay takes over the entire screen when active.
	if m.showPromptEditor && m.promptEditor != nil {
		return m.promptEditor.View()
	}
	// Cleanup manager overlay takes over the entire screen when active.
	if m.showCleanupMgr && m.cleanupManager != nil {
		return m.cleanupManager.View()
	}
	if m.windowWidth > 0 && m.windowWidth < minTermWidth {
		return fmt.Sprintf("\n  Terminal too narrow (%d cols). Please resize to at least %d columns.", m.windowWidth, minTermWidth)
	}
	if m.windowHeight > 0 && m.windowHeight < minTermHeight {
		return fmt.Sprintf("\n  Terminal too short (%d rows). Please resize to at least %d rows.", m.windowHeight, minTermHeight)
	}
	if m.loading {
		return m.renderLoadingView()
	}
	if m.emailFullScreen {
		return m.renderFullScreenEmail()
	}
	if m.cleanupFullScreen {
		return m.renderCleanupPreview()
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

	// Unsubscribe confirmation prompt intercepts all keys
	if m.pendingUnsubscribe {
		switch msg.String() {
		case "y", "Y":
			m.pendingUnsubscribe = false
			action := m.pendingUnsubscribeAction
			m.pendingUnsubscribeAction = nil
			m.pendingUnsubscribeDesc = ""
			if action != nil {
				return m, action()
			}
		case "n", "N", "esc":
			m.pendingUnsubscribe = false
			m.pendingUnsubscribeAction = nil
			m.pendingUnsubscribeDesc = ""
		}
		return m, nil
	}

	// 3-way unsubscribe choice (Cleanup tab) intercepts all keys
	if m.unsubConfirmMode {
		switch msg.String() {
		case "h", "H":
			sender := m.unsubConfirmSender
			m.unsubConfirmMode = false
			m.unsubConfirmSender = ""
			// Hard unsubscribe: delete all emails from this sender via deletion worker
			folder := m.currentFolder
			ch := m.deletionRequestCh
			go func() {
				ch <- models.DeletionRequest{Sender: sender, IsDomain: false, Folder: folder}
			}()
			m.deleting = true
			m.deletionsPending = 1
			m.deletionsTotal = 1
			return m, m.listenForDeletionResults()
		case "s", "S":
			sender := m.unsubConfirmSender
			m.unsubConfirmMode = false
			m.unsubConfirmSender = ""
			return m, createSoftUnsubscribeRuleCmd(m.backend, sender)
		case "esc":
			m.unsubConfirmMode = false
			m.unsubConfirmSender = ""
		}
		return m, nil
	}

	// Attachment save prompt intercepts all keys while active
	if m.attachmentSavePrompt {
		switch msg.String() {
		case "enter":
			if m.emailBody != nil && m.selectedAttachment < len(m.emailBody.Attachments) {
				att := &m.emailBody.Attachments[m.selectedAttachment]
				path := expandTilde(m.attachmentSaveInput.Value())
				m.attachmentSavePrompt = false
				m.attachmentSaveInput.Blur()
				return m, saveAttachmentCmd(m.backend, att, path)
			}
			m.attachmentSavePrompt = false
			m.attachmentSaveInput.Blur()
		case "esc":
			m.attachmentSavePrompt = false
			m.attachmentSaveInput.Blur()
		default:
			var cmd tea.Cmd
			m.attachmentSaveInput, cmd = m.attachmentSaveInput.Update(msg)
			return m, cmd
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
			m.semanticScores = nil
			m.searchError = ""
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

	// Reset pending 'yy' sequence on any key other than 'y'
	if m.pendingY && msg.String() != "y" {
		m.pendingY = false
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

	// Contacts tab gets its own key handler (except for global keys handled below)
	if m.activeTab == tabContacts {
		switch msg.String() {
		case "1", "2", "3", "4", "q", "ctrl+c", "r", "f", "c", "l", "L":
			// fall through to global handler
		default:
			return m.handleContactsKey(msg)
		}
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
		if m.quickReplyOpen && len(m.quickReplies) > 0 {
			return m.openQuickReply(m.quickReplies[0])
		}
		if !m.loading && m.activeTab != tabTimeline {
			var extraCmds []tea.Cmd
			if m.activeTab == tabCompose && composeHasContent(m) && !m.draftSaving {
				m.draftSaving = true
				if m.lastDraftUID != 0 {
					extraCmds = append(extraCmds, m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder))
					m.lastDraftUID = 0
					m.lastDraftFolder = ""
				}
				extraCmds = append(extraCmds, m.saveDraftCmd())
			}
			m.activeTab = tabTimeline
			m.setFocusedPanel(panelTimeline)
			extraCmds = append(extraCmds, m.loadTimelineEmails())
			return m, tea.Batch(extraCmds...)
		}
		return m, nil

	case "2":
		if m.quickReplyOpen && len(m.quickReplies) > 1 {
			return m.openQuickReply(m.quickReplies[1])
		}
		if !m.loading && m.activeTab != tabCompose {
			m.activeTab = tabCompose
			m.timelineTable.Blur()
			m.summaryTable.Blur()
			m.detailsTable.Blur()
			m.composeField = 0
			m.composeTo.Focus()
			m.composeSubject.Blur()
			m.composeBody.Blur()
			// Clear reply/AI context for new compose (not a reply)
			m.replyContextEmail = nil
			m.composeAIThread = false
			m.composeAIPanel = false
			m.composeAIDiff = ""
			m.composeAISubjectHint = ""
			m.composeAIResponse.SetValue("")
			m.composeAILoading = false
		}
		return m, nil

	case "3":
		if m.quickReplyOpen && len(m.quickReplies) > 2 {
			return m.openQuickReply(m.quickReplies[2])
		}
		if !m.loading && m.activeTab != tabCleanup {
			var extraCmds []tea.Cmd
			if m.activeTab == tabCompose && composeHasContent(m) && !m.draftSaving {
				m.draftSaving = true
				if m.lastDraftUID != 0 {
					extraCmds = append(extraCmds, m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder))
					m.lastDraftUID = 0
					m.lastDraftFolder = ""
				}
				extraCmds = append(extraCmds, m.saveDraftCmd())
			}
			m.activeTab = tabCleanup
			m.setFocusedPanel(panelSummary)
			return m, tea.Batch(extraCmds...)
		}
		return m, nil

	case "4":
		if m.quickReplyOpen && len(m.quickReplies) > 3 {
			return m.openQuickReply(m.quickReplies[3])
		}
		if !m.loading && m.activeTab != tabContacts {
			var extraCmds []tea.Cmd
			if m.activeTab == tabCompose && composeHasContent(m) && !m.draftSaving {
				m.draftSaving = true
				if m.lastDraftUID != 0 {
					extraCmds = append(extraCmds, m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder))
					m.lastDraftUID = 0
					m.lastDraftFolder = ""
				}
				extraCmds = append(extraCmds, m.saveDraftCmd())
			}
			m.activeTab = tabContacts
			m.contactFocusPanel = 0
			extraCmds = append(extraCmds, m.loadContacts())
			return m, tea.Batch(extraCmds...)
		}
		return m, m.loadContacts()

	case "5":
		if m.quickReplyOpen && len(m.quickReplies) > 4 {
			return m.openQuickReply(m.quickReplies[4])
		}
		return m, nil

	case "6":
		if m.quickReplyOpen && len(m.quickReplies) > 5 {
			return m.openQuickReply(m.quickReplies[5])
		}
		return m, nil

	case "7":
		if m.quickReplyOpen && len(m.quickReplies) > 6 {
			return m.openQuickReply(m.quickReplies[6])
		}
		return m, nil

	case "8":
		if m.quickReplyOpen && len(m.quickReplies) > 7 {
			return m.openQuickReply(m.quickReplies[7])
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
			// Clear any active chat filter on refresh.
			m.chatFilterMode = false
			m.chatFilteredEmails = nil
			m.chatFilterLabel = ""
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
		if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupPreviewEmail != nil && !m.loading && !m.deleting {
			email := m.cleanupPreviewEmail
			m.cleanupPreviewDeleting = true
			m.cleanupPreviewIsArchive = false
			m.deleting = true
			m.deletionsPending++
			m.deletionsTotal++
			ch := m.deletionRequestCh
			go func() {
				ch <- models.DeletionRequest{MessageID: email.MessageID, Folder: email.Folder, IsArchive: false}
			}()
			return m, m.listenForDeletionResults()
		}
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

	case "A":
		// Re-classify the currently focused single email with AI.
		if !m.loading && m.classifier != nil {
			var target *models.EmailData
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupPreviewEmail != nil {
				target = m.cleanupPreviewEmail
			} else if m.activeTab == tabTimeline {
				cursor := m.timelineTable.Cursor()
				if cursor < len(m.threadRowMap) {
					ref := m.threadRowMap[cursor]
					if ref.kind == rowKindThread {
						target = ref.group.emails[0]
					} else {
						target = ref.group.emails[ref.emailIdx]
					}
				}
			}
			if target != nil {
				return m, m.reclassifyEmailCmd(target)
			}
		} else if m.classifier == nil {
			m.statusMessage = "No AI configured"
		}
		return m, nil

	case "W":
		if m.activeTab == tabCleanup && !m.showRuleEditor {
			sender := ""
			domain := ""
			cursor := m.summaryTable.Cursor()
			if s, ok := m.rowToSender[cursor]; ok {
				if m.groupByDomain {
					domain = s
				} else {
					sender = s
				}
			}
			m.ruleEditor = NewRuleEditor(sender, domain, m.windowWidth, m.windowHeight)
			m.showRuleEditor = true
			return m, m.ruleEditor.Init()
		}
		return m, nil

	case "P":
		if !m.showRuleEditor && !m.showPromptEditor && !m.showSettings {
			m.showPromptEditor = true
			m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
			return m, m.promptEditor.Init()
		}
		return m, nil

	case "C":
		if m.activeTab == tabCleanup && !m.showCleanupMgr {
			m.cleanupManager = NewCleanupManager(m.backend, m.windowWidth, m.windowHeight)
			m.showCleanupMgr = true
			return m, m.cleanupManager.Init()
		}
		return m, nil

	case "S":
		if !m.showSettings {
			m.showSettings = true
			m.settingsPanel = NewSettingsWithPath(SettingsModePanel, m.cfg, m.configPath)
			return m, m.settingsPanel.Init()
		}
		return m, nil

	case "e":
		if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupPreviewEmail != nil && !m.loading && !m.deleting {
			email := m.cleanupPreviewEmail
			m.cleanupPreviewDeleting = true
			m.cleanupPreviewIsArchive = true
			m.deleting = true
			m.deletionsPending++
			m.deletionsTotal++
			ch := m.deletionRequestCh
			go func() {
				ch <- models.DeletionRequest{MessageID: email.MessageID, Folder: email.Folder, IsArchive: true}
			}()
			return m, m.listenForDeletionResults()
		}
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

	case "*":
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
				if email != nil {
					return m, m.toggleStarCmd(email)
				}
			}
		}
		return m, nil

	case "u":
		if m.activeTab == tabTimeline && m.emailBody != nil && m.selectedTimelineEmail != nil {
			if m.emailBody.ListUnsubscribe != "" {
				sender := m.selectedTimelineEmail.Sender
				body := m.emailBody
				m.pendingUnsubscribe = true
				m.pendingUnsubscribeDesc = fmt.Sprintf("Unsubscribe from %s?", sender)
				m.pendingUnsubscribeAction = func() tea.Cmd {
					return unsubscribeCmd(body)
				}
			}
		} else if m.activeTab == tabCleanup && !m.loading && !m.deleting {
			cursor := m.summaryTable.Cursor()
			if sender, ok := m.rowToSender[cursor]; ok && sender != "" {
				m.unsubConfirmMode = true
				m.unsubConfirmSender = sender
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
			if m.quickReplyOpen && len(m.quickReplies) > 0 {
				return m.openQuickReply(m.quickReplies[m.quickReplyIdx])
			}
			if m.focusedPanel == panelSidebar {
				m.selectSidebarFolder()
				// Clear chat filter when switching folders.
				m.chatFilterMode = false
				m.chatFilteredEmails = nil
				m.chatFilterLabel = ""
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
					m.inlineImageDescs = nil // reset per-email image descriptions
					m.bodyScrollOffset = 0
					m.updateTableDimensions(m.windowWidth, m.windowHeight)
					return m, m.loadEmailBodyCmd(email.Folder, email.UID)
				}
			} else if m.focusedPanel == panelDetails && m.activeTab == tabCleanup {
				// Open or scroll cleanup preview
				if m.showCleanupPreview {
					m.cleanupBodyScrollOffset++
				} else {
					idx := m.detailsTable.Cursor()
					if idx < len(m.detailsEmails) {
						email := m.detailsEmails[idx]
						m.cleanupPreviewEmail = email
						m.showCleanupPreview = true
						m.cleanupBodyLoading = true
						m.cleanupEmailBody = nil
						m.cleanupBodyScrollOffset = 0
						m.cleanupBodyWrappedLines = nil
						m.cleanupPreviewHadSidebar = m.showSidebar
						m.showSidebar = false
						m.updateTableDimensions(m.windowWidth, m.windowHeight)
						return m, fetchCleanupBodyCmd(m.backend, email)
					}
				}
			} else if m.focusedPanel == panelSummary {
				m.updateDetailsTable()
			}
		}
		return m, nil

	case "ctrl+q":
		// Toggle quick reply picker (only in preview panel or full-screen)
		if m.activeTab == tabTimeline && (m.focusedPanel == panelPreview || m.emailFullScreen) && m.emailBody != nil {
			m.quickReplyOpen = !m.quickReplyOpen
			if m.quickReplyOpen {
				if m.quickReplyIdx >= len(m.quickReplies) {
					m.quickReplyIdx = 0
				}
			}
		}
		return m, nil

	case "z":
		if m.activeTab == tabCleanup && m.showCleanupPreview {
			m.cleanupFullScreen = !m.cleanupFullScreen
			m.cleanupBodyWrappedLines = nil // force re-wrap at new width
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
			return m, nil
		}
		if !m.loading && m.selectedTimelineEmail != nil {
			m.emailFullScreen = !m.emailFullScreen
			m.bodyWrappedLines = nil // force re-wrap at new width
		}
		return m, nil

	case "esc":
		if m.quickReplyOpen {
			m.quickReplyOpen = false
			return m, nil
		}
		if m.visualMode {
			m.visualMode = false
			m.pendingY = false
			return m, nil
		}
		if m.emailFullScreen {
			m.emailFullScreen = false
			m.bodyWrappedLines = nil
			return m, nil
		}
		if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupFullScreen {
			m.cleanupFullScreen = false
			m.cleanupBodyWrappedLines = nil
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
			return m, nil
		}
		if m.activeTab == tabCleanup && m.showCleanupPreview {
			m.showCleanupPreview = false
			m.cleanupPreviewEmail = nil
			m.cleanupEmailBody = nil
			m.cleanupBodyLoading = false
			m.cleanupBodyScrollOffset = 0
			m.cleanupBodyWrappedLines = nil
			m.cleanupFullScreen = false
			m.cleanupPreviewDeleting = false
			m.cleanupPreviewIsArchive = false
			m.showSidebar = m.cleanupPreviewHadSidebar
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
			return m, nil
		}
		// Clear chat filter first; a second Esc will then close the preview.
		if m.activeTab == tabTimeline && m.chatFilterMode {
			m.chatFilterMode = false
			m.chatFilteredEmails = nil
			m.chatFilterLabel = ""
			m.updateTimelineTable()
			return m, nil
		}
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

	case "s":
		// Save highlighted attachment from preview panel
		if !m.loading && m.focusedPanel == panelPreview && m.emailBody != nil &&
			len(m.emailBody.Attachments) > 0 && !m.attachmentSavePrompt {
			att := m.emailBody.Attachments[m.selectedAttachment]
			defaultPath := expandTilde("~/Downloads/" + att.Filename)
			m.attachmentSaveInput.SetValue(defaultPath)
			m.attachmentSaveInput.Focus()
			m.attachmentSavePrompt = true
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
				m.replyContextEmail = email
				m.composeAIThread = true
				m.composeTo.SetValue(email.Sender)
				subject := email.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}
				m.composeSubject.SetValue(subject)
				m.composeField = 4
				m.composeTo.Blur()
				m.composeSubject.Blur()
				m.composeBody.Focus()
			}
		}
		return m, nil

	case "up", "k":
		if !m.loading {
			if m.quickReplyOpen {
				if m.quickReplyIdx > 0 {
					m.quickReplyIdx--
				}
				return m, nil
			}
			if m.emailFullScreen {
				if m.visualMode {
					if m.visualEnd > m.visualStart {
						m.visualEnd--
					}
				} else if m.bodyScrollOffset > 0 {
					m.bodyScrollOffset--
				}
				return m, nil
			}
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.focusedPanel == panelDetails {
				if m.cleanupBodyScrollOffset > 0 {
					m.cleanupBodyScrollOffset--
				}
				return m, nil
			}
			if m.activeTab == tabTimeline {
				if m.focusedPanel == panelPreview {
					if m.visualMode {
						if m.visualEnd > m.visualStart {
							m.visualEnd--
						}
					} else if m.bodyScrollOffset > 0 {
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
			if m.quickReplyOpen {
				if m.quickReplyIdx < len(m.quickReplies)-1 {
					m.quickReplyIdx++
				}
				return m, nil
			}
			if m.emailFullScreen {
				if m.visualMode {
					if m.visualEnd < len(m.bodyWrappedLines)-1 {
						m.visualEnd++
					}
				} else {
					m.bodyScrollOffset++
				}
				return m, nil
			}
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.focusedPanel == panelDetails {
				m.cleanupBodyScrollOffset++
				return m, nil
			}
			if m.activeTab == tabTimeline {
				if m.focusedPanel == panelPreview {
					if m.visualMode {
						if m.visualEnd < len(m.bodyWrappedLines)-1 {
							m.visualEnd++
						}
					} else {
						m.bodyScrollOffset++
					}
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

	case "v":
		if m.emailFullScreen || (m.activeTab == tabTimeline && m.focusedPanel == panelPreview) {
			if len(m.bodyWrappedLines) > 0 {
				m.visualMode = !m.visualMode
				if m.visualMode {
					m.visualStart = m.bodyScrollOffset
					m.visualEnd = m.bodyScrollOffset
				}
			}
		}
		return m, nil

	case "m":
		m.mouseMode = !m.mouseMode
		if m.mouseMode {
			return m, tea.DisableMouse
		}
		return m, tea.EnableMouseCellMotion

	case "y":
		if m.pendingY {
			// yy: copy current line
			m.pendingY = false
			if m.bodyScrollOffset < len(m.bodyWrappedLines) {
				return m, copyToClipboard(m.bodyWrappedLines[m.bodyScrollOffset])
			}
		} else if m.visualMode {
			// y in visual mode: copy selection
			m.visualMode = false
			m.pendingY = false
			start, end := m.visualStart, m.visualEnd
			if start > end {
				start, end = end, start
			}
			if end >= len(m.bodyWrappedLines) {
				end = len(m.bodyWrappedLines) - 1
			}
			if start < len(m.bodyWrappedLines) {
				selected := strings.Join(m.bodyWrappedLines[start:end+1], "\n")
				return m, copyToClipboard(selected)
			}
		} else {
			m.pendingY = true
		}
		return m, nil

	case "Y":
		// Copy all body lines
		m.visualMode = false
		m.pendingY = false
		if len(m.bodyWrappedLines) > 0 {
			return m, copyToClipboard(strings.Join(m.bodyWrappedLines, "\n"))
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
	} else if m.activeTab == tabContacts {
		mainContent = m.renderContactsTab(m.windowWidth, m.windowHeight)
	} else {
		// Cleanup tab
		if m.showCleanupPreview && m.cleanupFullScreen {
			// Full-screen: the entire content area is the preview
			mainContent = m.renderCleanupPreview()
		} else {
			var summaryView string
			if m.stats != nil && len(m.stats) == 0 {
				summaryView = m.emptyStateView("No emails in this folder  •  press r to refresh")
			} else {
				summaryView = m.baseStyle.Render(renderStyledTableView(&m.summaryTable, 1))
			}
			detailsView := m.baseStyle.Render(renderStyledTableView(&m.detailsTable, 2))
			if m.showCleanupPreview {
				// 3-column layout: summary | details | preview (sidebar hidden while preview is open)
				previewPanel := m.renderCleanupPreview()
				mainContent = lipgloss.JoinHorizontal(lipgloss.Top, summaryView, "  ", detailsView, previewPanel)
			} else if m.showSidebar && !m.sidebarTooWide {
				sidebarView := m.baseStyle.Render(m.renderSidebar())
				mainContent = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, "  ", summaryView, "  ", detailsView)
			} else {
				mainContent = lipgloss.JoinHorizontal(lipgloss.Top, summaryView, "  ", detailsView)
			}
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
