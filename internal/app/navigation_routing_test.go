package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func altKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: string(r), Code: r, Mod: tea.ModAlt}
}

func functionKey(n int) tea.KeyPressMsg {
	switch n {
	case 1:
		return tea.KeyPressMsg{Code: tea.KeyF1}
	case 2:
		return tea.KeyPressMsg{Code: tea.KeyF2}
	case 3:
		return tea.KeyPressMsg{Code: tea.KeyF3}
	case 4:
		return tea.KeyPressMsg{Code: tea.KeyF4}
	default:
		return tea.KeyPressMsg{}
	}
}

func commandIsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestHandleOverlayKey_ChatEscapeRestoresTimelineFocus(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.showChat = true
	m.focusedPanel = panelChat
	m.windowWidth = 120
	m.windowHeight = 40

	model, _, handled := m.handleOverlayKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatal("expected chat escape to be handled")
	}

	updated := model.(*Model)
	if updated.showChat {
		t.Fatal("expected chat panel to close on Esc")
	}
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected focus to return to timeline, got %d", updated.focusedPanel)
	}
}

func TestHandleTabKey_SwitchingAwayFromComposeStartsDraftPersistence(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabCompose
	m.loading = false
	m.composeTo.SetValue("alice@example.com")
	m.composeBody.SetValue("hello")

	model, cmd, handled := m.handleTabKey(keyRune('1'))
	if !handled {
		t.Fatal("expected tab key to be handled")
	}
	if cmd == nil {
		t.Fatal("expected compose exit to produce draft-related command(s)")
	}

	updated := model.(*Model)
	if updated.activeTab != tabTimeline {
		t.Fatalf("expected active tab to switch to timeline, got %d", updated.activeTab)
	}
	if !updated.draftSaving {
		t.Fatal("expected draftSaving=true when leaving non-empty compose tab")
	}
}

func TestComposePlainQAndDigitsInsertTextInsteadOfGlobalActions(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeField = 4
	m.composeTo.Blur()
	m.composeBody.Focus()

	for _, key := range []string{"q", "1", "2", "3", "4"} {
		model, cmd := m.handleKeyMsg(keyRunes(key))
		if commandIsQuit(cmd) {
			t.Fatalf("plain %q from compose returned quit command", key)
		}
		m = model.(*Model)
		if m.activeTab != tabCompose {
			t.Fatalf("plain %q changed active tab to %d", key, m.activeTab)
		}
	}

	if got, want := m.composeBody.Value(), "q1234"; got != want {
		t.Fatalf("compose body value=%q, want %q", got, want)
	}
}

func TestBrowsePlainQStillQuits(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline

	_, cmd := m.handleKeyMsg(keyRunes("q"))
	if !commandIsQuit(cmd) {
		t.Fatal("expected plain q to quit from browse context")
	}
}

func TestComposeFunctionTabSwitchesAndPersistsDraft(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeTo.SetValue("alice@example.com")
	m.composeBody.SetValue("draft body")

	model, cmd := m.handleKeyMsg(functionKey(1))
	updated := model.(*Model)

	if updated.activeTab != tabTimeline {
		t.Fatalf("F1 activeTab=%d, want Timeline", updated.activeTab)
	}
	if cmd == nil {
		t.Fatal("expected F1 leaving non-empty compose to produce draft persistence command")
	}
	if !updated.draftSaving {
		t.Fatal("expected F1 leaving non-empty compose to mark draftSaving")
	}
}

func TestComposeFunctionKeysSwitchTabsAndDoNotTypeIntoDraft(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{name: "F1", key: functionKey(1), want: tabTimeline},
		{name: "F2", key: functionKey(2), want: tabContacts},
		{name: "F3", key: functionKey(3), want: tabContacts},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, 140, 40)
			m.activeTab = tabCompose
			m.composeField = 4
			m.composeTo.SetValue("alice@example.com")
			m.composeTo.Blur()
			m.composeBody.Focus()
			m.composeBody.SetValue("draft body")

			model, cmd := m.handleKeyMsg(tc.key)
			updated := model.(*Model)

			if updated.activeTab != tc.want {
				t.Fatalf("%s activeTab=%d, want %d", tc.name, updated.activeTab, tc.want)
			}
			if got := updated.composeBody.Value(); got != "draft body" {
				t.Fatalf("%s typed into draft, body=%q", tc.name, got)
			}
			if cmd == nil {
				t.Fatalf("expected %s leaving non-empty compose to produce draft persistence command", tc.name)
			}
			if !updated.draftSaving {
				t.Fatalf("expected %s leaving non-empty compose to mark draftSaving", tc.name)
			}
		})
	}
}

