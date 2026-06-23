package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func newScopedBulkMutationServer(t *testing.T) (*Server, *backend.MultiBackend) {
	t.Helper()
	mb := backend.NewMultiDemoBackend()
	if err := mb.SwitchAccount(backend.AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}
	return &Server{backend: mb}, mb
}

func TestDaemonBulkDeleteRequiresScopedRefsForMultiAccount(t *testing.T) {
	s, _ := newScopedBulkMutationServer(t)
	body, _ := json.Marshal(map[string]any{"message_ids": []string{"same-message"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/bulk-delete", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	s.handleBulkDelete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected unscoped multi-account bulk delete to return 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDaemonBulkDeleteRoutesByLocalIDs(t *testing.T) {
	s, mb := newScopedBulkMutationServer(t)
	_, personalRef := duplicateMessageRefs(t, mb)
	body, _ := json.Marshal(map[string]any{
		"message_ids": []string{personalRef.MessageID},
		"local_ids":   []string{personalRef.LocalID},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/bulk-delete", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	s.handleBulkDelete(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected scoped bulk delete to succeed, got %d: %s", rr.Code, rr.Body.String())
	}
	if email, _ := mb.GetEmailByRef(personalRef); email != nil {
		t.Fatalf("personal scoped message still visible after bulk delete: %#v", email.MessageRef())
	}
}

type batchRecordingMutationBackend struct {
	*backend.MultiBackend
	batchRefs  [][]models.MessageRef
	singleRefs []models.MessageRef
}

func (b *batchRecordingMutationBackend) DeleteEmailsByRef(refs []models.MessageRef) error {
	b.batchRefs = append(b.batchRefs, append([]models.MessageRef(nil), refs...))
	return nil
}

func (b *batchRecordingMutationBackend) DeleteEmailByRef(ref models.MessageRef) error {
	b.singleRefs = append(b.singleRefs, ref)
	return b.MultiBackend.DeleteEmailByRef(ref)
}

func TestDaemonBulkDeleteUsesScopedBatchMutation(t *testing.T) {
	mb := backend.NewMultiDemoBackend()
	if err := mb.SwitchAccount(backend.AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}
	_, personalRef := duplicateMessageRefs(t, mb)
	recorder := &batchRecordingMutationBackend{MultiBackend: mb}
	s := &Server{backend: recorder}
	body, _ := json.Marshal(map[string]any{
		"message_ids": []string{personalRef.MessageID, personalRef.MessageID},
		"local_ids":   []string{personalRef.LocalID, personalRef.LocalID},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/bulk-delete", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	s.handleBulkDelete(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected scoped bulk delete to succeed, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := len(recorder.batchRefs); got != 1 {
		t.Fatalf("batch calls = %d, want 1", got)
	}
	if got := len(recorder.batchRefs[0]); got != 2 {
		t.Fatalf("batch size = %d, want 2", got)
	}
	if len(recorder.singleRefs) != 0 {
		t.Fatalf("expected no single DeleteEmailByRef calls, got %#v", recorder.singleRefs)
	}
}

func TestDaemonThreadAndSenderMutationsRequireSourceForMultiAccount(t *testing.T) {
	s, _ := newScopedBulkMutationServer(t)
	tests := []struct {
		name   string
		path   string
		body   map[string]any
		invoke func(*httptest.ResponseRecorder, *http.Request)
	}{
		{
			name: "delete thread",
			path: "/v1/threads/delete",
			body: map[string]any{"folder": "INBOX", "subject": "Roadmap"},
			invoke: func(rr *httptest.ResponseRecorder, req *http.Request) {
				s.handleDeleteThread(rr, req)
			},
		},
		{
			name: "archive sender",
			path: "/v1/senders/newsletter@example.com/archive",
			body: map[string]any{"folder": "INBOX"},
			invoke: func(rr *httptest.ResponseRecorder, req *http.Request) {
				req.SetPathValue("sender", "newsletter@example.com")
				s.handleArchiveSender(rr, req)
			},
		},
		{
			name: "soft unsubscribe",
			path: "/v1/senders/newsletter@example.com/soft-unsubscribe",
			body: map[string]any{"to_folder": "Disabled Subscriptions"},
			invoke: func(rr *httptest.ResponseRecorder, req *http.Request) {
				req.SetPathValue("sender", "newsletter@example.com")
				s.handleSoftUnsubscribeSender(rr, req)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(raw))
			rr := httptest.NewRecorder()
			tt.invoke(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for unscoped %s, got %d: %s", tt.name, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestDaemonThreadSenderAndDraftMutationsAcceptSourceScope(t *testing.T) {
	s, mb := newScopedBulkMutationServer(t)
	body, _ := json.Marshal(map[string]any{
		"folder":     "INBOX",
		"subject":    "Roadmap",
		"source_id":  "personal-mail",
		"account_id": "personal",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/threads/archive", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleArchiveThread(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected scoped archive thread to succeed, got %d: %s", rr.Code, rr.Body.String())
	}

	uid, folder, err := mb.SaveDraftForAccount("personal-mail", "to@example.test", "", "", "Scoped draft", "body")
	if err != nil {
		t.Fatalf("SaveDraftForAccount: %v", err)
	}
	req = httptest.NewRequest(http.MethodDelete, "/v1/drafts/1?folder="+folder+"&source_id=personal-mail&account_id=personal", nil)
	req.SetPathValue("uid", "1")
	rr = httptest.NewRecorder()
	if uid != 1 {
		t.Fatalf("demo draft UID = %d, want deterministic first UID 1", uid)
	}
	s.handleDeleteDraft(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected scoped delete draft to succeed, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDaemonDraftDeleteRequiresSourceForMultiAccount(t *testing.T) {
	s, _ := newScopedBulkMutationServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/v1/drafts/1?folder=Drafts", nil)
	req.SetPathValue("uid", "1")
	rr := httptest.NewRecorder()

	s.handleDeleteDraft(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected unscoped multi-account draft delete to return 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
