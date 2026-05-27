package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func subscribeTestBroadcaster(b *Broadcaster) <-chan string {
	sub := &subscriber{ch: make(chan string, 16)}
	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return sub.ch
}

func nextSSELine(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case line := <-ch:
		return line
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for SSE line")
		return ""
	}
}

func TestDaemonBroadcastProgressAddsScopedFieldsWithoutRenamingEvent(t *testing.T) {
	b := NewBroadcaster()
	ch := subscribeTestBroadcaster(b)
	s := &Server{broadcaster: b}

	s.broadcastProgress(models.ProgressInfo{
		Phase:        "fetching",
		Message:      "fetching rows",
		CollectionID: "INBOX",
	})

	line := nextSSELine(t, ch)
	if !strings.Contains(line, "event: progress") {
		t.Fatalf("progress event name changed: %q", line)
	}
	for _, want := range []string{
		`"Phase":"fetching"`,
		`"source_id":"default-mail"`,
		`"account_id":"default"`,
		`"collection_id":"INBOX"`,
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("progress event missing %s:\n%s", want, line)
		}
	}
}

func TestDaemonBroadcastNewEmailsAddsScopedCompanionEvent(t *testing.T) {
	b := NewBroadcaster()
	ch := subscribeTestBroadcaster(b)
	s := &Server{broadcaster: b}

	email := (&models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "event-message@example.test",
		Folder:    "INBOX",
	}).MessageRef()
	s.broadcastNewEmails(models.NewEmailsNotification{
		SourceID:  "work-mail",
		AccountID: "work",
		Emails: []*models.EmailData{{
			SourceID:  email.SourceID,
			AccountID: email.AccountID,
			LocalID:   email.LocalID,
			MessageID: email.MessageID,
			Folder:    email.Folder,
		}},
		Folder: "INBOX",
	})

	legacy := nextSSELine(t, ch)
	scoped := nextSSELine(t, ch)
	if !strings.Contains(legacy, "event: new_emails") {
		t.Fatalf("legacy new_emails event missing: %q", legacy)
	}
	if !strings.Contains(scoped, "event: new_emails_scoped") {
		t.Fatalf("scoped companion event missing: %q", scoped)
	}
	for _, want := range []string{
		`"source_id":"work-mail"`,
		`"account_id":"work"`,
		`"collection_id":"INBOX"`,
		`"item_ids":["mail:work-mail:work:INBOX:event-message@example.test"]`,
	} {
		if !strings.Contains(scoped, want) {
			t.Fatalf("scoped new_emails event missing %s:\n%s", want, scoped)
		}
	}
}

func TestDaemonMutationHandlerBroadcastsScopedMutationEvent(t *testing.T) {
	b := NewBroadcaster()
	ch := subscribeTestBroadcaster(b)
	s := &Server{backend: backend.NewScopedDemoBackend(backend.AccountInfo{
		SourceID:  "work-mail",
		AccountID: "work",
	}), broadcaster: b}

	req := httptest.NewRequest(http.MethodPost, "/v1/emails/demo-welcome-to-herald@demo.local/read?folder=INBOX&source_id=work-mail&account_id=work", nil)
	req.SetPathValue("id", "demo-welcome-to-herald@demo.local")
	rr := httptest.NewRecorder()
	s.handleMarkRead(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
	line := nextSSELine(t, ch)
	if !strings.Contains(line, "event: mutation") {
		t.Fatalf("mutation event missing: %q", line)
	}
	var event struct {
		Event string `json:"-"`
		Data  struct {
			Operation    string `json:"operation"`
			SourceID     string `json:"source_id"`
			AccountID    string `json:"account_id"`
			CollectionID string `json:"collection_id"`
			ItemID       string `json:"item_id"`
			MessageID    string `json:"message_id"`
		}
	}
	data := strings.TrimPrefix(strings.Split(line, "\n")[1], "data: ")
	if err := json.Unmarshal([]byte(data), &event.Data); err != nil {
		t.Fatalf("decode mutation event: %v\n%s", err, line)
	}
	if event.Data.Operation != "mark_read" || event.Data.SourceID != "work-mail" || event.Data.AccountID != "work" || event.Data.CollectionID != "INBOX" {
		t.Fatalf("unexpected mutation event: %#v", event.Data)
	}
	if event.Data.ItemID != "mail:work-mail:work:INBOX:demo-welcome-to-herald@demo.local" {
		t.Fatalf("item_id = %q", event.Data.ItemID)
	}
	if event.Data.MessageID != "demo-welcome-to-herald@demo.local" {
		t.Fatalf("message_id = %q", event.Data.MessageID)
	}
}

func TestDaemonBroadcastSyncAndValidIDsExposeScopedFields(t *testing.T) {
	b := NewBroadcaster()
	ch := subscribeTestBroadcaster(b)
	s := &Server{broadcaster: b}

	s.broadcastSyncEvent(models.FolderSyncEvent{
		SourceID:   "work-mail",
		AccountID:  "work",
		Folder:     "INBOX",
		Generation: 9,
		Phase:      models.SyncPhaseRowsCached,
	})
	syncLine := nextSSELine(t, ch)
	if !strings.Contains(syncLine, "event: sync") {
		t.Fatalf("sync event missing: %q", syncLine)
	}
	for _, want := range []string{
		`"SourceID":"work-mail"`,
		`"AccountID":"work"`,
		`"collection_id":"INBOX"`,
		`"Folder":"INBOX"`,
	} {
		if !strings.Contains(syncLine, want) {
			t.Fatalf("sync event missing %s:\n%s", want, syncLine)
		}
	}

	s.broadcastValidIDsNotification(models.ValidIDsNotification{
		SourceID:     "work-mail",
		AccountID:    "work",
		CollectionID: "INBOX",
		IDs: map[string]bool{
			"stale@example.test": false,
			"valid@example.test": true,
		},
	})
	legacy := nextSSELine(t, ch)
	scoped := nextSSELine(t, ch)
	if !strings.Contains(legacy, "event: valid_ids") {
		t.Fatalf("legacy valid_ids event missing: %q", legacy)
	}
	if !strings.Contains(scoped, "event: valid_ids_scoped") {
		t.Fatalf("scoped valid_ids companion event missing: %q", scoped)
	}
	for _, want := range []string{
		`"source_id":"work-mail"`,
		`"account_id":"work"`,
		`"collection_id":"INBOX"`,
		`"item_ids":["valid@example.test"]`,
	} {
		if !strings.Contains(scoped, want) {
			t.Fatalf("valid_ids scoped event missing %s:\n%s", want, scoped)
		}
	}
}
