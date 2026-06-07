package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/agent"
	"github.com/herald-email/herald-mail-app/internal/aicheck"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/memory"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/oauth"
	"github.com/herald-email/herald-mail-app/internal/render"
)

// SettingsMode controls wizard vs panel layout.
type SettingsMode int

const (
	// SettingsModeWizard is fullscreen, no cancel.
	SettingsModeWizard SettingsMode = iota
	// SettingsModePanel is an overlay; esc to cancel.
	SettingsModePanel
)

// SettingsOptions controls optional settings-form behavior for callers that
// need a narrower first-run surface than the in-app settings panel.
type SettingsOptions struct {
	ShowExperimentalEmailServices bool
	FirstRunAccountOnly           bool
	FirstRunPreferencesOnly       bool
}

type settingsPanelSection string

const (
	aiProviderOllamaDefault = "ollama-default"
	aiProviderOllamaCustom  = "ollama-custom"
	aiProviderDisabled      = "disabled"

	aiSetupPresetOllamaDefault    = aiProviderOllamaDefault
	aiSetupPresetOllamaCustom     = aiProviderOllamaCustom
	aiSetupPresetOpenAIDefault    = "openai-default"
	aiSetupPresetClaudeDefault    = "claude-default"
	aiSetupPresetOpenAICompatible = "openai-compatible"
	aiSetupPresetDisabled         = aiProviderDisabled
	aiSetupPresetAdvancedManual   = "advanced-manual"
	defaultClaudeModel            = "claude-sonnet-4-6"

	firstRunPreferenceEnter     = "enter-herald"
	firstRunPreferenceCustomize = "customize-setup"

	defaultOllamaHost     = "http://localhost:11434"
	defaultOllamaModel    = "gemma3:4b"
	defaultChatModel      = "gpt-5-mini"
	defaultChatEffort     = agent.ReasoningEffortLow
	defaultEmbeddingModel = "nomic-embed-text-v2-moe"
	defaultOpenAIBaseURL  = "https://api.openai.com/v1"
	defaultOpenAIModel    = defaultChatModel
	defaultOpenAIEmbed    = "text-embedding-3-small"
	customModelChoice     = "custom"

	settingsPanelSectionMenu           settingsPanelSection = ""
	settingsPanelSectionAccounts       settingsPanelSection = "accounts"
	settingsPanelSectionAccount        settingsPanelSection = "account"
	settingsPanelSectionAddAccount     settingsPanelSection = "add-account"
	settingsPanelSectionAI             settingsPanelSection = "ai"
	settingsPanelSectionSync           settingsPanelSection = "sync-cleanup"
	settingsPanelSectionMemories       settingsPanelSection = "memories"
	settingsPanelSectionCalendar       settingsPanelSection = "calendar"
	settingsPanelSectionKeyboard       settingsPanelSection = "keyboard"
	settingsPanelSectionThemeSelection settingsPanelSection = "theme-selection"
	settingsPanelSectionThemeEditor    settingsPanelSection = "theme-editor"
	settingsPanelSectionSign           settingsPanelSection = "signature"

	settingsThemeForegroundKey       = "theme.foreground"
	settingsThemeForegroundPickerKey = "theme.foreground_picker"
	settingsThemeBackgroundKey       = "theme.background"
	settingsThemeBackgroundPickerKey = "theme.background_picker"

	settingsCleanupToolNone       = ""
	settingsCleanupToolAutomation = "automation-rules"
	settingsCleanupToolPrompts    = "custom-prompts"
	settingsCleanupToolRules      = "cleanup-rules"

	settingsMemoryTaskExtraction = "memory_extraction"
	settingsMemoryTaskTrack      = "track_status_update"
	settingsMemoryTaskRadar      = "compose_radar_nudges"
	settingsMemoryTaskDossiers   = "dossiers"
	settingsMemoryTaskObsidian   = "obsidian_section_format"
	settingsMemoryTaskResearch   = "research_note_summary"

	settingsAccountEditExisting    = "existing"
	settingsAccountEditAddMail     = "add-mail"
	settingsAccountEditAddCalendar = "add-calendar"
)

func isThemeSelectionSection(section settingsPanelSection) bool {
	return section == settingsPanelSectionThemeSelection
}

func isThemeEditorSection(section settingsPanelSection) bool {
	return section == settingsPanelSectionThemeEditor
}

func isThemeSettingsSection(section settingsPanelSection) bool {
	return isThemeSelectionSection(section) || isThemeEditorSection(section)
}

// SettingsSavedMsg is sent when the user completes the form and saves.
type SettingsSavedMsg struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	ValidateAccount            bool
	ValidateCalendar           bool
	CalendarSourceIDs          []models.SourceID
	ValidateOllamaModels       bool
	RemovedAccountID           models.AccountID
	RemovedSourceIDs           []models.SourceID
}

// SettingsCancelledMsg is sent when the user presses esc in panel mode.
type SettingsCancelledMsg struct{}

// SettingsToolRequestedMsg is sent when a Settings category should hand off to
// a compact manager/editor instead of saving configuration values.
type SettingsToolRequestedMsg struct {
	Tool string
}

// OAuthRequiredMsg is sent when a Google mail or calendar source needs OAuth.
type OAuthRequiredMsg struct {
	Email                      string
	ServiceLabel               string
	Config                     *config.Config // partially-built config with vendor presets applied
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	ValidateAccount            bool
	ValidateCalendar           bool
	CalendarSourceIDs          []models.SourceID
	SourceIDs                  []models.SourceID
}

var googleOAuthCredentialsConfigured = oauth.Configured

// Settings is a self-contained huh-based settings form component.
type Settings struct {
	mode       SettingsMode
	form       *huh.Form
	cfg        *config.Config // working copy
	configPath string         // path to the config file for saving
	width      int
	height     int
	done       bool // set once we've emitted the completion message
	saveButton bool

	showExperimentalEmailServices bool
	firstRunAccountOnly           bool
	firstRunPreferencesOnly       bool
	firstRunOtherProvider         bool
	firstRunPreferenceAction      string
	firstRunCustomizePreferences  bool
	panelSection                  settingsPanelSection
	panelMenuChoice               string
	panelStatus                   string
	accountsMenuChoice            string
	selectedAccountID             string
	addAccountChoice              string
	accountEditMode               string
	accountDisplayName            string
	calendarProvider              string
	calendarDisplayName           string
	calendarEmail                 string
	caldavURL                     string
	caldavUsername                string
	caldavPassword                string
	alsoAddCalendar               bool
	deleteAccount                 bool
	accountDeletePending          bool
	aiModelWarning                *aicheck.Result
	disableAIFromWarning          bool
	lastAIProvider                string

	// form field backing variables — account
	provider string
	email    string
	password string
	imapHost string
	imapPort string
	smtpHost string
	smtpPort string

	editGmailAdvanced bool

	// form field backing variables — AI provider
	aiSetupPreset        string
	aiProvider           string
	claudeAPIKey         string
	claudeModel          string
	openAIAPIKey         string
	openAIBaseURL        string
	openAIModel          string
	openAIEmbeddingModel string
	embeddingProvider    string
	agentReasoningEffort string
	ollamaHost           string
	ollamaModel          string
	ollamaModelChoice    string
	ollamaModelCustom    string
	embedModel           string
	embedModelChoice     string
	embedModelCustom     string

	// form field backing variables — sync & cleanup
	syncPollStr                string
	syncIDLE                   bool
	cleanupScheduleStr         string
	cacheStoragePolicy         string
	reclaimOfflineCacheStorage bool
	cleanupToolSelection       string

	// form field backing variables — memories
	memoryEnabled                  bool
	memoryDirectory                string
	memorySourceFolders            string
	memoryTaskChoices              []string
	memoryObsidianEnabled          bool
	memoryVaultPath                string
	memoryFrontmatterMode          string
	memoryYAMLHeaders              bool
	memoryLinkMode                 string
	memoryTagMode                  string
	memoryUpdateCadence            string
	memoryLowConfidenceDisposition string
	memoryChatThresholdStr         string
	memoryDossierThresholdStr      string
	memoryObsidianThresholdStr     string
	memoryComposeThresholdStr      string
	memoryMatchThresholdStr        string
	memoryStaleAfterDaysStr        string
	memoryRetentionDaysStr         string
	memoryPeopleDestination        string
	memoryCompaniesDestination     string
	memoryJobSearchDestination     string
	memoryProjectsDestination      string
	memoryThreadsDestination       string
	memoryResearchDestination      string
	memoryDailyDestination         string
	memoryInboxDestination         string
	memoryPromptTemplateChoice     string
	memoryStoreStats               memory.StoreStats
	memoryObsidianSyncState        memory.ObsidianSyncState

	// form field backing variables — calendar
	calendarWeekStart string

	// form field backing variables — compose
	signatureText string

	// form field backing variables — keyboard
	keyboardProfile string
	customKeymap    string

	// form field backing variables — theme
	themeName        string
	themeInstallPath string
	themeRole        string
	themeFG          string
	themeBG          string
	themeOverrides   map[string]config.ThemeOverride
	themeSaveAs      string
	themeResetRole   bool
	themeResetAll    bool

	bypassWizardBackValidation bool
}

type conditionalSettingsField struct {
	field huh.Field
	hide  func() bool
}

func hideSettingsFieldWhen(field huh.Field, hide func() bool) huh.Field {
	return &conditionalSettingsField{field: field, hide: hide}
}

func requiredSettingsValue(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

func (f *conditionalSettingsField) hidden() bool {
	return f != nil && f.hide != nil && f.hide()
}

func (f *conditionalSettingsField) Init() tea.Cmd {
	return f.field.Init()
}

func (f *conditionalSettingsField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if f.hidden() {
		return f, nil
	}
	model, cmd := f.field.Update(msg)
	if field, ok := model.(huh.Field); ok {
		f.field = field
	}
	return f, cmd
}

func (f *conditionalSettingsField) View() string {
	if f.hidden() {
		return ""
	}
	return f.field.View()
}

func (f *conditionalSettingsField) Blur() tea.Cmd {
	if f.hidden() {
		return nil
	}
	return f.field.Blur()
}

func (f *conditionalSettingsField) Focus() tea.Cmd {
	if f.hidden() {
		return nil
	}
	return f.field.Focus()
}

func (f *conditionalSettingsField) Error() error {
	if f.hidden() {
		return nil
	}
	return f.field.Error()
}

func (f *conditionalSettingsField) Run() error {
	if f.hidden() {
		return nil
	}
	return f.field.Run()
}

func (f *conditionalSettingsField) RunAccessible(w io.Writer, r io.Reader) error {
	if f.hidden() {
		return nil
	}
	return f.field.RunAccessible(w, r)
}

func (f *conditionalSettingsField) Skip() bool {
	return f.hidden() || f.field.Skip()
}

func (f *conditionalSettingsField) Zoom() bool {
	return !f.hidden() && f.field.Zoom()
}

func (f *conditionalSettingsField) KeyBinds() []key.Binding {
	if f.hidden() {
		return nil
	}
	return f.field.KeyBinds()
}

func (f *conditionalSettingsField) WithTheme(theme huh.Theme) huh.Field {
	f.field = f.field.WithTheme(theme)
	return f
}

func (f *conditionalSettingsField) WithKeyMap(keymap *huh.KeyMap) huh.Field {
	f.field = f.field.WithKeyMap(keymap)
	return f
}

func (f *conditionalSettingsField) WithWidth(width int) huh.Field {
	f.field = f.field.WithWidth(width)
	return f
}

func (f *conditionalSettingsField) WithHeight(height int) huh.Field {
	f.field = f.field.WithHeight(height)
	return f
}

func (f *conditionalSettingsField) WithPosition(position huh.FieldPosition) huh.Field {
	f.field = f.field.WithPosition(position)
	return f
}

func (f *conditionalSettingsField) GetKey() string {
	return f.field.GetKey()
}

func (f *conditionalSettingsField) GetValue() any {
	return f.field.GetValue()
}

type settingsPanelLayout struct {
	panelWidth  int
	panelHeight int
	formWidth   int
	formHeight  int
}

// NewSettings creates a Settings component, pre-filling fields from an existing config.
// If existing is nil, a zero-value config is used.
func NewSettings(mode SettingsMode, existing *config.Config) *Settings {
	return NewSettingsWithOptions(mode, existing, defaultSettingsOptions(mode))
}

// NewSettingsWithOptions creates a Settings component with caller-specified options.
func NewSettingsWithOptions(mode SettingsMode, existing *config.Config, opts SettingsOptions) *Settings {
	return NewSettingsWithPathAndOptions(mode, existing, "", opts)
}

// NewSettingsWithPath creates a Settings component with an explicit config file path for saving.
func NewSettingsWithPath(mode SettingsMode, existing *config.Config, configPath string) *Settings {
	return NewSettingsWithPathAndOptions(mode, existing, configPath, defaultSettingsOptions(mode))
}

// NewSettingsWithPathAndOptions creates a Settings component with an explicit
// config path and caller-specified options.
func NewSettingsWithPathAndOptions(mode SettingsMode, existing *config.Config, configPath string, opts SettingsOptions) *Settings {
	s := &Settings{
		mode:                          mode,
		cfg:                           &config.Config{},
		configPath:                    configPath,
		syncIDLE:                      true, // sensible default
		saveButton:                    true,
		panelMenuChoice:               string(settingsPanelSectionAccounts),
		accountsMenuChoice:            settingsAccountEditAddMail,
		addAccountChoice:              settingsAccountEditAddMail,
		accountEditMode:               settingsAccountEditExisting,
		calendarProvider:              "google_calendar",
		showExperimentalEmailServices: opts.ShowExperimentalEmailServices,
		firstRunAccountOnly:           opts.FirstRunAccountOnly,
		firstRunPreferencesOnly:       opts.FirstRunPreferencesOnly,
		firstRunPreferenceAction:      firstRunPreferenceEnter,
	}

	if existing != nil {
		// Deep copy the relevant fields.
		s.cfg = existing
		s.provider = existing.Vendor
		s.email = existing.Credentials.Username
		s.password = existing.Credentials.Password
		s.imapHost = existing.Server.Host
		s.imapPort = portToString(existing.Server.Port)
		s.smtpHost = existing.SMTP.Host
		s.smtpPort = portToString(existing.SMTP.Port)

		// AI provider fields
		s.aiProvider = existing.AI.Provider
		s.ollamaHost = existing.Ollama.Host
		s.ollamaModel = existing.Ollama.Model
		s.embeddingProvider = existing.EffectiveEmbeddingProvider()
		s.embedModel = existing.Ollama.EmbeddingModel
		if s.embedModel == "" && s.embeddingProvider == config.EmbeddingProviderOllama {
			s.embedModel = existing.EffectiveEmbeddingModel()
		}
		s.claudeAPIKey = existing.Claude.APIKey
		s.claudeModel = existing.Claude.Model
		s.openAIAPIKey = existing.OpenAI.APIKey
		s.openAIBaseURL = existing.OpenAI.BaseURL
		s.openAIModel = existing.OpenAI.Model
		s.openAIEmbeddingModel = existing.OpenAI.EmbeddingModel
		if s.openAIEmbeddingModel == "" && s.embeddingProvider == config.EmbeddingProviderOpenAI {
			s.openAIEmbeddingModel = existing.EffectiveEmbeddingModel()
		}
		s.agentReasoningEffort = agent.NormalizeReasoningEffort(existing.AI.Agent.ReasoningEffort)
		if s.agentReasoningEffort == "" {
			s.agentReasoningEffort = defaultChatEffort
		}

		// Sync & cleanup fields
		s.syncPollStr = strconv.Itoa(existing.Sync.PollIntervalMinutes)
		s.syncIDLE = existing.Sync.IDLEEnabled
		s.cleanupScheduleStr = strconv.Itoa(existing.Cleanup.ScheduleHours)
		s.cacheStoragePolicy = config.NormalizeCacheStoragePolicy(existing.Cache.StoragePolicy)
		s.calendarWeekStart = config.NormalizeCalendarWeekStart(existing.Calendar.WeekStart)
		s.signatureText = existing.Compose.Signature.Text
		s.keyboardProfile = existing.Keyboard.Profile
		s.customKeymap = existing.Keyboard.CustomKeymap
		s.themeName = existing.Theme.Name
		s.themeOverrides = cloneThemeOverrides(existing.Theme.Overrides)
		s.themeRole = firstThemeRole(existing.Theme.Overrides)
		if override, ok := existing.Theme.Overrides[s.themeRole]; ok {
			s.themeFG = override.Foreground
			s.themeBG = override.Background
		}

		if existing.IsGmailOAuth() {
			s.provider = "gmail-oauth"
			s.email = existing.Gmail.Email
		} else if existing.Gmail.Email != "" && s.email == "" {
			s.email = existing.Gmail.Email
		}
	}
	if existing != nil {
		s.syncMemoryFieldsFromConfig(existing.Memories)
	} else {
		s.syncMemoryFieldsFromConfig(memory.DefaultSettings())
	}

	// Default new setup to Google's supported browser authorization path.
	if s.provider == "" {
		s.provider = "gmail-oauth"
	}
	if s.firstRunAccountOnly {
		s.firstRunOtherProvider = s.provider != "gmail-oauth"
	}
	s.normalizeAIProvider()
	s.lastAIProvider = s.aiProvider
	_ = s.syncExternalAIDefaults()
	if s.firstRunAccountOnly && s.provider == "gmail-oauth" {
		s.alsoAddCalendar = true
		s.calendarProvider = "google_calendar"
		if existing == nil || strings.TrimSpace(existing.AI.Provider) == "" {
			s.aiProvider = aiProviderDisabled
			s.syncAIDefaults()
			_ = s.syncExternalAIDefaults()
		}
	}
	if s.firstRunPreferencesOnly && (existing == nil || strings.TrimSpace(existing.AI.Provider) == "") {
		s.aiProvider = aiProviderDisabled
		s.syncAIDefaults()
		_ = s.syncExternalAIDefaults()
	}
	s.aiSetupPreset = s.inferAISetupPreset()
	if s.syncPollStr == "" {
		s.syncPollStr = "5" // default only on first run; 0 is valid (IDLE-only mode)
	}
	if s.cleanupScheduleStr == "" {
		s.cleanupScheduleStr = "0"
	}
	if s.cacheStoragePolicy == "" {
		s.cacheStoragePolicy = config.CacheStoragePolicyNoAttachments
	}
	s.calendarWeekStart = config.NormalizeCalendarWeekStart(s.calendarWeekStart)
	if s.keyboardProfile == "" {
		s.keyboardProfile = keyboardProfileDefault
	}
	if s.themeName == "" {
		s.themeName = "inherited"
	}
	if s.themeRole == "" {
		s.themeRole = "chrome.tab_active"
	}
	if s.themeOverrides == nil {
		s.themeOverrides = make(map[string]config.ThemeOverride)
	}
	s.loadThemeFieldsForRole(s.themeRole)
	s.syncAIModelChoicesFromValues()

	s.syncProviderDefaults("", s.provider)
	s.buildForm()
	return s
}

func defaultSettingsOptions(mode SettingsMode) SettingsOptions {
	return SettingsOptions{
		ShowExperimentalEmailServices: true,
	}
}

func (s *Settings) syncMemoryFieldsFromConfig(settings memory.Settings) {
	settings.ApplyDefaults()
	s.memoryEnabled = settings.Enabled
	s.memoryDirectory = strings.TrimSpace(firstNonEmptyString(settings.Directory, memory.DefaultDirectory))
	s.memorySourceFolders = strings.Join(settings.Sources.Folders, ", ")
	s.memoryTaskChoices = settingsMemoryTaskChoices(settings.Tasks)
	s.memoryObsidianEnabled = settings.Obsidian.Enabled
	s.memoryVaultPath = strings.TrimSpace(settings.Obsidian.VaultPath)
	s.memoryFrontmatterMode = memory.NormalizeFrontmatterMode(settings.Obsidian.FrontmatterMode)
	s.memoryYAMLHeaders = settings.Obsidian.YAMLHeaders
	s.memoryLinkMode = memory.NormalizeLinkMode(settings.Obsidian.LinkMode)
	s.memoryTagMode = memory.NormalizeTagMode(settings.Obsidian.TagMode)
	s.memoryUpdateCadence = settingsMemoryUpdateCadence(settings.UpdateRules.Cadence)
	s.memoryLowConfidenceDisposition = settingsMemoryLowConfidenceDisposition(settings.UpdateRules.LowConfidenceDisposition)
	s.memoryChatThresholdStr = settingsFormatFloat(settings.Thresholds.ChatRetrieval)
	s.memoryDossierThresholdStr = settingsFormatFloat(settings.Thresholds.Dossier)
	s.memoryObsidianThresholdStr = settingsFormatFloat(settings.Thresholds.ObsidianWrite)
	s.memoryComposeThresholdStr = settingsFormatFloat(settings.Thresholds.ComposeRadar)
	s.memoryMatchThresholdStr = settingsFormatFloat(settings.Thresholds.Match)
	s.memoryStaleAfterDaysStr = strconv.Itoa(settings.UpdateRules.StaleAfterDays)
	s.memoryRetentionDaysStr = strconv.Itoa(settings.UpdateRules.RetentionDays)
	s.memoryPeopleDestination = settings.Destinations.People
	s.memoryCompaniesDestination = settings.Destinations.Companies
	s.memoryJobSearchDestination = settings.Destinations.JobSearch
	s.memoryProjectsDestination = settings.Destinations.Projects
	s.memoryThreadsDestination = settings.Destinations.Threads
	s.memoryResearchDestination = settings.Destinations.Research
	s.memoryDailyDestination = settings.Destinations.DailyBriefing
	s.memoryInboxDestination = settings.Destinations.Inbox
	if len(settings.Prompts) > 0 {
		s.memoryPromptTemplateChoice = settings.Prompts[0].Name
	}
}

func settingsFormatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}

func settingsParseFloatOr(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 || parsed > 1 {
		return fallback
	}
	return parsed
}

func settingsParsePositiveIntOr(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func settingsParseNonNegativeIntOr(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func settingsParseCSV(value string, fallback []string) []string {
	raw := strings.NewReplacer("\n", ",", ";", ",").Replace(value)
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool, len(parts))
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[strings.ToLower(part)] {
			continue
		}
		out = append(out, part)
		seen[strings.ToLower(part)] = true
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func settingsMemoryThresholdValidator(label string) func(string) error {
	return func(value string) error {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || parsed < 0 || parsed > 1 {
			return fmt.Errorf("%s must be between 0 and 1", label)
		}
		return nil
	}
}

func settingsPositiveIntValidator(label string) func(string) error {
	return func(value string) error {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed <= 0 {
			return fmt.Errorf("%s must be a positive integer", label)
		}
		return nil
	}
}

func settingsNonNegativeIntValidator(label string) func(string) error {
	return func(value string) error {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 0 {
			return fmt.Errorf("%s must be zero or a positive integer", label)
		}
		return nil
	}
}

func settingsMemoryUpdateCadence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "compose_open", "after_sync", "daily_briefing", "background_idle":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "manual"
	}
}

func settingsMemoryLowConfidenceDisposition(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case memory.LowConfidenceHidden, memory.LowConfidenceReview:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return memory.LowConfidenceChat
	}
}

func settingsBoolWord(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func settingsMaybeUnset(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(not set)"
	}
	return value
}

