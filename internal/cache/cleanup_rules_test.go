package cache

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func newTestCacheForCleanup(t *testing.T) *Cache {
	t.Helper()
	c, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory cache: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestSaveCleanupRuleInsert(t *testing.T) {
	c := newTestCacheForCleanup(t)

	rule := &models.CleanupRule{
		Name:          "Test Rule",
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
	if rule.ID == 0 {
		t.Fatal("expected rule.ID to be set after insert")
	}

	got, err := c.GetCleanupRule(rule.ID)
	if err != nil {
		t.Fatalf("GetCleanupRule: %v", err)
	}
	if got.Name != rule.Name {
		t.Errorf("Name: got %q, want %q", got.Name, rule.Name)
	}
	if got.MatchType != rule.MatchType {
		t.Errorf("MatchType: got %q, want %q", got.MatchType, rule.MatchType)
	}
	if got.MatchValue != rule.MatchValue {
		t.Errorf("MatchValue: got %q, want %q", got.MatchValue, rule.MatchValue)
	}
	if got.Action != rule.Action {
		t.Errorf("Action: got %q, want %q", got.Action, rule.Action)
	}
	if !got.Enabled {
		t.Error("expected Enabled=true")
	}
}

func TestSaveCleanupRuleUpdate(t *testing.T) {
	c := newTestCacheForCleanup(t)

	rule := &models.CleanupRule{
		Name:          "Original",
		MatchType:     "domain",
		MatchValue:    "example.com",
		Action:        "archive",
		OlderThanDays: 14,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	if err := c.SaveCleanupRule(rule); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rule.Name = "Updated"
	rule.OlderThanDays = 60
	if err := c.SaveCleanupRule(rule); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := c.GetCleanupRule(rule.ID)
	if err != nil {
		t.Fatalf("GetCleanupRule: %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("Name: got %q, want %q", got.Name, "Updated")
	}
	if got.OlderThanDays != 60 {
		t.Errorf("OlderThanDays: got %d, want 60", got.OlderThanDays)
	}
}

func TestDeleteCleanupRule(t *testing.T) {
	c := newTestCacheForCleanup(t)

	rule := &models.CleanupRule{
		Name:          "To Delete",
		MatchType:     "sender",
		MatchValue:    "foo@bar.com",
		Action:        "delete",
		OlderThanDays: 30,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	if err := c.SaveCleanupRule(rule); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := c.DeleteCleanupRule(rule.ID); err != nil {
		t.Fatalf("DeleteCleanupRule: %v", err)
	}

	rules, err := c.GetAllCleanupRules()
	if err != nil {
		t.Fatalf("GetAllCleanupRules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestFindEmailsMatchingCleanupRule_Sender(t *testing.T) {
	c := newTestCacheForCleanup(t)

	old := time.Now().AddDate(0, 0, -40)
	recent := time.Now().AddDate(0, 0, -5)

	// Insert test emails
	emails := []models.EmailData{
		{MessageID: "1", Sender: "spam@example.com", Subject: "Old spam", Date: old, Folder: "INBOX"},
		{MessageID: "2", Sender: "spam@example.com", Subject: "Recent spam", Date: recent, Folder: "INBOX"},
		{MessageID: "3", Sender: "other@domain.com", Subject: "Other", Date: old, Folder: "INBOX"},
	}
	for _, e := range emails {
		if err := c.CacheEmail(&e); err != nil {
			t.Fatalf("CacheEmail %s: %v", e.MessageID, err)
		}
	}

	rule := &models.CleanupRule{
		MatchType:     "sender",
		MatchValue:    "spam@example.com",
		Action:        "delete",
		OlderThanDays: 30,
	}

	found, err := c.FindEmailsMatchingCleanupRule(rule)
	if err != nil {
		t.Fatalf("FindEmailsMatchingCleanupRule: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("expected 1 email, got %d", len(found))
	}
	if len(found) > 0 && found[0].MessageID != "1" {
		t.Errorf("expected MessageID=1, got %q", found[0].MessageID)
	}
}

func TestFindEmailsMatchingCleanupRule_Domain(t *testing.T) {
	c := newTestCacheForCleanup(t)

	old := time.Now().AddDate(0, 0, -40)
	recent := time.Now().AddDate(0, 0, -5)

	emails := []models.EmailData{
		{MessageID: "a", Sender: "user@newsletter.io", Subject: "Old newsletter", Date: old, Folder: "INBOX"},
		{MessageID: "b", Sender: "promo@newsletter.io", Subject: "Also old", Date: old, Folder: "INBOX"},
		{MessageID: "c", Sender: "user@newsletter.io", Subject: "Recent", Date: recent, Folder: "INBOX"},
		{MessageID: "d", Sender: "user@other.com", Subject: "Different domain", Date: old, Folder: "INBOX"},
	}
	for _, e := range emails {
		if err := c.CacheEmail(&e); err != nil {
			t.Fatalf("CacheEmail %s: %v", e.MessageID, err)
		}
	}

	rule := &models.CleanupRule{
		MatchType:     "domain",
		MatchValue:    "newsletter.io",
		Action:        "delete",
		OlderThanDays: 30,
	}

	found, err := c.FindEmailsMatchingCleanupRule(rule)
	if err != nil {
		t.Fatalf("FindEmailsMatchingCleanupRule: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("expected 2 emails, got %d", len(found))
	}
	for _, f := range found {
		if f.MessageID == "c" {
			t.Error("should not include the recent email")
		}
		if f.MessageID == "d" {
			t.Error("should not include other domain")
		}
	}
}

func TestUpdateCleanupRuleLastRun(t *testing.T) {
	c := newTestCacheForCleanup(t)

	rule := &models.CleanupRule{
		Name:          "Last run test",
		MatchType:     "sender",
		MatchValue:    "x@y.com",
		Action:        "delete",
		OlderThanDays: 7,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	if err := c.SaveCleanupRule(rule); err != nil {
		t.Fatalf("insert: %v", err)
	}

	runTime := time.Now().UTC().Truncate(time.Second)
	if err := c.UpdateCleanupRuleLastRun(rule.ID, runTime); err != nil {
		t.Fatalf("UpdateCleanupRuleLastRun: %v", err)
	}

	got, err := c.GetCleanupRule(rule.ID)
	if err != nil {
		t.Fatalf("GetCleanupRule: %v", err)
	}
	if got.LastRun == nil {
		t.Fatal("expected LastRun to be set")
	}
	if !got.LastRun.Equal(runTime) {
		t.Errorf("LastRun: got %v, want %v", got.LastRun, runTime)
	}
}
