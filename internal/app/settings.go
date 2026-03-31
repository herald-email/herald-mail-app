package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/config"
)

// SettingsMode controls wizard vs panel layout.
type SettingsMode int

const (
	// SettingsModeWizard is fullscreen, no cancel.
	SettingsModeWizard SettingsMode = iota
	// SettingsModePanel is an overlay; esc to cancel.
	SettingsModePanel
)

// SettingsSavedMsg is sent when the user completes the form and saves.
type SettingsSavedMsg struct {
	Config *config.Config
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

	// form field backing variables — account
	provider    string
	email       string
	password    string
	imapHost    string
	imapPort    string
	smtpHost    string
	smtpPort    string

	// form field backing variables — AI provider
	aiProvider   string
	claudeAPIKey string
	claudeModel  string
	openAIAPIKey string
	openAIBaseURL string
	openAIModel  string
	ollamaHost   string
	ollamaModel  string
	embedModel   string

	// form field backing variables — sync & cleanup
	syncPollStr        string
	syncIDLE           bool
	cleanupScheduleStr string
}

// NewSettings creates a Settings component, pre-filling fields from an existing config.
// If existing is nil, a zero-value config is used.
func NewSettings(mode SettingsMode, existing *config.Config) *Settings {
	return NewSettingsWithPath(mode, existing, "")
}

// NewSettingsWithPath creates a Settings component with an explicit config file path for saving.
func NewSettingsWithPath(mode SettingsMode, existing *config.Config, configPath string) *Settings {
	s := &Settings{
		mode:       mode,
		cfg:        &config.Config{},
		configPath: configPath,
		syncIDLE:   true, // sensible default
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
		s.embedModel = existing.Ollama.EmbeddingModel
		s.claudeAPIKey = existing.Claude.APIKey
		s.claudeModel = existing.Claude.Model
		s.openAIAPIKey = existing.OpenAI.APIKey
		s.openAIBaseURL = existing.OpenAI.BaseURL
		s.openAIModel = existing.OpenAI.Model

		// Sync & cleanup fields
		s.syncPollStr = strconv.Itoa(existing.Sync.PollIntervalMinutes)
		s.syncIDLE = existing.Sync.IDLEEnabled
		s.cleanupScheduleStr = strconv.Itoa(existing.Cleanup.ScheduleHours)

		// If Gmail OAuth, use the Gmail email field.
		if existing.Gmail.Email != "" {
			s.email = existing.Gmail.Email
		}
	}

	// Default provider to "imap" if empty.
	if s.provider == "" {
		s.provider = "imap"
	}
	if s.aiProvider == "" {
		s.aiProvider = "ollama"
	}
	if s.syncPollStr == "" {
		s.syncPollStr = "5" // default only on first run; 0 is valid (IDLE-only mode)
	}
	if s.cleanupScheduleStr == "" {
		s.cleanupScheduleStr = "0"
	}

	s.buildForm()
	return s
}

// buildForm constructs the huh.Form with groups for account, AI provider, and sync preferences.
func (s *Settings) buildForm() {
	// Group 1 — Account type selection
	accountGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Account Type").
			Options(
				huh.NewOption("Gmail (OAuth2)", "gmail"),
				huh.NewOption("Standard IMAP", "imap"),
				huh.NewOption("ProtonMail Bridge", "protonmail"),
				huh.NewOption("Fastmail", "fastmail"),
				huh.NewOption("iCloud", "icloud"),
				huh.NewOption("Outlook", "outlook"),
			).
			Value(&s.provider),
	)

	// Group 2a — Credentials for non-Gmail providers
	credentialsGroup := huh.NewGroup(
		huh.NewInput().Title("Email address").Value(&s.email).Validate(validateEmail),
		huh.NewInput().Title("Password").EchoMode(huh.EchoModePassword).Value(&s.password),
		huh.NewInput().Title("IMAP Host").Value(&s.imapHost),
		huh.NewInput().Title("IMAP Port").Value(&s.imapPort).
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
		huh.NewInput().Title("SMTP Host").Value(&s.smtpHost),
		huh.NewInput().Title("SMTP Port").Value(&s.smtpPort).
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
	).WithHideFunc(func() bool { return s.provider == "gmail" })

	// Group 2b — Gmail OAuth notice
	gmailGroup := huh.NewGroup(
		huh.NewNote().
			Title("Gmail OAuth2").
			Description("After saving, you'll be guided through\nGoogle authorization in your browser."),
		huh.NewInput().Title("Gmail address").Value(&s.email).Validate(validateEmail),
	).WithHideFunc(func() bool { return s.provider != "gmail" })

	// Group 3 — AI provider selection
	aiProviderGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("AI Provider").
			Options(
				huh.NewOption("Ollama (local)", "ollama"),
				huh.NewOption("Claude API", "claude"),
				huh.NewOption("OpenAI / compatible", "openai"),
			).
			Value(&s.aiProvider),
	).Title("AI Provider")

	// Group 3a — Ollama settings (shown only when provider = ollama)
	ollamaGroup := huh.NewGroup(
		huh.NewInput().Title("Ollama Host").Value(&s.ollamaHost).Placeholder("http://localhost:11434"),
		huh.NewInput().Title("Ollama Model").Value(&s.ollamaModel).Placeholder("gemma3:4b"),
		huh.NewInput().Title("Embedding Model").Value(&s.embedModel).Placeholder("nomic-embed-text"),
	).WithHideFunc(func() bool { return s.aiProvider != "ollama" })

	// Group 3b — Claude settings (shown only when provider = claude)
	claudeGroup := huh.NewGroup(
		huh.NewInput().Title("Claude API Key").EchoMode(huh.EchoModePassword).Value(&s.claudeAPIKey),
		huh.NewInput().Title("Claude Model").Placeholder("claude-sonnet-4-6").Value(&s.claudeModel),
	).WithHideFunc(func() bool { return s.aiProvider != "claude" })

	// Group 3c — OpenAI settings (shown only when provider = openai)
	openAIGroup := huh.NewGroup(
		huh.NewInput().Title("OpenAI API Key").EchoMode(huh.EchoModePassword).Value(&s.openAIAPIKey),
		huh.NewInput().Title("OpenAI Base URL").Placeholder("https://api.openai.com/v1").Value(&s.openAIBaseURL),
		huh.NewInput().Title("OpenAI Model").Placeholder("gpt-4o").Value(&s.openAIModel),
	).WithHideFunc(func() bool { return s.aiProvider != "openai" })

	// Group 4 — Sync & Cleanup preferences
	syncGroup := huh.NewGroup(
		huh.NewInput().
			Title("Poll Interval (minutes)").
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
		huh.NewInput().
			Title("Auto-Cleanup Schedule (hours)").
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

	s.form = huh.NewForm(
		accountGroup,
		credentialsGroup,
		gmailGroup,
		aiProviderGroup,
		ollamaGroup,
		claudeGroup,
		openAIGroup,
		syncGroup,
	).
		WithShowHelp(true).
		WithShowErrors(true)

	if s.width > 0 {
		s.form = s.form.WithWidth(s.formWidth())
	}
	if s.height > 0 {
		s.form = s.form.WithHeight(s.height)
	}
}

