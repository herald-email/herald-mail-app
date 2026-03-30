package cache

import (
	"testing"
	"time"

	"mail-processor/internal/models"
)

// newTestCache creates an in-memory SQLite cache for testing.
func newTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test cache: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// --- extractDomain ---

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user@example.com", "example.com"},
		{"user@mail.example.com", "example.com"},
		{"user@a.b.example.com", "example.com"},
		{"user@example.co.uk", "example.co.uk"},
		{"user@foo.example.co.uk", "example.co.uk"},
		{"user@example.com.au", "example.com.au"},
		{"user@single", "single"},
		{"notanemail", "notanemail"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractDomain(tt.input)
			if got != tt.want {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- CacheEmail / GetCachedIDs ---

func TestCacheEmail_StoresAndRetrieves(t *testing.T) {
	c := newTestCache(t)

	email := &models.EmailData{
		MessageID: "<abc@example.com>",
		UID:       42,
		Sender:    "sender@example.com",
		Subject:   "Hello",
		Date:      time.Now().Truncate(time.Second),
		Size:      1024,
		Folder:    "INBOX",
	}

	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}

	ids, err := c.GetCachedIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedIDs: %v", err)
	}
	if !ids[email.MessageID] {
		t.Errorf("expected message ID %q to be cached", email.MessageID)
	}
}

func TestCacheEmail_UIDisPersisted(t *testing.T) {
	c := newTestCache(t)

	email := &models.EmailData{
		MessageID: "<uid-test@example.com>",
		UID:       99,
		Sender:    "sender@example.com",
		Subject:   "UID test",
		Date:      time.Now(),
		Folder:    "INBOX",
	}
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}

	uids, err := c.GetCachedUIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedUIDs: %v", err)
	}
	if !uids[99] {
		t.Errorf("expected UID 99 to be cached, got %v", uids)
	}
}

func TestGetCachedIDs_EmptyFolderReturnsEmpty(t *testing.T) {
	c := newTestCache(t)

	ids, err := c.GetCachedIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty map, got %v", ids)
	}
}

