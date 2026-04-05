package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Category represents an AI-assigned email category
type Category = string

const (
	CategorySubscription  Category = "sub"
	CategoryNewsletter    Category = "news"
	CategoryImportant     Category = "imp"
	CategoryTransactional Category = "txn"
	CategorySocial        Category = "soc"
	CategorySpam          Category = "spam"
	CategoryUnknown       Category = ""
)

// Classifier uses a local Ollama instance to tag emails
type Classifier struct {
	host           string
	model          string
	embeddingModel string
	client         *http.Client
}

// New creates a Classifier talking to the given Ollama host
func New(host, model string) *Classifier {
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "gemma2:2b"
	}
	return &Classifier{
		host:           strings.TrimRight(host, "/"),
		model:          model,
		embeddingModel: "nomic-embed-text",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetEmbeddingModel overrides the default embedding model
func (c *Classifier) SetEmbeddingModel(model string) {
	if model != "" {
		c.embeddingModel = model
	}
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

// Classify returns a short category tag for an email given its sender + subject.
// Returns CategoryUnknown on any error (so callers can skip gracefully).
func (c *Classifier) Classify(sender, subject string) (Category, error) {
	prompt := fmt.Sprintf(`You are an email tagger. Given the sender and subject below, respond with EXACTLY ONE of these tags and nothing else:

sub   = unsolicited marketing, promotions, deals, offers, gift cards, discounts (even from financial or retail brands)
news  = newsletters or editorial content from a list you chose to subscribe to
imp   = genuinely important: bills/invoices YOU must pay, legal notices, doctor/appointment, direct personal/work email
txn   = transactional: receipts, order confirmations, shipping updates, booking confirmations
soc   = social media notifications
spam  = phishing, scams, or unsolicited junk

Key rule: promotional emails advertising offers (e.g. "gift card", "limited time", "earn rewards", "save X%%") are ALWAYS sub or spam, never imp — even if the sender is a bank or financial service.

Sender: %s
Subject: %s

Tag:`, sender, subject)

	body, err := json.Marshal(generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return CategoryUnknown, err
	}

	resp, err := c.client.Post(c.host+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return CategoryUnknown, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CategoryUnknown, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CategoryUnknown, err
	}

	return normalizeCategory(strings.TrimSpace(result.Response)), nil
}

// ChatMessage is a single turn in a conversation
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall // set on role="assistant" messages that contain tool calls
	ToolCallID string     // set on role="tool" messages (links to the ToolCall.ID)
	ToolName   string     // set on role="tool" messages (the tool that was called)
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Tools    []Tool        `json:"tools,omitempty"`
}

type chatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
}

// Chat sends a multi-turn conversation to Ollama and returns the assistant reply
func (c *Classifier) Chat(messages []ChatMessage) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return "", err
	}
	resp, err := c.client.Post(c.host+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama chat failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Message.Content), nil
}

