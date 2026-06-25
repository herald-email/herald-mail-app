package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// TestToggleStarCmd verifies that StarResultMsg updates IsStarred in timelineEmails
// and sets a toast status message.
func TestToggleStarCmd(t *testing.T) {
	m := &Model{
		backend:         &stubBackend{},
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
	}
	m.timeline.emails = []*models.EmailData{
		{MessageID: "msg-star-1", Subject: "Star me", Sender: "alice@example.com", Date: time.Now(), IsStarred: false},
	}

	// Simulate receiving a successful StarResultMsg (starred = true).
	result, _ := m.Update(StarResultMsg{MessageID: "msg-star-1", Starred: true})
	updated := result.(*Model)

	found := false
	for _, e := range updated.timeline.emails {
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
	for _, e := range updated2.timeline.emails {
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
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
	}
	m.timeline.emails = []*models.EmailData{
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
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
	}

	now := time.Now()
	// Two emails: the unstarred one is newer (would sort first by date), starred one is older.
	m.timeline.emails = []*models.EmailData{
		{MessageID: "unstarred-1", Subject: "Unstarred recent", Sender: "bob@example.com", Date: now, IsStarred: false},
		{MessageID: "starred-1", Subject: "Starred older", Sender: "alice@example.com", Date: now.Add(-24 * time.Hour), IsStarred: true},
	}

	m.updateTimelineTable()

	if len(m.timeline.threadGroups) < 2 {
		t.Fatalf("expected 2 thread groups, got %d", len(m.timeline.threadGroups))
	}

	// After sort, the first group must be the starred one.
	if !m.timeline.threadGroups[0].emails[0].IsStarred {
		t.Error("expected starred thread group to appear first after sort")
	}
	if m.timeline.threadGroups[1].emails[0].IsStarred {
		t.Error("expected unstarred thread group to appear second after sort")
	}
}

func TestTimelineStarredSingleRowUsesWarningMarkerAndSubjectStyle(t *testing.T) {
	theme := ThemeByName("herald-dark")
	m := &Model{
		backend:         &stubBackend{},
		theme:           theme,
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
	}
	now := time.Now()
	m.timeline.emails = []*models.EmailData{
		{MessageID: "starred-1", Subject: "Starred older", Sender: "Alice <alice@example.com>", Date: now.Add(-time.Hour), IsRead: true, IsStarred: true},
		{MessageID: "unstarred-1", Subject: "Unstarred recent", Sender: "Bob <bob@example.com>", Date: now, IsRead: true, IsStarred: false},
	}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}

	wantStar := theme.Severity.Warning.Style().Bold(true).Render("★")
	if sender := rows[0][1]; !strings.Contains(sender, wantStar) {
		t.Fatalf("starred sender cell should contain warning-styled star %q, got raw %q plain %q", wantStar, sender, stripANSI(sender))
	}
	wantSubject := theme.Severity.Warning.Style().Bold(true).Render("Starred older")
	if subject := rows[0][2]; subject != wantSubject {
		t.Fatalf("starred subject cell = raw %q plain %q, want raw %q", subject, stripANSI(subject), wantSubject)
	}
}

func TestTimelineStarredUnstarredRowKeepsBlankMarkerSlot(t *testing.T) {
	theme := ThemeByName("herald-dark")
	m := &Model{
		backend:         &stubBackend{},
		theme:           theme,
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
	}
	now := time.Now()
	m.timeline.emails = []*models.EmailData{
		{MessageID: "starred-1", Subject: "Starred older", Sender: "Alice <alice@example.com>", Date: now.Add(-time.Hour), IsRead: true, IsStarred: true},
		{MessageID: "unstarred-1", Subject: "Unstarred recent", Sender: "Bob <bob@example.com>", Date: now, IsRead: true, IsStarred: false},
	}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}

	sender := stripANSI(rows[1][1])
	if strings.Contains(sender, "★") {
		t.Fatalf("unstarred sender cell should not contain a star, got %q", sender)
	}
	if !strings.HasPrefix(sender, "  ") {
		t.Fatalf("unstarred sender cell should preserve unread/star indicator slots, got %q", sender)
	}
	if subject := rows[1][2]; strings.Contains(subject, "\x1b[") {
		t.Fatalf("unstarred subject cell should not receive starred styling, got raw %q plain %q", subject, stripANSI(subject))
	}
}

func TestTimelineStarredCollapsedThreadUsesWarningMarkerAndSubjectStyle(t *testing.T) {
	theme := ThemeByName("herald-dark")
	m := &Model{
		backend:         &stubBackend{},
		theme:           theme,
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
	}
	now := time.Now()
	m.timeline.emails = []*models.EmailData{
		{MessageID: "thread-1", Subject: "Project Sync", Sender: "Alice <alice@example.com>", Date: now, IsRead: true, IsStarred: true},
		{MessageID: "thread-2", Subject: "Re: Project Sync", Sender: "Bob <bob@example.com>", Date: now.Add(-time.Hour), IsRead: true, IsStarred: false},
	}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 collapsed thread row", len(rows))
	}

	wantStar := theme.Severity.Warning.Style().Bold(true).Render("★")
	if sender := rows[0][1]; !strings.Contains(sender, wantStar) {
		t.Fatalf("collapsed starred thread sender cell should contain warning-styled star %q, got raw %q plain %q", wantStar, sender, stripANSI(sender))
	}
	wantSubject := theme.Severity.Warning.Style().Bold(true).Render("[2] Project Sync")
	if subject := rows[0][2]; subject != wantSubject {
		t.Fatalf("collapsed starred thread subject cell = raw %q plain %q, want raw %q", subject, stripANSI(subject), wantSubject)
	}
}
