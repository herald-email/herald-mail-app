package app

import (
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsentSaveRebuildsAIClient(t *testing.T) {
	previous := ollamaAppConfig("http://localhost:11434", "chat", "embed")
	client, err := ai.NewFromConfig(previous)
	if err != nil {
		t.Fatal(err)
	}
	m := New(&stubBackend{}, nil, "", client, false)
	m.SetConfig(previous)
	next := *previous
	next.GrantSharing(true, true, false)
	m.applySettingsSaved(SettingsSavedMsg{Config: &next})
	if m.classifier == client {
		t.Fatal("consent save retained client bound to denied config")
	}
}

func TestConsentPanelKeepsWrappedLinesInsideField(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s.width, s.height = 80, 24
	line := strings.Repeat("x", s.formWidth())
	if !strings.Contains(s.renderPanelWithFormView(line), line) {
		t.Fatal("outer box wraps an already fitted form line")
	}
}

func TestMailSettingsOffersPasswordCommand(t *testing.T) {
	s := NewSettingsWithOptions(SettingsModeWizard, nil, SettingsOptions{FirstRunAccountOnly: true})
	s.provider = "imap"
	s.buildForm()
	renderSettingsViewContainingForTest(t, s, 100, 40, "Password command")
}

func TestConsentSaveActivatesChatAfterPersistingGrant(t *testing.T) {
	previous := ollamaAppConfig("http://localhost:11434", "chat", "embed")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := previous.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := New(&stubBackend{}, nil, "", nil, false)
	m.SetConfig(loaded)
	m.configPath = path
	next := *loaded
	next.GrantSharing(true, false, false)
	m.applySettingsSaved(SettingsSavedMsg{Config: &next})
	if m.chatAgent == nil {
		t.Fatal("saved consent did not activate chat")
	}
	next.GrantSharing(false, false, false)
	m.applySettingsSaved(SettingsSavedMsg{Config: &next})
	if m.chatAgent != nil {
		t.Fatal("revocation left chat enabled")
	}
}

func TestDemoAISettingsDoNotRequestRestart(t *testing.T) {
	previous := ollamaAppConfig("http://localhost:11434", "chat", "old-embed")
	m := New(backend.NewDemoBackend(), nil, "", nil, false)
	m.SetConfig(previous)
	next := *previous
	next.Ollama.EmbeddingModel = "new-embed"
	next.Semantic.Model = "new-embed"
	m.applySettingsSaved(SettingsSavedMsg{Config: &next})
	if strings.Contains(m.statusMessage, "Restart") {
		t.Fatal(m.statusMessage)
	}
}
