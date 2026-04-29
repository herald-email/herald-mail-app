package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestOpenTimelineSearch_CapturesOriginSnapshot(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.timeline.expandedThreads["hello"] = true
	m.updateTimelineTable()
	m.timelineTable.SetCursor(2)
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = m.timeline.emails[1]
	m.timeline.body = &models.EmailBody{TextPlain: "preview"}

	m.openTimelineSearch()

	if !m.timeline.searchMode {
		t.Fatal("expected search mode to open")
	}
	if m.timeline.searchOrigin == nil {
		t.Fatal("expected search origin snapshot")
	}
	if m.timeline.searchOrigin.cursor != 2 {
		t.Fatalf("expected cursor 2 in search origin, got %d", m.timeline.searchOrigin.cursor)
	}
	if !m.timeline.searchOrigin.expandedThreads["hello"] {
		t.Fatal("expected expanded threads to be captured")
	}
	if m.timeline.selectedEmail != nil {
		t.Fatal("expected preview to close when entering search")
	}
	if m.timeline.searchFocus != timelineSearchFocusInput {
		t.Fatalf("expected input focus, got %d", m.timeline.searchFocus)
	}
}

func TestTimelineSemanticSearchUsesSlashQuestionPrefix(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, cmd := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := model.(*Model)
	if cmd != nil {
		t.Fatal("expected opening search to be synchronous")
	}
	if !updated.timeline.searchMode {
		t.Fatal("expected search mode to open")
	}

	model, _ = updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	updated = model.(*Model)
	if !updated.timeline.searchMode {
		t.Fatal("expected search mode to remain open")
	}
	if got := updated.timeline.searchInput.Value(); got != "?" {
		t.Fatalf("expected semantic search prefix %q, got %q", "?", got)
	}
	if updated.timeline.searchFocus != timelineSearchFocusInput {
		t.Fatalf("expected search input focus, got %d", updated.timeline.searchFocus)
	}
	if updated.showHelp {
		t.Fatal("expected ? inside Timeline search input to type the semantic prefix, not open help")
	}
}

func TestTimelineSearch_DebouncesTypingAndIgnoresStaleTokens(t *testing.T) {
	backend := &layoutBackend{}
	m := New(backend, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.openTimelineSearch()

	if _, _, handled := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}); !handled {
		t.Fatal("expected search input to handle typing")
	}
	firstToken := m.timeline.searchToken
	if backend.searchCalls != 0 {
		t.Fatalf("expected no immediate search call, got %d", backend.searchCalls)
	}

	if _, _, handled := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")}); !handled {
		t.Fatal("expected second search input update to be handled")
	}
	secondToken := m.timeline.searchToken
	if secondToken == firstToken {
		t.Fatal("expected search token to advance after typing")
	}

	model, cmd, handled := m.handleTimelineMsg(TimelineSearchDebounceMsg{Query: "d", Token: firstToken})
	if !handled {
		t.Fatal("expected stale debounce to be handled")
	}
	if cmd != nil {
		t.Fatal("expected stale debounce to be ignored")
	}
	m = model.(*Model)

	model, cmd, handled = m.handleTimelineMsg(TimelineSearchDebounceMsg{Query: "di", Token: secondToken})
	if !handled {
		t.Fatal("expected current debounce to be handled")
	}
	if cmd == nil {
		t.Fatal("expected current debounce to trigger search")
	}
	m = model.(*Model)
	if backend.searchCalls != 0 {
		t.Fatalf("expected backend search to start only when debounce command runs, got %d", backend.searchCalls)
	}
	msg := cmd()
	if backend.searchCalls != 1 {
		t.Fatalf("expected one backend search call after debounce fired, got %d", backend.searchCalls)
	}
	if _, ok := msg.(SearchResultMsg); !ok {
		t.Fatalf("expected SearchResultMsg, got %T", msg)
	}
}

func TestTimelineSearch_EnterMovesFromInputToResults(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("swift")
	m.timeline.searchResults = []*models.EmailData{m.timeline.emails[0]}
	m.timeline.searchResultsQuery = "swift"

	model, _, handled := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected Enter in search input to be handled")
	}
	updated := model.(*Model)
	if updated.timeline.searchFocus != timelineSearchFocusResults {
		t.Fatalf("expected results focus, got %d", updated.timeline.searchFocus)
	}
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected focus on timeline list, got %d", updated.focusedPanel)
	}
}

