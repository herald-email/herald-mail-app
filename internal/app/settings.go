package app

import (
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
	"github.com/herald-email/herald-mail-app/internal/config"
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

	defaultOllamaHost     = "http://localhost:11434"
	defaultOllamaModel    = "llama3.2:1b"
	defaultEmbeddingModel = "nomic-embed-text"
	customModelChoice     = "custom"

	settingsPanelSectionMenu     settingsPanelSection = ""
	settingsPanelSectionAccount  settingsPanelSection = "account"
	settingsPanelSectionAI       settingsPanelSection = "ai"
	settingsPanelSectionSync     settingsPanelSection = "sync-cleanup"
	settingsPanelSectionKeyboard settingsPanelSection = "keyboard"
	settingsPanelSectionTheme    settingsPanelSection = "theme"
	settingsPanelSectionSign     settingsPanelSection = "signature"

	settingsThemeForegroundKey       = "theme.foreground"
	settingsThemeForegroundPickerKey = "theme.foreground_picker"
	settingsThemeBackgroundKey       = "theme.background"
	settingsThemeBackgroundPickerKey = "theme.background_picker"
)

// SettingsSavedMsg is sent when the user completes the form and saves.
type SettingsSavedMsg struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	ValidateAccount            bool
}

// SettingsCancelledMsg is sent when the user presses esc in panel mode.
type SettingsCancelledMsg struct{}

// OAuthRequiredMsg is sent when Gmail is selected and OAuth flow needs to run.
type OAuthRequiredMsg struct {
	Email  string
	Config *config.Config // partially-built config with vendor presets applied
}

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
	panelSection                  settingsPanelSection
	panelMenuChoice               string
	panelStatus                   string

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
	aiProvider        string
	claudeAPIKey      string
	claudeModel       string
	openAIAPIKey      string
	openAIBaseURL     string
	openAIModel       string
	ollamaHost        string
	ollamaModel       string
	ollamaModelChoice string
	ollamaModelCustom string
	embedModel        string
	embedModelChoice  string
	embedModelCustom  string

	// form field backing variables — sync & cleanup
	syncPollStr                string
	syncIDLE                   bool
	cleanupScheduleStr         string
	cacheStoragePolicy         string
	reclaimOfflineCacheStorage bool

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

func (f *conditionalSettingsField) hidden() bool {
	return f != nil && f.hide != nil && f.hide()
}

func (f *conditionalSettingsField) Init() tea.Cmd {
	return f.field.Init()
}

func (f *conditionalSettingsField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
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
		panelMenuChoice:               string(settingsPanelSectionAccount),
		showExperimentalEmailServices: opts.ShowExperimentalEmailServices,
		firstRunAccountOnly:           opts.FirstRunAccountOnly,
		firstRunPreferencesOnly:       opts.FirstRunPreferencesOnly,
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
		s.embedModel = existing.EffectiveEmbeddingModel()
		s.claudeAPIKey = existing.Claude.APIKey
		s.claudeModel = existing.Claude.Model
		s.openAIAPIKey = existing.OpenAI.APIKey
		s.openAIBaseURL = existing.OpenAI.BaseURL
		s.openAIModel = existing.OpenAI.Model

		// Sync & cleanup fields
		s.syncPollStr = strconv.Itoa(existing.Sync.PollIntervalMinutes)
		s.syncIDLE = existing.Sync.IDLEEnabled
		s.cleanupScheduleStr = strconv.Itoa(existing.Cleanup.ScheduleHours)
		s.cacheStoragePolicy = config.NormalizeCacheStoragePolicy(existing.Cache.StoragePolicy)
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

	// Default provider to "imap" if empty.
	if s.provider == "" {
		s.provider = "imap"
	}
	s.normalizeAIProvider()
	if s.syncPollStr == "" {
		s.syncPollStr = "5" // default only on first run; 0 is valid (IDLE-only mode)
	}
	if s.cleanupScheduleStr == "" {
		s.cleanupScheduleStr = "0"
	}
	if s.cacheStoragePolicy == "" {
		s.cacheStoragePolicy = config.CacheStoragePolicyNoAttachments
	}
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
		ShowExperimentalEmailServices: mode == SettingsModePanel,
	}
}

