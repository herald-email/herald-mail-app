package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"gopkg.in/yaml.v3"
)

func TestSettingsIdentitiesEntryVisible(t *testing.T) {
	s := NewSettings(SettingsModePanel, identitySettingsFixture())
	s.panelSection = settingsPanelSectionAccounts
	s.accountsMenuChoice = "account:work"
	if !s.openSelectedAccountFromList() {
		t.Fatal("open account")
	}
	rendered := stripANSI(renderSettingsViewForTest(t, s, 220, 50))
	calendar := strings.Index(rendered, "Include Google Calendar")
	identities := strings.Index(rendered, "Edit sending identities")
	if calendar < 0 || identities <= calendar || strings.Contains(rendered, "Edit account details") {
		t.Fatalf("identity button should follow Calendar without top selector:\n%s", rendered)
	}
}

func identitySettingsFixture() *config.Config {
	return &config.Config{Sources: []config.SourceConfig{
		{ID: "mail-work", AccountID: "work", Kind: "mail", Provider: "gmail", Google: config.GoogleConfig{Email: "me@example.test", RefreshToken: "keep-token"}, Compose: config.ComposeConfig{Signature: config.SignatureConfig{Text: "keep signature"}}},
		{ID: "calendar-work", AccountID: "work", Kind: "calendar", Provider: "google_calendar", Google: config.GoogleConfig{Email: "me@example.test", RefreshToken: "calendar-token"}},
		{ID: "mail-other", AccountID: "other", Kind: "mail", Provider: "imap", Credentials: config.CredentialsConfig{Username: "other@example.test", Password: "keep-password"}},
	}}
}

func openIdentitySettings(t *testing.T, cfg *config.Config) *Settings {
	t.Helper()
	s := NewSettings(SettingsModePanel, cfg)
	s.panelSection = settingsPanelSectionAccounts
	s.accountsMenuChoice = "account:work"
	s.buildForm()
	if !s.openSelectedAccountFromList() {
		t.Fatal("open account")
	}

	for i := 0; i < 8 && s.form.GetFocusedField().GetKey() != "account.identities"; i++ {
		s = updateSettingsForTest(t, s, huh.NextField())
	}
	if s.form.GetFocusedField().GetKey() != "account.identities" {
		t.Fatal("identity button not reachable")
	}

	s, _ = updateAndPumpSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.panelSection != settingsPanelSectionIdentities {
		t.Fatalf("failed to open manager: %s\n%s", s.panelSection, s.form.View())
	}
	return s
}

func selectIdentityAction(s *Settings, choice string) tea.Cmd {
	s.identity.choice = choice
	return s.completeIdentityForm()
}

func TestSettingsIdentitiesLifecycleAndCancel(t *testing.T) {
	cfg := identitySettingsFixture()
	s := openIdentitySettings(t, cfg)
	selectIdentityAction(s, "add")
	s.identity.name = "Public"
	s.identity.address = "public@example.test"
	s.identity.makeDefault = true
	s.completeIdentityForm()
	if cfg.Sources[0].Compose.DefaultIdentity != "" || len(cfg.Sources[0].Compose.Identities) != 0 {
		t.Fatal("staged edit leaked into active config")
	}
	if s.identity.compose.DefaultIdentity != "public@example.test" {
		t.Fatal("default not staged")
	}
	selectIdentityAction(s, "0")
	s.identity.address = "edited@example.test"
	s.completeIdentityForm()
	if s.identity.compose.DefaultIdentity != "edited@example.test" {
		t.Fatal("editing default did not follow its address")
	}
	selectIdentityAction(s, "0")
	s.identity.address = "discard@example.test"
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyEscape})
	if s.identity.compose.Identities[0].Address != "edited@example.test" {
		t.Fatal("escape applied editor")
	}
	selectIdentityAction(s, "0")
	s.identity.action = "remove"
	s.completeIdentityForm()
	if s.panelSection != settingsPanelSectionIdentityRemove || s.identity.remove {
		t.Fatal("removal needs safe confirmation")
	}
	s.completeIdentityForm()
	if len(s.identity.compose.Identities) != 1 {
		t.Fatal("cancel removed identity")
	}
	s.identity.action = "remove"
	s.completeIdentityForm()
	s.identity.remove = true
	s.completeIdentityForm()
	if len(s.identity.compose.Identities) != 0 || s.identity.compose.DefaultIdentity != "" {
		t.Fatal("remove default must restore primary")
	}
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyEscape})
	if s.panelSection != settingsPanelSectionAccount || len(cfg.Sources[0].Compose.Identities) != 0 {
		t.Fatal("manager escape must discard")
	}
}