// formWidth returns the width the form should use.
func (s *Settings) formWidth() int {
	if s.mode == SettingsModePanel {
		w := int(float64(s.width) * 0.8)
		if w < 40 {
			w = 40
		}
		if w > s.width {
			w = s.width
		}
		return w
	}
	return s.width
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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s.form = s.form.WithWidth(s.formWidth()).WithHeight(s.height)
		return s, nil

	case tea.KeyMsg:
		// In panel mode, esc cancels if the form isn't yet complete.
		if s.mode == SettingsModePanel && msg.Type == tea.KeyEscape {
			if s.form.State != huh.StateCompleted {
				s.done = true
				return s, func() tea.Msg { return SettingsCancelledMsg{} }
			}
		}
	}

	// Forward to the form.
	model, cmd := s.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		s.form = f
	}

	// Check if the form just completed.
	if s.form.State == huh.StateCompleted && !s.done {
		s.done = true
		cfg := s.buildConfig()
		if s.provider == "gmail" {
			// OAuth flow will handle saving after tokens received.
			builtCfg := cfg
			return s, tea.Batch(cmd, func() tea.Msg {
				return OAuthRequiredMsg{Email: s.email, Config: builtCfg}
			})
		}
		// Non-Gmail: signal done; the caller is responsible for saving.
		return s, tea.Batch(cmd, func() tea.Msg {
			return SettingsSavedMsg{Config: cfg}
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
func (s *Settings) View() string {
	formView := s.form.View()

	if s.mode == SettingsModePanel {
		w := s.formWidth()
		box := lipgloss.NewStyle().
			Width(w).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2)

		rendered := box.Render(formView)
		return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, rendered)
	}

	return formView
}

// buildConfig constructs a config.Config from the current form field values.
// It starts from a copy of the existing config so that fields not managed by
// this form (Daemon, Semantic, Classification.Prompts, OAuth tokens, etc.) are
// preserved unchanged.
func (s *Settings) buildConfig() *config.Config {
	// Shallow copy preserves all non-pointer fields; pointer/slice fields that
	// this form does not modify are left pointing at the same underlying data
	// (safe because we never mutate them — we only overwrite scalar fields below).
	cfg := *s.cfg
	cfg.Vendor = s.provider

	if s.provider == "gmail" {
		cfg.Gmail.Email = s.email
	} else {
		cfg.Credentials.Username = s.email
		cfg.Credentials.Password = s.password
	}

	cfg.Server.Host = s.imapHost
	cfg.Server.Port = parsePort(s.imapPort)
	cfg.SMTP.Host = s.smtpHost
	cfg.SMTP.Port = parsePort(s.smtpPort)

	// AI provider
	cfg.AI.Provider = s.aiProvider
	cfg.Ollama.Host = s.ollamaHost
	cfg.Ollama.Model = s.ollamaModel
	cfg.Ollama.EmbeddingModel = s.embedModel
	cfg.Claude.APIKey = s.claudeAPIKey
	cfg.Claude.Model = s.claudeModel
	cfg.OpenAI.APIKey = s.openAIAPIKey
	cfg.OpenAI.BaseURL = s.openAIBaseURL
	cfg.OpenAI.Model = s.openAIModel

	// Sync & cleanup
	if n, err := strconv.Atoi(s.syncPollStr); err == nil {
		cfg.Sync.PollIntervalMinutes = n
	}
	cfg.Sync.IDLEEnabled = s.syncIDLE
	if n, err := strconv.Atoi(s.cleanupScheduleStr); err == nil {
		cfg.Cleanup.ScheduleHours = n
	}

	applyVendorPreset(&cfg)
	return &cfg
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