func settingsMemoryRetentionLabel(value string) string {
	days := settingsParseNonNegativeIntOr(firstNonEmptyString(value, "0"), 0)
	if days == 0 {
		return "forever"
	}
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

func settingsMemoryFrontmatterOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Minimal YAML", memory.FrontmatterMinimal),
		huh.NewOption("Full YAML", memory.FrontmatterFull),
		huh.NewOption("Generated-section metadata", memory.FrontmatterGenerated),
		huh.NewOption("No visible YAML", memory.FrontmatterNone),
	}
}

func settingsMemoryLinkModeOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Wiki links", memory.LinkModeWiki),
		huh.NewOption("Markdown links", memory.LinkModeMarkdown),
		huh.NewOption("Path text", memory.LinkModePath),
		huh.NewOption("No links", memory.LinkModeNone),
	}
}

func settingsMemoryTagModeOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Conservative tags", memory.TagModeConservative),
		huh.NewOption("Workflow tags", memory.TagModeWorkflow),
		huh.NewOption("No tags", memory.TagModeNone),
		huh.NewOption("Custom config tags", memory.TagModeCustom),
	}
}

func settingsMemoryCadenceOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Manual - only when explicitly refreshed", "manual"),
		huh.NewOption("On compose open - extract before reply context", "compose_open"),
		huh.NewOption("After sync - extract after mail sync", "after_sync"),
		huh.NewOption("Daily briefing - extract during briefing generation", "daily_briefing"),
		huh.NewOption("Background idle - extract when the app is idle", "background_idle"),
	}
}

func settingsMemoryTaskOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Extract memories from cached mail", settingsMemoryTaskExtraction),
		huh.NewOption("Update track status", settingsMemoryTaskTrack),
		huh.NewOption("Show Compose Radar nudges", settingsMemoryTaskRadar),
		huh.NewOption("Build contact and thread dossiers", settingsMemoryTaskDossiers),
		huh.NewOption("Format Obsidian sections", settingsMemoryTaskObsidian),
		huh.NewOption("Summarize research notes", settingsMemoryTaskResearch),
	}
}

func settingsMemoryTaskChoices(tasks memory.TaskSettings) []string {
	tasks.ApplyDefaults()
	var choices []string
	if tasks.MemoryExtraction {
		choices = append(choices, settingsMemoryTaskExtraction)
	}
	if tasks.TrackStatusUpdate {
		choices = append(choices, settingsMemoryTaskTrack)
	}
	if tasks.ComposeRadarNudges {
		choices = append(choices, settingsMemoryTaskRadar)
	}
	if tasks.Dossiers {
		choices = append(choices, settingsMemoryTaskDossiers)
	}
	if tasks.ObsidianSectionFormat {
		choices = append(choices, settingsMemoryTaskObsidian)
	}
	if tasks.ResearchNoteSummary {
		choices = append(choices, settingsMemoryTaskResearch)
	}
	return choices
}

func settingsMemoryTasksFromChoices(choices []string) memory.TaskSettings {
	enabled := make(map[string]bool, len(choices))
	for _, choice := range choices {
		enabled[strings.TrimSpace(choice)] = true
	}
	return memory.NewTaskSettings(
		enabled[settingsMemoryTaskExtraction],
		enabled[settingsMemoryTaskTrack],
		enabled[settingsMemoryTaskRadar],
		enabled[settingsMemoryTaskDossiers],
		enabled[settingsMemoryTaskObsidian],
		enabled[settingsMemoryTaskResearch],
	)
}

func settingsMemoryTaskSummary(choices []string) string {
	options := settingsMemoryTaskOptions()
	enabled := make(map[string]bool, len(choices))
	for _, choice := range choices {
		enabled[strings.TrimSpace(choice)] = true
	}
	on := 0
	var lines []string
	for _, option := range options {
		state := "off"
		if enabled[option.Value] {
			state = "on"
			on++
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", option.Key, state))
	}
	return fmt.Sprintf("%d of %d memory tasks enabled\n%s", on, len(options), strings.Join(lines, "\n"))
}

func settingsMemoryLowConfidenceOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Show in chat only", memory.LowConfidenceChat),
		huh.NewOption("Hide", memory.LowConfidenceHidden),
		huh.NewOption("Review queue", memory.LowConfidenceReview),
	}
}

func (s *Settings) memoryPromptTemplatesForSettings() []memory.PromptTemplate {
	settings := memory.DefaultSettings()
	if s != nil && s.cfg != nil {
		settings = s.cfg.Memories
		settings.ApplyDefaults()
	}
	return append([]memory.PromptTemplate(nil), settings.Prompts...)
}

func (s *Settings) memoryPromptTemplateSummary() string {
	templates := s.memoryPromptTemplatesForSettings()
	lines := []string{fmt.Sprintf("%d exposed templates", len(templates))}
	for _, tmpl := range templates {
		name := strings.TrimSpace(tmpl.Name)
		version := strings.TrimSpace(tmpl.Version)
		if name == "" {
			continue
		}
		if version != "" {
			lines = append(lines, fmt.Sprintf("- %s (%s)", name, version))
		} else {
			lines = append(lines, "- "+name)
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Settings) memoryPromptTemplateOptions() []huh.Option[string] {
	templates := s.memoryPromptTemplatesForSettings()
	if len(templates) == 0 {
		return []huh.Option[string]{huh.NewOption("Default templates", "default")}
	}
	options := make([]huh.Option[string], 0, len(templates))
	for _, tmpl := range templates {
		name := strings.TrimSpace(tmpl.Name)
		if name == "" {
			continue
		}
		label := name
		if version := strings.TrimSpace(tmpl.Version); version != "" {
			label = fmt.Sprintf("%s (%s)", name, version)
		}
		options = append(options, huh.NewOption(label, name))
	}
	if len(options) == 0 {
		return []huh.Option[string]{huh.NewOption("Default templates", "default")}
	}
	return options
}

func (s *Settings) memoryResearchStatusLine() string {
	settings := memory.DefaultSettings()
	if s != nil && s.cfg != nil {
		settings = s.cfg.Memories
		settings.ApplyDefaults()
	}
	external := "off unless explicitly enabled in config"
	if settings.Research.ExternalOptIn {
		external = "opt-in on"
	}
	privateBodies := "off"
	if settings.Research.PrivateBodiesAllowed {
		privateBodies = "on"
	}
	return fmt.Sprintf("Safe external research: %s / private body web research: %s", external, privateBodies)
}

func (s *Settings) refreshMemoryStoreStats() {
	if s == nil {
		return
	}
	settings := memory.DefaultSettings()
	if s.cfg != nil {
		settings = s.cfg.Memories
	}
	memorySettings := s.buildMemorySettingsConfig(settings)
	s.memoryStoreStats = memory.StoreStatsForSettings(context.Background(), memorySettings)
	s.memoryObsidianSyncState = memory.ObsidianSyncStateForSettings(context.Background(), memorySettings)
}

func (s *Settings) memoryStoreStatsLine() string {
	stats := s.memoryStoreStats
	if stats.Unavailable {
		detail := strings.TrimSpace(stats.Error)
		if detail == "" {
			detail = "unavailable"
		}
		return "Memory records: unavailable (" + render.Truncate(detail, 72) + ")"
	}
	return fmt.Sprintf("Memory records: total %d / stale %d / review %d", stats.Total, stats.Stale, stats.ReviewNeeded)
}

func (s *Settings) memoryObsidianSyncLine() string {
	state := s.memoryObsidianSyncState
	if !s.memoryObsidianEnabled {
		return ""
	}
	if state.Unavailable {
		detail := strings.TrimSpace(state.Error)
		if detail == "" {
			detail = "unavailable"
		}
		return "Obsidian writes: unavailable (" + render.Truncate(detail, 72) + ")"
	}
	approval := "approved"
	if state.PreviewRequired {
		approval = "needs preview approval"
	} else if !state.Approved && state.PendingWrites > 0 {
		approval = "preview optional"
	}
	lastRun := "never"
	if !state.LastRun.IsZero() {
		lastRun = state.LastRun.Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("Obsidian writes: pending %d / applied %d / failed %d / last %s / %s",
		state.PendingWrites,
		state.AppliedWrites,
		state.FailedWrites,
		lastRun,
		approval,
	)
}

func (s *Settings) memoryStatusDescription() string {
	folders := strings.Join(settingsParseCSV(s.memorySourceFolders, []string{"INBOX", "Sent"}), ", ")
	if folders == "" {
		folders = "INBOX, Sent"
	}
	taskCount := len(s.memoryTaskChoices)
	taskTotal := len(settingsMemoryTaskOptions())
	lines := []string{
		fmt.Sprintf("Enabled: %s", settingsBoolWord(s.memoryEnabled)),
		"Store: " + settingsMaybeUnset(firstNonEmptyString(s.memoryDirectory, memory.DefaultDirectory)),
		"Immutable records: on",
		"Sources: " + folders,
		"Vault: " + settingsMaybeUnset(s.memoryVaultPath),
		fmt.Sprintf("Memory tasks: %d of %d enabled", taskCount, taskTotal),
		fmt.Sprintf("Obsidian: %s / YAML %s / %s links / %s tags",
			settingsBoolWord(s.memoryObsidianEnabled),
			settingsBoolWord(s.memoryYAMLHeaders),
			memory.NormalizeLinkMode(s.memoryLinkMode),
			memory.NormalizeTagMode(s.memoryTagMode),
		),
		fmt.Sprintf("Extraction trigger: %s", settingsMemoryUpdateCadence(s.memoryUpdateCadence)),
		fmt.Sprintf("Prompts: %d exposed templates", len(s.memoryPromptTemplatesForSettings())),
		s.memoryStoreStatsLine(),
		s.memoryResearchStatusLine(),
	}
	if syncLine := s.memoryObsidianSyncLine(); syncLine != "" {
		lines = append(lines[:len(lines)-1], syncLine, lines[len(lines)-1])
	}
	return strings.Join(lines, "\n")
}

func (s *Settings) accountTypeDescription() string {
	if s.mode == SettingsModePanel {
		return "Recommended: Gmail OAuth. Supported: Standard IMAP and Gmail App Password. Experimental: ProtonMail Bridge, Fastmail, iCloud, Outlook."
	}
	return "Recommended: Gmail OAuth. Supported: Standard IMAP and Gmail App Password."
}

func (s *Settings) accountTypeOptions() []huh.Option[string] {
	if s.mode == SettingsModePanel {
		return []huh.Option[string]{
			huh.NewOption("Gmail OAuth", "gmail-oauth"),
			huh.NewOption("Standard IMAP", "imap"),
			huh.NewOption("Gmail (IMAP + App Password)", "gmail"),
			huh.NewOption("ProtonMail Bridge (Experimental)", "protonmail"),
			huh.NewOption("Fastmail (Experimental)", "fastmail"),
			huh.NewOption("iCloud (Experimental)", "icloud"),
			huh.NewOption("Outlook (Experimental)", "outlook"),
		}
	}

	options := []huh.Option[string]{
		huh.NewOption("Gmail OAuth", "gmail-oauth"),
		huh.NewOption("Standard IMAP", "imap"),
		huh.NewOption("Gmail (IMAP + App Password)", "gmail"),
	}
	return append(options,
		huh.NewOption("ProtonMail Bridge", "protonmail"),
		huh.NewOption("Fastmail", "fastmail"),
		huh.NewOption("iCloud", "icloud"),
		huh.NewOption("Outlook", "outlook"),
	)
}

func (s *Settings) providerPresetDescription(base string) string {
	if s.mode == SettingsModeWizard {
		return base
	}
	return base + " This path is still experimental."
}

func (s *Settings) validateSetupEmail(value string) error {
	if s.bypassWizardBackValidation {
		return nil
	}
	return validateEmail(value)
}

func ollamaChatModelOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("gemma3:4b - recommended quality default (~3.3GB)", "gemma3:4b"),
		huh.NewOption("qwen3.5:0.8b - downgrade for constrained RAM (~1.0GB)", "qwen3.5:0.8b"),
		huh.NewOption("llama3.2:1b - smallest downgrade, weaker translation (~1.3GB)", "llama3.2:1b"),
		huh.NewOption("llama3.2:3b - downgrade, llama3.x translation weaker (~2.0GB)", "llama3.2:3b"),
		huh.NewOption("Custom model name", customModelChoice),
	}
}

func ollamaEmbeddingModelOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("nomic-embed-text-v2-moe - recommended multilingual default (~958MB)", "nomic-embed-text-v2-moe"),
		huh.NewOption("nomic-embed-text - smaller downgrade (~274MB, 2K context)", "nomic-embed-text"),
		huh.NewOption("all-minilm - smallest embeddings (~46MB)", "all-minilm"),
		huh.NewOption("mxbai-embed-large - larger general-purpose embeddings (~670MB)", "mxbai-embed-large"),
		huh.NewOption("bge-m3 - multilingual, larger (~1.2GB)", "bge-m3"),
		huh.NewOption("Custom model name", customModelChoice),
	}
}

func modelChoiceForValue(value string, options []huh.Option[string], defaultValue string) (choice, custom string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultValue
	}
	for _, option := range options {
		if option.Value == value {
			return value, ""
		}
	}
	return customModelChoice, value
}

func selectedModelValue(choice, custom, current, defaultValue string) string {
	switch strings.TrimSpace(choice) {
	case "":
		if trimmed := strings.TrimSpace(current); trimmed != "" {
			return trimmed
		}
		return defaultValue
	case customModelChoice:
		if trimmed := strings.TrimSpace(custom); trimmed != "" {
			return trimmed
		}
		if trimmed := strings.TrimSpace(current); trimmed != "" {
			return trimmed
		}
		return defaultValue
	default:
		return strings.TrimSpace(choice)
	}
}

func aiSetupPresetOptions(current string) []huh.Option[string] {
	return prioritizeOption([]huh.Option[string]{
		huh.NewOption("Ollama local default", aiSetupPresetOllamaDefault),
		huh.NewOption("Ollama local custom", aiSetupPresetOllamaCustom),
		huh.NewOption("OpenAI default", aiSetupPresetOpenAIDefault),
		huh.NewOption("Claude API default", aiSetupPresetClaudeDefault),
		huh.NewOption("OpenAI-compatible endpoint", aiSetupPresetOpenAICompatible),
		huh.NewOption("AI disabled", aiSetupPresetDisabled),
		huh.NewOption("Advanced manual config", aiSetupPresetAdvancedManual),
	}, current)
}

func chatRoleOptions(current string) []huh.Option[string] {
	ollamaValue := aiProviderOllamaDefault
	if strings.TrimSpace(current) == aiProviderOllamaCustom {
		ollamaValue = aiProviderOllamaCustom
	}
	return prioritizeOption([]huh.Option[string]{
		huh.NewOption("Ollama local", ollamaValue),
		huh.NewOption("OpenAI / compatible", "openai"),
		huh.NewOption("Claude API", "claude"),
		huh.NewOption("AI disabled", aiProviderDisabled),
	}, current)
}

func embeddingRoleOptions(current string) []huh.Option[string] {
	return prioritizeOption([]huh.Option[string]{
		huh.NewOption("Ollama local", config.EmbeddingProviderOllama),
		huh.NewOption("OpenAI / compatible", config.EmbeddingProviderOpenAI),
	}, current)
}

func prioritizeOption(options []huh.Option[string], current string) []huh.Option[string] {
	current = strings.TrimSpace(current)
	if current == "" || len(options) == 0 || options[0].Value == current {
		return options
	}
	for i, option := range options {
		if option.Value != current {
			continue
		}
		out := make([]huh.Option[string], 0, len(options))
		out = append(out, option)
		out = append(out, options[:i]...)
		out = append(out, options[i+1:]...)
		return out
	}
	return options
}

func (s *Settings) syncAIModelChoicesFromValues() {
	s.ollamaModelChoice, s.ollamaModelCustom = modelChoiceForValue(s.ollamaModel, ollamaChatModelOptions(), defaultOllamaModel)
	s.embedModelChoice, s.embedModelCustom = modelChoiceForValue(s.embedModel, ollamaEmbeddingModelOptions(), defaultEmbeddingModel)
}

func (s *Settings) requiresAccountValidation() bool {
	if s.mode == SettingsModeWizard {
		return !s.firstRunPreferencesOnly
	}
	if s.panelSection != settingsPanelSectionAccount || s.deleteAccount || !s.accountDetailShowsMail() {
		return false
	}
	if s.accountEditMode == settingsAccountEditExisting {
		return s.mailCredentialsChanged()
	}
	return true
}

func (s *Settings) mailCredentialsChanged() bool {
	source, ok := s.existingSourceForKind(models.SourceKindMail)
	if !ok {
		return true
	}
	if strings.TrimSpace(s.provider) != "" && !mailProvidersEquivalentForSource(s.provider, source) {
		return true
	}
	if strings.TrimSpace(s.email) != strings.TrimSpace(sourceAddressForSettings(source)) {
		return true
	}
	if s.provider != "gmail-oauth" && !settingsMailSourceUsesGoogleOAuth(source) && s.password != source.Credentials.Password {
		return true
	}
	return false
}

func mailProvidersEquivalentForSource(formProvider string, source config.SourceConfig) bool {
	if strings.TrimSpace(formProvider) == "gmail-oauth" && settingsMailSourceUsesGoogleOAuth(source) {
		return true
	}
	return mailProvidersEquivalent(formProvider, source.Provider)
}

func mailProvidersEquivalent(formProvider, sourceProvider string) bool {
	form := mailSourceProviderForSettings(strings.TrimSpace(formProvider))
	source := strings.TrimSpace(sourceProvider)
	if form == source {
		return true
	}
	return (form == "gmail" && source == "gmail_api") || (form == "gmail_api" && source == "gmail")
}

func (s *Settings) calendarCredentialsChanged() bool {
	source, ok := s.existingSourceForKind(models.SourceKindCalendar)
	if !ok {
		return true
	}
	provider := strings.TrimSpace(source.Provider)
	if provider == "" {
		provider = "google_calendar"
	}
	if calendarProviderUsesCalDAV(provider) {
		return strings.TrimSpace(s.caldavUsername) != strings.TrimSpace(source.CalDAV.Username) ||
			s.caldavPassword != source.CalDAV.Password
	}
	return strings.TrimSpace(s.calendarEmail) != strings.TrimSpace(source.Google.Email)
}

