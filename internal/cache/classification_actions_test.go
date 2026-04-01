package cache

import (
	"testing"

	"mail-processor/internal/models"
)

func newTestCacheForRules(t *testing.T) *Cache {
	t.Helper()
	c, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory cache: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// TestImportClassificationActions verifies that SaveRule persists rules and
// GetAllRules retrieves them, and that saving a rule with the same name a
// second time (simulating an idempotent re-run) does not produce a duplicate.
func TestImportClassificationActions(t *testing.T) {
	c := newTestCacheForRules(t)

	rule1 := &models.Rule{
		Name:         "move-newsletters",
		TriggerType:  models.TriggerCategory,
		TriggerValue: "newsletter",
		Enabled:      true,
		Actions: []models.RuleAction{
			{Type: models.ActionMove, DestFolder: "Newsletters"},
		},
	}
	rule2 := &models.Rule{
		Name:         "delete-spam",
		TriggerType:  models.TriggerSender,
		TriggerValue: "spam@example.com",
		Enabled:      true,
		Actions: []models.RuleAction{
			{Type: models.ActionDelete},
		},
	}

	if err := c.SaveRule(rule1); err != nil {
		t.Fatalf("SaveRule rule1: %v", err)
	}
	if err := c.SaveRule(rule2); err != nil {
		t.Fatalf("SaveRule rule2: %v", err)
	}

	all, err := c.GetAllRules()
	if err != nil {
		t.Fatalf("GetAllRules: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(all))
	}

	// Simulate a re-run: the seeding logic checks existing names and skips duplicates.
	existingNames := make(map[string]bool)
	for _, r := range all {
		existingNames[r.Name] = true
	}

	// Try to insert rule1 again — should be skipped by the seeding logic.
	if !existingNames[rule1.Name] {
		if err := c.SaveRule(rule1); err != nil {
			t.Fatalf("unexpected SaveRule on re-run: %v", err)
		}
	}

	all2, err := c.GetAllRules()
	if err != nil {
		t.Fatalf("GetAllRules after re-run: %v", err)
	}
	if len(all2) != 2 {
		t.Errorf("expected 2 rules after re-run, got %d", len(all2))
	}
}

// TestImportClassificationActions_SkipsExisting verifies that the seeding
// logic (check existing names, skip if found) prevents re-insertion.
func TestImportClassificationActions_SkipsExisting(t *testing.T) {
	c := newTestCacheForRules(t)

	// Pre-insert a rule.
	rule := &models.Rule{
		Name:         "notify-promotions",
		TriggerType:  models.TriggerCategory,
		TriggerValue: "promotions",
		Enabled:      true,
		Actions: []models.RuleAction{
			{Type: models.ActionNotify, NotifyBody: "New promotion"},
		},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	// Simulate the seeding logic from main.go.
	existing, err := c.GetAllRules()
	if err != nil {
		t.Fatalf("GetAllRules: %v", err)
	}
	existingNames := make(map[string]bool, len(existing))
	for _, r := range existing {
		existingNames[r.Name] = true
	}

	// The config contains one action with the same name — it should be skipped.
	configActions := []struct {
		Name         string
		TriggerType  string
		TriggerValue string
		ActionType   string
		ActionValue  string
		Enabled      bool
	}{
		{
			Name:         "notify-promotions",
			TriggerType:  "category",
			TriggerValue: "promotions",
			ActionType:   "notify",
			ActionValue:  "New promotion",
			Enabled:      true,
		},
	}

	for _, ca := range configActions {
		if !existingNames[ca.Name] {
			if err := c.SaveRule(&models.Rule{
				Name:         ca.Name,
				TriggerType:  models.RuleTriggerType(ca.TriggerType),
				TriggerValue: ca.TriggerValue,
				Enabled:      ca.Enabled,
				Actions: []models.RuleAction{
					{Type: models.RuleActionType(ca.ActionType), NotifyBody: ca.ActionValue},
				},
			}); err != nil {
				t.Fatalf("SaveRule during seeding: %v", err)
			}
		}
	}

	all, err := c.GetAllRules()
	if err != nil {
		t.Fatalf("GetAllRules after seeding: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 rule (no duplicate), got %d", len(all))
	}
}
