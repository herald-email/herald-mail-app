package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mail-processor/internal/backend"
)

func newFolderTestServer(t *testing.T) *Server {
	t.Helper()
	b := backend.NewDemoBackend()
	return &Server{backend: b}
}

func TestRegisterRoutes_RoutesFolderRenameWithoutPanic(t *testing.T) {
	s := newFolderTestServer(t)
	mux := http.NewServeMux()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registerRoutes panicked: %v", r)
		}
	}()
	s.registerRoutes(mux)

	for _, path := range []string{
		"/v1/folders/OldFolder/rename",
		"/v1/folders/Work%2FProjects/rename",
	} {
		body, _ := json.Marshal(map[string]string{"new_name": "Archive/2025"})
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d — body: %s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestHandleCreateFolder(t *testing.T) {
	s := newFolderTestServer(t)

	body, _ := json.Marshal(map[string]string{"name": "Work/Projects"})
	req := httptest.NewRequest(http.MethodPost, "/v1/folders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleCreateFolder(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] == "" {
		t.Errorf("expected non-empty message field, got: %v", result)
	}
}

func TestHandleCreateFolder_MissingName(t *testing.T) {
	s := newFolderTestServer(t)

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/v1/folders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleCreateFolder(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleRenameFolder(t *testing.T) {
	s := newFolderTestServer(t)

	body, _ := json.Marshal(map[string]string{"new_name": "Archive/2025"})
	req := httptest.NewRequest(http.MethodPost, "/v1/folders/OldFolder/rename", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "OldFolder")
	rr := httptest.NewRecorder()

	s.handleRenameFolder(rr, req)

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

func TestHandleRenameFolder_MissingNewName(t *testing.T) {
	s := newFolderTestServer(t)

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/v1/folders/OldFolder/rename", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "OldFolder")
	rr := httptest.NewRecorder()

	s.handleRenameFolder(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleDeleteFolder(t *testing.T) {
	s := newFolderTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/v1/folders/OldFolder", nil)
	req.SetPathValue("name", "OldFolder")
	rr := httptest.NewRecorder()

	s.handleDeleteFolder(rr, req)

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

func TestHandleSyncAllFolders(t *testing.T) {
	s := newFolderTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/sync/all", nil)
	rr := httptest.NewRecorder()

	s.handleSyncAllFolders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] == "" {
		t.Errorf("expected non-empty message field, got: %v", result)
	}
}

func TestHandleGetSyncStatus(t *testing.T) {
	s := newFolderTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/sync/status", nil)
	rr := httptest.NewRecorder()

	s.handleGetSyncStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	// DemoBackend returns nil map which encodes as "null" — valid JSON object response
	if rr.Body.Len() == 0 {
		t.Errorf("expected non-empty response body")
	}
}
