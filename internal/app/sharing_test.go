package app

import (
	"context"
	"github.com/fugue-labs/gollem/core"
	"github.com/herald-email/herald-mail-app/internal/config"
	"testing"
)

func TestSharingSettingsRequiresAffirmativeChoices(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Provider = "ollama"
	cfg.Ollama.Host = "http://localhost:11434"
	cfg.GrantSharing(true, true, true)
	s := NewSettings(SettingsModePanel, cfg)
	s.panelSection = settingsPanelSectionAI
	s.buildForm()
	if s.allowAISharing || s.allowBackgroundSharing || s.allowMCPSharing {
		t.Fatal("consent preselected")
	}
	revoked := s.buildConfig()
	if revoked.CheckSharing("ai") == nil || revoked.CheckSharing("mcp") == nil {
		t.Fatal("No did not revoke")
	}
	s.allowAISharing = true
	s.allowMCPSharing = true
	granted := s.buildConfig()
	if granted.CheckSharing("ai") != nil || granted.CheckSharing("mcp") != nil {
		t.Fatal("affirmative consent not saved")
	}
	if granted.CheckSharing("background") == nil {
		t.Fatal("background implicitly granted")
	}
	s.allowBackgroundSharing = true
	if s.buildConfig().CheckSharing("background") != nil {
		t.Fatal("separate background consent not saved")
	}
	for _, size := range [][2]int{{220, 50}, {80, 24}, {50, 15}, {220, 50}} {
		renderSettingsRawViewForTest(t, s, size[0], size[1])
	}
}

func TestSharingAgentBlocksRequestsAfterRevocation(t *testing.T) {
	cfg := &config.Config{}
	cfg.GrantSharing(true, false, false)
	model := &sharingModel{Model: core.NewTestModel(), cfg: cfg, fingerprint: cfg.SharingFingerprint("ai")}
	cfg.GrantSharing(false, false, false)
	if _, err := model.Request(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("agent request bypassed consent")
	}
	if _, err := model.RequestStream(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("agent stream bypassed consent")
	}
	m := &Model{}
	m.SetConfig(&config.Config{})
	if m.chatAgent != nil {
		t.Fatal("unconsented agent enabled")
	}
}