// buildForm constructs the huh.Form with groups for account, AI provider, and sync preferences.
func (s *Settings) buildForm() {
	if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionMenu {
		s.buildPanelMenuForm()
		return
	}
	if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAccounts {
		s.buildAccountsListForm()
		return
	}
	if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAddAccount {
		s.panelSection = settingsPanelSectionAccounts
		s.buildAccountsListForm()
		return
	}
	s.normalizeCalendarProviderChoice()
	s.syncCalendarProviderDefaults("", s.calendarProvider)
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		s.firstRunOtherProvider = s.provider != "gmail-oauth"
	}

	// Group 1 — Account type selection
	accountTitle := "Account Type"
	accountGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title(accountTitle).
			Description(s.accountTypeDescription()).
			Options(s.accountTypeOptions()...).
			Value(&s.provider),
	).WithHideFunc(func() bool { return s.mailSettingsHidden() })

	credentialsIntro := huh.NewNote().
		TitleFunc(func() string {
			if s.provider == "imap" {
				if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
					return "Custom IMAP"
				}
				return "Standard IMAP"
			}
			if s.mode == SettingsModeWizard {
				return "IMAP preset"
			}
			return "Experimental preset"
		}, &s.provider).
		DescriptionFunc(func() string {
			switch s.provider {
			case "imap":
				return "Use this for providers where you already know the IMAP and SMTP settings."
			case "protonmail":
				return s.providerPresetDescription("Requires ProtonMail Bridge on localhost. Herald prefills the known Bridge ports." + providerPresetSummary(s.provider))
			default:
				return s.providerPresetDescription("Herald prefills the known IMAP/SMTP defaults for this provider." + providerPresetSummary(s.provider))
			}
		}, &s.provider)

	// Group 2a — Credentials for Standard IMAP and experimental vendor presets
	credentialsGroup := huh.NewGroup(
		credentialsIntro,
		huh.NewInput().Title("Email address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
		huh.NewInput().Title("Password").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.password),
		huh.NewInput().Title("IMAP Host").Inline(true).Value(&s.imapHost).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "imap-host") }, &s.provider),
		huh.NewInput().Title("IMAP Port").Inline(true).Value(&s.imapPort).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "imap-port") }, &s.provider).
			Validate(func(v string) error {
				if v == "" {
					return nil
				}
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 || n > 65535 {
					return errors.New("must be a port number (1–65535)")
				}
				return nil
			}),
		huh.NewInput().Title("SMTP Host").Inline(true).Value(&s.smtpHost).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "smtp-host") }, &s.provider),
		huh.NewInput().Title("SMTP Port").Inline(true).Value(&s.smtpPort).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "smtp-port") }, &s.provider).
			Validate(func(v string) error {
				if v == "" {
					return nil
				}
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 || n > 65535 {
					return errors.New("must be a port number (1–65535)")
				}
				return nil
			}),
	).WithHideFunc(func() bool { return s.mailSettingsHidden() || s.provider == "gmail" || s.provider == "gmail-oauth" })

	// Group 2b — Gmail IMAP fallback guidance and credentials
	gmailGroup := huh.NewGroup(
		huh.NewNote().
			Title("Gmail via IMAP App Password").
			Description("Fallback Gmail setup. Use your Gmail address and a Google App Password when OAuth is unavailable for your account."),
		huh.NewInput().Title("Gmail address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
		huh.NewInput().Title("App Password").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.password),
		huh.NewConfirm().
			Title("Edit advanced Gmail server settings").
			Value(&s.editGmailAdvanced),
	).WithHideFunc(func() bool { return s.mailSettingsHidden() || s.provider != "gmail" })

	gmailAdvancedGroup := huh.NewGroup(
		huh.NewInput().Title("IMAP Host").Inline(true).Value(&s.imapHost).Placeholder("imap.gmail.com"),
		huh.NewInput().Title("IMAP Port").Inline(true).Value(&s.imapPort).Placeholder("993").
			Validate(func(v string) error {
				if v == "" {
					return nil
				}
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 || n > 65535 {
					return errors.New("must be a port number (1–65535)")
				}
				return nil
			}),
		huh.NewInput().Title("SMTP Host").Inline(true).Value(&s.smtpHost).Placeholder("smtp.gmail.com"),
		huh.NewInput().Title("SMTP Port").Inline(true).Value(&s.smtpPort).Placeholder("587").
			Validate(func(v string) error {
				if v == "" {
					return nil
				}
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 || n > 65535 {
					return errors.New("must be a port number (1–65535)")
				}
				return nil
			}),
	).WithHideFunc(func() bool { return s.mailSettingsHidden() || s.provider != "gmail" || !s.editGmailAdvanced })

	// Group 2c — Gmail OAuth notice
	gmailOAuthGroup := huh.NewGroup(
		huh.NewNote().
			Title("Gmail OAuth").
			Description("Recommended browser-based Gmail setup. Herald verifies Google access before saving."),
		huh.NewInput().Title("Gmail address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
	).WithHideFunc(func() bool { return s.mailSettingsHidden() || s.provider != "gmail-oauth" })

	firstRunGoogleGroup := huh.NewGroup(
		huh.NewNote().
			Title("Mail and Calendar").
			Description("Mail: Enabled\nCalendar: On by default\nFinishing this step verifies Google access before saving."),
		huh.NewInput().
			Title("Google address (optional)").
			Placeholder("you@gmail.com").
			Inline(true).
			Value(&s.email),
		huh.NewConfirm().
			Title("Include Google Calendar").
			Description("Mail is enabled. Adds a Google Calendar source with the same Google account.").
			Affirmative("Calendar on").
			Negative("Mail only").
			Value(&s.alsoAddCalendar),
	).Title("Google account")

	aiPanelSaveFields := func() []huh.Field {
		if s.mode != SettingsModePanel {
			return nil
		}
		return []huh.Field{
			huh.NewConfirm().
				Title("Save changes").
				Affirmative("Save").
				Negative("Cancel").
				Value(&s.saveButton),
		}
	}

	s.refreshAISetupPresetFromRoles()

	// Group 3 — Compact AI setup presets
	aiProviderGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("AI Setup").
			Description("Choose a preset, or use advanced manual config to assign chat and embeddings separately.").
			Options(aiSetupPresetOptions(s.aiSetupPreset)...).
			Value(&s.aiSetupPreset),
	).Title("AI Setup")

	ollamaDefaultFields := []huh.Field{
		huh.NewNote().
			Title("Ollama local default").
			Description("Default chat: gemma3:4b.\nDefault embeddings: nomic-embed-text-v2-moe.\nBest with 16GB RAM; 8GB works but slower.\nUse custom Ollama to downgrade.\nAvoid llama3.x for translation-heavy work."),
	}
	ollamaDefaultFields = append(ollamaDefaultFields, aiPanelSaveFields()...)
	ollamaDefaultGroup := huh.NewGroup(ollamaDefaultFields...).WithHideFunc(func() bool { return s.aiSetupPreset != aiSetupPresetOllamaDefault })

	// Group 3a — Ollama settings (shown only when provider = custom Ollama)
	ollamaFields := []huh.Field{
		huh.NewInput().Title("Ollama Host").Inline(true).Value(&s.ollamaHost).Placeholder(defaultOllamaHost),
		huh.NewNote().
			Title("Model recommendations").
			Description("Chat default: gemma3:4b.\nLower RAM: qwen3.5:0.8b or llama3.2:1b/3b.\nAvoid llama3.x for translation-heavy work.\nEmbedding default: nomic-embed-text-v2-moe.\nSmaller embeddings: nomic-embed-text, all-minilm.\nMore embeddings: mxbai-embed-large or bge-m3."),
		huh.NewSelect[string]().
			Title("Chat Model").
			Options(ollamaChatModelOptions()...).
			Value(&s.ollamaModelChoice),
		hideSettingsFieldWhen(
			huh.NewInput().Title("Custom Chat Model").Inline(true).Value(&s.ollamaModelCustom).Placeholder(defaultOllamaModel),
			func() bool { return s.ollamaModelChoice != customModelChoice },
		),
		huh.NewSelect[string]().
			Title("Embedding Model").
			Options(ollamaEmbeddingModelOptions()...).
			Value(&s.embedModelChoice),
		hideSettingsFieldWhen(
			huh.NewInput().Title("Custom Embedding Model").Inline(true).Value(&s.embedModelCustom).Placeholder(defaultEmbeddingModel),
			func() bool { return s.embedModelChoice != customModelChoice },
		),
	}
	ollamaFields = append(ollamaFields, aiPanelSaveFields()...)
	ollamaGroup := huh.NewGroup(ollamaFields...).WithHideFunc(func() bool {
		if s.advancedAISetup() {
			return configAIProvider(s.aiProvider) != "ollama"
		}
		return s.aiSetupPreset != aiSetupPresetOllamaCustom
	})

	ollamaWarningGroup := huh.NewGroup(
		huh.NewNote().
			Title("AI unavailable").
			Description(s.aiModelWarningDescription()),
		huh.NewConfirm().
			Title("Disable AI").
			Affirmative("Disable AI").
			Negative("Keep Ollama").
			Value(&s.disableAIFromWarning),
	).WithHideFunc(func() bool {
		return s.mode != SettingsModePanel || s.panelSection != settingsPanelSectionAI || s.aiModelWarning == nil || s.aiModelWarning.OK()
	})

	// Group 3b — Claude settings (shown only when provider = claude)
	claudeGroup := huh.NewGroup(
		huh.NewInput().Title("Claude API Key").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.claudeAPIKey).Validate(requiredSettingsValue("Claude API Key")),
		huh.NewInput().Title("Claude Model").Inline(true).Placeholder(defaultClaudeModel).Value(&s.claudeModel),
	).WithHideFunc(func() bool { return configAIProvider(s.aiProvider) != "claude" })

	// Group 3c — OpenAI settings (shown only when provider = openai)
	openAIGroup := huh.NewGroup(
		huh.NewInput().Title("OpenAI API Key").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.openAIAPIKey).Validate(requiredSettingsValue("OpenAI API Key")),
		huh.NewInput().Title("OpenAI Base URL").Inline(true).Placeholder(defaultOpenAIBaseURL).Value(&s.openAIBaseURL),
		huh.NewInput().Title("OpenAI Model").Inline(true).Placeholder(defaultChatModel).Value(&s.openAIModel),
		huh.NewSelect[string]().
			Title("Chat Reasoning Effort").
			Options(
				huh.NewOption("Low (fastest)", agent.ReasoningEffortLow),
				huh.NewOption("Medium", agent.ReasoningEffortMedium),
				huh.NewOption("High", agent.ReasoningEffortHigh),
				huh.NewOption("XHigh (slowest)", agent.ReasoningEffortXHigh),
			).
			Value(&s.agentReasoningEffort),
	).WithHideFunc(func() bool { return configAIProvider(s.aiProvider) != "openai" })

	externalEmbeddingFields := []huh.Field{
		huh.NewSelect[string]().
			Title("Embedding Role").
			Options(embeddingRoleOptions(s.embeddingProvider)...).
			Value(&s.embeddingProvider),
		hideSettingsFieldWhen(
			huh.NewInput().Title("Embedding API Key").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.openAIAPIKey).Validate(requiredSettingsValue("Embedding API Key")),
			func() bool {
				return s.aiProvider == "openai" || s.effectiveSettingsEmbeddingProvider() != config.EmbeddingProviderOpenAI
			},
		),
		hideSettingsFieldWhen(
			huh.NewInput().Title("Embedding Base URL").Inline(true).Placeholder(defaultOpenAIBaseURL).Value(&s.openAIBaseURL),
			func() bool {
				return s.aiProvider == "openai" || s.effectiveSettingsEmbeddingProvider() != config.EmbeddingProviderOpenAI
			},
		),
		hideSettingsFieldWhen(
			huh.NewInput().Title("OpenAI Embedding Model").Inline(true).Placeholder(defaultOpenAIEmbed).Value(&s.openAIEmbeddingModel),
			func() bool { return s.effectiveSettingsEmbeddingProvider() != config.EmbeddingProviderOpenAI },
		),
		hideSettingsFieldWhen(
			huh.NewInput().Title("Ollama Host").Inline(true).Value(&s.ollamaHost).Placeholder(defaultOllamaHost),
			func() bool { return s.effectiveSettingsEmbeddingProvider() != config.EmbeddingProviderOllama },
		),
		hideSettingsFieldWhen(
			huh.NewSelect[string]().
				Title("Ollama Embedding Model").
				Options(ollamaEmbeddingModelOptions()...).
				Value(&s.embedModelChoice),
			func() bool { return s.effectiveSettingsEmbeddingProvider() != config.EmbeddingProviderOllama },
		),
		hideSettingsFieldWhen(
			huh.NewInput().Title("Custom Ollama Embedding Model").Inline(true).Value(&s.embedModelCustom).Placeholder(defaultEmbeddingModel),
			func() bool {
				return s.effectiveSettingsEmbeddingProvider() != config.EmbeddingProviderOllama || s.embedModelChoice != customModelChoice
			},
		),
	}
	externalEmbeddingFields = append(externalEmbeddingFields, aiPanelSaveFields()...)
	externalEmbeddingGroup := huh.NewGroup(externalEmbeddingFields...).Title("Embedding Role").WithHideFunc(func() bool {
		return !s.advancedAISetup() && s.aiProvider != "claude" && s.aiProvider != "openai"
	})

	disabledAIFields := []huh.Field{
		huh.NewNote().
			Title("AI disabled").
			Description("Chat, classification, semantic search, quick replies, and Compose assistance stay off until AI is re-enabled."),
	}
	disabledAIFields = append(disabledAIFields, aiPanelSaveFields()...)
	disabledAIGroup := huh.NewGroup(disabledAIFields...).WithHideFunc(func() bool { return s.aiSetupPreset != aiSetupPresetDisabled })

	advancedAIGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Chat Role").
			Options(chatRoleOptions(s.aiProvider)...).
			Value(&s.aiProvider),
		huh.NewSelect[string]().
			Title("Embedding Role").
			Options(embeddingRoleOptions(s.embeddingProvider)...).
			Value(&s.embeddingProvider),
	).Title("Advanced AI").WithHideFunc(func() bool { return !s.advancedAISetup() })

	offlineCacheSelect := func() huh.Field {
		return huh.NewSelect[string]().
			Title("Offline Cache").
			Options(
				huh.NewOption("Lightweight previews", config.CacheStoragePolicyLightweight),
				huh.NewOption("Message bodies without attachments", config.CacheStoragePolicyNoAttachments),
				huh.NewOption("Full offline archive", config.CacheStoragePolicyPreserveAll),
			).
			Value(&s.cacheStoragePolicy)
	}

	wizardCacheGroup := huh.NewGroup(
		offlineCacheSelect(),
	).Title("Offline Cache")

	// Group 4 — Sync & Cleanup preferences
	syncGroup := huh.NewGroup(
		huh.NewInput().
			Title("Poll Interval (minutes)").
			Inline(true).
			Description("0 = use IMAP IDLE only").
			Placeholder("5").
			Value(&s.syncPollStr).
			Validate(func(v string) error {
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return errors.New("must be a non-negative integer")
				}
				return nil
			}),
		huh.NewConfirm().
			Title("Enable IMAP IDLE").
			Value(&s.syncIDLE),
		huh.NewSelect[string]().
			Title("Cleanup Tools").
			Options(
				huh.NewOption("Keep editing settings", settingsCleanupToolNone),
				huh.NewOption("Automation rules", settingsCleanupToolAutomation),
				huh.NewOption("Custom prompts", settingsCleanupToolPrompts),
				huh.NewOption("Cleanup rules", settingsCleanupToolRules),
			).
			Value(&s.cleanupToolSelection),
		offlineCacheSelect(),
		huh.NewConfirm().
			Title("Reclaim offline cache storage").
			Description("Estimate removable preview bytes before pruning; text, headers, and attachment metadata stay cached.").
			Affirmative("Reclaim").
			Negative("Skip").
			Value(&s.reclaimOfflineCacheStorage),
		huh.NewInput().
			Title("Auto-Cleanup Schedule (hours)").
			Inline(true).
			Description("0 = disabled").
			Placeholder("24").
			Value(&s.cleanupScheduleStr).
			Validate(func(v string) error {
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					return errors.New("must be a non-negative integer")
				}
				return nil
			}),
	).Title("Sync & Cleanup")

	if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionMemories {
		s.refreshMemoryStoreStats()
	}
	memorySetupGroup := huh.NewGroup(
		huh.NewNote().
			Title("Herald Memories").
			Description("Store local, source-backed relationship context for replies, contacts, chat, and optional Obsidian output."),
		hideSettingsFieldWhen(
			huh.NewNote().
				Title("Current status").
				DescriptionFunc(func() string {
					return s.memoryStatusDescription()
				}, []any{
					&s.memoryEnabled,
					&s.memoryDirectory,
					&s.memorySourceFolders,
					&s.memoryTaskChoices,
					&s.memoryVaultPath,
					&s.memoryObsidianEnabled,
					&s.memoryYAMLHeaders,
					&s.memoryLinkMode,
					&s.memoryTagMode,
					&s.memoryUpdateCadence,
					&s.memoryLowConfidenceDisposition,
					&s.memoryChatThresholdStr,
					&s.memoryDossierThresholdStr,
					&s.memoryObsidianThresholdStr,
					&s.memoryComposeThresholdStr,
					&s.memoryMatchThresholdStr,
					&s.memoryStaleAfterDaysStr,
					&s.memoryRetentionDaysStr,
				}),
			func() bool { return s.mode != SettingsModePanel },
		),
		huh.NewConfirm().
			Title("Herald Memories").
			Affirmative("On").
			Negative("Off").
			Value(&s.memoryEnabled),
		huh.NewInput().
			Title("Memory directory").
			Inline(true).
			Placeholder(memory.DefaultDirectory).
			Value(&s.memoryDirectory).
			Validate(requiredSettingsValue("Memory directory")),
		huh.NewInput().
			Title("Source folders").
			Inline(true).
			Placeholder("INBOX, Sent").
			Value(&s.memorySourceFolders).
			Validate(requiredSettingsValue("Source folders")),
		huh.NewConfirm().
			Title("Obsidian-friendly output").
			Affirmative("On").
			Negative("Off").
			Value(&s.memoryObsidianEnabled),
		hideSettingsFieldWhen(
			huh.NewInput().
				Title("Obsidian vault path").
				Inline(true).
				Placeholder("~/Documents/Obsidian").
				Value(&s.memoryVaultPath),
			func() bool { return !s.memoryObsidianEnabled },
		),
	).Title("Memory Setup")

	memoryTasksGroup := huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Memory tasks").
			DescriptionFunc(func() string {
				return settingsMemoryTaskSummary(s.memoryTaskChoices)
			}, []any{&s.memoryTaskChoices}).
			Options(settingsMemoryTaskOptions()...).
			Value(&s.memoryTaskChoices),
	).Title("Memory Tasks")

	memoryObsidianGroup := huh.NewGroup(
		huh.NewInput().
			Title("Obsidian vault path").
			Inline(true).
			Placeholder("~/Documents/Obsidian").
			Value(&s.memoryVaultPath),
		huh.NewConfirm().
			Title("Show YAML headers").
			Affirmative("Show").
			Negative("Hide").
			Value(&s.memoryYAMLHeaders),
		huh.NewSelect[string]().
			Title("Frontmatter mode").
			Options(settingsMemoryFrontmatterOptions()...).
			Value(&s.memoryFrontmatterMode),
		huh.NewSelect[string]().
			Title("Link mode").
			Options(settingsMemoryLinkModeOptions()...).
			Value(&s.memoryLinkMode),
		huh.NewSelect[string]().
			Title("Tag mode").
			Options(settingsMemoryTagModeOptions()...).
			Value(&s.memoryTagMode),
		huh.NewInput().
			Title("People destination").
			Inline(true).
			Placeholder("People").
			Value(&s.memoryPeopleDestination),
		huh.NewInput().
			Title("Companies destination").
			Inline(true).
			Placeholder("Job search/active").
			Value(&s.memoryCompaniesDestination),
		huh.NewInput().
			Title("Job search destination").
			Inline(true).
			Placeholder("Job search").
			Value(&s.memoryJobSearchDestination),
		huh.NewInput().
			Title("Projects destination").
			Inline(true).
			Placeholder("Projects").
			Value(&s.memoryProjectsDestination),
		huh.NewInput().
			Title("Threads destination").
			Inline(true).
			Placeholder("Threads").
			Value(&s.memoryThreadsDestination),
		huh.NewInput().
			Title("Research destination").
			Inline(true).
			Placeholder("Research").
			Value(&s.memoryResearchDestination),
		huh.NewInput().
			Title("Daily briefing destination").
			Inline(true).
			Placeholder("Scheduled Task Artifacts").
			Value(&s.memoryDailyDestination),
		huh.NewInput().
			Title("Inbox destination").
			Inline(true).
			Placeholder("Memory Inbox").
			Value(&s.memoryInboxDestination),
	).Title("Obsidian Output").WithHideFunc(func() bool {
		return !s.memoryObsidianEnabled
	})

	memoryExtractionGroup := huh.NewGroup(
		huh.NewNote().
			Title("Automatic extraction trigger").
			Description("Controls when Herald creates or updates memory records. Compose, chat, and dossiers may still read existing memories."),
		huh.NewSelect[string]().
			Title("Automatic extraction trigger").
			Options(settingsMemoryCadenceOptions()...).
			Value(&s.memoryUpdateCadence),
	).Title("Extraction Trigger")

	memoryPromptGroup := huh.NewGroup(
		huh.NewNote().
			Title("Prompt template inventory").
			DescriptionFunc(func() string {
				return s.memoryPromptTemplateSummary()
			}, []any{&s.memoryPromptTemplateChoice}),
	).Title("Prompt Templates")

	memoryAdvancedGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Low-confidence memories").
			Options(settingsMemoryLowConfidenceOptions()...).
			Value(&s.memoryLowConfidenceDisposition),
		huh.NewInput().
			Title("Chat retrieval threshold").
			Inline(true).
			Placeholder("0.35").
			Value(&s.memoryChatThresholdStr).
			Validate(settingsMemoryThresholdValidator("Chat retrieval threshold")),
		huh.NewInput().
			Title("Dossier threshold").
			Inline(true).
			Placeholder("0.55").
			Value(&s.memoryDossierThresholdStr).
			Validate(settingsMemoryThresholdValidator("Dossier threshold")),
		huh.NewInput().
			Title("Obsidian write threshold").
			Inline(true).
			Placeholder("0.70").
			Value(&s.memoryObsidianThresholdStr).
			Validate(settingsMemoryThresholdValidator("Obsidian write threshold")),
		huh.NewInput().
			Title("Compose Radar threshold").
			Inline(true).
			Placeholder("0.75").
			Value(&s.memoryComposeThresholdStr).
			Validate(settingsMemoryThresholdValidator("Compose Radar threshold")),
		huh.NewInput().
			Title("Match threshold").
			Inline(true).
			Placeholder("0.80").
			Value(&s.memoryMatchThresholdStr).
			Validate(settingsMemoryThresholdValidator("Match threshold")),
		huh.NewInput().
			Title("Stale after days").
			Inline(true).
			Placeholder("45").
			Value(&s.memoryStaleAfterDaysStr).
			Validate(settingsPositiveIntValidator("Stale after days")),
		huh.NewInput().
			Title("Retention days").
			Inline(true).
			Placeholder("0").
			Value(&s.memoryRetentionDaysStr).
			Validate(settingsNonNegativeIntValidator("Retention days")),
	).Title("Advanced Rules")

	calendarGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Week starts on").
			Options(
				huh.NewOption("Monday", config.CalendarWeekStartMonday),
				huh.NewOption("Sunday", config.CalendarWeekStartSunday),
			).
			Value(&s.calendarWeekStart),
	).Title("Calendar")

	composeGroup := huh.NewGroup(
		huh.NewText().
			Title("Email Signature").
			Description("Enter=newline. Tab=Save. Empty disables. Bare -- gets space.").
			Placeholder("-- \nYour Name").
			Lines(5).
			Value(&s.signatureText),
		huh.NewConfirm().
			Title("Save Settings").
			Affirmative("Save Settings").
			Negative("").
			Value(&s.saveButton),
	).Title("Compose")

	keyboardGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Keyboard Profile").
			Options(
				huh.NewOption("Default", keyboardProfileDefault),
				huh.NewOption("Vim", keyboardProfileVim),
				huh.NewOption("Emacs", keyboardProfileEmacs),
				huh.NewOption("Custom YAML", keyboardProfileCustom),
			).
			Value(&s.keyboardProfile),
		hideSettingsFieldWhen(
			huh.NewInput().
				Title("Custom Keymap").
				Inline(true).
				Placeholder("~/.config/herald/keymaps/work.yaml").
				Value(&s.customKeymap),
			func() bool { return s.keyboardProfile != keyboardProfileCustom },
		),
	).Title("Keyboard")

	wizardThemeGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Current Theme").
			Options(settingsThemeOptions()...).
			Value(&s.themeName),
	).Title("Theme")

	themeSelectionGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Current Theme").
			Options(settingsThemeOptions()...).
			Value(&s.themeName),
		huh.NewInput().
			Title("Install local theme YAML").
			Description("Validated and copied into ~/.herald/themes. Leave blank to skip install.").
			Placeholder("~/Downloads/quiet-slate.yaml").
			Value(&s.themeInstallPath),
		huh.NewConfirm().
			Title("Save changes").
			Affirmative("Save").
			Negative("Cancel").
			Value(&s.saveButton),
	).Title("Theme Selection")

	themeEditorGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Theme Role").
			Options(settingsThemeRoleOptions()...).
			Value(&s.themeRole),
		huh.NewInput().
			Title("Foreground").
			Description("Use inherit, ansi:N, xterm:N, or #RRGGBB. Picker updates this value instantly.").
			Placeholder("inherit").
			Key(settingsThemeForegroundKey).
			Value(&s.themeFG),
		newThemeColorPickerField("Foreground Picker", &s.themeFG).Key(settingsThemeForegroundPickerKey),
		huh.NewInput().
			Title("Background").
			Description("Use inherit, ansi:N, xterm:N, or #RRGGBB. Picker updates this value instantly.").
			Placeholder("xterm:25").
			Key(settingsThemeBackgroundKey).
			Value(&s.themeBG),
		newThemeColorPickerField("Background Picker", &s.themeBG).Key(settingsThemeBackgroundPickerKey),
		huh.NewNote().
			Title("Live preview").
			DescriptionFunc(func() string {
				return s.themePreviewDescription()
			}, []any{&s.themeName, &s.themeRole, &s.themeFG, &s.themeBG}),
		huh.NewConfirm().
			Title("Reset selected role").
			Affirmative("Reset Role").
			Negative("Keep").
			Value(&s.themeResetRole),
		huh.NewConfirm().
			Title("Reset all theme overrides").
			Affirmative("Reset All").
			Negative("Keep").
			Value(&s.themeResetAll),
		huh.NewInput().
			Title("Save As New Theme").
			Description("Optional slug. Uses current overrides and saves a local theme file.").
			Placeholder("quiet-slate").
			Value(&s.themeSaveAs),
		huh.NewConfirm().
			Title("Save changes").
			Affirmative("Save").
			Negative("Cancel").
			Value(&s.saveButton),
	).Title("Theme Editor")

	panelSignatureGroup := huh.NewGroup(
		huh.NewText().
			Title("Email Signature").
			Description("Enter=newline. Tab=Save. Empty disables. Bare -- gets space.").
			Placeholder("-- \nYour Name").
			Lines(5).
			Value(&s.signatureText),
		huh.NewConfirm().
			Title("Save changes").
			Affirmative("Save").
			Negative("Cancel").
			Value(&s.saveButton),
	).Title("Signature")

	inlineSaveField := func(title, affirmative string) huh.Field {
		return huh.NewConfirm().
			Title(title).
			Affirmative(affirmative).
			Negative("Cancel").
			Value(&s.saveButton)
	}

	alsoCalendarGroup := huh.NewGroup(
		huh.NewConfirm().
			Title("Also add calendar").
			Description("Adds a paired calendar source for supported providers. Calendar details stay editable before save.").
			Affirmative("Add calendar").
			Negative("Mail only").
			Value(&s.alsoAddCalendar),
	).WithHideFunc(func() bool {
		return !s.accountDetailShowsMail() || s.accountDetailHasCalendar() || !calendarPairingSupportedProvider(s.provider) || s.provider == "gmail-oauth"
	})

	calendarProviderGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Calendar Provider").
			Options(s.calendarProviderOptions()...).
			Value(&s.calendarProvider),
	).WithHideFunc(func() bool {
		return !s.accountDetailShowsCalendar() || (s.accountDetailShowsMail() && s.provider == "gmail-oauth")
	})

	googleCalendarGroup := huh.NewGroup(
		huh.NewInput().
			Title("Google Calendar identity").
			Inline(true).
			Placeholder("you@gmail.com").
			Value(&s.calendarEmail),
	).WithHideFunc(func() bool {
		return !s.accountDetailShowsCalendar() || s.calendarProvider != "google_calendar" || (s.accountDetailShowsMail() && s.provider == "gmail-oauth")
	})

	caldavGroup := huh.NewGroup(
		huh.NewInput().
			Title("CalDAV URL").
			Inline(true).
			PlaceholderFunc(func() string {
				return calendarProviderURLPlaceholder(s.effectiveCalendarProvider())
			}, []any{&s.calendarProvider, &s.provider, &s.alsoAddCalendar}).
			Value(&s.caldavURL),
		huh.NewInput().
			Title("CalDAV Username").
			Inline(true).
			PlaceholderFunc(func() string {
				return calendarProviderUsernamePlaceholder(s.effectiveCalendarProvider())
			}, []any{&s.calendarProvider, &s.provider, &s.alsoAddCalendar}).
			Value(&s.caldavUsername),
		huh.NewInput().
			Title("CalDAV Password").
			Inline(true).
			PlaceholderFunc(func() string {
				return calendarProviderPasswordPlaceholder(s.effectiveCalendarProvider())
			}, []any{&s.calendarProvider, &s.provider, &s.alsoAddCalendar}).
			EchoMode(huh.EchoModePassword).
			Value(&s.caldavPassword),
	).WithHideFunc(func() bool {
		return !s.accountDetailShowsCalendar() || !calendarProviderUsesCalDAV(s.effectiveCalendarProvider())
	})

	saveGroup := huh.NewGroup(
		huh.NewConfirm().
			Title("Save changes").
			Affirmative("Save").
			Negative("Cancel").
			Value(&s.saveButton),
	)
	connectGroup := huh.NewGroup(
		huh.NewConfirm().
			Title("Verify access").
			Affirmative("Connect").
			Negative("Cancel").
			Value(&s.saveButton),
	).Title("Connect mail account")

	accountDeleteConfirmGroup := huh.NewGroup(
		huh.NewNote().
			TitleFunc(func() string {
				name := strings.TrimSpace(s.accountDisplayName)
				if name == "" {
					name = s.selectedAccountID
				}
				if name == "" {
					name = "selected account"
				}
				return "Disconnect " + name + "?"
			}, &s.accountDisplayName).
			Description("This removes the account from Herald and deletes its local cached mail/calendar data. Provider mail and calendars are not deleted."),
		huh.NewConfirm().
			Title("Disconnect account").
			Affirmative("Disconnect").
			Negative("Cancel").
			Value(&s.deleteAccount),
	).Title("Disconnect account")

	existingAccountEditGroup := huh.NewGroup(
		huh.NewNote().
			TitleFunc(func() string {
				name := strings.TrimSpace(s.accountDisplayName)
				if name == "" {
					name = "Account"
				}
				return name
			}, &s.accountDisplayName).
			DescriptionFunc(func() string {
				capability := s.accountDetailCapability()
				if capability == "" {
					capability = "Account"
				}
				return capability + ". Edit the label and credentials, then save."
			}, []any{&s.provider, &s.calendarProvider, &s.alsoAddCalendar}),
		huh.NewInput().
			Title("Display Name").
			Inline(true).
			PlaceholderFunc(func() string {
				if s.accountDetailShowsCalendar() && !s.accountDetailShowsMail() {
					return "Family Calendar"
				}
				return "Work Gmail"
			}, []any{&s.provider, &s.calendarProvider, &s.alsoAddCalendar}).
			Value(&s.accountDisplayName),
		hideSettingsFieldWhen(
			huh.NewInput().
				TitleFunc(func() string {
					if s.provider == "gmail-oauth" {
						return "Gmail address"
					}
					return "Email address"
				}, &s.provider).
				Inline(true).
				Value(&s.email).
				Validate(s.validateSetupEmail),
			func() bool { return !s.accountDetailShowsMail() },
		),
		hideSettingsFieldWhen(
			huh.NewConfirm().
				Title("Include Google Calendar").
				Description("Keeps a paired Google Calendar source on this account. Turning it off removes only Herald's calendar source.").
				Affirmative("Mail + Calendar").
				Negative("Mail only").
				Value(&s.alsoAddCalendar),
			func() bool {
				return !s.existingAccountCanToggleCalendarPairing()
			},
		),
		hideSettingsFieldWhen(
			huh.NewInput().
				TitleFunc(func() string {
					if s.provider == "gmail" {
						return "App Password"
					}
					return "Password"
				}, &s.provider).
				Inline(true).
				EchoMode(huh.EchoModePassword).
				Value(&s.password),
			func() bool { return !s.accountDetailShowsMail() || s.provider == "gmail-oauth" },
		),
		hideSettingsFieldWhen(
			huh.NewInput().
				Title("Google Calendar identity").
				Inline(true).
				Value(&s.calendarEmail),
			func() bool {
				return !s.accountDetailShowsCalendar() || s.accountDetailShowsMail() || calendarProviderUsesCalDAV(s.effectiveCalendarProvider())
			},
		),
		hideSettingsFieldWhen(
			huh.NewInput().
				Title("Calendar Username").
				Inline(true).
				Value(&s.caldavUsername),
			func() bool {
				return !s.accountDetailShowsCalendar() || s.accountDetailShowsMail() || !calendarProviderUsesCalDAV(s.effectiveCalendarProvider())
			},
		),
		hideSettingsFieldWhen(
			huh.NewInput().
				Title("Calendar Password").
				Inline(true).
				EchoMode(huh.EchoModePassword).
				Value(&s.caldavPassword),
			func() bool {
				return !s.accountDetailShowsCalendar() || s.accountDetailShowsMail() || !calendarProviderUsesCalDAV(s.effectiveCalendarProvider())
			},
		),
		inlineSaveField("Save changes", "Save"),
	).Title("Account")

	gmailOAuthAddGroup := huh.NewGroup(
		huh.NewNote().
			Title("Gmail OAuth").
			Description("Recommended browser-based Gmail setup. Herald validates Gmail and Google Calendar before saving."),
		huh.NewInput().Title("Gmail address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
		huh.NewConfirm().
			Title("Include Google Calendar").
			Description("Adds Google Calendar with the same Google account and OAuth flow.").
			Affirmative("Add calendar").
			Negative("Mail only").
			Value(&s.alsoAddCalendar),
		inlineSaveField("Connect account", "Connect"),
	).WithHideFunc(func() bool {
		return s.mailSettingsHidden() || s.provider != "gmail-oauth" || s.mode != SettingsModePanel || s.accountEditMode != settingsAccountEditAddMail
	})

	credentialsAddGroup := huh.NewGroup(
		credentialsIntro,
		huh.NewInput().Title("Email address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
		huh.NewInput().Title("Password").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.password),
		huh.NewInput().Title("IMAP Host").Inline(true).Value(&s.imapHost).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "imap-host") }, &s.provider),
		huh.NewInput().Title("IMAP Port").Inline(true).Value(&s.imapPort).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "imap-port") }, &s.provider),
		huh.NewInput().Title("SMTP Host").Inline(true).Value(&s.smtpHost).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "smtp-host") }, &s.provider),
		huh.NewInput().Title("SMTP Port").Inline(true).Value(&s.smtpPort).
			PlaceholderFunc(func() string { return providerPresetPlaceholder(s.provider, "smtp-port") }, &s.provider),
		inlineSaveField("Connect account", "Connect"),
	).WithHideFunc(func() bool {
		return s.mailSettingsHidden() || s.accountEditMode != settingsAccountEditAddMail || s.provider == "gmail" || s.provider == "gmail-oauth"
	})

	gmailAddGroup := huh.NewGroup(
		huh.NewNote().
			Title("Gmail via IMAP App Password").
			Description("Fallback Gmail setup. Use your Gmail address and a Google App Password when OAuth is unavailable for your account."),
		huh.NewInput().Title("Gmail address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
		huh.NewInput().Title("App Password").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.password),
		huh.NewConfirm().
			Title("Edit advanced Gmail server settings").
			Value(&s.editGmailAdvanced),
		inlineSaveField("Connect account", "Connect"),
	).WithHideFunc(func() bool {
		return s.mailSettingsHidden() || s.accountEditMode != settingsAccountEditAddMail || s.provider != "gmail"
	})

	googleCalendarAddOnlyGroup := huh.NewGroup(
		huh.NewInput().
			Title("Google Calendar identity").
			Inline(true).
			Placeholder("you@gmail.com").
			Value(&s.calendarEmail),
		inlineSaveField("Connect calendar", "Connect"),
	).WithHideFunc(func() bool {
		return s.accountEditMode != settingsAccountEditAddCalendar || s.calendarProvider != "google_calendar"
	})

	caldavAddOnlyGroup := huh.NewGroup(
		huh.NewInput().
			Title("CalDAV URL").
			Inline(true).
			PlaceholderFunc(func() string {
				return calendarProviderURLPlaceholder(s.effectiveCalendarProvider())
			}, []any{&s.calendarProvider, &s.provider, &s.alsoAddCalendar}).
			Value(&s.caldavURL),
		huh.NewInput().
			Title("CalDAV Username").
			Inline(true).
			PlaceholderFunc(func() string {
				return calendarProviderUsernamePlaceholder(s.effectiveCalendarProvider())
			}, []any{&s.calendarProvider, &s.provider, &s.alsoAddCalendar}).
			Value(&s.caldavUsername),
		huh.NewInput().
			Title("CalDAV Password").
			Inline(true).
			PlaceholderFunc(func() string {
				return calendarProviderPasswordPlaceholder(s.effectiveCalendarProvider())
			}, []any{&s.calendarProvider, &s.provider, &s.alsoAddCalendar}).
			EchoMode(huh.EchoModePassword).
			Value(&s.caldavPassword),
		inlineSaveField("Connect calendar", "Connect"),
	).WithHideFunc(func() bool {
		return s.accountEditMode != settingsAccountEditAddCalendar || !calendarProviderUsesCalDAV(s.effectiveCalendarProvider())
	})

	groups := []*huh.Group{
		accountGroup,
		credentialsGroup,
		gmailGroup,
		gmailAdvancedGroup,
		gmailOAuthGroup,
		aiProviderGroup,
		advancedAIGroup,
		ollamaDefaultGroup,
		ollamaGroup,
		claudeGroup,
		openAIGroup,
		externalEmbeddingGroup,
		disabledAIGroup,
		wizardCacheGroup,
		keyboardGroup,
		wizardThemeGroup,
		composeGroup,
	}

	if s.mode == SettingsModePanel {
		switch s.panelSection {
		case settingsPanelSectionAccount:
			if s.accountDeletePending {
				groups = []*huh.Group{accountDeleteConfirmGroup}
			} else if s.accountEditMode == settingsAccountEditAddMail {
				groups = []*huh.Group{
					accountGroup,
					credentialsAddGroup,
					gmailAddGroup,
					gmailAdvancedGroup,
					gmailOAuthAddGroup,
					alsoCalendarGroup,
					calendarProviderGroup,
					googleCalendarGroup,
					caldavGroup,
				}
			} else if s.accountEditMode == settingsAccountEditAddCalendar {
				groups = []*huh.Group{
					calendarProviderGroup,
					googleCalendarAddOnlyGroup,
					caldavAddOnlyGroup,
				}
			} else {
				groups = []*huh.Group{
					existingAccountEditGroup,
				}
			}
		case settingsPanelSectionAI:
			groups = []*huh.Group{
				aiProviderGroup,
				advancedAIGroup,
				ollamaDefaultGroup,
				ollamaGroup,
				claudeGroup,
				openAIGroup,
				externalEmbeddingGroup,
				disabledAIGroup,
			}
			if s.hasAIModelWarning() {
				groups = append([]*huh.Group{ollamaWarningGroup}, groups...)
			}
		case settingsPanelSectionSync:
			groups = []*huh.Group{
				syncGroup,
				saveGroup.Title("Sync & Cleanup"),
			}
		case settingsPanelSectionMemories:
			groups = []*huh.Group{
				memorySetupGroup,
				memoryTasksGroup,
				memoryObsidianGroup,
				memoryExtractionGroup,
				memoryPromptGroup,
				memoryAdvancedGroup,
				saveGroup.Title("Memories"),
			}
		case settingsPanelSectionCalendar:
			groups = []*huh.Group{
				calendarGroup,
				saveGroup.Title("Calendar"),
			}
		case settingsPanelSectionKeyboard:
			groups = []*huh.Group{
				keyboardGroup,
				saveGroup.Title("Keyboard"),
			}
		case settingsPanelSectionThemeSelection:
			groups = []*huh.Group{themeSelectionGroup}
		case settingsPanelSectionThemeEditor:
			groups = []*huh.Group{themeEditorGroup}
		case settingsPanelSectionSign:
			groups = []*huh.Group{panelSignatureGroup}
		}
	}
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		if s.provider == "gmail-oauth" && !s.firstRunOtherProvider {
			groups = []*huh.Group{
				accountGroup,
				firstRunGoogleGroup,
			}
		} else {
			s.firstRunOtherProvider = true
			groups = []*huh.Group{
				accountGroup,
				credentialsGroup,
				gmailGroup,
				gmailAdvancedGroup,
				connectGroup,
			}
		}
	} else if s.mode == SettingsModeWizard && s.firstRunPreferencesOnly {
		enterGroup := huh.NewGroup(
			huh.NewNote().
				Title("Advanced settings").
				Description("Memories: On with local defaults\nTheme: Inherited\nAI: Disabled\nKeyboard: Default\nOffline Cache: Message bodies without attachments\nSignature: None"),
			huh.NewSelect[string]().
				Title("Next").
				Options(
					huh.NewOption("Enter Herald", firstRunPreferenceEnter),
					huh.NewOption("Customize setup", firstRunPreferenceCustomize),
				).
				Value(&s.firstRunPreferenceAction),
		).Title("Advanced settings")
		if !s.firstRunCustomizePreferences {
			groups = []*huh.Group{enterGroup}
		} else {
			groups = []*huh.Group{
				memorySetupGroup,
				memoryTasksGroup,
				aiProviderGroup,
				advancedAIGroup,
				ollamaDefaultGroup,
				ollamaGroup,
				claudeGroup,
				openAIGroup,
				externalEmbeddingGroup,
				disabledAIGroup,
				wizardCacheGroup,
				keyboardGroup,
				wizardThemeGroup,
				composeGroup,
			}
		}
	}

	s.form = huh.NewForm(groups...).
		WithTheme(huh.ThemeFunc(heraldHuhTheme)).
		WithShowHelp(true).
		WithShowErrors(true).
		WithKeyMap(settingsFormKeyMap())

	if s.width > 0 {
		s.form = s.form.WithWidth(s.formWidth())
	}
	if s.height > 0 {
		s.form = s.form.WithHeight(s.formHeight())
	}
	s.prepareFormForView()
}

