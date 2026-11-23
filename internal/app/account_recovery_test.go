package app

import (
	"context"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/accountcheck"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"golang.org/x/oauth2"
)

func TestAccountRecoveryTargetsSelectedAccountAndPreservesLiveTokens(t *testing.T) {
	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "home", AccountID: "home", Kind: "mail", Provider: "gmail", Google: config.GoogleConfig{Email: "home@example.test", RefreshToken: "home-token"}},
		{ID: "work", AccountID: "work", Kind: "mail", Provider: "gmail", Google: config.GoogleConfig{Email: "work@example.test", RefreshToken: "work-token"}},
		{ID: "work-calendar", AccountID: "work", Kind: "calendar", Provider: "google_calendar", Google: config.GoogleConfig{Email: "work@example.test", RefreshToken: "calendar-token"}},
	}}
	for _, profile := range []string{"default", "vim", "emacs"} {
		t.Run(profile, func(t *testing.T) {
			b := newAccountAwareStubBackend([]backend.AccountInfo{{SourceID: "home", AccountID: "home"}, {SourceID: "work", AccountID: "work", Address: "work@example.test"}})
			m := accountSwitcherTestModel(b)
			m.cfg = cfg
			m.keyboard = NewKeyboardResolver(nil)
			m.keyboard.bindings = builtInKeyboardBindings(profile)
			m.showAccountSwitcher, m.accountSwitcherCursor = true, 2
			_, cmd, handled := m.handleAccountSwitcherKey(shortcutKeyPressMsg("r"))
			if !handled || cmd == nil {
				t.Fatal("recovery shortcut not handled")
			}
			msg, ok := cmd().(OAuthRequiredMsg)
			if !ok || len(msg.SourceIDs) != 2 || msg.SourceIDs[0] != "work" || msg.SourceIDs[1] != "work-calendar" {
				t.Fatalf("wrong recovery target: %#v", msg.SourceIDs)
			}
			if !msg.ValidateAccount || msg.ValidationSourceID != "work" {
				t.Fatal("recovery must validate and reload selected account")
			}
			if err := applyGoogleOAuthToken(msg.Config, "work@example.test", &oauth2.Token{AccessToken: "new", RefreshToken: "new"}, msg.SourceIDs); err != nil {
				t.Fatal(err)
			}
			if cfg.Sources[1].Google.RefreshToken != "work-token" || cfg.Sources[2].Google.RefreshToken != "calendar-token" || msg.Config.Sources[0].Google.RefreshToken != "home-token" {
				t.Fatal("recovery mutated live or unrelated tokens")
			}
		})
	}
}

func TestAccountRecoveryValidationUsesSelectedSource(t *testing.T) {
	old := validateAccountConfig
	defer func() { validateAccountConfig = old }()
	cfg := &config.Config{Sources: []config.SourceConfig{{ID: "home", Kind: "mail"}, {ID: "work", Kind: "mail", Provider: "gmail", Google: config.GoogleConfig{Email: "work@example.test"}}}}
	validateAccountConfig = func(_ context.Context, candidate *config.Config, _ string) accountcheck.Result {
		if len(candidate.Sources) != 1 || candidate.Sources[0].ID != "work" {
			t.Fatal("validated unrelated account")
		}
		return accountcheck.Result{}
	}
	msg := validateAccountSettingsCmd(SettingsSavedMsg{Config: cfg, ValidationSourceID: "work"}, "")().(AccountValidationMsg)
	if msg.Config != cfg || len(msg.Config.Sources) != 2 {
		t.Fatal("validation discarded other accounts")
	}
}

func TestSingleAccountCredentialRecovery(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.SetConfig(&config.Config{Sources: []config.SourceConfig{{ID: "solo", AccountID: "solo", Kind: "mail", Provider: "imap", Credentials: config.CredentialsConfig{Username: "solo@example.test"}}}})
	m.recordAccountSyncStatus(models.FolderSyncEvent{SourceID: models.DefaultMailSourceID, Phase: models.SyncPhaseError, Error: "AUTHENTICATIONFAILED"})
	if !strings.Contains(m.accountSignInWarning(), "sign in") || !m.accountSwitcherShortcutAvailable() {
		t.Fatal("single local account has no recovery warning")
	}
	m.recoverAccount(m.accounts[0])
	if !m.showSettings || m.settingsPanel.selectedAccountID != "solo" {
		t.Fatal("did not open selected account settings")
	}
}

func TestAccountHealthIgnoresStaleFailures(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{{SourceID: "work"}})
	m := accountSwitcherTestModel(b)
	m.recordAccountSyncStatus(models.FolderSyncEvent{SourceID: "work", Generation: 2, Phase: models.SyncPhaseComplete})
	m.recordAccountSyncStatus(models.FolderSyncEvent{SourceID: "work", Generation: 1, Phase: models.SyncPhaseError, Error: "invalid_grant"})
	if accountNeedsSignIn(m.accountStatuses["work"]) {
		t.Fatal("stale failure restored warning")
	}
}

func TestAccountRecoveryReconnectsSelectedAccount(t *testing.T) {
	oldFactory := newValidatedLocalBackend
	defer func() { newValidatedLocalBackend = oldFactory }()
	accounts := []backend.AccountInfo{{SourceID: "home", AccountID: "home"}, {SourceID: "work", AccountID: "work"}}
	next := newAccountAwareStubBackend(accounts)
	newValidatedLocalBackend = func(*config.Config, string, ai.AIClient) (backend.Backend, error) { return next, nil }
	m := accountSwitcherTestModel(newAccountAwareStubBackend(accounts))
	m.accountStatusGenerations = map[models.SourceID]int64{"work": 40}
	cfg := &config.Config{Sources: []config.SourceConfig{{ID: "home", Kind: "mail", AccountID: "home"}, {ID: "work", Kind: "mail", AccountID: "work"}}}
	m.finishAccountValidation(AccountValidationMsg{Config: cfg, ValidationSourceID: "work"})
	if m.activeSourceID != "work" {
		t.Fatalf("reconnected %q, want work", m.activeSourceID)
	}
	if len(m.accountStatusGenerations) != 0 {
		t.Fatal("old backend generations retained")
	}
}

