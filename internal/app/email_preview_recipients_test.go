package app

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func recipientHeaderEmail() *models.EmailData {
	return &models.EmailData{
		MessageID: "multi-recipient-preview",
		UID:       77,
		Sender:    "Mina Park <mina@cobalt-works.example>",
		Subject:   "Example: Thread with Cobalt Works",
		Date:      time.Date(2026, 4, 24, 11, 30, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
}

func recipientHeaderBody() *models.EmailBody {
	return &models.EmailBody{
		To:        "Rowan Finch <demo@demo.local>, Rae Stone <rae@cobalt-works.example>",
		CC:        "Hiring Panel <panel@cobalt-works.example>",
		BCC:       "Hidden Reviewer <hidden@cobalt-works.example>",
		TextPlain: "Scheduling context for the interview loop.",
	}
}

func requireHeaderOrder(t *testing.T, rendered string, labels ...string) {
	t.Helper()
	last := -1
	for _, label := range labels {
		idx := strings.Index(rendered, label)
		if idx < 0 {
			t.Fatalf("expected preview header to contain %q, got:\n%s", label, rendered)
		}
		if idx <= last {
			t.Fatalf("expected %q after previous header in:\n%s", label, rendered)
		}
		last = idx
	}
}

func TestTimelineSplitPreviewShowsLoadedToAndCcHeaders(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.previewWidth = 96
	email := recipientHeaderEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = recipientHeaderBody()

	rendered := stripANSI(m.renderEmailPreview())

	requireHeaderOrder(t, rendered, "From:", "To:", "Cc:", "Date:", "Subj:", "Tags:", "Actions:")
	for _, want := range []string{
		"To: Rowan Finch <demo@demo.local>, Rae Stone <rae@cobalt-works.example>",
		"Cc: Hiring Panel <panel@cobalt-works.example>",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected preview to contain %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Bcc:") {
		t.Fatalf("preview header must not expose Bcc, got:\n%s", rendered)
	}
}

func TestTimelineFullScreenPreviewShowsLoadedToAndCcHeaders(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	email := recipientHeaderEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = recipientHeaderBody()

	rendered := stripANSI(m.renderFullScreenEmail())

	requireHeaderOrder(t, rendered, "From:", "To:", "Cc:", "Date:", "Subj:", "Tags:", "Actions:")
	if !strings.Contains(rendered, "To: Rowan Finch <demo@demo.local>, Rae Stone <rae@cobalt-works.example>") {
		t.Fatalf("expected full-screen preview To header, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Cc: Hiring Panel <panel@cobalt-works.example>") {
		t.Fatalf("expected full-screen preview Cc header, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Bcc:") {
		t.Fatalf("full-screen preview header must not expose Bcc, got:\n%s", rendered)
	}
}

func TestCleanupPreviewShowsLoadedToAndCcHeaders(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCleanup
	m.showCleanupPreview = true
	m.cleanupPreviewWidth = 96
	m.cleanupPreviewEmail = recipientHeaderEmail()
	m.cleanupEmailBody = recipientHeaderBody()

	split := stripANSI(m.renderCleanupPreview())
	requireHeaderOrder(t, split, "From:", "To:", "Cc:", "Date:", "Subj:", "Tags:", "Actions:")
	if !strings.Contains(split, "To: Rowan Finch <demo@demo.local>, Rae Stone <rae@cobalt-works.example>") {
		t.Fatalf("expected cleanup split preview To header, got:\n%s", split)
	}
	if !strings.Contains(split, "Cc: Hiring Panel <panel@cobalt-works.example>") {
		t.Fatalf("expected cleanup split preview Cc header, got:\n%s", split)
	}

	m.cleanupFullScreen = true
	full := stripANSI(m.renderCleanupPreview())
	requireHeaderOrder(t, full, "From:", "To:", "Cc:", "Date:", "Subj:", "Tags:", "Actions:")
	if strings.Contains(split, "Bcc:") || strings.Contains(full, "Bcc:") {
		t.Fatalf("cleanup preview header must not expose Bcc, got split:\n%s\nfull:\n%s", split, full)
	}
}
