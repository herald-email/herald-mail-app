package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func mousePress(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func mouseWheelDown(x, y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown}
}

func mouseWheelUp(x, y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelUp}
}

func makeMouseTimelineModel(t *testing.T) *Model {
	t.Helper()
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.showSidebar = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m
}

func makeMouseThreadTimelineModel(t *testing.T) (*Model, *models.EmailData, *models.EmailData) {
	t.Helper()
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.showSidebar = false
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	root := &models.EmailData{
		MessageID: "thread-root",
		Sender:    "Rowan Finch <demo@demo.local>",
		Subject:   "Re: Next Steps with Cobalt Works!",
		Date:      now,
		Size:      8704,
		Folder:    "INBOX",
		UID:       26,
	}
	child := &models.EmailData{
		MessageID: "thread-child",
		Sender:    "Mina Park <mina@cobalt-works.example>",
		Subject:   "Next Steps with Cobalt Works!",
		Date:      root.Date.Add(-3 * time.Minute),
		Size:      9216,
		Folder:    "INBOX",
		UID:       27,
	}
	m.timeline.emails = []*models.EmailData{root, child}
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m, root, child
}

func TestMouseClickTabSwitchesWithoutTypingIntoCompose(t *testing.T) {
	m := makeMouseTimelineModel(t)

	contactsTabX := visibleWidth(" Herald  ") + tabMouseWidth(topLevelTabNavigation[0]) + 1
	model, _ := m.Update(mousePress(contactsTabX, 0))
	updated := model.(*Model)

	if updated.activeTab != tabContacts {
		t.Fatalf("expected mouse click on title-row tab to switch to Contacts, got tab %d", updated.activeTab)
	}
	if got := updated.composeTo.Value(); got != "" {
		t.Fatalf("expected tab mouse click not to type into compose field, got %q", got)
	}
}

func TestMouseClickTimelineRowOpensPreview(t *testing.T) {
	m := makeMouseTimelineModel(t)

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected row click to open a timeline preview")
	}
	if updated.timeline.selectedEmail.MessageID != "msg-001" {
		t.Fatalf("expected first email selected, got %s", updated.timeline.selectedEmail.MessageID)
	}
	if cmd == nil {
		t.Fatal("expected row click to request body loading")
	}
}

func TestMouseClickCollapsedThreadRootFirstSelectsPreviewWithoutExpanding(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected collapsed thread-root click to select the top email")
	}
	if updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatalf("selected email = %q, want %q", updated.timeline.selectedEmail.MessageID, root.MessageID)
	}
	if updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected first click on unselected collapsed root to keep thread collapsed")
	}
	if len(updated.timeline.threadRowMap) != 1 {
		t.Fatalf("expected collapsed thread to remain one visible row, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd == nil {
		t.Fatal("expected first collapsed root click to request body loading")
	}
}

func TestMouseClickSelectedCollapsedThreadRootExpandsWithoutRefetch(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)
	m.timeline.selectedEmail = root

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if !updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected second click on selected collapsed root to expand the thread")
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatal("expected selected root email to remain selected after expand")
	}
	if len(updated.timeline.threadRowMap) != 2 {
		t.Fatalf("expected expanded thread rows, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd != nil {
		t.Fatal("expected expand click not to refetch the already selected preview")
	}
}

func TestMouseClickExpandedThreadRootFirstSelectsPreviewWithoutFolding(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)
	m.timeline.expandedThreads[normalizeSubject(root.Subject)] = true
	m.updateTimelineTable()

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected expanded thread-root click to select the top email")
	}
	if updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatalf("selected email = %q, want %q", updated.timeline.selectedEmail.MessageID, root.MessageID)
	}
	if !updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected first click on unselected expanded root to keep thread expanded")
	}
	if len(updated.timeline.threadRowMap) != 2 {
		t.Fatalf("expected expanded thread to remain two visible rows, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd == nil {
		t.Fatal("expected first expanded root click to request body loading")
	}
}

func TestMouseClickSelectedExpandedThreadRootFoldsWithoutClearingPreview(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)
	m.timeline.expandedThreads[normalizeSubject(root.Subject)] = true
	m.timeline.selectedEmail = root
	m.updateTimelineTable()

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected second click on selected expanded root to fold the thread")
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatal("expected selected root email to remain selected after fold")
	}
	if len(updated.timeline.threadRowMap) != 1 {
		t.Fatalf("expected folded thread to become one visible row, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd != nil {
		t.Fatal("expected fold click not to refetch the already selected preview")
	}
}

func TestMouseWheelTimelinePreviewScrollsBody(t *testing.T) {
	m := makeMouseTimelineModel(t)
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{TextPlain: strings.Repeat("line\n", 80)}
	m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
	m.timeline.bodyLoading = false
	m.timeline.bodyWrappedLines = strings.Split(strings.Repeat("line\n", 80), "\n")
	m.setFocusedPanel(panelPreview)
	m.updateTableDimensions(120, 40)

	previewX := m.windowWidth - 3
	model, _ := m.Update(mouseWheelDown(previewX, 10))
	updated := model.(*Model)

	if updated.timeline.bodyScrollOffset != 3 {
		t.Fatalf("expected preview wheel to scroll body by 3 lines, got %d", updated.timeline.bodyScrollOffset)
	}

	model, _ = updated.Update(mouseWheelUp(previewX, 10))
	updated = model.(*Model)
	if updated.timeline.bodyScrollOffset != 0 {
		t.Fatalf("expected preview wheel up to scroll back to top, got %d", updated.timeline.bodyScrollOffset)
	}
}

func TestMouseModeToggleReleasesAndRestoresCapture(t *testing.T) {
	m := makeMouseTimelineModel(t)

	model, cmd := m.Update(keyRunes("m"))
	updated := model.(*Model)
	if !updated.timeline.mouseMode {
		t.Fatal("expected m to enter terminal mouse-selection mode")
	}
	if cmd != nil {
		t.Fatal("expected m to update mouse capture through the next Bubble Tea v2 view")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("MouseMode=%v, want MouseModeNone", got)
	}

	model, cmd = updated.Update(keyRunes("m"))
	updated = model.(*Model)
	if updated.timeline.mouseMode {
		t.Fatal("expected second m to restore TUI mouse capture mode")
	}
	if cmd != nil {
		t.Fatal("expected second m to update mouse capture through the next Bubble Tea v2 view")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode=%v, want MouseModeCellMotion", got)
	}
}
