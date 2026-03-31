package ai

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrEmbeddingNotSupported is returned by AI backends that do not support
// text embedding (e.g. Claude, OpenAI chat-only configurations).
var ErrEmbeddingNotSupported = errors.New("embedding not supported by this backend")

// ErrToolsNotSupported is returned by ChatWithTools when tools are not supported.
var ErrToolsNotSupported = errors.New("tool use not supported by this backend")

// Tool describes a callable function for AI tool use.
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  ToolParams `json:"parameters"`
}

type ToolParams struct {
	Type       string              `json:"type"` // "object"
	Properties map[string]ToolProp `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type ToolProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolCall is returned when the AI wants to call a function.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// AIClient is the common interface implemented by all AI backends.
// Callers are responsible for adding nomic-embed-text prefixes
// (search_document: / search_query:) before calling Embed().
type AIClient interface {
	// Chat sends a multi-turn conversation and returns the assistant reply.
	Chat(messages []ChatMessage) (string, error)
	// ChatWithTools sends messages with available tools. Returns either a text
	// response OR tool calls (not both). Returns ErrToolsNotSupported if the
	// backend doesn't support tools.
	ChatWithTools(messages []ChatMessage, tools []Tool) (response string, calls []ToolCall, err error)
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
