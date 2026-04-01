package app

import (
	"errors"
	"testing"
	"time"

	"mail-processor/internal/models"
)

// TestToggleStarCmd verifies that StarResultMsg updates IsStarred in timelineEmails
// and sets a toast status message.
func TestToggleStarCmd(t *testing.T) {
	m := &Model{
		backend:         &stubBackend{},
		expandedThreads: make(map[string]bool),
		classifications: make(map[string]string),
	}
	m.timelineEmails = []*models.EmailData{
		{MessageID: "msg-star-1", Subject: "Star me", Sender: "alice@example.com", Date: time.Now(), IsStarred: false},
	}

	// Simulate receiving a successful StarResultMsg (starred = true).
	result, _ := m.Update(StarResultMsg{MessageID: "msg-star-1", Starred: true})
	updated := result.(*Model)

	found := false
	for _, e := range updated.timelineEmails {
		if e.MessageID == "msg-star-1" {
			found = true
			if !e.IsStarred {
				t.Error("expected IsStarred=true after StarResultMsg{Starred:true}, got false")
			}
		}
	}
	if !found {
		t.Error("email msg-star-1 not found in timelineEmails")
	}
	if updated.statusMessage != "★ Starred" {
		t.Errorf("statusMessage = %q, want %q", updated.statusMessage, "★ Starred")
	}

	// Simulate unstarring.
	result2, _ := updated.Update(StarResultMsg{MessageID: "msg-star-1", Starred: false})
	updated2 := result2.(*Model)
	for _, e := range updated2.timelineEmails {
		if e.MessageID == "msg-star-1" && e.IsStarred {
			t.Error("expected IsStarred=false after StarResultMsg{Starred:false}, got true")
		}
	}
	if updated2.statusMessage != "☆ Unstarred" {
		t.Errorf("statusMessage = %q, want %q", updated2.statusMessage, "☆ Unstarred")
	}
}

// TestToggleStarCmdError verifies that a StarResultMsg with an error sets an error status.
func TestToggleStarCmdError(t *testing.T) {
	m := &Model{
		backend:         &stubBackend{},
		expandedThreads: make(map[string]bool),
		classifications: make(map[string]string),
	}
	m.timelineEmails = []*models.EmailData{
		{MessageID: "msg-star-2", Subject: "Error case", Sender: "bob@example.com", Date: time.Now()},
	}

	result, _ := m.Update(StarResultMsg{MessageID: "msg-star-2", Err: errors.New("IMAP offline")})
	updated := result.(*Model)

	if updated.statusMessage != "Star failed: IMAP offline" {
		t.Errorf("statusMessage = %q, want %q", updated.statusMessage, "Star failed: IMAP offline")
	}
}

// TestStarredSort verifies that starred thread groups sort before unstarred ones.
func TestStarredSort(t *testing.T) {
	m := &Model{
		backend:         &stubBackend{},
		expandedThreads: make(map[string]bool),
		classifications: make(map[string]string),
	}

	now := time.Now()
	// Two emails: the unstarred one is newer (would sort first by date), starred one is older.
	m.timelineEmails = []*models.EmailData{
		{MessageID: "unstarred-1", Subject: "Unstarred recent", Sender: "bob@example.com", Date: now, IsStarred: false},
		{MessageID: "starred-1", Subject: "Starred older", Sender: "alice@example.com", Date: now.Add(-24 * time.Hour), IsStarred: true},
	}

	m.updateTimelineTable()

	if len(m.threadGroups) < 2 {
		t.Fatalf("expected 2 thread groups, got %d", len(m.threadGroups))
	}

	// After sort, the first group must be the starred one.
	if !m.threadGroups[0].emails[0].IsStarred {
		t.Error("expected starred thread group to appear first after sort")
	}
	if m.threadGroups[1].emails[0].IsStarred {
		t.Error("expected unstarred thread group to appear second after sort")
	}
}