func (s *Settings) buildPanelMenuForm() {
	fields := []huh.Field{}
	description := "Choose one area to edit. Save returns here; Esc closes without saving."
	if status := strings.TrimSpace(s.panelStatus); status != "" {
		description = status + "\n" + description
	}
	fields = append(fields,
		huh.NewSelect[string]().
			Title("Settings").
			Description(description).
			Options(
				huh.NewOption("Accounts", string(settingsPanelSectionAccounts)),
				huh.NewOption("AI", string(settingsPanelSectionAI)),
				huh.NewOption("Sync & Cleanup", string(settingsPanelSectionSync)),
				huh.NewOption("Memories", string(settingsPanelSectionMemories)),
				huh.NewOption("Calendar", string(settingsPanelSectionCalendar)),
				huh.NewOption("Keyboard", string(settingsPanelSectionKeyboard)),
				huh.NewOption("Theme Selection", string(settingsPanelSectionThemeSelection)),
				huh.NewOption("Theme Editor", string(settingsPanelSectionThemeEditor)),
				huh.NewOption("Signature", string(settingsPanelSectionSign)),
			).
			Value(&s.panelMenuChoice),
	)
	s.form = huh.NewForm(huh.NewGroup(fields...).Title("Settings")).
		WithTheme(huh.ThemeFunc(heraldHuhTheme)).
		WithShowHelp(true).
		WithShowErrors(true).
		WithKeyMap(settingsPanelMenuKeyMap())

	if s.width > 0 {
		s.form = s.form.WithWidth(s.formWidth())
	}
	if s.height > 0 {
		s.form = s.form.WithHeight(s.formHeight())
	}
	s.prepareFormForView()
}

func (s *Settings) buildAccountsListForm() {
	options := s.accountListOptions()
	if !accountListHasOption(options, s.accountsMenuChoice) && len(options) > 0 {
		s.accountsMenuChoice = settingsAccountEditAddMail
		if !accountListHasOption(options, s.accountsMenuChoice) {
			s.accountsMenuChoice = options[0].Value
		}
	}
	description := "Choose an account or source to edit. Disconnecting removes Herald settings only."
	if status := strings.TrimSpace(s.panelStatus); status != "" {
		description = status + "\n" + description
	}
	s.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Accounts").
			Description(description).
			Options(options...).
			Value(&s.accountsMenuChoice),
	).Title("Accounts")).
		WithTheme(huh.ThemeFunc(heraldHuhTheme)).
		WithShowHelp(true).
		WithShowErrors(true).
		WithKeyMap(settingsPanelMenuKeyMap())

	if s.width > 0 {
		s.form = s.form.WithWidth(s.formWidth())
	}
	if s.height > 0 {
		s.form = s.form.WithHeight(s.formHeight())
	}
	s.prepareFormForView()
}

func accountListHasOption(options []huh.Option[string], value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func accountGroupProviderLabel(group config.AccountGroup) string {
	seen := make(map[string]bool)
	var labels []string
	add := func(label string) {
		label = strings.TrimSpace(label)
		if label == "" || seen[label] {
			return
		}
		seen[label] = true
		labels = append(labels, label)
	}
	for _, source := range group.Sources {
		kind := strings.TrimSpace(source.Kind)
		provider := strings.TrimSpace(source.Provider)
		switch kind {
		case "", string(models.SourceKindMail):
			if settingsMailSourceUsesGoogleOAuth(source) {
				add("Gmail OAuth")
				continue
			}
			switch provider {
			case "gmail":
				add("Gmail IMAP")
			case "gmail_api":
				add("Gmail API")
			case "imap":
				add("IMAP")
			default:
				add(provider)
			}
		case string(models.SourceKindCalendar):
			switch provider {
			case "google_calendar":
				add("Google Calendar")
			case "caldav":
				add("CalDAV")
			default:
				add(provider)
			}
		default:
			add(provider)
		}
	}
	if len(labels) == 0 {
		return strings.TrimSpace(group.Provider)
	}
	return strings.Join(labels, " + ")
}

func (s *Settings) accountListOptions() []huh.Option[string] {
	var options []huh.Option[string]
	if s.cfg != nil {
		for _, group := range s.cfg.AccountGroups() {
			label := strings.TrimSpace(group.DisplayName)
			if label == "" {
				label = group.AccountID
			}
			meta := []string{group.Capability}
			if address := strings.TrimSpace(group.Address); address != "" {
				meta = append(meta, address)
			}
			if provider := accountGroupProviderLabel(group); provider != "" {
				meta = append(meta, provider)
			}
			options = append(options, huh.NewOption(label+" · "+strings.Join(meta, " · "), "account:"+group.AccountID))
		}
	}
	options = append(options,
		huh.NewOption("Add account", settingsAccountEditAddMail),
		huh.NewOption("Add calendar only", settingsAccountEditAddCalendar),
	)
	return options
}

func (s *Settings) setSize(width, height int) {
	s.width = width
	s.height = height
	if s.form != nil {
		s.form = s.form.WithWidth(s.formWidth()).WithHeight(s.formHeight())
		s.prepareFormForView()
	}
}

func (s *Settings) prepareFormForView() {
	if s.form != nil {
		_ = s.form.Init()
	}
}

func settingsFormKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Select.SetFilter = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "exit filter"), key.WithDisabled())
	keymap.MultiSelect.SetFilter = key.NewBinding(key.WithKeys("enter", "esc"), key.WithHelp("esc", "exit filter"), key.WithDisabled())
	keymap.Text.NewLine = key.NewBinding(key.WithKeys("enter", "alt+enter", "ctrl+j"), key.WithHelp("enter", "new line"))
	keymap.Text.Next = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next"))
	keymap.Text.Submit = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save"))
	return keymap
}

func settingsPanelMenuKeyMap() *huh.KeyMap {
	keymap := settingsFormKeyMap()
	keymap.Select.Submit = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open • esc exit"))
	return keymap
}

func settingsThemeOptions() []huh.Option[string] {
	names := ThemeDisplayNames(DefaultThemeDir())
	seen := make(map[string]bool, len(names))
	options := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		options = append(options, huh.NewOption(themeDisplayName(name), name))
	}
	return options
}

