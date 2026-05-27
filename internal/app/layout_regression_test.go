package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type layoutBackend struct {
	stubBackend
	timelineEmails []*models.EmailData
}

func (b *layoutBackend) GetTimelineEmails(_ string) ([]*models.EmailData, error) {
	return b.timelineEmails, nil
}

func makeSizedModel(tb testing.TB, width, height int) *Model {
	tb.Helper()
	b := &layoutBackend{}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = updated.(*Model)
	m.loading = false
	m.currentFolder = "INBOX"
	m.folderStatus = map[string]models.FolderStatus{
		"INBOX": {Unseen: 12, Total: 38},
	}
	return m
}

func assertFitsWidth(t *testing.T, width int, rendered string) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if v := visibleWidth(line); v > width {
			t.Fatalf("line %d visible width=%d exceeds width %d:\n%s", i, v, width, rendered)
		}
		if !utf8.ValidString(line) {
			t.Fatalf("line %d is not valid UTF-8: %q", i, line)
		}
		if strings.Contains(line, "���") {
			t.Fatalf("line %d contains replacement glyphs: %q", i, line)
		}
	}
}

func assertFitsHeight(t *testing.T, height int, rendered string) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	if len(lines) > height {
		t.Fatalf("rendered height=%d exceeds height %d:\n%s", len(lines), height, rendered)
	}
}

func TestStatusChrome_UTF8SafeAt80Cols(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	status := m.renderStatusBar()
	hints := m.renderKeyHints()
	assertFitsWidth(t, 80, status)
	assertFitsWidth(t, 80, hints)
}

func TestKeyHints_WrapToTwoLinesWhenNeeded(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{
		TextPlain: strings.Repeat("preview ", 30),
		Attachments: []models.Attachment{
			{Filename: "one.pdf"},
			{Filename: "two.pdf"},
		},
	}
	m.focusedPanel = panelPreview

	hints := m.renderKeyHints()
	assertFitsWidth(t, 80, hints)

	lines := strings.Split(stripANSI(hints), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected wrapped key hints to use two lines, got %d:\n%s", len(lines), stripANSI(hints))
	}
}

func TestMinimumSizeMessage_FitsWidthAndIncludesTarget(t *testing.T) {
	m := makeSizedModel(t, 50, 15)
	rendered := m.View().Content
	assertFitsWidth(t, 50, rendered)

	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, "60") || !strings.Contains(strings.ToLower(stripped), "resize") {
		t.Fatalf("minimum-size message should include a concrete resize target, got %q", stripped)
	}
}

func TestMainView_TitleBarSpansTerminalWidth(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	lines := strings.Split(m.renderMainView(), "\n")
	if len(lines) == 0 {
		t.Fatal("expected rendered chrome")
	}
	title := lines[0]
	if got := visibleWidth(title); got != 80 {
		t.Fatalf("expected title bar to span 80 columns, got %d: %q", got, stripANSI(title))
	}
	if !strings.Contains(stripANSI(title), "Herald") {
		t.Fatalf("expected title bar to contain Herald, got %q", stripANSI(title))
	}
}

func TestMainView_TitleRowIncludesTabsWithoutSeparateTabLine(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	lines := strings.Split(stripANSI(m.renderMainView()), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected rendered chrome and content, got %d lines", len(lines))
	}
	title := lines[0]
	for _, want := range []string{"Herald", "1  Timeline", "2  Contacts"} {
		if !strings.Contains(title, want) {
			t.Fatalf("expected title row to contain %q, got %q", want, title)
		}
	}
	for _, stale := range []string{"1  Timeline", "2  Cleanup", "3  Contacts"} {
		if strings.Contains(lines[1], stale) {
			t.Fatalf("expected no separate tab line below title row, got %q", lines[1])
		}
	}
	for _, stale := range []string{"2  Cleanup", "3  Contacts"} {
		if strings.Contains(title, stale) {
			t.Fatalf("expected retired tab label %q to be absent, got %q", stale, title)
		}
	}
	if strings.TrimSpace(lines[1]) == "" {
		t.Fatalf("expected content to start immediately after title row, got blank line: %#v", lines[:3])
	}
}

