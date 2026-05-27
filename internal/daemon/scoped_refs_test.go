package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func newScopedReadTestServer(t *testing.T) (*Server, *backend.MultiBackend) {
	t.Helper()
	mb := backend.NewMultiDemoBackend()
	if err := mb.SwitchAccount(backend.AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}
	return &Server{backend: mb}, mb
}

func TestDaemonGetEmailsAcceptsOptionalSourceScope(t *testing.T) {
	s, _ := newScopedReadTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/emails?folder=INBOX&source_id=work-mail&account_id=work", nil)
	rr := httptest.NewRecorder()
	s.handleGetEmails(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var emails []models.EmailData
	if err := json.NewDecoder(rr.Body).Decode(&emails); err != nil {
		t.Fatalf("decode emails: %v", err)
	}
	if len(emails) == 0 {
		t.Fatalf("expected scoped emails, got none")
	}
	for _, email := range emails {
		if email.SourceID != "work-mail" || email.AccountID != "work" {
			t.Fatalf("unscoped email leaked into response: %#v", email.MessageRef())
		}
	}
}

func TestDaemonGetEmailUsesScopedRefWhenDuplicateMessageIDExists(t *testing.T) {
	s, mb := newScopedReadTestServer(t)
	emails, err := mb.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatalf("GetTimelineEmails: %v", err)
	}
	var duplicateID string
	seen := map[string]models.SourceID{}
	for _, email := range emails {
		if prev, ok := seen[email.MessageID]; ok && prev != email.SourceID {
			duplicateID = email.MessageID
			break
		}
		seen[email.MessageID] = email.SourceID
	}
	if duplicateID == "" {
		t.Fatalf("demo multi-account mailbox did not expose duplicate message IDs")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/emails/"+duplicateID+"?folder=INBOX&source_id=personal-mail&account_id=personal", nil)
	req.SetPathValue("id", duplicateID)
	rr := httptest.NewRecorder()
	s.handleGetEmail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var email models.EmailData
	if err := json.NewDecoder(rr.Body).Decode(&email); err != nil {
		t.Fatalf("decode email: %v", err)
	}
	if email.SourceID != "personal-mail" || email.AccountID != "personal" {
		t.Fatalf("GetEmail did not honor scoped ref: got %#v", email.MessageRef())
	}
}
