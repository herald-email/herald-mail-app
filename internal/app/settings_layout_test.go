package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func renderSettingsRawViewForTest(t *testing.T, s *Settings, width, height int) string {
	t.Helper()
	updated, _ := s.Update(tea.WindowSizeMsg{Width: width, Height: height})
	s = updated.(*Settings)
	rendered := s.View()
	assertFitsWidth(t, width, rendered)
	assertFitsHeight(t, height, rendered)
	return rendered
}

func renderSettingsViewForTest(t *testing.T, s *Settings, width, height int) string {
	t.Helper()
	return ansi.Strip(renderSettingsRawViewForTest(t, s, width, height))
}

func TestSettingsWizardView_RendersHeraldChrome(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if !strings.Contains(rendered, "Herald Setup") {
		t.Fatalf("expected Herald setup title, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Supported") || !strings.Contains(rendered, "Experimental") {
		t.Fatalf("expected supported vs experimental messaging, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "╭") || !strings.Contains(rendered, "╯") {
		t.Fatalf("expected bordered wizard shell, got:\n%s", rendered)
	}
}

func TestSettingsWizardView_MinimumSizeGuardAt50x15(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	rendered := renderSettingsViewForTest(t, s, 50, 15)

	if !strings.Contains(rendered, "Terminal too narrow") {
		t.Fatalf("expected minimum-size guard, got:\n%s", rendered)
	}
}

func TestSettingsWizard_GmailIMAPStepIncludesGuidance(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.provider = "gmail"
	s.buildForm()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{
		"App Password",
		"imap.gmail.com",
		"smtp.gmail.com",
		"[click] App passwords",
		"Workspace",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected Gmail guidance to include %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizard_SelectingGmailKeepsAccountTypeVisible(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	updated, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s = updated.(*Settings)

	updated, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s = updated.(*Settings)
	rendered := ansi.Strip(s.View())

	for _, want := range []string{
		"Account Type",
		"Gmail (IMAP + App Password)",
		"Gmail OAuth (Experimental)",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected provider switch to keep account step visible and include %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizard_GmailSummaryUsesShortClickableLinks(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	updated, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s = updated.(*Settings)

	updated, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s = updated.(*Settings)
	rendered := s.View()
	plain := ansi.Strip(rendered)

	for _, want := range []string{
		"[click] App passwords",
		"[click] Add Gmail to another client",
		"[click] Workspace IMAP setup",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected short clickable Gmail docs label %q, got:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "https://support.google.com/mail/answer/185833?hl=en") {
		t.Fatalf("expected raw Gmail docs URL to be hidden behind a short clickable label, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;https://support.google.com/mail/answer/185833?hl=en") {
		t.Fatalf("expected OSC 8 hyperlink for Gmail docs, got raw view:\n%q", rendered)
	}
}