func TestChromeHeightBudget_MainViewFills80x24(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.updateTableDimensions(80, 24)

	rendered := m.renderMainView()
	lines := strings.Split(stripANSI(rendered), "\n")
	if len(lines) != 24 {
		t.Fatalf("expected main view to fill 80x24 exactly, got %d lines:\n%s", len(lines), stripANSI(rendered))
	}
	assertFitsWidth(t, 80, rendered)
}

func TestBottomChromeDividerSeparatesStatusAndKeyHints(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.statusMessage = "Settings saved."

	rendered := m.renderMainView()
	lines := strings.Split(stripANSI(rendered), "\n")
	if len(lines) != 24 {
		t.Fatalf("expected main view to fill 80x24 exactly, got %d lines:\n%s", len(lines), stripANSI(rendered))
	}

	dividerRow := -1
	for i, line := range lines {
		if line == strings.Repeat("─", 80) {
			dividerRow = i
			break
		}
	}
	if dividerRow < 0 {
		t.Fatalf("expected full-width line divider between status and key hints, got:\n%s", stripANSI(rendered))
	}
	if dividerRow == 0 || dividerRow >= len(lines)-1 {
		t.Fatalf("divider row should sit between status and key hints, row=%d total=%d:\n%s", dividerRow, len(lines), stripANSI(rendered))
	}
	if !strings.Contains(lines[dividerRow-1], "Settings saved.") {
		t.Fatalf("expected status row immediately above divider, got %q", lines[dividerRow-1])
	}
	if !strings.Contains(lines[dividerRow+1], "?: help") {
		t.Fatalf("expected key hints immediately below divider, got %q", lines[dividerRow+1])
	}
	assertFitsWidth(t, 80, rendered)
}

func TestComposeView_Fits80x24(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabCompose
	rendered := m.renderMainView()
	assertFitsWidth(t, 80, rendered)

	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, "To:") || !strings.Contains(stripped, "┐") {
		t.Fatalf("compose view should render closed field borders, got:\n%s", stripped)
	}
}

func TestPromptEditor_Fits80x24(t *testing.T) {
	editor := NewPromptEditor(nil, 80, 24)

	rendered := editor.View().Content
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)

	stripped := stripANSI(rendered)
	for _, want := range []string{
		"New Custom Prompt",
		"Saving does not run the prompt",
		"Example:",
		"Settings > Sync & Cleanup",
	} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("expected prompt editor to contain %q, got:\n%s", want, stripped)
		}
	}
}

func TestPromptEditor_MinimumSizeGuardAt50x15(t *testing.T) {
	editor := NewPromptEditor(nil, 50, 15)

	rendered := editor.View().Content
	assertFitsWidth(t, 50, rendered)

	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, "Terminal too narrow") {
		t.Fatalf("expected prompt editor to use minimum-size guard, got:\n%s", stripped)
	}
	if strings.Contains(stripped, "New Custom Prompt") {
		t.Fatalf("expected guard to replace clipped prompt editor, got:\n%s", stripped)
	}
}

func TestRuleEditor_Fits80x24(t *testing.T) {
	editor := NewRuleEditor("ShopifyBrand <orders@shopify-brand.example>", "", 80, 24)

	rendered := editor.View().Content
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)
}

