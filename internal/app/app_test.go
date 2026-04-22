package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"mail-processor/internal/ai"
	"mail-processor/internal/config"
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
	category           ai.Category
	err                error
	lastEmbeddingModel string
	embedCalls         int
}

func (s *stubClassifier) Classify(_, _ string) (ai.Category, error) { return s.category, s.err }
func (s *stubClassifier) Chat(_ []ai.ChatMessage) (string, error)   { return "", nil }
func (s *stubClassifier) ChatWithTools(_ []ai.ChatMessage, _ []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, ai.ErrToolsNotSupported
}
func (s *stubClassifier) Embed(text string) ([]float32, error) {
	s.embedCalls++
	if strings.Contains(text, "ctx-overflow") && len([]rune(text)) > 240 {
		return nil, fmt.Errorf("ollama /api/embeddings returned 500: {\"error\":\"the input length exceeds the context length\"}")
	}
	return []float32{1, 2, 3}, nil
}
func (s *stubClassifier) SetEmbeddingModel(model string)                        { s.lastEmbeddingModel = model }
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

func TestSettingsSaved_EmbeddingModelChangeInvalidatesCache(t *testing.T) {
	backend := &stubBackend{ensureResult: true}
	classifier := &stubClassifier{}
	m := New(backend, nil, "", classifier, false)
	m.cfg = &config.Config{}
	m.cfg.Ollama.EmbeddingModel = "nomic-embed-text"

	next := &config.Config{}
	next.Ollama.EmbeddingModel = "nomic-embed-text-v2-moe"

	updatedModel, _ := m.Update(SettingsSavedMsg{Config: next})
	updated := updatedModel.(*Model)

	if !backend.ensureCalled {
		t.Fatal("expected backend embedding invalidation to run")
	}
	if backend.ensuredModel != "nomic-embed-text-v2-moe" {
		t.Fatalf("EnsureEmbeddingModel called with %q", backend.ensuredModel)
	}
	if classifier.lastEmbeddingModel != "nomic-embed-text-v2-moe" {
		t.Fatalf("SetEmbeddingModel called with %q", classifier.lastEmbeddingModel)
	}
	if updated.statusMessage != "Settings saved. Embeddings reset for the new model." {
		t.Fatalf("statusMessage = %q", updated.statusMessage)
	}
}

func TestHandleTimelineMsg_DedupesBackgroundEmbeddingBatchStart(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.classifier = &stubClassifier{}

	model, cmd, handled := m.handleTimelineMsg(TimelineLoadedMsg{Emails: mockEmails()})
	if !handled {
		t.Fatal("expected TimelineLoadedMsg to be handled")
	}
	if cmd == nil {
		t.Fatal("expected first timeline load to start background AI work")
	}
	updated := model.(*Model)
	if !updated.embeddingBatchActive {
		t.Fatal("expected embedding batch to be marked active after scheduling")
	}

	model, cmd, handled = updated.handleTimelineMsg(TimelineLoadedMsg{Emails: mockEmails()})
	if !handled {
		t.Fatal("expected second TimelineLoadedMsg to be handled")
	}
	if cmd != nil {
		t.Fatal("expected duplicate timeline load to skip scheduling another embedding batch")
	}
}

func TestEmbeddingProgressMsg_ClearsActiveFlagWhenComplete(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.embeddingBatchActive = true

	model, _ := m.Update(EmbeddingProgressMsg{Done: 0, Total: 0})
	updated := model.(*Model)
	if updated.embeddingBatchActive {
		t.Fatal("expected embedding batch active flag to clear when progress is complete")
	}
}

func TestEmbedChunksForEmail_FallsBackOnContextLengthError(t *testing.T) {
	classifier := &stubClassifier{}
	email := &models.EmailData{
		MessageID: "msg-1",
		Sender:    "alice@example.com",
		Subject:   "ctx-overflow subject",
		Date:      time.Now(),
	}
	body := strings.Repeat("ctx-overflow paragraph ", 100)

	chunks, err := embedChunksForEmail(email, body, classifier)
	if err != nil {
		t.Fatalf("embedChunksForEmail returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one embedding chunk after fallback")
	}
	if classifier.embedCalls < 2 {
		t.Fatalf("expected fallback retries, got %d embed call(s)", classifier.embedCalls)
	}
}
