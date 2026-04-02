package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"mail-processor/internal/backend"
	"mail-processor/internal/models"
)

// newReplyTestServer builds a daemon Server wired to a DemoBackend for reply/forward/attachment tests.
func newReplyTestServer(t *testing.T) *Server {
	t.Helper()
	b := backend.NewDemoBackend()
	return &Server{backend: b}
}

// attachmentBackend is a test backend that returns a fixed attachment for GetAttachment.
type attachmentBackend struct {
	backend.Backend // embed DemoBackend to satisfy all other methods
	data            []byte
	filename        string
	mimeType        string
}

func newAttachmentBackend(data []byte, filename, mimeType string) *attachmentBackend {
	return &attachmentBackend{
		Backend:  backend.NewDemoBackend(),
		data:     data,
		filename: filename,
		mimeType: mimeType,
	}
}

func (b *attachmentBackend) ListAttachments(messageID string) ([]models.Attachment, error) {
	return []models.Attachment{
		{Filename: b.filename, MIMEType: b.mimeType, Size: len(b.data)},
	}, nil
}

func (b *attachmentBackend) GetAttachment(messageID, filename string) (*models.Attachment, error) {
	if filename != b.filename {
		return nil, fmt.Errorf("attachment %q not found", filename)
	}
	return &models.Attachment{
		Filename: b.filename,
		MIMEType: b.mimeType,
		Size:     len(b.data),
		Data:     b.data,
	}, nil
}

func TestHandleReplyEmail(t *testing.T) {
	s := newReplyTestServer(t)

	body, _ := json.Marshal(map[string]string{"body": "Thanks for your email!"})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/msg123/reply", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "msg123")
	rr := httptest.NewRecorder()

	s.handleReplyEmail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] != "Reply sent" {
		t.Errorf("expected 'Reply sent', got %q", result["message"])
	}
}

func TestHandleReplyEmail_BadBody(t *testing.T) {
	s := newReplyTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/emails/msg123/reply", bytes.NewReader([]byte("not-json")))
	req.SetPathValue("id", "msg123")
	rr := httptest.NewRecorder()

	s.handleReplyEmail(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleForwardEmail(t *testing.T) {
	s := newReplyTestServer(t)

	body, _ := json.Marshal(map[string]string{"to": "friend@example.com", "body": "FYI"})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/msg123/forward", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "msg123")
	rr := httptest.NewRecorder()

	s.handleForwardEmail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["message"] != "Forwarded" {
		t.Errorf("expected 'Forwarded', got %q", result["message"])
	}
}

func TestHandleForwardEmail_MissingTo(t *testing.T) {
	s := newReplyTestServer(t)

	body, _ := json.Marshal(map[string]string{"body": "no recipient"})
	req := httptest.NewRequest(http.MethodPost, "/v1/emails/msg123/forward", bytes.NewReader(body))
	req.SetPathValue("id", "msg123")
	rr := httptest.NewRecorder()

	s.handleForwardEmail(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleListAttachments_Empty(t *testing.T) {
	s := newReplyTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/emails/msg123/attachments", nil)
	req.SetPathValue("id", "msg123")
	rr := httptest.NewRecorder()

	s.handleListAttachments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
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

func TestHandleListAttachments_WithData(t *testing.T) {
	b := newAttachmentBackend([]byte("pdfcontent"), "report.pdf", "application/pdf")
	s := &Server{backend: b}

	req := httptest.NewRequest(http.MethodGet, "/v1/emails/msg123/attachments", nil)
	req.SetPathValue("id", "msg123")
	rr := httptest.NewRecorder()

	s.handleListAttachments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var items []struct {
		Filename string `json:"filename"`
		MIMEType string `json:"mimeType"`
		Size     int    `json:"size"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(items))
	}
	if items[0].Filename != "report.pdf" {
		t.Errorf("expected filename 'report.pdf', got %q", items[0].Filename)
	}
	if items[0].MIMEType != "application/pdf" {
		t.Errorf("expected mimeType 'application/pdf', got %q", items[0].MIMEType)
	}
}

func TestHandleGetAttachment_Base64(t *testing.T) {
	rawData := []byte("hello attachment")
	b := newAttachmentBackend(rawData, "hello.txt", "text/plain")
	s := &Server{backend: b}

	req := httptest.NewRequest(http.MethodGet, "/v1/emails/msg123/attachments/hello.txt", nil)
	req.SetPathValue("id", "msg123")
	req.SetPathValue("filename", "hello.txt")
	rr := httptest.NewRecorder()

	s.handleGetAttachment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	dataStr, ok := result["data"].(string)
	if !ok {
		t.Fatalf("expected 'data' field as string, got %T", result["data"])
	}
	decoded, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if string(decoded) != string(rawData) {
		t.Errorf("expected %q, got %q", string(rawData), string(decoded))
	}
}

func TestHandleGetAttachment_SaveToPath(t *testing.T) {
	rawData := []byte("file content")
	b := newAttachmentBackend(rawData, "doc.txt", "text/plain")
	s := &Server{backend: b}

	tmpFile, err := os.CreateTemp("", "attachment-test-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	url := fmt.Sprintf("/v1/emails/msg123/attachments/doc.txt?dest_path=%s", tmpFile.Name())
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("id", "msg123")
	req.SetPathValue("filename", "doc.txt")
	rr := httptest.NewRecorder()

	s.handleGetAttachment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["path"] != tmpFile.Name() {
		t.Errorf("expected path %q, got %q", tmpFile.Name(), result["path"])
	}

	// Verify file was written
	written, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(written) != string(rawData) {
		t.Errorf("file content mismatch: expected %q, got %q", string(rawData), string(written))
	}
}
