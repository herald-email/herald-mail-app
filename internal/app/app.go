package app

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/accountcheck"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/aicheck"
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
	tabContacts = 2
	tabCalendar = 3
)

// LoadingMsg represents a loading state update
type LoadingMsg struct {
	Info models.ProgressInfo
}

// FoldersLoadedMsg carries the folder list fetched after connect
type FoldersLoadedMsg struct {
	SourceID         models.SourceID
	Folders          []string
	AccountSnapshots []backend.AccountFolderSnapshot
}

// FolderStatusMsg carries MESSAGES/UNSEEN counts for all folders
type FolderStatusMsg struct {
	SourceID models.SourceID
	Status   map[string]models.FolderStatus
}

// CalendarAgendaLoadedMsg carries cache-backed events for the Calendar tab.
type CalendarAgendaLoadedMsg struct {
	Events      []models.CalendarEvent
	Collections []models.CalendarCollection
	Err         error
}

// CalendarSearchLoadedMsg carries cache-backed search results for the Calendar tab.
type CalendarSearchLoadedMsg struct {
	Query  string
	Events []models.CalendarEvent
	Err    error
}

// CrossSourceSearchLoadedMsg carries blended cache-backed mail/event search
// results for the Calendar command-center foundation.
type CrossSourceSearchLoadedMsg struct {
	Query   string
	Results []models.CrossSourceSearchResult
	Err     error
}

// CalendarMeetingPrepMsg carries read-only cached context for the selected
// calendar event.
type CalendarMeetingPrepMsg struct {
	Ref  models.EventRef
	Prep models.CalendarMeetingPrep
	Err  error
}

// CalendarTravelBufferMsg carries read-only cached travel context for the
// selected calendar event.
type CalendarTravelBufferMsg struct {
	Ref    models.EventRef
	Buffer models.CalendarTravelBuffer
	Err    error
}

// CalendarAISummaryMsg carries read-only cached AI summary context for the
// selected calendar event.
type CalendarAISummaryMsg struct {
	Ref     models.EventRef
	Summary models.CalendarAISummary
	Err     error
}

// CalendarEventDetailMsg carries a selected read-only event detail.
type CalendarEventDetailMsg struct {
	Ref   models.EventRef
	Event *models.CalendarEvent
	Err   error
}

// CalendarEventSavedMsg carries a local/cache-backed calendar edit result.
type CalendarEventSavedMsg struct {
	Ref   models.EventRef
	Event *models.CalendarEvent
	Err   error
}

// CalendarEventRSVPMsg carries a provider-backed calendar RSVP result.
type CalendarEventRSVPMsg struct {
	Ref    models.EventRef
	Status string
	Event  *models.CalendarEvent
	Err    error
}

// CalendarInvitationSavedMsg carries an event imported from a mail invitation.
type CalendarInvitationSavedMsg struct {
	Ref   models.EventRef
	Event *models.CalendarEvent
	Err   error
}

// StartupHydratedMsg carries cached startup data used to progressively hydrate
// the UI while live IMAP loading continues in the background.
type StartupHydratedMsg struct {
	SourceID      models.SourceID
	Folder        string
	Emails        []*models.EmailData
	Err           error
	FinishLoading bool
	StatusMessage string
}

// TimelineLoadedMsg carries emails sorted by date for the timeline tab
type TimelineLoadedMsg struct {
	SourceID models.SourceID
	Folder   string
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
	MessageRef models.MessageRef
	MessageID  string
	Category   string
	Done       int
	Total      int
}

// ClassifyDoneMsg signals classification is complete
type ClassifyDoneMsg struct{}

// ReclassifyResultMsg carries the result of re-classifying a single email.
type ReclassifyResultMsg struct {
	MessageRef models.MessageRef
	MessageID  string
	Category   string
	Err        error
}

// ValidIDsMsg is sent when background reconciliation has determined the live
// set of valid message IDs from the server. All views should re-filter.
type ValidIDsMsg struct {
	ValidIDs map[string]bool
}

// EmailBodyMsg carries the result of fetching an email body from IMAP
type EmailBodyMsg struct {
	Body           *models.EmailBody
	Err            error
	MessageID      string // used to discard stale body fetches from rapid cursor movement
	Folder         string
	UID            uint32
	LoadSource     string
	LoadStartedAt  time.Time
	LoadFinishedAt time.Time
	LoadDuration   time.Duration
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
	ReplyAll  bool
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

type previewPrewarmMsg struct {
	Folder     string
	Generation int64
	Remaining  []previewPrewarmTarget
	Done       int
	Total      int
	Warmed     int
	Skipped    int
	Err        error
}

type CacheStoragePolicyAppliedMsg struct {
	Policy string
	Result models.PreviewCachePruneResult
	Err    error
}

type PreviewCacheReclaimEstimateMsg struct {
	Policy   string
	Estimate models.PreviewCacheStorageEstimate
	Err      error
}

type PreviewCacheReclaimMsg struct {
	Policy string
	Result models.PreviewCacheReclaimResult
	Err    error
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
	SourceID  models.SourceID
	AccountID models.AccountID
	Emails    []*models.EmailData
	Folder    string
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
	Folder     string
	Generation int64
	Done       int
	Total      int
	Notice     string
}

// EmbeddingDoneMsg signals background embedding finished
type EmbeddingDoneMsg struct {
	Folder     string
	Generation int64
}

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
	MessageRef models.MessageRef
	MessageID  string
	Category   string
	Err        error
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
	Generation int64
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
	UID             uint32
	Folder          string
	SourceID        models.SourceID
	ReplaceUID      uint32
	ReplaceFolder   string
	ReplaceSourceID models.SourceID
	Err             error
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
	Result   string
	Original string
	Err      error
}

// AISubjectMsg carries the result of an AI subject-suggestion request.
type AISubjectMsg struct {
	Subject string
	Err     error
}

// AccountValidationMsg carries setup/account-settings connection validation.
type AccountValidationMsg struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	ValidateCalendar           bool
	CalendarSourceIDs          []models.SourceID
	Result                     accountcheck.Result
}

type CalendarValidationMsg struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	SourceIDs                  []models.SourceID
	Err                        error
}

type OllamaModelValidationMsg struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	Result                     aicheck.Result
}

type OllamaModelWarningMsg struct {
	Result aicheck.Result
}

type accountValidationState struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	ValidateCalendar           bool
	CalendarSourceIDs          []models.SourceID
	Checking                   bool
	Message                    string
}

type aiModelValidationState struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	Checking                   bool
	Message                    string
}

var validateAccountConfig = accountcheck.Validate
var validateCalendarConfig = validateCalendarSources
var validateOllamaModels = aicheck.ValidateOllamaModels

var newValidatedLocalBackend = func(cfg *config.Config, configPath string, classifier ai.AIClient) (backend.Backend, error) {
	mailSources := 0
	if cfg != nil {
		for _, source := range cfg.NormalizedSources() {
			if strings.TrimSpace(source.Kind) == "" || source.Kind == string(models.SourceKindMail) {
				mailSources++
			}
		}
	}
	if mailSources > 1 {
		return backend.NewMultiLocal(cfg, configPath, classifier)
	}
	return backend.NewLocal(cfg, configPath, classifier)
}

func validateCalendarSources(ctx context.Context, cfg *config.Config, sourceIDs []models.SourceID) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	want := make(map[models.SourceID]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		if id != "" {
			want[id] = true
		}
	}
	checked := 0
	for _, source := range cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) != string(models.SourceKindCalendar) {
			continue
		}
		sourceID := models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID)
		if len(want) > 0 && !want[sourceID] {
			continue
		}
		logCalendarValidationSource(source)
		opened, err := backend.DefaultSourceRegistry().Open(ctx, source, backend.SourceDeps{ProfileConfig: cfg})
		if err != nil {
			return fmt.Errorf("calendar source %s failed to open: %w", source.ID, err)
		}
		if opened.Calendar == nil {
			_ = opened.Close()
			return fmt.Errorf("calendar source %s did not provide a calendar adapter", source.ID)
		}
		if _, err := opened.Calendar.ListCalendars(ctx); err != nil {
			_ = opened.Close()
			return fmt.Errorf("calendar source %s failed validation: %w", source.ID, err)
		}
		_ = opened.Close()
		checked++
	}
	if checked == 0 {
		return fmt.Errorf("no calendar sources to validate")
	}
	return nil
}

func logCalendarValidationSource(source config.SourceConfig) {
	if strings.TrimSpace(source.Provider) != "caldav" {
		logger.Debug("Calendar validation source: id=%s provider=%s account=%s", source.ID, source.Provider, source.AccountID)
		return
	}
	host, pathPresent := sanitizedCalendarValidationHostAndPath(source.CalDAV.URL)
	username := strings.TrimSpace(source.CalDAV.Username)
	logger.Debug(
		"Calendar validation source: id=%s provider=%s account=%s caldav_host=%s caldav_path_present=%t username_present=%t username_has_at=%t password_present=%t",
		source.ID,
		source.Provider,
		source.AccountID,
		host,
		pathPresent,
		username != "",
		strings.Contains(username, "@"),
		source.CalDAV.Password != "",
	)
}

func sanitizedCalendarValidationHostAndPath(rawURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "invalid", false
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Host))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		host = "empty"
	}
	return host, strings.Trim(parsed.EscapedPath(), "/") != ""
}

