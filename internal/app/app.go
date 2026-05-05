package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/cleanup"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
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
	tabContacts = 3
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

// StartupHydratedMsg carries cached startup data used to progressively hydrate
// the UI while live IMAP loading continues in the background.
type StartupHydratedMsg struct {
	Stats         map[string]*models.SenderStats
	Emails        []*models.EmailData
	Err           error
	FinishLoading bool
	StatusMessage string
}

// TimelineLoadedMsg carries emails sorted by date for the timeline tab
type TimelineLoadedMsg struct {
	Emails   []*models.EmailData
	Notice   string
	ReadOnly bool
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
	Body      *models.EmailBody
	Err       error
	MessageID string // used to discard stale body fetches from rapid cursor movement
}

// TimelineForwardBodyMsg carries a body fetch result for Timeline forwarding.
type TimelineForwardBodyMsg struct {
	Email     *models.EmailData
	Body      *models.EmailBody
	Err       error
	MessageID string
	RequestID int
}

// TimelineReplyBodyMsg carries a body fetch result for Timeline replying.
type TimelineReplyBodyMsg struct {
	Email     *models.EmailData
	Body      *models.EmailBody
	Err       error
	MessageID string
	RequestID int
}

// TimelineDraftBodyMsg carries a body fetch result for Timeline draft editing.
type TimelineDraftBodyMsg struct {
	Email     *models.EmailData
	Body      *models.EmailBody
	Err       error
	MessageID string
	RequestID int
}

// TimelineDraftSentMsg carries the result of sending a saved draft directly
// from Timeline without opening Compose.
type TimelineDraftSentMsg struct {
	Email     *models.EmailData
	Err       error
	MessageID string
}

// QuickRepliesMsg is sent when AI quick reply generation completes.
type QuickRepliesMsg struct {
	Replies []string
	Err     error
}

// CleanupEmailBodyMsg carries the result of fetching an email body from the Cleanup tab
type CleanupEmailBodyMsg struct {
	MessageID string
	Body      *models.EmailBody
	Err       error
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
	Source string // "local", "fts", "imap", "semantic", "hybrid", "cross"
	Token  int
	Err    error // non-nil when the search failed with a user-visible error
}

