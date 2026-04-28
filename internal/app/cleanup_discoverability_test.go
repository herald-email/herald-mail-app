package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

type cleanupDiscoverabilityBackend struct {
	stubBackend
	rules        []*models.Rule
	prompts      []*models.CustomPrompt
	cleanupRules []*models.CleanupRule
}

func (b *cleanupDiscoverabilityBackend) GetAllRules() ([]*models.Rule, error) {
	return b.rules, nil
}

func (b *cleanupDiscoverabilityBackend) GetAllCustomPrompts() ([]*models.CustomPrompt, error) {
	return b.prompts, nil
}

func (b *cleanupDiscoverabilityBackend) GetAllCleanupRules() ([]*models.CleanupRule, error) {
	return b.cleanupRules, nil
}

func pressKey(t *testing.T, m *Model, key string) *Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(*Model)
}

func TestCleanupRuleOverlay_ExplainsPurposeAndShowsSavedRules(t *testing.T) {
	b := &cleanupDiscoverabilityBackend{
		rules: []*models.Rule{
			{
				ID:           7,
				Name:         "archive receipts",
				Enabled:      true,
				TriggerType:  models.TriggerSender,
				TriggerValue: "billing@example.com",
				Actions:      []models.RuleAction{{Type: models.ActionArchive}},
			},
		},
	}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	m = pressKey(t, m, "W")
	rendered := stripANSI(m.View())

	for _, want := range []string{
		"future matching mail",
		"Saved automation rules",
		"archive receipts",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected rule overlay to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestPromptOverlay_ExplainsPurposeAndShowsSavedPrompts(t *testing.T) {
	b := &cleanupDiscoverabilityBackend{
		prompts: []*models.CustomPrompt{
			{ID: 3, Name: "invoice-extractor", OutputVar: "invoice_id"},
		},
	}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	m = pressKey(t, m, "P")
	rendered := stripANSI(m.View())

	for _, want := range []string{
		"reusable AI instructions",
		"Saving does not run",
		"Example: sender triage",
		"Use W to attach",
		"classify_email_custom",
		"Results are stored per email",
		"Saved prompts",
		"invoice-extractor",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected prompt overlay to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestCleanupManager_ExplainsManualVsScheduledResults(t *testing.T) {
	lastRun := time.Date(2026, 4, 22, 9, 15, 0, 0, time.UTC)
	b := &cleanupDiscoverabilityBackend{
		cleanupRules: []*models.CleanupRule{
			{
				ID:            2,
				Name:          "Trim newsletters",
				MatchType:     "sender",
				MatchValue:    "newsletter@example.com",
				Action:        "delete",
				OlderThanDays: 30,
				Enabled:       true,
				LastRun:       &lastRun,
			},
		},
	}
	mgr := NewCleanupManager(b, 120, 40)
	if cmd := mgr.Init(); cmd != nil {
		msg := cmd()
		mgr, _ = mgr.Update(msg)
	}

	rendered := stripANSI(mgr.View())
	for _, want := range []string{
		"Runs on demand or on schedule",
		"Saved cleanup rules live here",
		"Trim newsletters",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected cleanup manager to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestRuleSavedStatus_ExplainsWhereToFindItAgain(t *testing.T) {
	m := New(&cleanupDiscoverabilityBackend{}, nil, "", nil, false)

	updated, _ := m.Update(RuleEditorDoneMsg{
		Rule: &models.Rule{
			Name:         "Archive billing mail",
			TriggerType:  models.TriggerSender,
			TriggerValue: "billing@example.com",
			Actions:      []models.RuleAction{{Type: models.ActionArchive}},
		},
	})
	m = updated.(*Model)

	if !strings.Contains(m.statusMessage, "Reopen W") {
		t.Fatalf("expected rule save status to explain where to find it again, got %q", m.statusMessage)
	}
}

func TestPromptSavedStatus_ExplainsWhereToFindItAgain(t *testing.T) {
	m := New(&cleanupDiscoverabilityBackend{}, nil, "", nil, false)

	updated, _ := m.Update(PromptEditorDoneMsg{
		Prompt: &models.CustomPrompt{Name: "invoice-extractor"},
	})
	m = updated.(*Model)

	if !strings.Contains(m.statusMessage, "Reopen P") {
		t.Fatalf("expected prompt save status to explain where to find it again, got %q", m.statusMessage)
	}
}