func (s *Settings) accountTypeDescription() string {
	if s.mode == SettingsModePanel {
		return "Recommended: Gmail OAuth. Supported: Standard IMAP and Gmail App Password. Experimental: ProtonMail Bridge, Fastmail, iCloud, Outlook."
	}
	if s.showExperimentalEmailServices {
		return "Experimental: Gmail OAuth."
	}
	return "OAuth onboarding is hidden unless -experimental is set."
}

func (s *Settings) accountTypeOptions() []huh.Option[string] {
	if s.mode == SettingsModePanel {
		return []huh.Option[string]{
			huh.NewOption("Standard IMAP", "imap"),
			huh.NewOption("Gmail OAuth", "gmail-oauth"),
			huh.NewOption("Gmail (IMAP + App Password)", "gmail"),
			huh.NewOption("ProtonMail Bridge (Experimental)", "protonmail"),
			huh.NewOption("Fastmail (Experimental)", "fastmail"),
			huh.NewOption("iCloud (Experimental)", "icloud"),
			huh.NewOption("Outlook (Experimental)", "outlook"),
		}
	}

	options := []huh.Option[string]{
		huh.NewOption("Standard IMAP", "imap"),
		huh.NewOption("Gmail (IMAP + App Password)", "gmail"),
	}
	if s.showExperimentalEmailServices {
		options = append(options, huh.NewOption("Gmail OAuth (Experimental)", "gmail-oauth"))
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
		huh.NewOption("llama3.2:1b - safe default (~1.3GB)", "llama3.2:1b"),
		huh.NewOption("qwen3.5:0.8b - smallest curated option (~1.0GB)", "qwen3.5:0.8b"),
		huh.NewOption("llama3.2:3b - stronger text, larger (~2.0GB)", "llama3.2:3b"),
		huh.NewOption("gemma3:4b - larger/vision-capable (~3.3GB)", "gemma3:4b"),
		huh.NewOption("Custom model name", customModelChoice),
	}
}

func ollamaEmbeddingModelOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("nomic-embed-text - safe default (~274MB, 2K context)", "nomic-embed-text"),
		huh.NewOption("all-minilm - smallest embeddings (~46MB)", "all-minilm"),
		huh.NewOption("nomic-embed-text-v2-moe - multilingual, larger (~958MB)", "nomic-embed-text-v2-moe"),
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

func (s *Settings) syncAIModelChoicesFromValues() {
	s.ollamaModelChoice, s.ollamaModelCustom = modelChoiceForValue(s.ollamaModel, ollamaChatModelOptions(), defaultOllamaModel)
	s.embedModelChoice, s.embedModelCustom = modelChoiceForValue(s.embedModel, ollamaEmbeddingModelOptions(), defaultEmbeddingModel)
}

func (s *Settings) requiresAccountValidation() bool {
	if s.mode == SettingsModeWizard {
		return !s.firstRunPreferencesOnly
	}
	return s.panelSection == settingsPanelSectionAccount
}