func TestReplyComposeEscapePromptsBeforeLeaving(t *testing.T) {
	m := newReplyPreservedComposeModel()
	m.composeReturnSet = true
	m.composeReturnTab = tabTimeline
	m.composeReturnPanel = panelTimeline

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := model.(*Model)

	if cmd != nil {
		t.Fatalf("reply Compose Esc returned command %T before prompt answer", cmd)
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab=%d, want Compose while prompt is unanswered", updated.activeTab)
	}
	if !updated.pendingComposeExitPrompt {
		t.Fatal("expected reply Compose Esc to ask whether to keep or discard the draft")
	}
	if updated.draftSaving {
		t.Fatal("reply Compose prompt should not start draft saving before the user chooses keep")
	}
	status := stripANSI(updated.renderStatusBar())
	if !strings.Contains(status, "Keep reply draft") || !strings.Contains(status, "discard") {
		t.Fatalf("reply Compose prompt status missing keep/discard affordances:\n%s", status)
	}
}

func TestReplyComposeExitPromptKeepSavesDraftAndReturnsTimeline(t *testing.T) {
	m := newReplyPreservedComposeModel()
	m.composeReturnSet = true
	m.composeReturnTab = tabTimeline
	m.composeReturnPanel = panelTimeline
	model, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := model.(*Model)

	model, cmd, handled := updated.handleOverlayKey(keyRunes("k"))
	updated = model.(*Model)

	if !handled {
		t.Fatal("expected keep key to be handled by reply/forward exit prompt")
	}
	if updated.pendingComposeExitPrompt {
		t.Fatal("keep should close the reply/forward exit prompt")
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab=%d, want Timeline after keeping draft", updated.activeTab)
	}
	if !updated.draftSaving {
		t.Fatal("keep should start draft saving before leaving reply Compose")
	}
	if cmd == nil {
		t.Fatal("keep should return a draft save command")
	}
}

func TestForwardComposeExitPromptDiscardDeletesSavedDraftAndReturnsTimeline(t *testing.T) {
	backend := &draftDeleteRecordingBackend{}
	m := newPreservedComposeModel()
	m.backend = backend
	m.composeReturnSet = true
	m.composeReturnTab = tabTimeline
	m.composeReturnPanel = panelTimeline
	m.lastDraftUID = 42
	m.lastDraftFolder = "Drafts"
	m.lastDraftReplaceable = true

	model, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := model.(*Model)
	if !updated.pendingComposeExitPrompt {
		t.Fatal("expected forward Compose Esc to ask whether to keep or discard the draft")
	}

	model, cmd, handled := updated.handleOverlayKey(keyRunes("d"))
	updated = model.(*Model)

	if !handled {
		t.Fatal("expected discard key to be handled by reply/forward exit prompt")
	}
	if updated.pendingComposeExitPrompt {
		t.Fatal("discard should close the reply/forward exit prompt")
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab=%d, want Timeline after discarding draft", updated.activeTab)
	}
	if updated.draftSaving {
		t.Fatal("discard should not start draft saving")
	}
	if updated.composePreserved != nil {
		t.Fatal("discard should clear preserved reply/forward context")
	}
	if updated.lastDraftUID != 0 || updated.lastDraftFolder != "" || updated.lastDraftReplaceable {
		t.Fatalf("discard should clear tracked draft state, got uid=%d folder=%q replaceable=%v", updated.lastDraftUID, updated.lastDraftFolder, updated.lastDraftReplaceable)
	}
	if cmd == nil {
		t.Fatal("discard should delete the already saved replaceable draft")
	}
	raw := cmd()
	if _, ok := raw.(DraftDeletedMsg); !ok {
		t.Fatalf("discard command returned %T, want DraftDeletedMsg", raw)
	}
	if len(backend.deleted) != 1 || backend.deleted[0].uid != 42 || backend.deleted[0].folder != "Drafts" {
		t.Fatalf("expected discard to delete saved draft 42/Drafts, got %#v", backend.deleted)
	}
}

