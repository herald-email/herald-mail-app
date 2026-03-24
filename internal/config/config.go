package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Vendor      string `yaml:"vendor"` // gmail | protonmail | fastmail | outlook | icloud
	Credentials struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"credentials"`
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	SMTP struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"smtp"`
	Ollama struct {
		Host           string `yaml:"host"`            // default: http://localhost:11434
		Model          string `yaml:"model"`           // default: gemma2:2b
		EmbeddingModel string `yaml:"embedding_model"` // default: nomic-embed-text
	} `yaml:"ollama"`
	Sync struct {
		Idle       bool `yaml:"idle"`       // default: true
		Interval   int  `yaml:"interval"`   // fallback poll seconds, default: 60
		Background bool `yaml:"background"` // sync other folders, default: true
		Notify     bool `yaml:"notify"`     // status bar flash, default: true
	} `yaml:"sync"`
	Semantic struct {
		Enabled   bool    `yaml:"enabled"`    // default: true when Ollama configured
		Model     string  `yaml:"model"`      // default: nomic-embed-text
		BatchSize int     `yaml:"batch_size"` // default: 20
		MinScore  float64 `yaml:"min_score"`  // default: 0.65
	} `yaml:"semantic"`
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

// applyVendorPreset fills in server/smtp host+port when a vendor shortcut is set
// and the user has not provided explicit values.
func (c *Config) applyVendorPreset() {
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

// applyDefaults sets sensible defaults for optional config fields
func (c *Config) applyDefaults() {
	if c.Ollama.EmbeddingModel == "" {
		c.Ollama.EmbeddingModel = "nomic-embed-text"
	}
	if c.Sync.Interval == 0 {
		c.Sync.Interval = 60
	}
	if c.Semantic.BatchSize == 0 {
		c.Semantic.BatchSize = 20
	}
	if c.Semantic.MinScore == 0 {
		c.Semantic.MinScore = 0.65
	}
	// Enable IDLE and background sync by default
	// (zero-value bool is false; we use a separate flag to detect "not set")
	// Users must explicitly set idle: false to disable
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

	config.applyVendorPreset()
	config.applyDefaults()

	// Validate required fields
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// validate checks that all required configuration fields are present
func (c *Config) validate() error {
	if c.Credentials.Username == "" {
		return fmt.Errorf("missing credentials.username")
	}
	if c.Credentials.Password == "" {
		return fmt.Errorf("missing credentials.password")
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
		fmt.Printf("Warning: Config file has loose permissions (%v). Consider running: chmod 600 %s\n", 
			mode, configPath)
	}

	return nil
}