// buildForm constructs the huh.Form with groups for account, AI provider, and sync preferences.
func (s *Settings) buildForm() {
	if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionMenu {
		s.buildPanelMenuForm()
		return
	}

	// Group 1 — Account type selection
	accountGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Account Type").
			Description(s.accountTypeDescription()).
			Options(s.accountTypeOptions()...).
			Value(&s.provider),
	)

	credentialsIntro := huh.NewNote().
		TitleFunc(func() string {
			if s.provider == "imap" {
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
	).WithHideFunc(func() bool { return s.provider == "gmail" || s.provider == "gmail-oauth" })

	// Group 2b — Gmail IMAP fallback guidance and credentials
	gmailGroup := huh.NewGroup(
		huh.NewNote().
			Title("Personal Gmail via IMAP").
			Description("Normal Gmail setup. Use your Gmail address and a Google App Password. Google Workspace accounts may still require OAuth."),
		huh.NewInput().Title("Gmail address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
		huh.NewInput().Title("App Password").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.password),
		huh.NewConfirm().
			Title("Edit advanced Gmail server settings").
			Value(&s.editGmailAdvanced),
	).WithHideFunc(func() bool { return s.provider != "gmail" })

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
	).WithHideFunc(func() bool { return s.provider != "gmail" || !s.editGmailAdvanced })

	// Group 2c — Gmail OAuth notice
	gmailOAuthGroup := huh.NewGroup(
		huh.NewNote().
			Title("Gmail OAuth").
			Description("Experimental browser-based Gmail setup. Source builds need Google OAuth env vars or make build-release-local."),
		huh.NewInput().Title("Gmail address").Inline(true).Value(&s.email).Validate(s.validateSetupEmail),
	).WithHideFunc(func() bool { return s.provider != "gmail-oauth" })

	// Group 3 — AI provider selection
	aiProviderGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("AI Provider").
			Options(
				huh.NewOption("Ollama (local default)", aiProviderOllamaDefault),
				huh.NewOption("Ollama (local custom)", aiProviderOllamaCustom),
				huh.NewOption("Claude API", "claude"),
				huh.NewOption("OpenAI / compatible", "openai"),
				huh.NewOption("AI features disabled", aiProviderDisabled),
			).
			Value(&s.aiProvider),
	).Title("AI Provider")

	ollamaDefaultGroup := huh.NewGroup(
		huh.NewNote().
			Title("Ollama local default").
			Description("Uses http://localhost:11434 with llama3.2:1b and nomic-embed-text. On 8GB Macs, larger models can pressure memory; use custom Ollama for smaller/larger local models or choose an external AI provider."),
	).WithHideFunc(func() bool { return s.aiProvider != aiProviderOllamaDefault })

	// Group 3a — Ollama settings (shown only when provider = custom Ollama)
	ollamaGroup := huh.NewGroup(
		huh.NewInput().Title("Ollama Host").Inline(true).Value(&s.ollamaHost).Placeholder(defaultOllamaHost),
		huh.NewNote().
			Title("Model recommendations").
			Description("Chat: llama3.2:1b, qwen3.5:0.8b, llama3.2:3b, gemma3:4b, Custom model name.\nEmbeddings: nomic-embed-text, all-minilm, nomic-embed-text-v2-moe, mxbai-embed-large, bge-m3, Custom model name."),
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
	).WithHideFunc(func() bool { return s.aiProvider != aiProviderOllamaCustom })

	// Group 3b — Claude settings (shown only when provider = claude)
	claudeGroup := huh.NewGroup(
		huh.NewInput().Title("Claude API Key").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.claudeAPIKey),
		huh.NewInput().Title("Claude Model").Inline(true).Placeholder("claude-sonnet-4-6").Value(&s.claudeModel),
	).WithHideFunc(func() bool { return s.aiProvider != "claude" })

	// Group 3c — OpenAI settings (shown only when provider = openai)
	openAIGroup := huh.NewGroup(
		huh.NewInput().Title("OpenAI API Key").Inline(true).EchoMode(huh.EchoModePassword).Value(&s.openAIAPIKey),
		huh.NewInput().Title("OpenAI Base URL").Inline(true).Placeholder("https://api.openai.com/v1").Value(&s.openAIBaseURL),
		huh.NewInput().Title("OpenAI Model").Inline(true).Placeholder("gpt-4o").Value(&s.openAIModel),
	).WithHideFunc(func() bool { return s.aiProvider != "openai" })

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

	composeGroup := huh.NewGroup(
		huh.NewText().
			Title("Email Signature").
			Description("Enter adds a line. Tab moves to Save Settings. Empty disables.").
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

	themeGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Current Theme").
			Options(settingsThemeOptions()...).
			Value(&s.themeName),
		huh.NewInput().
			Title("Install local theme YAML").
			Description("Validated and copied into ~/.herald/themes. Leave blank to skip install.").
			Placeholder("~/Downloads/quiet-slate.yaml").
			Value(&s.themeInstallPath),
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
	).Title("Theme")

	panelSignatureGroup := huh.NewGroup(
		huh.NewText().
			Title("Email Signature").
			Description("Enter adds a line. Tab moves to Save. Empty disables.").
			Placeholder("-- \nYour Name").
			Lines(5).
			Value(&s.signatureText),
		huh.NewConfirm().
			Title("Save changes").
			Affirmative("Save").
			Negative("Cancel").
			Value(&s.saveButton),
	).Title("Signature")

	saveGroup := huh.NewGroup(
		huh.NewConfirm().
			Title("Save changes").
			Affirmative("Save").
			Negative("Cancel").
			Value(&s.saveButton),
	)
	connectGroup := huh.NewGroup(
		huh.NewConfirm().
			Title("Validate IMAP and SMTP").
			Affirmative("Connect").
			Negative("Cancel").
			Value(&s.saveButton),
	).Title("Connect account")

	groups := []*huh.Group{
		accountGroup,
		credentialsGroup,
		gmailGroup,
		gmailAdvancedGroup,
		gmailOAuthGroup,
		aiProviderGroup,
		ollamaDefaultGroup,
		ollamaGroup,
		claudeGroup,
		openAIGroup,
		wizardCacheGroup,
		keyboardGroup,
		wizardThemeGroup,
		composeGroup,
	}

	if s.mode == SettingsModePanel {
		switch s.panelSection {
		case settingsPanelSectionAccount:
			groups = []*huh.Group{
				accountGroup,
				credentialsGroup,
				gmailGroup,
				gmailAdvancedGroup,
				gmailOAuthGroup,
				saveGroup.Title("Account setup"),
			}
		case settingsPanelSectionAI:
			groups = []*huh.Group{
				aiProviderGroup,
				ollamaDefaultGroup,
				ollamaGroup,
				claudeGroup,
				openAIGroup,
				saveGroup.Title("AI"),
			}
		case settingsPanelSectionSync:
			groups = []*huh.Group{
				syncGroup,
				saveGroup.Title("Sync & Cleanup"),
			}
		case settingsPanelSectionKeyboard:
			groups = []*huh.Group{
				keyboardGroup,
				saveGroup.Title("Keyboard"),
			}
		case settingsPanelSectionTheme:
			groups = []*huh.Group{themeGroup}
		case settingsPanelSectionSign:
			groups = []*huh.Group{panelSignatureGroup}
		}
	}
	if s.mode == SettingsModeWizard && s.firstRunAccountOnly {
		groups = []*huh.Group{
			accountGroup,
			credentialsGroup,
			gmailGroup,
			gmailAdvancedGroup,
			gmailOAuthGroup,
			connectGroup,
		}
	} else if s.mode == SettingsModeWizard && s.firstRunPreferencesOnly {
		groups = []*huh.Group{
			aiProviderGroup,
			ollamaDefaultGroup,
			ollamaGroup,
			claudeGroup,
			openAIGroup,
			wizardCacheGroup,
			keyboardGroup,
			wizardThemeGroup,
			composeGroup,
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
	if strings.TrimSpace(s.panelStatus) != "" {
		fields = append(fields, huh.NewNote().Title(s.panelStatus))
	}
	fields = append(fields,
		huh.NewSelect[string]().
			Title("Settings").
			Description("Choose one area to edit. Save returns here; Esc closes without saving.").
			Options(
				huh.NewOption("Account setup", string(settingsPanelSectionAccount)),
				huh.NewOption("AI", string(settingsPanelSectionAI)),
				huh.NewOption("Sync & Cleanup", string(settingsPanelSectionSync)),
				huh.NewOption("Keyboard", string(settingsPanelSectionKeyboard)),
				huh.NewOption("Theme", string(settingsPanelSectionTheme)),
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
	if s.mode == SettingsModePanel && s.panelSection != settingsPanelSectionTheme {
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
	if s.mode == SettingsModePanel && s.panelSection != settingsPanelSectionTheme {
		return false
	}
	switch s.form.GetFocusedField().GetKey() {
	case settingsThemeForegroundKey, settingsThemeBackgroundKey:
		return true
	default:
		return false
	}
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

	panelH := shortcutHelpMaxHeight
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
		if s.mode == SettingsModeWizard && msg.Code == tea.KeyEscape {
			return s, s.navigateWizardBack(msg)
		}
		if s.mode == SettingsModeWizard && msg.Code == tea.KeyTab && msg.Mod.Contains(tea.ModShift) {
			return s, s.navigateWizardBack(msg)
		}
		// In panel mode, esc cancels if the active field has no local esc action.
		if s.mode == SettingsModePanel && msg.Code == tea.KeyEscape {
			if s.form.State != huh.StateCompleted && !s.focusedFieldHandlesKey(msg) {
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
	prevThemeRole := s.themeRole
	prevThemeFG := s.themeFG
	prevThemeBG := s.themeBG
	model, cmd := s.form.Update(formMsg)
	if f, ok := model.(*huh.Form); ok {
		s.form = f
	}
	if prevProvider != s.provider {
		s.syncProviderDefaults(prevProvider, s.provider)
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

		s.done = true
		if s.mode == SettingsModePanel && !s.saveButton {
			return s, tea.Batch(cmd, func() tea.Msg { return SettingsCancelledMsg{} })
		}

		if s.mode == SettingsModePanel && s.panelSection == settingsPanelSectionTheme {
			if err := s.applyThemeFileActions(); err != nil {
				s.done = false
				s.panelStatus = "Theme install failed: " + err.Error()
				s.saveButton = true
				s.buildForm()
				return s, tea.Batch(cmd, s.form.Init())
			}
		}

		cfg := s.buildConfig()
		if s.provider == "gmail-oauth" {
			// OAuth flow will handle saving after tokens received.
			builtCfg := cfg
			return s, tea.Batch(cmd, func() tea.Msg {
				return OAuthRequiredMsg{Email: s.email, Config: builtCfg}
			})
		}
		// Non-Gmail: signal done; the caller is responsible for saving.
		return s, tea.Batch(cmd, func() tea.Msg {
			return SettingsSavedMsg{
				Config:                     cfg,
				ReturnToMenu:               s.mode == SettingsModePanel,
				ReclaimOfflineCacheStorage: s.reclaimOfflineCacheStorage,
				ValidateAccount:            s.requiresAccountValidation(),
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
	currentFormView := s.form.View()
	if s.ensureProviderDefaults() || s.needsPresetFieldRefresh(currentFormView) {
		s.refreshFormPreservingVisibleGroup(s.visibleSettingsGroupTarget(currentFormView))
		currentFormView = s.form.View()
	}
	s.syncAIDefaults()
	formView := strings.TrimRight(currentFormView, "\n")

	if s.mode == SettingsModePanel {
		rendered := s.renderPanel()
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
	formView := s.panelFormView()
	layout := s.panelLayout()
	box := lipgloss.NewStyle().
		Width(layout.formWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(defaultTheme.Setup.Border.ForegroundColor()).
		Padding(1, 2)

	rendered := strings.TrimRight(box.Render(formView), "\n")
	lines := strings.Split(rendered, "\n")
	if len(lines) > layout.panelHeight {
		lines = lines[:layout.panelHeight]
	}
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, layout.panelWidth)
	}
	return strings.Join(lines, "\n")
}

func (s *Settings) panelFormView() string {
	formView := strings.TrimRight(s.form.View(), "\n")
	if s.panelSection == settingsPanelSectionMenu {
		formView = strings.Replace(formView, "enter submit", "enter open • esc exit", 1)
	}
	return s.panelFormViewWithFooterDivider(formView)
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

// buildConfig constructs a config.Config from the current form field values.
// It starts from a copy of the existing config so that fields not managed by
// this form (Daemon, Semantic, Classification.Prompts, OAuth tokens, etc.) are
// preserved unchanged.
func (s *Settings) buildConfig() *config.Config {
	s.ensureProviderDefaults()
	s.syncAIDefaults()
	// Shallow copy preserves all non-pointer fields; pointer/slice fields that
	// this form does not modify are left pointing at the same underlying data
	// (safe because we never mutate them — we only overwrite scalar fields below).
	cfg := *s.cfg
	if s.requiresAccountValidation() {
		cfg.Vendor = configVendorForProvider(s.provider)
		cfg.Gmail.AccessToken = ""
		cfg.Gmail.RefreshToken = ""
		cfg.Gmail.TokenExpiry = ""
		cfg.Gmail.Email = ""
		cfg.Credentials.Username = ""
		cfg.Credentials.Password = ""

		if s.provider == "gmail-oauth" {
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
	cfg.AI.Provider = configAIProvider(s.aiProvider)
	cfg.Ollama.Host = s.ollamaHost
	cfg.Ollama.Model = s.ollamaModel
	cfg.Ollama.EmbeddingModel = s.embedModel
	cfg.Semantic.Model = s.embedModel
	cfg.Claude.APIKey = s.claudeAPIKey
	cfg.Claude.Model = s.claudeModel
	cfg.OpenAI.APIKey = s.openAIAPIKey
	cfg.OpenAI.BaseURL = s.openAIBaseURL
	cfg.OpenAI.Model = s.openAIModel

	switch s.aiProvider {
	case aiProviderOllamaDefault:
		cfg.Ollama.Host = defaultOllamaHost
		cfg.Ollama.Model = defaultOllamaModel
		cfg.Ollama.EmbeddingModel = defaultEmbeddingModel
		cfg.Semantic.Model = defaultEmbeddingModel
	case aiProviderDisabled:
		cfg.Ollama.Host = ""
		cfg.Ollama.Model = ""
		cfg.Ollama.EmbeddingModel = ""
		cfg.Semantic.Model = ""
		cfg.Claude.APIKey = ""
		cfg.OpenAI.APIKey = ""
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
	cfg.Compose.Signature.Text = s.signatureText
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

	applyVendorPreset(&cfg)
	return &cfg
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
	if path := strings.TrimSpace(s.themeInstallPath); path != "" {
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
	if slug := strings.TrimSpace(s.themeSaveAs); slug != "" {
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
	case "gmail", "gmail-oauth":
		return "gmail"
	default:
		return provider
	}
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
		"IMAP Host>",
		"AI Provider",
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
	newIMAPHost, newIMAPPort, newSMTPHost, newSMTPPort, newOK := providerPresetValues(newProvider)
	if !newOK {
		return
	}

	if s.imapHost == "" || (oldOK && s.imapHost == oldIMAPHost) {
		s.imapHost = newIMAPHost
	}
	if s.imapPort == "" || (oldOK && s.imapPort == oldIMAPPort) {
		s.imapPort = newIMAPPort
	}
	if s.smtpHost == "" || (oldOK && s.smtpHost == oldSMTPHost) {
		s.smtpHost = newSMTPHost
	}
	if s.smtpPort == "" || (oldOK && s.smtpPort == oldSMTPPort) {
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
			wizardSummaryLine("Account:", "validated. Finish optional Herald preferences."),
			wizardSummaryLine("Next:", "choose AI, sync, theme, keyboard, and signature settings."),
		}
	}
	switch s.provider {
	case "gmail":
		return []string{
			wizardSummaryLine("Recommended:", "Gmail via IMAP + App Password."),
			wizardSummaryLine("Defaults:", "imap.gmail.com:993 and smtp.gmail.com:587."),
			wizardSummaryLine("Workspace:", "some accounts may still require OAuth."),
			wizardSummaryDoc("App passwords", "https://myaccount.google.com/apppasswords"),
			wizardSummaryDoc("Add Gmail to another client", "https://support.google.com/mail/answer/75726?hl=en"),
			wizardSummaryDoc("Workspace IMAP setup", "https://knowledge.workspace.google.com/admin/sync/set-up-gmail-with-a-third-party-email-client"),
		}
	case "gmail-oauth":
		return []string{
			wizardSummaryLine("Experimental:", "Gmail OAuth in a browser."),
			wizardSummaryLine("Visible with:", "-experimental during first-run setup, or from in-app settings."),
			wizardSummaryLine("Best with:", "Homebrew or release binaries, which include OAuth defaults."),
			wizardSummaryLine("Source builds:", "set HERALD_GOOGLE_CLIENT_ID and HERALD_GOOGLE_CLIENT_SECRET."),
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
			wizardSummaryLine("Recommended:", "Gmail (IMAP + App Password) for Gmail."),
			wizardSummaryLine("Supported:", "Standard IMAP plus ProtonMail Bridge, Fastmail, iCloud, Outlook."),
			wizardSummaryLine("Experimental:", "start with -experimental to include OAuth onboarding."),
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