func TestFunctionKeyF2SwitchesToContacts(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline

	model, _ := m.handleKeyMsg(functionKey(2))
	updated := model.(*Model)

	if updated.activeTab != tabContacts {
		t.Fatalf("F2 activeTab=%d, want Contacts", updated.activeTab)
	}
}

func TestRetiredCleanupBrowseShortcutsDoNotOpenManagers(t *testing.T) {
	for _, key := range []string{"W", "P", "C"} {
		t.Run(key, func(t *testing.T) {
			m := makeSizedModel(t, 140, 40)
			m.activeTab = tabTimeline

			model, _ := m.handleKeyMsg(keyRunes(key))
			updated := model.(*Model)

			if updated.showRuleEditor || updated.showPromptEditor || updated.showCleanupMgr {
				t.Fatalf("retired browse shortcut %s opened a manager/editor", key)
			}
			if !strings.Contains(updated.statusMessage, "Settings > Sync & Cleanup") {
				t.Fatalf("expected retired shortcut %s to point to Settings, got %q", key, updated.statusMessage)
			}
		})
	}
}

func TestTimelineLowercaseCOpensBlankComposeAndEscapeReturnsTimeline(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.loading = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timelineTable.SetCursor(1)
	m.setFocusedPanel(panelTimeline)
	m.composeTo.SetValue("stale@example.com")
	m.composeSubject.SetValue("stale subject")
	m.composeBody.SetValue("stale body")
	m.suggestions = []models.ContactData{{Email: "stale@example.com"}}
	m.suggestionIdx = 0
	m.attachmentInputActive = true
	m.attachmentPathInput.SetValue("/tmp/stale.txt")
	m.attachmentCompletions = []attachmentPathCandidate{{Display: "stale.txt", Value: "/tmp/stale.txt"}}
	m.attachmentCompletionVisible = true
	m.attachmentCompletionIdx = 0
	m.composeAIInput.SetValue("stale prompt")

	model, cmd := m.handleKeyMsg(keyRunes("c"))
	updated := model.(*Model)
	if cmd != nil {
		t.Fatalf("expected Timeline c to open blank Compose synchronously, got command %T", cmd)
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab=%d, want Compose", updated.activeTab)
	}
	if updated.composeTo.Value() != "" || updated.composeSubject.Value() != "" || updated.composeBody.Value() != "" {
		t.Fatalf("expected blank Compose fields, got to=%q subject=%q body=%q", updated.composeTo.Value(), updated.composeSubject.Value(), updated.composeBody.Value())
	}
	if len(updated.suggestions) != 0 || updated.suggestionIdx != -1 {
		t.Fatalf("expected blank Compose to clear autocomplete state, suggestions=%d idx=%d", len(updated.suggestions), updated.suggestionIdx)
	}
	if updated.attachmentInputActive || updated.attachmentPathInput.Value() != "" || len(updated.attachmentCompletions) != 0 {
		t.Fatalf("expected blank Compose to clear attachment prompt, active=%v path=%q completions=%d", updated.attachmentInputActive, updated.attachmentPathInput.Value(), len(updated.attachmentCompletions))
	}
	if updated.composeAIInput.Value() != "" {
		t.Fatalf("expected blank Compose to clear AI prompt, got %q", updated.composeAIInput.Value())
	}
	if updated.composeField != composeFieldTo {
		t.Fatalf("composeField=%d, want To field", updated.composeField)
	}

	model, cmd = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated = model.(*Model)
	if cmd != nil {
		t.Fatalf("expected Esc from empty Compose to be synchronous, got command %T", cmd)
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("Esc activeTab=%d, want Timeline", updated.activeTab)
	}
	if updated.timelineTable.Cursor() != 1 {
		t.Fatalf("timeline cursor=%d, want restored cursor 1", updated.timelineTable.Cursor())
	}
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("focusedPanel=%d, want Timeline panel", updated.focusedPanel)
	}
}