func TestContactsInlinePreview_NoFooterBorderLeakAt80x24(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabContacts
	m.contactFocusPanel = 1
	contact := models.ContactData{
		Email:       "newsletter@techweekly.example",
		DisplayName: "Tech Weekly",
		EmailCount:  8,
	}
	m.contactsFiltered = []models.ContactData{contact}
	m.contactDetail = &contact
	m.contactDetailEmails = []*models.EmailData{
		{
			MessageID: "msg-1",
			Sender:    "Tech Weekly <newsletter@techweekly.example>",
			Subject:   "This Week in Tech #1",
			Date:      time.Date(2026, 4, 20, 18, 38, 0, 0, time.UTC),
			Folder:    "INBOX",
		},
	}
	m.contactPreviewEmail = m.contactDetailEmails[0]
	m.contactPreviewBody = &models.EmailBody{TextPlain: strings.Repeat("Lorem ipsum ", 20)}
	rendered := m.renderMainView()
	assertFitsWidth(t, 80, rendered)

	lines := strings.Split(stripANSI(rendered), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected footer lines in rendered output")
	}
	for _, line := range lines[len(lines)-2:] {
		if strings.ContainsAny(line, "┐└┘") {
			t.Fatalf("footer should not contain leaked border glyphs, got %q", line)
		}
	}
}

func TestTimelinePreview_PanelHeightsStayAligned(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{TextPlain: strings.Repeat("Lorem ipsum ", 40)}
	m.focusedPanel = panelPreview

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	tableView := stripANSI(m.baseStyle.Render(renderStyledTableView(&m.timelineTable, 0)))
	previewView := stripANSI(m.renderEmailPreview())

	tableLines := strings.Split(tableView, "\n")
	previewLines := strings.Split(previewView, "\n")
	if len(tableLines) != len(previewLines) {
		t.Fatalf("expected aligned panel heights, table=%d preview=%d", len(tableLines), len(previewLines))
	}
}

func tableColumnTitles(cols []table.Column) []string {
	titles := make([]string, 0, len(cols))
	for _, col := range cols {
		if col.Width > 0 {
			titles = append(titles, normalizedTableColumnTitle(col.Title))
		}
	}
	return titles
}

func hasColumnTitle(cols []table.Column, title string) bool {
	for _, col := range cols {
		if col.Width > 0 && normalizedTableColumnTitle(col.Title) == title {
			return true
		}
	}
	return false
}

func normalizedTableColumnTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.TrimSuffix(title, " ↑")
	title = strings.TrimSuffix(title, " ↓")
	return title
}

func TestTimelineReadingFirstColumnsPrioritizeSenderAndSubject(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTableDimensions(220, 50)
	m.updateTimelineTable()

	for _, removed := range []string{"Size KB", "Att"} {
		if hasColumnTitle(m.timelineTable.Columns(), removed) {
			t.Fatalf("Timeline columns should not include %q: %#v", removed, tableColumnTitles(m.timelineTable.Columns()))
		}
	}
	for _, want := range []string{"✓", "Sender", "Subject", "When", "Tag"} {
		if !hasColumnTitle(m.timelineTable.Columns(), want) {
			t.Fatalf("wide Timeline columns should include %q: %#v", want, tableColumnTitles(m.timelineTable.Columns()))
		}
	}
	if m.timeline.senderWidth > 36 {
		t.Fatalf("wide sender column should be capped so subject stays dominant, senderWidth=%d", m.timeline.senderWidth)
	}
	if m.timeline.subjectWidth <= m.timeline.senderWidth {
		t.Fatalf("subject should be wider than sender, sender=%d subject=%d", m.timeline.senderWidth, m.timeline.subjectWidth)
	}

	m.timeline.selectedEmail = m.timeline.emails[0]
	m.updateTableDimensions(80, 24)
	m.updateTimelineTable()
	for _, removed := range []string{"Size KB", "Att", "Tag"} {
		if hasColumnTitle(m.timelineTable.Columns(), removed) {
			t.Fatalf("80x24 split Timeline should hide %q before shrinking sender/subject: %#v", removed, tableColumnTitles(m.timelineTable.Columns()))
		}
	}
	if m.timeline.senderWidth < 10 || m.timeline.subjectWidth < 14 {
		t.Fatalf("80x24 split Timeline should preserve usable sender/subject widths, sender=%d subject=%d", m.timeline.senderWidth, m.timeline.subjectWidth)
	}
}

