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
