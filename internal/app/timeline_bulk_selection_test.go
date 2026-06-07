package app

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func readDeletionRequests(t *testing.T, ch <-chan models.DeletionRequest, want int) []models.DeletionRequest {
	t.Helper()

	reqs := make([]models.DeletionRequest, 0, want)
	timeout := time.After(500 * time.Millisecond)
	for len(reqs) < want {
		select {
		case req := <-ch:
			reqs = append(reqs, req)
		case <-timeout:
			t.Fatalf("timed out waiting for %d deletion requests; got %d", want, len(reqs))
		}
	}
	return reqs
}

func timelineBulkEmails() []*models.EmailData {
	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	return []*models.EmailData{
		{MessageID: "thread-new", Sender: "alice@example.com", Subject: "Quarterly plan", Date: now, Folder: "INBOX"},
		{MessageID: "thread-old", Sender: "bob@example.com", Subject: "Re: Quarterly plan", Date: now.Add(-time.Minute), Folder: "INBOX"},
		{MessageID: "solo", Sender: "carol@example.com", Subject: "Solo update", Date: now.Add(-2 * time.Minute), Folder: "INBOX"},
	}
}

func timelineRangeEmails() []*models.EmailData {
	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	return []*models.EmailData{
		{MessageID: "msg-0", Sender: "a@example.com", Subject: "Alpha", Date: now, Folder: "INBOX"},
		{MessageID: "msg-1", Sender: "b@example.com", Subject: "Bravo", Date: now.Add(-time.Minute), Folder: "INBOX"},
		{MessageID: "msg-2", Sender: "c@example.com", Subject: "Charlie", Date: now.Add(-2 * time.Minute), Folder: "INBOX"},
		{MessageID: "msg-3", Sender: "d@example.com", Subject: "Delta", Date: now.Add(-3 * time.Minute), Folder: "INBOX"},
	}
}

func timelineRangeEmailCount(count int) []*models.EmailData {
	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	emails := make([]*models.EmailData, 0, count)
	for i := 0; i < count; i++ {
		emails = append(emails, &models.EmailData{
			MessageID: "msg-" + strconv.Itoa(i),
			Sender:    "sender-" + strconv.Itoa(i) + "@example.com",
			Subject:   "Message " + strconv.Itoa(i),
			Date:      now.Add(-time.Duration(i) * time.Minute),
			Folder:    "INBOX",
		})
	}
	return emails
}

func requireTimelineSelectedIDs(t *testing.T, m *Model, want ...string) {
	t.Helper()
	if got := len(m.timeline.selectedMessageIDs); got != len(want) {
		t.Fatalf("selected count=%d, want %d; selected=%#v", got, len(want), m.timeline.selectedMessageIDs)
	}
	for _, id := range want {
		if !m.timeline.selectedMessageIDs[id] {
			t.Fatalf("expected %s to be selected; selected=%#v", id, m.timeline.selectedMessageIDs)
		}
	}
}

func timelineKeyPress(t *testing.T, m *Model, msg tea.KeyPressMsg) *Model {
	t.Helper()
	model, _, handled := m.handleTimelineKey(msg)
	if !handled {
		t.Fatalf("expected Timeline key %q to be handled", msg.String())
	}
	return model.(*Model)
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

func TestTimelineShiftDownRangeTogglesAnchorAndNextVisibleRow(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineBulkEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"thread-new": true, "thread-old": true}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	updated := timelineKeyPress(t, m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})

	if got := updated.timelineTable.Cursor(); got != 1 {
		t.Fatalf("cursor=%d, want 1", got)
	}
	requireTimelineSelectedIDs(t, updated, "solo")
}

func TestTimelineShiftRangeTogglesFromBaseSelectionWhenShrinking(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"msg-1": true, "msg-3": true}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	updated := timelineKeyPress(t, m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	updated = timelineKeyPress(t, updated, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	requireTimelineSelectedIDs(t, updated, "msg-0", "msg-2", "msg-3")

	updated = timelineKeyPress(t, updated, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})

	if got := updated.timelineTable.Cursor(); got != 1 {
		t.Fatalf("cursor=%d, want 1", got)
	}
	requireTimelineSelectedIDs(t, updated, "msg-0", "msg-3")
	if updated.timeline.selectedMessageIDs["msg-2"] {
		t.Fatalf("expected msg-2 to restore to its unselected base state; selected=%#v", updated.timeline.selectedMessageIDs)
	}
}

