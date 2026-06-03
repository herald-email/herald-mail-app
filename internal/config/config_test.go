package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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

func setHomeForTest(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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

func TestConfigNormalizedSourcesFromLegacyMailConfig(t *testing.T) {
	var cfg Config
	cfg.Credentials.Username = "user@example.com"
	cfg.Credentials.Password = "secret"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 1143
	cfg.SMTP.Host = "127.0.0.1"
	cfg.SMTP.Port = 1025
	cfg.Compose.Signature.Text = "-- \nLegacy Signature"

	sources := cfg.NormalizedSources()
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].ID != "default-mail" || sources[0].Kind != "mail" || sources[0].Provider != "imap" || sources[0].AccountID != "default" {
		t.Fatalf("source = %#v, want default mail source", sources[0])
	}
	if sources[0].Credentials.Username != "user@example.com" || sources[0].IMAP.Host != "127.0.0.1" || sources[0].SMTP.Port != 1025 {
		t.Fatalf("legacy fields were not copied into source: %#v", sources[0])
	}
	if got := sources[0].Compose.Signature.Text; got != cfg.Compose.Signature.Text {
		t.Fatalf("legacy source signature = %q, want %q", got, cfg.Compose.Signature.Text)
	}
}

func TestConfigNormalizedSourcesKeepsExplicitSources(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{
				ID:        "work-mail",
				Kind:      "mail",
				Provider:  "imap",
				AccountID: "work",
				Compose:   ComposeConfig{Signature: SignatureConfig{Text: "-- \nWork Signature"}},
			},
			{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work"},
		},
	}

	sources := cfg.NormalizedSources()
	if len(sources) != 2 {
		t.Fatalf("len(sources) = %d, want 2", len(sources))
	}
	if sources[0].ID != "work-mail" || sources[0].Kind != "mail" || sources[0].Provider != "imap" || sources[0].AccountID != "work" {
		t.Fatalf("first source = %#v, want explicit work mail source", sources[0])
	}
	if got := sources[0].Compose.Signature.Text; got != "-- \nWork Signature" {
		t.Fatalf("explicit mail source signature = %q, want work signature", got)
	}
	if sources[1].ID != "work-calendar" || sources[1].Kind != "calendar" || sources[1].Provider != "google_calendar" || sources[1].AccountID != "work" {
		t.Fatalf("second source = %#v, want explicit work calendar source", sources[1])
	}
}

func TestConfigNormalizedSourcesUsesLegacySignatureAsExplicitMailFallback(t *testing.T) {
	var cfg Config
	cfg.Compose.Signature.Text = "-- \nDefault Signature"
	cfg.Sources = []SourceConfig{
		{ID: "work-mail", Kind: "mail", Provider: "imap", AccountID: "work"},
		{
			ID:        "personal-mail",
			Kind:      "mail",
			Provider:  "imap",
			AccountID: "personal",
			Compose:   ComposeConfig{Signature: SignatureConfig{Text: "-- \nPersonal Signature"}},
		},
	}

	sources := cfg.NormalizedSources()
	if got := sources[0].Compose.Signature.Text; got != cfg.Compose.Signature.Text {
		t.Fatalf("fallback signature = %q, want legacy default %q", got, cfg.Compose.Signature.Text)
	}
	if got := sources[1].Compose.Signature.Text; got != "-- \nPersonal Signature" {
		t.Fatalf("explicit signature = %q, want personal signature", got)
	}
}

func TestConfigNormalizedSourcesDefaultsCalendarProviders(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{Kind: "calendar", AccountID: "personal"},
			{ID: "family-caldav", Kind: "calendar", Provider: "caldav", CalDAV: CalDAVConfig{URL: "https://cal.example/cal"}},
		},
	}

	sources := cfg.NormalizedSources()
	if len(sources) != 2 {
		t.Fatalf("len(sources) = %d, want 2", len(sources))
	}
	if sources[0].ID != "default-calendar" || sources[0].Provider != "google_calendar" || sources[0].AccountID != "personal" {
		t.Fatalf("default calendar source = %#v, want default-calendar/google_calendar/personal", sources[0])
	}
	if sources[1].ID != "family-caldav" || sources[1].Provider != "caldav" || sources[1].CalDAV.URL != "https://cal.example/cal" {
		t.Fatalf("caldav source = %#v, want preserved caldav URL", sources[1])
	}
}

