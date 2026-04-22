package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

// makeModelWithEmails creates a minimal model with some timeline emails for testing.
func makeModelWithEmails() *Model {
	m := &Model{
		timeline: TimelineState{expandedThreads: make(map[string]bool)},
	}
	m.timeline.emails = []*models.EmailData{
		{MessageID: "msg-1", Subject: "Invoice Q1", Sender: "alice@example.com", Date: time.Now()},
		{MessageID: "msg-2", Subject: "Meeting notes", Sender: "bob@example.com", Date: time.Now()},
		{MessageID: "msg-3", Subject: "Newsletter", Sender: "news@example.com", Date: time.Now()},
	}
	return m
}

func TestChatFilterActivated(t *testing.T) {
	m := makeModelWithEmails()

	msg := ChatFilterActivatedMsg{
		Emails: []*models.EmailData{
			{MessageID: "msg-1", Subject: "Invoice Q1"},
		},
		Label: "invoices",
	}

	newM, _ := m.Update(msg)
	updated := newM.(*Model)

	if !updated.timeline.chatFilterMode {
		t.Error("expected chatFilterMode=true after filter activated")
	}
	if len(updated.timeline.chatFilteredEmails) != 1 {
		t.Errorf("expected 1 filtered email, got %d", len(updated.timeline.chatFilteredEmails))
	}
	if updated.timeline.chatFilterLabel != "invoices" {
		t.Errorf("expected label 'invoices', got %q", updated.timeline.chatFilterLabel)
	}
	if updated.activeTab != tabTimeline {
		t.Errorf("expected activeTab=tabTimeline, got %d", updated.activeTab)
	}
}

func TestChatFilterClearedOnEsc(t *testing.T) {
	m := makeModelWithEmails()
	m.timeline.chatFilterMode = true
	m.timeline.chatFilteredEmails = m.timeline.emails[:1]
	m.timeline.chatFilterLabel = "test"
	m.activeTab = tabTimeline
	m.timeline.expandedThreads = make(map[string]bool)

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := newM.(*Model)

	if updated.timeline.chatFilterMode {
		t.Error("expected chatFilterMode=false after Esc")
	}
	if updated.timeline.chatFilteredEmails != nil {
		t.Error("expected chatFilteredEmails=nil after Esc")
	}
	if updated.timeline.chatFilterLabel != "" {
		t.Errorf("expected chatFilterLabel empty after Esc, got %q", updated.timeline.chatFilterLabel)
	}
}

func TestUpdateTimelineTablePriority(t *testing.T) {
	m := makeModelWithEmails()

	// When chatFilterMode=true, filtered emails have highest priority
	m.timeline.chatFilterMode = true
	m.timeline.chatFilteredEmails = m.timeline.emails[:1]
	m.timeline.searchMode = true
	m.timeline.searchResults = m.timeline.emails[:2]

	m.updateTimelineTable()

	// chatFilterMode should remain true after updateTimelineTable
	if !m.timeline.chatFilterMode {
		t.Error("chatFilterMode should remain true after updateTimelineTable")
	}
	// Thread row map should only contain 1 entry (from chatFilteredEmails)
	if len(m.timeline.threadRowMap) != 1 {
		t.Errorf("expected 1 row from chatFilteredEmails, got %d", len(m.timeline.threadRowMap))
	}
}
