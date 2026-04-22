package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

func TestLoadTimelineEmails_AllMailOnlyUsesVirtualView(t *testing.T) {
	b := &stubBackend{
		virtualFolderResult: &models.VirtualFolderResult{
			Name:      "All Mail only",
			Supported: true,
			Emails: []*models.EmailData{
				{MessageID: "<a@x.com>", Sender: "a@x.com", Subject: "only in all mail", Folder: "All Mail"},
			},
		},
	}
	m := New(b, nil, "", nil, false)
	m.currentFolder = virtualFolderAllMailOnly

	msg := m.loadTimelineEmails()().(TimelineLoadedMsg)
	if len(msg.Emails) != 1 {
		t.Fatalf("expected 1 virtual-folder email, got %d", len(msg.Emails))
	}
	if !msg.ReadOnly {
		t.Fatalf("expected virtual folder load to be read-only")
	}
}

func TestTimelineHints_AllMailOnlyReadOnly(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = []*models.EmailData{
		{MessageID: "<a@x.com>", Sender: "a@x.com", Subject: "only in all mail", Folder: "All Mail"},
	}
	m.updateTimelineTable()

	hints, ok := m.timelineKeyHints(m.chromeState(m.buildLayoutPlan(m.windowWidth, m.windowHeight)))
	if !ok {
		t.Fatal("expected timeline hints")
	}
	for _, forbidden := range []string{"D: delete", "e: archive", "R: reply", "F: forward", "A: re-classify", "ctrl+q"} {
		if strings.Contains(hints, forbidden) {
			t.Fatalf("expected read-only hints to omit %q, got: %s", forbidden, hints)
		}
	}
	if !strings.Contains(hints, "read-only") {
		t.Fatalf("expected read-only hint context, got: %s", hints)
	}
}

func TestRenderTimelineView_AllMailOnlyUnsupportedNotice(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = []*models.EmailData{}
	m.timeline.virtualNotice = "Provider does not expose All Mail"
	m.updateTimelineTable()

	view := m.renderTimelineView()
	if !strings.Contains(view, "Provider does not expose All Mail") {
		t.Fatalf("expected unsupported notice in timeline view, got:\n%s", view)
	}
}

func TestAllMailOnlyIgnoresMutatingKeys(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = []*models.EmailData{
		{MessageID: "<a@x.com>", Sender: "a@x.com", Subject: "only in all mail", Folder: "All Mail"},
	}
	m.updateTimelineTable()

	for _, key := range []string{"D", "e", "A", "R", "F", "*", "u"} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = updated.(*Model)
	}

	if m.pendingDeleteConfirm {
		t.Fatal("expected no pending destructive confirmation in read-only inspector")
	}
	if m.activeTab != tabTimeline {
		t.Fatalf("expected to remain on timeline tab, got %d", m.activeTab)
	}
}
