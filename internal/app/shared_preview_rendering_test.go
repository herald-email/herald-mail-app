package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func richHTMLPreviewBody() *models.EmailBody {
	return &models.EmailBody{
		TextPlain: "plain fallback should not render",
		TextHTML: `<html><body>
			<h1>HTML wins</h1>
			<p><strong>Budget alert</strong> for <em>Project Orion</em>.</p>
			<ul><li>Review reserved capacity</li><li>Share the report</li></ul>
			<p><a href="https://reports.example.test/orion?utm_source=email&token=abcdefghijklmnopqrstuvwxyz0123456789">Open report</a></p>
			<p><img alt="Remote chart" src="https://reports.example.test/chart.png"></p>
		</body></html>`,
	}
}

func assertRichHTMLPreview(t *testing.T, rendered string) {
	t.Helper()
	visible := ansi.Strip(rendered)
	for _, want := range []string{
		"HTML wins",
		"Budget alert",
		"Project Orion",
		"Review reserved capacity",
		"Share the report",
		"Open report",
		"Remote chart",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("preview missing %q:\n%s", want, visible)
		}
	}
	for _, bad := range []string{
		"plain fallback should not render",
		"# HTML wins",
		"**Budget alert**",
		"*Project Orion*",
		"abcdefghijklmnopqrstuvwxyz0123456789",
		"utm_source=email",
		"reports.example.test/chart.png",
	} {
		if strings.Contains(visible, bad) {
			t.Fatalf("preview leaked %q:\n%s", bad, visible)
		}
	}
}

func TestTimelineSplitPreviewPrefersSharedHTMLMarkdownRendering(t *testing.T) {
	email := &models.EmailData{
		MessageID: "rich-html-timeline",
		Sender:    "reports@example.test",
		Subject:   "Rich HTML",
		Date:      time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 140, 50)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = richHTMLPreviewBody()

	assertRichHTMLPreview(t, m.renderEmailPreview())
}

func TestTimelineFullScreenPreviewUsesSharedHTMLMarkdownRendering(t *testing.T) {
	email := &models.EmailData{
		MessageID: "rich-html-fullscreen",
		Sender:    "reports@example.test",
		Subject:   "Rich HTML",
		Date:      time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 120, 40)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.fullScreen = true
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = richHTMLPreviewBody()

	assertRichHTMLPreview(t, m.renderFullScreenEmail())
}

func TestCleanupSplitPreviewPrefersSharedHTMLMarkdownRendering(t *testing.T) {
	email := &models.EmailData{
		MessageID: "rich-html-cleanup",
		Sender:    "reports@example.test",
		Subject:   "Rich HTML",
		Date:      time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 140, 50)
	defer m.cleanup()
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupPreviewEmail = email
	m.cleanupEmailBody = richHTMLPreviewBody()
	m.cleanupPreviewWidth = 110

	assertRichHTMLPreview(t, m.renderCleanupPreview())
}

func TestCleanupFullScreenPreviewUsesSharedHTMLMarkdownRendering(t *testing.T) {
	email := &models.EmailData{
		MessageID: "rich-html-cleanup-fullscreen",
		Sender:    "reports@example.test",
		Subject:   "Rich HTML",
		Date:      time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 120, 40)
	defer m.cleanup()
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewEmail = email
	m.cleanupEmailBody = richHTMLPreviewBody()
	m.cleanupPreviewWidth = 120

	assertRichHTMLPreview(t, m.renderCleanupPreview())
}

func TestContactsInlinePreviewUsesSharedHTMLMarkdownRendering(t *testing.T) {
	email := &models.EmailData{
		MessageID: "rich-html-contact",
		Sender:    "reports@example.test",
		Subject:   "Rich HTML",
		Date:      time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	contact := models.ContactData{
		Email:       "reports@example.test",
		DisplayName: "Reports",
		EmailCount:  1,
	}
	m := makeSizedModel(t, 160, 50)
	defer m.cleanup()
	m.activeTab = tabContacts
	m.contactsFiltered = []models.ContactData{contact}
	m.contactDetail = &contact
	m.contactDetailEmails = []*models.EmailData{email}
	m.contactPreviewEmail = email
	m.contactPreviewBody = richHTMLPreviewBody()
	m.contactPreviewLoading = false

	assertRichHTMLPreview(t, m.renderContactsTab(160, 50))
}
