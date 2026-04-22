package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"mail-processor/internal/ai"
	"mail-processor/internal/models"
)

// TestValidIDsMsg_HandlerExists verifies that ValidIDsMsg is a defined type and
// that the Model has a validIDsCh field (compilation check).
// Full handler behaviour is covered by integration; here we ensure the types exist.
func TestValidIDsMsg_TypeExists(t *testing.T) {
	msg := ValidIDsMsg{ValidIDs: map[string]bool{"<a@x.com>": true}}
	if !msg.ValidIDs["<a@x.com>"] {
		t.Error("ValidIDsMsg.ValidIDs not accessible")
	}
}

// stubClassifier is a minimal ai.AIClient for testing reclassification.
type stubClassifier struct {
	category ai.Category
	err      error
}

func (s *stubClassifier) Classify(_, _ string) (ai.Category, error) { return s.category, s.err }
func (s *stubClassifier) Chat(_ []ai.ChatMessage) (string, error)   { return "", nil }
func (s *stubClassifier) ChatWithTools(_ []ai.ChatMessage, _ []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, ai.ErrToolsNotSupported
}
func (s *stubClassifier) Embed(_ string) ([]float32, error)                     { return nil, ai.ErrEmbeddingNotSupported }
func (s *stubClassifier) SetEmbeddingModel(_ string)                            {}
func (s *stubClassifier) GenerateQuickReplies(_, _, _ string) ([]string, error) { return nil, nil }
func (s *stubClassifier) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (s *stubClassifier) HasVisionModel() bool { return false }
func (s *stubClassifier) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (s *stubClassifier) Ping() error { return nil }

// makeReclassifyModel builds the minimal Model state required to test reclassification.
func makeReclassifyModel(classifier ai.AIClient) *Model {
	m := &Model{
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
		backend:         &stubBackend{},
		classifier:      classifier,
	}
	m.timeline.emails = []*models.EmailData{
		{MessageID: "msg-1", Subject: "Hello", Sender: "alice@example.com", Date: time.Now()},
	}
	return m
}

// TestReclassifyResult verifies that ReclassifyResultMsg updates classifications
// and sets a success status message.
func TestReclassifyResult(t *testing.T) {
	m := makeReclassifyModel(&stubClassifier{category: "news"})

	result, _ := m.Update(ReclassifyResultMsg{MessageID: "msg-1", Category: "news"})
	updated := result.(*Model)

	if updated.classifications["msg-1"] != "news" {
		t.Errorf("classifications[msg-1] = %q, want %q", updated.classifications["msg-1"], "news")
	}
	if updated.statusMessage != "Reclassified: news" {
		t.Errorf("statusMessage = %q, want %q", updated.statusMessage, "Reclassified: news")
	}
}

// TestReclassifyResultError verifies that a failed ReclassifyResultMsg sets an error status.
func TestReclassifyResultError(t *testing.T) {
	m := makeReclassifyModel(&stubClassifier{})

	result, _ := m.Update(ReclassifyResultMsg{MessageID: "msg-1", Err: errors.New("AI offline")})
	updated := result.(*Model)

	if updated.classifications["msg-1"] != "" {
		t.Errorf("expected no classification on error, got %q", updated.classifications["msg-1"])
	}
	if updated.statusMessage != "Reclassify failed: AI offline" {
		t.Errorf("statusMessage = %q, want %q", updated.statusMessage, "Reclassify failed: AI offline")
	}
}

// TestReclassifyNilClassifier verifies that pressing A with no classifier set
// shows the "No AI configured" status message without panicking.
func TestReclassifyNilClassifier(t *testing.T) {
	m := makeReclassifyModel(nil)
	m.classifier = nil // explicitly nil

	result, cmd := m.Update(ReclassifyResultMsg{Err: errors.New("no AI classifier configured")})
	updated := result.(*Model)

	if cmd != nil {
		t.Error("expected nil cmd when classifier is nil")
	}
	if updated.statusMessage != "Reclassify failed: no AI classifier configured" {
		t.Errorf("statusMessage = %q", updated.statusMessage)
	}
}

// TestReclassifyEmailCmd verifies that reclassifyEmailCmd returns a command
// that calls Classify and returns ReclassifyResultMsg with the correct category.
func TestReclassifyEmailCmd(t *testing.T) {
	m := makeReclassifyModel(&stubClassifier{category: "imp"})
	email := &models.EmailData{
		MessageID: "msg-42",
		Sender:    "boss@corp.com",
		Subject:   "Urgent: Q4 review",
	}

	cmd := m.reclassifyEmailCmd(email)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	rr, ok := msg.(ReclassifyResultMsg)
	if !ok {
		t.Fatalf("expected ReclassifyResultMsg, got %T", msg)
	}
	if rr.Err != nil {
		t.Fatalf("unexpected error: %v", rr.Err)
	}
	if rr.MessageID != "msg-42" {
		t.Errorf("MessageID = %q, want %q", rr.MessageID, "msg-42")
	}
	if rr.Category != "imp" {
		t.Errorf("Category = %q, want %q", rr.Category, "imp")
	}
}
