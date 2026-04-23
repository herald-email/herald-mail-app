package app

import (
	"strings"
	"testing"
	"time"

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

func TestCleanupAllMailOnly_ShowsSelectionCheckmark(t *testing.T) {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	b := &stubBackend{
		virtualFolderResult: &models.VirtualFolderResult{
			Name:      "All Mail only",
			Supported: true,
			Reason:    "Read-only: messages in All Mail with no other folder assignment.",
			Emails: []*models.EmailData{
				{MessageID: "<a@x.com>", Sender: "Alpha <alpha@example.com>", Subject: "first", Date: now.Add(-2 * time.Hour), Folder: "All Mail"},
				{MessageID: "<b@x.com>", Sender: "Alpha <alpha@example.com>", Subject: "second", Date: now.Add(-1 * time.Hour), Folder: "All Mail"},
				{MessageID: "<c@x.com>", Sender: "Beta <beta@example.com>", Subject: "third", Date: now.Add(-30 * time.Minute), Folder: "All Mail"},
			},
		},
	}

	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.currentFolder = virtualFolderAllMailOnly
	m.loading = true

	updated, _ = m.Update(m.loadTimelineEmails()())
	m = updated.(*Model)

	if got := len(m.summaryTable.Rows()); got == 0 {
		t.Fatalf("expected Cleanup All Mail only view to hydrate summary rows, got %d", got)
	}
	selectedKey, ok := m.summaryKeyAtCursor()
	if !ok {
		t.Fatal("expected Cleanup All Mail only view to expose a summary key at the cursor")
	}

	m.toggleSelection()

	if !m.selectedSummaryKeys[selectedKey] {
		t.Fatalf("expected %q to be selected in Cleanup All Mail only view", selectedKey)
	}

	foundCheckmark := false
	for rowIdx, key := range m.rowToSender {
		if key != selectedKey {
			continue
		}
		row := m.summaryTable.Rows()[rowIdx]
		foundCheckmark = len(row) > 0 && row[0] == "✓"
		break
	}
	if !foundCheckmark {
		t.Fatalf("expected selected Cleanup All Mail only row %q to render a checkmark", selectedKey)
	}

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "1 sender selected") {
		t.Fatalf("expected Cleanup All Mail only status to reflect the selected row, got %q", status)
	}
}

func TestCleanupAllMailOnly_SidebarSwitchClearsStaleSummaryRows(t *testing.T) {
	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.currentFolder = "INBOX"
	m.stats = makeCleanupStats()
	m.updateSummaryTable()
	m.updateDetailsTable()

	if got := len(m.summaryTable.Rows()); got == 0 {
		t.Fatal("expected baseline Cleanup summary rows before switching folders")
	}

	m.folderTree = buildFolderTree([]string{"INBOX", "Archive"})
	items := flattenTree(m.folderTree)
	for i, item := range items {
		if item.node != nil && item.node.fullPath == virtualFolderAllMailOnly {
			m.sidebarCursor = i
			break
		}
	}
	m.setFocusedPanel(panelSidebar)
	m.selectSidebarFolder()

	if got := len(m.summaryTable.Rows()); got != 0 {
		t.Fatalf("expected switching to All Mail only to clear stale Cleanup summary rows immediately, got %d rows", got)
	}
	if got := len(m.detailsTable.Rows()); got != 0 {
		t.Fatalf("expected switching to All Mail only to clear stale Cleanup detail rows immediately, got %d rows", got)
	}
	if _, ok := m.summaryKeyAtCursor(); ok {
		t.Fatal("expected no selectable stale Cleanup summary key after switching to All Mail only")
	}
}

func TestCleanupAllMailOnly_BlocksDeleteAndArchiveConfirmations(t *testing.T) {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCleanup
	m.currentFolder = virtualFolderAllMailOnly
	m.groupByDomain = true
	m.timeline.emails = []*models.EmailData{
		{MessageID: "<a@x.com>", Sender: "Alpha <alpha@example.com>", Subject: "first", Date: now.Add(-2 * time.Hour), Folder: "All Mail"},
		{MessageID: "<b@x.com>", Sender: "Alpha <alpha@example.com>", Subject: "second", Date: now.Add(-1 * time.Hour), Folder: "All Mail"},
	}
	m.hydrateCleanupFromVirtualFolderEmails(m.timeline.emails)
	m.setFocusedPanel(panelSummary)

	for _, key := range []string{"D", "e"} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = updated.(*Model)
		if m.pendingDeleteConfirm {
			t.Fatalf("expected %q to be blocked in Cleanup All Mail only view", key)
		}
	}

	if m.statusMessage == "" {
		t.Fatal("expected read-only Cleanup action to set a visible status message")
	}
	if !strings.Contains(strings.ToLower(m.statusMessage), "read-only") {
		t.Fatalf("expected read-only status message, got %q", m.statusMessage)
	}
	if m.deletionsPending != 0 || m.deletionsTotal != 0 {
		t.Fatalf("expected no queued deletions, got pending=%d total=%d", m.deletionsPending, m.deletionsTotal)
	}
}

func TestCleanupAllMailOnlyHints_AdvertiseReadOnly(t *testing.T) {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCleanup
	m.currentFolder = virtualFolderAllMailOnly
	m.timeline.emails = []*models.EmailData{
		{MessageID: "<a@x.com>", Sender: "Alpha <alpha@example.com>", Subject: "first", Date: now.Add(-2 * time.Hour), Folder: "All Mail"},
	}
	m.hydrateCleanupFromVirtualFolderEmails(m.timeline.emails)
	m.setFocusedPanel(panelSummary)

	summaryHints := stripANSI(m.renderKeyHints())
	if strings.Contains(summaryHints, "D: delete") || strings.Contains(summaryHints, "e: archive") {
		t.Fatalf("expected Cleanup summary hints to omit mutating actions in read-only view, got %q", summaryHints)
	}
	if !strings.Contains(summaryHints, "read-only") {
		t.Fatalf("expected Cleanup summary hints to advertise read-only context, got %q", summaryHints)
	}

	m.showCleanupPreview = true
	m.cleanupPreviewWidth = 48
	m.cleanupPreviewEmail = m.timeline.emails[0]
	m.cleanupEmailBody = &models.EmailBody{TextPlain: "hello"}
	m.detailsEmails = []*models.EmailData{m.timeline.emails[0]}
	m.setFocusedPanel(panelDetails)

	previewHints := stripANSI(m.renderKeyHints())
	if strings.Contains(previewHints, "D: delete") || strings.Contains(previewHints, "e: archive") {
		t.Fatalf("expected Cleanup preview hints to omit mutating actions in read-only view, got %q", previewHints)
	}
	if !strings.Contains(previewHints, "read-only") {
		t.Fatalf("expected Cleanup preview hints to advertise read-only context, got %q", previewHints)
	}

	preview := stripANSI(m.renderCleanupPreview())
	if strings.Contains(preview, "D: delete") || strings.Contains(preview, "e: archive") {
		t.Fatalf("expected Cleanup preview panel to omit mutating actions in read-only view, got:\n%s", preview)
	}
	if !strings.Contains(preview, "read-only") {
		t.Fatalf("expected Cleanup preview panel to advertise read-only context, got:\n%s", preview)
	}
}
