package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	"gopkg.in/yaml.v3"
)

// ExpandPath replaces a leading "~" with the current user's home directory.
func ExpandPath(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, p[1:]), nil
}

type CredentialsConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type GoogleConfig struct {
	AccessToken  string `yaml:"access_token,omitempty"`
	RefreshToken string `yaml:"refresh_token,omitempty"`
	TokenExpiry  string `yaml:"token_expiry,omitempty"`
	Email        string `yaml:"email,omitempty"`
	APIBaseURL   string `yaml:"api_base_url,omitempty"`
	TokenURL     string `yaml:"token_url,omitempty"`
}

type CalDAVConfig struct {
	URL      string `yaml:"url,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type SourceConfig struct {
	ID          string `yaml:"id"`
	Kind        string `yaml:"kind"`     // mail | calendar
	Provider    string `yaml:"provider"` // imap | gmail | google_calendar | caldav
	DisplayName string `yaml:"display_name,omitempty"`
	AccountID   string `yaml:"account_id,omitempty"`

	Credentials CredentialsConfig `yaml:"credentials,omitempty"`
	IMAP        ServerConfig      `yaml:"imap,omitempty"`
	SMTP        ServerConfig      `yaml:"smtp,omitempty"`
	Google      GoogleConfig      `yaml:"google,omitempty"`
	CalDAV      CalDAVConfig      `yaml:"caldav,omitempty"`
	Compose     ComposeConfig     `yaml:"compose,omitempty"`
}

type SignatureConfig struct {
	Text string `yaml:"text,omitempty"`
}

type ComposeConfig struct {
	Signature SignatureConfig `yaml:"signature,omitempty"`
}

type CalendarConfig struct {
	WeekStart         string   `yaml:"week_start,omitempty"` // monday | sunday
	SelectedCalendars []string `yaml:"selected_calendars,omitempty"`
}

type NotificationConfig struct {
	Enabled                  bool `yaml:"enabled"`
	NewMail                  bool `yaml:"new_mail"`
	SyncFailures             bool `yaml:"sync_failures"`
	DeletionCompletion       bool `yaml:"deletion_completion"`
	ClassificationCompletion bool `yaml:"classification_completion"`
	ChatResults              bool `yaml:"chat_results"`
	Sound                    bool `yaml:"sound"`

	enabledSet      bool
	newMailSet      bool
	syncFailuresSet bool
}

func (n *NotificationConfig) UnmarshalYAML(value *yaml.Node) error {
	type notificationConfigYAML struct {
		Enabled                  *bool `yaml:"enabled"`
		NewMail                  *bool `yaml:"new_mail"`
		SyncFailures             *bool `yaml:"sync_failures"`
		DeletionCompletion       *bool `yaml:"deletion_completion"`
		ClassificationCompletion *bool `yaml:"classification_completion"`
		ChatResults              *bool `yaml:"chat_results"`
		Sound                    *bool `yaml:"sound"`
	}
	var decoded notificationConfigYAML
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	if decoded.Enabled != nil {
		n.Enabled = *decoded.Enabled
		n.enabledSet = true
	}
	if decoded.NewMail != nil {
		n.NewMail = *decoded.NewMail
		n.newMailSet = true
	}
	if decoded.SyncFailures != nil {
		n.SyncFailures = *decoded.SyncFailures
		n.syncFailuresSet = true
	}
	if decoded.DeletionCompletion != nil {
		n.DeletionCompletion = *decoded.DeletionCompletion
	}
	if decoded.ClassificationCompletion != nil {
		n.ClassificationCompletion = *decoded.ClassificationCompletion
	}
	if decoded.ChatResults != nil {
		n.ChatResults = *decoded.ChatResults
	}
	if decoded.Sound != nil {
		n.Sound = *decoded.Sound
	}
	return nil
}

type AccountGroup struct {
	AccountID         string
	DisplayName       string
	Provider          string
	Address           string
	Capability        string
	Sources           []SourceConfig
	MailSourceID      string
	CalendarSourceIDs []string
}

// Config represents the application configuration
type Config struct {
	Vendor string `yaml:"vendor"` // gmail | protonmail | fastmail | outlook | icloud
	Cache  struct {
		DatabasePath  string `yaml:"database_path,omitempty"`
		StoragePolicy string `yaml:"storage_policy,omitempty"` // lightweight | no_attachments | preserve_all
	} `yaml:"cache,omitempty"`
	Compose  ComposeConfig `yaml:"compose,omitempty"`
	Keyboard struct {
		Profile      string `yaml:"profile,omitempty"`
		CustomKeymap string `yaml:"custom_keymap,omitempty"`
	} `yaml:"keyboard,omitempty"`
	Calendar    CalendarConfig    `yaml:"calendar,omitempty"`
	Theme       ThemeConfig       `yaml:"theme,omitempty"`
	Sources     []SourceConfig    `yaml:"sources,omitempty"`
	Credentials CredentialsConfig `yaml:"credentials"`
	Server      ServerConfig      `yaml:"server"`
	SMTP        ServerConfig      `yaml:"smtp"`
	Ollama      struct {
		Host           string `yaml:"host"`            // default: http://localhost:11434
		Model          string `yaml:"model"`           // default: gemma3:4b
		EmbeddingModel string `yaml:"embedding_model"` // default: nomic-embed-text-v2-moe
	} `yaml:"ollama"`
	Sync struct {
		Idle                bool `yaml:"idle"`                  // default: true
		Interval            int  `yaml:"interval"`              // fallback poll seconds, default: 60
		Background          bool `yaml:"background"`            // sync other folders, default: true
		Notify              bool `yaml:"notify"`                // status bar flash, default: true
		PollIntervalMinutes int  `yaml:"poll_interval_minutes"` // 0 = IDLE only; default 5
		IDLEEnabled         bool `yaml:"idle_enabled"`          // default true
	} `yaml:"sync"`
	Notifications NotificationConfig `yaml:"notifications,omitempty"`
	Semantic      struct {
		Enabled   bool    `yaml:"enabled"`    // default: true when AI is configured
		Provider  string  `yaml:"provider"`   // ollama | openai; default: inferred from AI provider
		Model     string  `yaml:"model"`      // default: provider embedding model
		BatchSize int     `yaml:"batch_size"` // default: 20
		MinScore  float64 `yaml:"min_score"`  // default: 0.65
	} `yaml:"semantic"`
	Gmail struct {
		AccessToken  string `yaml:"access_token,omitempty"`
		RefreshToken string `yaml:"refresh_token,omitempty"`
		// TokenExpiry is the OAuth access-token expiry in RFC3339 format.
		TokenExpiry string `yaml:"token_expiry,omitempty"`
		Email       string `yaml:"email,omitempty"`
	} `yaml:"gmail,omitempty"`
	Daemon struct {
		Port     int    `yaml:"port"`     // default: 7272
		BindAddr string `yaml:"bind"`     // default: "127.0.0.1"
		PidFile  string `yaml:"pid_file"` // default: ~/.local/share/herald/daemon.pid
		LogFile  string `yaml:"log_file"` // default: ~/.local/share/herald/daemon.log
	} `yaml:"daemon"`

	Classification struct {
		Prompts []struct {
			Name         string `yaml:"name"`
			SystemText   string `yaml:"system_text"`
			UserTemplate string `yaml:"user_template"`
			OutputVar    string `yaml:"output_var"`
		} `yaml:"prompts"`
	} `yaml:"classification"`

	ClassificationActions []struct {
		Name         string `yaml:"name"`
		TriggerType  string `yaml:"trigger_type"` // "category" | "sender" | "domain"
		TriggerValue string `yaml:"trigger_value"`
		ActionType   string `yaml:"action_type"`  // "notify" | "move" | "archive" | "delete" | "command" | "webhook"
		ActionValue  string `yaml:"action_value"` // folder name, command, URL, or notification text
		Enabled      bool   `yaml:"enabled"`
	} `yaml:"classification_actions"`

	Cleanup struct {
		ScheduleHours int `yaml:"schedule_hours"` // 0 = disabled (no auto-run)
	} `yaml:"cleanup"`

	Claude struct {
		APIKey string `yaml:"api_key"`
		Model  string `yaml:"model"` // default: "claude-sonnet-4-6"
	} `yaml:"claude"`

	OpenAI struct {
		APIKey         string `yaml:"api_key"`
		BaseURL        string `yaml:"base_url"`        // default: "https://api.openai.com/v1"
		Model          string `yaml:"model"`           // default: "gpt-5.4-mini"
		EmbeddingModel string `yaml:"embedding_model"` // default: "text-embedding-3-small"
	} `yaml:"openai"`

	AI struct {
		Provider                        string `yaml:"provider"`                           // "ollama" | "claude" | "openai" | "disabled"; default: "ollama"
		LocalMaxConcurrency             int    `yaml:"local_max_concurrency"`              // default: 1
		ExternalMaxConcurrency          int    `yaml:"external_max_concurrency"`           // default: 4
		BackgroundQueueLimit            int    `yaml:"background_queue_limit"`             // default: 64
		PauseBackgroundWhileInteractive bool   `yaml:"pause_background_while_interactive"` // default: true
	} `yaml:"ai"`
}

type ThemeConfig struct {
	Name      string                   `yaml:"name,omitempty"`
	Overrides map[string]ThemeOverride `yaml:"overrides,omitempty"`
}

type ThemeOverride struct {
	Foreground    string `yaml:"fg,omitempty"`
	Background    string `yaml:"bg,omitempty"`
	Bold          bool   `yaml:"bold,omitempty"`
	Faint         bool   `yaml:"faint,omitempty"`
	Underline     bool   `yaml:"underline,omitempty"`
	Reverse       bool   `yaml:"reverse,omitempty"`
	Strikethrough bool   `yaml:"strikethrough,omitempty"`
}

// vendorPreset holds IMAP/SMTP defaults for a known mail provider
type vendorPreset struct {
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
}

var vendorPresets = map[string]vendorPreset{
	"protonmail": {"127.0.0.1", 1143, "127.0.0.1", 1025},
	"gmail":      {"imap.gmail.com", 993, "smtp.gmail.com", 587},
	"gmail_api":  {"imap.gmail.com", 993, "smtp.gmail.com", 587},
	"outlook":    {"outlook.office365.com", 993, "smtp.office365.com", 587},
	"fastmail":   {"imap.fastmail.com", 993, "smtp.fastmail.com", 587},
	"icloud":     {"imap.mail.me.com", 993, "smtp.mail.me.com", 587},
}

const (
	CacheStoragePolicyLightweight   = "lightweight"
	CacheStoragePolicyNoAttachments = "no_attachments"
	CacheStoragePolicyPreserveAll   = "preserve_all"

	CalendarWeekStartMonday = "monday"
	CalendarWeekStartSunday = "sunday"

	EmbeddingProviderOllama = "ollama"
	EmbeddingProviderOpenAI = "openai"

	defaultOllamaEmbeddingModel = "nomic-embed-text-v2-moe"
	defaultOpenAIEmbeddingModel = "text-embedding-3-small"
)

func NormalizeCacheStoragePolicy(policy string) string {
	switch strings.TrimSpace(policy) {
	case CacheStoragePolicyLightweight:
		return CacheStoragePolicyLightweight
	case CacheStoragePolicyNoAttachments:
		return CacheStoragePolicyNoAttachments
	case CacheStoragePolicyPreserveAll:
		return CacheStoragePolicyPreserveAll
	default:
		return CacheStoragePolicyNoAttachments
	}
}

func NormalizeCalendarWeekStart(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CalendarWeekStartSunday:
		return CalendarWeekStartSunday
	default:
		return CalendarWeekStartMonday
	}
}

// ApplyVendorPreset fills in server/smtp host+port when a vendor shortcut is
// set and the user has not provided explicit values. Exported so that other
// packages (e.g. the settings form) can apply presets to a freshly built config.
func (c *Config) ApplyVendorPreset() {
	if c.Vendor == "" {
		return
	}
	preset, ok := vendorPresets[c.Vendor]
	if !ok {
		return
	}
	if c.Server.Host == "" {
		c.Server.Host = preset.IMAPHost
	}
	if c.Server.Port == 0 {
		c.Server.Port = preset.IMAPPort
	}
	if c.SMTP.Host == "" {
		c.SMTP.Host = preset.SMTPHost
	}
	if c.SMTP.Port == 0 {
		c.SMTP.Port = preset.SMTPPort
	}
}

// IsGmailOAuth returns true when the config contains a Gmail OAuth refresh token,
// indicating the user authenticates via OAuth rather than username/password.
func (c *Config) IsGmailOAuth() bool {
	return c.Gmail.RefreshToken != ""
}

// NormalizeEmbeddingProvider returns a supported embedding provider name.
func NormalizeEmbeddingProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case EmbeddingProviderOpenAI, "openai-compatible", "openai_compatible":
		return EmbeddingProviderOpenAI
	default:
		return EmbeddingProviderOllama
	}
}

// EffectiveEmbeddingProvider returns the embedding provider used for semantic
// search. A blank semantic.provider follows the active AI provider for OpenAI
// configs and otherwise preserves the local Ollama default.
func (c *Config) EffectiveEmbeddingProvider() string {
	if c == nil {
		return EmbeddingProviderOllama
	}
	if strings.TrimSpace(c.Semantic.Provider) != "" {
		return NormalizeEmbeddingProvider(c.Semantic.Provider)
	}
	if strings.TrimSpace(c.AI.Provider) == EmbeddingProviderOpenAI {
		return EmbeddingProviderOpenAI
	}
	return EmbeddingProviderOllama
}

// EffectiveEmbeddingModel returns the configured embedding model for the
// effective provider. Legacy OpenAI configs saved before semantic.provider
// existed may carry the old Ollama default in semantic.model; when OpenAI is
// the effective provider, that old local default migrates to openai.embedding_model.
func (c *Config) EffectiveEmbeddingModel() string {
	if c == nil {
		return defaultOllamaEmbeddingModel
	}
	provider := c.EffectiveEmbeddingProvider()
	model := strings.TrimSpace(c.Semantic.Model)
	switch provider {
	case EmbeddingProviderOpenAI:
		if model != "" &&
			model != defaultOllamaEmbeddingModel &&
			model != strings.TrimSpace(c.Ollama.EmbeddingModel) {
			return model
		}
		if trimmed := strings.TrimSpace(c.OpenAI.EmbeddingModel); trimmed != "" {
			return trimmed
		}
		return defaultOpenAIEmbeddingModel
	default:
		if model != "" {
			return model
		}
		if trimmed := strings.TrimSpace(c.Ollama.EmbeddingModel); trimmed != "" {
			return trimmed
		}
		return defaultOllamaEmbeddingModel
	}
}

// EffectiveEmbeddingIdentity is the cache-visible embedding identity. It
// includes the provider so identical model strings from different vendors do
// not accidentally share semantic vectors.
func (c *Config) EffectiveEmbeddingIdentity() string {
	if c == nil {
		return EmbeddingProviderOllama + ":" + defaultOllamaEmbeddingModel
	}
	return c.EffectiveEmbeddingProvider() + ":" + c.EffectiveEmbeddingModel()
}

func (c Config) NormalizedSources() []SourceConfig {
	if len(c.Sources) > 0 {
		return normalizeExplicitSources(c.Sources, c.Compose.Signature.Text)
	}
	return []SourceConfig{legacyDefaultMailSource(c)}
}

func (c Config) ExplicitSourcesForEdit() []SourceConfig {
	sources := c.Sources
	if len(sources) == 0 {
		sources = c.NormalizedSources()
	}
	return normalizeExplicitSources(sources, c.Compose.Signature.Text)
}

func (c Config) AccountGroups() []AccountGroup {
	sources := c.ExplicitSourcesForEdit()
	byID := make(map[string]*AccountGroup)
	var order []string
	for _, source := range sources {
		accountID := strings.TrimSpace(source.AccountID)
		if accountID == "" {
			accountID = string(models.DefaultAccountID)
		}
		group := byID[accountID]
		if group == nil {
			group = &AccountGroup{AccountID: accountID}
			byID[accountID] = group
			order = append(order, accountID)
		}
		group.Sources = append(group.Sources, source)
		if group.DisplayName == "" {
			group.DisplayName = strings.TrimSpace(source.DisplayName)
		}
		if group.Provider == "" {
			group.Provider = strings.TrimSpace(source.Provider)
		}
		if group.Address == "" {
			group.Address = sourceAddress(source)
		}
		switch strings.TrimSpace(source.Kind) {
		case "", string(models.SourceKindMail):
			if group.MailSourceID == "" {
				group.MailSourceID = source.ID
			}
		case string(models.SourceKindCalendar):
			group.CalendarSourceIDs = append(group.CalendarSourceIDs, source.ID)
		}
	}
	sort.Strings(order)
	groups := make([]AccountGroup, 0, len(order))
	for _, id := range order {
		group := *byID[id]
		if group.DisplayName == "" {
			group.DisplayName = id
		}
		hasMail := group.MailSourceID != ""
		hasCalendar := len(group.CalendarSourceIDs) > 0
		switch {
		case hasMail && hasCalendar:
			group.Capability = "Mail + Calendar"
		case hasCalendar:
			group.Capability = "Calendar"
		default:
			group.Capability = "Mail"
		}
		groups = append(groups, group)
	}
	return groups
}

func (c Config) RemoveAccountSources(accountID string) (Config, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		accountID = string(models.DefaultAccountID)
	}
	next := c
	var kept []SourceConfig
	for _, source := range c.ExplicitSourcesForEdit() {
		sourceAccountID := strings.TrimSpace(source.AccountID)
		if sourceAccountID == "" {
			sourceAccountID = string(models.DefaultAccountID)
		}
		if sourceAccountID == accountID {
			continue
		}
		kept = append(kept, source)
	}
	if len(kept) == len(c.ExplicitSourcesForEdit()) {
		return next, fmt.Errorf("account %q is not configured", accountID)
	}
	if !hasMailSource(kept) {
		return next, fmt.Errorf("cannot remove the last mail source")
	}
	next.Sources = kept
	next.syncLegacyFieldsFromFirstMailSource()
	return next, nil
}

func (c *Config) syncLegacyFieldsFromFirstMailSource() {
	if c == nil {
		return
	}
	for _, source := range c.ExplicitSourcesForEdit() {
		if strings.TrimSpace(source.Kind) != "" && source.Kind != string(models.SourceKindMail) {
			continue
		}
		c.Vendor = source.Provider
		c.Credentials = source.Credentials
		c.Server = source.IMAP
		c.SMTP = source.SMTP
		c.Gmail.AccessToken = source.Google.AccessToken
		c.Gmail.RefreshToken = source.Google.RefreshToken
		c.Gmail.TokenExpiry = source.Google.TokenExpiry
		c.Gmail.Email = source.Google.Email
		return
	}
}

func normalizeExplicitSources(sources []SourceConfig, defaultSignature string) []SourceConfig {
	normalized := make([]SourceConfig, 0, len(sources))
	for _, source := range sources {
		source.ID = strings.TrimSpace(source.ID)
		source.Kind = strings.TrimSpace(source.Kind)
		source.Provider = strings.TrimSpace(source.Provider)
		source.AccountID = strings.TrimSpace(source.AccountID)
		if source.Kind == "" {
			source.Kind = string(models.SourceKindMail)
		}
		if source.Provider == "" {
			if source.Kind == string(models.SourceKindCalendar) {
				source.Provider = "google_calendar"
			} else {
				source.Provider = "imap"
			}
		}
		if source.AccountID == "" {
			source.AccountID = string(models.DefaultAccountID)
		}
		if source.ID == "" {
			if source.Kind == string(models.SourceKindCalendar) {
				source.ID = "default-calendar"
			} else {
				source.ID = string(models.DefaultMailSourceID)
			}
		}
		if source.Kind == string(models.SourceKindMail) && strings.TrimSpace(source.Compose.Signature.Text) == "" {
			source.Compose.Signature.Text = defaultSignature
		}
		normalized = append(normalized, source)
	}
	return normalized
}

func sourceAddress(source SourceConfig) string {
	for _, value := range []string{
		source.Credentials.Username,
		source.Google.Email,
		source.CalDAV.Username,
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func hasMailSource(sources []SourceConfig) bool {
	for _, source := range sources {
		if strings.TrimSpace(source.Kind) == "" || source.Kind == string(models.SourceKindMail) {
			return true
		}
	}
	return false
}

func legacyDefaultMailSource(c Config) SourceConfig {
	provider := "imap"
	if c.IsGmailOAuth() {
		provider = "gmail"
	}
	return SourceConfig{
		ID:          string(models.DefaultMailSourceID),
		Kind:        string(models.SourceKindMail),
		Provider:    provider,
		DisplayName: "Default Mail",
		AccountID:   string(models.DefaultAccountID),
		Credentials: c.Credentials,
		IMAP:        c.Server,
		SMTP:        c.SMTP,
		Google: GoogleConfig{
			AccessToken:  c.Gmail.AccessToken,
			RefreshToken: c.Gmail.RefreshToken,
			TokenExpiry:  c.Gmail.TokenExpiry,
			Email:        c.Gmail.Email,
		},
		Compose: c.Compose,
	}
}

// EnsureCacheDatabasePath returns the SQLite cache path for this config. An
// explicit YAML path wins; otherwise Herald generates a per-config path,
// persists it to YAML, and returns that generated path.
func EnsureCacheDatabasePath(configPath string, c *Config) (string, error) {
	if c == nil {
		return "", fmt.Errorf("config is nil")
	}
	if configured := strings.TrimSpace(c.Cache.DatabasePath); configured != "" {
		expanded, err := ExpandPath(configured)
		if err != nil {
			return "", err
		}
		return expanded, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	cacheDir := filepath.Join(home, ".herald", "cached")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	stem := sanitizeCacheStem(strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath)))
	candidate := filepath.Join(cacheDir, stem+".db")
	exists, err := fileExists(candidate)
	if err != nil {
		return "", err
	}
	if exists {
		candidate, err = disambiguatedCachePath(cacheDir, stem)
		if err != nil {
			return "", err
		}
	}

	c.Cache.DatabasePath = candidate
	if err := c.Save(configPath); err != nil {
		return "", fmt.Errorf("failed to persist generated cache path: %w", err)
	}
	return candidate, nil
}

func sanitizeCacheStem(stem string) string {
	var b strings.Builder
	for _, r := range stem {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	clean := strings.Trim(b.String(), "-_.")
	if clean == "" {
		return "config"
	}
	return clean
}

func disambiguatedCachePath(cacheDir, stem string) (string, error) {
	date := time.Now().Format("20060102")
	for i := 0; i < 20; i++ {
		token, err := randomHex(6)
		if err != nil {
			return "", err
		}
		candidate := filepath.Join(cacheDir, fmt.Sprintf("%s-%s-%s.db", stem, date, token))
		exists, err := fileExists(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("failed to find unused cache path for %s", stem)
}

func randomHex(chars int) (string, error) {
	buf := make([]byte, (chars+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate cache path suffix: %w", err)
	}
	return hex.EncodeToString(buf)[:chars], nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to inspect cache path %s: %w", path, err)
}

// Save marshals the config to YAML and writes it atomically to path with 0600 permissions.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after successful rename
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to rename config file: %w", err)
	}
	return nil
}

// applyDefaults sets sensible defaults for optional config fields
func (c *Config) applyDefaults() {
	c.Cache.StoragePolicy = NormalizeCacheStoragePolicy(c.Cache.StoragePolicy)
	c.Calendar.WeekStart = NormalizeCalendarWeekStart(c.Calendar.WeekStart)
	if c.Ollama.Model == "" {
		c.Ollama.Model = "gemma3:4b"
	}
	if c.Ollama.EmbeddingModel == "" {
		c.Ollama.EmbeddingModel = defaultOllamaEmbeddingModel
	}
	if c.OpenAI.BaseURL == "" {
		c.OpenAI.BaseURL = "https://api.openai.com/v1"
	}
	if c.OpenAI.Model == "" {
		c.OpenAI.Model = "gpt-5.4-mini"
	}
	if c.OpenAI.EmbeddingModel == "" {
		c.OpenAI.EmbeddingModel = defaultOpenAIEmbeddingModel
	}
	if c.Semantic.Provider == "" {
		c.Semantic.Provider = c.EffectiveEmbeddingProvider()
	} else {
		c.Semantic.Provider = NormalizeEmbeddingProvider(c.Semantic.Provider)
	}
	if c.Semantic.Model == "" {
		c.Semantic.Model = c.EffectiveEmbeddingModel()
	}
	if c.Sync.Interval == 0 {
		c.Sync.Interval = 60
	}
	if !c.Notifications.enabledSet {
		c.Notifications.Enabled = true
	}
	if !c.Notifications.newMailSet {
		c.Notifications.NewMail = true
	}
	if !c.Notifications.syncFailuresSet {
		c.Notifications.SyncFailures = true
	}
	if c.Semantic.BatchSize == 0 {
		c.Semantic.BatchSize = 20
	}
	if c.Semantic.MinScore == 0 {
		c.Semantic.MinScore = 0.30
	}
	// Enable IDLE and background sync by default
	// (zero-value bool is false; we use a separate flag to detect "not set")
	// Users must explicitly set idle: false to disable

	// AI provider defaults
	if c.AI.Provider == "" {
		c.AI.Provider = "ollama"
	}
	if c.AI.LocalMaxConcurrency == 0 {
		c.AI.LocalMaxConcurrency = 1
	}
	if c.AI.ExternalMaxConcurrency == 0 {
		c.AI.ExternalMaxConcurrency = 4
	}
	if c.AI.BackgroundQueueLimit == 0 {
		c.AI.BackgroundQueueLimit = 64
	}
	if !c.AI.PauseBackgroundWhileInteractive {
		c.AI.PauseBackgroundWhileInteractive = true
	}
	if c.Claude.Model == "" {
		c.Claude.Model = "claude-sonnet-4-6"
	}
	if c.Keyboard.Profile == "" {
		c.Keyboard.Profile = "default"
	}
	if strings.TrimSpace(c.Theme.Name) == "" {
		c.Theme.Name = "inherited"
	}
	if c.Theme.Overrides == nil {
		c.Theme.Overrides = make(map[string]ThemeOverride)
	}

	// Daemon defaults
	if c.Daemon.Port == 0 {
		c.Daemon.Port = 7272
	}
	if c.Daemon.BindAddr == "" {
		c.Daemon.BindAddr = "127.0.0.1"
	}
	if c.Daemon.PidFile == "" || c.Daemon.LogFile == "" {
		if home, err := os.UserHomeDir(); err == nil {
			if c.Daemon.PidFile == "" {
				c.Daemon.PidFile = filepath.Join(home, ".local", "share", "herald", "daemon.pid")
			}
			if c.Daemon.LogFile == "" {
				c.Daemon.LogFile = filepath.Join(home, ".local", "share", "herald", "daemon.log")
			}
		}
	}
}

// Load reads and parses the configuration file
func Load(configPath string) (*Config, error) {
	// Check file permissions for security
	if err := checkFilePermissions(configPath); err != nil {
		return nil, fmt.Errorf("config file permission check failed: %w", err)
	}

	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	config.ApplyVendorPreset()
	config.applyDefaults()

	// Validate required fields
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// validate checks that all required configuration fields are present
func (c *Config) validate() error {
	if len(c.Sources) > 0 {
		return c.validateExplicitSources()
	}
	// Gmail OAuth users authenticate via token; skip username/password checks.
	if !c.IsGmailOAuth() {
		if c.Credentials.Username == "" {
			return fmt.Errorf("missing credentials.username")
		}
		if c.Credentials.Password == "" {
			return fmt.Errorf("missing credentials.password")
		}
	}
	if c.Server.Host == "" {
		return fmt.Errorf("missing server.host")
	}
	if c.Server.Port == 0 {
		return fmt.Errorf("missing server.port")
	}
	return nil
}

func (c *Config) validateExplicitSources() error {
	sources := c.NormalizedSources()
	if !hasMailSource(sources) {
		return fmt.Errorf("missing mail source")
	}
	for _, source := range sources {
		if strings.TrimSpace(source.Kind) != "" && source.Kind != string(models.SourceKindMail) {
			continue
		}
		if explicitMailSourceUsesGoogleOAuth(source) {
			continue
		}
		if strings.TrimSpace(source.Credentials.Username) == "" {
			return fmt.Errorf("mail source %q missing credentials.username", source.ID)
		}
		if strings.TrimSpace(source.Credentials.Password) == "" {
			return fmt.Errorf("mail source %q missing credentials.password", source.ID)
		}
		if strings.TrimSpace(source.IMAP.Host) == "" {
			return fmt.Errorf("mail source %q missing imap.host", source.ID)
		}
		if source.IMAP.Port == 0 {
			return fmt.Errorf("mail source %q missing imap.port", source.ID)
		}
		if strings.TrimSpace(source.SMTP.Host) == "" {
			return fmt.Errorf("mail source %q missing smtp.host", source.ID)
		}
		if source.SMTP.Port == 0 {
			return fmt.Errorf("mail source %q missing smtp.port", source.ID)
		}
	}
	return nil
}

func explicitMailSourceUsesGoogleOAuth(source SourceConfig) bool {
	provider := strings.ToLower(strings.TrimSpace(source.Provider))
	if provider != "gmail" && provider != "gmail_api" {
		return false
	}
	return strings.TrimSpace(source.Google.RefreshToken) != ""
}

// checkFilePermissions ensures the config file has secure permissions
func checkFilePermissions(configPath string) error {
	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}

	mode := info.Mode()
	// Check if group or others have any permissions (Unix-like systems)
	if mode&0o077 != 0 {
		logger.Warn("Config file has loose permissions (%v). Consider running: chmod 600 %s", mode, configPath)
	}

	return nil
}
