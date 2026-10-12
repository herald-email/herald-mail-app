package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"strings"
	"testing"
)

func TestTimelineMoveShortcutOpensDestinationPicker(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.focusedPanel = panelTimeline
	m.folders = []string{"INBOX", "Receipts", "Social"}
	m.timeline.emails = timelineBulkEmails()
	m.updateTimelineTable()
	m.toggleTimelineSelection()
	m, _ = func() (*Model, tea.Cmd) {
		model, cmd := m.Update(tea.KeyPressMsg{Code: 'M', Text: "M"})
		return model.(*Model), cmd
	}()
	if !strings.Contains(m.View().Content, "Move 2 emails") {
		t.Fatalf("move shortcut did not open picker:\n%s", m.View().Content)
	}
}

type moveTestBackend struct {
	stubBackend
	refs         []models.MessageRef
	destinations []string
	failID       string
}

func (b *moveTestBackend) MoveEmailByRef(ref models.MessageRef, to string) error {
	b.refs = append(b.refs, ref)
	b.destinations = append(b.destinations, to)
	if ref.MessageID == b.failID {
		return fmt.Errorf("fixture move rejected")
	}
	return nil
}
func newMoveTestModel(t *testing.T) (*Model, *moveTestBackend) {
	b := &moveTestBackend{}
	m := newMoveModelWithBackend(t, b)
	return m, b
}