func TestSettingsIdentitiesValidationAndLiteralInput(t *testing.T) {
	s := openIdentitySettings(t, identitySettingsFixture())
	selectIdentityAction(s, "add")
	for _, bad := range []string{"broken", "me@example.test", "ME@example.test", "Name <alias@example.test>", "a@example.test\nBcc:x@example.test"} {
		if s.validateIdentityEdit(bad) == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	s = updateSettingsForTest(t, s, keyRunes("M d q S 日本語"))
	if s.identity.name != "M d q S 日本語" {
		t.Fatalf("literal input lost: %q", s.identity.name)
	}
	s.identity.name = "Alias"
	s.identity.address = "alias@example.test"
	s.completeIdentityForm()
	selectIdentityAction(s, "add")
	if s.validateIdentityEdit("ALIAS@example.test") == nil {
		t.Fatal("duplicate accepted")
	}
	s.leaveIdentitySection()
	selectIdentityAction(s, "primary")
	view := stripANSI(renderSettingsViewForTest(t, s, 80, 24))
	if strings.Contains(view, "Remove identity") || strings.Contains(view, "Email address") {
		t.Fatal("primary address must not be editable/removable")
	}
}

func TestSettingsIdentitiesSavePreservesConfigAndUpdatesCompose(t *testing.T) {
	cfg := identitySettingsFixture()
	s := openIdentitySettings(t, cfg)
	selectIdentityAction(s, "add")
	s.identity.name = "Alias"
	s.identity.address = "alias@example.test"
	s.identity.makeDefault = true
	s.completeIdentityForm()
	msg := selectIdentityAction(s, "save")().(SettingsIdentitiesSavedMsg)
	// Refreshing a token while the manager is open must not be reverted.
	cfg.Sources[0].Google.AccessToken = "fresh-token"
	expected := *cfg
	expected.Sources = append([]config.SourceConfig(nil), cfg.Sources...)
	expected.Sources[0].Compose.Identities = msg.Compose.Identities
	expected.Sources[0].Compose.DefaultIdentity = msg.Compose.DefaultIdentity
	next, err := configWithIdentities(cfg, msg)
	if err != nil || !reflect.DeepEqual(next, &expected) {
		t.Fatalf("unrelated config changed: %v", err)
	}
	m := New(newAccountAwareStubBackend([]backend.AccountInfo{{SourceID: "mail-work", Address: "me@example.test"}, {SourceID: "mail-other", Address: "other@example.test"}}), nil, "me@example.test", nil, false)
	m.cfg = cfg
	m.settingsPanel = s
	m.configPath = filepath.Join(t.TempDir(), "config.yaml")
	m.saveSettingsIdentities(msg)
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	var disk config.Config
	if err := yaml.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(disk.Sources, next.Sources) {
		t.Fatal("disk save mismatch")
	}
	info, _ := os.Stat(m.configPath)
	if info.Mode().Perm() != 0600 {
		t.Fatal("config permissions")
	}
	if cfg.Sources[0].Compose.DefaultIdentity != "" {
		t.Fatal("original config mutated")
	}
	reopened := openIdentitySettings(t, &disk)
	if reopened.identity.compose.DefaultIdentity != "alias@example.test" {
		t.Fatal("default not persisted")
	}
	// A multi-account backend supplies the selected source to Compose.
	m.composeSourceID = "mail-work"
	if !strings.Contains(m.composeFromAddress(), "alias@example.test") {
		t.Fatalf("Compose did not pick saved default: %s", m.composeFromAddress())
	}
}

func TestSettingsIdentitiesWriteFailureRetainsStagedEdits(t *testing.T) {
	cfg := identitySettingsFixture()
	s := openIdentitySettings(t, cfg)
	selectIdentityAction(s, "add")
	s.identity.address = "alias@example.test"
	s.completeIdentityForm()
	msg := selectIdentityAction(s, "save")().(SettingsIdentitiesSavedMsg)
	m := New(&stubBackend{}, nil, "me@example.test", nil, false)
	m.cfg = cfg
	m.settingsPanel = s
	m.configPath = filepath.Join(t.TempDir(), "missing", "config.yaml")
	m.saveSettingsIdentities(msg)
	if m.cfg != cfg || s.done || len(s.identity.compose.Identities) != 1 || !strings.Contains(s.identity.status, "Could not save") {
		t.Fatal("failed save changed config or discarded edits")
	}
	m.configPath = filepath.Join(t.TempDir(), "config.yaml")
	m.saveSettingsIdentities(msg)
	if !strings.Contains(s.identity.status, "saved") || len(m.cfg.Sources[0].Compose.Identities) != 1 {
		t.Fatal("retry failed")
	}
}

func TestSettingsIdentitiesLegacyAndCalendarOnly(t *testing.T) {
	cfg := &config.Config{Credentials: config.CredentialsConfig{Username: "me@example.test"}, Compose: config.ComposeConfig{Signature: config.SignatureConfig{Text: "keep"}}}
	msg := SettingsIdentitiesSavedMsg{Legacy: true, Compose: config.ComposeConfig{Identities: []config.IdentityConfig{{Address: "alias@example.test"}}}}
	next, err := configWithIdentities(cfg, msg)
	if err != nil || len(next.Sources) != 0 || next.Compose.Signature.Text != "keep" || len(next.Compose.Identities) != 1 {
		t.Fatalf("legacy preservation: %v", err)
	}
	calendar := identitySettingsFixture()
	calendar.Sources = calendar.Sources[1:2]
	s := NewSettings(SettingsModePanel, calendar)
	s.panelSection = settingsPanelSectionAccounts
	s.accountsMenuChoice = "account:work"
	s.openSelectedAccountFromList()
	view := stripANSI(renderSettingsViewForTest(t, s, 100, 32))
	if strings.Contains(view, "Edit sending identities") || s.openIdentityManager() {
		t.Fatal("calendar offered mail identities")
	}
}

func TestSettingsIdentitiesLongListAndResize(t *testing.T) {
	cfg := identitySettingsFixture()
	for i := 0; i < 40; i++ {
		cfg.Sources[0].Compose.Identities = append(cfg.Sources[0].Compose.Identities, config.IdentityConfig{Address: fmt.Sprintf("alias%d@example.test", i)})
	}
	s := openIdentitySettings(t, cfg)
	for _, size := range [][2]int{{220, 50}, {80, 24}, {50, 15}, {80, 24}, {220, 50}} {
		rendered := stripANSI(renderSettingsViewForTest(t, s, size[0], size[1]))
		if strings.Count(rendered, "\n")+1 > size[1] {
			t.Fatalf("overflow at %v", size)
		}
	}
	for i := 0; i < 42; i++ {
		s, _ = updateAndPumpSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if s.identity.choice != "save" {
		t.Fatalf("save unreachable: %q", s.identity.choice)
	}
}

func TestSettingsIdentitiesNewAliasKeepsPrimaryDefault(t *testing.T) {
	s := openIdentitySettings(t, identitySettingsFixture())
	selectIdentityAction(s, "add")
	if s.identity.makeDefault {
		t.Fatal("a new alias should not replace the primary default unless selected")
	}
	s.identity.address = "new@example.test"
	s.completeIdentityForm()
	if s.identity.compose.DefaultIdentity != "" {
		t.Fatal("adding alias replaced default")
	}
}