func TestValidateExplicitSourcesDoesNotRequireLegacyMailFields(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{
				ID:          "work-mail",
				Kind:        "mail",
				Provider:    "imap",
				DisplayName: "Work Mail",
				AccountID:   "work",
				Credentials: CredentialsConfig{Username: "user@example.com", Password: "secret"},
				IMAP:        ServerConfig{Host: "imap.example.com", Port: 993},
				SMTP:        ServerConfig{Host: "smtp.example.com", Port: 587},
			},
			{
				ID:          "work-calendar",
				Kind:        "calendar",
				Provider:    "google_calendar",
				DisplayName: "Work Calendar",
				AccountID:   "work",
				Google:      GoogleConfig{RefreshToken: "refresh-token", Email: "user@example.com"},
			},
		},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("explicit source config should validate without legacy credentials/server fields: %v", err)
	}
}

func TestValidateExplicitGmailOAuthSourceDoesNotRequireIMAPSMTP(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{
				ID:          "work-gmail-mail",
				Kind:        "mail",
				Provider:    "gmail",
				DisplayName: "Work Gmail",
				AccountID:   "work",
				Google: GoogleConfig{
					Email:        "work@gmail.com",
					RefreshToken: "refresh-token",
				},
			},
			{
				ID:          "work-gmail-calendar",
				Kind:        "calendar",
				Provider:    "google_calendar",
				DisplayName: "Work Calendar",
				AccountID:   "work",
				Google: GoogleConfig{
					Email:        "work@gmail.com",
					RefreshToken: "refresh-token",
				},
			},
		},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("Gmail OAuth source should validate without IMAP/SMTP fields: %v", err)
	}
}

func TestAccountGroupsGroupMailAndCalendarByAccountID(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{
				ID:          "work-mail",
				Kind:        "mail",
				Provider:    "gmail",
				DisplayName: "Work Gmail",
				AccountID:   "work",
				Credentials: CredentialsConfig{Username: "work@example.com"},
			},
			{
				ID:          "work-calendar",
				Kind:        "calendar",
				Provider:    "google_calendar",
				DisplayName: "Work Calendar",
				AccountID:   "work",
				Google:      GoogleConfig{Email: "work@example.com"},
			},
			{
				ID:          "family-calendar",
				Kind:        "calendar",
				Provider:    "caldav",
				DisplayName: "Family CalDAV",
				AccountID:   "family",
				CalDAV:      CalDAVConfig{URL: "https://caldav.example/family"},
			},
		},
	}

	groups := cfg.AccountGroups()
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2: %#v", len(groups), groups)
	}
	if groups[0].AccountID != "family" || groups[0].Capability != "Calendar" || groups[0].DisplayName != "Family CalDAV" {
		t.Fatalf("first group = %#v, want family calendar-only group", groups[0])
	}
	if groups[1].AccountID != "work" || groups[1].Capability != "Mail + Calendar" || groups[1].Address != "work@example.com" {
		t.Fatalf("second group = %#v, want work mail+calendar group", groups[1])
	}
}

func TestRemoveAccountSourcesBlocksLastMailSourceAndPreservesOtherSources(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{ID: "work-mail", Kind: "mail", AccountID: "work", Credentials: CredentialsConfig{Username: "work@example.com"}, IMAP: ServerConfig{Host: "imap.example.com", Port: 993}, SMTP: ServerConfig{Host: "smtp.example.com", Port: 587}},
			{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work"},
			{ID: "personal-mail", Kind: "mail", AccountID: "personal", Credentials: CredentialsConfig{Username: "me@example.com"}, IMAP: ServerConfig{Host: "imap.example.com", Port: 993}, SMTP: ServerConfig{Host: "smtp.example.com", Port: 587}},
		},
	}

	updated, err := cfg.RemoveAccountSources("work")
	if err != nil {
		t.Fatalf("RemoveAccountSources(work) returned error: %v", err)
	}
	if len(updated.Sources) != 1 || updated.Sources[0].ID != "personal-mail" {
		t.Fatalf("updated sources = %#v, want only personal-mail", updated.Sources)
	}

	if _, err := updated.RemoveAccountSources("personal"); err == nil || !strings.Contains(err.Error(), "last mail") {
		t.Fatalf("expected deleting final mail source to be blocked, got %v", err)
	}
}

func TestLoadRoundTripPreservesExplicitSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := minimalOAuthConfig()
	original.Sources = []SourceConfig{
		{
			ID:          "work-mail",
			Kind:        "mail",
			Provider:    "imap",
			DisplayName: "Work Mail",
			AccountID:   "work",
			Credentials: CredentialsConfig{Username: "user@example.com", Password: "secret"},
			IMAP:        ServerConfig{Host: "imap.example.com", Port: 993},
			SMTP:        ServerConfig{Host: "smtp.example.com", Port: 587},
		},
		{
			ID:          "work-calendar",
			Kind:        "calendar",
			Provider:    "google_calendar",
			DisplayName: "Work Calendar",
			AccountID:   "work",
			Google:      GoogleConfig{RefreshToken: "refresh-token", APIBaseURL: "https://calendar.test", TokenURL: "https://oauth.test/token"},
		},
		{
			ID:          "family-calendar",
			Kind:        "calendar",
			Provider:    "caldav",
			DisplayName: "Family Calendar",
			AccountID:   "family",
			CalDAV:      CalDAVConfig{URL: "https://caldav.test/calendars/family/", Username: "family", Password: "secret"},
		},
	}
	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	sources := loaded.NormalizedSources()
	if len(sources) != 3 {
		t.Fatalf("len(sources) = %d, want 3", len(sources))
	}
	if sources[0].DisplayName != "Work Mail" || sources[0].IMAP.Host != "imap.example.com" {
		t.Fatalf("mail source did not roundtrip: %#v", sources[0])
	}
	if sources[1].DisplayName != "Work Calendar" || sources[1].Google.RefreshToken != "refresh-token" || sources[1].Google.APIBaseURL != "https://calendar.test" || sources[1].Google.TokenURL != "https://oauth.test/token" {
		t.Fatalf("calendar source did not roundtrip: %#v", sources[1])
	}
	if sources[2].Provider != "caldav" || sources[2].CalDAV.URL != "https://caldav.test/calendars/family/" || sources[2].CalDAV.Username != "family" {
		t.Fatalf("caldav source did not roundtrip: %#v", sources[2])
	}
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

func TestSaveRoundTrip_ComposeSignature(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := minimalOAuthConfig()
	original.Compose.Signature.Text = "-- \nRowan\nHerald Labs"

	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if got := loaded.Compose.Signature.Text; got != original.Compose.Signature.Text {
		t.Fatalf("Compose.Signature.Text = %q, want %q", got, original.Compose.Signature.Text)
	}
}

func TestLoadKeyboardConfigDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := minimalOAuthConfig()
	original.Keyboard.Profile = "vim"
	original.Keyboard.CustomKeymap = "~/.config/herald/keymaps/work.yaml"
	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if got := loaded.Keyboard.Profile; got != "vim" {
		t.Fatalf("Keyboard.Profile = %q, want vim", got)
	}
	if got := loaded.Keyboard.CustomKeymap; got != original.Keyboard.CustomKeymap {
		t.Fatalf("Keyboard.CustomKeymap = %q, want %q", got, original.Keyboard.CustomKeymap)
	}

	defaultPath := filepath.Join(dir, "default.yaml")
	defaultConfig := minimalOAuthConfig()
	if err := defaultConfig.Save(defaultPath); err != nil {
		t.Fatalf("Save(default) failed: %v", err)
	}
	loadedDefault, err := Load(defaultPath)
	if err != nil {
		t.Fatalf("Load(default) failed: %v", err)
	}
	if got := loadedDefault.Keyboard.Profile; got != "default" {
		t.Fatalf("default Keyboard.Profile = %q, want default", got)
	}
}

func TestLoadCalendarWeekStartDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calendar.yaml")
	body := []byte(strings.Join([]string{
		"vendor: gmail",
		"server:",
		"  host: imap.gmail.com",
		"  port: 993",
		"gmail:",
		"  refresh_token: rt-token",
		"  email: user@gmail.com",
		"calendar:",
		"  week_start: sunday",
		"",
	}, "\n"))
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if got := calendarWeekStartForConfigTest(t, loaded); got != "sunday" {
		t.Fatalf("Calendar.WeekStart = %q, want sunday", got)
	}

	defaultPath := filepath.Join(dir, "default.yaml")
	defaultConfig := minimalOAuthConfig()
	if err := defaultConfig.Save(defaultPath); err != nil {
		t.Fatalf("Save(default) failed: %v", err)
	}
	loadedDefault, err := Load(defaultPath)
	if err != nil {
		t.Fatalf("Load(default) failed: %v", err)
	}
	if got := calendarWeekStartForConfigTest(t, loadedDefault); got != "monday" {
		t.Fatalf("default Calendar.WeekStart = %q, want monday", got)
	}
}

func TestCalendarSelectedCalendarsConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calendar-selected.yaml")
	original := minimalOAuthConfig()
	original.Calendar.SelectedCalendars = []string{
		"demo-calendar|default|work",
		"family-calendar|family|home",
	}

	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if !reflect.DeepEqual(loaded.Calendar.SelectedCalendars, original.Calendar.SelectedCalendars) {
		t.Fatalf("Calendar.SelectedCalendars = %#v, want %#v", loaded.Calendar.SelectedCalendars, original.Calendar.SelectedCalendars)
	}
}

