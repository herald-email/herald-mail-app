package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// mockClassifier implements ai.AIClient and always returns the given category.
type mockClassifier struct {
	category ai.Category
	err      error
}

func (m *mockClassifier) Classify(sender, subject string) (ai.Category, error) {
	return m.category, m.err
}
func (m *mockClassifier) Chat(_ []ai.ChatMessage) (string, error) { return "", nil }
func (m *mockClassifier) ChatWithTools(_ []ai.ChatMessage, _ []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, nil
}
func (m *mockClassifier) Embed(_ string) ([]float32, error) { return nil, nil }
func (m *mockClassifier) SetEmbeddingModel(_ string)        {}
func (m *mockClassifier) GenerateQuickReplies(_, _, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockClassifier) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (m *mockClassifier) HasVisionModel() bool { return false }
func (m *mockClassifier) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (m *mockClassifier) Ping() error { return nil }

// newTestServer builds a minimal daemon Server wired to an in-memory cache and
// the supplied classifier. The backend field is left nil because handleClassifyFolder
// only uses s.cache and s.classifier.
func newTestServer(t *testing.T, classifier ai.AIClient) (*Server, *cache.Cache) {
	t.Helper()
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	s := &Server{
		cache:      c,
		classifier: classifier,
	}
	return s, c
}

// seedEmail inserts an email into the cache for the given folder.
func seedEmail(t *testing.T, c *cache.Cache, msgID, folder string) {
	t.Helper()
	err := c.CacheEmail(&models.EmailData{
		MessageID: msgID,
		Sender:    "sender@example.com",
		Subject:   "Test subject",
		Folder:    folder,
		Date:      time.Now(),
	})
	if err != nil {
		t.Fatalf("CacheEmail %s: %v", msgID, err)
	}
}

// TestClassifyFolderHandler verifies that:
//   - 3 emails in total (1 already classified, 2 unclassified)
//   - Returns {"classified": 2, "skipped": 1, "total": 3}
func TestClassifyFolderHandler(t *testing.T) {
	cls := &mockClassifier{category: "newsletter"}
	s, c := newTestServer(t, cls)

	const folder = "INBOX"

	// Seed 3 emails.
	seedEmail(t, c, "msg-1", folder)
	seedEmail(t, c, "msg-2", folder)
	seedEmail(t, c, "msg-3", folder)

	// Pre-classify msg-1 so it should be skipped.
	if err := c.SetClassification("msg-1", "work"); err != nil {
		t.Fatalf("SetClassification: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"folder": folder})
	req := httptest.NewRequest(http.MethodPost, "/v1/classify/folder", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleClassifyFolder(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]int
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result["classified"] != 2 {
		t.Errorf("classified: want 2, got %d", result["classified"])
	}
	if result["skipped"] != 1 {
		t.Errorf("skipped: want 1, got %d", result["skipped"])
	}
	if result["total"] != 3 {
		t.Errorf("total: want 3, got %d", result["total"])
	}
}

// TestClassifyFolderHandler_MissingFolder verifies that an empty folder returns 400.
func TestClassifyFolderHandler_MissingFolder(t *testing.T) {
	cls := &mockClassifier{category: "newsletter"}
	s, _ := newTestServer(t, cls)

	req := httptest.NewRequest(http.MethodPost, "/v1/classify/folder", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleClassifyFolder(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// TestClassifyFolderHandler_NoClassifier verifies that a nil classifier returns 503.
func TestClassifyFolderHandler_NoClassifier(t *testing.T) {
	s, _ := newTestServer(t, nil)

	body, _ := json.Marshal(map[string]string{"folder": "INBOX"})
	req := httptest.NewRequest(http.MethodPost, "/v1/classify/folder", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleClassifyFolder(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}