func TestComposeEscapeReturnsToTimelinePreviewOrigin(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.loading = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	email := m.timeline.emails[0]
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "preview body"}
	m.setFocusedPanel(panelPreview)

	model, _ := m.handleKeyMsg(keyRunes("c"))
	updated := model.(*Model)
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab=%d, want Compose", updated.activeTab)
	}

	model, cmd := updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated = model.(*Model)
	if cmd != nil {
		t.Fatalf("expected Esc from empty Compose to be synchronous, got command %T", cmd)
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("Esc activeTab=%d, want Timeline", updated.activeTab)
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != email.MessageID {
		t.Fatalf("expected preview email %q restored, got %#v", email.MessageID, updated.timeline.selectedEmail)
	}
	if updated.focusedPanel != panelPreview {
		t.Fatalf("focusedPanel=%d, want Preview panel", updated.focusedPanel)
	}
}

func TestComposeEscapeReturnsToTimelineSearchResultsOrigin(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.loading = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("meeting")
	m.timeline.searchResults = []*models.EmailData{m.timeline.emails[0]}
	m.timeline.searchResultsQuery = "meeting"
	m.timeline.searchFocus = timelineSearchFocusResults
	m.timeline.searchInput.Blur()
	m.timeline.emails = m.timeline.searchResults
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)

	model, _ := m.handleKeyMsg(keyRunes("c"))
	updated := model.(*Model)
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab=%d, want Compose", updated.activeTab)
	}

	model, cmd := updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated = model.(*Model)
	if cmd != nil {
		t.Fatalf("expected Esc from empty Compose to be synchronous, got command %T", cmd)
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("Esc activeTab=%d, want Timeline", updated.activeTab)
	}
	if !updated.timeline.searchMode || updated.timeline.searchFocus != timelineSearchFocusResults {
		t.Fatalf("expected Timeline search results origin, searchMode=%v focus=%d", updated.timeline.searchMode, updated.timeline.searchFocus)
	}
	if got := updated.timeline.searchInput.Value(); got != "meeting" {
		t.Fatalf("search query=%q, want meeting", got)
	}
}

func TestRenumberedTopLevelTabNavigationRoutesTimelineAndContacts(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.loading = false

	model, _ := m.handleKeyMsg(functionKey(2))
	updated := model.(*Model)
	if updated.activeTab != tabContacts {
		t.Fatalf("F2 activeTab=%d, want Contacts", updated.activeTab)
	}

	updated.activeTab = tabTimeline
	model, _ = updated.handleKeyMsg(functionKey(3))
	updated = model.(*Model)
	if updated.activeTab != tabContacts {
		t.Fatalf("F3 activeTab=%d, want Contacts", updated.activeTab)
	}

	updated.activeTab = tabTimeline
	model, _ = updated.handleKeyMsg(keyRunes("2"))
	updated = model.(*Model)
	if updated.activeTab != tabContacts {
		t.Fatalf("2 activeTab=%d, want Contacts", updated.activeTab)
	}

	updated.activeTab = tabTimeline
	model, _ = updated.handleKeyMsg(keyRunes("3"))
	updated = model.(*Model)
	if updated.activeTab != tabTimeline {
		t.Fatalf("3 activeTab=%d, want no top-level tab switch", updated.activeTab)
	}

	tabBar := stripANSI(updated.renderTabBar())
	for _, stale := range []string{"Cleanup", "Compose", "F4", "3  Contacts"} {
		if strings.Contains(tabBar, stale) {
			t.Fatalf("expected top tab bar to omit %q, got %q", stale, tabBar)
		}
	}
	for _, want := range []string{"1  Timeline", "2  Contacts"} {
		if !strings.Contains(tabBar, want) {
			t.Fatalf("expected tab bar to contain %q, got %q", want, tabBar)
		}
	}
}