// Model represents the main application state
type Model struct {
	backend      backend.Backend
	progressCh   <-chan models.ProgressInfo
	syncEventsCh <-chan models.FolderSyncEvent

	// UI State
	loading                   bool
	deleting                  bool
	deletionProgress          models.DeletionResult
	deletionsPending          int  // Number of deletions waiting to complete
	deletionsTotal            int  // Total deletions in current batch
	connectionLost            bool // true while IMAP connection is down during deletion
	loadingSpinner            int
	startTime                 time.Time
	progressInfo              models.ProgressInfo
	syncAccumulator           syncAccumulator
	syncGeneration            int64
	syncSourceGenerations     map[models.SourceID]int64
	syncingFolder             string
	syncCountsSettled         bool
	showLogs                  bool
	showHelp                  bool
	helpScrollOffset          int
	helpSearchActive          bool
	helpSearch                string
	keyEventTypes             bool
	activeHintMods            tea.KeyMod
	modifierHintMods          tea.KeyMod
	modifierHintFallbackToken int
	windowWidth               int
	windowHeight              int
	subjectColWidth           int

	// Deletion channels
	deletionRequestCh chan models.DeletionRequest
	deletionResultCh  chan models.DeletionResult

	// Rule engine channels
	ruleRequestCh   chan models.AutomationEvent
	ruleResultCh    chan models.RuleResult
	rulesFiredCount int // total rules fired across session

	// Classification channel (buffered; one result per email)
	classifyCh chan ClassifyProgressMsg

	// validIDsCh receives the live valid-ID set from background reconciliation.
	validIDsCh <-chan map[string]bool

	// Tables
	timelineTable table.Model
	logViewer     *LogViewer

	// Display options
	currentFolder  string
	sidebarTooWide bool // set by layout when sidebar + terminal width leaves < 16 variable cols

	// Tabs
	activeTab int // tabTimeline, tabCompose, or tabContacts

	// Timeline
	timeline TimelineState

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
	composeSourceID    models.SourceID
	composeTo          textinput.Model
	composeCC          textinput.Model
	composeBCC         textinput.Model
	composeCCExpanded  bool
	composeBCCExpanded bool
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
	fieldKeyMode       string

	// Autocomplete (compose address fields)
	suggestions   []models.ContactData // current autocomplete candidates (empty = dropdown hidden)
	suggestionIdx int                  // selected row index (-1 = none selected)

	// Compose AI toolbar (Ctrl+K)
	composeAIPanel        bool
	composeAIInput        textinput.Model // free-form prompt
	composeAIResponse     textarea.Model  // editable AI rewrite
	composeAIOriginal     string          // original draft shown while reviewing an AI rewrite
	composeAIShowOriginal bool            // true = review original instead of editable suggestion
	composeAIDiff         string          // lipgloss-styled word diff (display only)
	composeAILoading      bool
	composeAIThread       bool              // true = include reply context in prompt
	composeAIMenu         string            // open command-bar dropdown: translate/style
	composeAIStyle        string            // selected style dropdown option
	composeAITranslate    string            // selected translation language
	composeAIUndoBody     string            // previous body before accepted AI rewrite
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
	lastDraftSourceID    models.SourceID
	lastDraftReplaceable bool // true when lastDraftUID points at a canonical Drafts-folder copy
	draftSaving          bool // true while a SaveDraft cmd is in flight (prevents concurrent saves)

	// Sidebar
	folders                []string
	folderTree             []*folderNode
	folderStatus           map[string]models.FolderStatus
	accountFolderSnapshots []backend.AccountFolderSnapshot
	sidebarExpanded        map[string]bool
	showSidebar            bool
	sidebarCursor          int
	focusedPanel           int // panelSidebar, panelSummary, panelDetails

	// Multi-account chrome. These fields are populated only when the backend
	// implements backend.AccountAwareBackend; single-account sessions hide them.
	accounts               []backend.AccountInfo
	accountStatuses        map[models.SourceID]backend.AccountStatus
	activeSourceID         models.SourceID
	activeAccountID        models.AccountID
	accountSelectedFolders map[models.SourceID]string
	showAccountSwitcher    bool
	accountSwitcherCursor  int

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
	embeddingDone            int
	embeddingTotal           int
	embeddingBatchActive     bool
	contactEnrichmentActive  bool
	backgroundWorkGeneration int64
	previewPrewarmActive     bool
	previewPrewarmDone       int
	previewPrewarmTotal      int
	previewPrewarmWarmed     int
	previewPrewarmSkipped    int

	// Demo mode — set when DemoBackend is detected; shows [DEMO] in status bar
	demoMode           bool
	showDemoWelcome    bool
	demoKeyOverlay     bool
	demoKeyOverlayKeys []string

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
	cfg          *config.Config
	configPath   string
	keyboard     *KeyboardResolver
	keyboardWarn string
	theme        Theme
	themeWarn    string

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

	// Account validation overlay used when saving first-class mail account settings.
	accountValidation *accountValidationState

	// Ollama model validation and advisory warning state for local AI setup.
	aiModelValidation *aiModelValidationState
	aiModelWarning    *aicheck.Result

	// Local inline-image preview links. Disabled for SSH sessions, where
	// localhost would point at the server instead of the user's browser.
	localImageLinks   bool
	previewImageMode  previewImageMode
	imagePreviewLinks *imagePreviewServer

	// General status message (shown briefly after actions like settings save)
	statusMessage string

	pendingPreviewCacheReclaim  bool
	previewCacheReclaimPolicy   string
	previewCacheReclaimEstimate models.PreviewCacheStorageEstimate

	// Contacts-only status message. This is cleared when leaving the contacts
	// workflow so contact actions do not leak stale notices into other tabs.
	contactStatusMessage string

	// Calendar tab. Calendar is additive and appears only when the backend
	// advertises a cache-backed read-only agenda surface.
	calendarAvailable           bool
	calendarLoading             bool
	calendarCollections         []models.CalendarCollection
	calendarEvents              []models.CalendarEvent
	calendarCursor              int
	calendarRailCursor          int
	calendarFocus               calendarFocusPanel
	calendarHiddenCollections   map[string]bool
	calendarView                calendarViewMode
	calendarSearchQuery         string
	calendarSearchResults       []models.CalendarEvent
	calendarSearchCursor        int
	calendarSearchLoading       bool
	crossSourceSearchQuery      string
	crossSourceSearchResults    []models.CrossSourceSearchResult
	crossSourceSearchCursor     int
	crossSourceSearchLoading    bool
	calendarDay                 time.Time
	calendarWeekStart           time.Time
	calendarThreeDayStart       time.Time
	calendarDetailOpen          bool
	calendarDetailLoading       bool
	calendarDetail              *models.CalendarEvent
	calendarMeetingPrepOpen     bool
	calendarMeetingPrepLoading  bool
	calendarMeetingPrep         *models.CalendarMeetingPrep
	calendarTravelBufferOpen    bool
	calendarTravelBufferLoading bool
	calendarTravelBuffer        *models.CalendarTravelBuffer
	calendarAISummaryOpen       bool
	calendarAISummaryLoading    bool
	calendarAISummary           *models.CalendarAISummary
	calendarEdit                calendarEventEditState
	calendarInvitation          calendarInvitationPromptState
	calendarStatus              string

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
	theme := defaultTheme
	baseStyle := theme.BasePanelStyle()
	headerStyle := theme.TitleBarStyle()
	loadingStyle := theme.LoadingStyle()
	progressStyle := theme.ProgressStyle()

	// Create role-driven table styles shared by list-like panes.
	activeStyle := theme.TableStyles(true)
	inactiveStyle := theme.TableStyles(false)

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
	composeAIInput.Placeholder = " custom instruction..."
	composeAIInput.CharLimit = 512

	composeAIResponse := textarea.New()
	composeAIResponse.Placeholder = "AI suggestion will appear here..."
	composeAIResponse.SetWidth(38)
	composeAIResponse.SetHeight(8)
	composeAIResponse.CharLimit = 0

	// Create deletion channels
	deletionRequestCh := make(chan models.DeletionRequest, 10)
	deletionResultCh := make(chan models.DeletionResult, 10)

	// Create rule engine channels
	ruleRequestCh := make(chan models.AutomationEvent, 20)
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
		backend:         b,
		progressCh:      b.Progress(),
		syncEventsCh:    b.SyncEvents(),
		loading:         true,
		dryRun:          dryRun,
		startTime:       time.Now(),
		currentFolder:   "INBOX",
		folders:         []string{"INBOX"},
		folderTree:      buildFolderTree([]string{"INBOX"}),
		folderStatus:    make(map[string]models.FolderStatus),
		sidebarExpanded: make(map[string]bool),
		showSidebar:     true,
		focusedPanel:    panelTimeline,
		accountStatuses: make(map[models.SourceID]backend.AccountStatus),
		accountSelectedFolders: map[models.SourceID]string{
			models.DefaultMailSourceID: "INBOX",
		},
		timelineTable:   timelineTable,
		logViewer:       logViewer,
		chatInput:       chatInput,
		classifier:      classifier,
		classifications: make(map[string]string),
		timeline: TimelineState{
			expandedThreads:     make(map[string]bool),
			selectedMessageIDs:  make(map[string]bool),
			searchInput:         searchInput,
			attachmentSaveInput: attachmentSaveInput,
		},
		calendarFocus:             calendarFocusMain,
		calendarHiddenCollections: make(map[string]bool),
		classifyCh:                make(chan ClassifyProgressMsg, 50),
		syncAccumulator:           newSyncAccumulator(defaultSyncFlushCount, defaultSyncFlushDelay),
		syncSourceGenerations:     make(map[models.SourceID]int64),
		mailer:                    mailer,
		fromAddress:               fromAddress,
		composeTo:                 composeTo,
		composeCC:                 composeCC,
		composeBCC:                composeBCC,
		composeSubject:            composeSubject,
		composeBody:               composeBody,
		suggestionIdx:             -1,
		composeAIPanel:            true,
		composeAIInput:            composeAIInput,
		composeAIResponse:         composeAIResponse,
		attachmentCompletionIdx:   -1,
		localImageLinks:           true,
		previewImageMode:          previewImageModeAuto,
		imagePreviewLinks:         newImagePreviewServer(),
		theme:                     theme,
		baseStyle:                 baseStyle,
		headerStyle:               headerStyle,
		loadingStyle:              loadingStyle,
		progressStyle:             progressStyle,
		activeTableStyle:          activeStyle,
		inactiveTableStyle:        inactiveStyle,
		deletionRequestCh:         deletionRequestCh,
		deletionResultCh:          deletionResultCh,
		ruleRequestCh:             ruleRequestCh,
		ruleResultCh:              ruleResultCh,
		attachmentPathInput:       attachmentPathInput,
		keyboard:                  NewKeyboardResolver(nil),
	}

	m.syncAccountsFromBackend()
	m.refreshCalendarAvailability()

	// Detect demo mode via DemoBackendMarker interface
	if marker, ok := b.(interface{ IsDemo() bool }); ok && marker.IsDemo() {
		m.demoMode = true
		m.showDemoWelcome = true
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
	m.applyKeyboardConfig(cfg)
	m.applyThemeConfig(cfg)
}

func validateAccountSettingsCmd(cfg *config.Config, configPath string, returnToMenu, reclaimOfflineCache, validateCalendar bool, calendarSourceIDs []models.SourceID) tea.Cmd {
	return func() tea.Msg {
		result := validateAccountConfig(context.Background(), cfg, configPath)
		return AccountValidationMsg{
			Config:                     cfg,
			ReturnToMenu:               returnToMenu,
			ReclaimOfflineCacheStorage: reclaimOfflineCache,
			ValidateCalendar:           validateCalendar,
			CalendarSourceIDs:          calendarSourceIDs,
			Result:                     result,
		}
	}
}

