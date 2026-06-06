package app

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/memory"
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

func TestTimelinePreviewShowsHeraldThreadMemoryDossier(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	email := recipientHeaderEmail()
	email.MessageID = "demo-example-thread-with-cobalt-works@demo.local"
	email.Date = now
	openLoop := memory.PrepareMemoryForAppend(memory.Memory{
		ID:             "mem-thread-open-loop",
		Kind:           memory.KindOpenQuestion,
		Claim:          "Mina asked whether the Cobalt Works interview schedule still works.",
		Summary:        "Mina asked whether the Cobalt Works interview schedule still works.",
		Topic:          "Example: Thread with Cobalt Works",
		People:         []string{"Mina Park", "mina@cobalt-works.example"},
		Company:        "Cobalt Works",
		Domain:         "cobalt-works.example",
		Status:         memory.StatusWaiting,
		Confidence:     0.93,
		LastActivityAt: now,
		ObsidianTarget: "Job search/active/Cobalt Works/Memory.md",
		Evidence: []memory.Evidence{{
			SourceType: memory.SourceEmail,
			MessageID:  email.MessageID,
			Folder:     "INBOX",
			Date:       now,
			Snippet:    "Does the interview schedule still work for you?",
		}},
	}, now)
	trackStatus := memory.PrepareMemoryForAppend(memory.Memory{
		ID:             "mem-thread-track",
		Kind:           memory.KindTrackStatus,
		Claim:          "Cobalt Works interview is waiting on scheduling follow-up.",
		Summary:        "Cobalt Works interview is waiting on scheduling follow-up.",
		Topic:          "Example: Thread with Cobalt Works",
		People:         []string{"Mina Park", "mina@cobalt-works.example"},
		Company:        "Cobalt Works",
		Domain:         "cobalt-works.example",
		Status:         memory.StatusWaiting,
		Confidence:     0.90,
		LastActivityAt: now.Add(-time.Hour),
		Evidence: []memory.Evidence{{
			SourceType: memory.SourceEmail,
			MessageID:  "demo-cobalt-track",
			Folder:     "INBOX",
			Date:       now.Add(-time.Hour),
		}},
	}, now)
	backend := &contactMemoryTestBackend{memories: []memory.Memory{openLoop, trackStatus}}
	m := makeSizedModel(t, 140, 40)
	m.backend = backend
	m.activeTab = tabTimeline
	m.timeline.previewWidth = 96
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = recipientHeaderBody()

	cmd := m.loadTimelineThreadMemoryDossier(email)
	if cmd == nil {
		t.Fatal("expected thread memory dossier command")
	}
	model, _, handled := m.handleTimelineMsg(cmd())
	if !handled {
		t.Fatal("expected ThreadMemoryDossierMsg to be handled")
	}
	m = model.(*Model)
	if len(backend.lastQueries) == 0 || backend.lastQueries[0].Topic != "Example: Thread with Cobalt Works" {
		t.Fatalf("thread memory queries = %#v", backend.lastQueries)
	}

	rendered := stripANSI(m.renderEmailPreview())
	for _, want := range []string{
		"Herald Memories",
		"Thread: Example: Thread with Cobalt Works",
		"Track: Example: Thread with Cobalt Works",
		"Open loop: Mina asked whether",
		"Vault: Job search/active/Cobalt Works/Memory.md",
		email.MessageID,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in thread memory preview:\n%s", want, rendered)
		}
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