func TestComposeAltPrintableKeysStayTextSafe(t *testing.T) {
	m := makeSizedModel(t, 180, 40)
	m.activeTab = tabCompose
	m.composeField = 4
	m.composeTo.Blur()
	m.composeBody.Focus()
	m.composeBody.SetValue("draft")
	m.showSidebar = false

	model, _ := m.handleKeyMsg(altKey('l'))
	m = model.(*Model)
	if m.showLogs {
		t.Fatal("alt+l should not open logs from compose")
	}
	if got := m.composeBody.Value(); !strings.HasPrefix(got, "draft") {
		t.Fatalf("alt+l changed draft unexpectedly, body=%q", got)
	}

	model, _ = m.handleKeyMsg(altKey('l'))
	m = model.(*Model)
	if m.showLogs {
		t.Fatal("alt+l should remain text-safe from compose")
	}

	model, _ = m.handleKeyMsg(altKey('c'))
	m = model.(*Model)
	if m.showChat {
		t.Fatal("alt+c should not open chat from compose")
	}
	if got := m.composeBody.Value(); !strings.HasPrefix(got, "draft") {
		t.Fatalf("alt+c changed draft unexpectedly, body=%q", got)
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.showChat {
		t.Fatal("chat should remain closed after alt+c")
	}

	model, _ = m.handleKeyMsg(altKey('f'))
	m = model.(*Model)
	if m.showSidebar {
		t.Fatal("alt+f should not toggle sidebar preference from compose")
	}
	if got := m.composeBody.Value(); !strings.HasPrefix(got, "draft") {
		t.Fatalf("alt+f changed draft unexpectedly, body=%q", got)
	}

	model, cmd := m.handleKeyMsg(altKey('r'))
	m = model.(*Model)
	if m.loading {
		t.Fatal("alt+r should not start refresh from compose")
	}
	if cmd != nil {
		t.Fatalf("expected alt+r to remain local to compose, got command %T", cmd)
	}
	if got := m.composeBody.Value(); !strings.HasPrefix(got, "draft") {
		t.Fatalf("alt+r changed draft unexpectedly, body=%q", got)
	}
}

func TestTimelineSearchPlainQIsTextAndCtrlCQuits(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.openTimelineSearch()

	model, cmd := m.handleKeyMsg(keyRunes("q"))
	m = model.(*Model)
	if commandIsQuit(cmd) {
		t.Fatal("plain q from timeline search returned quit command")
	}
	if got := m.timeline.searchInput.Value(); got != "q" {
		t.Fatalf("timeline search value=%q, want q", got)
	}

	_, cmd = m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !commandIsQuit(cmd) {
		t.Fatal("expected ctrl+c to quit from timeline search")
	}
}

func TestRangeFallbackKeysStayInsideTextEntrySurfaces(t *testing.T) {
	t.Run("compose body", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		m.activeTab = tabCompose
		m.composeField = composeFieldBody
		m.composeTo.Blur()
		m.composeBody.Focus()

		for _, key := range []string{"V", "j", "k"} {
			model, cmd := m.handleKeyMsg(keyRunes(key))
			if commandIsQuit(cmd) {
				t.Fatalf("plain %q from compose returned quit command", key)
			}
			m = model.(*Model)
		}

		if got := m.composeBody.Value(); got != "Vjk" {
			t.Fatalf("compose body value=%q, want Vjk", got)
		}
	})

	t.Run("timeline search", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		m.activeTab = tabTimeline
		m.openTimelineSearch()

		for _, key := range []string{"V", "j", "k"} {
			model, cmd := m.handleKeyMsg(keyRunes(key))
			if commandIsQuit(cmd) {
				t.Fatalf("plain %q from timeline search returned quit command", key)
			}
			m = model.(*Model)
		}

		if got := m.timeline.searchInput.Value(); got != "Vjk" {
			t.Fatalf("timeline search value=%q, want Vjk", got)
		}
		if m.timeline.rangeMode {
			t.Fatal("timeline search keys should not enter Timeline range mode")
		}
	})

	t.Run("prompt editor overlay", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		m.activeTab = tabTimeline
		m.timeline.emails = timelineRangeEmails()
		m.updateTimelineTable()
		m.showPromptEditor = true
		m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)

		for _, key := range []string{"V", "j", "k"} {
			model, _ := m.Update(keyRunes(key))
			m = model.(*Model)
			if m.timeline.rangeMode {
				t.Fatalf("plain %q from prompt editor entered Timeline range mode", key)
			}
		}
	})

	t.Run("rule editor overlay", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		m.activeTab = tabTimeline
		m.timeline.emails = timelineRangeEmails()
		m.updateTimelineTable()
		m.showRuleEditor = true
		m.ruleEditor = NewRuleEditor("alice@example.com", "", m.windowWidth, m.windowHeight)

		for _, key := range []string{"V", "j", "k"} {
			model, _ := m.Update(keyRunes(key))
			m = model.(*Model)
			if m.timeline.rangeMode {
				t.Fatalf("plain %q from rule editor entered Timeline range mode", key)
			}
		}
	})
}

