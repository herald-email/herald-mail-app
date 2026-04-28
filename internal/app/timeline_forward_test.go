package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"mail-processor/internal/models"
)

type forwardBodyBackend struct {
	stubBackend
	body  *models.EmailBody
	err   error
	calls int
}

func (b *forwardBodyBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	b.calls++
	return b.body, b.err
}

func newTimelineForwardModel(b *forwardBodyBackend) (*Model, *models.EmailData) {
	email := &models.EmailData{
		MessageID: "msg-forward",
		UID:       42,
		Folder:    "INBOX",
		Sender:    "noreply@google.com",
		Subject:   "Your Google data has been exported",
		Date:      time.Date(2026, 4, 27, 20, 42, 0, 0, time.UTC),
	}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabTimeline
	m.currentFolder = "INBOX"
	m.timeline.emails = []*models.EmailData{email}
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)
	return m, email
}

func TestTimelineForwardWithoutPreviewFetchesBodyBeforeCompose(t *testing.T) {
	backend := &forwardBodyBackend{body: &models.EmailBody{TextPlain: "full forwarded body"}}
	m, _ := newTimelineForwardModel(backend)

	model, cmd, handled := m.handleTimelineKey(keyRunes("F"))
	updated := model.(*Model)

	if !handled {
		t.Fatal("expected F to be handled")
	}
	if cmd == nil {
		t.Fatal("expected F without loaded preview to return a body fetch command")
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab = %d, want Timeline while body loads", updated.activeTab)
	}
	if !strings.Contains(updated.statusMessage, "Loading forwarded message body") {
		t.Fatalf("statusMessage = %q, want loading message", updated.statusMessage)
	}
	if backend.calls != 0 {
		t.Fatalf("FetchEmailBody called before command execution: %d", backend.calls)
	}

	msg := cmd()
	if _, ok := msg.(TimelineForwardBodyMsg); !ok {
		t.Fatalf("cmd returned %T, want TimelineForwardBodyMsg", msg)
	}
	if backend.calls != 1 {
		t.Fatalf("FetchEmailBody calls = %d, want 1", backend.calls)
	}
}

func TestTimelineForwardBodyResultOpensComposeWithFetchedBody(t *testing.T) {
	backend := &forwardBodyBackend{body: &models.EmailBody{TextPlain: "Your account, your data.\n\nFull body line."}}
	m, _ := newTimelineForwardModel(backend)

	model, cmd, _ := m.handleTimelineKey(keyRunes("F"))
	updated := model.(*Model)
	msg := cmd().(TimelineForwardBodyMsg)

	model, _, handled := updated.handleTimelineMsg(msg)
	updated = model.(*Model)

	if !handled {
		t.Fatal("expected TimelineForwardBodyMsg to be handled")
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab = %d, want Compose", updated.activeTab)
	}
	if got := updated.composeSubject.Value(); got != "Fwd: Your Google data has been exported" {
		t.Fatalf("compose subject = %q", got)
	}
	body := updated.composeBody.Value()
	for _, want := range []string{
		"--- Forwarded message ---",
		"From: noreply@google.com",
		"Subject: Your Google data has been exported",
		"Your account, your data.",
		"Full body line.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("compose body missing %q:\n%s", want, body)
		}
	}
}

func TestTimelineForwardWithLoadedPreviewOpensComposeImmediately(t *testing.T) {
	backend := &forwardBodyBackend{body: &models.EmailBody{TextPlain: "should not fetch"}}
	m, email := newTimelineForwardModel(backend)
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "preview-loaded body"}

	model, cmd, handled := m.handleTimelineKey(keyRunes("F"))
	updated := model.(*Model)

	if !handled {
		t.Fatal("expected F to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no fetch command when matching preview body is loaded, got %T", cmd)
	}
	if backend.calls != 0 {
		t.Fatalf("FetchEmailBody calls = %d, want 0", backend.calls)
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab = %d, want Compose", updated.activeTab)
	}
	if !strings.Contains(updated.composeBody.Value(), "preview-loaded body") {
		t.Fatalf("compose body missing loaded preview body:\n%s", updated.composeBody.Value())
	}
}

func TestTimelineForwardIgnoresStaleBodyResult(t *testing.T) {
	backend := &forwardBodyBackend{body: &models.EmailBody{TextPlain: "first body"}}
	m, _ := newTimelineForwardModel(backend)

	_, firstCmd, _ := m.handleTimelineKey(keyRunes("F"))
	backend.body = &models.EmailBody{TextPlain: "second body"}
	_, secondCmd, _ := m.handleTimelineKey(keyRunes("F"))

	firstMsg := firstCmd().(TimelineForwardBodyMsg)
	model, _, handled := m.handleTimelineMsg(firstMsg)
	updated := model.(*Model)
	if !handled {
		t.Fatal("expected stale TimelineForwardBodyMsg to be handled")
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab = %d after stale result, want Timeline", updated.activeTab)
	}
	if updated.composeBody.Value() != "" {
		t.Fatalf("stale result populated compose body: %q", updated.composeBody.Value())
	}

	secondMsg := secondCmd().(TimelineForwardBodyMsg)
	model, _, _ = updated.handleTimelineMsg(secondMsg)
	updated = model.(*Model)
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab = %d after latest result, want Compose", updated.activeTab)
	}
	if !strings.Contains(updated.composeBody.Value(), "second body") {
		t.Fatalf("latest compose body missing second body:\n%s", updated.composeBody.Value())
	}
}

func TestTimelineForwardFetchErrorOpensComposeWithVisibleFailureNote(t *testing.T) {
	backend := &forwardBodyBackend{err: errors.New("imap unavailable")}
	m, _ := newTimelineForwardModel(backend)

	model, cmd, _ := m.handleTimelineKey(keyRunes("F"))
	updated := model.(*Model)
	msg := cmd().(TimelineForwardBodyMsg)

	model, _, _ = updated.handleTimelineMsg(msg)
	updated = model.(*Model)

	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab = %d, want Compose", updated.activeTab)
	}
	if !strings.Contains(updated.composeBody.Value(), "Forward body failed to load") {
		t.Fatalf("compose body missing failure note:\n%s", updated.composeBody.Value())
	}
	if !strings.Contains(updated.composeStatus, "Forward body failed to load") {
		t.Fatalf("composeStatus = %q, want failure status", updated.composeStatus)
	}
}
