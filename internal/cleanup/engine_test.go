package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// mockBackend is a minimal Backend stub for engine tests.
// It records DeleteEmail / MoveEmail calls.
type mockBackend struct {
	deleted []string // message IDs passed to DeleteEmail
	moved   []string // message IDs passed to MoveEmail
	// embed the no-op stub for all unneeded interface methods
	noopBackend
}

func (m *mockBackend) DeleteEmail(messageID, folder string) error {
	m.deleted = append(m.deleted, messageID)
	return nil
}

func (m *mockBackend) MoveEmail(messageID, fromFolder, toFolder string) error {
	m.moved = append(m.moved, messageID)
	return nil
}

func newTestCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func seedEmail(t *testing.T, c *cache.Cache, msgID, sender, folder string, date time.Time) {
	t.Helper()
	if err := c.CacheEmail(&models.EmailData{
		MessageID: msgID,
		Sender:    sender,
		Subject:   "test",
		Date:      date,
		Folder:    folder,
	}); err != nil {
		t.Fatalf("CacheEmail %s: %v", msgID, err)
	}
}

func TestRunRule(t *testing.T) {
	c := newTestCache(t)
	mb := &mockBackend{}
	engine := NewEngine(c, mb, nil)

	old := time.Now().AddDate(0, 0, -40)
	recent := time.Now().AddDate(0, 0, -5)

	seedEmail(t, c, "match-old", "spam@example.com", "INBOX", old)
	seedEmail(t, c, "match-recent", "spam@example.com", "INBOX", recent)
	seedEmail(t, c, "other-old", "safe@other.com", "INBOX", old)

	rule := &models.CleanupRule{
		ID:            1,
		MatchType:     "sender",
		MatchValue:    "spam@example.com",
		Action:        "delete",
		OlderThanDays: 30,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	// Save the rule so UpdateCleanupRuleLastRun has a valid ID
	if err := c.SaveCleanupRule(rule); err != nil {
		t.Fatalf("SaveCleanupRule: %v", err)
	}

	count, err := engine.RunRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("RunRule: %v", err)
	}
	if count != 1 {
		t.Errorf("count: got %d, want 1", count)
	}
	if len(mb.deleted) != 1 || mb.deleted[0] != "match-old" {
		t.Errorf("deleted: %v, want [match-old]", mb.deleted)
	}
}

func TestDryRunCleanupEngine(t *testing.T) {
	c := newTestCache(t)
	mb := &mockBackend{}
	engine := NewEngineWithDryRun(c, mb, nil, true)

	old := time.Now().AddDate(0, 0, -40)
	seedEmail(t, c, "dry-old", "spam@example.com", "INBOX", old)

	rule := &models.CleanupRule{
		Name:          "dry-run-rule",
		MatchType:     "sender",
		MatchValue:    "spam@example.com",
		Action:        "delete",
		OlderThanDays: 30,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	if err := c.SaveCleanupRule(rule); err != nil {
		t.Fatalf("SaveCleanupRule: %v", err)
	}

	count, err := engine.RunRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("RunRule: %v", err)
	}
	// Should still count "would process" emails
	if count != 1 {
		t.Errorf("dry-run count: got %d, want 1", count)
	}
	// Backend delete must NOT have been called
	if len(mb.deleted) != 0 {
		t.Errorf("dry-run: DeleteEmail should not be called, got %v", mb.deleted)
	}
	if len(mb.moved) != 0 {
		t.Errorf("dry-run: MoveEmail should not be called, got %v", mb.moved)
	}
	got, err := c.GetCleanupRule(rule.ID)
	if err != nil {
		t.Fatalf("GetCleanupRule: %v", err)
	}
	if got.LastRun != nil {
		t.Fatalf("dry-run should not update last_run, got %v", got.LastRun)
	}
}