func TestRenderKeyHints_FollowsNormalizedVisiblePanels(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.showSidebar = true
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1", Subject: "Hello"}
	m.timeline.bodyMessageID = "msg-1"
	m.timeline.body = &models.EmailBody{TextPlain: "body"}
	m.focusedPanel = panelSidebar

	m.updateTableDimensions(80, 24)
	hints := stripANSI(m.renderKeyHints())

	if strings.Contains(hints, "space: expand") {
		t.Fatalf("expected hidden sidebar hints to disappear, got %q", hints)
	}
	if !strings.Contains(hints, "Ctrl+R: reply") {
		t.Fatalf("expected timeline hints to stay on a visible panel after focus normalization, got %q", hints)
	}
}

func TestRenderKeyHints_AdvertisesFunctionKeysAsPrimaryTabSwitcher(t *testing.T) {
	tests := []struct {
		name     string
		model    func() *Model
		wantTabs bool
	}{
		{
			name:     "timeline list",
			wantTabs: false,
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabTimeline
				m.timeline.emails = mockEmails()
				m.updateTimelineTable()
				return m
			},
		},
		{
			name:     "timeline chat filter",
			wantTabs: true,
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabTimeline
				m.timeline.chatFilterMode = true
				m.timeline.emails = mockEmails()
				m.updateTimelineTable()
				return m
			},
		},
		{
			name:     "timeline read-only diagnostic",
			wantTabs: true,
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabTimeline
				m.currentFolder = virtualFolderAllMailOnly
				m.timeline.emails = mockEmails()
				m.updateTimelineTable()
				return m
			},
		},
		{
			name:     "compose",
			wantTabs: false,
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabCompose
				return m
			},
		},
		{
			name:     "contacts list",
			wantTabs: true,
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabContacts
				m.contactFocusPanel = 0
				return m
			},
		},
		{
			name:     "contacts detail",
			wantTabs: true,
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabContacts
				m.contactFocusPanel = 1
				return m
			},
		},
		{
			name:     "sidebar",
			wantTabs: true,
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabTimeline
				m.showSidebar = true
				m.setFocusedPanel(panelSidebar)
				return m
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hints := stripANSI(tc.model().renderKeyHints())
			if tc.wantTabs && !strings.Contains(hints, "1-2: tabs") {
				t.Fatalf("expected primary numbered tab hint, got %q", hints)
			}
			if !tc.wantTabs && strings.Contains(hints, "1-2: tabs") {
				t.Fatalf("expected calm Timeline list hint to omit tab switcher, got %q", hints)
			}
			for _, stale := range []string{"1-3: tabs", "F1-F4: tabs", "1/2/3/4: tabs", "alt+1/2/3/4: tabs", "Alt+1/2/3/4: tabs"} {
				if strings.Contains(hints, stale) {
					t.Fatalf("expected no stale tab hint %q, got %q", stale, hints)
				}
			}
		})
	}
}

func TestRenderKeyHints_TimelineListAdvertisesPanelSwitching(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	hints := stripANSI(m.renderKeyHints())
	if strings.Contains(hints, "tab/shift+tab: panels") {
		t.Fatalf("expected calm Default Timeline list hints to omit legacy panel aliases, got %q", hints)
	}
	help := m.timelineShortcutHelpSection()
	if !shortcutHelpSectionsContain([]shortcutHelpSection{help}, "F6 / Shift+F6", "switch visible panels") {
		t.Fatalf("expected shortcut help to advertise F6 panel switching, got %#v", help)
	}
}