// TimelineSearchDebounceMsg fires after the Timeline search debounce window.
type TimelineSearchDebounceMsg struct {
	Query string
	Token int
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
	Done   int
	Total  int
	Notice string
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

// SoftUnsubResultMsg carries the result of a Hide Future Mail request.
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
type ContactEnrichedMsg struct {
	Count      int
	Notice     string
	Background bool
}

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
	UID           uint32
	Folder        string
	ReplaceUID    uint32
	ReplaceFolder string
	Err           error
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
	backend      backend.Backend
	progressCh   <-chan models.ProgressInfo
	syncEventsCh <-chan models.FolderSyncEvent

	// UI State
	loading           bool
	deleting          bool
	deletionProgress  models.DeletionResult
	deletionsPending  int  // Number of deletions waiting to complete
	deletionsTotal    int  // Total deletions in current batch
	connectionLost    bool // true while IMAP connection is down during deletion
	loadingSpinner    int
	startTime         time.Time
	progressInfo      models.ProgressInfo
	syncAccumulator   syncAccumulator
	syncGeneration    int64
	syncingFolder     string
	syncCountsSettled bool
	showLogs          bool
	showHelp          bool
	helpScrollOffset  int
	windowWidth       int
	windowHeight      int
	subjectColWidth   int

	// Deletion channels
	deletionRequestCh chan models.DeletionRequest
	deletionResultCh  chan models.DeletionResult

	// Rule engine channels
	ruleRequestCh   chan models.RuleRequest
	ruleResultCh    chan models.RuleResult
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
	groupByDomain       bool
	currentFolder       string
	selectedSender      string
	selectedSummaryKeys map[string]bool     // Selected sender/domain keys in summary table
	selectedMessages    map[string]bool     // Selected messages by MessageID (across all senders)
	rowToSender         map[int]string      // Maps row index to original sender (before sanitization)
	detailsEmails       []*models.EmailData // Current emails shown in details table
	sidebarTooWide      bool                // set by layout when sidebar + terminal width leaves < 16 variable cols

	// Tabs
	activeTab int // tabCleanup, tabTimeline, or tabCompose

	// Timeline
	timeline TimelineState

	// Email body preview (cleanup tab)
	showCleanupPreview         bool
	cleanupPreviewEmail        *models.EmailData
	cleanupEmailBody           *models.EmailBody
	cleanupBodyLoading         bool
	cleanupBodyScrollOffset    int
	cleanupBodyWrappedLines    []string
	cleanupBodyWrappedWidth    int
	cleanupPreviewDocLayout    *previewDocumentLayout
	cleanupPreviewDocWidth     int
	cleanupPreviewDocRows      int
	cleanupPreviewDocMode      previewImageMode
	cleanupPreviewDocMessageID string
	cleanupPreviewWidth        int // computed in updateTableDimensions
	cleanupPreviewHadSidebar   bool
	cleanupFullScreen          bool // true = preview takes entire screen
	cleanupPreviewDeleting     bool // true = deletion/archive was triggered from preview
	cleanupPreviewIsArchive    bool // true = the preview action was archive (not delete)

	// Chat panel
	showChat         bool
	chatMessages     []ai.ChatMessage // conversation history
	chatWrappedLines [][]string       // cached wrapText output per message; nil = invalid
	chatWrappedWidth int              // width at which chatWrappedLines was built
	chatInput        textinput.Model
	chatWaiting      bool // waiting for Ollama response

	// AI classification
	classifier      ai.AIClient
	classifying     bool
	classifications map[string]string // messageID → category
	classifyTotal   int
	classifyDone    int

	// Compose
	mailer             *appsmtp.Client
	fromAddress        string
	composeTo          textinput.Model
	composeCC          textinput.Model
	composeBCC         textinput.Model
	composeSubject     textinput.Model
	composeBody        textarea.Model
	composeField       int    // 0=To, 1=CC, 2=BCC, 3=Subject, 4=Body
	composeStatus      string // last send result message
	composePreview     bool   // show glamour markdown preview
	composeAttachments []models.ComposeAttachment
	composePreserved   *composePreservedContext
	composeReturnSet   bool
	composeReturnTab   int
	composeReturnPanel int

	// Autocomplete (compose address fields)
	suggestions   []models.ContactData // current autocomplete candidates (empty = dropdown hidden)
	suggestionIdx int                  // selected row index (-1 = none selected)

	// Compose AI panel (Ctrl+G)
	composeAIPanel        bool
	composeAIInput        textinput.Model // free-form prompt
	composeAIResponse     textarea.Model  // editable AI rewrite
	composeAIDiff         string          // lipgloss-styled word diff (display only)
	composeAILoading      bool
	composeAIThread       bool              // true = include reply context in prompt
	composeAISubjectHint  string            // pending subject suggestion ("" = none)
	replyContextEmail     *models.EmailData // set when reply is initiated; nil for new emails
	attachmentPathInput   textinput.Model
	attachmentInputActive bool

	// Compose attachment path completion
	attachmentCompletions       []attachmentPathCandidate
	attachmentCompletionIdx     int
	attachmentCompletionVisible bool
	attachmentCompletionAnchor  string

	// Draft auto-save state
	lastDraftUID         uint32 // UID of last auto-saved draft; 0 = not saved yet
	lastDraftFolder      string // folder of last auto-saved draft
	lastDraftReplaceable bool   // true when lastDraftUID points at a canonical Drafts-folder copy
	draftSaving          bool   // true while a SaveDraft cmd is in flight (prevents concurrent saves)

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

	// IMAP IDLE / background sync
	syncStatusMode string // "idle", "polling", "off"
	syncCountdown  int    // seconds until next poll

	// Background embedding
	embeddingDone           int
	embeddingTotal          int
	embeddingBatchActive    bool
	contactEnrichmentActive bool

	// Demo mode — set when DemoBackend is detected; shows [DEMO] in status bar
	demoMode bool

	// Dry-run mode — log rule/cleanup actions without executing destructive ones
	dryRun bool
	// Contacts tab
	contactsList        []models.ContactData
	contactsFiltered    []models.ContactData
	contactsIdx         int
	contactSearch       string
	contactSearchMode   string // "" | "keyword" | "semantic"
	contactDetail       *models.ContactData
	contactDetailEmails []*models.EmailData
	contactDetailIdx    int
	contactFocusPanel   int // 0 = list, 1 = detail

	// Inline email preview within Contacts tab
	contactPreviewEmail   *models.EmailData
	contactPreviewBody    *models.EmailBody
	contactPreviewLoading bool

	// Config
	cfg        *config.Config
	configPath string

	// Settings panel overlay
	showSettings  bool
	settingsPanel *Settings

	// Rule editor overlay
	showRuleEditor    bool
	ruleEditor        *RuleEditor
	ruleDryRunPreview *ruleDryRunPreview

	// Cleanup manager overlay
	showCleanupMgr   bool
	cleanupManager   *CleanupManager
	cleanupScheduler *cleanup.Scheduler

	// Prompt editor overlay
	showPromptEditor bool
	promptEditor     *PromptEditor

	// OAuth wait overlay (shown after Gmail is chosen in the S-key settings panel)
	oauthWait *OAuthWaitModel

	// Local inline-image preview links. Disabled for SSH sessions, where
	// localhost would point at the server instead of the user's browser.
	localImageLinks   bool
	previewImageMode  previewImageMode
	imagePreviewLinks *imagePreviewServer

	// General status message (shown briefly after actions like settings save)
	statusMessage string

	// Contacts-only status message. This is cleared when leaving the contacts
	// workflow so contact actions do not leak stale notices into other tabs.
	contactStatusMessage string

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
	baseStyle := defaultTheme.BasePanelStyle()
	headerStyle := defaultTheme.TitleBarStyle()
	loadingStyle := defaultTheme.LoadingStyle()
	progressStyle := defaultTheme.ProgressStyle()

	// Create tables optimized for side-by-side display
	// Summary table: ~82 chars total (left side) - added selection column
	summaryTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "✓", Width: 1},
			{Title: "Sender/Domain", Width: 46},
			{Title: "Count", Width: 6},
			{Title: "Dates", Width: 20},
		}),
		table.WithFocused(true),
		table.WithHeight(11),
	)

	// Details table: ~69 chars total (right side) - added selection column
	detailsTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "✓", Width: 1},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: 32},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: 3},
		}),
		table.WithHeight(11),
	)

	// Create role-driven table styles shared by list-like panes.
	activeStyle := defaultTheme.TableStyles(true)
	inactiveStyle := defaultTheme.TableStyles(false)

	summaryTable.SetStyles(inactiveStyle)
	detailsTable.SetStyles(inactiveStyle)

	// Timeline table: full-width chronological email list
	timelineTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "✓", Width: 1},
			{Title: "Sender", Width: 20},
			{Title: "Subject", Width: 40},
			{Title: "When", Width: 12},
			{Title: "Tag", Width: 8},
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
		backend:             b,
		progressCh:          b.Progress(),
		syncEventsCh:        b.SyncEvents(),
		loading:             true,
		dryRun:              dryRun,
		startTime:           time.Now(),
		currentFolder:       "INBOX",
		folders:             []string{"INBOX"},
		folderTree:          buildFolderTree([]string{"INBOX"}),
		folderStatus:        make(map[string]models.FolderStatus),
		showSidebar:         true,
		focusedPanel:        panelTimeline,
		selectedSummaryKeys: make(map[string]bool),
		selectedMessages:    make(map[string]bool),
		rowToSender:         make(map[int]string),
		summaryTable:        summaryTable,
		detailsTable:        detailsTable,
		timelineTable:       timelineTable,
		logViewer:           logViewer,
		chatInput:           chatInput,
		classifier:          classifier,
		classifications:     make(map[string]string),
		timeline: TimelineState{
			expandedThreads:     make(map[string]bool),
			selectedMessageIDs:  make(map[string]bool),
			searchInput:         searchInput,
			attachmentSaveInput: attachmentSaveInput,
		},
		classifyCh:              make(chan ClassifyProgressMsg, 50),
		syncAccumulator:         newSyncAccumulator(defaultSyncFlushCount, defaultSyncFlushDelay),
		mailer:                  mailer,
		fromAddress:             fromAddress,
		composeTo:               composeTo,
		composeCC:               composeCC,
		composeBCC:              composeBCC,
		composeSubject:          composeSubject,
		composeBody:             composeBody,
		suggestionIdx:           -1,
		composeAIInput:          composeAIInput,
		composeAIResponse:       composeAIResponse,
		attachmentCompletionIdx: -1,
		localImageLinks:         true,
		previewImageMode:        previewImageModeAuto,
		imagePreviewLinks:       newImagePreviewServer(),
		baseStyle:               baseStyle,
		headerStyle:             headerStyle,
		loadingStyle:            loadingStyle,
		progressStyle:           progressStyle,
		activeTableStyle:        activeStyle,
		inactiveTableStyle:      inactiveStyle,
		deletionRequestCh:       deletionRequestCh,
		deletionResultCh:        deletionResultCh,
		ruleRequestCh:           ruleRequestCh,
		ruleResultCh:            ruleResultCh,
		attachmentPathInput:     attachmentPathInput,
	}

	// Detect demo mode via DemoBackendMarker interface
	if marker, ok := b.(interface{ IsDemo() bool }); ok && marker.IsDemo() {
		m.demoMode = true
	}

	// Start deletion worker goroutine
	go m.deletionWorker(deletionRequestCh, deletionResultCh)

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

