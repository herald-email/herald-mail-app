package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

func timelineBulkEmails() []*models.EmailData {
	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	return []*models.EmailData{
		{MessageID: "thread-new", Sender: "alice@example.com", Subject: "Quarterly plan", Date: now, Folder: "INBOX"},
		{MessageID: "thread-old", Sender: "bob@example.com", Subject: "Re: Quarterly plan", Date: now.Add(-time.Minute), Folder: "INBOX"},
		{MessageID: "solo", Sender: "carol@example.com", Subject: "Solo update", Date: now.Add(-2 * time.Minute), Folder: "INBOX"},
	}
}

func TestToggleTimelineSelection_CollapsedThreadSelectsRepresentedMessages(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineBulkEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)

	m.toggleTimelineSelection()

	if got := len(m.timeline.selectedMessageIDs); got != 2 {
		t.Fatalf("expected collapsed thread selection to include 2 messages, got %d", got)
	}
	for _, id := range []string{"thread-new", "thread-old"} {
		if !m.timeline.selectedMessageIDs[id] {
			t.Fatalf("expected %s to be selected", id)
		}
	}
	row := m.timelineTable.Rows()[0]
	if got := row[0]; got != "✓" {
		t.Fatalf("expected collapsed thread row to show full checkmark, got %q", got)
	}
}

func TestToggleTimelineSelection_ExpandedRowSelectsOnlyCurrentEmail(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineBulkEmails()
	m.timeline.expandedThreads["quarterly plan"] = true
	m.updateTimelineTable()
	m.timelineTable.SetCursor(1)

	m.toggleTimelineSelection()

	if got := len(m.timeline.selectedMessageIDs); got != 1 {
		t.Fatalf("expected one expanded child row selection, got %d", got)
	}
	if !m.timeline.selectedMessageIDs["thread-old"] {
		t.Fatal("expected current expanded child email to be selected")
	}
	if m.timeline.selectedMessageIDs["thread-new"] {
		t.Fatal("did not expect expanded sibling to be selected")
	}
}

func TestQueueRequests_TimelineSelectionTakesPriorityOverCursor(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = "INBOX"
	m.deletionRequestCh = make(chan models.DeletionRequest, 4)
	m.deletionResultCh = make(chan models.DeletionResult, 4)
	m.timeline.emails = timelineBulkEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"solo": true}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)

	cmd := m.queueRequests(false)
	if cmd == nil {
		t.Fatal("expected queueRequests to return deletion listener command")
	}

	req := <-m.deletionRequestCh
	if req.MessageID != "solo" {
		t.Fatalf("expected selected message to be queued, got %q", req.MessageID)
	}
	if m.deletionsTotal != 1 || m.deletionsPending != 1 {
		t.Fatalf("expected one queued deletion, got pending=%d total=%d", m.deletionsPending, m.deletionsTotal)
	}
}

func TestBuildDeleteDesc_TimelineSelectionMentionsDraftDiscard(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = []*models.EmailData{
		{MessageID: "normal", Sender: "a@example.com", Subject: "Normal", Folder: "INBOX"},
		{MessageID: "draft", Sender: "me@example.com", Subject: "Draft", Folder: "Drafts", IsDraft: true},
	}
	m.timeline.selectedMessageIDs = map[string]bool{"normal": true, "draft": true}
	m.updateTimelineTable()

	desc := m.buildDeleteDesc()
	if !strings.Contains(desc, "Delete 2 selected messages") || !strings.Contains(desc, "discard 1 draft") {
		t.Fatalf("expected selected delete copy to mention draft discard, got %q", desc)
	}
}

func TestBuildArchiveDesc_TimelineSelectionSkipsDrafts(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = []*models.EmailData{
		{MessageID: "normal", Sender: "a@example.com", Subject: "Normal", Folder: "INBOX"},
		{MessageID: "draft", Sender: "me@example.com", Subject: "Draft", Folder: "Drafts", IsDraft: true},
	}
	m.timeline.selectedMessageIDs = map[string]bool{"normal": true, "draft": true}
	m.updateTimelineTable()

	desc := m.buildArchiveDesc()
	if !strings.Contains(desc, "Archive 1 selected message") || !strings.Contains(desc, "skipping 1 draft") {
		t.Fatalf("expected selected archive copy to mention skipped draft, got %q", desc)
	}

	m.timeline.selectedMessageIDs = map[string]bool{"draft": true}
	if desc := m.buildArchiveDesc(); desc != "" {
		t.Fatalf("expected all-draft archive selection to have no confirmation, got %q", desc)
	}
}

func TestBuildArchiveDesc_CollapsedThreadMentionsSkippedDrafts(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	m.timeline.emails = []*models.EmailData{
		{MessageID: "normal", Sender: "a@example.com", Subject: "Plan", Date: now, Folder: "INBOX"},
		{MessageID: "draft", Sender: "me@example.com", Subject: "Re: Plan", Date: now.Add(-time.Minute), Folder: "Drafts", IsDraft: true},
	}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)

	desc := m.buildArchiveDesc()
	if !strings.Contains(desc, "Archive thread \"Plan\" (1 message") || !strings.Contains(desc, "skipping 1 draft") {
		t.Fatalf("expected collapsed thread archive copy to mention skipped draft, got %q", desc)
	}
}

func TestTimelineSelectionPrunedToVisibleWorkingSet(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = []*models.EmailData{{MessageID: "visible", Sender: "a@example.com", Subject: "Visible"}}
	m.timeline.selectedMessageIDs = map[string]bool{"visible": true, "gone": true}

	m.updateTimelineTable()

	if !m.timeline.selectedMessageIDs["visible"] {
		t.Fatal("expected visible selected message to remain selected")
	}
	if m.timeline.selectedMessageIDs["gone"] {
		t.Fatal("expected missing selected message to be pruned")
	}
}

func TestRenderStatusBar_TimelineSelectionIsTimelineScoped(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineBulkEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"thread-new": true, "solo": true}
	m.updateTimelineTable()

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "2 messages selected") {
		t.Fatalf("expected Timeline selected count in status, got %q", status)
	}

	m.activeTab = tabContacts
	status = stripANSI(m.renderStatusBar())
	if strings.Contains(status, "messages selected") {
		t.Fatalf("expected Timeline selected count to stay scoped to Timeline, got %q", status)
	}
}

func TestRenderKeyHints_TimelineSelectionActionsStayVisibleAt80Cols(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineBulkEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"solo": true}
	m.updateTimelineTable()
	m.focusedPanel = panelTimeline

	hints := m.renderKeyHints()
	assertFitsWidth(t, 80, hints)
	stripped := stripANSI(hints)
	requireHintSegments(t, stripped, "space: select", "D: delete selected", "e: archive selected")
}

func TestHandleTimelineSpaceIgnoredInReadOnlyDiagnostic(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = []*models.EmailData{{MessageID: "readonly", Sender: "a@example.com", Subject: "Read only"}}
	m.updateTimelineTable()

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeySpace})
	if !handled {
		t.Fatal("expected Timeline space key to be handled in read-only view")
	}
	updated := model.(*Model)
	if len(updated.timeline.selectedMessageIDs) != 0 {
		t.Fatalf("expected read-only Timeline to ignore selection, got %#v", updated.timeline.selectedMessageIDs)
	}
}
