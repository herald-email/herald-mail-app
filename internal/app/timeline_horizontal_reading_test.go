package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type readTrackingBackend struct {
	stubBackend
	markReadIDs   []string
	markUnreadIDs []string
}

func (b *readTrackingBackend) MarkRead(messageID, _ string) error {
	b.markReadIDs = append(b.markReadIDs, messageID)
	return nil
}

func (b *readTrackingBackend) MarkUnread(messageID, _ string) error {
	b.markUnreadIDs = append(b.markUnreadIDs, messageID)
	return nil
}

func makeHorizontalTimelineModel(t *testing.T, backend *readTrackingBackend) *Model {
	t.Helper()
	m := New(backend, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*Model)
	m.activeTab = tabTimeline
	m.loading = false
	m.currentFolder = "INBOX"
	m.folderStatus = map[string]models.FolderStatus{
		"INBOX": {Unseen: 2, Total: 3},
	}
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "msg-001",
			UID:       101,
			Sender:    "alice@example.com",
			Subject:   "Meeting tomorrow",
			Folder:    "INBOX",
		},
		{
			MessageID: "msg-002",
			UID:       102,
			Sender:    "bob@example.com",
			Subject:   "Invoice #4521",
			Folder:    "INBOX",
		},
	}
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m
}

func TestHorizontalTimelineRightOpensPreviewThenFocusesExistingPreview(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})

	model, cmd, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Fatal("expected right arrow to be handled")
	}
	if cmd == nil {
		t.Fatal("expected right arrow to start preview body load")
	}
	updated := model.(*Model)
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected focus to remain on Timeline, got %d", updated.focusedPanel)
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != "msg-001" {
		t.Fatalf("expected msg-001 preview, got %#v", updated.timeline.selectedEmail)
	}

	updated.timeline.bodyMessageID = "msg-001"
	updated.timeline.body = &models.EmailBody{TextPlain: "preview"}
	model, cmd, handled = updated.handleTimelineKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Fatal("expected right arrow on an open preview to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected right arrow to focus existing preview without reloading body, got %T", cmd)
	}
	updated = model.(*Model)
	if updated.focusedPanel != panelPreview {
		t.Fatalf("expected right arrow to focus preview when one is open, got %d", updated.focusedPanel)
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != "msg-001" {
		t.Fatalf("expected existing msg-001 preview to remain selected, got %#v", updated.timeline.selectedEmail)
	}

	updated.setFocusedPanel(panelTimeline)
	updated.timelineTable.MoveDown(1)
	model, cmd, handled = updated.handleTimelineKey(keyRunes("]"))
	if !handled {
		t.Fatal("expected ] to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected ] to focus existing preview without reloading body, got %T", cmd)
	}
	updated = model.(*Model)
	if updated.focusedPanel != panelPreview {
		t.Fatalf("expected ] to focus preview when one is open, got %d", updated.focusedPanel)
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != "msg-001" {
		t.Fatalf("expected ] to leave existing msg-001 preview selected, got %#v", updated.timeline.selectedEmail)
	}
}

func TestHorizontalTimelineRightOnCollapsedThreadPreviewsNewestWithoutExpanding(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	m.timeline.emails = []*models.EmailData{
		{MessageID: "newest", UID: 201, Sender: "alice@example.com", Subject: "Re: Roadmap", Folder: "INBOX"},
		{MessageID: "older", UID: 200, Sender: "bob@example.com", Subject: "Roadmap", Folder: "INBOX"},
	}
	m.updateTimelineTable()

	model, cmd, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Fatal("expected right arrow to be handled")
	}
	if cmd == nil {
		t.Fatal("expected collapsed thread preview to fetch body")
	}
	updated := model.(*Model)
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != "newest" {
		t.Fatalf("expected newest thread message preview, got %#v", updated.timeline.selectedEmail)
	}
	if updated.timeline.expandedThreads[normalizeSubject("Roadmap")] {
		t.Fatal("expected right arrow preview to leave collapsed thread folded")
	}
}

func TestHorizontalTimelineLeftFromPreviewFocusesTimelineWithoutClosing(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "msg-001"
	m.timeline.body = &models.EmailBody{TextPlain: "preview"}
	m.setFocusedPanel(panelPreview)

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("expected left arrow to be handled")
	}
	updated := model.(*Model)
	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected preview to remain open")
	}
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected left arrow to return focus to Timeline, got %d", updated.focusedPanel)
	}
	if updated.timeline.body == nil || updated.timeline.bodyMessageID != "msg-001" {
		t.Fatalf("expected preview body to remain loaded, got body=%#v messageID=%q", updated.timeline.body, updated.timeline.bodyMessageID)
	}
}