func TestMainView_ShowsTopSyncStripDuringProgressiveStartup(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.loading = true
	m.progressInfo = models.ProgressInfo{
		Phase:   "scanning",
		Message: "Opening INBOX...",
	}
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	rendered := m.renderMainView()
	assertFitsWidth(t, 120, rendered)

	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, "IMAP connected") {
		t.Fatalf("expected top sync strip in main view, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "opening INBOX — another mail client may be busy") {
		t.Fatalf("expected sync strip to include the active progress message, got:\n%s", stripped)
	}
}

func TestMainView_ShowsIncrementalFetchProgressInSyncStrip(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.loading = true
	m.progressInfo = models.ProgressInfo{
		Phase:   "fetching",
		Current: 50,
		Total:   120,
		Message: "Fetched 50/120 new emails into cache",
	}
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "Syncing INBOX") {
		t.Fatalf("expected sync strip to explain incremental cache updates, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "50/120 new rows cached") {
		t.Fatalf("expected sync strip to include fetch progress, got:\n%s", rendered)
	}
}

func TestRenderMainView_DoesNotInsertBlankLineAfterTitleTabs(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	rendered := strings.Split(stripANSI(m.renderMainView()), "\n")
	if len(rendered) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(rendered))
	}
	if strings.TrimSpace(rendered[1]) == "" {
		t.Fatalf("expected content to start immediately after title/tab row, got blank line: %#v", rendered[:4])
	}
}

func TestTopSyncStrip_HidesOnceSyncCountsAreSettled(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = true
	m.syncCountsSettled = true
	m.progressInfo.Message = "Found 426 senders"
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	if got := stripANSI(m.renderTopSyncStrip()); got != "" {
		t.Fatalf("expected no top sync strip after counts settle, got %q", got)
	}
}

func TestTopSyncStrip_DoesNotAnimateWithSpinnerGlyph(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.loading = true
	m.syncCountsSettled = false
	m.progressInfo.Message = "Syncing INBOX | 50/120 new rows cached"
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	rendered := stripANSI(m.renderTopSyncStrip())
	for _, glyph := range spinnerChars {
		if strings.Contains(rendered, glyph) {
			t.Fatalf("expected top sync strip to avoid spinner glyphs, got %q", rendered)
		}
	}
}

func TestSyncHydratedMsg_DoesNotOverwriteLiveFolderCounts(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.currentFolder = "INBOX"
	m.syncGeneration = 7
	m.folderStatus["INBOX"] = models.FolderStatus{Unseen: 101, Total: 1317}

	model, _ := m.Update(SyncHydratedMsg{
		Folder:     "INBOX",
		Generation: 7,
		Emails:     mockEmails(),
	})
	updated := model.(*Model)

	if got := updated.folderStatus["INBOX"]; got.Unseen != 101 || got.Total != 1317 {
		t.Fatalf("expected live folder counts to remain authoritative, got %+v", got)
	}
}

func TestStatusBar_DoesNotMixTimelineRowCountWithLiveFolderTotal(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.currentFolder = "INBOX"
	m.folderStatus["INBOX"] = models.FolderStatus{Unseen: 1010, Total: 1318}
	m.timeline.emails = make([]*models.EmailData, 0, 1316)
	for i := 0; i < 1316; i++ {
		m.timeline.emails = append(m.timeline.emails, &models.EmailData{MessageID: fmt.Sprintf("msg-%d", i)})
	}

	status := stripANSI(m.renderStatusBar())
	if strings.Contains(status, "1316 emails") {
		t.Fatalf("expected live status bar to avoid cache-derived row count drift, got %q", status)
	}
	if !strings.Contains(status, "1010 unread / 1318 total") {
		t.Fatalf("expected live folder status in status bar, got %q", status)
	}
}

