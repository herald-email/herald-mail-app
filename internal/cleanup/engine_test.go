package cleanup

import (
	"context"
	"testing"
	"time"

	"mail-processor/internal/cache"
	"mail-processor/internal/models"
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