func TestHorizontalTimelineLeftFromTimelineFoldsExpandedThreadBeforeFolders(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	m.timeline.emails = []*models.EmailData{
		{MessageID: "newest", UID: 201, Sender: "alice@example.com", Subject: "Re: Roadmap", Folder: "INBOX"},
		{MessageID: "older", UID: 200, Sender: "bob@example.com", Subject: "Roadmap", Folder: "INBOX"},
	}
	threadKey := normalizeSubject("Roadmap")
	m.timeline.expandedThreads[threadKey] = true
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "newest"
	m.timeline.body = &models.EmailBody{TextPlain: "thread preview"}
	m.setFocusedPanel(panelTimeline)

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("expected left arrow to be handled")
	}
	updated := model.(*Model)
	if updated.timeline.expandedThreads[threadKey] {
		t.Fatal("expected left arrow on expanded thread row to fold the thread")
	}
	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected folding a thread not to close the open preview")
	}
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected focus to stay on Timeline after folding, got %d", updated.focusedPanel)
	}
}

func TestHorizontalTimelineLeftFromTimelineClosesPreviewAndFocusesFolders(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "msg-001"
	m.timeline.body = &models.EmailBody{TextPlain: "preview"}
	m.showSidebar = false
	m.updateTableDimensions(140, 40)
	m.setFocusedPanel(panelTimeline)

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("expected left arrow to be handled")
	}
	updated := model.(*Model)
	if updated.timeline.selectedEmail != nil {
		t.Fatalf("expected single-email preview to close, got %#v", updated.timeline.selectedEmail)
	}
	if !updated.showSidebar {
		t.Fatal("expected left arrow to show sidebar after closing single-email preview")
	}
	if updated.focusedPanel != panelSidebar {
		t.Fatalf("expected left arrow to focus sidebar after closing preview, got %d", updated.focusedPanel)
	}

	updated.setFocusedPanel(panelTimeline)

	updated.showSidebar = false
	updated.updateTableDimensions(140, 40)
	model, _, handled = updated.handleTimelineKey(keyRunes("["))
	if !handled {
		t.Fatal("expected [ to be handled")
	}
	updated = model.(*Model)
	if !updated.showSidebar {
		t.Fatal("expected [ to show sidebar when no preview is open")
	}
	if updated.focusedPanel != panelSidebar {
		t.Fatalf("expected [ to focus sidebar, got %d", updated.focusedPanel)
	}

	collapsed := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	collapsed.timeline.emails = []*models.EmailData{
		{MessageID: "newest", UID: 201, Sender: "alice@example.com", Subject: "Re: Roadmap", Folder: "INBOX"},
		{MessageID: "older", UID: 200, Sender: "bob@example.com", Subject: "Roadmap", Folder: "INBOX"},
	}
	collapsed.updateTimelineTable()
	collapsed.timelineTable.SetCursor(0)
	collapsed.timeline.selectedEmail = collapsed.timeline.emails[0]
	collapsed.timeline.bodyMessageID = "newest"
	collapsed.timeline.body = &models.EmailBody{TextPlain: "thread preview"}
	collapsed.showSidebar = false
	collapsed.updateTableDimensions(140, 40)
	collapsed.setFocusedPanel(panelTimeline)

	model, _, handled = collapsed.handleTimelineKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("expected left arrow on collapsed thread to be handled")
	}
	collapsed = model.(*Model)
	if collapsed.timeline.selectedEmail != nil {
		t.Fatalf("expected collapsed-thread preview to close, got %#v", collapsed.timeline.selectedEmail)
	}
	if collapsed.focusedPanel != panelSidebar {
		t.Fatalf("expected collapsed-thread left arrow to focus sidebar, got %d", collapsed.focusedPanel)
	}
}

func TestHorizontalTimelineRightFromSidebarFocusesTimeline(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	m.showSidebar = true
	m.updateTableDimensions(140, 40)
	m.setFocusedPanel(panelSidebar)

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Fatal("expected right arrow from sidebar to be handled")
	}
	updated := model.(*Model)
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected right arrow to focus Timeline, got %d", updated.focusedPanel)
	}
	if updated.timeline.selectedEmail != nil {
		t.Fatalf("expected sidebar right arrow not to open preview, got %#v", updated.timeline.selectedEmail)
	}
}

func TestHorizontalTimelineBracketKeepsPreviewAttachmentPrecedence(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "msg-001"
	m.timeline.body = &models.EmailBody{
		Attachments: []models.Attachment{
			{Filename: "first.pdf"},
			{Filename: "second.pdf"},
		},
	}
	m.setFocusedPanel(panelPreview)

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("expected left arrow to be handled as pane navigation")
	}
	updated := model.(*Model)
	if updated.timeline.selectedAttachment != 0 {
		t.Fatalf("expected left arrow not to move attachment selection, got %d", updated.timeline.selectedAttachment)
	}
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected left arrow to move focus to Timeline, got %d", updated.focusedPanel)
	}
	updated.setFocusedPanel(panelPreview)

	model, _, handled = updated.handleTimelineKey(keyRunes("]"))
	if !handled {
		t.Fatal("expected ] to be handled as attachment navigation")
	}
	updated = model.(*Model)
	if updated.timeline.selectedAttachment != 1 {
		t.Fatalf("expected ] to advance attachment, got %d", updated.timeline.selectedAttachment)
	}
	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected attachment navigation not to close preview")
	}

	model, _, handled = updated.handleTimelineKey(keyRunes("["))
	if !handled {
		t.Fatal("expected [ to be handled as attachment navigation")
	}
	updated = model.(*Model)
	if updated.timeline.selectedAttachment != 0 {
		t.Fatalf("expected [ to move attachment back, got %d", updated.timeline.selectedAttachment)
	}
	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected attachment navigation not to close preview")
	}
}