func TestHandleEscKey_UnwindsTimelineSearchState(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.timeline.expandedThreads["hello"] = true
	m.updateTimelineTable()
	m.timelineTable.SetCursor(1)
	originCursor := m.timelineTable.Cursor()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("swift")
	m.timeline.searchResults = []*models.EmailData{m.timeline.emails[0]}
	m.timeline.searchResultsQuery = "swift"
	m.timeline.searchFocus = timelineSearchFocusResults

	model, _ := m.handleEscKey()
	m = model.(*Model)
	if m.timeline.searchFocus != timelineSearchFocusInput {
		t.Fatalf("expected first Esc to return to search input, got %d", m.timeline.searchFocus)
	}
	if m.timeline.searchInput.Value() != "swift" {
		t.Fatalf("expected query to remain, got %q", m.timeline.searchInput.Value())
	}

	model, _ = m.handleEscKey()
	m = model.(*Model)
	if m.timeline.searchMode {
		t.Fatal("expected second Esc to clear search mode")
	}
	if m.timelineTable.Cursor() != originCursor {
		t.Fatalf("expected original cursor %d restored, got %d", originCursor, m.timelineTable.Cursor())
	}
	if !m.timeline.expandedThreads["hello"] {
		t.Fatal("expected expanded threads to be restored")
	}
}

func TestHandleEscKey_FromPreviewReturnsToSearchResults(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("swift")
	m.timeline.searchResults = []*models.EmailData{m.timeline.emails[0]}
	m.timeline.searchResultsQuery = "swift"
	m.timeline.searchFocus = timelineSearchFocusResults
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{TextPlain: "hello"}
	m.focusedPanel = panelPreview

	model, _ := m.handleEscKey()
	m = model.(*Model)
	if m.timeline.selectedEmail != nil {
		t.Fatal("expected preview to close")
	}
	if m.timeline.searchFocus != timelineSearchFocusResults {
		t.Fatalf("expected to remain in result mode, got %d", m.timeline.searchFocus)
	}
	if m.focusedPanel != panelTimeline {
		t.Fatalf("expected focus to return to timeline results, got %d", m.focusedPanel)
	}
}

func TestHandleOverlayKey_CtrlIBypassesDebounceAndCtrlSIsIgnored(t *testing.T) {
	backend := &layoutBackend{}
	backend.imapSearchResult = []*models.EmailData{{MessageID: "imap-1", Sender: "server@example.com", Subject: "server result"}}
	m := New(backend, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("distributed systems")

	model, cmd, handled := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !handled {
		t.Fatal("expected Ctrl+S in search mode to be handled")
	}
	if cmd != nil {
		t.Fatal("expected Ctrl+S to be a no-op in timeline search mode")
	}
	m = model.(*Model)
	if backend.saveSearchCalls != 0 {
		t.Fatalf("expected Ctrl+S to avoid save-search path, got %d calls", backend.saveSearchCalls)
	}

	_, cmd, handled = m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyCtrlI})
	if !handled {
		t.Fatal("expected Ctrl+I to be handled")
	}
	if cmd == nil {
		t.Fatal("expected Ctrl+I to trigger immediate IMAP search")
	}
	msg := cmd()
	if backend.imapSearchCalls != 1 {
		t.Fatalf("expected one IMAP search call, got %d", backend.imapSearchCalls)
	}
	if _, ok := msg.(SearchResultMsg); !ok {
		t.Fatalf("expected SearchResultMsg, got %T", msg)
	}
}

func TestHandleTimelineKey_TabFromSearchPreviewReturnsToResults(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.searchMode = true
	m.timeline.searchFocus = timelineSearchFocusResults
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{TextPlain: "preview"}
	m.focusedPanel = panelPreview

	model, _, handled := m.handleTimelineKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Fatal("expected tab to be handled from search preview")
	}
	updated := model.(*Model)
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected focus to return to results list, got %d", updated.focusedPanel)
	}
	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected preview to remain open while returning focus to results")
	}
}
