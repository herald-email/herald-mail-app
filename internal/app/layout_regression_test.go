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
			TotalEmails:      8,
			AvgSize:          8.2 * 1024,
			WithAttachments:  0,
			FirstEmail:       now.AddDate(0, 0, -8),
			LastEmail:        now,
		},
		"Tech Weekly <newsletter@techweekly.example>": {
			TotalEmails:      8,
			AvgSize:          4.2 * 1024,
			WithAttachments:  0,
			FirstEmail:       now.AddDate(0, 0, -8),
			LastEmail:        now,
		},
	}
}

func TestStatusChrome_UTF8SafeAt80Cols(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timelineEmails = mockEmails()
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