func TestNumberTwoSwitchesToContactsInsteadOfRetiredCleanup(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.loading = false
	m.currentFolder = "INBOX"

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 50})
	m = updated.(*Model)

	updated, _ = m.Update(keyRunes("2"))
	m = updated.(*Model)

	rendered := stripANSI(m.renderMainView())
	if m.activeTab != tabContacts {
		t.Fatalf("expected 2 to switch to Contacts, got tab %d", m.activeTab)
	}
	if strings.Contains(rendered, "Cleanup") {
		t.Fatalf("expected 2 not to open retired Cleanup view, got:\n%s", rendered)
	}
}

func TestRenderSidebar_InactiveSelectedFolderRemainsVisible(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.folders = []string{"INBOX", "Sent", "Archive"}
	m.folderTree = buildFolderTree(m.folders)
	m.currentFolder = "INBOX"
	m.focusedPanel = panelTimeline
	m.showSidebar = true

	rendered := m.renderSidebar()
	lines := strings.Split(rendered, "\n")
	var inboxLine string
	for _, line := range lines {
		if strings.Contains(stripANSI(line), "INBOX") {
			inboxLine = line
			break
		}
	}
	if inboxLine == "" {
		t.Fatal("expected INBOX line in sidebar render")
	}
	if !strings.Contains(stripANSI(inboxLine), "›  INBOX") {
		t.Fatalf("expected inactive selected folder to keep a visible marker, got %q", stripANSI(inboxLine))
	}
}

func TestTimelineView_UsesConsistentPanelGaps(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.showSidebar = true
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]

	rendered := strings.Split(stripANSI(m.renderTimelineView()), "\n")
	if len(rendered) == 0 {
		t.Fatal("expected timeline view output")
	}
	top := rendered[0]
	if count := strings.Count(top, "┐ ┌"); count != 1 {
		t.Fatalf("expected a single gap between timeline and preview when preview auto-hides sidebar, got top line %q", top)
	}
}

func TestTimelineQuickReply_DoesNotOverflowScreen(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{TextPlain: strings.Repeat("Body line ", 80)}
	m.timeline.quickReplyOpen = true
	m.timeline.quickRepliesReady = true
	m.timeline.quickReplyIdx = 4
	m.timeline.quickReplies = []string{
		"No thanks.",
		"Thank you for reaching out.",
		"Thank you, I received it.",
		"I'll review this and follow up.",
		"I'll get back to you.",
		"[AI] This is a much longer reply suggestion that should wrap instead of being crushed into a single truncated row.",
		"[AI] Could you help me debug the setup code within the view and explain why SwiftUI keeps recreating it?",
		"[AI] Let's look at the setup code's position in the view tree and decide what should move out of init.",
	}
	m.focusedPanel = panelPreview
	m.updateTableDimensions(120, 40)

	rendered := m.renderMainView()
	lines := strings.Split(stripANSI(rendered), "\n")
	if len(lines) > 40 {
		t.Fatalf("expected quick reply view to fit terminal height, got %d lines", len(lines))
	}
	assertFitsWidth(t, 120, rendered)
	if strings.Contains(stripANSI(rendered), "0%────────────────") {
		t.Fatalf("expected scroll indicator and quick-reply separator to stay on separate lines, got:\n%s", stripANSI(rendered))
	}
}

func TestQuickReplyPicker_WrapsLongAIReplies(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.timeline.quickRepliesReady = true
	m.timeline.quickReplyIdx = 5
	m.timeline.quickReplies = []string{
		"No thanks.",
		"Thank you for reaching out.",
		"Thank you, I received it.",
		"I'll review this and follow up.",
		"I'll get back to you.",
		"[AI] This is a much longer reply suggestion that should wrap instead of being crushed into a single truncated row.",
	}

	lines := m.quickReplyPickerLines(38, 12)
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, "6. [AI]") {
		t.Fatalf("expected numbered AI reply in picker, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "suggestion that should wrap") {
		t.Fatalf("expected long AI reply to continue onto a wrapped line, got:\n%s", rendered)
	}
}