func (m *Model) SetLocalImageLinksEnabled(enabled bool) {
	m.localImageLinks = enabled
	if !enabled {
		m.revokeImagePreviews()
	}
	m.clearCleanupPreviewDocumentCache()
}

func (m *Model) SetPreviewImageMode(mode PreviewImageMode) {
	m.previewImageMode = previewImageMode(mode)
	m.clearTimelinePreviewDocumentCache()
	m.clearCleanupPreviewDocumentCache()
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	if !m.loading {
		return tea.Batch(
			m.listenForRuleResult(),
			m.importAppleContacts(),
			draftSaveTick(),
		)
	}
	return tea.Batch(
		m.startLoading(),
		m.tickSpinner(),
		m.listenForSyncEvents(),
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
			m.statusMessage = "Preparing rule dry-run preview..."
			return m, m.previewAutomationRuleCmd(msg.Rule)
		}
		return m, nil

	case RuleEditorCancelledMsg:
		m.showRuleEditor = false
		m.ruleEditor = nil
		return m, nil

	case RuleDryRunPreviewMsg:
		if msg.Rule != nil || (msg.Report != nil && msg.Report.Kind == models.RuleDryRunKindAutomation) {
			m.ruleDryRunPreview = newAutomationDryRunPreview(msg.Report, msg.Rule, msg.Err)
		} else {
			m.ruleDryRunPreview = newCleanupDryRunPreview(msg.Report, msg.CleanupRequest, msg.Err)
			if m.dryRun && m.ruleDryRunPreview.pendingCleanupRule == nil {
				m.ruleDryRunPreview.liveRunDisabled = true
			}
		}
		return m, nil

	case PromptEditorDoneMsg:
		m.showPromptEditor = false
		m.promptEditor = nil
		if msg.Prompt != nil {
			if err := m.backend.SaveCustomPrompt(msg.Prompt); err != nil {
				m.statusMessage = "Error saving prompt: " + err.Error()
			} else {
				m.statusMessage = "Prompt saved: " + msg.Prompt.Name + ". Reopen P to review saved prompts."
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
		previousEmbeddingModel := ""
		if m.cfg != nil {
			previousEmbeddingModel = m.cfg.EffectiveEmbeddingModel()
		}
		m.cfg = msg.Config
		m.statusMessage = "Settings saved."
		if m.classifier != nil {
			m.classifier.SetEmbeddingModel(m.cfg.EffectiveEmbeddingModel())
		}
		if previousEmbeddingModel != "" && previousEmbeddingModel != m.cfg.EffectiveEmbeddingModel() {
			type embeddingModelEnsurer interface {
				EnsureEmbeddingModel(string) (bool, error)
			}
			if manager, ok := m.backend.(embeddingModelEnsurer); ok {
				invalidated, err := manager.EnsureEmbeddingModel(m.cfg.EffectiveEmbeddingModel())
				if err != nil {
					m.statusMessage = fmt.Sprintf("Settings saved, but embedding reset failed: %v", err)
				} else if invalidated {
					m.statusMessage = "Settings saved. Embeddings reset for the new model."
				}
			} else {
				m.statusMessage = "Settings saved. Restart Herald to rebuild embeddings for the new model."
			}
		}
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

	if m.ruleDryRunPreview != nil {
		if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.updateTableDimensions(sizeMsg.Width, sizeMsg.Height)
			m.chatWrappedLines = nil
			if m.cleanupManager != nil {
				m.cleanupManager.setSize(sizeMsg.Width, sizeMsg.Height)
			}
			return m, tea.ClearScreen
		}
		if key, ok := msg.(tea.KeyPressMsg); ok {
			model, cmd, handled := m.handleDryRunPreviewKey(key)
			if handled {
				return model, cmd
			}
		}
	}

	// Forward all messages to the settings panel when it is active (intercepts
	// key presses and window-size events so the panel handles them exclusively).
	if m.showSettings && m.settingsPanel != nil {
		if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.updateTableDimensions(sizeMsg.Width, sizeMsg.Height)
			m.chatWrappedLines = nil
		}
		newModel, cmd := m.settingsPanel.Update(msg)
		m.settingsPanel = newModel.(*Settings)
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			return m, tea.Batch(cmd, tea.ClearScreen)
		}
		return m, cmd
	}

	// Forward all messages to the rule editor when it is active.
	if m.showRuleEditor && m.ruleEditor != nil {
		if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.updateTableDimensions(sizeMsg.Width, sizeMsg.Height)
			m.chatWrappedLines = nil
		}
		var ruleCmd tea.Cmd
		m.ruleEditor, ruleCmd = m.ruleEditor.Update(msg)
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			return m, tea.Batch(ruleCmd, tea.ClearScreen)
		}
		return m, ruleCmd
	}

	// Forward all messages to the prompt editor when it is active.
	if m.showPromptEditor && m.promptEditor != nil {
		if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.updateTableDimensions(sizeMsg.Width, sizeMsg.Height)
			m.chatWrappedLines = nil
		}
		var promptCmd tea.Cmd
		m.promptEditor, promptCmd = m.promptEditor.Update(msg)
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			return m, tea.Batch(promptCmd, tea.ClearScreen)
		}
		return m, promptCmd
	}

	// Handle cleanup manager messages
	switch msg := msg.(type) {
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
		total := 0
		for _, n := range msg.Results {
			total += n
		}
		m.statusMessage = fmt.Sprintf("Cleanup complete: %d email(s) processed", total)
		return m, nil

	case CleanupDryRunMsg:
		req := models.RuleDryRunRequest{
			Kind:            models.RuleDryRunKindCleanup,
			RuleID:          msg.RuleID,
			CleanupRule:     msg.CleanupRule,
			AllFolders:      true,
			IncludeDisabled: msg.RuleID != 0 || msg.CleanupRule != nil,
		}
		m.statusMessage = "Preparing cleanup dry-run preview..."
		return m, m.previewCleanupRulesCmd(req)
	}

	// Forward all messages to the cleanup manager when it is active.
	if m.showCleanupMgr && m.cleanupManager != nil {
		if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.updateTableDimensions(sizeMsg.Width, sizeMsg.Height)
			m.chatWrappedLines = nil
		}
		var cleanupCmd tea.Cmd
		m.cleanupManager, cleanupCmd = m.cleanupManager.Update(msg)
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			return m, tea.Batch(cleanupCmd, tea.ClearScreen)
		}
		return m, cleanupCmd
	}

	if model, cmd, handled := m.handleTimelineMsg(msg); handled {
		return model, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		if model, cmd, handled := m.handleMouseMsg(msg); handled {
			return model, cmd
		}

	case FoldersLoadedMsg:
		logger.Debug("FoldersLoadedMsg: folders=%d currentFolder=%s", len(msg.Folders), m.currentFolder)
		if len(msg.Folders) == 0 {
			logger.Debug("FoldersLoadedMsg: empty result; existing folders=%d", len(m.folders))
			if len(m.folders) <= 1 {
				return m, m.loadFoldersCmd(time.Second)
			}
			if !isVirtualAllMailOnlyFolder(m.currentFolder) && m.syncStatusMode == "" {
				return m, m.startSync(m.currentFolder)
			}
			return m, nil
		}
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
			loadCounts := func() tea.Msg {
				status, err := m.backend.GetFolderStatus(folders)
				if err != nil {
					logger.Warn("Failed to get folder status: %v", err)
					return FolderStatusMsg{Status: map[string]models.FolderStatus{}}
				}
				return FolderStatusMsg{Status: status}
			}
			if !isVirtualAllMailOnlyFolder(m.currentFolder) && m.syncStatusMode == "" {
				return m, tea.Batch(loadCounts, m.startSync(m.currentFolder))
			}
			return m, loadCounts
		}
		if !isVirtualAllMailOnlyFolder(m.currentFolder) && m.syncStatusMode == "" {
			return m, m.startSync(m.currentFolder)
		}
		return m, nil

	case FolderStatusMsg:
		logger.Debug("FolderStatusMsg: merging %d folder statuses", len(msg.Status))
		// Merge rather than replace so partial results don't wipe existing counts
		for folder, st := range msg.Status {
			logger.Debug("FolderStatusMsg: folder=%s total=%d unseen=%d", folder, st.Total, st.Unseen)
			m.folderStatus[folder] = st
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
			m.composePreserved = nil
			m.composeAIThread = false
			m.composeAIPanel = false
			m.composeAIDiff = ""
			m.composeAISubjectHint = ""
			m.composeAIResponse.SetValue("")
			m.composeAILoading = false
			// Delete the auto-saved draft (if any) since the email was sent
			if m.lastDraftUID != 0 {
				if !m.lastDraftReplaceable {
					m.lastDraftUID = 0
					m.lastDraftFolder = ""
					m.lastDraftReplaceable = false
					return m, nil
				}
				cmd := m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder)
				m.lastDraftUID = 0
				m.lastDraftFolder = ""
				m.lastDraftReplaceable = false
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
		for _, e := range m.timeline.emails {
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
			if m.timeline.selectedEmail != nil {
				sender := m.timeline.selectedEmail.Sender
				method := msg.Method
				url := msg.URL
				go func() {
					_ = m.backend.RecordUnsubscribe(sender, method, url)
				}()
			}
		}
		return m, nil

	case SoftUnsubResultMsg:
		if msg.Err != nil {
			m.statusMessage = "Error enabling Hide Future Mail: " + msg.Err.Error()
		} else {
			m.statusMessage = "Hide Future Mail enabled for " + msg.Sender
		}
		return m, nil

	case DraftSaveTickMsg:
		cmds := []tea.Cmd{draftSaveTick()} // always reschedule
		if m.activeTab == tabCompose && composeHasContent(m) && !m.draftSaving {
			m.draftSaving = true
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
			m.lastDraftReplaceable = true
			m.statusMessage = "Draft saved"
			if msg.ReplaceUID != 0 && (msg.ReplaceUID != msg.UID || msg.ReplaceFolder != msg.Folder) {
				return m, m.deleteDraftCmd(msg.ReplaceUID, msg.ReplaceFolder)
			}
		}
		return m, nil

	case DraftDeletedMsg:
		if msg.Err != nil {
			logger.Warn("delete old draft failed: %v", msg.Err)
		}
		return m, nil

	case StartupHydratedMsg:
		if msg.Err != nil {
			logger.Warn("startup snapshot hydrate failed: %v", msg.Err)
			return m, nil
		}
		logger.Debug("StartupHydratedMsg: stats=%d emails=%d finish=%v status=%q", len(msg.Stats), len(msg.Emails), msg.FinishLoading, strings.TrimSpace(msg.StatusMessage))
		if msg.Stats != nil {
			m.stats = msg.Stats
			m.updateSummaryTable()
			m.updateDetailsTable()
		}
		if msg.Emails != nil {
			m.finishTimelineRangeSelection()
			m.timeline.emails = msg.Emails
			m.updateTimelineTable()
		}
		m.loadClassifications()
		if msg.FinishLoading {
			m.loading = false
			m.statusMessage = msg.StatusMessage
		}
		return m, nil

	case ContactSuggestionsMsg:
		m.suggestions = msg.Contacts
		if len(m.suggestions) == 0 {
			m.suggestionIdx = -1
		} else {
			m.suggestionIdx = 0
		}
		if m.windowWidth > 0 {
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
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
		logger.Debug("ValidIDsMsg: folder=%s validIDs=%d", m.currentFolder, len(msg.ValidIDs))
		// Background reconciliation has produced the live valid-ID set.
		// The backend's filterByValidIDs now applies automatically; reload all views.
		if isVirtualAllMailOnlyFolder(m.currentFolder) {
			return m, nil
		}
		if stats, err := m.backend.GetSenderStatistics(m.currentFolder); err == nil {
			m.stats = stats
		}
		m.loadClassifications()
		m.updateSummaryTable()
		m.updateDetailsTable()
		return m, m.loadTimelineEmails()

	case SyncEventMsg:
		event := msg.Event
		logger.Debug("SyncEventMsg: folder=%s generation=%d phase=%s current=%d total=%d delta=%d message=%q", event.Folder, event.Generation, event.Phase, event.Current, event.Total, event.EventCount, strings.TrimSpace(event.Message))
		if event.Generation < m.syncGeneration {
			logger.Debug("SyncEventMsg: ignoring stale generation=%d current=%d", event.Generation, m.syncGeneration)
			return m, m.listenForSyncEvents()
		}
		if event.Folder != "" && event.Folder != m.currentFolder {
			// Keep listening, but do not let an older folder repaint the visible one.
			if event.Generation > m.syncGeneration {
				m.syncGeneration = event.Generation
			}
			logger.Debug("SyncEventMsg: ignoring event for non-current folder=%s currentFolder=%s generation=%d", event.Folder, m.currentFolder, event.Generation)
			return m, m.listenForSyncEvents()
		}

		if event.Generation > m.syncGeneration {
			m.syncGeneration = event.Generation
			m.syncAccumulator.reset(event.Folder, event.Generation)
			logger.Debug("SyncEventMsg: advanced sync generation to %d for folder=%s", m.syncGeneration, event.Folder)
		}

		if event.Phase == models.SyncPhaseReconcileStarted {
			logger.Debug("SyncEventMsg: reconcile started for folder=%s generation=%d", event.Folder, event.Generation)
			return m, m.listenForSyncEvents()
		}

		if event.Message != "" {
			m.progressInfo.Message = event.Message
			m.progressInfo.Current = event.Current
			m.progressInfo.Total = event.Total
		}
		m.syncingFolder = event.Folder
		m.loading = true
		if event.Phase == models.SyncPhaseComplete {
			m.syncCountsSettled = true
		} else if event.Phase != models.SyncPhaseReconcileStarted {
			m.syncCountsSettled = false
		}

		action := m.syncAccumulator.observe(event)
		logger.Debug("SyncEventMsg: accumulator action folder=%s generation=%d armTimer=%v flushNow=%v", event.Folder, event.Generation, action.ArmTimer, action.FlushNow)
		cmds := []tea.Cmd{m.listenForSyncEvents()}
		if action.ArmTimer {
			cmds = append(cmds, scheduleSyncFlush(event.Folder, event.Generation, m.syncAccumulator.flushDelay))
		}
		if action.FlushNow {
			finish := event.Phase == models.SyncPhaseComplete || event.Phase == models.SyncPhaseError
			status := ""
			if event.Phase == models.SyncPhaseComplete {
				status = strings.TrimSpace(event.Message)
			}
			cmds = append(cmds, m.loadSyncSnapshotCmd(event.Folder, event.Generation, finish, status))
		}
		if event.Phase == models.SyncPhaseComplete {
			m.validIDsCh = m.backend.ValidIDsCh()
			logger.Debug("SyncEventMsg: sync complete for folder=%s generation=%d validIDsChSet=%v", event.Folder, event.Generation, m.validIDsCh != nil)
			if m.validIDsCh != nil {
				cmds = append(cmds, m.listenForValidIDs())
			}
			cmds = append(cmds, m.loadFolderStatusCmd([]string{event.Folder}, 0))
			if !isVirtualAllMailOnlyFolder(m.currentFolder) {
				m.syncStatusMode = ""
				cmds = append(cmds, m.loadFoldersCmd(0))
			}
		}
		if event.Phase == models.SyncPhaseError {
			m.statusMessage = event.Message
			m.loading = false
			logger.Debug("SyncEventMsg: sync error folder=%s generation=%d error=%q", event.Folder, event.Generation, strings.TrimSpace(event.Error))
		}
		return m, tea.Batch(cmds...)

	case SyncFlushMsg:
		if !m.syncAccumulator.shouldFlush(msg) {
			logger.Debug("SyncFlushMsg: ignored folder=%s generation=%d", msg.Folder, msg.Generation)
			return m, nil
		}
		logger.Debug("SyncFlushMsg: flushing folder=%s generation=%d", msg.Folder, msg.Generation)
		return m, m.loadSyncSnapshotCmd(msg.Folder, msg.Generation, false, "")

	case SyncHydratedMsg:
		if msg.Generation != 0 && msg.Generation != m.syncGeneration {
			logger.Debug("SyncHydratedMsg: ignoring stale generation=%d current=%d folder=%s", msg.Generation, m.syncGeneration, msg.Folder)
			return m, nil
		}
		if msg.Folder != "" && msg.Folder != m.currentFolder {
			logger.Debug("SyncHydratedMsg: ignoring non-current folder=%s currentFolder=%s", msg.Folder, m.currentFolder)
			return m, nil
		}
		if msg.Err != nil {
			logger.Warn("sync snapshot hydrate failed: %v", msg.Err)
			if msg.FinishLoading {
				m.loading = false
			}
			return m, nil
		}
		logger.Debug("SyncHydratedMsg: folder=%s generation=%d stats=%d emails=%d finish=%v status=%q", msg.Folder, msg.Generation, len(msg.Stats), len(msg.Emails), msg.FinishLoading, strings.TrimSpace(msg.StatusMessage))
		if msg.Stats != nil {
			m.stats = msg.Stats
			m.updateSummaryTable()
			m.updateDetailsTable()
		}
		if msg.Emails != nil {
			m.finishTimelineRangeSelection()
			m.timeline.emails = msg.Emails
			m.updateTimelineTable()
		}
		m.loadClassifications()
		if msg.FinishLoading {
			m.loading = false
			m.progressInfo = models.ProgressInfo{}
			m.statusMessage = msg.StatusMessage
			logger.Debug("SyncHydratedMsg: loading finished folder=%s generation=%d", msg.Folder, msg.Generation)
		}
		return m, nil

	case EmailBodyMsg:
		// If this body load was triggered from the Contacts tab, handle it there.
		if m.contactPreviewLoading {
			m.contactPreviewLoading = false
			if msg.Err != nil {
				m.contactPreviewBody = &models.EmailBody{TextPlain: "(Failed to load body)"}
			} else {
				m.contactPreviewBody = msg.Body
			}
			return m, nil
		}
		return m, nil

	case CleanupEmailBodyMsg:
		if m.cleanupPreviewEmail == nil || msg.MessageID != m.cleanupPreviewEmail.MessageID {
			return m, nil
		}
		m.cleanupBodyLoading = false
		m.revokeImagePreviews()
		if msg.Err != nil {
			logger.Warn("Failed to fetch cleanup email body: %v", msg.Err)
			m.cleanupEmailBody = &models.EmailBody{TextPlain: "(Failed to load body)"}
		} else {
			m.cleanupEmailBody = msg.Body
			m.cleanupBodyWrappedLines = nil // force rewrap on next render
		}
		m.clearCleanupPreviewDocumentCache()
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
			for _, e := range m.timeline.emails {
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

	case LoadingMsg:
		m.progressInfo = msg.Info
		logger.Debug("LoadingMsg: phase=%s current=%d total=%d message=%q", msg.Info.Phase, msg.Info.Current, msg.Info.Total, strings.TrimSpace(msg.Info.Message))
		return m, nil

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

		// Track connection state for the status bar indicator
		if msg.ConnectionLost {
			m.connectionLost = true
		} else if msg.Error == nil {
			m.connectionLost = false
		}

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
				m.revokeImagePreviews()
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
				m.clearCleanupPreviewDocumentCache()
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

			m.pruneTimelineStateAfterDeletion(msg)

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
			m.connectionLost = false
			m.resetCleanupSelection()
			m.clearTimelineSelection()

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
		for _, e := range m.timeline.emails {
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
		if msg.Notice != "" {
			m.statusMessage = msg.Notice
		}
		if msg.Done < msg.Total {
			return m, m.runEmbeddingBatch()
		}
		m.embeddingBatchActive = false
		return m, nil

	case EmbeddingDoneMsg:
		m.embeddingDone = 0
		m.embeddingTotal = 0
		m.embeddingBatchActive = false
		return m, nil

	case ContactEnrichedMsg:
		if msg.Notice != "" {
			m.contactStatusMessage = msg.Notice
		}
		if msg.Background && msg.Count > 0 {
			if msg.Notice == "" {
				m.contactStatusMessage = fmt.Sprintf("Enriched %d contacts", msg.Count)
			}
			return m, m.runContactEnrichment()
		}
		if msg.Background {
			m.contactEnrichmentActive = false
		}
		if !msg.Background && msg.Count > 0 && msg.Notice == "" {
			m.contactStatusMessage = fmt.Sprintf("Enriched %d contacts", msg.Count)
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
		return m, tea.ClearScreen

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

func (m *Model) View() tea.View {
	// OAuth wait overlay takes over the entire screen when active.
	if m.oauthWait != nil {
		return m.oauthWait.View()
	}
	if m.windowWidth > 0 && m.windowWidth < minTermWidth {
		return m.buildView(renderMinSizeMessage(m.windowWidth, m.windowHeight))
	}
	if m.windowHeight > 0 && m.windowHeight < minTermHeight {
		return m.buildView(renderMinSizeMessage(m.windowWidth, m.windowHeight))
	}
	if m.showSettings && m.settingsPanel != nil {
		return m.buildView(m.renderSettingsOverlayView())
	}
	if m.ruleDryRunPreview != nil {
		w, h := m.compactOverlayViewportSize()
		return m.buildView(m.renderCompactOverlayView(m.ruleDryRunPreview.renderPanel(w, h)))
	}
	if m.showRuleEditor && m.ruleEditor != nil {
		return m.buildView(m.renderCompactOverlayView(m.ruleEditor.renderPanel()))
	}
	if m.showPromptEditor && m.promptEditor != nil {
		return m.buildView(m.renderCompactOverlayView(m.promptEditor.renderPanel()))
	}
	if m.showCleanupMgr && m.cleanupManager != nil {
		return m.buildView(m.renderCompactOverlayView(m.cleanupManager.renderPanel()))
	}
	if m.showHelp {
		return m.buildView(m.renderShortcutHelpView())
	}
	if m.loading && !m.hasVisibleStartupData() {
		return m.buildView(m.renderLoadingView())
	}
	if m.timeline.fullScreen {
		return m.buildView(m.renderFullScreenEmail())
	}
	if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupFullScreen {
		return m.buildView(m.renderCleanupPreview())
	}
	return m.buildView(m.renderMainView())
}

func (m *Model) buildView(content string) tea.View {
	v := newHeraldView(content)
	if !m.timeline.mouseMode {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// handleKeyMsg handles keyboard input
func (m *Model) handleKeyMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model, cmd, handled := m.handleShortcutHelpKey(msg); handled {
		return model, cmd
	}

	if model, cmd, handled := m.handleGlobalCommandKey(msg); handled {
		return model, cmd
	}

	if model, cmd, handled := m.handleLogsOverlayKey(msg); handled {
		return model, cmd
	}

	if model, cmd, handled := m.handleOverlayKey(msg); handled {
		return model, cmd
	}

	key := shortcutKey(msg)

	// Reset pending 'yy' sequence on any key other than 'y'
	if m.timeline.pendingY && key != "y" {
		m.timeline.pendingY = false
	}

	// Compose screen gets its own key handler
	if m.activeTab == tabCompose {
		return m.handleComposeKey(msg)
	}

	// Contacts tab gets its own key handler (except for global keys handled below)
	if m.activeTab == tabContacts {
		if m.contactSearchMode != "" {
			return m.handleContactsKey(msg)
		}
		if !isGlobalContactsKey(key) {
			return m.handleContactsKey(msg)
		}
	}

	if model, cmd, handled := m.handleTabKey(msg); handled {
		return model, cmd
	}

	if model, cmd, handled := m.handleTimelineKey(msg); handled {
		return model, cmd
	}

	switch key {
	case "m":
		if m.activeTab == tabCleanup && m.showCleanupPreview {
			return m, m.toggleMouseCaptureMode()
		}
		return m, nil

	case "f":
		return m, m.toggleSidebar()

	case "d":
		if !m.loading {
			m.toggleDomainMode()
		}
		return m, nil

	case "r":
		return m, m.refreshCurrentFolder()

	case " ":
		if m.canInteractWithVisibleData() {
			if m.focusedPanel == panelSidebar {
				m.toggleSidebarNode()
			} else {
				m.toggleSelection()
			}
		}
		return m, nil

	case "D":
		if m.activeTab == tabTimeline {
			m.finishTimelineRangeSelection()
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil
		}
		if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupPreviewEmail != nil && !m.loading && !m.deleting {
			email := m.cleanupPreviewEmail
			m.cleanupPreviewDeleting = true
			m.cleanupPreviewIsArchive = false
			m.deleting = true
			m.deletionsPending++
			m.deletionsTotal++
			ch := m.deletionRequestCh
			go func() {
				ch <- models.DeletionRequest{
					MessageID:          email.MessageID,
					Folder:             email.Folder,
					IsArchive:          false,
					AffectedMessageIDs: []string{email.MessageID},
				}
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
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil
		}
		if !m.loading && m.classifier != nil {
			var target *models.EmailData
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupPreviewEmail != nil {
				target = m.cleanupPreviewEmail
			} else if m.activeTab == tabTimeline {
				cursor := m.timelineTable.Cursor()
				if cursor < len(m.timeline.threadRowMap) {
					ref := m.timeline.threadRowMap[cursor]
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
			if lister, ok := m.backend.(interface {
				GetAllRules() ([]*models.Rule, error)
			}); ok {
				rules, err := lister.GetAllRules()
				m.ruleEditor.WithSavedRules(rules, err)
			}
			m.showRuleEditor = true
			return m, m.ruleEditor.Init()
		}
		return m, nil

	case "P":
		if !m.showRuleEditor && !m.showPromptEditor && !m.showSettings {
			m.showPromptEditor = true
			m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
			prompts, err := m.backend.GetAllCustomPrompts()
			m.promptEditor.WithSavedPrompts(prompts, err)
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
			m.settingsPanel.setSize(m.windowWidth, m.windowHeight)
			return m, m.settingsPanel.Init()
		}
		return m, nil

	case "e":
		if m.activeTab == tabTimeline {
			m.finishTimelineRangeSelection()
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil
		}
		if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupPreviewEmail != nil && !m.loading && !m.deleting {
			email := m.cleanupPreviewEmail
			m.cleanupPreviewDeleting = true
			m.cleanupPreviewIsArchive = true
			m.deleting = true
			m.deletionsPending++
			m.deletionsTotal++
			ch := m.deletionRequestCh
			go func() {
				ch <- models.DeletionRequest{
					MessageID:          email.MessageID,
					Folder:             email.Folder,
					IsArchive:          true,
					AffectedMessageIDs: []string{email.MessageID},
				}
			}()
			return m, m.listenForDeletionResults()
		}
		if !m.loading && !m.deleting && !m.pendingDeleteConfirm {
			if m.activeTab == tabTimeline && m.timelineSelectedCount() > 0 && len(m.selectedTimelineArchiveEmails()) == 0 {
				m.statusMessage = "Selected drafts cannot be archived"
				return m, nil
			}
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

	case "u":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil
		}
		if m.blockCleanupReadOnlyMutation() {
			return m, nil
		}
		if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupPreviewEmail != nil && m.cleanupEmailBody != nil && m.cleanupEmailBody.ListUnsubscribe != "" {
			sender := m.cleanupPreviewEmail.Sender
			body := m.cleanupEmailBody
			m.pendingUnsubscribe = true
			m.pendingUnsubscribeDesc = fmt.Sprintf("Unsubscribe from %s?", sender)
			m.pendingUnsubscribeAction = func() tea.Cmd { return unsubscribeCmd(body) }
		}
		return m, nil

	case "h", "H":
		if m.activeTab == tabCleanup && !m.loading && !m.deleting {
			if m.showCleanupPreview && m.cleanupPreviewEmail != nil {
				return m, createHideFutureMailCmd(m.backend, m.cleanupPreviewEmail.Sender)
			}
			cursor := m.summaryTable.Cursor()
			if sender, ok := m.rowToSender[cursor]; ok && sender != "" {
				return m, createHideFutureMailCmd(m.backend, sender)
			}
		}
		return m, nil

	case "enter":
		if m.canInteractWithVisibleData() {
			if m.focusedPanel == panelSidebar {
				m.selectSidebarFolder()
				m.clearTimelineChatFilter()
				return m, m.activateCurrentFolder()
			} else if m.focusedPanel == panelDetails && m.activeTab == tabCleanup {
				// Open or scroll cleanup preview
				if m.showCleanupPreview {
					m.cleanupBodyScrollOffset++
				} else {
					idx := m.detailsTable.Cursor()
					if idx < len(m.detailsEmails) {
						email := m.detailsEmails[idx]
						m.revokeImagePreviews()
						m.cleanupPreviewEmail = email
						m.showCleanupPreview = true
						m.cleanupBodyLoading = true
						m.cleanupEmailBody = nil
						m.cleanupBodyScrollOffset = 0
						m.cleanupBodyWrappedLines = nil
						m.clearCleanupPreviewDocumentCache()
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

	case "z":
		if m.activeTab == tabCleanup && m.showCleanupPreview {
			m.cleanupFullScreen = !m.cleanupFullScreen
			m.cleanupBodyWrappedLines = nil // force re-wrap at new width
			m.clearCleanupPreviewDocumentCache()
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
			return m, nil
		}
		return m, nil

	case "esc":
		return m.handleEscKey()

	case "tab", "ctrl+i":
		if m.canInteractWithVisibleData() {
			m.cyclePanel(true)
		}
		return m, nil

	case "shift+tab":
		if m.canInteractWithVisibleData() {
			m.cyclePanel(false)
		}
		return m, nil

	case "l", "L":
		return m, m.toggleLogs()

	case "c":
		return m, m.toggleChat()

	case "q":
		m.cleanup()
		return m, tea.Quit

	case "a":
		if cmd := m.startClassificationIfNeeded(); cmd != nil {
			return m, cmd
		}
		return m, nil

	case "up", "k":
		if m.canInteractWithVisibleData() {
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupFullScreen {
				if m.cleanupBodyScrollOffset > 0 {
					m.cleanupBodyScrollOffset--
				}
				return m, nil
			}
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.focusedPanel == panelDetails {
				if m.cleanupBodyScrollOffset > 0 {
					m.cleanupBodyScrollOffset--
				}
				return m, nil
			}
			if m.activeTab != tabTimeline {
				return m.handleNavigation(-1)
			}
		}
		return m, nil

	case "down", "j":
		if m.canInteractWithVisibleData() {
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupFullScreen {
				m.cleanupBodyScrollOffset++
				return m, nil
			}
			if m.activeTab == tabCleanup && m.showCleanupPreview && m.focusedPanel == panelDetails {
				m.cleanupBodyScrollOffset++
				return m, nil
			}
			if m.activeTab != tabTimeline {
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
	content.WriteString(m.headerStyle.Render("📧 Herald") + "\n\n")

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

	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	chrome := m.chromeState(plan)

	// Header
	content.WriteString(m.renderTitleBar(m.windowWidth) + "\n")

	if syncStrip := m.renderTopSyncStrip(); syncStrip != "" {
		content.WriteString(syncStrip + "\n")
	}

	// Content area
	var mainContent string
	if m.showLogs {
		mainContent = m.baseStyle.Width(m.logViewer.viewport.Width() + 2).Render(m.logViewer.View().Content)
	} else if m.activeTab == tabTimeline {
		mainContent = m.renderTimelineView()
	} else if m.activeTab == tabCompose {
		mainContent = m.renderComposeView()
	} else if m.activeTab == tabContacts {
		mainContent = m.renderContactsTab(m.windowWidth, m.windowHeight)
	} else {
		// Cleanup tab
		if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupFullScreen {
			// Full-screen: the entire content area is the preview
			mainContent = m.renderCleanupPreview()
		} else {
			var summaryView string
			if m.stats != nil && len(m.stats) == 0 {
				summaryView = m.emptyStateView("No emails in this folder  •  press r to refresh")
			} else {
				summaryPanelStyle := m.baseStyle.
					Width(plan.Cleanup.SummaryWidth + 2).
					BorderForeground(defaultTheme.PanelBorderColor(false))
				if chrome.FocusedPanel == panelSummary {
					summaryPanelStyle = summaryPanelStyle.BorderForeground(defaultTheme.PanelBorderColor(true))
				}
				summaryStyles := m.inactiveTableStyle
				if chrome.FocusedPanel == panelSummary {
					summaryStyles = m.activeTableStyle
				}
				summaryView = summaryPanelStyle.Render(renderStyledTableViewWithCompactLeadingCell(&m.summaryTable, summaryStyles))
			}
			detailsPanelStyle := m.baseStyle.
				Width(plan.Cleanup.DetailsWidth + 2).
				BorderForeground(defaultTheme.PanelBorderColor(false))
			if chrome.FocusedPanel == panelDetails {
				detailsPanelStyle = detailsPanelStyle.BorderForeground(defaultTheme.PanelBorderColor(true))
			}
			detailsStyles := m.inactiveTableStyle
			if chrome.FocusedPanel == panelDetails {
				detailsStyles = m.activeTableStyle
			}
			detailsView := detailsPanelStyle.Render(renderStyledTableViewWithCompactLeadingCell(&m.detailsTable, detailsStyles))
			if m.showCleanupPreview {
				// 3-column layout: summary | details | preview (sidebar hidden while preview is open)
				previewPanel := m.renderCleanupPreview()
				if plan.Cleanup.SummaryWidth == 0 {
					mainContent = lipgloss.JoinHorizontal(lipgloss.Top, detailsView, panelGap, previewPanel)
				} else {
					mainContent = lipgloss.JoinHorizontal(lipgloss.Top, summaryView, panelGap, detailsView, panelGap, previewPanel)
				}
			} else if plan.SidebarVisible && !m.sidebarTooWide {
				sidebarStyle := m.baseStyle.
					Width(sidebarContentWidth + 2).
					BorderForeground(defaultTheme.PanelBorderColor(false))
				if chrome.FocusedPanel == panelSidebar {
					sidebarStyle = sidebarStyle.BorderForeground(defaultTheme.PanelBorderColor(true))
				}
				sidebarView := sidebarStyle.Render(m.renderSidebar())
				mainContent = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, panelGap, summaryView, panelGap, detailsView)
			} else {
				mainContent = lipgloss.JoinHorizontal(lipgloss.Top, summaryView, panelGap, detailsView)
			}
		}
	}
	if plan.ChatVisible {
		chatView := m.baseStyle.Width(chatPanelWidth + 2).Render(m.renderChatPanel())
		content.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, mainContent, panelGap, chatView) + "\n")
	} else {
		content.WriteString(mainContent + "\n")
	}

	// Status bar + key hints (persistent bottom bar)
	content.WriteString(m.renderStatusBar() + "\n")
	content.WriteString(m.renderStatusHintDivider() + "\n")
	content.WriteString(m.renderKeyHints())

	return content.String()
}

// Helper functions and other methods continue in next part due to length...