// ChatWithTools sends a multi-turn conversation with tool definitions to Ollama.
// Returns either a text response OR tool calls (not both).
func (c *Classifier) ChatWithTools(messages []ChatMessage, tools []Tool) (string, []ToolCall, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Tools:    tools,
	})
	if err != nil {
		return "", nil, err
	}
	resp, err := c.client.Post(c.host+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("ollama chat failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, err
	}
	if len(result.Message.ToolCalls) > 0 {
		var calls []ToolCall
		for i, tc := range result.Message.ToolCalls {
			calls = append(calls, ToolCall{
				ID:        fmt.Sprintf("call_%d", i),
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		return "", calls, nil
	}
	return strings.TrimSpace(result.Message.Content), nil, nil
}

// Ping checks whether Ollama is running and the model is available
type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed returns a float32 embedding vector for the given text.
// Uses the embeddingModel (default: nomic-embed-text) via /api/embeddings.
func (c *Classifier) Embed(text string) ([]float32, error) {
	payload := embedRequest{Model: c.embeddingModel, Prompt: text}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Post(c.host+"/api/embeddings", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama /api/embeddings returned %d", resp.StatusCode)
	}
	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode embedding response: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}
	return result.Embedding, nil
}

func (c *Classifier) Ping() error {
	resp, err := c.client.Get(c.host + "/api/tags")
	if err != nil {
		return fmt.Errorf("ollama not reachable at %s: %w", c.host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	return nil
}

// IsVisionCapable returns true if the given model name supports image inputs.
func IsVisionCapable(modelName string) bool {
	lower := strings.ToLower(modelName)
	prefixes := []string{"gemma3", "llava", "bakllava", "moondream", "minicpm-v", "gemma3n"}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// HasVisionModel returns true if the classifier's configured model supports image inputs.
func (c *Classifier) HasVisionModel() bool {
	return IsVisionCapable(c.model)
}

type generateImageRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images"`
	Stream bool     `json:"stream"`
}

// DescribeImage sends an image to the vision model and returns a one-sentence description.
// imageBytes is the raw image data; mimeType is e.g. "image/jpeg".
// Returns an error if the model is not vision-capable or the request fails.
func (c *Classifier) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	if !c.HasVisionModel() {
		return "", fmt.Errorf("model %q does not support vision", c.model)
	}

	b64 := base64.StdEncoding.EncodeToString(imageBytes)
	payload := generateImageRequest{
		Model:  c.model,
		Prompt: "Describe this image in one sentence, focusing on what it shows.",
		Images: []string{b64},
		Stream: false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama image request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned %d", resp.StatusCode)
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Response), nil
}

// EnrichContact asks the model to extract company and topics from a list of
// recent email subjects involving the contact.
// Returns (company, topics, nil) on success, or ("", nil, err) on failure.
// JSON parsing errors are silently treated as ("", nil, nil) so the caller can skip gracefully.
func (c *Classifier) EnrichContact(email string, subjects []string) (company string, topics []string, err error) {
	var sb strings.Builder
	sb.WriteString("Based on these email subjects involving ")
	sb.WriteString(email)
	sb.WriteString(":\n")
	for _, s := range subjects {
		sb.WriteString("- ")
		sb.WriteString(s)
		sb.WriteString("\n")
	}
	sb.WriteString("\nExtract in JSON (respond with JSON only, no explanation):\n")
	sb.WriteString(`{"company": "Acme Corp", "topics": ["billing", "support"]}`)

	reply, err := c.Chat([]ChatMessage{
		{Role: "user", Content: sb.String()},
	})
	if err != nil {
		return "", nil, fmt.Errorf("EnrichContact chat failed: %w", err)
	}

	// Strip markdown code fences if present
	reply = strings.TrimSpace(reply)
	if strings.HasPrefix(reply, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(reply, "\n"); idx >= 0 {
			reply = reply[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(reply, "```"); idx >= 0 {
			reply = reply[:idx]
		}
		reply = strings.TrimSpace(reply)
	}

	var result struct {
		Company string   `json:"company"`
		Topics  []string `json:"topics"`
	}
	if err := json.Unmarshal([]byte(reply), &result); err != nil {
		return "", nil, fmt.Errorf("parse enrichment response: %w", err)
	}
	// Filter out placeholder values the LLM may echo from the prompt example.
	comp := strings.ToLower(strings.TrimSpace(result.Company))
	if comp == "" || comp == "string or empty" || comp == "acme corp" || comp == "unknown" {
		result.Company = ""
	}
	return result.Company, result.Topics, nil
}

// GenerateQuickReplies generates 3 short reply suggestions for the given email.
// Returns a JSON array of strings. On error returns nil (callers fall back to canned replies).
func (c *Classifier) GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error) {
	prompt := fmt.Sprintf(
		"Generate 3 very short (1–2 sentences max) reply options for this email.\nRespond with a JSON array of strings ONLY — no explanation, no markdown.\n\nFrom: %s\nSubject: %s\n\n%s",
		sender, subject, bodyPreview,
	)
	reply, err := c.Chat([]ChatMessage{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, fmt.Errorf("GenerateQuickReplies: %w", err)
	}
	reply = strings.TrimSpace(reply)
	// Strip markdown fences
	if strings.HasPrefix(reply, "```") {
		if idx := strings.Index(reply, "\n"); idx >= 0 {
			reply = reply[idx+1:]
		}
		if idx := strings.LastIndex(reply, "```"); idx >= 0 {
			reply = reply[:idx]
		}
		reply = strings.TrimSpace(reply)
	}
	var suggestions []string
	if err := json.Unmarshal([]byte(reply), &suggestions); err != nil {
		return nil, fmt.Errorf("parse quick replies: %w", err)
	}
	return suggestions, nil
}

// Compile-time interface check
var _ AIClient = (*Classifier)(nil)

func normalizeCategory(raw string) Category {
	raw = strings.ToLower(raw)
	raw = strings.TrimPrefix(raw, "tag:")
	raw = strings.TrimSpace(raw)
	// Accept the first word only (model might add explanation)
	if idx := strings.IndexAny(raw, " \t\n"); idx > 0 {
		raw = raw[:idx]
	}
	switch raw {
	case "sub", "subscription":
		return CategorySubscription
	case "news", "newsletter":
		return CategoryNewsletter
	case "imp", "important":
		return CategoryImportant
	case "txn", "transactional":
		return CategoryTransactional
	case "soc", "social":
		return CategorySocial
	case "spam":
		return CategorySpam
	}
	return CategoryUnknown
}