func TestInactiveAccountHealthHasIndependentGeneration(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{{SourceID: "home"}, {SourceID: "work"}})
	m := accountSwitcherTestModel(b)
	m.activeSourceID = "home"
	m.syncGeneration = 42
	m.recordAccountSyncStatus(models.FolderSyncEvent{SourceID: "work", Generation: 1, Phase: models.SyncPhaseError, Error: "invalid_grant"})
	if !accountNeedsSignIn(m.accountStatuses["work"]) {
		t.Fatal("unrelated active account generation hid auth failure")
	}
	if m.syncGeneration != 42 {
		t.Fatal("account health changed active mailbox generation")
	}
}

func TestAccountSignInErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{"no OAuth2 refresh token stored; please re-authenticate", true},
		{"oauth2: invalid_grant", true},
		{"IMAP AUTHENTICATIONFAILED", true},
		{"dial tcp: connection refused", false},
		{"failed to refresh OAuth2 token: context deadline exceeded", false},
		{"permission denied", false},
	} {
		if got := accountNeedsSignIn(backend.AccountStatus{Error: tc.message}); got != tc.want {
			t.Errorf("%q: got %v want %v", tc.message, got, tc.want)
		}
	}
}

func TestAccountAlarmVisibleInFavoriteRowsAndStatus(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{{SourceID: "work", DisplayName: "Work"}, {SourceID: "home", DisplayName: "Home"}})
	m := accountSwitcherTestModel(b)
	m.accountStatuses["work"] = backend.AccountStatus{Error: "invalid_grant"}
	item := sidebarItem{accountIdentity: true, kind: sidebarItemAccountFolder, account: "work", label: "Work", depth: 1, fullPath: "INBOX", status: &models.FolderStatus{Unseen: 1, Total: 2}}
	m.sidebarCursor = 99
	row := m.renderSidebarItemLine(0, item)
	if !strings.Contains(row, "🚨") {
		t.Fatal("favorite account row missing alarm")
	}
	alarmPrefix := strings.Split(m.theme.Severity.Destructive.Style().Bold(true).Render("PROBE"), "PROBE")[0]
	if alarmPrefix == "" || !strings.Contains(row, alarmPrefix) {
		t.Fatal("favorite row missing alarm colors")
	}
	if ansi.StringWidth(row) > sidebarContentWidth {
		t.Fatalf("alarm overflows sidebar: %d", ansi.StringWidth(row))
	}
	m.sidebarCursor = 0
	m.focusedPanel = panelSidebar
	selected := m.renderSidebarItemLine(0, item)
	if !strings.Contains(selected, "🚨") || !strings.Contains(stripANSI(selected), "›") {
		t.Fatal("selection hides alarm or focus marker")
	}
	status := m.renderStatusBar()
	if !strings.Contains(status, "🚨") || !strings.Contains(status, alarmPrefix) {
		t.Fatal("status bar missing high-contrast alarm")
	}
	folder := item
	folder.accountIdentity = false
	if strings.Contains(m.renderSidebarItemLine(0, folder), "🚨") {
		t.Fatal("alarm repeated on ordinary folder rows")
	}
	group := sidebarItem{kind: sidebarItemGroup, account: "work", label: "Work", syntheticKey: "account:work"}
	if row := stripANSI(m.renderSidebarItemLine(0, group)); !strings.Contains(row, "🚨") || (!strings.Contains(row, "▼") && !strings.Contains(row, "▶")) {
		t.Fatal("account heading lost alarm or expansion arrow")
	}
	m.accountStatuses["work"] = backend.AccountStatus{State: "live"}
	if strings.Contains(m.renderSidebarItemLine(0, item), "🚨") || strings.Contains(m.renderStatusBar(), "🚨") {
		t.Fatal("recovered account retains alarm")
	}
}

func TestMailConfigProjectionKeepsTokenReferenceScoped(t *testing.T) {
	for _, sync := range []struct {
		name string
		run  func(*config.Config)
	}{
		{"settings", syncLegacyMailFieldsForSettings}, {"oauth", syncLegacyGmailFromFirstMailSource},
	} {
		for _, ref := range []string{"", "selected-google-token"} {
			t.Run(sync.name+"/"+ref, func(t *testing.T) {
				cfg := &config.Config{Sources: []config.SourceConfig{{ID: "selected", Kind: "mail", Google: config.GoogleConfig{TokenRef: ref}}}}
				cfg.Gmail.TokenRef = "unrelated-google-token"
				sync.run(cfg)
				if cfg.Gmail.TokenRef != ref {
					t.Fatal("retained another account's token reference")
				}
			})
		}
	}
	old := validateAccountConfig
	defer func() { validateAccountConfig = old }()
	validateAccountConfig = func(_ context.Context, c *config.Config, _ string) accountcheck.Result {
		if c.IsGmailOAuth() {
			t.Fatal("password account validation inherited Google credentials")
		}
		return accountcheck.Result{}
	}
	cfg := &config.Config{Sources: []config.SourceConfig{{ID: "proton", Kind: "mail", Provider: "protonmail"}}}
	cfg.Gmail.TokenRef = "unrelated-google-token"
	validateAccountSettingsCmd(SettingsSavedMsg{Config: cfg, ValidationSourceID: "proton"}, "")()
}
