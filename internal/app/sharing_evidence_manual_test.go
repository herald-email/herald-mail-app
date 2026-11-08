//go:build sharing_evidence

package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"os"
	"testing"
)

// Manual harness opens the real settings flow with synthetic data and no account connections.
func TestSharingEvidence(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Provider = "openai"
	cfg.OpenAI.BaseURL = "https://api.openai.com/v1"
	cfg.OpenAI.Model = "gpt-5-mini"
	cfg.OpenAI.APIKey = "synthetic-not-a-key"
	s := NewSettings(SettingsModePanel, cfg)
	s.panelSection = settingsPanelSectionAI
	s.buildForm()

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	if _, err := tea.NewProgram(s, tea.WithInput(tty), tea.WithOutput(tty)).Run(); err != nil {
		t.Fatal(err)
	}
}
