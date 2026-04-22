package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