func TestTimelineBodyLoadMarksReadAndRefreshesRows(t *testing.T) {
	backend := &readTrackingBackend{}
	m := makeHorizontalTimelineModel(t, backend)
	email := m.timeline.emails[0]
	email.IsRead = false
	m.timeline.selectedEmail = email
	m.timeline.bodyLoading = true
	m.updateTimelineTable()
	if got := stripANSI(m.timelineTable.Rows()[0][1]); !strings.Contains(got, "●") {
		t.Fatalf("expected unread dot before body load, got %q", got)
	}

	model, cmd, handled := m.handleTimelineMsg(EmailBodyMsg{
		MessageID: email.MessageID,
		Body:      &models.EmailBody{TextPlain: "loaded body"},
	})
	if !handled {
		t.Fatal("expected EmailBodyMsg to be handled")
	}
	updated := model.(*Model)
	if !email.IsRead {
		t.Fatal("expected successful body load to mark email read locally")
	}
	if got := stripANSI(updated.timelineTable.Rows()[0][1]); strings.Contains(got, "●") {
		t.Fatalf("expected unread dot to disappear after body load, got %q", got)
	}
	if cmd == nil {
		t.Fatal("expected mark-read command")
	}
	cmd()
	if len(backend.markReadIDs) != 1 || backend.markReadIDs[0] != email.MessageID {
		t.Fatalf("expected backend MarkRead for %q, got %#v", email.MessageID, backend.markReadIDs)
	}
}

func TestTimelineMarkUnreadKeyUpdatesRowsAndBlocksReadOnly(t *testing.T) {
	backend := &readTrackingBackend{}
	m := makeHorizontalTimelineModel(t, backend)
	email := m.timeline.emails[0]
	email.IsRead = true
	m.timeline.selectedEmail = email
	m.updateTimelineTable()

	model, cmd, handled := m.handleTimelineKey(keyRunes("U"))
	if !handled {
		t.Fatal("expected U to be handled")
	}
	updated := model.(*Model)
	if email.IsRead {
		t.Fatal("expected U to mark email unread locally")
	}
	if got := stripANSI(updated.timelineTable.Rows()[0][1]); !strings.Contains(got, "●") {
		t.Fatalf("expected unread dot after U, got %q", got)
	}
	if cmd == nil {
		t.Fatal("expected mark-unread command")
	}
	cmd()
	if len(backend.markUnreadIDs) != 1 || backend.markUnreadIDs[0] != email.MessageID {
		t.Fatalf("expected backend MarkUnread for %q, got %#v", email.MessageID, backend.markUnreadIDs)
	}

	readonlyBackend := &readTrackingBackend{}
	readonly := makeHorizontalTimelineModel(t, readonlyBackend)
	readonly.currentFolder = virtualFolderAllMailOnly
	readonly.timeline.emails[0].IsRead = true
	readonly.timeline.selectedEmail = readonly.timeline.emails[0]
	model, cmd, handled = readonly.handleTimelineKey(keyRunes("U"))
	if !handled {
		t.Fatal("expected U to be consumed in read-only Timeline")
	}
	readonly = model.(*Model)
	if !readonly.timeline.emails[0].IsRead {
		t.Fatal("expected read-only Timeline to leave read state unchanged")
	}
	if cmd != nil {
		t.Fatalf("expected no command in read-only Timeline, got %T", cmd)
	}
	if len(readonlyBackend.markUnreadIDs) != 0 {
		t.Fatalf("expected read-only Timeline not to call MarkUnread, got %#v", readonlyBackend.markUnreadIDs)
	}
}

func TestHorizontalTimelineHintsAndHelpDescribeMovementAndUnread(t *testing.T) {
	m := makeHorizontalTimelineModel(t, &readTrackingBackend{})
	hints := stripANSI(m.renderKeyHints())
	for _, want := range []string{"right/]: preview", "left/[: folders", "U: unread"} {
		if !strings.Contains(hints, want) {
			t.Fatalf("expected Timeline hints to include %q, got:\n%s", want, hints)
		}
	}

	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "msg-001"
	m.timeline.body = &models.EmailBody{TextPlain: "preview"}
	m.setFocusedPanel(panelTimeline)
	hints = stripANSI(m.renderKeyHints())
	for _, want := range []string{"right/]: focus preview", "left/[: fold/folders"} {
		if !strings.Contains(hints, want) {
			t.Fatalf("expected open-preview Timeline hints to include %q, got:\n%s", want, hints)
		}
	}

	m.setFocusedPanel(panelPreview)
	help := m.timelineShortcutHelpSection()
	joined := ""
	for _, entry := range help.Entries {
		joined += entry.Key + " " + entry.Desc + "\n"
	}
	for _, want := range []string{"Left arrow", "Timeline list", "Right / ]", "U", "mark unread"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected Timeline help to include %q, got:\n%s", want, joined)
		}
	}
}
