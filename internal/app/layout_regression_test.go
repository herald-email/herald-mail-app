package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"mail-processor/internal/models"
)

type layoutBackend struct {
	stubBackend
	emailsBySender map[string][]*models.EmailData
	timelineEmails []*models.EmailData
}

func (b *layoutBackend) GetEmailsBySender(_ string) (map[string][]*models.EmailData, error) {
	return b.emailsBySender, nil
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

func makeCleanupEmails() map[string][]*models.EmailData {
	now := time.Date(2026, 4, 20, 18, 38, 0, 0, time.UTC)
	result := map[string][]*models.EmailData{
		"ShopifyBrand <orders@shopify-brand.example>": {},
		"Tech Weekly <newsletter@techweekly.example>": {},
	}
	for i := 0; i < 8; i++ {
		result["ShopifyBrand <orders@shopify-brand.example>"] = append(result["ShopifyBrand <orders@shopify-brand.example>"], &models.EmailData{
			MessageID: "shopify-" + string(rune('a'+i)),
			Sender:    "ShopifyBrand <orders@shopify-brand.example>",
			Subject:   "Your order has shipped #" + string(rune('1'+i)),
			Date:      now.AddDate(0, 0, -i),
			Size:      7000 + i*512,
			Folder:    "INBOX",
		})
	}
	for i := 0; i < 8; i++ {
		result["Tech Weekly <newsletter@techweekly.example>"] = append(result["Tech Weekly <newsletter@techweekly.example>"], &models.EmailData{
			MessageID: "tech-" + string(rune('a'+i)),
			Sender:    "Tech Weekly <newsletter@techweekly.example>",
			Subject:   "This Week in Tech #" + string(rune('1'+i)),
			Date:      now.AddDate(0, 0, -i),
			Size:      3000 + i*256,
			Folder:    "INBOX",
		})
	}
	return result
}

func makeCleanupStats() map[string]*models.SenderStats {
	now := time.Date(2026, 4, 20, 18, 38, 0, 0, time.UTC)
	return map[string]*models.SenderStats{
		"ShopifyBrand <orders@shopify-brand.example>": {
			TotalEmails:     8,
			AvgSize:         8.2 * 1024,
			WithAttachments: 0,
			FirstEmail:      now.AddDate(0, 0, -8),
			LastEmail:       now,
		},
		"Tech Weekly <newsletter@techweekly.example>": {
			TotalEmails:     8,
			AvgSize:         4.2 * 1024,
			WithAttachments: 0,
			FirstEmail:      now.AddDate(0, 0, -8),
			LastEmail:       now,
		},
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
	rendered := m.View()
	assertFitsWidth(t, 50, rendered)

	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, "60") || !strings.Contains(strings.ToLower(stripped), "resize") {
		t.Fatalf("minimum-size message should include a concrete resize target, got %q", stripped)
	}
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

func TestCleanupView_Fits80x24(t *testing.T) {
	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.currentFolder = "INBOX"
	m.folderStatus = map[string]models.FolderStatus{"INBOX": {Unseen: 12, Total: 38}}
	m.stats = makeCleanupStats()
	m.activeTab = tabCleanup
	m.updateSummaryTable()
	m.updateDetailsTable()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	rendered := m.renderMainView()
	assertFitsWidth(t, 80, rendered)
}

func TestCleanupPreview_Fits80x24(t *testing.T) {
	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.currentFolder = "INBOX"
	m.folderStatus = map[string]models.FolderStatus{"INBOX": {Unseen: 12, Total: 38}}
	m.stats = makeCleanupStats()
	m.activeTab = tabCleanup
	m.updateSummaryTable()
	m.updateDetailsTable()
	m.showCleanupPreview = true
	m.cleanupPreviewEmail = m.detailsEmails[0]
	m.cleanupEmailBody = &models.EmailBody{TextPlain: strings.Repeat("Hello world ", 20)}
	m.focusedPanel = panelDetails
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	rendered := m.renderMainView()
	assertFitsWidth(t, 80, rendered)
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

func TestRenderMainView_DoesNotInsertBlankLineAfterTabs(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	rendered := strings.Split(stripANSI(m.renderMainView()), "\n")
	if len(rendered) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(rendered))
	}
	if strings.TrimSpace(rendered[2]) == "" {
		t.Fatalf("expected content to start immediately after tab bar, got blank line: %#v", rendered[:4])
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

func TestCleanupSelectionPersistsAcrossReorderAndResize(t *testing.T) {
	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.loading = false
	m.currentFolder = "INBOX"
	m.showSidebar = false
	m.stats = makeCleanupStats()
	m.updateSummaryTable()
	m.updateDetailsTable()

	selectedKey, ok := m.summaryKeyAtCursor()
	if !ok {
		t.Fatal("expected a cleanup summary key at the current cursor")
	}
	m.toggleSelection()

	if !m.selectedSummaryKeys[selectedKey] {
		t.Fatalf("expected %q to be selected", selectedKey)
	}

	m.stats[selectedKey].TotalEmails = 1
	for key, stats := range m.stats {
		if key != selectedKey {
			stats.TotalEmails = 20
		}
	}
	m.updateSummaryTable()
	m.updateDetailsTable()

	foundCheckmark := false
	for rowIdx, key := range m.rowToSender {
		if key == selectedKey {
			row := m.summaryTable.Rows()[rowIdx]
			foundCheckmark = len(row) > 0 && row[0] == "✓"
			break
		}
	}
	if !foundCheckmark {
		t.Fatalf("expected selected cleanup key %q to keep its checkmark after reorder", selectedKey)
	}

	status := stripANSI(m.renderStatusBar())
	if !strings.Contains(status, "1 sender selected") {
		t.Fatalf("expected cleanup status to reflect stable selection, got %q", status)
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	foundCheckmark = false
	for rowIdx, key := range m.rowToSender {
		if key == selectedKey {
			row := m.summaryTable.Rows()[rowIdx]
			foundCheckmark = len(row) > 0 && row[0] == "✓"
			break
		}
	}
	if !foundCheckmark {
		t.Fatalf("expected selected cleanup key %q to keep its checkmark after resize", selectedKey)
	}
}

func TestCleanupDetailsRows_ReflowAfterResize(t *testing.T) {
	emailsBySender := makeCleanupEmails()
	const longSubject = "Your order has shipped and the package is out for delivery"
	emailsBySender["ShopifyBrand <orders@shopify-brand.example>"][0].Subject = longSubject

	b := &layoutBackend{emailsBySender: emailsBySender}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.loading = false
	m.currentFolder = "INBOX"
	m.stats = makeCleanupStats()
	m.updateSummaryTable()
	m.updateDetailsTable()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	if got := m.detailsTable.Rows()[0][2]; !strings.Contains(got, "…") {
		t.Fatalf("expected narrow cleanup subject to be truncated before resize, got %q", got)
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 220, Height: 50})
	m = updated.(*Model)

	want := sanitizeText(longSubject)
	if got := m.detailsTable.Rows()[0][2]; got != want {
		t.Fatalf("expected cleanup subject to reflow after resize, got %q want %q", got, want)
	}
}

func TestCleanupSummaryColumns_SimplifiedAndResponsive(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabCleanup
	m.updateTableDimensions(220, 50)

	wantTitles := []string{"✓", "Sender/Domain", "Count", "Dates"}
	gotCols := m.summaryTable.Columns()
	if len(gotCols) != len(wantTitles) {
		t.Fatalf("expected %d cleanup summary columns, got %d", len(wantTitles), len(gotCols))
	}
	for i, want := range wantTitles {
		if gotCols[i].Title != want {
			t.Fatalf("expected cleanup summary column %d to be %q, got %q", i, want, gotCols[i].Title)
		}
	}
	wideSenderWidth := gotCols[1].Width

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	gotCols = m.summaryTable.Columns()
	if len(gotCols) != len(wantTitles) {
		t.Fatalf("expected %d cleanup summary columns after resize, got %d", len(wantTitles), len(gotCols))
	}
	narrowSenderWidth := gotCols[1].Width
	if narrowSenderWidth >= wideSenderWidth {
		t.Fatalf("expected sender/domain column to shrink on narrower terminals, got wide=%d narrow=%d", wideSenderWidth, narrowSenderWidth)
	}
	if gotCols[0].Width < 1 {
		t.Fatalf("expected selection column to stay visible, got width %d", gotCols[0].Width)
	}
}

func TestBuildLayoutPlan_CleanupSharesWideScreensMoreEvenly(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeTab = tabCleanup

	plan := m.buildLayoutPlan(220, 50)
	total := plan.Cleanup.SummaryWidth + plan.Cleanup.DetailsWidth
	if total <= 0 {
		t.Fatalf("expected positive cleanup widths, got summary=%d details=%d", plan.Cleanup.SummaryWidth, plan.Cleanup.DetailsWidth)
	}
	if plan.Cleanup.SummaryWidth*100/total < 40 {
		t.Fatalf("expected cleanup summary panel to get a meaningful share of wide layouts, got summary=%d details=%d", plan.Cleanup.SummaryWidth, plan.Cleanup.DetailsWidth)
	}
}

func TestCleanupView_UsesCompactLeadingSelectionCell(t *testing.T) {
	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.loading = false
	m.currentFolder = "INBOX"
	m.stats = makeCleanupStats()
	m.updateSummaryTable()
	m.updateDetailsTable()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	lines := strings.Split(stripANSI(m.renderMainView()), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected cleanup view output, got %d lines", len(lines))
	}
	if !strings.Contains(lines[2], "┌") {
		t.Fatalf("expected cleanup border line, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "│✓") {
		t.Fatalf("expected cleanup summary header to start without extra left padding, got %q", lines[3])
	}
	if !strings.Contains(lines[4], "│ ") {
		t.Fatalf("expected cleanup data row to render flush to the border, got %q", lines[4])
	}
}

func TestCleanupView_TopBorderReflectsFocusedPanel(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(oldProfile)

	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.loading = false
	m.currentFolder = "INBOX"
	m.stats = makeCleanupStats()
	m.updateSummaryTable()
	m.updateDetailsTable()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	m.focusedPanel = panelSummary
	summaryBorder := strings.Split(m.renderMainView(), "\n")[2]

	m.focusedPanel = panelDetails
	detailsBorder := strings.Split(m.renderMainView(), "\n")[2]

	if summaryBorder == detailsBorder {
		t.Fatalf("expected cleanup top border to reflect focused panel")
	}
}

func TestCleanupPreview_UsesConsistentPanelGaps(t *testing.T) {
	b := &layoutBackend{emailsBySender: makeCleanupEmails()}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabCleanup
	m.loading = false
	m.currentFolder = "INBOX"
	m.stats = makeCleanupStats()
	m.updateSummaryTable()
	m.updateDetailsTable()
	m.showCleanupPreview = true
	m.cleanupPreviewEmail = m.detailsEmails[0]
	m.cleanupEmailBody = &models.EmailBody{TextPlain: strings.Repeat("Hello world ", 20)}
	m.focusedPanel = panelDetails

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 50})
	m = updated.(*Model)

	top := strings.Split(stripANSI(m.renderMainView()), "\n")[2]
	if count := strings.Count(top, "┐  ┌"); count != 2 {
		t.Fatalf("expected a single gap between each cleanup panel, got top line %q", top)
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
	if count := strings.Count(top, "┐  ┌"); count != 1 {
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
