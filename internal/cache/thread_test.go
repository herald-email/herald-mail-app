package cache

import (
	"testing"
	"time"
)

// insertThreadEmail inserts a test email with a specific sender, subject, and folder.
func insertThreadEmail(t *testing.T, c *Cache, messageID, sender, subject, folder string) {
	t.Helper()
	_, err := c.db.Exec(
		`INSERT INTO emails (message_id, sender, subject, date, size, has_attachments, folder)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		messageID, sender, subject, time.Now(), 100, 0, folder,
	)
	if err != nil {
		t.Fatalf("insertThreadEmail(%s): %v", messageID, err)
	}
}

func TestNormalizeSubjectGo(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Re: Hello world", "hello world"},
		{"Fwd: Meeting notes", "meeting notes"},
		{"FW: Invoice", "invoice"},
		{"AW: Antwort", "antwort"},
		{"RE: Re: Question", "question"}, // re: stripped twice (re: then re: with no space)
		{"No prefix here", "no prefix here"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeSubjectGo(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeSubjectGo(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestGetCachedFolders(t *testing.T) {
	c := newTestCache(t)

	// Insert emails in different folders
	insertThreadEmail(t, c, "msg-1", "alice@example.com", "Test 1", "INBOX")
	insertThreadEmail(t, c, "msg-2", "bob@example.com", "Test 2", "Sent")
	insertThreadEmail(t, c, "msg-3", "carol@example.com", "Test 3", "INBOX")

	folders, err := c.GetCachedFolders()
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 {
		t.Errorf("expected 2 folders, got %d: %v", len(folders), folders)
	}
	// Should be sorted alphabetically
	if folders[0] != "INBOX" || folders[1] != "Sent" {
		t.Errorf("unexpected folders: %v", folders)
	}
}

func TestGetEmailsByThread(t *testing.T) {
	c := newTestCache(t)

	insertThreadEmail(t, c, "msg-1", "alice@example.com", "Budget review Q1", "INBOX")
	insertThreadEmail(t, c, "msg-2", "bob@example.com", "Re: Budget review Q1", "INBOX")
	insertThreadEmail(t, c, "msg-3", "carol@example.com", "Fwd: Budget review Q1", "INBOX")
	insertThreadEmail(t, c, "msg-4", "dave@example.com", "Different topic", "INBOX")

	emails, err := c.GetEmailsByThread("INBOX", "Budget review Q1")
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 3 {
		t.Errorf("expected 3 emails in thread, got %d", len(emails))
	}
	// msg-4 should not be in the thread
	for _, e := range emails {
		if e.MessageID == "msg-4" {
			t.Error("email with different subject should not be in thread")
		}
	}
}