func TestTimelineShiftRangeStopsWhenPlainNavigationFollows(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(1)
	m.setFocusedPanel(panelTimeline)

	updated := timelineKeyPress(t, m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	requireTimelineSelectedIDs(t, updated, "msg-1", "msg-2")

	updated = timelineKeyPress(t, updated, keyRunes("j"))

	if updated.timeline.rangeMode {
		t.Fatal("expected plain navigation to finish shifted-arrow range mode")
	}
	if got := updated.timelineTable.Cursor(); got != 3 {
		t.Fatalf("cursor=%d, want 3", got)
	}
	requireTimelineSelectedIDs(t, updated, "msg-1", "msg-2")
}

func TestTimelineShiftRangesCanBeDiscontiguousAfterPlainNavigation(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmailCount(16)
	m.updateTimelineTable()
	m.timelineTable.SetCursor(4)
	m.setFocusedPanel(panelTimeline)

	updated := timelineKeyPress(t, m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	updated = timelineKeyPress(t, updated, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	requireTimelineSelectedIDs(t, updated, "msg-4", "msg-5", "msg-6")

	updated = timelineKeyPress(t, updated, keyRunes("j"))
	for i := 0; i < 5; i++ {
		updated = timelineKeyPress(t, updated, keyRunes("j"))
	}
	if got := updated.timelineTable.Cursor(); got != 12 {
		t.Fatalf("cursor=%d, want 12", got)
	}

	updated = timelineKeyPress(t, updated, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	updated = timelineKeyPress(t, updated, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})

	requireTimelineSelectedIDs(t, updated, "msg-4", "msg-5", "msg-6", "msg-12", "msg-13", "msg-14")
	for _, id := range []string{"msg-7", "msg-8", "msg-9", "msg-10", "msg-11", "msg-15"} {
		if updated.timeline.selectedMessageIDs[id] {
			t.Fatalf("expected %s to remain unselected; selected=%#v", id, updated.timeline.selectedMessageIDs)
		}
	}
}

func TestTimelineFallbackRangeModeTogglesAndEscClearsSelection(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"msg-2": true}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(1)
	m.setFocusedPanel(panelTimeline)

	updated := timelineKeyPress(t, m, keyRunes("V"))
	status := stripANSI(updated.renderStatusBar())
	if !strings.Contains(status, "range select") {
		t.Fatalf("expected range-select status, got %q", status)
	}

	updated = timelineKeyPress(t, updated, keyRunes("j"))
	updated = timelineKeyPress(t, updated, keyRunes("j"))
	requireTimelineSelectedIDs(t, updated, "msg-1", "msg-3")

	model, _ := updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated = model.(*Model)
	status = stripANSI(updated.renderStatusBar())
	if strings.Contains(status, "range select") {
		t.Fatalf("expected range-select status to clear after Esc, got %q", status)
	}
	requireTimelineSelectedIDs(t, updated)
}

func TestTimelineEscClearsSelectionInBrowseContext(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"msg-0": true, "msg-2": true}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(1)
	m.setFocusedPanel(panelTimeline)

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := model.(*Model)

	if cmd != nil {
		t.Fatal("expected Esc bulk-unselect to stay synchronous")
	}
	requireTimelineSelectedIDs(t, updated)
	if updated.timeline.rangeMode {
		t.Fatal("expected Esc bulk-unselect to finish range mode")
	}
}

func TestTimelineEscClosesPreviewBeforeClearingSelection(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"msg-0": true, "msg-2": true}
	m.timeline.selectedEmail = m.timeline.emails[1]
	m.updateTimelineTable()
	m.timelineTable.SetCursor(1)
	m.setFocusedPanel(panelPreview)

	model, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := model.(*Model)

	if updated.timeline.selectedEmail != nil {
		t.Fatalf("expected Esc to close preview first, got %#v", updated.timeline.selectedEmail)
	}
	requireTimelineSelectedIDs(t, updated, "msg-0", "msg-2")
}

