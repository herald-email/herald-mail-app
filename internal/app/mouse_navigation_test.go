package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func mousePress(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Type: tea.MouseLeft}
}

func mouseWheelDown(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, Type: tea.MouseWheelDown}
}

func mouseWheelUp(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress, Type: tea.MouseWheelUp}
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

func makeMouseCleanupModel(t *testing.T) *Model {
	t.Helper()
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCleanup
	m.showSidebar = false
	m.stats = makeCleanupStats()
	m.emailsBySender = makeCleanupEmails()
	if b, ok := m.backend.(*layoutBackend); ok {
		b.emailsBySender = m.emailsBySender
	}
	m.updateSummaryTable()
	m.updateDetailsTable()
	m.setFocusedPanel(panelSummary)
	return m
}

func TestMouseClickTabSwitchesWithoutTypingIntoCompose(t *testing.T) {
	m := makeMouseTimelineModel(t)

	model, _ := m.Update(mousePress(20, 1))
	updated := model.(*Model)

	if updated.activeTab != tabCompose {
		t.Fatalf("expected mouse click on tab bar to switch to Compose, got tab %d", updated.activeTab)
	}
	if got := updated.composeTo.Value(); got != "" {
		t.Fatalf("expected tab mouse click not to type into compose field, got %q", got)
	}
}

func TestMouseClickTimelineRowOpensPreview(t *testing.T) {
	m := makeMouseTimelineModel(t)

	model, cmd := m.Update(mousePress(5, 4))
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

	model, cmd := m.Update(mousePress(5, 4))
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

	model, cmd := m.Update(mousePress(5, 4))
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

	model, cmd := m.Update(mousePress(5, 4))
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

	model, cmd := m.Update(mousePress(5, 4))
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

func TestMouseClickCleanupSummaryUpdatesDetails(t *testing.T) {
	m := makeMouseCleanupModel(t)
	before, ok := m.summaryKeyAtCursor()
	if !ok {
		t.Fatal("expected initial cleanup summary row")
	}

	model, _ := m.Update(mousePress(5, 5))
	updated := model.(*Model)
	after, ok := updated.summaryKeyAtCursor()
	if !ok {
		t.Fatal("expected clicked cleanup summary row")
	}

	if before == after {
		t.Fatalf("expected click on second summary row to move selection from %q", before)
	}
	if updated.focusedPanel != panelSummary {
		t.Fatalf("expected summary click to focus summary panel, got %d", updated.focusedPanel)
	}
	if len(updated.detailsEmails) == 0 || updated.detailsEmails[0].Sender != after {
		t.Fatalf("expected details to refresh for %q, got %d detail emails", after, len(updated.detailsEmails))
	}
}

func TestMouseClickCleanupDetailsOpensPreview(t *testing.T) {
	m := makeMouseCleanupModel(t)
	m.setFocusedPanel(panelDetails)

	detailsX := m.summaryTable.Width() + panelGapWidth + 3
	model, cmd := m.Update(mousePress(detailsX, 4))
	updated := model.(*Model)

	if !updated.showCleanupPreview || updated.cleanupPreviewEmail == nil {
		t.Fatal("expected details row click to open cleanup preview")
	}
	if cmd == nil {
		t.Fatal("expected details row click to request cleanup body loading")
	}
}

func TestMouseWheelCleanupPreviewScrollsBody(t *testing.T) {
	m := makeMouseCleanupModel(t)
	m.showCleanupPreview = true
	m.cleanupPreviewEmail = m.detailsEmails[0]
	m.cleanupEmailBody = &models.EmailBody{TextPlain: strings.Repeat("line\n", 80)}
	m.cleanupBodyWrappedLines = strings.Split(strings.Repeat("line\n", 80), "\n")
	m.cleanupBodyLoading = false
	m.setFocusedPanel(panelDetails)
	m.updateTableDimensions(120, 40)

	previewX := m.windowWidth - 3
	model, _ := m.Update(mouseWheelDown(previewX, 10))
	updated := model.(*Model)

	if updated.cleanupBodyScrollOffset != 3 {
		t.Fatalf("expected cleanup preview wheel to scroll by 3 lines, got %d", updated.cleanupBodyScrollOffset)
	}
}

func TestMouseModeToggleReleasesAndRestoresCapture(t *testing.T) {
	m := makeMouseTimelineModel(t)

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	updated := model.(*Model)
	if !updated.timeline.mouseMode {
		t.Fatal("expected m to enter terminal mouse-selection mode")
	}
	if cmd == nil || reflect.TypeOf(cmd()) != reflect.TypeOf(tea.DisableMouse()) {
		t.Fatal("expected m to disable Bubble Tea mouse capture")
	}

	model, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	updated = model.(*Model)
	if updated.timeline.mouseMode {
		t.Fatal("expected second m to restore TUI mouse capture mode")
	}
	if cmd == nil || reflect.TypeOf(cmd()) != reflect.TypeOf(tea.EnableMouseCellMotion()) {
		t.Fatal("expected second m to enable Bubble Tea cell-motion mouse capture")
	}
}

func TestMouseModeToggleWorksInCleanupPreview(t *testing.T) {
	m := makeMouseCleanupModel(t)
	m.showCleanupPreview = true
	m.cleanupPreviewEmail = m.detailsEmails[0]
	m.setFocusedPanel(panelDetails)

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	updated := model.(*Model)
	if !updated.timeline.mouseMode {
		t.Fatal("expected m to enter terminal mouse-selection mode from cleanup preview")
	}
	if cmd == nil || reflect.TypeOf(cmd()) != reflect.TypeOf(tea.DisableMouse()) {
		t.Fatal("expected cleanup preview m to disable Bubble Tea mouse capture")
	}

	model, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	updated = model.(*Model)
	if updated.timeline.mouseMode {
		t.Fatal("expected second cleanup preview m to restore TUI mouse capture mode")
	}
	if cmd == nil || reflect.TypeOf(cmd()) != reflect.TypeOf(tea.EnableMouseCellMotion()) {
		t.Fatal("expected cleanup preview second m to enable Bubble Tea cell-motion mouse capture")
	}
}
