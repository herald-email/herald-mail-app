package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func renderSettingsRawViewForTest(t *testing.T, s *Settings, width, height int) string {
	t.Helper()
	updated, _ := s.Update(tea.WindowSizeMsg{Width: width, Height: height})
	s = updated.(*Settings)
	rendered := s.View().Content
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
	for _, want := range []string{"Recommended", "Supported", "Experimental"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected setup messaging to include %q, got:\n%s", want, rendered)
		}
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

func TestSettingsWizard_DefaultHidesGmailOAuthAndShowsIMAPPresets(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	var labels []string
	for _, option := range s.accountTypeOptions() {
		labels = append(labels, option.Key)
	}
	rendered := strings.Join(labels, "\n")

	for _, want := range []string{
		"Gmail (IMAP + App Password)",
		"ProtonMail Bridge",
		"Fastmail",
		"iCloud",
		"Outlook",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected default wizard account choices to include %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Gmail OAuth") {
		t.Fatalf("expected default wizard account choices to hide Gmail OAuth, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "(Experimental)") {
		t.Fatalf("expected IMAP-based wizard account choices to avoid experimental labels, got:\n%s", rendered)
	}
}

func TestSettingsWizard_ExperimentalShowsGmailOAuthMarked(t *testing.T) {
	s := NewSettingsWithOptions(SettingsModeWizard, nil, SettingsOptions{ShowExperimentalEmailServices: true})

	var labels []string
	for _, option := range s.accountTypeOptions() {
		labels = append(labels, option.Key)
	}
	rendered := strings.Join(labels, "\n")

	if !strings.Contains(rendered, "Gmail OAuth (Experimental)") {
		t.Fatalf("expected experimental wizard account choices to include marked Gmail OAuth, got:\n%s", rendered)
	}
}

func TestSettingsPanel_StillShowsGmailOAuth(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)

	var labels []string
	for _, option := range s.accountTypeOptions() {
		labels = append(labels, option.Key)
	}
	rendered := strings.Join(labels, "\n")

	if !strings.Contains(rendered, "Gmail OAuth") {
		t.Fatalf("expected in-app settings panel to keep Gmail OAuth visible, got:\n%s", rendered)
	}
}

func TestSettingsWizard_GmailSummaryUsesShortClickableLinks(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	updated, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s = updated.(*Settings)

	updated, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s = updated.(*Settings)
	rendered := s.View().Content
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
	if strings.Contains(plain, "https://myaccount.google.com/apppasswords") {
		t.Fatalf("expected raw Gmail docs URL to be hidden behind a short clickable label, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;https://myaccount.google.com/apppasswords") {
		t.Fatalf("expected OSC 8 hyperlink for Gmail docs, got raw view:\n%q", rendered)
	}
}

func TestSettingsWizard_AIProviderChoicesIncludeDefaultCustomAndDisabled(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.form.NextGroup()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{
		"Ollama (local default)",
		"Ollama (local custom)",
		"Claude API",
		"OpenAI / compatible",
		"AI features disabled",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected AI provider choices to include %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizard_OllamaDefaultDoesNotAskForCustomValues(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.aiProvider = "ollama-default"
	s.buildForm()
	s.form.NextGroup()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if strings.Contains(rendered, "Ollama Host>") || strings.Contains(rendered, "Ollama Model>") {
		t.Fatalf("expected default Ollama path to hide custom fields, got:\n%s", rendered)
	}
}