func TestTimelineEscClearsSearchBeforeSelection(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.timeline.selectedMessageIDs = map[string]bool{"msg-0": true, "msg-2": true}
	m.updateTimelineTable()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("alpha")
	m.setFocusedPanel(panelTimeline)

	model, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := model.(*Model)

	if updated.timeline.searchMode {
		t.Fatal("expected Esc to clear Timeline search before bulk unselect")
	}
	requireTimelineSelectedIDs(t, updated, "msg-0", "msg-2")
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

func TestQueueRequests_CarriesScopedMessageRef(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = "INBOX"
	m.deletionRequestCh = make(chan models.DeletionRequest, 4)
	m.deletionResultCh = make(chan models.DeletionResult, 4)
	email := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		LocalID:   "mail:work-mail:work:INBOX:scoped-msg",
		MessageID: "scoped-msg",
		Sender:    "alice@example.com",
		Subject:   "Scoped",
		Folder:    "INBOX",
	}
	m.timeline.emails = []*models.EmailData{email}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)

	cmd := m.queueRequests(false)
	if cmd == nil {
		t.Fatal("expected queueRequests to return deletion listener command")
	}

	req := <-m.deletionRequestCh
	want := email.MessageRef()
	if req.MessageRef != want {
		t.Fatalf("MessageRef = %#v, want %#v", req.MessageRef, want)
	}
	if req.SourceID != want.SourceID || req.AccountID != want.AccountID || req.LocalID != want.LocalID {
		t.Fatalf("legacy scope fields = (%q,%q,%q), want (%q,%q,%q)", req.SourceID, req.AccountID, req.LocalID, want.SourceID, want.AccountID, want.LocalID)
	}
}

func TestTimelineDeleteKeyExitsRangeModeBeforeConfirmation(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	m = timelineKeyPress(t, m, keyRunes("V"))
	m = timelineKeyPress(t, m, keyRunes("j"))

	model, _ := m.handleKeyMsg(keyRunes("d"))
	updated := model.(*Model)
	if updated.timeline.rangeMode {
		t.Fatal("expected d to exit Timeline range mode before confirmation")
	}
	if !updated.pendingDeleteConfirm {
		t.Fatal("expected d to open delete confirmation")
	}
	if !strings.Contains(updated.pendingDeleteDesc, "Delete 2 selected messages") {
		t.Fatalf("expected selected-message delete confirmation, got %q", updated.pendingDeleteDesc)
	}
	requireTimelineSelectedIDs(t, updated, "msg-0", "msg-1")
}

func TestTimelineImmediateDeleteKeyExitsRangeModeAndQueuesWithoutConfirmation(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.deletionRequestCh = make(chan models.DeletionRequest, 4)
	m.deletionResultCh = make(chan models.DeletionResult, 4)
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	m = timelineKeyPress(t, m, keyRunes("V"))
	m = timelineKeyPress(t, m, keyRunes("j"))

	model, cmd := m.handleKeyMsg(keyRunes("D"))
	updated := model.(*Model)
	if updated.timeline.rangeMode {
		t.Fatal("expected D to exit Timeline range mode before immediate delete")
	}
	if updated.pendingDeleteConfirm {
		t.Fatal("expected D to bypass delete confirmation")
	}
	if cmd == nil {
		t.Fatal("expected D to start listening for deletion results")
	}

	reqs := readDeletionRequests(t, updated.deletionRequestCh, 2)
	got := map[string]bool{}
	for _, req := range reqs {
		got[req.MessageID] = true
	}
	for _, want := range []string{"msg-0", "msg-1"} {
		if !got[want] {
			t.Fatalf("expected immediate delete to queue %s, got %#v", want, got)
		}
	}
}

func TestTimelineImmediateDeleteWithoutTargetDoesNotEnterDeletingState(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = nil
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)

	model, cmd := m.handleKeyMsg(keyRunes("D"))
	updated := model.(*Model)

	if cmd != nil {
		t.Fatal("expected no command when immediate delete has no target")
	}
	if updated.deleting {
		t.Fatal("expected immediate delete with no target to leave deleting=false")
	}
	if updated.pendingDeleteConfirm {
		t.Fatal("expected immediate delete with no target not to open confirmation")
	}
}