func themeDisplayName(name string) string {
	switch name {
	case "inherited":
		return "Inherited"
	case "herald-dark":
		return "Herald dark"
	case "herald-light":
		return "Herald light"
	default:
		parts := strings.Split(name, "-")
		for i, part := range parts {
			if part == "" {
				continue
			}
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
		return strings.Join(parts, " ")
	}
}

func settingsThemeRoleOptions() []huh.Option[string] {
	roles := themeRoleIDs()
	options := make([]huh.Option[string], 0, len(roles))
	for _, role := range roles {
		options = append(options, huh.NewOption(role, role))
	}
	return options
}

func themeRoleIDs() []string {
	roleMap := themeRoleMap(&defaultTheme)
	roles := make([]string, 0, len(roleMap))
	for role := range roleMap {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}

func firstThemeRole(overrides map[string]config.ThemeOverride) string {
	for _, role := range themeRoleIDs() {
		if _, ok := overrides[role]; ok {
			return role
		}
	}
	return "chrome.tab_active"
}

func (s *Settings) storeThemeFieldsForRole(role, fg, bg string) {
	storeThemeFieldsInMap(s.themeOverrides, role, fg, bg)
}

func storeThemeFieldsInMap(overrides map[string]config.ThemeOverride, role, fg, bg string) {
	if overrides == nil || strings.TrimSpace(role) == "" {
		return
	}
	fg = strings.TrimSpace(fg)
	bg = strings.TrimSpace(bg)
	if fg == "" && bg == "" {
		return
	}
	override := overrides[role]
	if fg != "" {
		override.Foreground = fg
	}
	if bg != "" {
		override.Background = bg
	}
	overrides[role] = override
}

func (s *Settings) loadThemeFieldsForRole(role string) {
	s.themeFG = ""
	s.themeBG = ""
	if s.themeOverrides == nil {
		s.themeOverrides = make(map[string]config.ThemeOverride)
	}
	if override, ok := s.themeOverrides[role]; ok {
		s.themeFG = override.Foreground
		s.themeBG = override.Background
	}
}

func (s *Settings) syncThemeRoleFields(prevRole, prevFG, prevBG string) {
	if s.mode == SettingsModePanel && !isThemeEditorSection(s.panelSection) {
		return
	}
	if prevRole != s.themeRole {
		s.storeThemeFieldsForRole(prevRole, prevFG, prevBG)
		s.loadThemeFieldsForRole(s.themeRole)
		return
	}
	s.storeThemeFieldsForRole(s.themeRole, s.themeFG, s.themeBG)
}

func (s *Settings) themePreviewDescription() string {
	fg := strings.TrimSpace(s.themeFG)
	bg := strings.TrimSpace(s.themeBG)
	if fg == "" {
		fg = "inherit"
	}
	if bg == "" {
		bg = "inherit"
	}
	cfg := s.buildConfig()
	theme, warning := ResolveThemeForConfig(cfg, "")
	if warning != "" {
		theme = ThemeByName(s.themeName)
	}
	if role, ok := themeRoleMap(&theme)[s.themeRole]; ok {
		preview := *role
		_ = applyThemeOverride(&preview, config.ThemeOverride{Foreground: fg, Background: bg})
		sample := preview.Style().Render(" Sample ")
		return fmt.Sprintf("%s\n%s  %s\nxterm-256 grid: arrows/hjkl. RGB: m then arrows. i inherits.\n%s", s.themeRole, themeColorSwatch("fg", fg), themeColorSwatch("bg", bg), sample)
	}
	return fmt.Sprintf("Role %s | %s | %s | xterm-256 grid and RGB picker values update instantly.", s.themeRole, themeColorSwatch("fg", fg), themeColorSwatch("bg", bg))
}

func themeColorSwatch(label, value string) string {
	color, err := parseThemeColor(value)
	if err != nil {
		return fmt.Sprintf("%s invalid:%s", label, value)
	}
	if color == nil {
		return fmt.Sprintf("%s inherit", label)
	}
	block := lipgloss.NewStyle().Background(color).Render("  ")
	return fmt.Sprintf("%s %s %s", label, block, value)
}

func (s *Settings) focusedFieldHandlesKey(msg tea.KeyPressMsg) bool {
	if s == nil || s.form == nil {
		return false
	}
	field := s.form.GetFocusedField()
	for _, binding := range field.KeyBinds() {
		if key.Matches(msg, binding) {
			return true
		}
	}
	return false
}

func (s *Settings) focusedFieldEscHelp() string {
	if s == nil || s.form == nil {
		return ""
	}
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	for _, binding := range s.form.GetFocusedField().KeyBinds() {
		if key.Matches(msg, binding) {
			return binding.Help().Desc
		}
	}
	return ""
}

func (s *Settings) shouldOpenThemePickerFromManualInput(msg tea.KeyPressMsg) bool {
	if s == nil || s.form == nil || (msg.Text != "/" && msg.Code != '/') {
		return false
	}
	if s.mode == SettingsModePanel && !isThemeEditorSection(s.panelSection) {
		return false
	}
	switch s.form.GetFocusedField().GetKey() {
	case settingsThemeForegroundKey, settingsThemeBackgroundKey:
		return true
	default:
		return false
	}
}

func (s *Settings) mailSettingsHidden() bool {
	return s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAccount && !s.accountDetailShowsMail()
}

func (s *Settings) accountDetailShowsMail() bool {
	if s.mode != SettingsModePanel || s.panelSection != settingsPanelSectionAccount {
		return true
	}
	switch s.accountEditMode {
	case settingsAccountEditAddCalendar:
		return false
	case settingsAccountEditAddMail:
		return true
	default:
		group, ok := s.selectedAccountGroup()
		return !ok || group.MailSourceID != ""
	}
}

func (s *Settings) accountDetailShowsCalendar() bool {
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		return !s.firstRunOtherProvider && s.alsoAddCalendar
	}
	if s.mode != SettingsModePanel || s.panelSection != settingsPanelSectionAccount {
		return false
	}
	switch s.accountEditMode {
	case settingsAccountEditAddCalendar:
		return true
	case settingsAccountEditAddMail:
		return s.alsoAddCalendar && calendarPairingSupportedProvider(s.provider)
	default:
		group, ok := s.selectedAccountGroup()
		if !ok {
			return false
		}
		if group.MailSourceID == "" {
			return len(group.CalendarSourceIDs) > 0
		}
		if s.existingAccountCanToggleCalendarPairing() {
			return s.alsoAddCalendar
		}
		return len(group.CalendarSourceIDs) > 0
	}
}

func (s *Settings) accountDetailHasCalendar() bool {
	group, ok := s.selectedAccountGroup()
	return ok && len(group.CalendarSourceIDs) > 0
}

func (s *Settings) googleCalendarSetupAvailable() bool {
	return s != nil
}

func (s *Settings) editingExistingGoogleCalendar() bool {
	if s == nil || s.accountEditMode != settingsAccountEditExisting {
		return false
	}
	group, ok := s.selectedAccountGroup()
	if !ok {
		return false
	}
	for _, source := range group.Sources {
		if strings.TrimSpace(source.Kind) == string(models.SourceKindCalendar) &&
			strings.TrimSpace(source.Provider) == "google_calendar" {
			return true
		}
	}
	return false
}

func (s *Settings) showGoogleCalendarProviderOption() bool {
	return s.googleCalendarSetupAvailable() || s.editingExistingGoogleCalendar()
}

func (s *Settings) defaultCalendarProvider() string {
	if s.showGoogleCalendarProviderOption() {
		return "google_calendar"
	}
	return "caldav"
}

func (s *Settings) normalizeCalendarProviderChoice() {
	if s == nil {
		return
	}
	if strings.TrimSpace(s.calendarProvider) == "" {
		s.calendarProvider = s.defaultCalendarProvider()
	}
	if strings.TrimSpace(s.calendarProvider) == "google_calendar" && !s.showGoogleCalendarProviderOption() {
		s.calendarProvider = "caldav"
	}
}

func (s *Settings) calendarProviderOptions() []huh.Option[string] {
	options := make([]huh.Option[string], 0, 4)
	if s.showGoogleCalendarProviderOption() {
		options = append(options, huh.NewOption("Google Calendar", "google_calendar"))
	}
	for _, preset := range calendarCalDAVPresets() {
		label := preset.OptionLabel
		if label == "" {
			label = preset.Label
		}
		options = append(options, huh.NewOption(label, preset.Provider))
	}
	options = append(options, huh.NewOption("Custom CalDAV", "caldav"))
	return options
}

type calendarCalDAVPreset struct {
	Provider            string
	Label               string
	OptionLabel         string
	URL                 string
	UsernamePlaceholder string
	PasswordPlaceholder string
	HelpSummary         string
	HelpLinks           []calendarProviderHelpLink
}

type calendarProviderHelpLink struct {
	Label string
	URL   string
}

func calendarCalDAVPresets() []calendarCalDAVPreset {
	return []calendarCalDAVPreset{
		{
			Provider:            "fastmail",
			Label:               "Fastmail Calendar",
			OptionLabel:         "Fastmail Calendar (app password)",
			URL:                 "https://caldav.fastmail.com/",
			UsernamePlaceholder: "you@fastmail.com",
			PasswordPlaceholder: "Fastmail app password",
			HelpSummary:         "Use your full Fastmail email address and a Fastmail app password with Mail, Contacts & Calendar access.",
			HelpLinks: []calendarProviderHelpLink{
				{Label: "App password", URL: "https://app.fastmail.com/settings/security"},
				{Label: "Server settings", URL: "https://www.fastmail.help/hc/en-us/articles/1500000278342"},
			},
		},
		{
			Provider:            "icloud",
			Label:               "iCloud Calendar",
			OptionLabel:         "iCloud Calendar (Apple app-specific password)",
			URL:                 "https://caldav.icloud.com/",
			UsernamePlaceholder: "you@icloud.com",
			PasswordPlaceholder: "Apple app-specific password",
			HelpSummary:         "Use your Apple Account email address and an Apple app-specific password for third-party calendar access.",
			HelpLinks: []calendarProviderHelpLink{
				{Label: "App password", URL: "https://support.apple.com/en-us/102654"},
			},
		},
		{
			Provider:            "yahoo",
			Label:               "Yahoo Calendar",
			OptionLabel:         "Yahoo Calendar (app password)",
			URL:                 "https://caldav.calendar.yahoo.com",
			UsernamePlaceholder: "Yahoo ID or email",
			PasswordPlaceholder: "Yahoo app password",
			HelpSummary:         "Use your Yahoo ID and a Yahoo app password with the Yahoo CalDAV server.",
			HelpLinks: []calendarProviderHelpLink{
				{Label: "App password", URL: "https://help.yahoo.com/kb/account/confirm-delete-password-sln15241.html"},
				{Label: "CalDAV setup", URL: "https://ca.help.yahoo.com/kb/new-mail-for-desktop/sync-access-calendar-multiple-devices-applications-sln4707.html"},
			},
		},
	}
}

func calendarCalDAVPresetForProvider(provider string) (calendarCalDAVPreset, bool) {
	provider = strings.TrimSpace(provider)
	for _, preset := range calendarCalDAVPresets() {
		if preset.Provider == provider {
			return preset, true
		}
	}
	return calendarCalDAVPreset{}, false
}

func calendarProviderUsesCalDAV(provider string) bool {
	if strings.TrimSpace(provider) == "caldav" {
		return true
	}
	_, ok := calendarCalDAVPresetForProvider(provider)
	return ok
}

func calendarProviderSourceProvider(provider string) string {
	if calendarProviderUsesCalDAV(provider) {
		return "caldav"
	}
	return strings.TrimSpace(provider)
}

func calendarProviderTitle(provider string) string {
	if strings.TrimSpace(provider) == "google_calendar" {
		return "Google Calendar"
	}
	if preset, ok := calendarCalDAVPresetForProvider(provider); ok {
		return preset.Label
	}
	return "Custom CalDAV"
}

func calendarProviderURLPlaceholder(provider string) string {
	if preset, ok := calendarCalDAVPresetForProvider(provider); ok {
		return preset.URL
	}
	return "https://caldav.example.com/"
}

func calendarProviderUsernamePlaceholder(provider string) string {
	if preset, ok := calendarCalDAVPresetForProvider(provider); ok {
		return preset.UsernamePlaceholder
	}
	return "you@example.com"
}

func calendarProviderPasswordPlaceholder(provider string) string {
	if preset, ok := calendarCalDAVPresetForProvider(provider); ok {
		return preset.PasswordPlaceholder
	}
	return "provider password or app password"
}

func calendarProviderHelpTitle(provider string) string {
	if preset, ok := calendarCalDAVPresetForProvider(provider); ok {
		return preset.Label + " setup"
	}
	return "Custom CalDAV setup"
}

func calendarProviderHelpDescription(provider string) string {
	if preset, ok := calendarCalDAVPresetForProvider(provider); ok {
		lines := []string{preset.HelpSummary}
		for _, link := range preset.HelpLinks {
			lines = append(lines, settingsDocLink(link.Label, link.URL))
		}
		return strings.Join(lines, "\n")
	}
	return "Use this for providers that publish a basic CalDAV endpoint. Google uses Herald's OAuth path; Proton Calendar and Microsoft Calendar are not basic CalDAV presets here."
}

func calendarProviderPanelHelpLines(provider string, width int) []string {
	var out []string
	for _, line := range strings.Split(calendarProviderHelpTitle(provider)+"\n"+calendarProviderHelpDescription(provider), "\n") {
		for _, wrapped := range render.WrapLines(line, width) {
			if strings.TrimSpace(ansi.Strip(wrapped)) != "" {
				out = append(out, wrapped)
			}
		}
	}
	return out
}

func settingsDocLink(label, rawURL string) string {
	return "[click] " + label
}

func (s *Settings) effectiveCalendarProvider() string {
	provider := strings.TrimSpace(s.calendarProvider)
	if provider == "" {
		provider = s.defaultCalendarProvider()
	}
	if s.accountDetailShowsMail() && s.alsoAddCalendar {
		if paired := pairedCalendarProviderForMailProvider(s.provider); paired != "" && provider == "caldav" && strings.TrimSpace(s.caldavURL) == "" {
			return paired
		}
	}
	return provider
}

func pairedCalendarProviderForMailProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case "fastmail", "icloud":
		return strings.TrimSpace(provider)
	case "gmail-oauth":
		return "google_calendar"
	default:
		return ""
	}
}

func (s *Settings) syncCalendarProviderDefaults(oldProvider, newProvider string) {
	if s == nil {
		return
	}
	oldPreset, oldOK := calendarCalDAVPresetForProvider(oldProvider)
	newPreset, newOK := calendarCalDAVPresetForProvider(newProvider)
	if !newOK {
		if oldOK && strings.TrimSpace(s.caldavURL) == oldPreset.URL {
			s.caldavURL = ""
		}
		return
	}
	if strings.TrimSpace(s.caldavURL) == "" || (oldOK && strings.TrimSpace(s.caldavURL) == oldPreset.URL) {
		s.caldavURL = newPreset.URL
	}
}

func (s *Settings) accountDetailCapability() string {
	hasMail := s.accountDetailShowsMail()
	hasCalendar := s.accountDetailShowsCalendar()
	switch {
	case hasMail && hasCalendar:
		return "Mail + Calendar"
	case hasCalendar:
		return "Calendar"
	case hasMail:
		return "Mail"
	default:
		return ""
	}
}

func (s *Settings) selectedAccountGroup() (config.AccountGroup, bool) {
	if s == nil || s.cfg == nil {
		return config.AccountGroup{}, false
	}
	for _, group := range s.cfg.AccountGroups() {
		if group.AccountID == s.selectedAccountID {
			return group, true
		}
	}
	return config.AccountGroup{}, false
}

func (s *Settings) resetAddAccountFields() {
	s.selectedAccountID = ""
	s.accountDisplayName = ""
	s.calendarDisplayName = ""
	s.calendarEmail = ""
	s.caldavURL = ""
	s.caldavUsername = ""
	s.caldavPassword = ""
	s.calendarProvider = s.defaultCalendarProvider()
	s.alsoAddCalendar = false
	s.deleteAccount = false
	s.accountDeletePending = false
	s.provider = "gmail-oauth"
	s.email = ""
	s.password = ""
	s.imapHost = ""
	s.imapPort = ""
	s.smtpHost = ""
	s.smtpPort = ""
	s.editGmailAdvanced = false
}

func calendarPairingSupportedProvider(provider string) bool {
	switch strings.TrimSpace(provider) {
	case "gmail-oauth", "fastmail", "icloud":
		return true
	default:
		return false
	}
}

func calendarPairingDefaultsOnForProvider(provider string) bool {
	return strings.TrimSpace(provider) == "gmail-oauth"
}

func (s *Settings) syncCalendarPairingForProviderChange(oldProvider, newProvider string) {
	if s == nil || oldProvider == newProvider {
		return
	}
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		s.firstRunOtherProvider = newProvider != "gmail-oauth"
		s.alsoAddCalendar = calendarPairingDefaultsOnForProvider(newProvider)
		if !calendarPairingSupportedProvider(newProvider) {
			s.calendarProvider = s.defaultCalendarProvider()
		}
		return
	}
	if s.mode != SettingsModePanel || s.panelSection != settingsPanelSectionAccount || s.accountEditMode != settingsAccountEditAddMail {
		return
	}
	if calendarPairingDefaultsOnForProvider(newProvider) {
		s.alsoAddCalendar = true
		s.calendarProvider = s.defaultCalendarProvider()
		return
	}
	s.alsoAddCalendar = false
	if !calendarPairingSupportedProvider(newProvider) {
		s.calendarProvider = s.defaultCalendarProvider()
	}
}

func (s *Settings) loadSelectedAccountFields() {
	group, ok := s.selectedAccountGroup()
	if !ok {
		return
	}
	s.alsoAddCalendar = len(group.CalendarSourceIDs) > 0
	s.accountDisplayName = group.DisplayName
	for _, source := range group.Sources {
		switch strings.TrimSpace(source.Kind) {
		case "", string(models.SourceKindMail):
			s.provider = source.Provider
			if s.provider == "" {
				s.provider = "imap"
			}
			if settingsMailSourceUsesGoogleOAuth(source) {
				s.provider = "gmail-oauth"
			}
			s.email = sourceAddressForSettings(source)
			s.password = source.Credentials.Password
			s.imapHost = source.IMAP.Host
			s.imapPort = portToString(source.IMAP.Port)
			s.smtpHost = source.SMTP.Host
			s.smtpPort = portToString(source.SMTP.Port)
		case string(models.SourceKindCalendar):
			s.calendarProvider = source.Provider
			if s.calendarProvider == "" {
				s.calendarProvider = "google_calendar"
			}
			s.calendarDisplayName = source.DisplayName
			s.calendarEmail = source.Google.Email
			s.caldavURL = source.CalDAV.URL
			s.caldavUsername = source.CalDAV.Username
			s.caldavPassword = source.CalDAV.Password
		}
	}
	if s.accountDisplayName == "" {
		s.accountDisplayName = group.AccountID
	}
}

func (s *Settings) existingAccountCanToggleCalendarPairing() bool {
	if s == nil || s.accountEditMode != settingsAccountEditExisting || !s.accountDetailShowsMail() {
		return false
	}
	return strings.TrimSpace(s.provider) == "gmail-oauth"
}

func (s *Settings) openSelectedAccountFromList() bool {
	if s == nil || s.mode != SettingsModePanel || s.panelSection != settingsPanelSectionAccounts {
		return false
	}
	switch s.accountsMenuChoice {
	case settingsAccountEditAddMail, settingsAccountEditAddCalendar:
		s.accountEditMode = s.accountsMenuChoice
		s.resetAddAccountFields()
		if s.accountEditMode == settingsAccountEditAddCalendar {
			s.alsoAddCalendar = false
			s.calendarProvider = s.defaultCalendarProvider()
			s.syncCalendarProviderDefaults("", s.calendarProvider)
		} else {
			s.alsoAddCalendar = calendarPairingDefaultsOnForProvider(s.provider)
			s.syncProviderDefaults("", s.provider)
		}
	case "":
		return false
	default:
		if !strings.HasPrefix(s.accountsMenuChoice, "account:") {
			return false
		}
		s.selectedAccountID = strings.TrimPrefix(s.accountsMenuChoice, "account:")
		s.accountEditMode = settingsAccountEditExisting
		s.deleteAccount = false
		s.accountDeletePending = false
		s.loadSelectedAccountFields()
	}
	s.panelSection = settingsPanelSectionAccount
	s.panelStatus = ""
	s.buildForm()
	return true
}

func (s *Settings) openSelectedAccountDeleteConfirm() bool {
	if s == nil || !strings.HasPrefix(s.accountsMenuChoice, "account:") {
		return false
	}
	s.selectedAccountID = strings.TrimPrefix(s.accountsMenuChoice, "account:")
	s.accountEditMode = settingsAccountEditExisting
	s.accountDeletePending = true
	s.deleteAccount = false
	s.loadSelectedAccountFields()
	s.panelSection = settingsPanelSectionAccount
	s.panelStatus = ""
	s.buildForm()
	return true
}

func (s *Settings) deleteSelectedAccountImmediately() tea.Cmd {
	if s == nil || !strings.HasPrefix(s.accountsMenuChoice, "account:") {
		return nil
	}
	s.selectedAccountID = strings.TrimPrefix(s.accountsMenuChoice, "account:")
	s.accountEditMode = settingsAccountEditExisting
	s.accountDeletePending = false
	s.deleteAccount = true
	s.loadSelectedAccountFields()
	s.panelSection = settingsPanelSectionAccount
	if err := s.validateSelectedAccountRemoval(); err != nil {
		s.panelStatus = err.Error()
		s.deleteAccount = false
		s.panelSection = settingsPanelSectionAccounts
		s.buildForm()
		return s.form.Init()
	}
	cfg := s.buildConfig()
	removedAccountID, removedSourceIDs := s.removedAccountScope()
	s.done = true
	return func() tea.Msg {
		return SettingsSavedMsg{
			Config:           cfg,
			ReturnToMenu:     true,
			RemovedAccountID: removedAccountID,
			RemovedSourceIDs: removedSourceIDs,
		}
	}
}

func (s *Settings) validateSelectedAccountRemoval() error {
	if s == nil || s.cfg == nil {
		return nil
	}
	_, err := s.cfg.RemoveAccountSources(s.selectedAccountID)
	return err
}

func (s *Settings) removedAccountScope() (models.AccountID, []models.SourceID) {
	if s == nil || !s.deleteAccount || s.accountEditMode != settingsAccountEditExisting {
		return "", nil
	}
	accountID := strings.TrimSpace(s.selectedAccountID)
	if accountID == "" {
		accountID = string(models.DefaultAccountID)
	}
	var sourceIDs []models.SourceID
	if s.cfg != nil {
		for _, source := range s.cfg.ExplicitSourcesForEdit() {
			sourceAccountID := strings.TrimSpace(source.AccountID)
			if sourceAccountID == "" {
				sourceAccountID = string(models.DefaultAccountID)
			}
			if sourceAccountID != accountID {
				continue
			}
			kind := models.SourceKind(strings.TrimSpace(source.Kind))
			if kind == "" {
				kind = models.SourceKindMail
			}
			sourceIDs = append(sourceIDs, models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultSourceIDForKind(kind)))
		}
	}
	return models.NormalizeAccountID(models.AccountID(accountID)), sourceIDs
}

func sourceAddressForSettings(source config.SourceConfig) string {
	for _, value := range []string{source.Credentials.Username, source.Google.Email, source.CalDAV.Username} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *Settings) consumeFormNavigationCmd(cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 8 {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			s.consumeFormNavigationCmd(child, depth+1)
		}
		return
	}
	if model, next := s.form.Update(msg); model != nil {
		if form, ok := model.(*huh.Form); ok {
			s.form = form
		}
		s.consumeFormNavigationCmd(next, depth+1)
	}
}

func (s *Settings) navigateWizardBack(msg tea.Msg) tea.Cmd {
	if s.mode != SettingsModeWizard || s.form == nil || s.form.State == huh.StateCompleted {
		return nil
	}
	s.bypassWizardBackValidation = true
	defer func() { s.bypassWizardBackValidation = false }()

	var cmd tea.Cmd
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.Code == tea.KeyEscape {
		cmd = s.form.PrevGroup()
	} else {
		cmd = s.form.PrevField()
	}
	s.consumeFormNavigationCmd(cmd, 0)
	return nil
}

func (s *Settings) keyHints() string {
	escHelp := s.focusedFieldEscHelp()
	if s.panelSection == settingsPanelSectionMenu {
		switch escHelp {
		case "exit filter":
			return "type: filter  │  esc: exit filter  │  enter: open category"
		case "clear filter":
			return "↑/↓: move  │  enter: open category  │  /: filter  │  esc: clear filter"
		default:
			return "↑/↓: move  │  enter: open category  │  /: filter  │  esc: exit settings"
		}
	}
	if s.panelSection == settingsPanelSectionAccounts {
		return "enter/e: edit │ d/\\: disconnect │ D/|: delete │ esc: back"
	}
	if s.panelSection == settingsPanelSectionAccount {
		return "tab: fields  │  enter: edit/select  │  esc: back to accounts"
	}
	if escHelp == "exit filter" {
		return "type: filter  │  esc: exit filter  │  enter: select"
	}
	if escHelp == "clear filter" {
		return "↑/↓: move  │  enter: select  │  /: filter  │  esc: clear filter"
	}
	return "tab: fields  │  enter: edit/select  │  esc: back to settings menu"
}