func TestGetCachedIDs_OnlyReturnsFolderEntries(t *testing.T) {
	c := newTestCache(t)

	inbox := &models.EmailData{MessageID: "<inbox@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: time.Now()}
	sent := &models.EmailData{MessageID: "<sent@x.com>", Sender: "a@x.com", Folder: "Sent", Date: time.Now()}

	for _, e := range []*models.EmailData{inbox, sent} {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	ids, err := c.GetCachedIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedIDs: %v", err)
	}
	if !ids["<inbox@x.com>"] {
		t.Error("expected INBOX message")
	}
	if ids["<sent@x.com>"] {
		t.Error("Sent message should not appear in INBOX query")
	}
}

// --- DeleteSenderEmails ---

func TestDeleteSenderEmails(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<1@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<2@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<3@x.com>", Sender: "bob@example.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.DeleteSenderEmails("alice@example.com", "INBOX"); err != nil {
		t.Fatalf("DeleteSenderEmails: %v", err)
	}

	ids, _ := c.GetCachedIDs("INBOX")
	if ids["<1@x.com>"] || ids["<2@x.com>"] {
		t.Error("alice's emails should be deleted")
	}
	if !ids["<3@x.com>"] {
		t.Error("bob's email should remain")
	}
}

// --- DeleteDomainEmails ---

func TestDeleteDomainEmails_ExactAndSubdomain(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<exact@x.com>", Sender: "user@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<sub@x.com>", Sender: "user@mail.example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<other@x.com>", Sender: "user@other.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.DeleteDomainEmails("example.com", "INBOX"); err != nil {
		t.Fatalf("DeleteDomainEmails: %v", err)
	}

	ids, _ := c.GetCachedIDs("INBOX")
	if ids["<exact@x.com>"] {
		t.Error("exact-domain email should be deleted")
	}
	if ids["<sub@x.com>"] {
		t.Error("subdomain email should be deleted")
	}
	if !ids["<other@x.com>"] {
		t.Error("unrelated domain email should remain")
	}
}

// --- GetNewestCachedDate ---

func TestGetNewestCachedDate_EmptyFolder(t *testing.T) {
	c := newTestCache(t)

	date, err := c.GetNewestCachedDate("INBOX")
	if err != nil {
		t.Fatalf("unexpected error on empty folder: %v", err)
	}
	if !date.IsZero() {
		t.Errorf("expected zero time for empty folder, got %v", date)
	}
}

func TestGetNewestCachedDate_ReturnsNewest(t *testing.T) {
	c := newTestCache(t)

	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	emails := []*models.EmailData{
		{MessageID: "<old@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: older},
		{MessageID: "<new@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: newer},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	got, err := c.GetNewestCachedDate("INBOX")
	if err != nil {
		t.Fatalf("GetNewestCachedDate: %v", err)
	}
	if !got.Equal(newer) {
		t.Errorf("expected %v, got %v", newer, got)
	}
}

// --- GetAllEmails grouping ---

func TestGetAllEmails_GroupBySender(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<1@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<2@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<3@x.com>", Sender: "bob@other.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	groups, err := c.GetAllEmails("INBOX", false)
	if err != nil {
		t.Fatalf("GetAllEmails: %v", err)
	}
	if len(groups["alice@example.com"]) != 2 {
		t.Errorf("expected 2 emails for alice, got %d", len(groups["alice@example.com"]))
	}
	if len(groups["bob@other.com"]) != 1 {
		t.Errorf("expected 1 email for bob, got %d", len(groups["bob@other.com"]))
	}
}

func TestGetAllEmails_GroupByDomain(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<1@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<2@x.com>", Sender: "bob@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<3@x.com>", Sender: "carol@other.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	groups, err := c.GetAllEmails("INBOX", true)
	if err != nil {
		t.Fatalf("GetAllEmails: %v", err)
	}
	if len(groups["example.com"]) != 2 {
		t.Errorf("expected 2 emails for example.com, got %d", len(groups["example.com"]))
	}
	if len(groups["other.com"]) != 1 {
		t.Errorf("expected 1 email for other.com, got %d", len(groups["other.com"]))
	}
}

// --- folder_sync_state ---

func TestGetSetFolderSyncState(t *testing.T) {
	c := newTestCache(t)

	// Unknown folder returns zeros, no error
	v, n, err := c.GetFolderSyncState("INBOX")
	if err != nil {
		t.Fatalf("GetFolderSyncState on empty: %v", err)
	}
	if v != 0 || n != 0 {
		t.Errorf("expected 0,0 for unknown folder, got %d,%d", v, n)
	}

	// Store and read back
	if err := c.SetFolderSyncState("INBOX", 12345, 999); err != nil {
		t.Fatalf("SetFolderSyncState: %v", err)
	}
	v, n, err = c.GetFolderSyncState("INBOX")
	if err != nil {
		t.Fatalf("GetFolderSyncState after set: %v", err)
	}
	if v != 12345 || n != 999 {
		t.Errorf("expected 12345,999 got %d,%d", v, n)
	}

	// Update replaces
	if err := c.SetFolderSyncState("INBOX", 12345, 1000); err != nil {
		t.Fatalf("SetFolderSyncState update: %v", err)
	}
	_, n, _ = c.GetFolderSyncState("INBOX")
	if n != 1000 {
		t.Errorf("expected updated uidnext=1000, got %d", n)
	}
}

// --- GetCachedUIDsAndMessageIDs ---

func TestGetCachedUIDsAndMessageIDs(t *testing.T) {
	c := newTestCache(t)

	withUID := &models.EmailData{MessageID: "<with-uid@x.com>", UID: 42, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()}
	noUID := &models.EmailData{MessageID: "<no-uid@x.com>", UID: 0, Sender: "b@x.com", Folder: "INBOX", Date: time.Now()}
	otherFolder := &models.EmailData{MessageID: "<other@x.com>", UID: 77, Sender: "c@x.com", Folder: "Sent", Date: time.Now()}

	for _, e := range []*models.EmailData{withUID, noUID, otherFolder} {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	rows, err := c.GetCachedUIDsAndMessageIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedUIDsAndMessageIDs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for INBOX, got %d", len(rows))
	}

	byID := make(map[string]uint32)
	for _, r := range rows {
		byID[r.MessageID] = r.UID
	}
	if byID["<with-uid@x.com>"] != 42 {
		t.Errorf("expected UID 42 for <with-uid@x.com>, got %d", byID["<with-uid@x.com>"])
	}
	if byID["<no-uid@x.com>"] != 0 {
		t.Errorf("expected UID 0 for <no-uid@x.com>, got %d", byID["<no-uid@x.com>"])
	}
	if _, ok := byID["<other@x.com>"]; ok {
		t.Error("Sent folder entry should not appear")
	}
}

// --- DeleteEmailsByUIDs ---

func TestDeleteEmailsByUIDs(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<uid1@x.com>", UID: 1, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid2@x.com>", UID: 2, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid3@x.com>", UID: 3, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid4@x.com>", UID: 4, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid5@x.com>", UID: 5, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.DeleteEmailsByUIDs("INBOX", []uint32{1, 3, 5}); err != nil {
		t.Fatalf("DeleteEmailsByUIDs: %v", err)
	}

	ids, _ := c.GetCachedIDs("INBOX")
	for _, gone := range []string{"<uid1@x.com>", "<uid3@x.com>", "<uid5@x.com>"} {
		if ids[gone] {
			t.Errorf("expected %s to be deleted", gone)
		}
	}
	for _, kept := range []string{"<uid2@x.com>", "<uid4@x.com>"} {
		if !ids[kept] {
			t.Errorf("expected %s to remain", kept)
		}
	}
}

func TestDeleteEmailsByUIDs_Empty(t *testing.T) {
	c := newTestCache(t)
	// No-op on empty slice should not error
	if err := c.DeleteEmailsByUIDs("INBOX", nil); err != nil {
		t.Fatalf("DeleteEmailsByUIDs with nil: %v", err)
	}
	if err := c.DeleteEmailsByUIDs("INBOX", []uint32{}); err != nil {
		t.Fatalf("DeleteEmailsByUIDs with empty: %v", err)
	}
}

// --- ClearFolder ---

func TestClearFolder(t *testing.T) {
	c := newTestCache(t)

	inbox1 := &models.EmailData{MessageID: "<i1@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: time.Now()}
	inbox2 := &models.EmailData{MessageID: "<i2@x.com>", Sender: "b@x.com", Folder: "INBOX", Date: time.Now()}
	sent := &models.EmailData{MessageID: "<s1@x.com>", Sender: "a@x.com", Folder: "Sent", Date: time.Now()}

	for _, e := range []*models.EmailData{inbox1, inbox2, sent} {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.ClearFolder("INBOX"); err != nil {
		t.Fatalf("ClearFolder: %v", err)
	}

	inboxIDs, _ := c.GetCachedIDs("INBOX")
	if len(inboxIDs) != 0 {
		t.Errorf("expected INBOX to be empty after ClearFolder, got %d entries", len(inboxIDs))
	}

	sentIDs, _ := c.GetCachedIDs("Sent")
	if !sentIDs["<s1@x.com>"] {
		t.Error("Sent folder should be unaffected by ClearFolder(INBOX)")
	}
}

// --- Rules CRUD ---

func TestSaveAndGetRule(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "test-rule",
		Enabled:      true,
		Priority:     10,
		TriggerType:  models.TriggerSender,
		TriggerValue: "spam@example.com",
		Actions: []models.RuleAction{
			{Type: models.ActionDelete},
			{Type: models.ActionNotify, NotifyTitle: "Spam!", NotifyBody: "Got one"},
		},
	}

	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}
	if rule.ID == 0 {
		t.Fatal("expected rule.ID to be set after insert")
	}

	rules, err := c.GetEnabledRules()
	if err != nil {
		t.Fatalf("GetEnabledRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	got := rules[0]
	if got.Name != rule.Name {
		t.Errorf("Name: got %q, want %q", got.Name, rule.Name)
	}
	if !got.Enabled {
		t.Error("expected Enabled=true")
	}
	if got.Priority != 10 {
		t.Errorf("Priority: got %d, want 10", got.Priority)
	}
	if got.TriggerType != models.TriggerSender {
		t.Errorf("TriggerType: got %q, want %q", got.TriggerType, models.TriggerSender)
	}
	if got.TriggerValue != "spam@example.com" {
		t.Errorf("TriggerValue: got %q", got.TriggerValue)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(got.Actions))
	}
	if got.Actions[0].Type != models.ActionDelete {
		t.Errorf("Actions[0].Type: got %q, want %q", got.Actions[0].Type, models.ActionDelete)
	}
	if got.Actions[1].NotifyTitle != "Spam!" {
		t.Errorf("Actions[1].NotifyTitle: got %q, want %q", got.Actions[1].NotifyTitle, "Spam!")
	}
}

func TestGetRuleByID(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "by-id-rule",
		Enabled:      true,
		TriggerType:  models.TriggerDomain,
		TriggerValue: "example.com",
		Actions:      []models.RuleAction{{Type: models.ActionArchive}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	got, err := c.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("GetRuleByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected rule, got nil")
	}
	if got.ID != rule.ID {
		t.Errorf("ID: got %d, want %d", got.ID, rule.ID)
	}
	if got.Name != "by-id-rule" {
		t.Errorf("Name: got %q", got.Name)
	}
}

func TestDeleteRule(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "to-delete",
		Enabled:      true,
		TriggerType:  models.TriggerCategory,
		TriggerValue: "spam",
		Actions:      []models.RuleAction{{Type: models.ActionDelete}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	if err := c.DeleteRule(rule.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}

	rules, err := c.GetEnabledRules()
	if err != nil {
		t.Fatalf("GetEnabledRules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}
}

// --- CustomPrompts CRUD ---

func TestSaveAndGetCustomPrompt(t *testing.T) {
	c := newTestCache(t)

	p := &models.CustomPrompt{
		Name:         "classify-newsletter",
		SystemText:   "You are an email classifier.",
		UserTemplate: "Is this a newsletter? {{.Subject}}",
		OutputVar:    "is_newsletter",
	}
	if err := c.SaveCustomPrompt(p); err != nil {
		t.Fatalf("SaveCustomPrompt: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected p.ID to be set after insert")
	}

	got, err := c.GetCustomPrompt(p.ID)
	if err != nil {
		t.Fatalf("GetCustomPrompt: %v", err)
	}
	if got == nil {
		t.Fatal("expected prompt, got nil")
	}
	if got.Name != p.Name {
		t.Errorf("Name: got %q, want %q", got.Name, p.Name)
	}
	if got.SystemText != p.SystemText {
		t.Errorf("SystemText: got %q", got.SystemText)
	}
	if got.UserTemplate != p.UserTemplate {
		t.Errorf("UserTemplate: got %q", got.UserTemplate)
	}
	if got.OutputVar != p.OutputVar {
		t.Errorf("OutputVar: got %q", got.OutputVar)
	}

	all, err := c.GetAllCustomPrompts()
	if err != nil {
		t.Fatalf("GetAllCustomPrompts: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(all))
	}
}

// --- AppendActionLog ---

func TestAppendActionLog(t *testing.T) {
	c := newTestCache(t)

	// Need a rule to satisfy the FK constraint
	rule := &models.Rule{
		Name:         "log-test-rule",
		Enabled:      true,
		TriggerType:  models.TriggerSender,
		TriggerValue: "a@b.com",
		Actions:      []models.RuleAction{{Type: models.ActionNotify}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	entry := &models.RuleActionLogEntry{
		RuleID:     rule.ID,
		MessageID:  "<test@example.com>",
		ActionType: models.ActionNotify,
		Status:     "ok",
		Detail:     "notification sent",
		ExecutedAt: time.Now(),
	}
	if err := c.AppendActionLog(entry); err != nil {
		t.Fatalf("AppendActionLog: %v", err)
	}
}

// --- TouchRuleLastTriggered ---

func TestTouchRuleLastTriggered(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "touch-test-rule",
		Enabled:      true,
		TriggerType:  models.TriggerSender,
		TriggerValue: "x@y.com",
		Actions:      []models.RuleAction{{Type: models.ActionDelete}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	if err := c.TouchRuleLastTriggered(rule.ID); err != nil {
		t.Fatalf("TouchRuleLastTriggered: %v", err)
	}

	got, err := c.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("GetRuleByID: %v", err)
	}
	if got.LastTriggered == nil {
		t.Error("expected LastTriggered to be non-nil after touch")
	}
}
