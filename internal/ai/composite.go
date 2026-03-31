package ai

import "context"

// CompositeClient routes most calls to a primary AIClient but delegates
// Embed() to an optional secondary (e.g. Ollama) for asymmetric embedding.
// This lets callers use Claude or OpenAI for chat while keeping Ollama for
// semantic search embeddings.
type CompositeClient struct {
	chat      AIClient // required: primary backend (Claude or OpenAI)
	embedding AIClient // optional: embedding backend (Ollama); nil = use chat.Embed()
}

// NewCompositeClient creates a CompositeClient.
// embedding may be nil; in that case Embed() delegates to chat.
func NewCompositeClient(chat, embedding AIClient) *CompositeClient {
	return &CompositeClient{chat: chat, embedding: embedding}
}

func (c *CompositeClient) Chat(messages []ChatMessage) (string, error) {
	return c.chat.Chat(messages)
}

func (c *CompositeClient) ChatWithTools(messages []ChatMessage, tools []Tool) (string, []ToolCall, error) {
	return c.chat.ChatWithTools(messages, tools)
}

func (c *CompositeClient) Classify(sender, subject string) (Category, error) {
	return c.chat.Classify(sender, subject)
}

// Embed delegates to the embedding backend if set, otherwise to the primary.
func (c *CompositeClient) Embed(text string) ([]float32, error) {
	if c.embedding != nil {
		return c.embedding.Embed(text)
	}
	return c.chat.Embed(text)
}

func (c *CompositeClient) SetEmbeddingModel(model string) {
	if c.embedding != nil {
		c.embedding.SetEmbeddingModel(model)
	} else {
		c.chat.SetEmbeddingModel(model)
	}
}

func (c *CompositeClient) GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error) {
	return c.chat.GenerateQuickReplies(sender, subject, bodyPreview)
}

func (c *CompositeClient) EnrichContact(email string, subjects []string) (string, []string, error) {
	return c.chat.EnrichContact(email, subjects)
}

func (c *CompositeClient) HasVisionModel() bool {
	return c.chat.HasVisionModel()
}

func (c *CompositeClient) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	return c.chat.DescribeImage(ctx, imageBytes, mimeType)
}

func (c *CompositeClient) Ping() error {
	return c.chat.Ping()
}

// Compile-time checks
var _ AIClient = (*CompositeClient)(nil)
var _ AIClient = (*ClaudeClient)(nil)
var _ AIClient = (*OpenAICompatClient)(nil)
