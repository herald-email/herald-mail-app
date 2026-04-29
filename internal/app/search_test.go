package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type embedErrorClassifier struct{ err error }

func (c *embedErrorClassifier) Chat(_ []ai.ChatMessage) (string, error) { return "", nil }
func (c *embedErrorClassifier) ChatWithTools(_ []ai.ChatMessage, _ []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, ai.ErrToolsNotSupported
}
func (c *embedErrorClassifier) Classify(_, _ string) (ai.Category, error) { return "", nil }
func (c *embedErrorClassifier) Embed(_ string) ([]float32, error)         { return nil, c.err }
func (c *embedErrorClassifier) SetEmbeddingModel(_ string)                {}
func (c *embedErrorClassifier) GenerateQuickReplies(_, _, _ string) ([]string, error) {
	return nil, nil
}
func (c *embedErrorClassifier) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (c *embedErrorClassifier) HasVisionModel() bool { return false }
func (c *embedErrorClassifier) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (c *embedErrorClassifier) Ping() error { return nil }

func TestPerformSearch_HybridMergesKeywordAndSemanticResults(t *testing.T) {
	now := time.Now()
	keywordFirst := &models.EmailData{MessageID: "kw-1", Sender: "alice@example.com", Subject: "SwiftUI setup", Date: now}
	shared := &models.EmailData{MessageID: "shared-1", Sender: "bob@example.com", Subject: "Slow SwiftUI view", Date: now.Add(-time.Hour)}
	semanticOnly := &models.EmailData{MessageID: "sem-1", Sender: "carol@example.com", Subject: "Render performance", Date: now.Add(-2 * time.Hour)}

	backend := &stubBackend{
		searchResult: []*models.EmailData{keywordFirst, shared},
		semanticResults: []*models.SemanticSearchResult{
			{Email: semanticOnly, Score: 0.83},
			{Email: shared, Score: 0.91},
		},
	}
	m := New(backend, nil, "", &stubClassifier{}, false)
	m.currentFolder = "INBOX"

	msg := m.performSearch("swfitui")().(SearchResultMsg)
	if msg.Source != "hybrid" {
		t.Fatalf("expected hybrid source, got %q", msg.Source)
	}
	if len(msg.Emails) != 3 {
		t.Fatalf("expected 3 merged emails, got %d", len(msg.Emails))
	}
	if msg.Emails[0].MessageID != "kw-1" || msg.Emails[1].MessageID != "shared-1" || msg.Emails[2].MessageID != "sem-1" {
		t.Fatalf("unexpected hybrid order: %#v", []string{msg.Emails[0].MessageID, msg.Emails[1].MessageID, msg.Emails[2].MessageID})
	}
	if len(msg.Scores) != 2 {
		t.Fatalf("expected semantic scores for deduped semantic results, got %d", len(msg.Scores))
	}
	if msg.Scores["shared-1"] != 0.91 || msg.Scores["sem-1"] != 0.83 {
		t.Fatalf("unexpected semantic scores: %#v", msg.Scores)
	}
	if backend.lastSemanticLimit != 100 {
		t.Fatalf("expected semantic limit 100, got %d", backend.lastSemanticLimit)
	}
	if backend.lastSemanticMinScore != 0.30 {
		t.Fatalf("expected default min score 0.30, got %.2f", backend.lastSemanticMinScore)
	}
}

func TestPerformSearch_HybridFallsBackToKeywordWhenSemanticUnavailable(t *testing.T) {
	backend := &stubBackend{
		searchResult: []*models.EmailData{
			{MessageID: "kw-1", Sender: "alice@example.com", Subject: "Invoice", Date: time.Now()},
		},
	}
	m := New(backend, nil, "", &embedErrorClassifier{err: errors.New("ollama unavailable")}, false)
	m.currentFolder = "INBOX"

	msg := m.performSearch("invoice")().(SearchResultMsg)
	if msg.Err != nil {
		t.Fatalf("expected graceful keyword fallback, got error %v", msg.Err)
	}
	if len(msg.Emails) != 1 || msg.Emails[0].MessageID != "kw-1" {
		t.Fatalf("expected keyword-only fallback result, got %#v", msg.Emails)
	}
}

func TestPerformSearch_ExplicitSemanticUsesConfiguredThreshold(t *testing.T) {
	backend := &stubBackend{
		semanticResults: []*models.SemanticSearchResult{
			{Email: &models.EmailData{MessageID: "sem-1", Sender: "alice@example.com", Subject: "Lease renewal", Date: time.Now()}, Score: 0.82},
		},
	}
	m := New(backend, nil, "", &stubClassifier{}, false)
	m.currentFolder = "INBOX"
	m.cfg = &config.Config{}
	m.cfg.Semantic.MinScore = 0.77

	msg := m.performSearch("? lease extension")().(SearchResultMsg)
	if msg.Source != "semantic" {
		t.Fatalf("expected semantic source, got %q", msg.Source)
	}
	if backend.lastSemanticLimit != 100 {
		t.Fatalf("expected semantic limit 100, got %d", backend.lastSemanticLimit)
	}
	if backend.lastSemanticMinScore != 0.77 {
		t.Fatalf("expected semantic min score 0.77, got %.2f", backend.lastSemanticMinScore)
	}
}