func (s *Settings) returnToPanelMenu() tea.Cmd {
	choice := string(s.panelSection)
	if choice == "" {
		choice = s.panelMenuChoice
	}
	next := NewSettingsWithPathAndOptions(
		SettingsModePanel,
		s.cfg,
		s.configPath,
		SettingsOptions{ShowExperimentalEmailServices: s.showExperimentalEmailServices},
	)
	next.width = s.width
	next.height = s.height
	next.panelMenuChoice = choice
	next.panelStatus = ""
	next.buildForm()
	*s = *next
	return s.form.Init()
}

func (s *Settings) returnToAccountsList() tea.Cmd {
	next := NewSettingsWithPathAndOptions(
		SettingsModePanel,
		s.cfg,
		s.configPath,
		SettingsOptions{ShowExperimentalEmailServices: s.showExperimentalEmailServices},
	)
	next.width = s.width
	next.height = s.height
	next.panelSection = settingsPanelSectionAccounts
	next.panelMenuChoice = string(settingsPanelSectionAccounts)
	next.accountsMenuChoice = s.accountsMenuChoice
	if !accountListHasOption(next.accountListOptions(), next.accountsMenuChoice) {
		next.accountsMenuChoice = settingsAccountEditAddMail
		if !accountListHasOption(next.accountListOptions(), next.accountsMenuChoice) {
			for _, option := range next.accountListOptions() {
				next.accountsMenuChoice = option.Value
				break
			}
		}
	}
	next.panelStatus = s.panelStatus
	next.buildForm()
	*s = *next
	return s.form.Init()
}

func (s *Settings) panelLayout() settingsPanelLayout {
	w := s.width
	if w <= 0 {
		w = 80
	}
	h := s.height
	if h <= 0 {
		h = 24
	}

	panelW := shortcutHelpMaxWidth
	if maxW := w - 4; maxW < panelW {
		panelW = maxW
	}
	if panelW < 40 {
		panelW = w
	}
	if panelW < 32 {
		panelW = 32
	}

	panelH := 36
	if maxH := h - 4; maxH < panelH {
		panelH = maxH
	}
	if panelH < 10 {
		panelH = h
	}
	if panelH < 6 {
		panelH = 6
	}

	formW := panelW - 6
	if formW < 20 {
		formW = 20
	}
	// huh reserves spacer/help rows outside the viewport in addition to our
	// border and vertical padding, so leave them in the outer panel budget.
	formH := panelH - 6
	if formH < 4 {
		formH = 4
	}

	return settingsPanelLayout{
		panelWidth:  panelW,
		panelHeight: panelH,
		formWidth:   formW,
		formHeight:  formH,
	}
}

// formWidth returns the width the form should use.
func (s *Settings) formWidth() int {
	if s.mode == SettingsModePanel {
		return s.panelLayout().formWidth
	}
	w := s.wizardBoxWidth() - 6
	if w < 52 {
		w = 52
	}
	return w
}

func (s *Settings) formHeight() int {
	if s.mode == SettingsModePanel {
		return s.panelLayout().formHeight
	}
	h := s.height - 12
	if h < 8 {
		h = 8
	}
	return h
}

func (s *Settings) wizardBoxWidth() int {
	return wizardBoxWidthFor(s.width)
}

// Init implements tea.Model.
func (s *Settings) Init() tea.Cmd {
	return s.form.Init()
}

// ResetForRetry reopens the current wizard settings form after a failed
// validation or OAuth attempt, preserving the values the user already typed.
func (s *Settings) ResetForRetry() tea.Cmd {
	if s == nil {
		return nil
	}
	s.done = false
	s.saveButton = true
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		s.firstRunOtherProvider = s.provider != "gmail-oauth"
	}
	s.buildForm()
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		s.form.NextGroup()
	}
	return s.form.Init()
}

// Update implements tea.Model.
func (s *Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.done {
		return s, nil
	}

	formMsg := msg
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.setSize(msg.Width, msg.Height)
		return s, nil

	case tea.KeyPressMsg:
		if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAccounts && s.form.State != huh.StateCompleted {
			switch shortcutKey(msg) {
			case "e":
				if s.openSelectedAccountFromList() {
					return s, s.form.Init()
				}
			case "d", "\\":
				if s.openSelectedAccountDeleteConfirm() {
					return s, s.form.Init()
				}
			case "D", "|":
				if cmd := s.deleteSelectedAccountImmediately(); cmd != nil {
					return s, cmd
				}
			}
		}
		if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionSync {
			switch shortcutKey(msg) {
			case "W":
				s.done = true
				return s, func() tea.Msg { return SettingsToolRequestedMsg{Tool: settingsCleanupToolAutomation} }
			case "P":
				s.done = true
				return s, func() tea.Msg { return SettingsToolRequestedMsg{Tool: settingsCleanupToolPrompts} }
			case "C":
				s.done = true
				return s, func() tea.Msg { return SettingsToolRequestedMsg{Tool: settingsCleanupToolRules} }
			}
		}
		if s.mode == SettingsModeWizard && msg.Code == tea.KeyEscape {
			return s, s.navigateWizardBack(msg)
		}
		if s.mode == SettingsModeWizard && msg.Code == tea.KeyTab && msg.Mod.Contains(tea.ModShift) {
			return s, s.navigateWizardBack(msg)
		}
		// In panel mode, esc cancels if the active field has no local esc action.
		if s.mode == SettingsModePanel && msg.Code == tea.KeyEscape {
			if s.form.State != huh.StateCompleted && !s.focusedFieldHandlesKey(msg) {
				if s.panelSection == settingsPanelSectionAccount || s.panelSection == settingsPanelSectionAddAccount {
					return s, s.returnToAccountsList()
				}
				if s.panelSection == settingsPanelSectionAccounts {
					return s, s.returnToPanelMenu()
				}
				if s.panelSection != settingsPanelSectionMenu {
					return s, s.returnToPanelMenu()
				}
				s.done = true
				return s, func() tea.Msg { return SettingsCancelledMsg{} }
			}
		}
		if s.shouldOpenThemePickerFromManualInput(msg) {
			formMsg = huh.NextField()
		}
	}

	// Forward to the form.
	prevProvider := s.provider
	prevCalendarProvider := s.calendarProvider
	prevAISetupPreset := s.aiSetupPreset
	prevAIProvider := s.aiProvider
	prevEmbeddingProvider := s.embeddingProvider
	prevThemeRole := s.themeRole
	prevThemeFG := s.themeFG
	prevThemeBG := s.themeBG
	model, cmd := s.form.Update(formMsg)
	if f, ok := model.(*huh.Form); ok {
		s.form = f
	}
	if prevProvider != s.provider {
		s.syncProviderDefaults(prevProvider, s.provider)
		s.syncCalendarPairingForProviderChange(prevProvider, s.provider)
	}
	if prevCalendarProvider != s.calendarProvider {
		s.syncCalendarProviderDefaults(prevCalendarProvider, s.calendarProvider)
	}
	if prevAISetupPreset != s.aiSetupPreset {
		s.applyAISetupPreset()
	} else if prevAIProvider != s.aiProvider || prevEmbeddingProvider != s.embeddingProvider {
		s.syncAIDefaults()
		_ = s.syncExternalAIDefaults()
	}
	if !s.advancedAISetup() {
		s.refreshAISetupPresetFromRoles()
	}
	s.syncAIDefaults()
	s.syncThemeRoleFields(prevThemeRole, prevThemeFG, prevThemeBG)

	// Check if the form just completed.
	if s.form.State == huh.StateCompleted && !s.done {
		if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionMenu {
			s.panelSection = settingsPanelSection(s.panelMenuChoice)
			s.panelStatus = ""
			s.saveButton = true
			s.buildForm()
			return s, tea.Batch(cmd, s.form.Init())
		}
		if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAccounts {
			if s.openSelectedAccountFromList() {
				return s, tea.Batch(cmd, s.form.Init())
			}
		}
		if s.mode == SettingsModeWizard && s.firstRunPreferencesOnly && !s.firstRunCustomizePreferences && s.firstRunPreferenceAction == firstRunPreferenceCustomize {
			s.firstRunCustomizePreferences = true
			s.firstRunPreferenceAction = firstRunPreferenceEnter
			s.saveButton = true
			s.done = false
			s.buildForm()
			return s, tea.Batch(cmd, s.form.Init())
		}
		s.done = true
		if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAccount && s.accountDeletePending {
			if !s.deleteAccount {
				return s, tea.Batch(cmd, s.returnToAccountsList())
			}
			if err := s.validateSelectedAccountRemoval(); err != nil {
				s.done = false
				s.panelStatus = err.Error()
				s.accountDeletePending = false
				s.deleteAccount = false
				s.panelSection = settingsPanelSectionAccounts
				s.buildForm()
				return s, tea.Batch(cmd, s.form.Init())
			}
		}
		if s.mode == SettingsModePanel && !s.saveButton {
			return s, tea.Batch(cmd, func() tea.Msg { return SettingsCancelledMsg{} })
		}
		if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionSync && s.cleanupToolSelection != settingsCleanupToolNone {
			tool := s.cleanupToolSelection
			return s, tea.Batch(cmd, func() tea.Msg {
				return SettingsToolRequestedMsg{Tool: tool}
			})
		}

		if s.mode == SettingsModePanel && isThemeSettingsSection(s.panelSection) {
			if err := s.applyThemeFileActions(); err != nil {
				s.done = false
				s.panelStatus = "Theme update failed: " + err.Error()
				s.saveButton = true
				s.buildForm()
				return s, tea.Batch(cmd, s.form.Init())
			}
		}

		cfg := s.buildConfig()
		removedAccountID, removedSourceIDs := s.removedAccountScope()
		if oauthMsg, ok := s.oauthRequiredMsg(cfg); ok {
			return s, tea.Batch(cmd, func() tea.Msg {
				return oauthMsg
			})
		}
		// Non-Gmail: signal done; the caller is responsible for saving.
		return s, tea.Batch(cmd, func() tea.Msg {
			return SettingsSavedMsg{
				Config:                     cfg,
				ReturnToMenu:               s.mode == SettingsModePanel,
				ReclaimOfflineCacheStorage: s.reclaimOfflineCacheStorage,
				ValidateAccount:            s.requiresAccountValidation(),
				ValidateCalendar:           s.requiresCalendarValidation(),
				CalendarSourceIDs:          s.calendarSourceIDsForValidation(cfg),
				ValidateOllamaModels:       s.requiresOllamaModelValidation(cfg),
				RemovedAccountID:           removedAccountID,
				RemovedSourceIDs:           removedSourceIDs,
			}
		})
	}

	// Check if the form was aborted (e.g. ctrl+c within the form).
	if s.form.State == huh.StateAborted && !s.done {
		s.done = true
		if s.mode == SettingsModePanel {
			return s, tea.Batch(cmd, func() tea.Msg { return SettingsCancelledMsg{} })
		}
		// In wizard mode, abort = quit.
		return s, tea.Batch(cmd, tea.Quit)
	}

	return s, cmd
}

// View implements tea.Model.
func (s *Settings) View() tea.View {
	s.syncAIDefaults()
	aiDefaultsChanged := s.syncExternalAIDefaults()
	currentFormView := s.form.View()
	if s.ensureProviderDefaults() || s.needsPresetFieldRefresh(currentFormView) {
		s.refreshFormPreservingVisibleGroup(s.visibleSettingsGroupTarget(currentFormView))
		currentFormView = s.form.View()
	}
	if aiDefaultsChanged {
		s.refreshFormPreservingVisibleGroup(s.visibleSettingsGroupTarget(currentFormView))
		currentFormView = s.form.View()
	}
	if s.needsFirstRunProviderDetailRefresh(currentFormView) {
		s.refreshFirstRunProviderDetailGroup()
		currentFormView = s.form.View()
	}
	formView := strings.TrimRight(currentFormView, "\n")

	if s.mode == SettingsModePanel {
		rendered := s.renderPanelWithFormView(formView)
		return newHeraldView(strings.TrimRight(lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, rendered), "\n"))
	}

	if s.width > 0 && s.width < minTermWidth {
		return newHeraldView(renderMinSizeMessage(s.width, s.height))
	}
	if s.height > 0 && s.height < minTermHeight {
		return newHeraldView(renderMinSizeMessage(s.width, s.height))
	}

	boxWidth := s.wizardBoxWidth()
	title := defaultTheme.Setup.Title.Style().Render("Herald Setup")
	summary := s.renderWizardSummary(boxWidth)
	box := lipgloss.NewStyle().
		Width(boxWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(defaultTheme.Setup.Border.ForegroundColor()).
		Padding(1, 2)

	rendered := lipgloss.JoinVertical(lipgloss.Left,
		title,
		summary,
		box.Render(formView),
	)
	rendered = strings.TrimRight(rendered, "\n")
	if s.width > 0 && s.height > 0 {
		return newHeraldView(strings.TrimRight(lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, rendered), "\n"))
	}
	return newHeraldView(rendered)
}

func (s *Settings) renderPanel() string {
	return s.renderPanelWithFormView(strings.TrimRight(s.form.View(), "\n"))
}

func (s *Settings) renderPanelWithFormView(currentFormView string) string {
	formView := s.panelFormViewFrom(currentFormView)
	layout := s.panelLayout()
	box := lipgloss.NewStyle().
		Width(layout.formWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(defaultTheme.Setup.Border.ForegroundColor()).
		Padding(1, 2)

	rendered := strings.TrimRight(box.Render(formView), "\n")
	lines := strings.Split(rendered, "\n")
	if len(lines) > layout.panelHeight && layout.panelHeight > 0 {
		bottomBorder := lines[len(lines)-1]
		lines = append(lines[:layout.panelHeight-1], bottomBorder)
	}
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, layout.panelWidth)
	}
	return s.panelFormViewLinkifyCalendarProviderHelp(strings.Join(lines, "\n"))
}

func (s *Settings) panelFormView() string {
	return s.panelFormViewFrom(strings.TrimRight(s.form.View(), "\n"))
}

func (s *Settings) panelFormViewFrom(formView string) string {
	if s.panelSection == settingsPanelSectionMenu {
		formView = strings.Replace(formView, "enter submit", "enter open • esc exit", 1)
	}
	formView = s.panelFormViewWithCalendarProviderHelp(formView)
	return s.panelFormViewWithFooterDivider(formView)
}

func (s *Settings) panelFormViewWithCalendarProviderHelp(formView string) string {
	if s == nil || s.mode != SettingsModePanel || s.panelSection != settingsPanelSectionAccount || !s.accountDetailShowsCalendar() {
		return formView
	}
	if !strings.Contains(ansi.Strip(formView), "CalDAV URL>") {
		return formView
	}
	provider := s.effectiveCalendarProvider()
	if !calendarProviderUsesCalDAV(provider) {
		return formView
	}
	lines := strings.Split(formView, "\n")
	insertAt := -1
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), "CalDAV URL>") {
			insertAt = i + 1
			break
		}
	}
	if insertAt < 0 {
		return formView
	}
	helpWidth := s.formWidth() - 6
	if helpWidth < 24 {
		helpWidth = 24
	}
	var help []string
	for _, line := range calendarProviderPanelHelpLines(provider, helpWidth) {
		help = append(help, "  "+line)
	}
	out := make([]string, 0, len(lines)+len(help))
	out = append(out, lines[:insertAt]...)
	out = append(out, help...)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n")
}

func (s *Settings) panelFormViewLinkifyCalendarProviderHelp(rendered string) string {
	if s == nil || s.mode != SettingsModePanel || s.panelSection != settingsPanelSectionAccount || !s.accountDetailShowsCalendar() {
		return rendered
	}
	provider := s.effectiveCalendarProvider()
	if !calendarProviderUsesCalDAV(provider) {
		return rendered
	}
	preset, ok := calendarCalDAVPresetForProvider(provider)
	if !ok {
		return rendered
	}
	for _, link := range preset.HelpLinks {
		visible := "[click] " + link.Label
		rendered = strings.ReplaceAll(rendered, visible, wizardHyperlink("[click]", link.URL)+" "+link.Label)
	}
	return rendered
}

func (s *Settings) panelFormViewWithFooterDivider(formView string) string {
	lines := strings.Split(formView, "\n")
	footerIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(ansi.Strip(lines[i])) != "" {
			footerIdx = i
			break
		}
	}
	if footerIdx <= 0 {
		return formView
	}

	if hint := strings.TrimSpace(s.keyHints()); hint != "" && s.panelSection == settingsPanelSectionAccounts {
		hintLine := defaultTheme.Chrome.FormHelp.Style().Render(hint)
		footerText := strings.TrimSpace(ansi.Strip(lines[footerIdx]))
		if settingsLooksLikeFooterHint(footerText) {
			lines[footerIdx] = hintLine
		} else {
			hintIdx := -1
			for i := len(lines) - 1; i > footerIdx; i-- {
				if strings.TrimSpace(ansi.Strip(lines[i])) == "" {
					hintIdx = i
					break
				}
			}
			if hintIdx < 0 {
				lines = append(lines, hintLine)
				footerIdx = len(lines) - 1
			} else {
				lines[hintIdx] = hintLine
				footerIdx = hintIdx
			}
		}
	}

	dividerWidth := s.panelLayout().formWidth - 6
	if dividerWidth < 20 {
		dividerWidth = 20
	}
	divider := strings.Repeat("─", dividerWidth)
	next := make([]string, 0, len(lines)+1)
	next = append(next, lines[:footerIdx]...)
	next = append(next, divider, lines[footerIdx])
	next = append(next, lines[footerIdx+1:]...)
	if hintBlankIdx := footerIdx + 2; hintBlankIdx < len(next) && strings.TrimSpace(ansi.Strip(next[hintBlankIdx])) == "" {
		next = append(next[:hintBlankIdx], next[hintBlankIdx+1:]...)
	}
	return strings.Join(next, "\n")
}

func settingsLooksLikeFooterHint(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, token := range []string{"enter", "esc", "tab", "filter", "submit", "toggle", "save", "up", "down", "↑", "↓"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

// buildConfig constructs a config.Config from the current form field values.
// It starts from a copy of the existing config so that fields not managed by
// this form (Daemon, Semantic, Classification.Prompts, OAuth tokens, etc.) are
// preserved unchanged.
func (s *Settings) buildConfig() *config.Config {
	s.refreshAISetupPresetFromRoles()
	s.applyAISetupPreset()
	s.ensureProviderDefaults()
	s.syncAIDefaults()
	_ = s.syncExternalAIDefaults()
	// Shallow copy preserves all non-pointer fields; pointer/slice fields that
	// this form does not modify are left pointing at the same underlying data
	// (safe because we never mutate them — we only overwrite scalar fields below).
	cfg := *s.cfg
	if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAccount {
		cfg = s.buildAccountSourcesConfig(cfg)
	}
	firstRunGoogleFastPath := s.mode == SettingsModeWizard && s.firstRunAccountOnly && s.provider == "gmail-oauth" && !s.firstRunOtherProvider
	if firstRunGoogleFastPath {
		cfg = s.buildFirstRunGoogleSourcesConfig(cfg)
	} else if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		cfg.Sources = nil
	}
	if s.requiresAccountValidation() {
		accountProvider := s.provider
		if firstRunGoogleFastPath {
			accountProvider = "gmail-oauth"
		}
		cfg.Vendor = configVendorForProvider(accountProvider)
		cfg.Gmail.AccessToken = ""
		cfg.Gmail.RefreshToken = ""
		cfg.Gmail.TokenExpiry = ""
		cfg.Gmail.Email = ""
		cfg.Credentials.Username = ""
		cfg.Credentials.Password = ""

		if accountProvider == "gmail-oauth" {
			cfg.Gmail.Email = s.email
		} else {
			cfg.Credentials.Username = s.email
			cfg.Credentials.Password = s.password
		}

		cfg.Server.Host = s.imapHost
		cfg.Server.Port = parsePort(s.imapPort)
		cfg.SMTP.Host = s.smtpHost
		cfg.SMTP.Port = parsePort(s.smtpPort)
	}

	// AI provider
	if s.disableAIFromWarning {
		s.aiProvider = aiProviderDisabled
	}
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		s.aiProvider = aiProviderDisabled
	}
	cfg.AI.Provider = configAIProvider(s.aiProvider)
	cfg.Ollama.Host = s.ollamaHost
	cfg.Ollama.Model = s.ollamaModel
	cfg.Ollama.EmbeddingModel = s.embedModel
	cfg.Claude.APIKey = s.claudeAPIKey
	cfg.Claude.Model = s.claudeModel
	cfg.OpenAI.APIKey = s.openAIAPIKey
	cfg.OpenAI.BaseURL = s.openAIBaseURL
	cfg.OpenAI.Model = s.openAIModel
	cfg.OpenAI.EmbeddingModel = s.openAIEmbeddingModel
	cfg.Semantic.Provider = s.effectiveSettingsEmbeddingProvider()
	switch cfg.Semantic.Provider {
	case config.EmbeddingProviderOpenAI:
		cfg.Semantic.Model = strings.TrimSpace(firstNonEmptyString(s.openAIEmbeddingModel, defaultOpenAIEmbed))
	default:
		cfg.Semantic.Model = strings.TrimSpace(firstNonEmptyString(s.embedModel, defaultEmbeddingModel))
	}
	cfg.AI.Agent.ReasoningEffort = ""
	if configAIProvider(s.aiProvider) == agent.ProviderOpenAI {
		cfg.AI.Agent.ReasoningEffort = agent.NormalizeReasoningEffort(s.agentReasoningEffort)
		if cfg.AI.Agent.ReasoningEffort == "" {
			cfg.AI.Agent.ReasoningEffort = defaultChatEffort
		}
	}

	switch s.aiProvider {
	case aiProviderOllamaDefault:
		cfg.Ollama.Host = defaultOllamaHost
		cfg.Ollama.Model = defaultOllamaModel
		cfg.Ollama.EmbeddingModel = defaultEmbeddingModel
		if !s.advancedAISetup() {
			cfg.Semantic.Provider = config.EmbeddingProviderOllama
			cfg.Semantic.Model = defaultEmbeddingModel
		}
	case aiProviderOllamaCustom:
		if !s.advancedAISetup() {
			cfg.Semantic.Provider = config.EmbeddingProviderOllama
			cfg.Semantic.Model = strings.TrimSpace(firstNonEmptyString(cfg.Ollama.EmbeddingModel, defaultEmbeddingModel))
		}
	case aiProviderDisabled:
		cfg.Ollama.Host = ""
		cfg.Ollama.Model = ""
		cfg.Ollama.EmbeddingModel = ""
		cfg.Semantic.Provider = ""
		cfg.Semantic.Model = ""
		cfg.Claude.APIKey = ""
		cfg.OpenAI.APIKey = ""
		cfg.AI.Agent = config.AgentConfig{}
	}

	// Sync & cleanup
	if n, err := strconv.Atoi(s.syncPollStr); err == nil {
		cfg.Sync.PollIntervalMinutes = n
	}
	cfg.Sync.IDLEEnabled = s.syncIDLE
	cfg.Cache.StoragePolicy = config.NormalizeCacheStoragePolicy(s.cacheStoragePolicy)
	if n, err := strconv.Atoi(s.cleanupScheduleStr); err == nil {
		cfg.Cleanup.ScheduleHours = n
	}
	cfg.Calendar.WeekStart = config.NormalizeCalendarWeekStart(s.calendarWeekStart)
	cfg.Compose.Signature.Text = normalizeSignatureText(s.signatureText)
	cfg.Keyboard.Profile = s.keyboardProfile
	cfg.Keyboard.CustomKeymap = strings.TrimSpace(s.customKeymap)
	cfg.Theme.Name = strings.TrimSpace(s.themeName)
	if cfg.Theme.Name == "" {
		cfg.Theme.Name = "inherited"
	}
	cfg.Theme.Overrides = cloneThemeOverrides(s.themeOverrides)
	if cfg.Theme.Overrides == nil {
		cfg.Theme.Overrides = make(map[string]config.ThemeOverride)
	}
	if s.themeResetAll {
		cfg.Theme.Overrides = make(map[string]config.ThemeOverride)
	} else if s.themeResetRole {
		delete(cfg.Theme.Overrides, s.themeRole)
	} else if strings.TrimSpace(s.themeFG) != "" || strings.TrimSpace(s.themeBG) != "" {
		storeThemeFieldsInMap(cfg.Theme.Overrides, s.themeRole, s.themeFG, s.themeBG)
	}
	if (s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionMemories) ||
		(s.mode == SettingsModeWizard && s.firstRunPreferencesOnly) {
		cfg.Memories = s.buildMemorySettingsConfig(cfg.Memories)
	}

	applyVendorPreset(&cfg)
	return &cfg
}