func validateCalendarSettingsCmd(cfg *config.Config, returnToMenu, reclaimOfflineCache bool, sourceIDs []models.SourceID) tea.Cmd {
	return func() tea.Msg {
		return CalendarValidationMsg{
			Config:                     cfg,
			ReturnToMenu:               returnToMenu,
			ReclaimOfflineCacheStorage: reclaimOfflineCache,
			SourceIDs:                  sourceIDs,
			Err:                        validateCalendarConfig(context.Background(), cfg, sourceIDs),
		}
	}
}

func validateOllamaModelsCmd(cfg *config.Config, returnToMenu, reclaimOfflineCache bool) tea.Cmd {
	return func() tea.Msg {
		result := validateOllamaModels(context.Background(), cfg)
		return OllamaModelValidationMsg{
			Config:                     cfg,
			ReturnToMenu:               returnToMenu,
			ReclaimOfflineCacheStorage: reclaimOfflineCache,
			Result:                     result,
		}
	}
}

func ollamaModelWarningCmd(cfg *config.Config, demoMode bool) tea.Cmd {
	if demoMode || !aicheck.OllamaConfigured(cfg) {
		return nil
	}
	return func() tea.Msg {
		return OllamaModelWarningMsg{Result: validateOllamaModels(context.Background(), cfg)}
	}
}

func aiRuntimeConfigChanged(previous, next *config.Config) bool {
	if previous == nil || next == nil {
		return previous != next
	}
	return previous.AI.Provider != next.AI.Provider ||
		previous.AI.LocalMaxConcurrency != next.AI.LocalMaxConcurrency ||
		previous.AI.ExternalMaxConcurrency != next.AI.ExternalMaxConcurrency ||
		previous.AI.BackgroundQueueLimit != next.AI.BackgroundQueueLimit ||
		previous.AI.PauseBackgroundWhileInteractive != next.AI.PauseBackgroundWhileInteractive ||
		previous.Ollama.Host != next.Ollama.Host ||
		previous.Ollama.Model != next.Ollama.Model ||
		previous.Claude.APIKey != next.Claude.APIKey ||
		previous.Claude.Model != next.Claude.Model ||
		previous.OpenAI.APIKey != next.OpenAI.APIKey ||
		previous.OpenAI.BaseURL != next.OpenAI.BaseURL ||
		previous.OpenAI.Model != next.OpenAI.Model
}

func aiEmbeddingConfigChanged(previous, next *config.Config) bool {
	if previous == nil || next == nil {
		return previous != next
	}
	return previous.EffectiveEmbeddingModel() != next.EffectiveEmbeddingModel()
}

func (m *Model) syncBackendAIClient(classifier ai.AIClient) {
	type aiClientSetter interface {
		SetAIClient(ai.AIClient)
	}
	if manager, ok := m.backend.(aiClientSetter); ok {
		manager.SetAIClient(classifier)
	}
}

func (m *Model) refreshAIClientForConfig(previous, next *config.Config) error {
	rebuild := aiRuntimeConfigChanged(previous, next) || (m.classifier == nil && aiEmbeddingConfigChanged(previous, next))
	if rebuild {
		if next == nil {
			m.classifier = nil
			m.syncBackendAIClient(nil)
			return nil
		}
		classifier, err := ai.NewFromConfig(next)
		if err != nil {
			return err
		}
		m.classifier = classifier
		m.syncBackendAIClient(classifier)
		return nil
	}
	if m.classifier != nil && next != nil {
		m.classifier.SetEmbeddingModel(next.EffectiveEmbeddingModel())
		m.syncBackendAIClient(m.classifier)
	}
	return nil
}

func (m *Model) beginAccountValidation(cfg *config.Config, returnToMenu, reclaimOfflineCache, validateCalendar bool, calendarSourceIDs []models.SourceID) tea.Cmd {
	m.showSettings = false
	m.settingsPanel = nil
	m.oauthWait = nil
	m.accountValidation = &accountValidationState{
		Config:                     cfg,
		ReturnToMenu:               returnToMenu,
		ReclaimOfflineCacheStorage: reclaimOfflineCache,
		ValidateCalendar:           validateCalendar,
		CalendarSourceIDs:          calendarSourceIDs,
		Checking:                   true,
		Message:                    "Checking IMAP and SMTP before saving account settings...",
	}
	m.statusMessage = "Validating account settings..."
	logger.Info("Validating account settings before applying config")
	return validateAccountSettingsCmd(cfg, m.configPath, returnToMenu, reclaimOfflineCache, validateCalendar, calendarSourceIDs)
}

func (m *Model) beginCalendarValidation(cfg *config.Config, returnToMenu, reclaimOfflineCache bool, sourceIDs []models.SourceID) tea.Cmd {
	m.showSettings = false
	m.settingsPanel = nil
	m.oauthWait = nil
	m.accountValidation = &accountValidationState{
		Config:                     cfg,
		ReturnToMenu:               returnToMenu,
		ReclaimOfflineCacheStorage: reclaimOfflineCache,
		CalendarSourceIDs:          sourceIDs,
		Checking:                   true,
		Message:                    "Checking calendar source before saving settings...",
	}
	m.statusMessage = "Validating calendar settings..."
	logger.Info("Validating calendar source settings before applying config")
	return validateCalendarSettingsCmd(cfg, returnToMenu, reclaimOfflineCache, sourceIDs)
}

func (m *Model) beginOllamaModelValidation(cfg *config.Config, returnToMenu, reclaimOfflineCache bool) tea.Cmd {
	m.showSettings = false
	m.settingsPanel = nil
	m.oauthWait = nil
	m.aiModelValidation = &aiModelValidationState{
		Config:                     cfg,
		ReturnToMenu:               returnToMenu,
		ReclaimOfflineCacheStorage: reclaimOfflineCache,
		Checking:                   true,
		Message:                    "Checking local Ollama models before saving AI settings...",
	}
	m.statusMessage = "Validating Ollama models..."
	logger.Info("Validating configured Ollama models before applying AI settings")
	return validateOllamaModelsCmd(cfg, returnToMenu, reclaimOfflineCache)
}

func (m *Model) finishAccountValidation(msg AccountValidationMsg) tea.Cmd {
	if m.accountValidation == nil {
		m.accountValidation = &accountValidationState{}
	}
	m.accountValidation.Checking = false
	m.accountValidation.Config = msg.Config
	m.accountValidation.ReturnToMenu = msg.ReturnToMenu
	m.accountValidation.ReclaimOfflineCacheStorage = msg.ReclaimOfflineCacheStorage
	if err := msg.Result.Err(); err != nil {
		m.accountValidation.Message = msg.Result.UserMessage(logger.Path(), m.configPath)
		m.statusMessage = "Account settings were not saved."
		logger.Error("Account settings validation failed: %v", err)
		return nil
	}
	if msg.ValidateCalendar {
		return m.beginCalendarValidation(msg.Config, msg.ReturnToMenu, msg.ReclaimOfflineCacheStorage, msg.CalendarSourceIDs)
	}
	return m.applyValidatedAccountConfig(msg.Config, msg.ReturnToMenu)
}

func (m *Model) finishCalendarValidation(msg CalendarValidationMsg) tea.Cmd {
	if m.accountValidation == nil {
		m.accountValidation = &accountValidationState{}
	}
	m.accountValidation.Checking = false
	m.accountValidation.Config = msg.Config
	m.accountValidation.ReturnToMenu = msg.ReturnToMenu
	m.accountValidation.ReclaimOfflineCacheStorage = msg.ReclaimOfflineCacheStorage
	m.accountValidation.CalendarSourceIDs = msg.SourceIDs
	if msg.Err != nil {
		m.accountValidation.Message = fmt.Sprintf("Calendar settings were not saved. %s", calendarValidationFailureMessage(msg.Config, msg.SourceIDs, msg.Err))
		m.statusMessage = "Calendar settings were not saved."
		logger.Error("Calendar settings validation failed: %v", msg.Err)
		return nil
	}
	return m.applyValidatedAccountConfig(msg.Config, msg.ReturnToMenu)
}

func calendarValidationFailureMessage(cfg *config.Config, sourceIDs []models.SourceID, err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "validation failed"
	}
	if calendarValidationNeedsICloudGuidance(cfg, sourceIDs, message) && !strings.Contains(strings.ToLower(message), "apple app-specific password") {
		message += ". " + iCloudCalDAVSettingsGuidance()
	}
	return message
}

func calendarValidationNeedsICloudGuidance(cfg *config.Config, sourceIDs []models.SourceID, message string) bool {
	if cfg == nil || !looksLikeCalendarAuthFailure(message) {
		return false
	}
	want := make(map[models.SourceID]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		if id != "" {
			want[id] = true
		}
	}
	for _, source := range cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) != string(models.SourceKindCalendar) || strings.TrimSpace(source.Provider) != "caldav" {
			continue
		}
		sourceID := models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID)
		if len(want) > 0 && !want[sourceID] {
			continue
		}
		if isICloudCalDAVSettingsURL(source.CalDAV.URL) {
			return true
		}
	}
	return false
}

func looksLikeCalendarAuthFailure(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "forbidden") ||
		strings.Contains(message, "401") ||
		strings.Contains(message, "403")
}

func isICloudCalDAVSettingsURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Host))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.TrimSuffix(host, ".")
	return host == "caldav.icloud.com" || strings.HasSuffix(host, ".icloud.com")
}

func iCloudCalDAVSettingsGuidance() string {
	return "For iCloud Calendar, use your Apple Account email and an Apple app-specific password. If you changed your Apple Account password, generate a new app-specific password. Apple Account two-factor authentication must be enabled."
}

