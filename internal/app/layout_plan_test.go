package app

import (
	"testing"

	"mail-processor/internal/models"
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

func TestBuildLayoutPlan_CleanupPreviewCollapsesSummaryAt80Cols(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.showCleanupPreview = true

	plan := m.buildLayoutPlan(80, 24)

	if plan.Cleanup.SummaryWidth != 0 {
		t.Fatalf("expected summary column to collapse at 80x24, got %d", plan.Cleanup.SummaryWidth)
	}
	if plan.Cleanup.DetailsWidth < 24 {
		t.Fatalf("expected usable details width, got %d", plan.Cleanup.DetailsWidth)
	}
	if plan.Cleanup.PreviewWidth < 20 {
		t.Fatalf("expected usable preview width, got %d", plan.Cleanup.PreviewWidth)
	}
}

func TestUpdateTableDimensions_NormalizesHiddenFocus(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.showSidebar = true
	m.showCleanupPreview = true
	m.focusedPanel = panelSidebar

	m.updateTableDimensions(80, 24)

	if m.focusedPanel != panelDetails {
		t.Fatalf("expected focus to move to details when sidebar/summary are hidden, got %d", m.focusedPanel)
	}
}
