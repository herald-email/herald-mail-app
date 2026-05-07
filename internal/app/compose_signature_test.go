package app

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const testSignature = "-- \nRowan Finch\nHerald Labs"

func withComposeSignature(m *Model) *Model {
	m.cfg = &config.Config{}
	m.cfg.Compose.Signature.Text = testSignature + "\n\n"
	return m
}

func TestBlankComposeInsertsConfiguredSignature(t *testing.T) {
	m := withComposeSignature(New(&stubBackend{}, nil, "rowan@example.com", nil, false))
	m.loading = false
	m.activeTab = tabTimeline

	cmd := m.openBlankComposeFromCurrent()

	if cmd != nil {
		t.Fatalf("expected blank compose open to be synchronous, got %T", cmd)
	}
	if got := m.composeBody.Value(); got != "\n\n"+testSignature {
		t.Fatalf("compose body = %q, want signature", got)
	}
	assertComposeBodyCursorAtStart(t, m)
}

func TestReplyAndForwardComposeAppendConfiguredSignature(t *testing.T) {
	backend := &forwardBodyBackend{body: &models.EmailBody{TextPlain: "original", TextHTML: "<p>original</p>"}}
	m, email := newTimelineForwardModel(backend)
	withComposeSignature(m)

	m.openTimelineReplyCompose(email, backend.body, "", false)
	if got := m.composeBody.Value(); got != "\n\n"+testSignature {
		t.Fatalf("reply body = %q, want signature", got)
	}
	assertComposeBodyCursorAtStart(t, m)
	if m.composePreserved == nil || m.composePreserved.kind != models.PreservedMessageKindReply {
		t.Fatal("expected preserved reply context")
	}

	m.openTimelineForwardCompose(email, backend.body, "")
	if got := m.composeBody.Value(); got != "\n\n"+testSignature {
		t.Fatalf("forward body = %q, want signature", got)
	}
	assertComposeBodyCursorAtStart(t, m)
	if m.composePreserved == nil || m.composePreserved.kind != models.PreservedMessageKindForward {
		t.Fatal("expected preserved forward context")
	}
}

func TestQuickReplyAppendsConfiguredSignature(t *testing.T) {
	m := withComposeSignature(New(&stubBackend{}, nil, "rowan@example.com", nil, false))
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-quick",
		Sender:    "alice@example.com",
		Subject:   "Check in",
		Date:      time.Now(),
	}

	model, cmd := m.openQuickReply("Sounds good.")
	updated := model.(*Model)

	if cmd != nil {
		t.Fatalf("expected quick reply open to be synchronous, got %T", cmd)
	}
	if got := updated.composeBody.Value(); got != "Sounds good.\n\n\n"+testSignature {
		t.Fatalf("quick reply body = %q", got)
	}
	assertComposeBodyCursorAtStart(t, updated)
}

func TestConfiguredSignatureIsNotDuplicated(t *testing.T) {
	m := withComposeSignature(New(&stubBackend{}, nil, "rowan@example.com", nil, false))
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-quick",
		Sender:    "alice@example.com",
		Subject:   "Check in",
		Date:      time.Now(),
	}

	model, _ := m.openQuickReply("Already signed.\n\n" + testSignature)
	updated := model.(*Model)

	if got := strings.Count(updated.composeBody.Value(), testSignature); got != 1 {
		t.Fatalf("signature count = %d in body %q, want 1", got, updated.composeBody.Value())
	}
}

func TestDraftEditDoesNotAppendConfiguredSignature(t *testing.T) {
	m := withComposeSignature(New(&stubBackend{}, nil, "rowan@example.com", nil, false))
	email := &models.EmailData{MessageID: "draft", UID: 42, Folder: "Drafts", Subject: "Saved", IsDraft: true}
	body := &models.EmailBody{To: "alice@example.com", Subject: "Saved", TextPlain: "Saved body"}

	m.openTimelineDraftCompose(email, body, "")

	if got := m.composeBody.Value(); got != "Saved body" {
		t.Fatalf("draft body = %q, want saved body unchanged", got)
	}
}

func TestSignatureOnlyBlankComposeDoesNotAutosave(t *testing.T) {
	m := withComposeSignature(New(&stubBackend{}, nil, "rowan@example.com", nil, false))
	m.loading = false
	m.activeTab = tabTimeline
	m.openBlankComposeFromCurrent()

	updatedModel, _ := m.Update(DraftSaveTickMsg{})
	updated := updatedModel.(*Model)

	if updated.draftSaving {
		t.Fatal("signature-only blank compose should not start draft autosave")
	}
}

func assertComposeBodyCursorAtStart(t *testing.T, m *Model) {
	t.Helper()
	if got := m.composeBody.Line(); got != 0 {
		t.Fatalf("compose body cursor line = %d, want 0", got)
	}
	if got := m.composeBody.LineInfo().ColumnOffset; got != 0 {
		t.Fatalf("compose body cursor column = %d, want 0", got)
	}
}
