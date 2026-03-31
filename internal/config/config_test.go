package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsGmailOAuth(t *testing.T) {
	t.Run("false when RefreshToken is empty", func(t *testing.T) {
		c := &Config{}
		if c.IsGmailOAuth() {
			t.Error("expected IsGmailOAuth() == false when RefreshToken is empty")
		}
	})

	t.Run("true when RefreshToken is set", func(t *testing.T) {
		c := &Config{}
		c.Gmail.RefreshToken = "some-refresh-token"
		if !c.IsGmailOAuth() {
			t.Error("expected IsGmailOAuth() == true when RefreshToken is set")
		}
	})
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write a minimal config file so Load can read it back (needs 0600 perms)
	original := &Config{}
	original.Vendor = "gmail"
	original.Server.Host = "imap.gmail.com"
	original.Server.Port = 993
	original.Gmail.RefreshToken = "rt-abc123"
	original.Gmail.AccessToken = "at-xyz"
	original.Gmail.TokenExpiry = "2026-01-01T00:00:00Z"
	original.Gmail.Email = "user@gmail.com"

	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify the saved file has 0600 permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected permissions 0600, got %04o", perm)
	}

	// Load back using Load() — it calls checkFilePermissions which passes for 0600
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.Gmail.RefreshToken != original.Gmail.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", loaded.Gmail.RefreshToken, original.Gmail.RefreshToken)
	}
	if loaded.Gmail.AccessToken != original.Gmail.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", loaded.Gmail.AccessToken, original.Gmail.AccessToken)
	}
	if loaded.Gmail.TokenExpiry != original.Gmail.TokenExpiry {
		t.Errorf("TokenExpiry: got %q, want %q", loaded.Gmail.TokenExpiry, original.Gmail.TokenExpiry)
	}
	if loaded.Gmail.Email != original.Gmail.Email {
		t.Errorf("Email: got %q, want %q", loaded.Gmail.Email, original.Gmail.Email)
	}
	if loaded.Server.Host != original.Server.Host {
		t.Errorf("Server.Host: got %q, want %q", loaded.Server.Host, original.Server.Host)
	}
	if loaded.Server.Port != original.Server.Port {
		t.Errorf("Server.Port: got %d, want %d", loaded.Server.Port, original.Server.Port)
	}
}

func TestDefaultOllamaModel(t *testing.T) {
	c := &Config{}
	c.applyDefaults()
	if c.Ollama.Model != "gemma3:4b" {
		t.Errorf("expected default Ollama model %q, got %q", "gemma3:4b", c.Ollama.Model)
	}
}

func TestDefaultOllamaModelNotOverridden(t *testing.T) {
	c := &Config{}
	c.Ollama.Model = "custom-model"
	c.applyDefaults()
	if c.Ollama.Model != "custom-model" {
		t.Errorf("applyDefaults() should not override an already-set model, got %q", c.Ollama.Model)
	}
}

func TestValidateSkipsCredentialsForGmailOAuth(t *testing.T) {
	c := &Config{}
	c.Gmail.RefreshToken = "token"
	c.Server.Host = "imap.gmail.com"
	c.Server.Port = 993
	// Username and Password intentionally left empty

	if err := c.validate(); err != nil {
		t.Errorf("validate() should not require credentials for Gmail OAuth, got: %v", err)
	}
}

func TestValidateRequiresCredentialsWithoutOAuth(t *testing.T) {
	c := &Config{}
	c.Server.Host = "imap.example.com"
	c.Server.Port = 993
	// No refresh token, no credentials

	if err := c.validate(); err == nil {
		t.Error("validate() should return error when credentials are missing and OAuth is not configured")
	}
}

func TestDaemonDefaults(t *testing.T) {
	c := &Config{}
	c.applyDefaults()

	if c.Daemon.Port != 7272 {
		t.Errorf("expected Daemon.Port == 7272, got %d", c.Daemon.Port)
	}
	if c.Daemon.BindAddr != "127.0.0.1" {
		t.Errorf("expected Daemon.BindAddr == %q, got %q", "127.0.0.1", c.Daemon.BindAddr)
	}
	if !strings.HasSuffix(c.Daemon.PidFile, filepath.Join("herald", "daemon.pid")) {
		t.Errorf("unexpected PidFile: %s", c.Daemon.PidFile)
	}
	if !strings.HasSuffix(c.Daemon.LogFile, filepath.Join("herald", "daemon.log")) {
		t.Errorf("unexpected LogFile: %s", c.Daemon.LogFile)
	}
}
