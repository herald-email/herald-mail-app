package daemon

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func duplicateMessageRefs(t *testing.T, mb *backend.MultiBackend) (models.MessageRef, models.MessageRef) {
	t.Helper()
	emails, err := mb.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatalf("GetTimelineEmails: %v", err)
	}
	seen := make(map[string]models.MessageRef)
	for _, email := range emails {
		ref := email.MessageRef()
		if prev, ok := seen[ref.MessageID]; ok && prev.SourceID != ref.SourceID {
			if ref.SourceID == "personal-mail" {
				return prev, ref
			}
			return ref, prev
		}
		seen[ref.MessageID] = ref
	}
	t.Fatalf("demo multi-account mailbox did not expose duplicate message IDs")
	return models.MessageRef{}, models.MessageRef{}
}

func TestDaemonMultiAccountMutationRequiresScopedRef(t *testing.T) {
	mb := backend.NewMultiDemoBackend()
	if err := mb.SwitchAccount(backend.AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}
	workRef, _ := duplicateMessageRefs(t, mb)
	s := &Server{backend: mb}

	req := httptest.NewRequest(http.MethodPost, "/v1/emails/"+url.PathEscape(workRef.MessageID)+"/read?folder=INBOX", nil)
	req.SetPathValue("id", workRef.MessageID)
	rr := httptest.NewRecorder()
	s.handleMarkRead(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected ambiguous multi-account write to return 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDaemonMultiAccountMutationRoutesByLocalID(t *testing.T) {
	mb := backend.NewMultiDemoBackend()
	if err := mb.SwitchAccount(backend.AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}
	workRef, personalRef := duplicateMessageRefs(t, mb)
	if err := mb.MarkUnreadByRef(workRef); err != nil {
		t.Fatalf("MarkUnreadByRef(work): %v", err)
	}
	if err := mb.MarkUnreadByRef(personalRef); err != nil {
		t.Fatalf("MarkUnreadByRef(personal): %v", err)
	}
	s := &Server{backend: mb}

	values := url.Values{}
	values.Set("local_id", personalRef.LocalID)
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/"+url.PathEscape(personalRef.MessageID)+"/read?"+values.Encode(), nil)
	req.SetPathValue("id", personalRef.MessageID)
	rr := httptest.NewRecorder()
	s.handleMarkRead(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected scoped write to succeed, got %d: %s", rr.Code, rr.Body.String())
	}
	workEmail, err := mb.GetEmailByRef(workRef)
	if err != nil {
		t.Fatalf("GetEmailByRef(work): %v", err)
	}
	personalEmail, err := mb.GetEmailByRef(personalRef)
	if err != nil {
		t.Fatalf("GetEmailByRef(personal): %v", err)
	}
	if workEmail.IsRead {
		t.Fatalf("work duplicate was marked read by a personal local_id scoped mutation: %#v", workEmail.MessageRef())
	}
	if !personalEmail.IsRead {
		t.Fatalf("personal duplicate was not marked read by local_id scoped mutation: %#v", personalEmail.MessageRef())
	}
}

func TestDaemonSingleAccountMutationKeepsLegacyMessageIDCompatibility(t *testing.T) {
	b := backend.NewScopedDemoBackend(backend.AccountInfo{SourceID: "default-mail", AccountID: "default"})
	s := &Server{backend: b}

	req := httptest.NewRequest(http.MethodPost, "/v1/emails/demo-welcome-to-herald@demo.local/read?folder=INBOX", nil)
	req.SetPathValue("id", "demo-welcome-to-herald@demo.local")
	rr := httptest.NewRecorder()
	s.handleMarkRead(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("legacy single-account write should still succeed, got %d: %s", rr.Code, rr.Body.String())
	}
}