func (m *Model) finishOllamaModelValidation(msg OllamaModelValidationMsg) tea.Cmd {
	if m.aiModelValidation == nil {
		m.aiModelValidation = &aiModelValidationState{}
	}
	m.aiModelValidation.Checking = false
	m.aiModelValidation.Config = msg.Config
	m.aiModelValidation.ReturnToMenu = msg.ReturnToMenu
	m.aiModelValidation.ReclaimOfflineCacheStorage = msg.ReclaimOfflineCacheStorage
	if err := msg.Result.Err(); err != nil {
		warning := msg.Result
		m.aiModelWarning = &warning
		message := "AI setup failed. " + msg.Result.UserMessage(logger.Path(), m.configPath)
		m.aiModelValidation.Message = message
		m.statusMessage = "AI settings were not saved."
		logger.Error("Ollama model validation failed: %v", err)
		m.showSettings = true
		m.settingsPanel = m.newSettingsPanel(settingsPanelSectionAI, msg.Result.CompactMessage())
		m.aiModelValidation = nil
		return m.settingsPanel.Init()
	}
	m.aiModelWarning = nil
	m.aiModelValidation = nil
	return m.applySettingsSaved(SettingsSavedMsg{
		Config:                     msg.Config,
		ReturnToMenu:               msg.ReturnToMenu,
		ReclaimOfflineCacheStorage: msg.ReclaimOfflineCacheStorage,
	})
}

func (m *Model) applyValidatedAccountConfig(cfg *config.Config, returnToMenu bool) tea.Cmd {
	if cfg == nil {
		m.accountValidation.Message = "Account validation finished without a config. Settings were not saved."
		m.statusMessage = "Account settings were not saved."
		return nil
	}
	nextBackend, err := newValidatedLocalBackend(cfg, m.configPath, m.classifier)
	if err != nil {
		m.accountValidation.Message = fmt.Sprintf("Account validated, but Herald could not prepare the mailbox backend: %v. Settings were not applied.", err)
		m.statusMessage = "Account settings were not applied."
		logger.Error("Validated account backend replacement failed: %v", err)
		return nil
	}
	if m.configPath != "" {
		if err := cfg.Save(m.configPath); err != nil {
			_ = nextBackend.Close()
			m.accountValidation.Message = fmt.Sprintf("Account validated, but config write failed: %v. Settings were not saved.", err)
			m.statusMessage = "Account settings were not saved."
			logger.Error("Account config save failed after validation: %v", err)
			return nil
		}
	}

	oldBackend := m.backend
	m.backend = nextBackend
	m.progressCh = nextBackend.Progress()
	m.syncEventsCh = nextBackend.SyncEvents()
	if oldBackend != nil && oldBackend != nextBackend {
		go func() {
			if err := oldBackend.Close(); err != nil {
				logger.Warn("Failed to close previous backend after account switch: %v", err)
			}
		}()
	}

	m.SetConfig(cfg)
	m.mailer = appsmtp.New(cfg)
	m.fromAddress = accountEmailAddress(cfg)
	m.resetMailboxStateForAccountSwitch()
	m.refreshCalendarAvailability()
	m.accountValidation = nil
	m.statusMessage = "Account settings validated and saved. Reconnecting..."

	var cmds []tea.Cmd
	if returnToMenu {
		m.showSettings = true
		m.settingsPanel = m.newSettingsPanel("", "Account settings validated and saved.")
		cmds = append(cmds, m.settingsPanel.Init())
	}
	cmds = append(cmds, m.startLoading(), m.listenForSyncEvents(), m.tickSpinner())
	return tea.Batch(cmds...)
}

func accountEmailAddress(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.IsGmailOAuth() && strings.TrimSpace(cfg.Gmail.Email) != "" {
		return strings.TrimSpace(cfg.Gmail.Email)
	}
	if strings.TrimSpace(cfg.Credentials.Username) != "" {
		return strings.TrimSpace(cfg.Credentials.Username)
	}
	return strings.TrimSpace(cfg.Gmail.Email)
}

func (m *Model) newSettingsPanel(section settingsPanelSection, status string) *Settings {
	panel := NewSettingsWithPath(SettingsModePanel, m.cfg, m.configPath)
	panel.aiModelWarning = m.aiModelWarning
	if section != "" {
		panel.panelSection = section
	}
	panel.panelStatus = status
	panel.buildForm()
	panel.setSize(m.windowWidth, m.windowHeight)
	return panel
}

func (m *Model) openSettingsCleanupTool(tool string) tea.Cmd {
	m.showSettings = false
	m.settingsPanel = nil

	switch tool {
	case settingsCleanupToolAutomation:
		m.ruleEditor = NewRuleEditor("", "", m.windowWidth, m.windowHeight)
		if lister, ok := m.backend.(interface {
			GetAllRules() ([]*models.Rule, error)
		}); ok {
			rules, err := lister.GetAllRules()
			m.ruleEditor.WithSavedRules(rules, err)
		}
		m.showRuleEditor = true
		return m.ruleEditor.Init()
	case settingsCleanupToolPrompts:
		m.showPromptEditor = true
		m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
		if m.backend != nil {
			prompts, err := m.backend.GetAllCustomPrompts()
			m.promptEditor.WithSavedPrompts(prompts, err)
		}
		return m.promptEditor.Init()
	case settingsCleanupToolRules:
		m.cleanupManager = NewCleanupManager(m.backend, m.windowWidth, m.windowHeight)
		m.showCleanupMgr = true
		return m.cleanupManager.Init()
	default:
		m.statusMessage = "Unknown cleanup settings tool."
		return nil
	}
}

func oauthStartFailureMessage(err error) string {
	if err == nil {
		return "OAuth could not start. Settings were not saved."
	}
	parts := []string{"OAuth could not start: " + err.Error() + ". Settings were not saved."}
	if path := logger.Path(); path != "" {
		parts = append(parts, "Debug log: "+path)
	}
	return strings.Join(parts, " ")
}

func (m *Model) resetMailboxStateForAccountSwitch() {
	m.resetMailboxStateForFolder("INBOX")
}

func (m *Model) resetMailboxStateForFolder(folder string) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = "INBOX"
	}
	m.syncGeneration = 0
	m.syncSourceGenerations = make(map[models.SourceID]int64)
	m.syncAccumulator.reset("", 0)
	m.syncCountsSettled = false
	m.syncingFolder = ""
	m.progressInfo = models.ProgressInfo{}
	m.validIDsCh = nil
	m.currentFolder = folder
	m.folders = []string{folder}
	m.folderTree = buildFolderTree(m.folders)
	m.folderStatus = make(map[string]models.FolderStatus)
	m.sidebarCursor = 0
	m.focusedPanel = panelTimeline
	m.timeline.emails = nil
	m.timeline.emailsCache = nil
	m.timeline.selectedEmail = nil
	m.timeline.selectedMessageIDs = make(map[string]bool)
	m.timeline.searchResults = nil
	m.timeline.searchMode = false
	m.timeline.searchError = ""
	m.timeline.searchToken++
	m.timeline.fullScreen = false
	m.timelineTable.SetRows([]table.Row{})
	m.timelineTable.SetCursor(0)
	m.classifications = make(map[string]string)
}

func (m *Model) applyThemeConfig(cfg *config.Config) {
	theme, warning := ResolveThemeForConfig(cfg, "")
	m.themeWarn = warning
	if warning != "" {
		logger.Warn("%s", warning)
		m.statusMessage = warning
	}
	m.applyTheme(theme)
}

func (m *Model) SetLaunchTheme(value string) error {
	theme, err := ResolveLaunchTheme(value, "")
	if err != nil {
		return err
	}
	m.themeWarn = ""
	m.applyTheme(theme)
	return nil
}

func (m *Model) applyTheme(theme Theme) {
	m.theme = theme
	m.baseStyle = theme.BasePanelStyle()
	m.headerStyle = theme.TitleBarStyle()
	m.loadingStyle = theme.LoadingStyle()
	m.progressStyle = theme.ProgressStyle()
	m.activeTableStyle = theme.TableStyles(true)
	m.inactiveTableStyle = theme.TableStyles(false)
	if m.logViewer != nil {
		m.logViewer.ApplyTheme(theme)
	}
	m.updateTableFocusStyles()
}

func (m *Model) applyKeyboardConfig(cfg *config.Config) {
	m.cfg = cfg
	m.keyboardWarn = ""
	resolver := NewKeyboardResolver(cfg)
	if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Keyboard.Profile), keyboardProfileCustom) {
		keymapPath := strings.TrimSpace(cfg.Keyboard.CustomKeymap)
		if keymapPath == "" {
			m.keyboardWarn = "Custom keyboard profile selected without custom_keymap; using vim defaults."
		} else {
			expanded, err := config.ExpandPath(keymapPath)
			if err != nil {
				m.keyboardWarn = "Keyboard keymap path failed: " + err.Error()
			} else if data, err := os.ReadFile(expanded); err != nil {
				m.keyboardWarn = "Keyboard keymap failed to load: " + err.Error()
			} else if err := resolver.ApplyCustomKeymap(data); err != nil {
				m.keyboardWarn = "Keyboard keymap invalid: " + err.Error()
			}
		}
	}
	m.keyboard = resolver
	if mode, ok := m.composeFieldDefaultMode(); ok {
		if m.fieldKeyMode == "" {
			m.fieldKeyMode = mode
		}
	} else {
		m.fieldKeyMode = ""
	}
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
}

func (m *Model) SetPreviewImageMode(mode PreviewImageMode) {
	m.previewImageMode = previewImageMode(mode)
	m.clearTimelinePreviewDocumentCache()
}

// SetDemoKeyOverlay toggles the small demo-media keypress overlay. It is only
// wired from demo CLI mode so normal mailbox sessions never expose it.
func (m *Model) SetDemoKeyOverlay(enabled bool) {
	m.demoKeyOverlay = enabled
	if !enabled {
		m.demoKeyOverlayKeys = nil
	}
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	startupAIWarning := ollamaModelWarningCmd(m.cfg, m.demoMode)
	if !m.loading {
		return tea.Batch(
			m.listenForRuleResult(),
			m.importAppleContacts(),
			draftSaveTick(),
			startupAIWarning,
		)
	}
	return tea.Batch(
		m.startLoading(),
		m.tickSpinner(),
		m.listenForSyncEvents(),
		m.listenForRuleResult(),
		m.importAppleContacts(),
		draftSaveTick(),
		startupAIWarning,
	)
}

