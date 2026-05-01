package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type timelineDraftBackend struct {
	stubBackend
	body                *models.EmailBody
	calls               int
	sendDraftCalls      int
	sentDraftUID        uint32
	sentDraftFolder     string
	deletedDraftMessage string
}

func (b *timelineDraftBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	b.calls++
	return b.body, nil
}

func (b *timelineDraftBackend) SendDraft(uid uint32, folder string) error {
	b.sendDraftCalls++
	b.sentDraftUID = uid
	b.sentDraftFolder = folder
	return nil
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

func TestUpdateTimelineTable_ExpandedReplyDraftShowsReplyAndDraftMarkers(t *testing.T) {
	now := time.Now()
	m := New(&stubBackend{}, nil, "me@example.com", nil, false)
	m.timeline.senderWidth = 40
	m.timeline.subjectWidth = 64
	m.timeline.expandedThreads["interview with cobalt works"] = true
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
	rows := m.timelineTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected expanded thread rows, got %d: %#v", len(rows), rows)
	}
	senderCell := stripANSI(rows[1][1])
	subjectCell := stripANSI(rows[1][2])
	if !strings.Contains(senderCell, threadReplyPrefix) || !strings.Contains(senderCell, "Draft") {
		t.Fatalf("reply draft sender cell should show both reply and draft state, got sender=%q subject=%q", senderCell, subjectCell)
	}
	if !strings.Contains(subjectCell, "Draft reply") {
		t.Fatalf("reply draft subject should be labelled Draft reply, got %q", subjectCell)
	}
	if !strings.Contains(subjectCell, "Interview with Cobalt Works") {
		t.Fatalf("reply draft subject should still show the thread subject, got %q", subjectCell)
	}
	if got := rows[1][6]; got != "" {
		t.Fatalf("draft marker must not use Tag column, got tag %q", got)
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
	if !strings.Contains(stripped, "State: Draft reply - E edit draft - Ctrl+S send") {
		t.Fatalf("expected draft state note in preview header, got:\n%s", stripped)
	}
}

func TestRenderEmailPreview_DraftReplyShowsThreadContext(t *testing.T) {
	now := time.Date(2026, 4, 30, 19, 26, 0, 0, time.UTC)
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.previewWidth = 72
	m.timeline.senderWidth = 36
	m.timeline.subjectWidth = 72
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "draft",
			UID:       42,
			Sender:    "me@example.com",
			Subject:   "Re: Staff Software Engineer at Fractional AI",
			Date:      now,
			Folder:    "Drafts",
			IsDraft:   true,
		},
		{
			MessageID: "original",
			UID:       8,
			Sender:    "Flavia da Silva <flavia.iespa@fractional.ai>",
			Subject:   "Staff Software Engineer at Fractional AI",
			Date:      now.Add(-11 * time.Hour),
			Folder:    "INBOX",
		},
	}
	m.updateTimelineTable()
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.bodyMessageID = "draft"
	m.timeline.body = &models.EmailBody{
		TextPlain: "Hi Flavia,\n\nThanks for reaching out.",
	}

	rendered := stripANSI(m.renderEmailPreview())
	for _, want := range []string{
		"State: Draft reply - E edit draft - Ctrl+S send",
		"Thread: 2 messages",
		"Draft reply",
		"me@example.com",
		"Flavia da Silva",
		"Hi Flavia",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected draft reply preview to include %q, got:\n%s", want, rendered)
		}
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

func TestTimelineSendDraftFromCollapsedThreadDoesNotOpenCompose(t *testing.T) {
	now := time.Now()
	backend := &timelineDraftBackend{}
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

	model, cmd, handled := m.handleTimelineKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	updated := model.(*Model)
	if !handled {
		t.Fatal("expected Ctrl+S to be handled on collapsed thread containing a draft")
	}
	if cmd != nil {
		t.Fatal("expected Ctrl+S to open confirmation before sending")
	}
	if !updated.pendingDeleteConfirm || !strings.Contains(updated.pendingDeleteDesc, "Send draft") {
		t.Fatalf("expected send-draft confirmation, pending=%v desc=%q", updated.pendingDeleteConfirm, updated.pendingDeleteDesc)
	}

	model, cmd, handled = updated.handleOverlayKey(keyRunes("y"))
	updated = model.(*Model)
	if !handled {
		t.Fatal("expected confirmation to be handled")
	}
	if cmd == nil {
		t.Fatal("expected send draft command after confirmation")
	}
	raw := cmd()
	msg, ok := raw.(TimelineDraftSentMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want TimelineDraftSentMsg", raw)
	}
	if msg.Err != nil {
		t.Fatalf("send draft returned error: %v", msg.Err)
	}
	if backend.sendDraftCalls != 1 || backend.sentDraftUID != 42 || backend.sentDraftFolder != "Drafts" {
		t.Fatalf("expected SendDraft(42, Drafts), got calls=%d uid=%d folder=%q", backend.sendDraftCalls, backend.sentDraftUID, backend.sentDraftFolder)
	}

	model, _, handled = updated.handleTimelineMsg(msg)
	updated = model.(*Model)
	if !handled {
		t.Fatal("expected TimelineDraftSentMsg to be handled")
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab = %d, want Timeline", updated.activeTab)
	}
	if updated.statusMessage != "Draft sent" {
		t.Fatalf("statusMessage = %q, want Draft sent", updated.statusMessage)
	}
	for _, email := range updated.timeline.emails {
		if email.MessageID == "draft" {
			t.Fatalf("sent draft should be removed from Timeline emails: %#v", updated.timeline.emails)
		}
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
	requireHintSegments(t, hints, "E: edit draft", "ctrl+s: send draft", "D: discard draft")
	if strings.Contains(hints, "R: reply") || strings.Contains(hints, "F: forward") {
		t.Fatalf("draft preview hints should prioritize draft workflow, got %q", hints)
	}
}
