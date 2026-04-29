package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/models"
)

func linkStressMarkdownBody() (string, string) {
	longURL := "https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?o=eyJmaXJzdF9uYW1lIjoiUm93YW4iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldSbFpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY"
	body := "Welcome to the trial.\n\n[Display in your browser](" + longURL + ")\n\n![Taskpad logo](https://taskpad.mail.example/_next/static/media/taskpad-logo.0-dsvhpw__1x7.png)\n\nHi Rowan"
	return body, longURL
}

func assertNoVisibleLinkDestination(t *testing.T, rendered, longURL string) {
	t.Helper()
	visible := ansi.Strip(rendered)
	if !strings.Contains(visible, "Display in your browser") {
		t.Fatalf("expected visible anchor label, got:\n%s", visible)
	}
	if !strings.Contains(visible, "Taskpad logo") {
		t.Fatalf("expected visible image-link label, got:\n%s", visible)
	}
	for _, bad := range []string{
		"taskpad.mail.example",
		"eyJmaXJzdF9uYW1l",
		"_next/static/media",
	} {
		if strings.Contains(visible, bad) {
			t.Fatalf("expected %q to be hidden from visible preview text, got:\n%s", bad, visible)
		}
	}
	if !strings.Contains(rendered, "\x1b]8;;"+longURL+"\x1b\\") {
		t.Fatalf("expected full URL to remain in OSC 8 target, got raw:\n%q", rendered)
	}
}

func TestTimelinePreview_HTMLMarkdownLinksUseReadableOSC8Labels(t *testing.T) {
	body, longURL := linkStressMarkdownBody()
	email := &models.EmailData{
		MessageID: "msg-link-stress",
		Sender:    "teams@example.test",
		Subject:   "Link stress",
		Date:      time.Date(2026, 4, 26, 2, 31, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 140, 50)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = email
	m.timeline.body = &models.EmailBody{TextPlain: body, IsFromHTML: true}
	m.timeline.bodyMessageID = email.MessageID

	assertNoVisibleLinkDestination(t, m.renderEmailPreview(), longURL)
}

func TestCleanupPreview_HTMLMarkdownLinksUseReadableOSC8Labels(t *testing.T) {
	body, longURL := linkStressMarkdownBody()
	email := &models.EmailData{
		MessageID: "cleanup-link-stress",
		Sender:    "teams@example.test",
		Subject:   "Link stress",
		Date:      time.Date(2026, 4, 26, 2, 31, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 140, 50)
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupPreviewEmail = email
	m.cleanupEmailBody = &models.EmailBody{TextPlain: body, IsFromHTML: true}
	m.cleanupPreviewWidth = 100

	assertNoVisibleLinkDestination(t, m.renderCleanupPreview(), longURL)
}