func (s *Settings) buildMemorySettingsConfig(existing memory.Settings) memory.Settings {
	settings := existing
	settings.ApplyDefaults()
	settings.Enabled = s.memoryEnabled
	settings.Immutable = true
	settings.Directory = strings.TrimSpace(firstNonEmptyString(s.memoryDirectory, memory.DefaultDirectory))
	settings.Profile = "obsidian-friendly"
	settings.Sources.Folders = settingsParseCSV(s.memorySourceFolders, []string{"INBOX", "Sent"})
	settings.Tasks = settingsMemoryTasksFromChoices(s.memoryTaskChoices)
	settings.Destinations.People = strings.TrimSpace(firstNonEmptyString(s.memoryPeopleDestination, settings.Destinations.People, "People"))
	settings.Destinations.Companies = strings.TrimSpace(firstNonEmptyString(s.memoryCompaniesDestination, settings.Destinations.Companies, "Job search/active"))
	settings.Destinations.JobSearch = strings.TrimSpace(firstNonEmptyString(s.memoryJobSearchDestination, settings.Destinations.JobSearch, "Job search"))
	settings.Destinations.Projects = strings.TrimSpace(firstNonEmptyString(s.memoryProjectsDestination, settings.Destinations.Projects, "Projects"))
	settings.Destinations.Threads = strings.TrimSpace(firstNonEmptyString(s.memoryThreadsDestination, settings.Destinations.Threads, "Threads"))
	settings.Destinations.Research = strings.TrimSpace(firstNonEmptyString(s.memoryResearchDestination, settings.Destinations.Research, "Research"))
	settings.Destinations.DailyBriefing = strings.TrimSpace(firstNonEmptyString(s.memoryDailyDestination, settings.Destinations.DailyBriefing, "Scheduled Task Artifacts"))
	settings.Destinations.Inbox = strings.TrimSpace(firstNonEmptyString(s.memoryInboxDestination, settings.Destinations.Inbox, "Memory Inbox"))

	settings.Thresholds.ChatRetrieval = settingsParseFloatOr(s.memoryChatThresholdStr, settings.Thresholds.ChatRetrieval)
	settings.Thresholds.Dossier = settingsParseFloatOr(s.memoryDossierThresholdStr, settings.Thresholds.Dossier)
	settings.Thresholds.ObsidianWrite = settingsParseFloatOr(s.memoryObsidianThresholdStr, settings.Thresholds.ObsidianWrite)
	settings.Thresholds.ComposeRadar = settingsParseFloatOr(s.memoryComposeThresholdStr, settings.Thresholds.ComposeRadar)
	settings.Thresholds.Match = settingsParseFloatOr(s.memoryMatchThresholdStr, settings.Thresholds.Match)

	settings.UpdateRules.Cadence = settingsMemoryUpdateCadence(s.memoryUpdateCadence)
	settings.UpdateRules.MatchThreshold = settings.Thresholds.Match
	settings.UpdateRules.StaleAfterDays = settingsParsePositiveIntOr(s.memoryStaleAfterDaysStr, settings.UpdateRules.StaleAfterDays)
	settings.UpdateRules.RetentionDays = settingsParseNonNegativeIntOr(s.memoryRetentionDaysStr, settings.UpdateRules.RetentionDays)
	settings.UpdateRules.LowConfidenceDisposition = settingsMemoryLowConfidenceDisposition(s.memoryLowConfidenceDisposition)

	settings.Obsidian.Enabled = s.memoryObsidianEnabled
	settings.Obsidian.VaultPath = strings.TrimSpace(s.memoryVaultPath)
	settings.Obsidian.FrontmatterMode = memory.NormalizeFrontmatterMode(s.memoryFrontmatterMode)
	settings.Obsidian.YAMLHeaders = s.memoryYAMLHeaders && settings.Obsidian.FrontmatterMode != memory.FrontmatterNone
	if !settings.Obsidian.YAMLHeaders {
		settings.Obsidian.FrontmatterMode = memory.FrontmatterNone
	}
	settings.Obsidian.LinkMode = memory.NormalizeLinkMode(s.memoryLinkMode)
	settings.Obsidian.TagMode = memory.NormalizeTagMode(s.memoryTagMode)
	if len(settings.Prompts) == 0 {
		settings.Prompts = memory.DefaultPromptTemplates()
	}
	return settings
}

func (s *Settings) buildFirstRunGoogleSourcesConfig(cfg config.Config) config.Config {
	email := strings.TrimSpace(s.email)
	accountID := settingsSlug(firstNonEmptyString(s.accountDisplayName, email, "google-account"))
	if accountID == "" {
		accountID = "google-account"
	}
	displayName := strings.TrimSpace(firstNonEmptyString(s.accountDisplayName, email, "Google Mail"))
	mailID := settingsUniqueSourceID(nil, firstNonEmptyString(displayName, accountID), "mail")
	mail := config.SourceConfig{
		ID:          mailID,
		Kind:        string(models.SourceKindMail),
		Provider:    "gmail",
		DisplayName: displayName,
		AccountID:   accountID,
		Google: config.GoogleConfig{
			Email:        email,
			AccessToken:  cfg.Gmail.AccessToken,
			RefreshToken: cfg.Gmail.RefreshToken,
			TokenExpiry:  cfg.Gmail.TokenExpiry,
		},
		Compose: cfg.Compose,
	}
	sources := []config.SourceConfig{mail}
	if s.alsoAddCalendar {
		calendarName := strings.TrimSpace(firstNonEmptyString(s.calendarDisplayName, displayName+" Calendar", "Google Calendar"))
		calendarID := settingsUniqueSourceID(sources, firstNonEmptyString(displayName, accountID), "calendar")
		sources = append(sources, config.SourceConfig{
			ID:          calendarID,
			Kind:        string(models.SourceKindCalendar),
			Provider:    "google_calendar",
			DisplayName: calendarName,
			AccountID:   accountID,
			Google: config.GoogleConfig{
				Email:        strings.TrimSpace(firstNonEmptyString(s.calendarEmail, email)),
				AccessToken:  cfg.Gmail.AccessToken,
				RefreshToken: cfg.Gmail.RefreshToken,
				TokenExpiry:  cfg.Gmail.TokenExpiry,
			},
		})
	}
	cfg.Sources = sources
	syncLegacyMailFieldsForSettings(&cfg)
	return cfg
}

func (s *Settings) buildAccountSourcesConfig(cfg config.Config) config.Config {
	if s.deleteAccount && s.accountEditMode == settingsAccountEditExisting {
		next, err := cfg.RemoveAccountSources(s.selectedAccountID)
		if err != nil {
			s.panelStatus = err.Error()
			return cfg
		}
		return next
	}

	sources := editableSourcesForSettings(cfg)
	accountID := strings.TrimSpace(s.selectedAccountID)
	if accountID == "" {
		accountID = settingsSlug(firstNonEmptyString(s.accountDisplayName, s.email, s.calendarEmail, s.caldavUsername, "account"))
	}
	if accountID == "" {
		accountID = "account"
	}
	if s.accountEditMode == settingsAccountEditExisting {
		var kept []config.SourceConfig
		for _, source := range sources {
			sourceAccountID := strings.TrimSpace(source.AccountID)
			if sourceAccountID == "" {
				sourceAccountID = string(models.DefaultAccountID)
			}
			if sourceAccountID != accountID {
				kept = append(kept, source)
			}
		}
		sources = kept
	}

	if s.accountDetailShowsMail() {
		sources = append(sources, s.mailSourceConfig(accountID, sources))
	}
	if s.accountDetailShowsCalendar() {
		sources = append(sources, s.calendarSourceConfig(accountID, sources))
	}
	cfg.Sources = sources
	syncLegacyMailFieldsForSettings(&cfg)
	return cfg
}

func editableSourcesForSettings(cfg config.Config) []config.SourceConfig {
	if len(cfg.Sources) > 0 {
		return cfg.ExplicitSourcesForEdit()
	}
	if strings.TrimSpace(cfg.Credentials.Username) == "" && strings.TrimSpace(cfg.Server.Host) == "" && strings.TrimSpace(cfg.SMTP.Host) == "" && strings.TrimSpace(cfg.Gmail.Email) == "" {
		return nil
	}
	return cfg.ExplicitSourcesForEdit()
}

func (s *Settings) mailSourceConfig(accountID string, existing []config.SourceConfig) config.SourceConfig {
	id := settingsUniqueSourceID(existing, firstNonEmptyString(s.accountDisplayName, s.email, accountID), "mail")
	existingSource, hasExistingSource := s.existingSourceForKind(models.SourceKindMail)
	usesExistingGoogleOAuth := s.accountEditMode == settingsAccountEditExisting && hasExistingSource && settingsMailSourceUsesGoogleOAuth(existingSource)
	usesGoogleOAuth := s.provider == "gmail-oauth" || usesExistingGoogleOAuth
	provider := mailSourceProviderForSettings(s.provider)
	if s.accountEditMode == settingsAccountEditExisting {
		if hasExistingSource && strings.TrimSpace(existingSource.ID) != "" {
			id = strings.TrimSpace(existingSource.ID)
		}
		if hasExistingSource && strings.TrimSpace(existingSource.Provider) != "" {
			provider = strings.TrimSpace(existingSource.Provider)
		}
	}
	source := config.SourceConfig{
		ID:          id,
		Kind:        string(models.SourceKindMail),
		Provider:    provider,
		DisplayName: strings.TrimSpace(firstNonEmptyString(s.accountDisplayName, s.email, accountID)),
		AccountID:   accountID,
		IMAP:        config.ServerConfig{Host: strings.TrimSpace(s.imapHost), Port: parsePort(s.imapPort)},
		SMTP:        config.ServerConfig{Host: strings.TrimSpace(s.smtpHost), Port: parsePort(s.smtpPort)},
		Compose:     s.cfg.Compose,
	}
	if usesGoogleOAuth {
		source.Google.Email = strings.TrimSpace(s.email)
		source.Google.AccessToken = s.cfg.Gmail.AccessToken
		source.Google.RefreshToken = s.cfg.Gmail.RefreshToken
		source.Google.TokenExpiry = s.cfg.Gmail.TokenExpiry
		if hasExistingSource {
			source.Google = existingSource.Google
			source.Google.Email = strings.TrimSpace(s.email)
		}
	} else {
		source.Credentials.Username = strings.TrimSpace(s.email)
		source.Credentials.Password = s.password
	}
	return source
}

func (s *Settings) calendarSourceConfig(accountID string, existing []config.SourceConfig) config.SourceConfig {
	providerChoice := s.effectiveCalendarProvider()
	provider := calendarProviderSourceProvider(providerChoice)
	if provider == "" {
		provider = "google_calendar"
	}
	name := firstNonEmptyString(s.calendarDisplayName, s.accountDisplayName+" Calendar", s.calendarEmail, s.caldavUsername, calendarProviderTitle(providerChoice), accountID+" Calendar")
	existingSource, hasExistingSource := s.existingSourceForKind(models.SourceKindCalendar)
	if s.accountEditMode == settingsAccountEditExisting {
		if s.accountDetailShowsMail() {
			name = firstNonEmptyString(s.accountDisplayName+" Calendar", s.calendarDisplayName, s.calendarEmail, s.caldavUsername, calendarProviderTitle(providerChoice), accountID+" Calendar")
		} else {
			name = firstNonEmptyString(s.accountDisplayName, s.calendarDisplayName, s.calendarEmail, s.caldavUsername, calendarProviderTitle(providerChoice), accountID)
		}
	}
	source := config.SourceConfig{
		ID:          settingsUniqueSourceID(existing, name, "calendar"),
		Kind:        string(models.SourceKindCalendar),
		Provider:    provider,
		DisplayName: strings.TrimSpace(name),
		AccountID:   accountID,
	}
	if s.accountEditMode == settingsAccountEditExisting {
		if hasExistingSource && strings.TrimSpace(existingSource.ID) != "" {
			source.ID = strings.TrimSpace(existingSource.ID)
		}
	}
	switch provider {
	case "caldav":
		source.CalDAV.URL = strings.TrimSpace(s.caldavURL)
		if source.CalDAV.URL == "" {
			if preset, ok := calendarCalDAVPresetForProvider(providerChoice); ok {
				source.CalDAV.URL = preset.URL
			}
		}
		source.CalDAV.Username = strings.TrimSpace(firstNonEmptyString(s.caldavUsername, s.calendarEmail, s.email))
		source.CalDAV.Password = s.caldavPassword
	default:
		source.Google.Email = strings.TrimSpace(firstNonEmptyString(s.calendarEmail, s.email))
		source.Google.AccessToken = s.cfg.Gmail.AccessToken
		source.Google.RefreshToken = s.cfg.Gmail.RefreshToken
		source.Google.TokenExpiry = s.cfg.Gmail.TokenExpiry
		if hasExistingSource {
			source.Google = existingSource.Google
			source.Google.Email = strings.TrimSpace(firstNonEmptyString(s.calendarEmail, s.email))
		}
	}
	return source
}

func (s *Settings) existingSourceIDForKind(kind models.SourceKind) string {
	source, ok := s.existingSourceForKind(kind)
	if !ok {
		return ""
	}
	return strings.TrimSpace(source.ID)
}

func (s *Settings) existingSourceForKind(kind models.SourceKind) (config.SourceConfig, bool) {
	if s == nil || s.cfg == nil {
		return config.SourceConfig{}, false
	}
	accountID := strings.TrimSpace(s.selectedAccountID)
	if accountID == "" {
		accountID = string(models.DefaultAccountID)
	}
	for _, source := range s.cfg.ExplicitSourcesForEdit() {
		sourceAccountID := strings.TrimSpace(source.AccountID)
		if sourceAccountID == "" {
			sourceAccountID = string(models.DefaultAccountID)
		}
		if sourceAccountID != accountID {
			continue
		}
		sourceKind := models.SourceKind(strings.TrimSpace(source.Kind))
		if sourceKind == "" {
			sourceKind = models.SourceKindMail
		}
		if sourceKind == kind {
			return source, true
		}
	}
	return config.SourceConfig{}, false
}

func settingsUniqueSourceID(existing []config.SourceConfig, base, suffix string) string {
	stem := settingsSlug(base)
	if stem == "" {
		stem = suffix
	}
	if !strings.HasSuffix(stem, "-"+suffix) {
		stem += "-" + suffix
	}
	used := make(map[string]bool, len(existing))
	for _, source := range existing {
		used[source.ID] = true
	}
	if !used[stem] {
		return stem
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", stem, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func settingsSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func syncLegacyMailFieldsForSettings(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, source := range cfg.ExplicitSourcesForEdit() {
		if strings.TrimSpace(source.Kind) != "" && source.Kind != string(models.SourceKindMail) {
			continue
		}
		cfg.Vendor = configVendorForProvider(source.Provider)
		cfg.Credentials = source.Credentials
		cfg.Server = source.IMAP
		cfg.SMTP = source.SMTP
		cfg.Gmail.AccessToken = source.Google.AccessToken
		cfg.Gmail.RefreshToken = source.Google.RefreshToken
		cfg.Gmail.TokenExpiry = source.Google.TokenExpiry
		cfg.Gmail.Email = source.Google.Email
		return
	}
}

func (s *Settings) requiresOllamaModelValidation(candidate *config.Config) bool {
	if candidate == nil || s.disableAIFromWarning {
		return false
	}
	if s.mode == SettingsModeWizard && s.firstRunPreferencesOnly {
		return aicheck.OllamaConfigured(candidate)
	}
	if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionAI {
		return aicheck.RequiresOllamaModelValidation(s.cfg, candidate)
	}
	return false
}

func (s *Settings) requiresCalendarValidation() bool {
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		return s.provider == "gmail-oauth" && !s.firstRunOtherProvider && s.alsoAddCalendar
	}
	if s.mode != SettingsModePanel ||
		s.panelSection != settingsPanelSectionAccount ||
		s.deleteAccount ||
		!s.accountDetailShowsCalendar() {
		return false
	}
	if s.accountEditMode == settingsAccountEditExisting {
		return s.calendarCredentialsChanged()
	}
	return true
}

func (s *Settings) oauthRequiredMsg(candidate *config.Config) (OAuthRequiredMsg, bool) {
	if s == nil || candidate == nil || s.deleteAccount {
		return OAuthRequiredMsg{}, false
	}
	if s.mode == SettingsModePanel && s.panelSection != settingsPanelSectionAccount {
		return OAuthRequiredMsg{}, false
	}
	if s.mode == SettingsModeWizard && s.firstRunPreferencesOnly {
		return OAuthRequiredMsg{}, false
	}
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly && (s.provider != "gmail-oauth" || s.firstRunOtherProvider) {
		return OAuthRequiredMsg{}, false
	}

	validateAccount := s.requiresAccountValidation()
	validateCalendar := s.requiresCalendarValidation()
	if s.accountEditMode == settingsAccountEditExisting && !validateAccount && !validateCalendar {
		return OAuthRequiredMsg{}, false
	}
	includeConfigured := (s.provider == "gmail-oauth" || (s.mode == SettingsModeWizard && s.firstRunAccountOnly && s.provider == "gmail-oauth" && !s.firstRunOtherProvider)) ||
		(s.accountEditMode == settingsAccountEditExisting && (validateAccount || validateCalendar))
	sourceIDs := s.googleOAuthSourceIDsForCurrentAccount(candidate, includeConfigured)
	needsCalendarOAuth := !includeConfigured && len(sourceIDs) > 0
	if !includeConfigured && !needsCalendarOAuth {
		return OAuthRequiredMsg{}, false
	}

	email := strings.TrimSpace(s.email)
	if email == "" && needsCalendarOAuth {
		email = strings.TrimSpace(s.calendarEmail)
	}
	serviceLabel := "Gmail OAuth"
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly && s.provider == "gmail-oauth" && !s.firstRunOtherProvider {
		serviceLabel = "Google account"
	}
	if (!s.accountDetailShowsMail() && s.accountDetailShowsCalendar()) || (!includeConfigured && needsCalendarOAuth) {
		serviceLabel = "Google Calendar OAuth"
	}
	return OAuthRequiredMsg{
		Email:                      email,
		ServiceLabel:               serviceLabel,
		Config:                     candidate,
		ReturnToMenu:               s.mode == SettingsModePanel,
		ReclaimOfflineCacheStorage: s.reclaimOfflineCacheStorage,
		ValidateAccount:            validateAccount,
		ValidateCalendar:           validateCalendar,
		CalendarSourceIDs:          s.calendarSourceIDsForValidation(candidate),
		SourceIDs:                  sourceIDs,
	}, true
}

func (s *Settings) googleOAuthSourceIDsForCurrentAccount(candidate *config.Config, includeConfigured bool) []models.SourceID {
	if s == nil || candidate == nil {
		return nil
	}
	accountID := s.currentAccountIDForSettings()
	var ids []models.SourceID
	for _, source := range candidate.NormalizedSources() {
		if accountID != "" && strings.TrimSpace(source.AccountID) != accountID {
			continue
		}
		if !settingsSourceUsesGoogleOAuth(source) {
			continue
		}
		if includeConfigured || googleOAuthSourceNeedsToken(source) {
			ids = append(ids, settingsSourceIDForSource(source))
		}
	}
	return ids
}

func settingsSourceIDForSource(source config.SourceConfig) models.SourceID {
	kind := models.SourceKind(strings.TrimSpace(source.Kind))
	if kind == "" {
		kind = models.SourceKindMail
	}
	return models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultSourceIDForKind(kind))
}

func settingsSourceUsesGoogleOAuth(source config.SourceConfig) bool {
	kind := strings.TrimSpace(source.Kind)
	switch kind {
	case "", string(models.SourceKindMail):
		return settingsMailSourceUsesGoogleOAuth(source)
	case string(models.SourceKindCalendar):
		return strings.TrimSpace(source.Provider) == "google_calendar"
	default:
		return false
	}
}

func settingsMailSourceUsesGoogleOAuth(source config.SourceConfig) bool {
	kind := strings.TrimSpace(source.Kind)
	if kind != "" && kind != string(models.SourceKindMail) {
		return false
	}
	provider := strings.TrimSpace(source.Provider)
	if provider == "gmail_api" {
		return true
	}
	if provider == "gmail" && strings.TrimSpace(source.Google.Email) != "" {
		return true
	}
	return strings.TrimSpace(source.Google.RefreshToken) != "" || strings.TrimSpace(source.Google.AccessToken) != ""
}

func googleOAuthSourceNeedsToken(source config.SourceConfig) bool {
	return strings.TrimSpace(source.Google.AccessToken) == "" &&
		strings.TrimSpace(source.Google.RefreshToken) == ""
}

func (s *Settings) calendarSourceIDsForValidation(candidate *config.Config) []models.SourceID {
	if !s.requiresCalendarValidation() || candidate == nil {
		return nil
	}
	accountID := s.currentAccountIDForSettings()
	var ids []models.SourceID
	for _, source := range candidate.NormalizedSources() {
		if strings.TrimSpace(source.Kind) != string(models.SourceKindCalendar) {
			continue
		}
		if accountID != "" && strings.TrimSpace(source.AccountID) != accountID {
			continue
		}
		ids = append(ids, models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID))
	}
	return ids
}

func (s *Settings) currentAccountIDForSettings() string {
	if s == nil {
		return ""
	}
	accountID := strings.TrimSpace(s.selectedAccountID)
	if accountID == "" {
		accountID = settingsSlug(firstNonEmptyString(s.accountDisplayName, s.email, s.calendarEmail, s.caldavUsername, "account"))
	}
	return accountID
}