func TestRenderTabBar_AdvertisesNumberKeys(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	rendered := stripANSI(m.renderTabBar())

	for _, want := range []string{"1  Timeline", "2  Contacts"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected tab bar to include %q, got %q", want, rendered)
		}
	}
	for _, stale := range []string{"2  Cleanup", "3  Contacts", "F1  Timeline", "F2  Compose", "F4"} {
		if strings.Contains(rendered, stale) {
			t.Fatalf("expected tab bar not to include stale label %q, got %q", stale, rendered)
		}
	}
}

func TestTimelinePreview_HidesSidebarWhileOpenWithoutChangingPreference(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.showSidebar = true
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	cmd := m.openTimelineEmail(m.timeline.emails[0])
	if cmd == nil {
		t.Fatal("expected opening timeline email to return body load command")
	}
	if !m.showSidebar {
		t.Fatal("expected sidebar preference to remain enabled")
	}
	if plan := m.buildLayoutPlan(120, 40); plan.SidebarVisible {
		t.Fatal("expected layout to auto-hide sidebar while timeline preview is open")
	}

	m.clearTimelinePreview()
	if !m.showSidebar {
		t.Fatal("expected sidebar preference to remain enabled after closing timeline preview")
	}
	if plan := m.buildLayoutPlan(120, 40); !plan.SidebarVisible {
		t.Fatal("expected sidebar to become visible again after closing timeline preview")
	}
}

func TestTabCyclesFocusWhileSyncingWithVisibleTimelineData(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = true
	m.showSidebar = true
	m.focusedPanel = panelSidebar
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.updateTableDimensions(120, 40)

	model, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	updated := model.(*Model)

	if updated.focusedPanel != panelTimeline {
		t.Fatalf("expected tab to move focus to timeline while syncing, got %d", updated.focusedPanel)
	}
}

func TestCtrlITreatedAsTabOutsideSearchMode(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = true
	m.showSidebar = true
	m.focusedPanel = panelTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.updateTableDimensions(120, 40)

	model, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'i', Mod: tea.ModCtrl})
	updated := model.(*Model)

	if updated.focusedPanel != panelSidebar {
		t.Fatalf("expected ctrl+i to cycle focus like tab, got %d", updated.focusedPanel)
	}
}

func TestNumberTabSwitchesToContactsWhileSyncingWithVisibleData(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = true
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, _, handled := m.handleTabKey(keyRune('2'))
	if !handled {
		t.Fatal("expected contacts tab key to be handled while syncing with visible data")
	}

	updated := model.(*Model)
	if updated.activeTab != tabContacts {
		t.Fatalf("expected active tab to switch to contacts, got %d", updated.activeTab)
	}
}

func TestFoldersLoadedMsg_PreservesExistingTreeWhenRefreshFails(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.folders = []string{"INBOX", "Sent", "Archive"}
	m.folderTree = buildFolderTree(m.folders)
	m.folderStatus = map[string]models.FolderStatus{
		"INBOX": {Unseen: 5, Total: 10},
	}

	model, _ := m.Update(FoldersLoadedMsg{Folders: nil})
	updated := model.(*Model)

	items := flattenTree(updated.folderTree)
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item.node.fullPath != "" {
			paths = append(paths, item.node.fullPath)
		}
	}

	joined := strings.Join(paths, ",")
	if !strings.Contains(joined, "Sent") || !strings.Contains(joined, "Archive") {
		t.Fatalf("expected folder refresh failure to preserve existing tree, got %v", paths)
	}
}

func TestLogOverlayToggleWorksWhileSyncingWithVisibleData(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = true
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, _ := m.handleKeyMsg(keyRune('L'))
	updated := model.(*Model)

	if !updated.showLogs {
		t.Fatal("expected L to toggle log overlay while syncing with visible data")
	}
}

func TestChatToggle_ShowsFallbackWhenTooNarrow(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.loading = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, _ := m.handleKeyMsg(keyRune('g'))
	updated := model.(*Model)

	if updated.showChat {
		t.Fatal("expected chat to remain closed when the terminal is too narrow")
	}
	if !strings.Contains(updated.statusMessage, "Chat") || !strings.Contains(updated.statusMessage, "widen") {
		t.Fatalf("expected a visible narrow-width fallback message, got %q", updated.statusMessage)
	}
}
