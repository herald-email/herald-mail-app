package app

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type timelineDraftBackend struct {
	stubBackend
	body  *models.EmailBody
	calls int
}

func (b *timelineDraftBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	b.calls++
	return b.body, nil
}

func TestUpdateTimelineTable_MarksDraftRowsAndCollapsedThreads(t *testing.T) {
	now := time.Now()
	m := New(&stubBackend{}, nil, "me@example.com", nil, false)
	m.timeline.senderWidth = 32
	m.timeline.subjectWidth = 64
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "latest",
			UID:       1,
			Sender:    "Rae <rae@cobalt-works.example>",
			Subject:   "Interview with Cobalt Works",
			Date:      now,
			Folder:    "INBOX",
		},
		{
			MessageID: "draft",
			UID:       2,
			Sender:    "me@example.com",
			Subject:   "Re: Interview with Cobalt Works",
			Date:      now.Add(-time.Minute),
			Folder:    "Drafts",
			IsDraft:   true,
		},
		{
			MessageID: "solo-draft",
			UID:       3,
			Sender:    "me@example.com",
			Subject:   "Follow-up note",
			Date:      now.Add(-2 * time.Minute),
			Folder:    "Drafts",
			IsDraft:   true,
		},
	}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected collapsed thread plus single draft row, got %d rows: %#v", len(rows), rows)
	}
	if got := rows[0][2]; !strings.Contains(got, "Draft") || !strings.Contains(got, "[2]") {
		t.Fatalf("expected collapsed thread subject to show draft marker and count, got %q", got)
	}
	if got := rows[0][6]; got != "" {
		t.Fatalf("draft marker must not use Tag column, got tag %q", got)
	}
	if got := rows[1][2]; !strings.Contains(got, "Draft") || !strings.Contains(got, "Follow-up note") {
		t.Fatalf("expected single draft subject to show draft marker, got %q", got)
	}
	if got := rows[1][6]; got != "" {
		t.Fatalf("single draft marker must not use Tag column, got tag %q", got)
	}
}

func TestRenderPreviewHeaderLines_DraftStateNote(t *testing.T) {
	email := &models.EmailData{
		Sender:  "me@example.com",
		Subject: "Re: Invitation to Technical Interview",
		Date:    time.Date(2026, 4, 28, 12, 3, 0, 0, time.UTC),
		IsDraft: true,
	}

	lines := renderPreviewHeaderLines(email, "", false, 80, true)
	stripped := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(stripped, "State: Draft - E edit draft") {
		t.Fatalf("expected draft state note in preview header, got:\n%s", stripped)
	}
}

func TestTimelineEditDraftFetchesBodyAndOpensCompose(t *testing.T) {
	now := time.Now()
	backend := &timelineDraftBackend{body: &models.EmailBody{
		To:        "rae@cobalt-works.example, mina@cobalt-works.example",
		CC:        "recruiting@example.com",
		BCC:       "hidden@example.com",
		Subject:   "Re: Interview with Cobalt Works",
		TextPlain: "Hi Rae,\n\nThanks for the details.",
	}}
	m := New(backend, nil, "me@example.com", nil, false)
	m.loading = false
	m.activeTab = tabTimeline
	m.currentFolder = "INBOX"
	m.timeline.senderWidth = 32
	m.timeline.subjectWidth = 64
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "latest",
			UID:       1,
			Sender:    "Rae <rae@cobalt-works.example>",
			Subject:   "Interview with Cobalt Works",
			Date:      now,
			Folder:    "INBOX",
		},
		{
			MessageID: "draft",
			UID:       42,
			Sender:    "me@example.com",
			Subject:   "Re: Interview with Cobalt Works",
			Date:      now.Add(-time.Minute),
			Folder:    "Drafts",
			IsDraft:   true,
		},
	}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)

	model, cmd, handled := m.handleTimelineKey(keyRunes("E"))
	updated := model.(*Model)
	if !handled {
		t.Fatal("expected E to be handled on collapsed thread containing a draft")
	}
	if cmd == nil {
		t.Fatal("expected draft body fetch command")
	}
	if backend.calls != 0 {
		t.Fatalf("FetchEmailBody called before command execution: %d", backend.calls)
	}
	msg, ok := cmd().(TimelineDraftBodyMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want TimelineDraftBodyMsg", msg)
	}
	if backend.calls != 1 {
		t.Fatalf("FetchEmailBody calls = %d, want 1", backend.calls)
	}

	model, _, handled = updated.handleTimelineMsg(msg)
	updated = model.(*Model)
	if !handled {
		t.Fatal("expected TimelineDraftBodyMsg to be handled")
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab = %d, want Compose", updated.activeTab)
	}
	if got := updated.composeTo.Value(); got != "rae@cobalt-works.example, mina@cobalt-works.example" {
		t.Fatalf("compose To = %q", got)
	}
	if got := updated.composeCC.Value(); got != "recruiting@example.com" {
		t.Fatalf("compose CC = %q", got)
	}
	if got := updated.composeBCC.Value(); got != "hidden@example.com" {
		t.Fatalf("compose BCC = %q", got)
	}
	if got := updated.composeSubject.Value(); got != "Re: Interview with Cobalt Works" {
		t.Fatalf("compose Subject = %q", got)
	}
	if got := updated.composeBody.Value(); !strings.Contains(got, "Thanks for the details.") {
		t.Fatalf("compose Body = %q", got)
	}
	if updated.lastDraftUID != 42 || updated.lastDraftFolder != "Drafts" {
		t.Fatalf("expected source draft tracking 42/Drafts, got %d/%q", updated.lastDraftUID, updated.lastDraftFolder)
	}
}

func TestRenderKeyHints_DraftPreviewPrioritizesEditAndDiscard(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "draft",
			UID:       42,
			Sender:    "me@example.com",
			Subject:   "Draft subject",
			Folder:    "Drafts",
			IsDraft:   true,
		},
	}
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "draft"
	m.timeline.body = &models.EmailBody{TextPlain: "draft body"}
	m.focusedPanel = panelPreview

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "E: edit draft", "D: discard draft")
	if strings.Contains(hints, "R: reply") || strings.Contains(hints, "F: forward") {
		t.Fatalf("draft preview hints should prioritize draft workflow, got %q", hints)
	}
}
