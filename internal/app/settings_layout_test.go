package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func renderSettingsViewForTest(t *testing.T, s *Settings, width, height int) string {
	t.Helper()
	updated, _ := s.Update(tea.WindowSizeMsg{Width: width, Height: height})
	s = updated.(*Settings)
	rendered := s.View()
	assertFitsWidth(t, width, rendered)
	assertFitsHeight(t, height, rendered)
	return stripANSI(rendered)
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
		"support.google.com",
		"Workspace",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected Gmail guidance to include %q, got:\n%s", want, rendered)
		}
	}
}