func TestPlanDryRunBuildsStructuredCleanupRows(t *testing.T) {
	c := newTestCache(t)
	old := time.Now().AddDate(0, 0, -45)
	recent := time.Now().AddDate(0, 0, -2)
	seedEmail(t, c, "match-old", "Packet Press <newsletter@packetpress.example>", "INBOX", old)
	seedEmail(t, c, "match-recent", "Packet Press <newsletter@packetpress.example>", "INBOX", recent)
	seedEmail(t, c, "other-old", "Other <other@example.com>", "INBOX", old)

	rule := &models.CleanupRule{
		ID:            4,
		Name:          "Archive old Packet Press",
		MatchType:     "sender",
		MatchValue:    "newsletter@packetpress.example",
		Action:        "archive",
		OlderThanDays: 30,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}

	report, err := PlanDryRun(c, models.RuleDryRunRequest{
		Kind:   models.RuleDryRunKindCleanup,
		RuleID: rule.ID,
		Folder: "INBOX",
	}, []*models.CleanupRule{rule})
	if err != nil {
		t.Fatalf("PlanDryRun: %v", err)
	}

	if report.RuleCount != 1 {
		t.Fatalf("RuleCount = %d, want 1", report.RuleCount)
	}
	if report.MatchCount != 1 {
		t.Fatalf("MatchCount = %d, want 1", report.MatchCount)
	}
	if report.ActionCount != 1 {
		t.Fatalf("ActionCount = %d, want 1", report.ActionCount)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.MessageID != "match-old" {
		t.Fatalf("MessageID = %q, want match-old", row.MessageID)
	}
	if row.Action != "archive" {
		t.Fatalf("Action = %q, want archive", row.Action)
	}
	if row.Domain != "packetpress.example" {
		t.Fatalf("Domain = %q, want packetpress.example", row.Domain)
	}
	if row.Folder != "INBOX" {
		t.Fatalf("Folder = %q, want INBOX", row.Folder)
	}
}

func TestRunAll_SkipsDisabled(t *testing.T) {
	c := newTestCache(t)
	mb := &mockBackend{}
	engine := NewEngine(c, mb, nil)

	old := time.Now().AddDate(0, 0, -40)
	seedEmail(t, c, "msg1", "spam@example.com", "INBOX", old)

	enabled := &models.CleanupRule{
		Name:          "Enabled",
		MatchType:     "sender",
		MatchValue:    "spam@example.com",
		Action:        "delete",
		OlderThanDays: 30,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	disabled := &models.CleanupRule{
		Name:          "Disabled",
		MatchType:     "sender",
		MatchValue:    "spam@example.com",
		Action:        "delete",
		OlderThanDays: 30,
		Enabled:       false,
		CreatedAt:     time.Now(),
	}
	if err := c.SaveCleanupRule(enabled); err != nil {
		t.Fatalf("SaveCleanupRule enabled: %v", err)
	}
	if err := c.SaveCleanupRule(disabled); err != nil {
		t.Fatalf("SaveCleanupRule disabled: %v", err)
	}

	results, err := engine.RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}

	// Enabled rule should process 1 email
	if results[enabled.ID] != 1 {
		t.Errorf("enabled rule count: got %d, want 1", results[enabled.ID])
	}
	// Disabled rule should not appear in results
	if _, ok := results[disabled.ID]; ok {
		t.Error("disabled rule should not appear in results")
	}
}

func TestRunAll_SkipsOverlappingRowsAfterFirstAction(t *testing.T) {
	c := newTestCache(t)
	mb := &mockBackend{}
	engine := NewEngine(c, mb, nil)

	old := time.Now().AddDate(0, 0, -40)
	seedEmail(t, c, "overlap", "Spam Team <spam@example.com>", "INBOX", old)

	first := &models.CleanupRule{
		Name:          "Delete sender",
		MatchType:     "sender",
		MatchValue:    "spam@example.com",
		Action:        "delete",
		OlderThanDays: 30,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	second := &models.CleanupRule{
		Name:          "Archive domain",
		MatchType:     "domain",
		MatchValue:    "example.com",
		Action:        "archive",
		OlderThanDays: 30,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	if err := c.SaveCleanupRule(first); err != nil {
		t.Fatalf("SaveCleanupRule first: %v", err)
	}
	if err := c.SaveCleanupRule(second); err != nil {
		t.Fatalf("SaveCleanupRule second: %v", err)
	}

	results, err := engine.RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}

	if results[first.ID] != 1 {
		t.Fatalf("first rule count = %d, want 1", results[first.ID])
	}
	if results[second.ID] != 0 {
		t.Fatalf("second overlapping rule count = %d, want 0 skipped", results[second.ID])
	}
	if len(mb.deleted) != 1 || mb.deleted[0] != "overlap" {
		t.Fatalf("deleted = %v, want [overlap]", mb.deleted)
	}
	if len(mb.moved) != 0 {
		t.Fatalf("overlapping skipped row should not move mail, moved = %v", mb.moved)
	}
}
