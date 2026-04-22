package app

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
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

func TestTimelineView_UsesConsistentPanelGaps(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]

	rendered := strings.Split(stripANSI(m.renderTimelineView()), "\n")
	if len(rendered) == 0 {
		t.Fatal("expected timeline view output")
	}
	top := rendered[0]
	if count := strings.Count(top, "┐  ┌"); count != 2 {
		t.Fatalf("expected equal 2-column gaps between sidebar/timeline and timeline/preview, got top line %q", top)
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
