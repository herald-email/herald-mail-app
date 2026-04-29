package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
)

// newDraftTestServer builds a minimal daemon Server wired to a DemoBackend.
func newDraftTestServer(t *testing.T) *Server {
	t.Helper()
	b := backend.NewDemoBackend()
	s := &Server{
		backend: b,
	}
	return s
}

func TestHandleSaveDraft(t *testing.T) {
	s := newDraftTestServer(t)

	body, _ := json.Marshal(map[string]string{
		"to":      "recipient@example.com",
		"subject": "Hello Draft",
		"body":    "Draft body text",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/drafts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleSaveDraft(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := result["uid"]; !ok {
		t.Errorf("response missing 'uid' field: %v", result)
	}
	if _, ok := result["folder"]; !ok {
		t.Errorf("response missing 'folder' field: %v", result)
	}
}

func TestHandleListDrafts(t *testing.T) {
	s := newDraftTestServer(t)

	// Save a draft first
	saveBody, _ := json.Marshal(map[string]string{
		"to":      "test@example.com",
		"subject": "My Draft",
		"body":    "Draft content",
	})
	saveReq := httptest.NewRequest(http.MethodPost, "/v1/drafts", bytes.NewReader(saveBody))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRR := httptest.NewRecorder()
	s.handleSaveDraft(saveRR, saveReq)
	if saveRR.Code != http.StatusOK {
		t.Fatalf("save draft failed: %d — %s", saveRR.Code, saveRR.Body.String())
	}

	// Now list drafts
	listReq := httptest.NewRequest(http.MethodGet, "/v1/drafts", nil)
	listRR := httptest.NewRecorder()
	s.handleListDrafts(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", listRR.Code, listRR.Body.String())
	}

	var drafts []map[string]any
	if err := json.NewDecoder(listRR.Body).Decode(&drafts); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(drafts) != 1 {
		t.Errorf("expected 1 draft, got %d", len(drafts))
	}
}

func TestHandleListDrafts_EmptyIsArray(t *testing.T) {
	s := newDraftTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/drafts", nil)
	rr := httptest.NewRecorder()
	s.handleListDrafts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Must be a JSON array, not null
	var result any
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := result.([]any); !ok {
		t.Errorf("expected JSON array, got %T: %v", result, result)
	}
}

func TestHandleDeleteDraft(t *testing.T) {
	s := newDraftTestServer(t)

	// Save a draft to get a UID
	saveBody, _ := json.Marshal(map[string]string{
		"to":      "del@example.com",
		"subject": "To Delete",
		"body":    "will be deleted",
	})
	saveReq := httptest.NewRequest(http.MethodPost, "/v1/drafts", bytes.NewReader(saveBody))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRR := httptest.NewRecorder()
	s.handleSaveDraft(saveRR, saveReq)
	if saveRR.Code != http.StatusOK {
		t.Fatalf("save draft failed: %d", saveRR.Code)
	}

	var saved map[string]any
	if err := json.NewDecoder(saveRR.Body).Decode(&saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	uid := saved["uid"]

	// DELETE /v1/drafts/{uid}?folder=Drafts
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/v1/drafts/%v?folder=Drafts", uid), nil)
	delReq.SetPathValue("uid", fmt.Sprintf("%v", uid))
	delRR := httptest.NewRecorder()
	s.handleDeleteDraft(delRR, delReq)

	if delRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d — body: %s", delRR.Code, delRR.Body.String())
	}

	// Verify the draft is gone
	listReq := httptest.NewRequest(http.MethodGet, "/v1/drafts", nil)
	listRR := httptest.NewRecorder()
	s.handleListDrafts(listRR, listReq)

	var drafts []any
	json.NewDecoder(listRR.Body).Decode(&drafts)
	if len(drafts) != 0 {
		t.Errorf("expected 0 drafts after delete, got %d", len(drafts))
	}
}