func newMoveModelWithBackend(t *testing.T, b backend.Backend) *Model {
	m := New(b, nil, "", nil, false)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.loading = false
	m.currentFolder = "INBOX"
	m.activeTab = tabTimeline
	m.focusedPanel = panelTimeline
	m.folders = []string{"INBOX", "Receipts", "Social"}
	m.timeline.emails = timelineBulkEmails()
	m.updateTimelineTable()
	m.toggleTimelineSelection()
	return m
}
func TestMailMovePartialFailureKeepsFailedSelection(t *testing.T) {
	m, b := newMoveTestModel(t)
	b.failID = "thread-old"
	m.openMailMovePicker()
	m.mailMove.input.SetValue("Receipts")
	_, cmd := m.handleMailMoveKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	result := cmd().(mailMoveResultMsg)
	m.finishMailMove(result)
	if len(b.refs) != 2 || b.refs[0].Folder != "INBOX" || b.destinations[0] != "Receipts" {
		t.Fatalf("incorrect calls: %+v %v", b.refs, b.destinations)
	}
	if m.timeline.selectedMessageIDs["thread-new"] || !m.timeline.selectedMessageIDs["thread-old"] {
		t.Fatalf("partial failure lost remaining selection: %v", m.timeline.selectedMessageIDs)
	}
	if !strings.Contains(m.statusMessage, "Moved 1 email") || !strings.Contains(m.statusMessage, "1 failed") {
		t.Fatal(m.statusMessage)
	}
}
func TestMailMoveCancelAndFilterAreNonMutating(t *testing.T) {
	m, b := newMoveTestModel(t)
	m.openMailMovePicker()
	for _, r := range "NoSuchFolderM" {
		m.handleMailMoveKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if len(m.mailMove.matches()) != 0 {
		t.Fatal("filter did not preserve typed characters")
	}
	if _, cmd := m.handleMailMoveKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("empty results started a move")
	}
	m.handleMailMoveKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.mailMove != nil || len(b.refs) != 0 || len(m.timeline.selectedMessageIDs) != 2 {
		t.Fatal("cancel changed mail or selections")
	}
}
func TestMailMoveRejectsDraftsMixedSourcesAndReadOnly(t *testing.T) {
	for _, kind := range []string{"draft", "mixed", "read-only", "busy"} {
		t.Run(kind, func(t *testing.T) {
			m, b := newMoveTestModel(t)
			switch kind {
			case "draft":
				m.timeline.emails[0].IsDraft = true
			case "mixed":
				m.timeline.emails[0].SourceID = "one"
				m.timeline.emails[1].SourceID = "two"
				m.timeline.selectedMessageIDs = map[string]bool{}
				for _, email := range m.timeline.emails[:2] {
					m.timeline.selectedMessageIDs[timelineSelectionKey(email)] = true
				}
			case "read-only":
				m.currentFolder = virtualFolderAllMailOnly
			case "busy":
				m.deleting = true
			}
			m.openMailMovePicker()
			if m.mailMove != nil || len(b.refs) != 0 {
				t.Fatal("invalid selection allowed move")
			}
		})
	}
}
func TestMailMoveUsesSelectedSourceFoldersAndRefs(t *testing.T) {
	m, b := newMoveTestModel(t)
	m.accounts = []backend.AccountInfo{{SourceID: "one"}, {SourceID: "two"}}
	m.activeSourceID = backend.AllAccountsSourceID
	m.accountFolderSnapshots = []backend.AccountFolderSnapshot{{Account: backend.AccountInfo{SourceID: "one"}, Folders: []string{"INBOX", "Personal"}}, {Account: backend.AccountInfo{SourceID: "two"}, Folders: []string{"INBOX", "Work"}}}
	m.timeline.selectedMessageIDs = map[string]bool{}
	for _, email := range m.timeline.emails[:2] {
		email.SourceID = "two"
		m.timeline.selectedMessageIDs[timelineSelectionKey(email)] = true
	}
	m.openMailMovePicker()
	if m.mailMove == nil || strings.Join(m.mailMove.folders, ",") != "Work" {
		t.Fatalf("wrong destination scope: %+v", m.mailMove)
	}
	_, cmd := m.handleMailMoveKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	cmd()
	for _, ref := range b.refs {
		if ref.SourceID != "two" {
			t.Fatalf("wrong source: %+v", ref)
		}
	}
}
func TestMailMoveProfilesAndCustomHints(t *testing.T) {
	for _, profile := range []string{"default", "vim", "emacs"} {
		cfg := &config.Config{}
		cfg.Keyboard.Profile = profile
		resolver := NewKeyboardResolver(cfg)
		if got, ok := resolver.Resolve("timeline", "normal", "M"); !ok || got != CommandMailMove {
			t.Fatalf("%s: %s %v", profile, got, ok)
		}
	}
	m, _ := newMoveTestModel(t)
	resolver := NewKeyboardResolver(nil)
	if err := resolver.ApplyCustomKeymap([]byte("extends: default\nbindings:\n  timeline:\n    normal:\n      x: mail.move\n")); err != nil {
		t.Fatal(err)
	}
	m.keyboard = resolver
	if !strings.Contains(stripANSI(m.renderKeyHints()), "x: move selected") {
		t.Fatal("custom move hint missing")
	}
	if !shortcutHelpSectionsContain(m.shortcutHelpSections(), "x", "move highlighted") {
		t.Fatal("custom move help missing")
	}
	m.handleKeyMsg(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.mailMove == nil {
		t.Fatal("custom move key did not route")
	}
}
func TestMailMovePickerFitsAndScrolls(t *testing.T) {
	m, _ := newMoveTestModel(t)
	for i := 0; i < 30; i++ {
		m.folders = append(m.folders, fmt.Sprintf("Nested/Long folder %02d", i))
	}
	m.openMailMovePicker()
	for i := 0; i < 25; i++ {
		m.handleMailMoveKey(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	for _, size := range [][2]int{{220, 50}, {80, 24}, {50, 15}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View().Content
		assertFitsWidth(t, size[0], view)
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatalf("picker exceeds height %d", size[1])
		}
	}
}

func TestMailMovePrunesAliasedSearchStateWithoutDuplicatingRows(t *testing.T) {
	m, _ := newMoveTestModel(t)
	m.timeline.emailsCache = m.timeline.emails
	m.timeline.searchResults = m.timeline.emails
	m.timeline.chatFilteredEmails = m.timeline.emails
	m.finishMailMove(mailMoveResultMsg{moved: []string{"thread-new"}, destination: "Social"})
	for _, rows := range [][]*models.EmailData{m.timeline.emails, m.timeline.emailsCache, m.timeline.searchResults, m.timeline.chatFilteredEmails} {
		if len(rows) != 2 || rows[0].MessageID != "thread-old" || rows[1].MessageID != "solo" {
			t.Fatalf("corrupted search rows: %+v", rows)
		}
	}
}

func TestMailMoveRefreshesFolderCounts(t *testing.T) {
	b := &folderStatusRecordingBackend{status: map[string]models.FolderStatus{"INBOX": {Total: 24}, "Social": {Total: 1}}}
	m := newMoveModelWithBackend(t, b)
	cmd := m.finishMailMove(mailMoveResultMsg{moved: []string{"thread-new"}, destination: "Social"})
	refreshed := false
	for _, msg := range collectCommandMessagesForTest(cmd) {
		if status, ok := msg.(FolderStatusMsg); ok {
			refreshed = true
			if status.Status["INBOX"].Total != 24 || status.Status["Social"].Total != 1 {
				t.Fatalf("missing fresh counts: %+v", status.Status)
			}
		}
	}
	if !refreshed {
		t.Fatal("move did not refresh source/destination counts")
	}
}

func TestMailMovePasteFiltersWithoutBrowseCommands(t *testing.T) {
	m, b := newMoveTestModel(t)
	m.openMailMovePicker()
	m.Update(tea.PasteMsg{Content: "Social"})
	if m.mailMove.input.Value() != "Social" || strings.Join(m.mailMove.matches(), ",") != "Social" || len(b.refs) != 0 {
		t.Fatal("paste did not stay in folder filter")
	}
}
