package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
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