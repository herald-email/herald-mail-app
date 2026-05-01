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

func TestComposeAltTabSwitchesAndPersistsDraft(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeTo.SetValue("alice@example.com")
	m.composeBody.SetValue("draft body")

	model, cmd := m.handleKeyMsg(altKey('1'))
	updated := model.(*Model)

	if updated.activeTab != tabTimeline {
		t.Fatalf("alt+1 activeTab=%d, want Timeline", updated.activeTab)
	}
	if cmd == nil {
		t.Fatal("expected alt+1 leaving non-empty compose to produce draft persistence command")
	}
	if !updated.draftSaving {
		t.Fatal("expected alt+1 leaving non-empty compose to mark draftSaving")
	}
}

func TestComposeFunctionKeysSwitchTabsAndDoNotTypeIntoDraft(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{name: "F1", key: functionKey(1), want: tabTimeline},
		{name: "F2", key: functionKey(2), want: tabCleanup},
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

func TestFunctionKeyF2SwitchesToCleanup(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline

	model, _ := m.handleKeyMsg(functionKey(2))
	updated := model.(*Model)

	if updated.activeTab != tabCleanup {
		t.Fatalf("F2 activeTab=%d, want Cleanup", updated.activeTab)
	}
}

func TestTimelineCOpensBlankComposeAndEscapeReturnsTimeline(t *testing.T) {
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

	model, cmd := m.handleKeyMsg(keyRunes("C"))
	updated := model.(*Model)
	if cmd != nil {
		t.Fatalf("expected Timeline C to open blank Compose synchronously, got command %T", cmd)
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

	model, _ := m.handleKeyMsg(keyRunes("C"))
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

	model, _ := m.handleKeyMsg(keyRunes("C"))
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

func TestRenumberedTopLevelTabNavigationRoutesCleanupAndContacts(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.loading = false

	model, _ := m.handleKeyMsg(functionKey(2))
	updated := model.(*Model)
	if updated.activeTab != tabCleanup {
		t.Fatalf("F2 activeTab=%d, want Cleanup", updated.activeTab)
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
	if updated.activeTab != tabCleanup {
		t.Fatalf("2 activeTab=%d, want Cleanup", updated.activeTab)
	}

	updated.activeTab = tabTimeline
	model, _ = updated.handleKeyMsg(keyRunes("3"))
	updated = model.(*Model)
	if updated.activeTab != tabContacts {
		t.Fatalf("3 activeTab=%d, want Contacts", updated.activeTab)
	}

	tabBar := stripANSI(updated.renderTabBar())
	if strings.Contains(tabBar, "Compose") || strings.Contains(tabBar, "F4") {
		t.Fatalf("expected top tab bar without Compose/F4, got %q", tabBar)
	}
	for _, want := range []string{"F1  Timeline", "F2  Cleanup", "F3  Contacts"} {
		if !strings.Contains(tabBar, want) {
			t.Fatalf("expected tab bar to contain %q, got %q", want, tabBar)
		}
	}
}

func TestCleanupFullScreenPlainTabSwitchClosesPreview(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabCleanup
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewEmail = &models.EmailData{MessageID: "cleanup-a", Subject: "Cleanup A"}
	m.cleanupEmailBody = &models.EmailBody{TextPlain: "cleanup body"}
	m.cleanupPreviewHadSidebar = true
	m.showSidebar = false
	m.cleanupPreviewDocLayout = &previewDocumentLayout{TotalRows: 1}

	model, _ := m.handleKeyMsg(keyRunes("1"))
	updated := model.(*Model)

	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab=%d, want Timeline", updated.activeTab)
	}
	if updated.cleanupFullScreen {
		t.Fatal("cleanupFullScreen should be false after switching tabs")
	}
	if updated.showCleanupPreview || updated.cleanupPreviewEmail != nil || updated.cleanupEmailBody != nil {
		t.Fatal("cleanup preview should be closed when switching away")
	}
	if !updated.showSidebar {
		t.Fatal("sidebar should be restored from cleanup preview state")
	}
	if updated.cleanupPreviewDocLayout != nil {
		t.Fatal("cleanup document cache should be cleared")
	}

	model, _ = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	afterEsc := model.(*Model)
	if afterEsc.activeTab != tabTimeline {
		t.Fatalf("Esc after tab switch should not be trapped, activeTab=%d", afterEsc.activeTab)
	}
}

func TestCleanupFullScreenAltTabSwitchClosesPreview(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabCleanup
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewEmail = &models.EmailData{MessageID: "cleanup-a", Subject: "Cleanup A"}
	m.cleanupEmailBody = &models.EmailBody{TextPlain: "cleanup body"}
	m.cleanupPreviewHadSidebar = true
	m.showSidebar = false

	model, _ := m.handleKeyMsg(altKey('1'))
	updated := model.(*Model)

	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab=%d, want Timeline", updated.activeTab)
	}
	if updated.cleanupFullScreen {
		t.Fatal("cleanupFullScreen should be false after alt tab switch")
	}
	if updated.showCleanupPreview || updated.cleanupPreviewEmail != nil || updated.cleanupEmailBody != nil {
		t.Fatal("cleanup preview should be closed after alt tab switch")
	}
	if !updated.showSidebar {
		t.Fatal("sidebar should be restored from cleanup preview state")
	}
}

func TestComposeAltGlobalCommandsDoNotTypeIntoDraft(t *testing.T) {
	m := makeSizedModel(t, 180, 40)
	m.activeTab = tabCompose
	m.composeField = 4
	m.composeTo.Blur()
	m.composeBody.Focus()
	m.composeBody.SetValue("draft")
	m.showSidebar = false

	model, _ := m.handleKeyMsg(altKey('l'))
	m = model.(*Model)
	if !m.showLogs {
		t.Fatal("expected alt+l to open logs from compose")
	}
	if got := m.composeBody.Value(); got != "draft" {
		t.Fatalf("alt+l typed into draft, body=%q", got)
	}

	model, _ = m.handleKeyMsg(altKey('l'))
	m = model.(*Model)
	if m.showLogs {
		t.Fatal("expected second alt+l to close logs from compose")
	}

	model, _ = m.handleKeyMsg(altKey('c'))
	m = model.(*Model)
	if !m.showChat {
		t.Fatal("expected alt+c to open chat from compose")
	}
	if got := m.composeBody.Value(); got != "draft" {
		t.Fatalf("alt+c typed into draft, body=%q", got)
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.showChat {
		t.Fatal("expected esc to close chat after alt+c")
	}

	model, _ = m.handleKeyMsg(altKey('f'))
	m = model.(*Model)
	if !m.showSidebar {
		t.Fatal("expected alt+f to toggle sidebar preference from compose")
	}
	if got := m.composeBody.Value(); got != "draft" {
		t.Fatalf("alt+f typed into draft, body=%q", got)
	}

	model, cmd := m.handleKeyMsg(altKey('r'))
	m = model.(*Model)
	if !m.loading {
		t.Fatal("expected alt+r to start refresh from compose")
	}
	if cmd == nil {
		t.Fatal("expected alt+r refresh to return a command")
	}
	if got := m.composeBody.Value(); got != "draft" {
		t.Fatalf("alt+r typed into draft, body=%q", got)
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

func TestRenderKeyHints_FollowsNormalizedVisiblePanels(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.loading = false
	m.showSidebar = true
	m.showCleanupPreview = true
	m.focusedPanel = panelSidebar

	m.updateTableDimensions(80, 24)
	hints := stripANSI(m.renderKeyHints())

	if strings.Contains(hints, "space: expand") {
		t.Fatalf("expected hidden sidebar hints to disappear, got %q", hints)
	}
	if !strings.Contains(hints, "scroll preview") {
		t.Fatalf("expected cleanup preview hints after focus normalization, got %q", hints)
	}
}

func TestRenderKeyHints_AdvertisesFunctionKeysAsPrimaryTabSwitcher(t *testing.T) {
	tests := []struct {
		name  string
		model func() *Model
	}{
		{
			name: "timeline list",
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabTimeline
				m.timeline.emails = mockEmails()
				m.updateTimelineTable()
				return m
			},
		},
		{
			name: "timeline chat filter",
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
			name: "timeline read-only diagnostic",
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
			name: "compose",
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabCompose
				return m
			},
		},
		{
			name: "contacts list",
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabContacts
				m.contactFocusPanel = 0
				return m
			},
		},
		{
			name: "contacts detail",
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabContacts
				m.contactFocusPanel = 1
				return m
			},
		},
		{
			name: "cleanup summary",
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabCleanup
				m.stats = makeCleanupStats()
				m.updateSummaryTable()
				return m
			},
		},
		{
			name: "cleanup details",
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabCleanup
				m.stats = makeCleanupStats()
				m.updateSummaryTable()
				m.setFocusedPanel(panelDetails)
				return m
			},
		},
		{
			name: "sidebar",
			model: func() *Model {
				m := makeSizedModel(t, 120, 40)
				m.activeTab = tabCleanup
				m.showSidebar = true
				m.setFocusedPanel(panelSidebar)
				return m
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hints := stripANSI(tc.model().renderKeyHints())
			if !strings.Contains(hints, "F1-F3: tabs") {
				t.Fatalf("expected primary F-key tab hint, got %q", hints)
			}
			for _, stale := range []string{"F1-F4: tabs", "1/2/3/4: tabs", "alt+1/2/3/4: tabs", "Alt+1/2/3/4: tabs"} {
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
	if !strings.Contains(hints, "tab/shift+tab: panels") {
		t.Fatalf("expected Timeline list hints to advertise panel switching, got %q", hints)
	}
}

func TestRenderTabBar_AdvertisesFunctionKeys(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	rendered := stripANSI(m.renderTabBar())

	for _, want := range []string{"F1  Timeline", "F2  Cleanup", "F3  Contacts"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected tab bar to include %q, got %q", want, rendered)
		}
	}
	for _, stale := range []string{"F2  Compose", "F4", "  1  Timeline", "  2  Cleanup", "  3  Contacts"} {
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

func TestNumberTabSwitchesToCleanupWhileSyncingWithVisibleData(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = true
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, _, handled := m.handleTabKey(keyRune('2'))
	if !handled {
		t.Fatal("expected cleanup tab key to be handled while syncing with visible data")
	}

	updated := model.(*Model)
	if updated.activeTab != tabCleanup {
		t.Fatalf("expected active tab to switch to cleanup, got %d", updated.activeTab)
	}
}

func TestSelectSidebarFolder_CleanupRestoresSummaryTableFocus(t *testing.T) {
	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.loading = false
	m.currentFolder = "INBOX"
	m.showSidebar = true
	m.stats = makeCleanupStats()
	m.folderTree = buildFolderTree([]string{"INBOX", "Archive"})
	m.updateSummaryTable()
	m.updateDetailsTable()
	m.setFocusedPanel(panelSidebar)
	m.sidebarCursor = 1

	m.selectSidebarFolder()

	if m.focusedPanel != panelSummary {
		t.Fatalf("expected focus to return to cleanup summary, got %d", m.focusedPanel)
	}
	if !m.summaryTable.Focused() {
		t.Fatal("expected cleanup summary table to be focused after selecting a folder from the sidebar")
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

	model, _ := m.handleKeyMsg(keyRune('l'))
	updated := model.(*Model)

	if !updated.showLogs {
		t.Fatal("expected l to toggle log overlay while syncing with visible data")
	}
}

func TestChatToggle_ShowsFallbackWhenTooNarrow(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.loading = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, _ := m.handleKeyMsg(keyRune('c'))
	updated := model.(*Model)

	if updated.showChat {
		t.Fatal("expected chat to remain closed when the terminal is too narrow")
	}
	if !strings.Contains(updated.statusMessage, "Chat") || !strings.Contains(updated.statusMessage, "widen") {
		t.Fatalf("expected a visible narrow-width fallback message, got %q", updated.statusMessage)
	}
}