func (m *Model) applySettingsSaved(msg SettingsSavedMsg) tea.Cmd {
	previousCfg := m.cfg
	previousEmbeddingModel := ""
	previousCachePolicy := config.CacheStoragePolicyNoAttachments
	if m.cfg != nil {
		previousEmbeddingModel = m.cfg.EffectiveEmbeddingModel()
		previousCachePolicy = config.NormalizeCacheStoragePolicy(m.cfg.Cache.StoragePolicy)
	}
	m.SetConfig(msg.Config)
	if m.cfg == nil || !aicheck.OllamaConfigured(m.cfg) {
		m.aiModelWarning = nil
	}
	nextCachePolicy := config.CacheStoragePolicyNoAttachments
	if m.cfg != nil {
		nextCachePolicy = config.NormalizeCacheStoragePolicy(m.cfg.Cache.StoragePolicy)
	}
	var settingsCmds []tea.Cmd
	m.statusMessage = "Settings saved."
	if m.keyboardWarn != "" {
		m.statusMessage = "Settings saved. " + m.keyboardWarn
	}
	if m.themeWarn != "" {
		m.statusMessage = "Settings saved. " + m.themeWarn
	}
	if err := m.refreshAIClientForConfig(previousCfg, m.cfg); err != nil {
		m.statusMessage = fmt.Sprintf("Settings saved, but AI reload failed: %v", err)
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
	if previousCachePolicy != nextCachePolicy && !msg.ReclaimOfflineCacheStorage {
		type cacheStoragePolicyApplier interface {
			ApplyCacheStoragePolicy(string) (models.PreviewCachePruneResult, error)
		}
		if manager, ok := m.backend.(cacheStoragePolicyApplier); ok {
			policy := nextCachePolicy
			settingsCmds = append(settingsCmds, func() tea.Msg {
				result, err := manager.ApplyCacheStoragePolicy(policy)
				return CacheStoragePolicyAppliedMsg{Policy: policy, Result: result, Err: err}
			})
		} else {
			m.statusMessage = "Settings saved. Restart Herald to prune existing preview cache for the new policy."
		}
	}
	if msg.ReclaimOfflineCacheStorage {
		type previewCacheReclaimEstimator interface {
			EstimateOfflineCacheStorageReclaim(string) (models.PreviewCacheStorageEstimate, error)
		}
		if manager, ok := m.backend.(previewCacheReclaimEstimator); ok {
			policy := nextCachePolicy
			settingsCmds = append(settingsCmds, func() tea.Msg {
				estimate, err := manager.EstimateOfflineCacheStorageReclaim(policy)
				return PreviewCacheReclaimEstimateMsg{Policy: policy, Estimate: estimate, Err: err}
			})
			m.statusMessage = "Settings saved. Estimating offline cache reclaim..."
		} else {
			m.statusMessage = "Settings saved. Restart Herald to reclaim offline cache storage."
		}
	}
	if msg.ReturnToMenu {
		m.showSettings = true
		m.settingsPanel = m.newSettingsPanel("", m.statusMessage)
		initCmd := m.settingsPanel.Init()
		if len(settingsCmds) > 0 {
			settingsCmds = append(settingsCmds, initCmd)
			return tea.Batch(settingsCmds...)
		}
		return initCmd
	}
	m.showSettings = false
	m.settingsPanel = nil
	if len(settingsCmds) > 0 {
		return tea.Batch(settingsCmds...)
	}
	return nil
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
				m.statusMessage = "Prompt saved: " + msg.Prompt.Name + ". Review saved prompts in Settings > Sync & Cleanup."
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
	case AccountValidationMsg:
		return m, m.finishAccountValidation(msg)

	case CalendarValidationMsg:
		return m, m.finishCalendarValidation(msg)

	case OllamaModelValidationMsg:
		return m, m.finishOllamaModelValidation(msg)

	case OllamaModelWarningMsg:
		if err := msg.Result.Err(); err != nil {
			warning := msg.Result
			m.aiModelWarning = &warning
			m.classifier = nil
			m.statusMessage = "AI unavailable. Open Settings > AI for repair commands."
			logger.Warn("Startup Ollama model check failed: %v", err)
		} else {
			m.aiModelWarning = nil
		}
		return m, nil

	case SettingsSavedMsg:
		if msg.ValidateAccount {
			return m, m.beginAccountValidation(msg.Config, msg.ReturnToMenu, msg.ReclaimOfflineCacheStorage, msg.ValidateCalendar, msg.CalendarSourceIDs)
		}
		if msg.ValidateCalendar {
			return m, m.beginCalendarValidation(msg.Config, msg.ReturnToMenu, msg.ReclaimOfflineCacheStorage, msg.CalendarSourceIDs)
		}
		if msg.ValidateOllamaModels {
			return m, m.beginOllamaModelValidation(msg.Config, msg.ReturnToMenu, msg.ReclaimOfflineCacheStorage)
		}
		return m, m.applySettingsSaved(msg)

	case SettingsToolRequestedMsg:
		return m, m.openSettingsCleanupTool(msg.Tool)

	case CacheStoragePolicyAppliedMsg:
		if msg.Err != nil {
			m.statusMessage = fmt.Sprintf("Preview cache policy update failed: %v", msg.Err)
			return m, nil
		}
		bytesRemoved := msg.Result.AttachmentBytesRemoved + msg.Result.InlineImageBytesRemoved
		m.statusMessage = fmt.Sprintf("Preview cache policy applied: %s (%d rows pruned, %d bytes removed)", msg.Policy, msg.Result.RowsChanged, bytesRemoved)
		return m, nil

	case PreviewCacheReclaimEstimateMsg:
		if msg.Err != nil {
			m.pendingPreviewCacheReclaim = false
			m.statusMessage = fmt.Sprintf("Offline cache reclaim estimate failed: %v", msg.Err)
			if m.showSettings && m.settingsPanel != nil {
				m.settingsPanel.panelStatus = m.statusMessage
				m.settingsPanel.buildForm()
				m.settingsPanel.setSize(m.windowWidth, m.windowHeight)
			}
			return m, nil
		}
		m.pendingPreviewCacheReclaim = true
		m.previewCacheReclaimPolicy = msg.Policy
		m.previewCacheReclaimEstimate = msg.Estimate
		m.statusMessage = "Review offline cache reclaim estimate."
		return m, nil

	case PreviewCacheReclaimMsg:
		m.pendingPreviewCacheReclaim = false
		if msg.Err != nil {
			m.statusMessage = fmt.Sprintf("Offline cache reclaim failed: %v", msg.Err)
		} else {
			bytesRemoved := msg.Result.PruneResult.AttachmentBytesRemoved + msg.Result.PruneResult.InlineImageBytesRemoved
			compactionStatus := "compaction complete"
			if msg.Result.CompactionError != "" {
				compactionStatus = "compaction failed: " + msg.Result.CompactionError
			} else if !msg.Result.Compacted {
				compactionStatus = "compaction skipped"
			}
			m.statusMessage = fmt.Sprintf("Offline cache reclaimed: %d rows pruned, %s removed, %s", msg.Result.PruneResult.RowsChanged, formatPreviewCacheBytes(bytesRemoved), compactionStatus)
		}
		if m.showSettings && m.settingsPanel != nil {
			m.settingsPanel.panelStatus = m.statusMessage
			m.settingsPanel.buildForm()
			m.settingsPanel.setSize(m.windowWidth, m.windowHeight)
		}
		return m, nil

	case SettingsCancelledMsg:
		m.applyThemeConfig(m.cfg)
		m.showSettings = false
		m.settingsPanel = nil
		return m, nil

	case OAuthRequiredMsg:
		// Gmail chosen in the settings panel — launch the OAuth wait overlay.
		m.showSettings = false
		m.settingsPanel = nil
		oauthModel, err := NewOAuthWaitModelWithOptions(msg.Email, msg.Config, m.configPath, OAuthWaitOptions{
			ReturnToMenu:               msg.ReturnToMenu,
			ReclaimOfflineCacheStorage: msg.ReclaimOfflineCacheStorage,
			ValidateAccount:            msg.ValidateAccount,
			ValidateCalendar:           msg.ValidateCalendar,
			CalendarSourceIDs:          msg.CalendarSourceIDs,
			SourceIDs:                  msg.SourceIDs,
		})
		if err != nil {
			m.accountValidation = &accountValidationState{
				Checking: false,
				Message:  oauthStartFailureMessage(err),
			}
			m.statusMessage = "OAuth could not start. Settings were not saved."
			logger.Error("OAuth start failed: %v", err)
			return m, nil
		}
		m.oauthWait = oauthModel
		return m, m.oauthWait.Init()

	case OAuthDoneMsg:
		if msg.ValidateAccount {
			return m, m.beginAccountValidation(msg.Config, msg.ReturnToMenu, msg.ReclaimOfflineCacheStorage, msg.ValidateCalendar, msg.CalendarSourceIDs)
		}
		if msg.ValidateCalendar {
			return m, m.beginCalendarValidation(msg.Config, msg.ReturnToMenu, msg.ReclaimOfflineCacheStorage, msg.CalendarSourceIDs)
		}
		return m, m.applySettingsSaved(SettingsSavedMsg{
			Config:                     msg.Config,
			ReturnToMenu:               msg.ReturnToMenu,
			ReclaimOfflineCacheStorage: msg.ReclaimOfflineCacheStorage,
		})

	case OAuthErrorMsg:
		m.oauthWait = nil
		message := msg.UserMessage
		if message == "" && msg.Err != nil {
			message = msg.Err.Error()
		}
		if message == "" {
			message = "OAuth authorization failed. Settings were not saved."
		}
		m.accountValidation = &accountValidationState{
			Checking: false,
			Message:  message,
		}
		m.statusMessage = "OAuth authorization failed. Settings were not saved."
		logger.Error("OAuth failed: %v", msg.Err)
		return m, nil
	}

	// Forward all messages to the OAuth wait overlay when active.
	if m.oauthWait != nil {
		newModel, cmd := m.oauthWait.Update(msg)
		m.oauthWait = newModel.(*OAuthWaitModel)
		return m, cmd
	}

	if m.accountValidation != nil {
		if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.updateTableDimensions(sizeMsg.Width, sizeMsg.Height)
			m.chatWrappedLines = nil
			return m, tea.ClearScreen
		}
		if key, ok := msg.(tea.KeyPressMsg); ok && !m.accountValidation.Checking {
			switch shortcutKey(key) {
			case "enter", "esc", "q":
				m.accountValidation = nil
				return m, nil
			}
		}
		return m, nil
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

	if m.pendingPreviewCacheReclaim {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			model, cmd, handled := m.handlePreviewCacheReclaimKey(key)
			if handled {
				return model, cmd
			}
		}
	}

	// Forward all messages to the settings panel when it is active (intercepts
	// key presses and window-size events so the panel handles them exclusively).
	if m.showSettings && m.settingsPanel != nil {
		if _, isMouse := msg.(tea.MouseMsg); isMouse {
			return m, nil
		}
		_, isKey := msg.(tea.KeyPressMsg)
		prevSection := m.settingsPanel.panelSection
		prevDone := m.settingsPanel.done
		if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.updateTableDimensions(sizeMsg.Width, sizeMsg.Height)
			m.chatWrappedLines = nil
		}
		newModel, cmd := m.settingsPanel.Update(msg)
		m.settingsPanel = newModel.(*Settings)
		if isThemeSettingsSection(prevSection) && !isThemeSettingsSection(m.settingsPanel.panelSection) {
			m.applyThemeConfig(m.cfg)
		} else if isThemeSettingsSection(m.settingsPanel.panelSection) {
			m.applyThemeConfig(m.settingsPanel.buildConfig())
		}
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			return m, tea.Batch(cmd, tea.ClearScreen)
		}
		if isKey || cmd != nil || prevSection != m.settingsPanel.panelSection || prevDone != m.settingsPanel.done {
			return m, cmd
		}
		// Background app messages, especially sync completion after an account
		// save, must still reach the main model while settings is open.
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
	case tea.KeyboardEnhancementsMsg:
		m.keyEventTypes = msg.SupportsEventTypes()
		return m, nil

	case tea.KeyReleaseMsg:
		m.handleModifierHintRelease(msg)
		return m, nil

	case modifierHintExpiredMsg:
		if msg.Token == m.modifierHintFallbackToken {
			m.modifierHintMods = 0
		}
		return m, nil

	case tea.KeyPressMsg:
		modifierCmd, handledModifierOnly := m.handleModifierHintPress(msg)
		if handledModifierOnly {
			return m, modifierCmd
		}
		model, cmd := m.handleKeyMsg(msg)
		return model, tea.Batch(modifierCmd, cmd)

	case tea.MouseMsg:
		if model, cmd, handled := m.handleMouseMsg(msg); handled {
			return model, cmd
		}

	case FoldersLoadedMsg:
		logger.Debug("FoldersLoadedMsg: folders=%d currentFolder=%s", len(msg.Folders), m.currentFolder)
		if !m.scopedResultMatchesActive(msg.SourceID) {
			logger.Debug("FoldersLoadedMsg: ignoring stale source=%s active=%s", msg.SourceID, m.activeSourceID)
			return m, nil
		}
		m.accountFolderSnapshots = msg.AccountSnapshots
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
		items := m.visibleSidebarItems()
		m.sidebarCursor = 0
		for i, item := range items {
			if m.sidebarItemMatchesCurrentFolder(item) {
				m.sidebarCursor = i
				break
			}
		}
		m.normalizeSidebarCursor(1)
		// Only fetch counts on first load to avoid flickering on every folder switch
		if len(m.folderStatus) == 0 {
			folders := msg.Folders
			sourceID := m.activeSourceID
			loadCounts := func() tea.Msg {
				status, err := m.backend.GetFolderStatus(folders)
				if err != nil {
					logger.Warn("Failed to get folder status: %v", err)
					return FolderStatusMsg{SourceID: sourceID, Status: map[string]models.FolderStatus{}}
				}
				return FolderStatusMsg{SourceID: sourceID, Status: status}
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
		if !m.scopedResultMatchesActive(msg.SourceID) {
			logger.Debug("FolderStatusMsg: ignoring stale source=%s active=%s", msg.SourceID, m.activeSourceID)
			return m, nil
		}
		// Merge rather than replace so partial results don't wipe existing counts
		for folder, st := range msg.Status {
			logger.Debug("FolderStatusMsg: folder=%s total=%d unseen=%d", folder, st.Total, st.Unseen)
			m.folderStatus[folder] = st
		}
		return m, nil

	case CalendarAgendaLoadedMsg:
		m.calendarLoading = false
		if msg.Err != nil {
			m.calendarCollections = nil
			m.calendarEvents = nil
			m.calendarDetail = nil
			m.calendarStatus = "Calendar agenda failed: " + msg.Err.Error()
			return m, nil
		}
		m.calendarEvents = normalizeCalendarEventsForDisplay(msg.Events)
		m.calendarCollections = m.mergeCalendarCollections(msg.Collections)
		m.pruneCalendarCollectionState()
		if m.calendarCursor >= len(m.calendarEvents) {
			m.calendarCursor = len(m.calendarEvents) - 1
		}
		if m.calendarCursor < 0 {
			m.calendarCursor = 0
		}
		if m.calendarView == "" {
			m.calendarView = calendarViewAgenda
		}
		if m.calendarView == calendarViewDay {
			m.calendarDay = m.selectedCalendarDay()
			m.selectFirstCalendarEventForDay(m.calendarDay)
		} else if m.calendarView == calendarViewWeek {
			m.calendarWeekStart = m.selectedCalendarWeekStart()
			m.selectFirstCalendarEventForWeek(m.calendarWeekStart)
		} else if m.calendarView == calendarViewThreeDay {
			m.calendarThreeDayStart = m.selectedCalendarThreeDayStart()
			m.selectFirstCalendarEventForThreeDay(m.calendarThreeDayStart)
		} else if m.calendarView == calendarViewSearch || m.calendarView == calendarViewCrossSearch {
			m.calendarDetail = m.selectedCalendarEvent()
		} else {
			m.calendarDetail = m.selectedCalendarEvent()
		}
		m.calendarStatus = fmt.Sprintf("Loaded %d calendar event(s)", len(m.calendarEvents))
		return m, nil

	case CalendarSearchLoadedMsg:
		if msg.Query != m.calendarSearchQuery || m.calendarView != calendarViewSearch {
			return m, nil
		}
		m.calendarSearchLoading = false
		if strings.TrimSpace(msg.Query) == "" {
			m.calendarSearchResults = nil
			m.calendarDetail = nil
			m.calendarStatus = "Type to search cached calendar events"
			return m, nil
		}
		if msg.Err != nil {
			m.calendarSearchResults = nil
			m.calendarDetail = nil
			m.calendarStatus = "Calendar search failed: " + msg.Err.Error()
			return m, nil
		}
		m.calendarSearchResults = msg.Events
		if m.calendarSearchCursor >= len(m.calendarSearchResults) {
			m.calendarSearchCursor = len(m.calendarSearchResults) - 1
		}
		if m.calendarSearchCursor < 0 {
			m.calendarSearchCursor = 0
		}
		m.calendarDetail = m.selectedCalendarEvent()
		m.calendarStatus = fmt.Sprintf("Search found %d cached event(s)", len(m.calendarSearchResults))
		return m, nil

	case CrossSourceSearchLoadedMsg:
		if msg.Query != m.crossSourceSearchQuery || m.calendarView != calendarViewCrossSearch {
			return m, nil
		}
		m.crossSourceSearchLoading = false
		if strings.TrimSpace(msg.Query) == "" {
			m.crossSourceSearchResults = nil
			m.calendarDetail = nil
			m.calendarStatus = "Type to search cached mail and calendar events"
			return m, nil
		}
		if msg.Err != nil {
			m.crossSourceSearchResults = nil
			m.calendarDetail = nil
			m.calendarStatus = "Cross-source search failed: " + msg.Err.Error()
			return m, nil
		}
		m.crossSourceSearchResults = msg.Results
		if m.crossSourceSearchCursor >= len(m.crossSourceSearchResults) {
			m.crossSourceSearchCursor = len(m.crossSourceSearchResults) - 1
		}
		if m.crossSourceSearchCursor < 0 {
			m.crossSourceSearchCursor = 0
		}
		m.selectCrossSourceSearchResult()
		m.calendarStatus = fmt.Sprintf("Search found %d cached mail/event result(s)", len(m.crossSourceSearchResults))
		return m, nil

	case CalendarMeetingPrepMsg:
		selected := m.calendarDetail
		if selected == nil {
			selected = m.selectedCalendarEvent()
		}
		if selected == nil || selected.Ref.WithDefaults().LocalID != msg.Ref.WithDefaults().LocalID {
			return m, nil
		}
		m.calendarMeetingPrepLoading = false
		if msg.Err != nil {
			m.calendarMeetingPrep = nil
			m.calendarStatus = "Meeting prep failed: " + msg.Err.Error()
			return m, nil
		}
		prep := msg.Prep
		prep.Event.Ref = prep.Event.Ref.WithDefaults()
		m.calendarMeetingPrep = &prep
		m.calendarMeetingPrepOpen = true
		m.calendarDetailOpen = false
		m.calendarStatus = fmt.Sprintf("Meeting prep found %d cached mail and %d nearby event(s)", len(prep.RelatedMail), len(prep.RelatedEvents))
		return m, nil

	case CalendarTravelBufferMsg:
		selected := m.calendarDetail
		if selected == nil {
			selected = m.selectedCalendarEvent()
		}
		if selected == nil || selected.Ref.WithDefaults().LocalID != msg.Ref.WithDefaults().LocalID {
			return m, nil
		}
		m.calendarTravelBufferLoading = false
		if msg.Err != nil {
			m.calendarTravelBuffer = nil
			m.calendarStatus = "Travel buffer failed: " + msg.Err.Error()
			return m, nil
		}
		buffer := msg.Buffer
		buffer.Event.Ref = buffer.Event.Ref.WithDefaults()
		m.calendarTravelBuffer = &buffer
		m.calendarTravelBufferOpen = true
		m.calendarDetailOpen = false
		m.calendarStatus = fmt.Sprintf("Travel buffer found %d cached mail, %d nearby event(s), and %d suggestion(s)", len(buffer.RelatedMail), len(buffer.NearbyEvents), len(buffer.Recommendations))
		return m, nil

	case CalendarAISummaryMsg:
		selected := m.calendarDetail
		if selected == nil {
			selected = m.selectedCalendarEvent()
		}
		if selected == nil || selected.Ref.WithDefaults().LocalID != msg.Ref.WithDefaults().LocalID {
			return m, nil
		}
		m.calendarAISummaryLoading = false
		if msg.Err != nil {
			m.calendarAISummary = nil
			m.calendarStatus = "AI summary failed: " + msg.Err.Error()
			return m, nil
		}
		summary := msg.Summary
		summary.Event.Ref = summary.Event.Ref.WithDefaults()
		m.calendarAISummary = &summary
		m.calendarAISummaryOpen = true
		m.calendarDetailOpen = false
		m.calendarStatus = fmt.Sprintf("AI summary used %d cached mail and %d nearby event(s)", len(summary.RelatedMail), len(summary.NearbyEvents))
		return m, nil

	case CalendarEventDetailMsg:
		if selected := m.selectedCalendarEvent(); selected == nil || selected.Ref.WithDefaults().LocalID != msg.Ref.WithDefaults().LocalID {
			return m, nil
		}
		m.calendarDetailLoading = false
		if msg.Err != nil {
			m.calendarStatus = "Calendar detail failed: " + msg.Err.Error()
			return m, nil
		}
		if msg.Event != nil {
			m.calendarDetail = msg.Event
		}
		m.calendarDetailOpen = true
		return m, nil

	case CalendarEventSavedMsg:
		if msg.Ref.WithDefaults().LocalID != m.calendarEdit.Ref.WithDefaults().LocalID {
			return m, nil
		}
		if msg.Err != nil {
			m.calendarEdit.Saving = false
			m.calendarEdit.Error = calendarMutationErrorStatus("Save failed", msg.Err)
			m.calendarStatus = m.calendarEdit.Error
			return m, nil
		}
		if msg.Event != nil {
			m.applySavedCalendarEvent(*msg.Event)
		}
		m.calendarEdit = calendarEventEditState{}
		m.calendarDetailOpen = true
		m.calendarDetailLoading = false
		m.calendarStatus = "Saved calendar event"
		return m, nil

	case CalendarEventRSVPMsg:
		selected := m.calendarDetail
		if selected == nil {
			selected = m.selectedCalendarEvent()
		}
		if selected != nil && msg.Ref.WithDefaults().LocalID != selected.Ref.WithDefaults().LocalID {
			return m, nil
		}
		if msg.Err != nil {
			m.calendarStatus = calendarMutationErrorStatus("RSVP failed", msg.Err)
			return m, nil
		}
		if msg.Event != nil {
			m.applySavedCalendarEvent(*msg.Event)
		}
		m.calendarDetailOpen = true
		m.calendarDetailLoading = false
		m.calendarStatus = "Saved RSVP " + msg.Status
		return m, nil

	case CalendarInvitationSavedMsg:
		m.calendarInvitation.Saving = false
		if msg.Err != nil {
			m.calendarInvitation.Error = calendarMutationErrorStatus("Invitation import failed", msg.Err)
			m.calendarStatus = m.calendarInvitation.Error
			return m, nil
		}
		if msg.Event != nil {
			m.applySavedCalendarEvent(*msg.Event)
			m.calendarEvents = normalizeCalendarEventsForDisplay(m.calendarEvents)
		}
		m.calendarInvitation = calendarInvitationPromptState{}
		m.calendarStatus = "Added invitation to calendar"
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
			m.resetComposeAIBar()
			// Delete the auto-saved draft (if any) since the email was sent
			if m.lastDraftUID != 0 {
				if !m.lastDraftReplaceable {
					m.lastDraftUID = 0
					m.lastDraftFolder = ""
					m.lastDraftSourceID = ""
					m.lastDraftReplaceable = false
					return m, nil
				}
				cmd := m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder)
				m.lastDraftUID = 0
				m.lastDraftFolder = ""
				m.lastDraftSourceID = ""
				m.lastDraftReplaceable = false
				return m, cmd
			}
		}
		return m, nil

	case ClassifyProgressMsg:
		m.classifyDone = msg.Done
		m.classifyTotal = msg.Total
		m.setClassificationKeys(msg.MessageRef, msg.MessageID, msg.Category)
		// Refresh tables to show updated tags
		m.updateTimelineTable()
		// Push newly classified email to rule engine (non-blocking)
		for _, e := range m.timeline.emails {
			if e.MessageID == msg.MessageID || e.MessageRef().LocalID == msg.MessageRef.LocalID {
				select {
				case m.ruleRequestCh <- models.NewMailAutomationEvent(e, msg.Category):
				default:
				}
				break
			}
		}
		return m, m.listenForClassification()

	case ClassifyDoneMsg:
		m.classifying = false
		m.updateTimelineTable()
		logger.Info("Classification complete: %d emails tagged", m.classifyDone)
		return m, nil

	case ReclassifyResultMsg:
		if msg.Err != nil {
			m.statusMessage = "Reclassify failed: " + msg.Err.Error()
			return m, nil
		}
		m.setClassificationKeys(msg.MessageRef, msg.MessageID, msg.Category)
		m.statusMessage = "Reclassified: " + msg.Category
		m.updateTimelineTable()
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
			m.lastDraftSourceID = msg.SourceID
			m.lastDraftReplaceable = true
			m.statusMessage = "Draft saved"
			if msg.ReplaceUID != 0 && (msg.ReplaceUID != msg.UID || msg.ReplaceFolder != msg.Folder) {
				return m, m.deleteDraftForSourceCmd(msg.ReplaceSourceID, msg.ReplaceUID, msg.ReplaceFolder)
			}
		}
		return m, nil

	case DraftDeletedMsg:
		if msg.Err != nil {
			logger.Warn("delete old draft failed: %v", msg.Err)
		}
		return m, nil

	case StartupHydratedMsg:
		if !m.scopedResultMatchesActive(msg.SourceID) || (msg.Folder != "" && msg.Folder != m.currentFolder) {
			logger.Debug("StartupHydratedMsg: ignoring stale source=%s active=%s folder=%s current=%s", msg.SourceID, m.activeSourceID, msg.Folder, m.currentFolder)
			return m, nil
		}
		if msg.Err != nil {
			logger.Warn("startup snapshot hydrate failed: %v", msg.Err)
			return m, nil
		}
		logger.Debug("StartupHydratedMsg: emails=%d finish=%v status=%q", len(msg.Emails), msg.FinishLoading, strings.TrimSpace(msg.StatusMessage))
		if msg.Emails != nil {
			m.finishTimelineRangeSelection()
			m.timeline.emails = msg.Emails
			m.reflowCurrentLayout()
		}
		m.loadClassifications()
		if msg.FinishLoading {
			hadTopSyncStrip := m.hasTopSyncStrip()
			m.loading = false
			m.statusMessage = msg.StatusMessage
			m.reflowIfTopSyncStripChanged(hadTopSyncStrip)
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
			if status, ok := composeAIStatusForRewriteError(msg.Err); ok {
				m.composeStatus = status
			} else {
				m.composeStatus = fmt.Sprintf("AI error: %v", msg.Err)
			}
			m.refreshComposeLayout()
			return m, nil
		}
		original := msg.Original
		if original == "" {
			original = m.composeBody.Value()
		}
		suggestion := cleanComposeAISuggestion(msg.Result, original)
		m.composeAIMenu = ""
		m.composeAIDiff = wordDiffWithTheme(m.theme, original, suggestion)
		m.composeAIOriginal = original
		m.composeAIShowOriginal = false
		m.composeAIResponse.SetValue(suggestion)
		m.composeAIResponse.MoveToBegin()
		m.composeAIInput.Blur()
		m.composeBody.Blur()
		m.composeAIResponse.Focus()
		m.refreshComposeLayout()
		return m, nil

	case AISubjectMsg:
		m.composeAILoading = false
		if msg.Err != nil {
			m.composeStatus = fmt.Sprintf("AI error: %v", msg.Err)
			m.refreshComposeLayout()
			return m, nil
		}
		m.composeAISubjectHint = strings.TrimSpace(msg.Subject)
		m.refreshComposeLayout()
		return m, nil

	case ValidIDsMsg:
		logger.Debug("ValidIDsMsg: folder=%s validIDs=%d", m.currentFolder, len(msg.ValidIDs))
		// Background reconciliation has produced the live valid-ID set.
		// The backend's filterByValidIDs now applies automatically; reload all views.
		if isVirtualAllMailOnlyFolder(m.currentFolder) {
			return m, nil
		}
		m.loadClassifications()
		return m, m.loadTimelineEmails()

	case SyncEventMsg:
		event := msg.Event
		logger.Debug("SyncEventMsg: folder=%s generation=%d phase=%s current=%d total=%d delta=%d message=%q", event.Folder, event.Generation, event.Phase, event.Current, event.Total, event.EventCount, strings.TrimSpace(event.Message))
		if !m.syncEventMatchesActive(event.SourceID) {
			logger.Debug("SyncEventMsg: ignoring stale source=%s active=%s", event.SourceID, m.activeSourceID)
			return m, m.listenForSyncEvents()
		}
		currentSourceGeneration := m.syncGenerationForSource(event.SourceID)
		if event.Generation < currentSourceGeneration {
			logger.Debug("SyncEventMsg: ignoring stale generation=%d current=%d source=%s", event.Generation, currentSourceGeneration, event.SourceID)
			return m, m.listenForSyncEvents()
		}
		if event.Folder != "" && event.Folder != m.currentFolder {
			// Keep listening, but do not let an older folder repaint the visible one.
			if event.Generation > currentSourceGeneration {
				m.setSyncGenerationForSource(event.SourceID, event.Generation)
			}
			logger.Debug("SyncEventMsg: ignoring event for non-current folder=%s currentFolder=%s generation=%d", event.Folder, m.currentFolder, event.Generation)
			return m, m.listenForSyncEvents()
		}

		if event.Generation > currentSourceGeneration {
			m.setSyncGenerationForSource(event.SourceID, event.Generation)
		}
		if event.Generation > m.syncAccumulator.generation {
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
		hadTopSyncStrip := m.hasTopSyncStrip()
		m.syncingFolder = event.Folder
		m.loading = true
		if event.Phase == models.SyncPhaseComplete {
			m.syncCountsSettled = true
		} else if event.Phase != models.SyncPhaseReconcileStarted {
			m.syncCountsSettled = false
		}
		m.reflowIfTopSyncStripChanged(hadTopSyncStrip)

		action := m.syncAccumulator.observe(event)
		logger.Debug("SyncEventMsg: accumulator action folder=%s generation=%d armTimer=%v flushNow=%v", event.Folder, event.Generation, action.ArmTimer, action.FlushNow)
		cmds := []tea.Cmd{m.listenForSyncEvents()}
		if action.ArmTimer {
			cmds = append(cmds, scheduleSyncFlush(event.SourceID, event.Folder, event.Generation, m.syncAccumulator.flushDelay))
		}
		if action.FlushNow {
			finish := event.Phase == models.SyncPhaseComplete || event.Phase == models.SyncPhaseError
			status := ""
			if event.Phase == models.SyncPhaseComplete {
				status = strings.TrimSpace(event.Message)
			}
			cmds = append(cmds, m.loadSyncSnapshotForSourceCmd(event.SourceID, event.Folder, event.Generation, finish, status))
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
		return m, m.loadSyncSnapshotForSourceCmd(msg.SourceID, msg.Folder, msg.Generation, false, "")

	case SyncHydratedMsg:
		if !m.scopedResultMatchesActive(msg.SourceID) {
			logger.Debug("SyncHydratedMsg: ignoring stale source=%s active=%s folder=%s", msg.SourceID, m.activeSourceID, msg.Folder)
			return m, nil
		}
		if !m.syncHydratedGenerationMatches(msg.SyncSourceID, msg.Generation) {
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
		logger.Debug("SyncHydratedMsg: folder=%s generation=%d emails=%d finish=%v status=%q", msg.Folder, msg.Generation, len(msg.Emails), msg.FinishLoading, strings.TrimSpace(msg.StatusMessage))
		if msg.Emails != nil {
			m.finishTimelineRangeSelection()
			m.timeline.emails = msg.Emails
			m.reflowCurrentLayout()
		}
		m.loadClassifications()
		cmds := make([]tea.Cmd, 0, 1)
		if msg.FinishLoading {
			hadTopSyncStrip := m.hasTopSyncStrip()
			m.loading = false
			m.progressInfo = models.ProgressInfo{}
			m.statusMessage = msg.StatusMessage
			m.reflowIfTopSyncStripChanged(hadTopSyncStrip)
			logger.Debug("SyncHydratedMsg: loading finished folder=%s generation=%d", msg.Folder, msg.Generation)
			if cmd := m.startPreviewPrewarmerIfNeeded(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
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
		} else {
			// Remove from local cache immediately for instant UI update
			if msg.Sender != "" {
				logger.Info("Deletion complete: %s", msg.Sender)
			} else if msg.MessageID != "" {
				logger.Info("Deletion complete: message %s", msg.MessageID)
				// Remove individual message from cache
				// We don't have sender info here, so we'll update on full reload
			}

			m.pruneTimelineStateAfterDeletion(msg)
		}

		// Check if all deletions are complete
		if m.deletionsPending <= 0 {
			logger.Info("All %d deletions complete, reloading data...", m.deletionsTotal)
			m.deleting = false
			m.deletionsPending = 0
			m.deletionsTotal = 0
			m.connectionLost = false
			m.clearTimelineSelection()

			// Reload data after all deletions complete to sync with server
			// Also refresh timeline and sidebar folder counts
			folders := m.folders
			sourceID := m.activeSourceID
			refreshCounts := func() tea.Msg {
				status, err := m.backend.GetFolderStatus(folders)
				if err != nil {
					logger.Warn("Failed to refresh folder status: %v", err)
					return FolderStatusMsg{SourceID: sourceID, Status: map[string]models.FolderStatus{}}
				}
				return FolderStatusMsg{SourceID: sourceID, Status: status}
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
			m.setClassificationKeys(msg.MessageRef, msg.MessageID, msg.Category)
			m.updateTimelineTable()
		}
		// Send to rule engine regardless; use empty category on error so rules
		// that don't require a category can still fire.
		cat := msg.Category
		for _, e := range m.timeline.emails {
			if e.MessageID == msg.MessageID || e.MessageRef().LocalID == msg.MessageRef.LocalID {
				select {
				case m.ruleRequestCh <- models.NewMailAutomationEvent(e, cat):
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

	case previewPrewarmMsg:
		return m, m.handlePreviewPrewarmMsg(msg)

	case EmbeddingProgressMsg:
		if msg.Generation != m.backgroundWorkGeneration || (msg.Folder != "" && msg.Folder != m.currentFolder) {
			logger.Debug("EmbeddingProgressMsg: ignoring stale background batch folder=%s generation=%d currentFolder=%s currentGeneration=%d", msg.Folder, msg.Generation, m.currentFolder, m.backgroundWorkGeneration)
			return m, nil
		}
		m.embeddingDone = msg.Done
		m.embeddingTotal = msg.Total
		if msg.Notice != "" {
			m.statusMessage = msg.Notice
		}
		if msg.Done < msg.Total {
			return m, m.runEmbeddingBatch(msg.Folder, msg.Generation)
		}
		m.embeddingBatchActive = false
		return m, nil

	case EmbeddingDoneMsg:
		if msg.Generation != m.backgroundWorkGeneration || (msg.Folder != "" && msg.Folder != m.currentFolder) {
			logger.Debug("EmbeddingDoneMsg: ignoring stale background batch folder=%s generation=%d currentFolder=%s currentGeneration=%d", msg.Folder, msg.Generation, m.currentFolder, m.backgroundWorkGeneration)
			return m, nil
		}
		m.embeddingDone = 0
		m.embeddingTotal = 0
		m.embeddingBatchActive = false
		return m, nil

	case ContactEnrichedMsg:
		if msg.Background && msg.Generation != m.backgroundWorkGeneration {
			logger.Debug("ContactEnrichedMsg: ignoring stale background enrichment generation=%d currentGeneration=%d", msg.Generation, m.backgroundWorkGeneration)
			return m, nil
		}
		if msg.Notice != "" {
			m.contactStatusMessage = msg.Notice
		}
		if msg.Background && msg.Count > 0 {
			if msg.Notice == "" {
				m.contactStatusMessage = fmt.Sprintf("Enriched %d contacts", msg.Count)
			}
			return m, m.runContactEnrichment(msg.Generation)
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
		if m.composeAIReviewActive() && msg.Height <= 24 {
			m.composeAIResponse.MoveToBegin()
		}
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
	if m.accountValidation != nil {
		return m.buildView(m.renderAccountValidationOverlayView())
	}
	if m.aiModelValidation != nil {
		return m.buildView(m.renderAIModelValidationOverlayView())
	}
	if m.showSettings && m.settingsPanel != nil {
		view := m.renderSettingsOverlayView()
		if m.pendingPreviewCacheReclaim {
			view = m.renderPreviewCacheReclaimOverlayView(view)
		}
		return m.buildView(view)
	}
	if m.pendingPreviewCacheReclaim {
		return m.buildView(m.renderPreviewCacheReclaimOverlayView(m.renderMainView()))
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
	if m.showAccountSwitcher {
		return m.buildView(m.renderAccountSwitcherOverlayView())
	}
	if m.showHelp {
		return m.buildView(m.renderShortcutHelpView())
	}
	if m.shouldShowDemoWelcomeOverlay() {
		return m.buildView(m.renderDemoWelcomeOverlayView())
	}
	if m.loading && !m.hasVisibleStartupData() {
		return m.buildView(m.renderLoadingView())
	}
	if m.timeline.fullScreen {
		return m.buildView(m.renderFullScreenEmail())
	}
	return m.buildView(m.renderMainView())
}

func (m *Model) buildView(content string) tea.View {
	content = m.renderDemoKeyOverlay(content)
	if m.theme.Text.Primary.Background != nil {
		content = m.theme.RenderScreen(content, m.windowWidth, m.windowHeight)
	}
	v := newHeraldView(content)
	if !m.timeline.mouseMode {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// handleKeyMsg handles keyboard input
func (m *Model) handleKeyMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model, cmd, handled := m.handleDemoWelcomeKey(msg); handled {
		return model, cmd
	}

	m.recordDemoKeyOverlayPress(msg)

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

	if normalized := m.normalizeShortcutKeyForActiveScope(key); normalized != "" && normalized != key {
		key = normalized
		msg = shortcutKeyPressMsg(normalized)
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

	if m.activeTab == tabCalendar {
		return m.handleCalendarKey(msg)
	}

	if model, cmd, handled := m.handleTimelineKey(msg); handled {
		return model, cmd
	}

	switch key {
	case "m":
		return m, nil

	case "B", "f":
		return m, m.toggleSidebar()

	case "ctrl+r", "r":
		return m, m.refreshCurrentFolder()

	case " ":
		if m.canInteractWithVisibleData() {
			if m.focusedPanel == panelSidebar {
				m.toggleSidebarNode()
			}
		}
		return m, nil

	case "d":
		return m, m.confirmDeleteSelected()

	case "D":
		return m, m.deleteSelectedImmediately()

	case "backspace":
		return m, m.confirmDeleteSelected()

	case "shift+backspace":
		return m, m.deleteSelectedImmediately()

	case "A", "T":
		// Re-classify the currently focused single email with AI.
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil
		}
		if !m.loading && m.classifier != nil {
			var target *models.EmailData
			if m.activeTab == tabTimeline {
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
		m.statusMessage = "Cleanup tools moved to Settings > Sync & Cleanup."
		return m, nil

	case "P":
		m.statusMessage = "Custom prompts moved to Settings > Sync & Cleanup."
		return m, nil

	case "C":
		m.statusMessage = "Cleanup tools moved to Settings > Sync & Cleanup."
		return m, nil

	case "S":
		if !m.showSettings {
			m.showSettings = true
			m.settingsPanel = m.newSettingsPanel("", "")
			return m, m.settingsPanel.Init()
		}
		return m, nil

	case "a", "e":
		if m.activeTab == tabTimeline {
			m.finishTimelineRangeSelection()
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil
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
		return m, nil

	case "h", "H":
		return m, nil

	case "enter":
		if m.canInteractWithVisibleData() {
			if m.focusedPanel == panelSidebar {
				if cmd, handledAccount := m.selectSidebarFolder(); handledAccount {
					m.clearTimelineChatFilter()
					return m, cmd
				}
				m.clearTimelineChatFilter()
				return m, m.activateCurrentFolder()
			}
		}
		return m, nil

	case "z":
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

	case "g", "c":
		return m, m.toggleChat()

	case "q":
		m.cleanup()
		return m, tea.Quit

	case "t":
		if cmd := m.startClassificationIfNeeded(); cmd != nil {
			return m, cmd
		}
		return m, nil

	case "up", "k":
		if m.canInteractWithVisibleData() {
			if m.activeTab != tabTimeline {
				return m.handleNavigation(-1)
			}
		}
		return m, nil

	case "down", "j":
		if m.canInteractWithVisibleData() {
			if m.activeTab != tabTimeline {
				return m.handleNavigation(1)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) normalizeShortcutKeyForActiveScope(key string) string {
	if m == nil || m.keyboard == nil || key == "" {
		return key
	}
	scope := keyboardScopeGlobal
	switch m.activeTab {
	case tabTimeline:
		scope = "timeline"
	case tabContacts:
		scope = "contacts"
	case tabCalendar:
		scope = "calendar"
	}
	command, ok := m.keyboard.Resolve(scope, keyboardModeNormal, key)
	if !ok {
		return key
	}
	if canonical := canonicalKeyForCommand(scope, command); canonical != "" {
		return canonical
	}
	return key
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
	} else if m.activeTab == tabCalendar {
		mainContent = m.renderCalendarView()
	} else {
		mainContent = m.renderTimelineView()
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
