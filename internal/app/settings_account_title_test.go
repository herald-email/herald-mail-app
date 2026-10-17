package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
)

func accountTitleSettingsForTest(t *testing.T) *Settings {
	t.Helper()
	s := NewSettings(SettingsModePanel, &config.Config{Sources: []config.SourceConfig{{
		ID: "work-mail", AccountID: "work", Kind: "mail", Provider: "imap", DisplayName: "Work",
		Credentials: config.CredentialsConfig{Username: "work@example.test", Password: "secret"},
		IMAP:        config.ServerConfig{Host: "imap.example.test", Port: 993},
		SMTP:        config.ServerConfig{Host: "smtp.example.test", Port: 587},
	}}})
	s.panelSection = settingsPanelSectionAccounts
	s.accountsMenuChoice = "account:work"
	if !s.openSelectedAccountFromList() {
		t.Fatal("open account")
	}
	s.setSize(220, 50)
	return s
}

func assertAccountTitleForTest(t *testing.T, s *Settings, want string) {
	t.Helper()
	view := stripANSI(s.form.View())
	matches := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == want {
			matches++
		}
	}
	// The edit group itself is also called Account; count the note separately.
	if matches > 0 && (want != "Account" || matches == 2) {
		return
	}
	t.Fatalf("account title %q missing from form:\n%s", want, view)
}

func TestSettingsAccountTitleUpdatesImmediately(t *testing.T) {
	s := accountTitleSettingsForTest(t)
	assertAccountTitleForTest(t, s, "Work")

	// Do not execute any returned commands: the title must reflect the same
	// input update, without waiting for a background title evaluation.
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	assertAccountTitleForTest(t, s, "Account")
	for _, r := range "  日本語 Mail  " {
		s = updateSettingsForTest(t, s, keyRunes(string(r)))
		want := strings.TrimSpace(s.accountDisplayName)
		if want == "" {
			want = "Account"
		}
		assertAccountTitleForTest(t, s, want)
	}

	// Updating a note must not rebuild the form or steal the input's focus.
	_, pending := s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if pending == nil {
		t.Fatal("Tab did not schedule navigation")
	}
	// Deliver navigation without recursively running cursor blink timers.
	s = updateSettingsForTest(t, s, huh.NextField())
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	s = updateSettingsForTest(t, s, keyRunes("renamed@example.test"))
	if s.email != "renamed@example.test" || s.accountDisplayName != "  日本語 Mail  " {
		t.Fatalf("Tab lost focus or edited the label: email=%q label=%q", s.email, s.accountDisplayName)
	}
	_, pending = s.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if pending == nil {
		t.Fatal("Shift+Tab did not schedule navigation")
	}
	s = updateSettingsForTest(t, s, huh.PrevField())
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyBackspace})
	assertAccountTitleForTest(t, s, "日本語 Mail")
	if s.accountDisplayName != "  日本語 Mail " {
		t.Fatalf("Shift+Tab did not restore label focus: %q", s.accountDisplayName)
	}
	if got := s.buildConfig().Sources[0].DisplayName; got != "日本語 Mail" {
		t.Fatalf("saved label = %q", got)
	}
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyEscape})
	s.accountsMenuChoice = "account:work"
	if !s.openSelectedAccountFromList() {
		t.Fatal("reopen account")
	}
	s = updateSettingsForTest(t, s, nil)
	assertAccountTitleForTest(t, s, "Work")
}

func TestSettingsAccountTitlePendingCommands(t *testing.T) {
	s := accountTitleSettingsForTest(t)
	s = updateSettingsForTest(t, s, keyRunes(" renamed"))
	_, pending := s.Update(nil)

	// Bubble Tea runs commands off the UI loop. Hold their results while the
	// loop continues updating Huh inputs, which write through their accessors
	// even for messages that don't change the text. Run with -race.
	started := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		close(started)
		var run func(tea.Cmd)
		run = func(cmd tea.Cmd) {
			if cmd == nil {
				return
			}
			if batch, ok := cmd().(tea.BatchMsg); ok {
				for _, child := range batch {
					run(child)
				}
			}
		}
		run(pending)
	}()
	<-started
	for range 32 {
		s = updateSettingsForTest(t, s, nil)
	}
	<-finished
	assertAccountTitleForTest(t, s, "Work renamed")
}

func TestSettingsAccountTitleDisconnectFallbacks(t *testing.T) {
	for _, tc := range []struct{ name, id, want string }{
		{"  Work  ", "work", "Disconnect Work?"},
		{" \t ", "work", "Disconnect work?"},
		{"", "", "Disconnect selected account?"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			s := accountTitleSettingsForTest(t)
			s.accountDisplayName = tc.name
			s.selectedAccountID = tc.id
			s.accountDeletePending = true
			s.buildForm()
			s = updateSettingsForTest(t, s, nil)
			assertAccountTitleForTest(t, s, tc.want)
		})
	}
}