func calendarWeekStartForConfigTest(t *testing.T, cfg *Config) string {
	t.Helper()
	root := reflect.ValueOf(cfg).Elem()
	calendarField := root.FieldByName("Calendar")
	if !calendarField.IsValid() {
		t.Fatal("Config is missing Calendar settings")
	}
	weekStart := calendarField.FieldByName("WeekStart")
	if !weekStart.IsValid() {
		t.Fatal("Config.Calendar is missing WeekStart")
	}
	return weekStart.String()
}

func TestNormalizeCacheStoragePolicyDefaultsToNoAttachments(t *testing.T) {
	for _, input := range []string{"", "unknown", " lightweight "} {
		got := NormalizeCacheStoragePolicy(input)
		if input == " lightweight " {
			if got != CacheStoragePolicyLightweight {
				t.Fatalf("NormalizeCacheStoragePolicy(%q) = %q, want %q", input, got, CacheStoragePolicyLightweight)
			}
			continue
		}
		if got != CacheStoragePolicyNoAttachments {
			t.Fatalf("NormalizeCacheStoragePolicy(%q) = %q, want %q", input, got, CacheStoragePolicyNoAttachments)
		}
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

func TestEnsureCacheDatabasePathKeepsExplicitRelativeYAMLPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.yaml")
	wantPath := filepath.Join("custom", "relative-cache.db")

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
		t.Fatalf("got cache path %q, want explicit relative path %q", got, wantPath)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after) failed: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("EnsureCacheDatabasePath rewrote an explicit relative cache path")
	}
}

func TestEnsureCacheDatabasePathExpandsExplicitHomeYAMLPath(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	setHomeForTest(t, home)
	path := filepath.Join(dir, "work.yaml")
	configuredPath := filepath.Join("~", ".herald", "cached", "explicit-cache.db")
	wantPath := filepath.Join(home, ".herald", "cached", "explicit-cache.db")

	original := minimalOAuthConfig()
	original.Cache.DatabasePath = configuredPath
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
		t.Fatalf("got cache path %q, want expanded path %q", got, wantPath)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after) failed: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("EnsureCacheDatabasePath rewrote an explicit home-relative cache path")
	}
}

func TestEnsureCacheDatabasePathGeneratesAndPersistsPerConfigPath(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	setHomeForTest(t, home)
	workingDir := filepath.Join(dir, "working-dir")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(workingDir) failed: %v", err)
	}
	chdirForTest(t, workingDir)
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
	want := filepath.Join(home, ".herald", "cached", "work-account.db")
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
	home := filepath.Join(dir, "home")
	setHomeForTest(t, home)
	workingDir := filepath.Join(dir, "working-dir")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(workingDir) failed: %v", err)
	}
	chdirForTest(t, workingDir)
	path := filepath.Join(dir, "work-account.yaml")

	cacheDir := filepath.Join(home, ".herald", "cached")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	existing := filepath.Join(cacheDir, "work-account.db")
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
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(filepath.ToSlash(cacheDir)) + `/work-account-[0-9]{8}-[a-f0-9]{6}\.db$`)
	if !pattern.MatchString(filepath.ToSlash(got)) {
		t.Fatalf("got disambiguated path %q, want absolute ~/.herald/cached/work-account-YYYYMMDD-xxxxxx.db", got)
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

func TestDefaultEmbeddingModel(t *testing.T) {
	c := &Config{}
	c.applyDefaults()
	if c.Ollama.EmbeddingModel != "nomic-embed-text-v2-moe" {
		t.Errorf("expected default Ollama embedding model %q, got %q", "nomic-embed-text-v2-moe", c.Ollama.EmbeddingModel)
	}
	if c.Semantic.Model != "nomic-embed-text-v2-moe" {
		t.Errorf("expected semantic model to follow default embedding model, got %q", c.Semantic.Model)
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

func TestNotificationDefaults(t *testing.T) {
	c := &Config{}
	c.applyDefaults()

	if !c.Notifications.Enabled {
		t.Fatal("notifications.enabled should default true")
	}
	if !c.Notifications.NewMail {
		t.Fatal("notifications.new_mail should default true")
	}
	if !c.Notifications.SyncFailures {
		t.Fatal("notifications.sync_failures should default true")
	}
	if c.Notifications.DeletionCompletion || c.Notifications.ClassificationCompletion || c.Notifications.ChatResults || c.Notifications.Sound {
		t.Fatalf("optional notification events should default false, got %#v", c.Notifications)
	}
}

func TestNotificationExplicitFalsePreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
gmail:
  refresh_token: token
server:
  host: imap.gmail.com
  port: 993
notifications:
  enabled: false
  new_mail: false
  sync_failures: false
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Notifications.Enabled || loaded.Notifications.NewMail || loaded.Notifications.SyncFailures {
		t.Fatalf("explicit false notification settings were not preserved: %#v", loaded.Notifications)
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
