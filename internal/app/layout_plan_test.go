package app

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestBuildLayoutPlan_TimelineShowsSidebarAndChatWhenRoomAllows(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.showSidebar = true
	m.showChat = true

	plan := m.buildLayoutPlan(220, 50)

	if !plan.SidebarVisible {
		t.Fatal("expected sidebar to remain visible at 220x50")
	}
	if !plan.ChatVisible {
		t.Fatal("expected chat to remain visible at 220x50")
	}
	if plan.Timeline.TableWidth <= 0 {
		t.Fatalf("expected positive timeline table width, got %d", plan.Timeline.TableWidth)
	}
}

func TestBuildLayoutPlan_HidesSidebarWhenTooNarrow(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.showSidebar = true

	plan := m.buildLayoutPlan(80, 24)

	if plan.SidebarVisible {
		t.Fatal("expected sidebar to auto-hide at 80x24")
	}
}

func TestBuildLayoutPlan_HidesSidebarWhenTimelinePreviewIsOpen(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.showSidebar = true
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1"}

	plan := m.buildLayoutPlan(220, 50)

	if plan.SidebarVisible {
		t.Fatal("expected sidebar to auto-hide while timeline preview is open")
	}
}

func TestUpdateTableDimensions_NormalizesHiddenFocus(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.showSidebar = true
	m.focusedPanel = panelSidebar

	m.updateTableDimensions(80, 24)

	if m.focusedPanel != panelTimeline {
		t.Fatalf("expected focus to move to timeline when sidebar is hidden, got %d", m.focusedPanel)
	}
}

func TestUpdateTableDimensions_TimelinePreviewDoesNotMarkSidebarTooWide(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.showSidebar = true
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1"}

	m.updateTableDimensions(220, 50)

	if m.sidebarTooWide {
		t.Fatal("expected timeline preview to hide the sidebar without raising a too-narrow warning")
	}
}
