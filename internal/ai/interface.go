package ai

import (
	"context"
	"errors"
)

// ErrEmbeddingNotSupported is returned by AI backends that do not support
// text embedding (e.g. Claude, OpenAI chat-only configurations).
var ErrEmbeddingNotSupported = errors.New("embedding not supported by this backend")

// AIClient is the common interface implemented by all AI backends.
// Callers are responsible for adding nomic-embed-text prefixes
// (search_document: / search_query:) before calling Embed().
type AIClient interface {
	// Chat sends a multi-turn conversation and returns the assistant reply.
	Chat(messages []ChatMessage) (string, error)
	// Classify returns a short category tag for an email.
	Classify(sender, subject string) (Category, error)
	// Embed returns a float32 embedding vector. May return ErrEmbeddingNotSupported.
	Embed(text string) ([]float32, error)
	// SetEmbeddingModel overrides the default embedding model name.
	SetEmbeddingModel(model string)
	// GenerateQuickReplies returns 3 short reply suggestions.
	GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error)
	// EnrichContact extracts company and topics from email subjects.
	EnrichContact(email string, subjects []string) (company string, topics []string, err error)
	// HasVisionModel returns true if the backend supports image description.
	HasVisionModel() bool
	// DescribeImage returns a one-sentence image description.
	DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error)
	// Ping checks whether the backend is reachable and ready.
	Ping() error
}
