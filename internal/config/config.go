package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
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
}

// Config represents the application configuration
type Config struct {
	Vendor string `yaml:"vendor"` // gmail | protonmail | fastmail | outlook | icloud
	Cache  struct {
		DatabasePath  string `yaml:"database_path,omitempty"`
		StoragePolicy string `yaml:"storage_policy,omitempty"` // lightweight | no_attachments | preserve_all
	} `yaml:"cache,omitempty"`
	Compose struct {
		Signature struct {
			Text string `yaml:"text,omitempty"`
		} `yaml:"signature,omitempty"`
	} `yaml:"compose,omitempty"`
	Keyboard struct {
		Profile      string `yaml:"profile,omitempty"`
		CustomKeymap string `yaml:"custom_keymap,omitempty"`
	} `yaml:"keyboard,omitempty"`
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
	Semantic struct {
		Enabled   bool    `yaml:"enabled"`    // default: true when Ollama configured
		Model     string  `yaml:"model"`      // default: configured Ollama embedding model
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
		APIKey  string `yaml:"api_key"`
		BaseURL string `yaml:"base_url"` // default: "https://api.openai.com/v1"
		Model   string `yaml:"model"`    // default: "gpt-4o"
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
	"outlook":    {"outlook.office365.com", 993, "smtp.office365.com", 587},
	"fastmail":   {"imap.fastmail.com", 993, "smtp.fastmail.com", 587},
	"icloud":     {"imap.mail.me.com", 993, "smtp.mail.me.com", 587},
}

const (
	CacheStoragePolicyLightweight   = "lightweight"
	CacheStoragePolicyNoAttachments = "no_attachments"
	CacheStoragePolicyPreserveAll   = "preserve_all"
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

// EffectiveEmbeddingModel returns the configured embedding model, preferring
// semantic.model when it is explicitly set and otherwise falling back to the
// Ollama embedding model.
func (c *Config) EffectiveEmbeddingModel() string {
	if strings.TrimSpace(c.Semantic.Model) != "" {
		return strings.TrimSpace(c.Semantic.Model)
	}
	return strings.TrimSpace(c.Ollama.EmbeddingModel)
}

func (c Config) NormalizedSources() []SourceConfig {
	if len(c.Sources) > 0 {
		return normalizeExplicitSources(c.Sources)
	}
	return []SourceConfig{legacyDefaultMailSource(c)}
}

func normalizeExplicitSources(sources []SourceConfig) []SourceConfig {
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
		normalized = append(normalized, source)
	}
	return normalized
}

func legacyDefaultMailSource(c Config) SourceConfig {
	return SourceConfig{
		ID:          string(models.DefaultMailSourceID),
		Kind:        string(models.SourceKindMail),
		Provider:    "imap",
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
	if c.Ollama.Model == "" {
		c.Ollama.Model = "gemma3:4b"
	}
	if c.Ollama.EmbeddingModel == "" {
		c.Ollama.EmbeddingModel = "nomic-embed-text-v2-moe"
	}
	if c.Semantic.Model == "" {
		c.Semantic.Model = c.Ollama.EmbeddingModel
	}
	if c.Sync.Interval == 0 {
		c.Sync.Interval = 60
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
	if c.OpenAI.BaseURL == "" {
		c.OpenAI.BaseURL = "https://api.openai.com/v1"
	}
	if c.OpenAI.Model == "" {
		c.OpenAI.Model = "gpt-4o"
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