func (s *Settings) hasAIModelWarning() bool {
	return s.aiModelWarning != nil && !s.aiModelWarning.OK()
}

func (s *Settings) aiModelWarningDescription() string {
	if !s.hasAIModelWarning() {
		return ""
	}
	return s.aiModelWarning.CompactMessage() + "\nUse Disable AI to save this config with local AI features off."
}

func cloneThemeOverrides(overrides map[string]config.ThemeOverride) map[string]config.ThemeOverride {
	if len(overrides) == 0 {
		return make(map[string]config.ThemeOverride)
	}
	clone := make(map[string]config.ThemeOverride, len(overrides))
	for key, value := range overrides {
		clone[key] = value
	}
	return clone
}

func (s *Settings) applyThemeFileActions() error {
	if path := strings.TrimSpace(s.themeInstallPath); path != "" && (s.mode != SettingsModePanel || isThemeSelectionSection(s.panelSection)) {
		expanded, err := config.ExpandPath(path)
		if err != nil {
			return err
		}
		installed, err := InstallThemeFile(expanded, DefaultThemeDir())
		if err != nil {
			return err
		}
		doc, err := LoadThemeFromFile(installed)
		if err != nil {
			return err
		}
		s.themeName = doc.Name
	}
	if slug := strings.TrimSpace(s.themeSaveAs); slug != "" && (s.mode != SettingsModePanel || isThemeEditorSection(s.panelSection)) {
		cfg := s.buildConfig()
		doc := ThemeDocument{
			Version:  1,
			Name:     slug,
			Inherits: cfg.Theme.Name,
			Roles:    cfg.Theme.Overrides,
		}
		if _, err := SaveThemeDocument(doc, DefaultThemeDir()); err != nil {
			return err
		}
		s.themeName = slug
	}
	return nil
}

func configVendorForProvider(provider string) string {
	switch provider {
	case "gmail", "gmail-oauth", "gmail_api":
		return "gmail"
	default:
		return provider
	}
}

func mailSourceProviderForSettings(provider string) string {
	if provider == "gmail-oauth" {
		return "gmail"
	}
	return configVendorForProvider(provider)
}

func configAIProvider(provider string) string {
	switch provider {
	case aiProviderOllamaDefault, aiProviderOllamaCustom:
		return "ollama"
	case aiProviderDisabled:
		return aiProviderDisabled
	default:
		return provider
	}
}

func (s *Settings) advancedAISetup() bool {
	return strings.TrimSpace(s.aiSetupPreset) == aiSetupPresetAdvancedManual
}

func (s *Settings) inferAISetupPreset() string {
	if s == nil {
		return aiSetupPresetOllamaDefault
	}
	chatProvider := configAIProvider(s.aiProvider)
	embeddingProvider := s.effectiveSettingsEmbeddingProvider()
	switch chatProvider {
	case aiProviderDisabled:
		return aiSetupPresetDisabled
	case "openai":
		if embeddingProvider != config.EmbeddingProviderOpenAI {
			return aiSetupPresetAdvancedManual
		}
		if strings.TrimSpace(s.openAIBaseURL) != "" && strings.TrimSpace(s.openAIBaseURL) != defaultOpenAIBaseURL {
			return aiSetupPresetOpenAICompatible
		}
		return aiSetupPresetOpenAIDefault
	case "claude":
		if embeddingProvider != config.EmbeddingProviderOllama {
			return aiSetupPresetAdvancedManual
		}
		return aiSetupPresetClaudeDefault
	case "ollama":
		if embeddingProvider != config.EmbeddingProviderOllama {
			return aiSetupPresetAdvancedManual
		}
		if s.aiProvider == aiProviderOllamaCustom {
			return aiSetupPresetOllamaCustom
		}
		if s.hasCustomOllamaValues() {
			return aiSetupPresetOllamaCustom
		}
		return aiSetupPresetOllamaDefault
	default:
		return aiSetupPresetAdvancedManual
	}
}

func (s *Settings) aiSetupPresetMatchesRoles() bool {
	if s == nil {
		return false
	}
	preset := strings.TrimSpace(s.aiSetupPreset)
	if preset == "" {
		return false
	}
	if preset == aiSetupPresetAdvancedManual {
		return true
	}
	return preset == s.inferAISetupPreset()
}

func (s *Settings) refreshAISetupPresetFromRoles() {
	if s == nil || s.advancedAISetup() {
		return
	}
	if !s.aiSetupPresetMatchesRoles() {
		s.aiSetupPreset = s.inferAISetupPreset()
	}
}

func (s *Settings) applyAISetupPreset() {
	if s == nil {
		return
	}
	switch strings.TrimSpace(s.aiSetupPreset) {
	case aiSetupPresetOllamaDefault:
		s.aiProvider = aiProviderOllamaDefault
		s.embeddingProvider = config.EmbeddingProviderOllama
		s.ollamaHost = defaultOllamaHost
		s.ollamaModel = defaultOllamaModel
		s.embedModel = defaultEmbeddingModel
	case aiSetupPresetOllamaCustom:
		s.aiProvider = aiProviderOllamaCustom
		s.embeddingProvider = config.EmbeddingProviderOllama
		if strings.TrimSpace(s.ollamaHost) == "" {
			s.ollamaHost = defaultOllamaHost
		}
	case aiSetupPresetOpenAIDefault:
		s.aiProvider = "openai"
		s.embeddingProvider = config.EmbeddingProviderOpenAI
		s.openAIBaseURL = defaultOpenAIBaseURL
		if strings.TrimSpace(s.openAIModel) == "" {
			s.openAIModel = defaultOpenAIModel
		}
		if strings.TrimSpace(s.openAIEmbeddingModel) == "" {
			s.openAIEmbeddingModel = defaultOpenAIEmbed
		}
	case aiSetupPresetOpenAICompatible:
		s.aiProvider = "openai"
		s.embeddingProvider = config.EmbeddingProviderOpenAI
		if strings.TrimSpace(s.openAIBaseURL) == "" {
			s.openAIBaseURL = defaultOpenAIBaseURL
		}
		if strings.TrimSpace(s.openAIModel) == "" {
			s.openAIModel = defaultOpenAIModel
		}
		if strings.TrimSpace(s.openAIEmbeddingModel) == "" {
			s.openAIEmbeddingModel = defaultOpenAIEmbed
		}
	case aiSetupPresetClaudeDefault:
		s.aiProvider = "claude"
		s.embeddingProvider = config.EmbeddingProviderOllama
		if strings.TrimSpace(s.claudeModel) == "" {
			s.claudeModel = defaultClaudeModel
		}
		if strings.TrimSpace(s.ollamaHost) == "" {
			s.ollamaHost = defaultOllamaHost
		}
		if strings.TrimSpace(s.embedModel) == "" {
			s.embedModel = defaultEmbeddingModel
		}
	case aiSetupPresetDisabled:
		s.aiProvider = aiProviderDisabled
	case aiSetupPresetAdvancedManual:
		if strings.TrimSpace(s.aiProvider) == "" {
			s.aiProvider = aiProviderOllamaDefault
		}
		if strings.TrimSpace(s.embeddingProvider) == "" {
			s.embeddingProvider = s.effectiveSettingsEmbeddingProvider()
		}
	default:
		s.aiSetupPreset = s.inferAISetupPreset()
	}
	s.syncAIDefaults()
	_ = s.syncExternalAIDefaults()
	s.syncAIModelChoicesFromValues()
}

func (s *Settings) normalizeAIProvider() {
	switch s.aiProvider {
	case "":
		if s.hasCustomOllamaValues() {
			s.aiProvider = aiProviderOllamaCustom
		} else {
			s.aiProvider = aiProviderOllamaDefault
		}
	case "ollama":
		if s.hasCustomOllamaValues() {
			s.aiProvider = aiProviderOllamaCustom
		} else {
			s.aiProvider = aiProviderOllamaDefault
		}
	}
	s.syncAIDefaults()
}

func (s *Settings) hasCustomOllamaValues() bool {
	if s.ollamaHost != "" && s.ollamaHost != defaultOllamaHost {
		return true
	}
	if s.ollamaModel != "" && s.ollamaModel != defaultOllamaModel {
		return true
	}
	if s.embedModel != "" && s.embedModel != defaultEmbeddingModel {
		return true
	}
	return false
}

func (s *Settings) syncAIDefaults() {
	if s.aiProvider != aiProviderOllamaDefault && s.aiProvider != aiProviderOllamaCustom {
		return
	}
	if s.ollamaHost == "" || s.aiProvider == aiProviderOllamaDefault {
		s.ollamaHost = defaultOllamaHost
	}
	if s.aiProvider == aiProviderOllamaDefault {
		s.ollamaModel = defaultOllamaModel
		s.embedModel = defaultEmbeddingModel
		s.ollamaModelChoice = defaultOllamaModel
		s.embedModelChoice = defaultEmbeddingModel
		s.ollamaModelCustom = ""
		s.embedModelCustom = ""
		return
	}
	if s.ollamaModelChoice == "" || s.embedModelChoice == "" {
		s.syncAIModelChoicesFromValues()
	}
	s.ollamaModel = selectedModelValue(s.ollamaModelChoice, s.ollamaModelCustom, s.ollamaModel, defaultOllamaModel)
	s.embedModel = selectedModelValue(s.embedModelChoice, s.embedModelCustom, s.embedModel, defaultEmbeddingModel)
}

func (s *Settings) syncExternalAIDefaults() bool {
	before := []string{
		s.openAIBaseURL,
		s.openAIModel,
		s.openAIEmbeddingModel,
		s.embeddingProvider,
		s.ollamaHost,
		s.embedModel,
		s.embedModelChoice,
		s.lastAIProvider,
	}
	advanced := s.advancedAISetup()
	if s.openAIBaseURL == "" {
		s.openAIBaseURL = defaultOpenAIBaseURL
	}
	if s.openAIModel == "" {
		s.openAIModel = defaultOpenAIModel
	}
	if s.openAIEmbeddingModel == "" {
		s.openAIEmbeddingModel = defaultOpenAIEmbed
	}
	if s.embeddingProvider == "" {
		if s.aiProvider == "openai" {
			s.embeddingProvider = config.EmbeddingProviderOpenAI
		} else {
			s.embeddingProvider = config.EmbeddingProviderOllama
		}
	} else {
		s.embeddingProvider = config.NormalizeEmbeddingProvider(s.embeddingProvider)
	}
	if !advanced && s.lastAIProvider != "" && s.lastAIProvider != s.aiProvider {
		if s.aiProvider == "openai" && s.embeddingProvider == config.EmbeddingProviderOllama {
			s.embeddingProvider = config.EmbeddingProviderOpenAI
		}
		if s.aiProvider == aiProviderOllamaDefault || s.aiProvider == aiProviderOllamaCustom {
			s.embeddingProvider = config.EmbeddingProviderOllama
		}
		s.lastAIProvider = s.aiProvider
	} else if s.lastAIProvider == "" {
		s.lastAIProvider = s.aiProvider
	}
	if s.effectiveSettingsEmbeddingProvider() == config.EmbeddingProviderOllama {
		if s.ollamaHost == "" {
			s.ollamaHost = defaultOllamaHost
		}
		if s.embedModel == "" {
			s.embedModel = defaultEmbeddingModel
		}
		if s.embedModelChoice == "" {
			s.syncAIModelChoicesFromValues()
		}
		s.embedModel = selectedModelValue(s.embedModelChoice, s.embedModelCustom, s.embedModel, defaultEmbeddingModel)
	}
	after := []string{
		s.openAIBaseURL,
		s.openAIModel,
		s.openAIEmbeddingModel,
		s.embeddingProvider,
		s.ollamaHost,
		s.embedModel,
		s.embedModelChoice,
		s.lastAIProvider,
	}
	for i := range before {
		if before[i] != after[i] {
			return true
		}
	}
	return false
}

func (s *Settings) effectiveSettingsEmbeddingProvider() string {
	provider := strings.TrimSpace(s.embeddingProvider)
	if provider != "" {
		return config.NormalizeEmbeddingProvider(provider)
	}
	if s.aiProvider == "openai" {
		return config.EmbeddingProviderOpenAI
	}
	return config.EmbeddingProviderOllama
}

func (s *Settings) ensureProviderDefaults() bool {
	before := []string{s.imapHost, s.imapPort, s.smtpHost, s.smtpPort}
	s.syncProviderDefaults("", s.provider)
	if s.provider == "gmail" {
		s.syncProviderDefaults("", "gmail")
	}
	after := []string{s.imapHost, s.imapPort, s.smtpHost, s.smtpPort}
	for i := range before {
		if before[i] != after[i] {
			return true
		}
	}
	return false
}

func (s *Settings) refreshFormPreservingVisibleGroup(target string) {
	if target == "" || s.form == nil || s.form.State != huh.StateNormal {
		return
	}
	s.buildForm()
	for i := 0; i < 20; i++ {
		if strings.Contains(s.form.View(), target) {
			return
		}
		s.consumeFormNavigationCmd(s.form.NextGroup(), 0)
	}
}

func (s *Settings) visibleSettingsGroupTarget(view string) string {
	for _, target := range []string{
		"Email address>",
		"Gmail address>",
		"Google address",
		"IMAP Host>",
		"AI Setup",
		"Chat Role",
		"OpenAI API Key",
		"OpenAI Model",
		"Embedding Role",
		"OpenAI Embedding Model",
		"Ollama Embedding Model",
		"Offline Cache",
		"Sync & Cleanup",
		"Keyboard Profile",
		"Current Theme",
		"Email Signature",
		"Settings",
	} {
		if strings.Contains(view, target) {
			return target
		}
	}
	return ""
}

func (s *Settings) needsPresetFieldRefresh(view string) bool {
	if !strings.Contains(view, "IMAP Host>") {
		return false
	}
	imapHost, imapPort, smtpHost, smtpPort, ok := providerPresetValues(s.provider)
	if !ok {
		return false
	}
	for _, want := range []string{imapHost, imapPort, smtpHost, smtpPort} {
		if want != "" && !strings.Contains(view, want) {
			return true
		}
	}
	return false
}

func (s *Settings) firstRunProviderDetailTarget() string {
	if s.mode != SettingsModeWizard || !s.firstRunAccountOnly {
		return ""
	}
	switch s.provider {
	case "gmail-oauth":
		if !s.firstRunOtherProvider {
			return "Google address"
		}
	case "gmail":
		return "Gmail address>"
	default:
		return "Email address>"
	}
	return ""
}

func (s *Settings) needsFirstRunProviderDetailRefresh(view string) bool {
	target := s.firstRunProviderDetailTarget()
	if target == "" || strings.Contains(view, "Account Type") {
		return false
	}
	if strings.Contains(view, target) {
		return false
	}
	switch s.provider {
	case "gmail-oauth":
		return !s.firstRunOtherProvider
	case "gmail":
		return strings.Contains(view, "Google account") || strings.Contains(view, "Google address")
	default:
		return strings.Contains(view, "Google account") ||
			strings.Contains(view, "Google address") ||
			strings.Contains(view, "Gmail address>")
	}
}

func (s *Settings) refreshFirstRunProviderDetailGroup() {
	s.buildForm()
	s.consumeFormNavigationCmd(s.form.NextGroup(), 0)
}

func providerPresetValues(provider string) (imapHost, imapPort, smtpHost, smtpPort string, ok bool) {
	vendor := configVendorForProvider(provider)
	if vendor == "" || vendor == "imap" {
		return "", "", "", "", false
	}
	cfg := &config.Config{}
	cfg.Vendor = vendor
	cfg.ApplyVendorPreset()
	if cfg.Server.Host == "" {
		return "", "", "", "", false
	}
	return cfg.Server.Host, portToString(cfg.Server.Port), cfg.SMTP.Host, portToString(cfg.SMTP.Port), true
}

func providerPresetPlaceholder(provider string, part string) string {
	imapHost, imapPort, smtpHost, smtpPort, ok := providerPresetValues(provider)
	if !ok {
		return ""
	}
	switch part {
	case "imap-host":
		return imapHost
	case "imap-port":
		return imapPort
	case "smtp-host":
		return smtpHost
	case "smtp-port":
		return smtpPort
	default:
		return ""
	}
}

func providerPresetSummary(provider string) string {
	imapHost, imapPort, smtpHost, smtpPort, ok := providerPresetValues(provider)
	if !ok {
		return ""
	}
	return fmt.Sprintf(" Defaults: IMAP %s:%s, SMTP %s:%s.", imapHost, imapPort, smtpHost, smtpPort)
}

func (s *Settings) syncProviderDefaults(oldProvider, newProvider string) {
	if newProvider != "gmail" {
		s.editGmailAdvanced = false
	}

	oldIMAPHost, oldIMAPPort, oldSMTPHost, oldSMTPPort, oldOK := providerPresetValues(oldProvider)
	gmailIMAPHost, gmailIMAPPort, gmailSMTPHost, gmailSMTPPort, gmailOK := providerPresetValues("gmail")
	staleGmailPreset := gmailOK &&
		s.imapHost == gmailIMAPHost &&
		s.imapPort == gmailIMAPPort &&
		s.smtpHost == gmailSMTPHost &&
		s.smtpPort == gmailSMTPPort
	matchesOldOrStaleGmail := func(current, oldValue, gmailValue string) bool {
		if oldOK && current == oldValue {
			return true
		}
		return staleGmailPreset && configVendorForProvider(newProvider) != "gmail" && current == gmailValue
	}
	newIMAPHost, newIMAPPort, newSMTPHost, newSMTPPort, newOK := providerPresetValues(newProvider)
	if !newOK {
		if configVendorForProvider(newProvider) == "imap" {
			if matchesOldOrStaleGmail(s.imapHost, oldIMAPHost, gmailIMAPHost) {
				s.imapHost = ""
			}
			if matchesOldOrStaleGmail(s.imapPort, oldIMAPPort, gmailIMAPPort) {
				s.imapPort = ""
			}
			if matchesOldOrStaleGmail(s.smtpHost, oldSMTPHost, gmailSMTPHost) {
				s.smtpHost = ""
			}
			if matchesOldOrStaleGmail(s.smtpPort, oldSMTPPort, gmailSMTPPort) {
				s.smtpPort = ""
			}
		}
		return
	}

	if s.imapHost == "" || matchesOldOrStaleGmail(s.imapHost, oldIMAPHost, gmailIMAPHost) {
		s.imapHost = newIMAPHost
	}
	if s.imapPort == "" || matchesOldOrStaleGmail(s.imapPort, oldIMAPPort, gmailIMAPPort) {
		s.imapPort = newIMAPPort
	}
	if s.smtpHost == "" || matchesOldOrStaleGmail(s.smtpHost, oldSMTPHost, gmailSMTPHost) {
		s.smtpHost = newSMTPHost
	}
	if s.smtpPort == "" || matchesOldOrStaleGmail(s.smtpPort, oldSMTPPort, gmailSMTPPort) {
		s.smtpPort = newSMTPPort
	}
}

func (s *Settings) renderWizardSummary(width int) string {
	if width <= 0 {
		width = 80
	}

	var lines []string
	for _, line := range s.wizardSummaryLines() {
		lines = append(lines, render.WrapLines(line, width)...)
	}
	return strings.Join(lines, "\n")
}

func (s *Settings) wizardSummaryLines() []string {
	if s.firstRunPreferencesOnly {
		return []string{
			wizardSummaryLine("Account:", "verified. Enter Herald now or customize optional preferences."),
			wizardSummaryLine("Defaults:", "AI off, message bodies without attachments, Default keyboard, inherited theme, no signature."),
		}
	}
	if s.firstRunAccountOnly && s.provider == "gmail-oauth" && !s.firstRunOtherProvider {
		return []string{
			wizardSummaryLine("Recommended:", "Google account in a browser."),
			wizardSummaryLine("Includes:", "Gmail now and Google Calendar by default."),
			wizardSummaryLine("Next:", "choose an account type, then verify access."),
		}
	}
	switch s.provider {
	case "gmail":
		return []string{
			wizardSummaryLine("Fallback:", "Gmail via IMAP + App Password."),
			wizardSummaryLine("Defaults:", "imap.gmail.com:993 and smtp.gmail.com:587."),
			wizardSummaryLine("Workspace:", "some accounts require OAuth instead of app passwords."),
			wizardSummaryDoc("App passwords", "https://myaccount.google.com/apppasswords"),
			wizardSummaryDoc("Add Gmail to another client", "https://support.google.com/mail/answer/75726?hl=en"),
			wizardSummaryDoc("Workspace IMAP setup", "https://knowledge.workspace.google.com/admin/sync/set-up-gmail-with-a-third-party-email-client"),
		}
	case "gmail-oauth":
		return []string{
			wizardSummaryLine("Recommended:", "Gmail OAuth in a browser."),
			wizardSummaryLine("Validates:", "Gmail IMAP and SMTP XOAUTH2 before saving."),
			wizardSummaryLine("Best with:", "Homebrew or release binaries, which include OAuth defaults."),
			wizardSummaryLine("Source builds:", "use .herald-dev.env or set HERALD_GOOGLE_CLIENT_ID and HERALD_GOOGLE_CLIENT_SECRET."),
		}
	case "protonmail":
		return []string{
			wizardSummaryLine("IMAP preset:", "requires ProtonMail Bridge running locally."),
			wizardSummaryLine("Defaults:", "IMAP 127.0.0.1:1143 and SMTP 127.0.0.1:1025 are prefilled."),
		}
	case "fastmail", "icloud", "outlook":
		return []string{
			wizardSummaryLine("IMAP preset:", "Herald prefills the known IMAP and SMTP defaults."),
			wizardSummaryLine("Credentials:", "use the provider password or app password required by your account."),
		}
	default:
		return []string{
			wizardSummaryLine("Recommended:", "Google account for the fastest setup."),
			wizardSummaryLine("Supported:", "Custom IMAP, Gmail App Password, ProtonMail Bridge, Fastmail, iCloud, Outlook."),
		}
	}
}

func wizardSummaryLine(label, body string) string {
	labelStyle := defaultTheme.Setup.SummaryLabel.Style()
	bodyStyle := defaultTheme.Setup.SummaryBody.Style()
	return labelStyle.Render(label) + " " + bodyStyle.Render(body)
}

func wizardSummaryDoc(label, rawURL string) string {
	linkStyle := defaultTheme.Setup.Link.Style()
	textStyle := defaultTheme.Setup.SummaryBody.Style()
	return wizardHyperlink(linkStyle.Render("[click]"), rawURL) + " " + textStyle.Render(label)
}

func wizardHyperlink(label, rawURL string) string {
	return "\033]8;;" + rawURL + "\033\\" + label + "\033]8;;\033\\"
}

// applyVendorPreset fills in server/smtp host+port when a vendor shortcut is
// set and the user has not provided explicit values.
func applyVendorPreset(cfg *config.Config) {
	cfg.ApplyVendorPreset()
}

// validateEmail checks that a string contains an @ sign.
func validateEmail(s string) error {
	if s == "" {
		return fmt.Errorf("email address is required")
	}
	if !strings.Contains(s, "@") {
		return fmt.Errorf("must be a valid email address")
	}
	return nil
}

// portToString converts a port int to a string, returning "" for zero.
func portToString(port int) string {
	if port == 0 {
		return ""
	}
	return strconv.Itoa(port)
}

// parsePort converts a port string to an int, returning 0 for empty/invalid.
func parsePort(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return p
}
