package config

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func minimalOAuthConfig() *Config {
	c := &Config{}
	c.Gmail.RefreshToken = "rt-token"
	c.Gmail.Email = "user@example.com"
	c.Server.Host = "imap.example.com"
	c.Server.Port = 993
	return c
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) failed: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	})
}

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

func TestEnsureCacheDatabasePathUsesConfiguredYAMLPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.yaml")
	wantPath := filepath.Join(dir, "explicit-cache.db")

	original := minimalOAuthConfig()
	original.Cache.DatabasePath = wantPath
	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(before) failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	got, err := EnsureCacheDatabasePath(path, loaded)
	if err != nil {
		t.Fatalf("EnsureCacheDatabasePath() failed: %v", err)
	}
	if got != wantPath {
		t.Fatalf("got cache path %q, want %q", got, wantPath)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after) failed: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("EnsureCacheDatabasePath rewrote a config that already had a cache path")
	}
}

func TestEnsureCacheDatabasePathGeneratesAndPersistsPerConfigPath(t *testing.T) {
	dir := t.TempDir()
	chdirForTest(t, dir)
	path := filepath.Join(dir, "work-account.yaml")

	original := minimalOAuthConfig()
	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	got, err := EnsureCacheDatabasePath(path, loaded)
	if err != nil {
		t.Fatalf("EnsureCacheDatabasePath() failed: %v", err)
	}
	want := filepath.Join("herald", "cached", "work-account.db")
	if got != want {
		t.Fatalf("got cache path %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Dir(got)); err != nil {
		t.Fatalf("expected cache directory to exist: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(rewritten) failed: %v", err)
	}
	if reloaded.Cache.DatabasePath != want {
		t.Fatalf("persisted cache path %q, want %q", reloaded.Cache.DatabasePath, want)
	}
}

func TestEnsureCacheDatabasePathDisambiguatesExistingGeneratedPath(t *testing.T) {
	dir := t.TempDir()
	chdirForTest(t, dir)
	path := filepath.Join(dir, "work-account.yaml")

	if err := os.MkdirAll(filepath.Join("herald", "cached"), 0o700); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	existing := filepath.Join("herald", "cached", "work-account.db")
	if err := os.WriteFile(existing, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing) failed: %v", err)
	}

	original := minimalOAuthConfig()
	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	got, err := EnsureCacheDatabasePath(path, loaded)
	if err != nil {
		t.Fatalf("EnsureCacheDatabasePath() failed: %v", err)
	}
	if got == existing {
		t.Fatalf("reused existing cache path %q", got)
	}
	pattern := regexp.MustCompile(`^herald/cached/work-account-[0-9]{8}-[a-f0-9]{6}\.db$`)
	if !pattern.MatchString(filepath.ToSlash(got)) {
		t.Fatalf("got disambiguated path %q, want herald/cached/work-account-YYYYMMDD-xxxxxx.db", got)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(rewritten) failed: %v", err)
	}
	if reloaded.Cache.DatabasePath != got {
		t.Fatalf("persisted cache path %q, want %q", reloaded.Cache.DatabasePath, got)
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

func TestConfigSave_SyncAndAIFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := &Config{}
	original.Gmail.RefreshToken = "rt-token" // use OAuth so no password required
	original.Gmail.Email = "user@gmail.com"
	original.Server.Host = "imap.gmail.com"
	original.Server.Port = 993

	original.AI.Provider = "claude"
	original.AI.LocalMaxConcurrency = 1
	original.AI.ExternalMaxConcurrency = 4
	original.AI.BackgroundQueueLimit = 64
	original.AI.PauseBackgroundWhileInteractive = true
	original.Claude.APIKey = "test-api-key"
	original.Claude.Model = "claude-sonnet-4-6"
	original.Sync.PollIntervalMinutes = 10
	original.Sync.IDLEEnabled = true
	original.Cleanup.ScheduleHours = 24

	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.AI.Provider != "claude" {
		t.Errorf("AI.Provider: got %q, want %q", loaded.AI.Provider, "claude")
	}
	if loaded.AI.LocalMaxConcurrency != 1 {
		t.Errorf("AI.LocalMaxConcurrency: got %d, want 1", loaded.AI.LocalMaxConcurrency)
	}
	if loaded.AI.ExternalMaxConcurrency != 4 {
		t.Errorf("AI.ExternalMaxConcurrency: got %d, want 4", loaded.AI.ExternalMaxConcurrency)
	}
	if loaded.AI.BackgroundQueueLimit != 64 {
		t.Errorf("AI.BackgroundQueueLimit: got %d, want 64", loaded.AI.BackgroundQueueLimit)
	}
	if !loaded.AI.PauseBackgroundWhileInteractive {
		t.Error("AI.PauseBackgroundWhileInteractive: got false, want true")
	}
	if loaded.Claude.APIKey != "test-api-key" {
		t.Errorf("Claude.APIKey: got %q, want %q", loaded.Claude.APIKey, "test-api-key")
	}
	if loaded.Sync.PollIntervalMinutes != 10 {
		t.Errorf("Sync.PollIntervalMinutes: got %d, want 10", loaded.Sync.PollIntervalMinutes)
	}
	if !loaded.Sync.IDLEEnabled {
		t.Error("Sync.IDLEEnabled: got false, want true")
	}
	if loaded.Cleanup.ScheduleHours != 24 {
		t.Errorf("Cleanup.ScheduleHours: got %d, want 24", loaded.Cleanup.ScheduleHours)
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

func TestAIDefaults(t *testing.T) {
	c := &Config{}
	c.applyDefaults()

	if c.AI.Provider != "ollama" {
		t.Errorf("expected AI.Provider == %q, got %q", "ollama", c.AI.Provider)
	}
	if c.AI.LocalMaxConcurrency != 1 {
		t.Errorf("expected AI.LocalMaxConcurrency == 1, got %d", c.AI.LocalMaxConcurrency)
	}
	if c.AI.ExternalMaxConcurrency != 4 {
		t.Errorf("expected AI.ExternalMaxConcurrency == 4, got %d", c.AI.ExternalMaxConcurrency)
	}
	if c.AI.BackgroundQueueLimit != 64 {
		t.Errorf("expected AI.BackgroundQueueLimit == 64, got %d", c.AI.BackgroundQueueLimit)
	}
	if !c.AI.PauseBackgroundWhileInteractive {
		t.Error("expected AI.PauseBackgroundWhileInteractive == true")
	}
}
