package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
)

func newBulkTestServer(t *testing.T) *Server {
	t.Helper()
	b := backend.NewDemoBackend()
	return &Server{backend: b}
}

func TestHandleDeleteThread(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]string{
		"folder":  "INBOX",
		"subject": "Test thread",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/threads/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleDeleteThread(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] == "" {
		t.Errorf("expected non-empty message field, got: %v", result)
	}
}

func TestHandleDeleteThread_MissingFields(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]string{"folder": "INBOX"}) // missing subject
	req := httptest.NewRequest(http.MethodPost, "/v1/threads/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleDeleteThread(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleArchiveThread(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]string{
		"folder":  "INBOX",
		"subject": "Newsletter subject",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/threads/archive", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleArchiveThread(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleBulkDelete(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"message_ids": []string{"id1", "id2", "id3"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/bulk-delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleBulkDelete(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] == "" {
		t.Errorf("expected non-empty message, got: %v", result)
	}
}

func TestHandleBulkDelete_EmptyIDs(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]any{"message_ids": []string{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/bulk-delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleBulkDelete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleBulkMove(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"message_ids": []string{"id1", "id2"},
		"to_folder":   "Archive",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/bulk-move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleBulkMove(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] == "" {
		t.Errorf("expected non-empty message, got: %v", result)
	}
}

func TestHandleBulkMove_MissingFolder(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]any{
		"message_ids": []string{"id1"},
		// to_folder missing
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/bulk-move", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleBulkMove(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleArchiveSender(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]string{"folder": "INBOX"})
	req := httptest.NewRequest(http.MethodPost, "/v1/senders/newsletter@example.com/archive", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("sender", "newsletter@example.com")
	rr := httptest.NewRecorder()

	s.handleArchiveSender(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUnsubscribeSender(t *testing.T) {
	s := newBulkTestServer(t)

	// DemoBackend no-ops UnsubscribeSender, so we just verify 200
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/msg123/unsubscribe", nil)
	req.SetPathValue("id", "msg123")
	rr := httptest.NewRecorder()

	s.handleUnsubscribeSender(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] != "Unsubscribed" {
		t.Errorf("expected 'Unsubscribed', got: %q", result["message"])
	}
}

func TestHandleSoftUnsubscribeSender(t *testing.T) {
	s := newBulkTestServer(t)

	body, _ := json.Marshal(map[string]string{"to_folder": "Disabled Subscriptions"})
	req := httptest.NewRequest(http.MethodPost, "/v1/senders/news@example.com/soft-unsubscribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("sender", "news@example.com")
	rr := httptest.NewRecorder()

	s.handleSoftUnsubscribeSender(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] != "Rule created" {
		t.Errorf("expected 'Rule created', got: %q", result["message"])
	}
}
