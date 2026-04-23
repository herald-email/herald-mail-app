package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

func TestHandleOverlayKey_ChatEscapeRestoresTimelineFocus(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.showChat = true
	m.focusedPanel = panelChat
	m.windowWidth = 120
	m.windowHeight = 40

	model, _, handled := m.handleOverlayKey(tea.KeyMsg{Type: tea.KeyEsc})
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

	model, cmd, handled := m.handleTabKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
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

	model, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyTab})
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

	model, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlI})
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

	model, _, handled := m.handleTabKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if !handled {
		t.Fatal("expected cleanup tab key to be handled while syncing with visible data")
	}

	updated := model.(*Model)
	if updated.activeTab != tabCleanup {
		t.Fatalf("expected active tab to switch to cleanup, got %d", updated.activeTab)
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

	model, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
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

	model, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	updated := model.(*Model)

	if updated.showChat {
		t.Fatal("expected chat to remain closed when the terminal is too narrow")
	}
	if !strings.Contains(updated.statusMessage, "Chat") || !strings.Contains(updated.statusMessage, "widen") {
		t.Fatalf("expected a visible narrow-width fallback message, got %q", updated.statusMessage)
	}
}