func TestTimelineBackspaceAliasesDeleteConfirmationAndImmediateDelete(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	model, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyBackspace})
	updated := model.(*Model)
	if !updated.pendingDeleteConfirm {
		t.Fatal("expected Backspace to open delete confirmation")
	}

	m = makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.deletionRequestCh = make(chan models.DeletionRequest, 4)
	m.deletionResultCh = make(chan models.DeletionResult, 4)
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModShift})
	updated = model.(*Model)
	if updated.pendingDeleteConfirm {
		t.Fatal("expected Shift+Backspace to bypass delete confirmation")
	}
	if cmd == nil {
		t.Fatal("expected Shift+Backspace to queue immediate delete")
	}
	reqs := readDeletionRequests(t, updated.deletionRequestCh, 1)
	if reqs[0].MessageID != "msg-0" {
		t.Fatalf("expected Shift+Backspace to queue current message msg-0, got %q", reqs[0].MessageID)
	}
}

func TestTimelineArchiveKeyExitsRangeModeBeforeConfirmation(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	m = timelineKeyPress(t, m, keyRunes("V"))
	m = timelineKeyPress(t, m, keyRunes("j"))

	model, _ := m.handleKeyMsg(keyRunes("e"))
	updated := model.(*Model)
	if updated.timeline.rangeMode {
		t.Fatal("expected e to exit Timeline range mode before confirmation")
	}
	if !updated.pendingDeleteConfirm {
		t.Fatal("expected e to open archive confirmation")
	}
	if !updated.pendingArchive || !strings.Contains(updated.pendingDeleteDesc, "Archive 2 selected messages") {
		t.Fatalf("expected selected-message archive confirmation, got archive=%v desc=%q", updated.pendingArchive, updated.pendingDeleteDesc)
	}
	requireTimelineSelectedIDs(t, updated, "msg-0", "msg-1")
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
	requireHintSegments(t, stripped, "Del: delete selected", "Shift+Del: delete now", "A: archive selected")
	for _, legacy := range []string{"d: delete selected", "D: delete now", "a: archive selected"} {
		if strings.Contains(stripped, legacy) {
			t.Fatalf("Default bottom hints should omit legacy alias %q, got:\n%s", legacy, stripped)
		}
	}
}

func TestRenderKeyHints_TimelineRangeModeShowsRangeActions(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	m = timelineKeyPress(t, m, keyRunes("V"))

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "down/up: toggle range", "V: done", "Esc: clear", "Del: delete selected", "Shift+Del: delete now")
	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "range select") || !strings.Contains(status, "1 message selected") {
		t.Fatalf("expected range status and selected count, got %q", status)
	}

	m.activeTab = tabContacts
	status = stripANSI(m.renderStatusBar())
	if strings.Contains(status, "range select") || strings.Contains(status, "messages selected") {
		t.Fatalf("expected range status to stay scoped to Timeline, got %q", status)
	}
}

func TestRenderKeyHints_TimelineShiftRangeModeShowsMomentaryActions(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineRangeEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.setFocusedPanel(panelTimeline)

	m = timelineKeyPress(t, m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "shift+↑/↓: toggle range", "plain ↑/↓: done", "Esc: clear", "Del: delete selected", "Shift+Del: delete now")
	if strings.Contains(hints, "j/k: toggle range") {
		t.Fatalf("shift range should not advertise plain j/k extension, got %q", hints)
	}
}

func TestHandleTimelineSpaceIgnoredInReadOnlyDiagnostic(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = []*models.EmailData{{MessageID: "readonly", Sender: "a@example.com", Subject: "Read only"}}
	m.updateTimelineTable()

	model, _, handled := m.handleTimelineKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if !handled {
		t.Fatal("expected Timeline space key to be handled in read-only view")
	}
	updated := model.(*Model)
	if len(updated.timeline.selectedMessageIDs) != 0 {
		t.Fatalf("expected read-only Timeline to ignore selection, got %#v", updated.timeline.selectedMessageIDs)
	}
}

func TestHandleTimelineRangeKeysIgnoredInReadOnlyDiagnostic(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = timelineRangeEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)

	for _, msg := range []tea.KeyPressMsg{
		{Code: tea.KeyDown, Mod: tea.ModShift},
		keyRunes("V"),
	} {
		model, _, _ := m.handleTimelineKey(msg)
		m = model.(*Model)
		if len(m.timeline.selectedMessageIDs) != 0 {
			t.Fatalf("expected read-only Timeline to ignore range key %q, got %#v", msg.String(), m.timeline.selectedMessageIDs)
		}
	}
}
