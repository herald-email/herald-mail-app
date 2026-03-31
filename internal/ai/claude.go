package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClaudeClient implements AIClient using the Anthropic Claude Messages API.
type ClaudeClient struct {
	apiKey  string
	model   string
	baseURL string // defaults to "https://api.anthropic.com"; overridable for tests
	client  *http.Client
}

// NewClaudeClient creates a ClaudeClient.
// model defaults to "claude-sonnet-4-6" if empty.
func NewClaudeClient(apiKey, model string) *ClaudeClient {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &ClaudeClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.anthropic.com",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// SetEmbeddingModel is a no-op for Claude (no embedding support).
func (c *ClaudeClient) SetEmbeddingModel(_ string) {}

// Embed is not supported by Claude; returns ErrEmbeddingNotSupported.
func (c *ClaudeClient) Embed(_ string) ([]float32, error) {
	return nil, ErrEmbeddingNotSupported
}

// HasVisionModel always returns true for Claude (all models support vision).
func (c *ClaudeClient) HasVisionModel() bool { return true }

type claudeContentBlock struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *claudeImgSource `json:"source,omitempty"`
}

type claudeImgSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// claudeTextContent marshals a plain text content array to json.RawMessage.
func claudeTextContent(text string) json.RawMessage {
	b, _ := json.Marshal([]claudeContentBlock{{Type: "text", Text: text}})
	return json.RawMessage(b)
}

// claudeBlocksContent marshals an arbitrary slice of content blocks.
func claudeBlocksContent(blocks []claudeContentBlock) json.RawMessage {
	b, _ := json.Marshal(blocks)
	return json.RawMessage(b)
}

type claudeTool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema ToolParams `json:"input_schema"`
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []claudeMessage `json:"messages"`
	Tools     []claudeTool    `json:"tools,omitempty"`
}

type claudeResponseBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type claudeResponse struct {
	Content []claudeResponseBlock `json:"content"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *ClaudeClient) doRequest(ctx context.Context, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("claude request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var result claudeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode claude response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("claude error: %s", result.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude returned %d", resp.StatusCode)
	}
	for _, block := range result.Content {
		if block.Type == "text" {
			return strings.TrimSpace(block.Text), nil
		}
	}
	return "", fmt.Errorf("no text block in claude response")
}

// Chat sends a multi-turn conversation. System messages are promoted to the
// top-level "system" field per the Claude API spec.
func (c *ClaudeClient) Chat(messages []ChatMessage) (string, error) {
	var systemPrompt string
	var claudeMsgs []claudeMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		claudeMsgs = append(claudeMsgs, claudeMessage{
			Role:    m.Role,
			Content: claudeTextContent(m.Content),
		})
	}
	if len(claudeMsgs) == 0 {
		return "", fmt.Errorf("no non-system messages")
	}
	payload := claudeRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  claudeMsgs,
	}
	return c.doRequest(context.Background(), payload)
}

// ChatWithTools sends a conversation with tool definitions to Claude.
// Returns either a text response OR tool calls (not both).
func (c *ClaudeClient) ChatWithTools(messages []ChatMessage, tools []Tool) (string, []ToolCall, error) {
	var systemPrompt string
	var claudeMsgs []claudeMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		switch m.Role {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				// Build tool_use content blocks for each call
				var blocks []map[string]any
				for _, tc := range m.ToolCalls {
					blocks = append(blocks, map[string]any{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Name,
						"input": json.RawMessage(tc.Arguments),
					})
				}
				b, _ := json.Marshal(blocks)
				claudeMsgs = append(claudeMsgs, claudeMessage{
					Role:    "assistant",
					Content: json.RawMessage(b),
				})
			} else {
				claudeMsgs = append(claudeMsgs, claudeMessage{
					Role:    "assistant",
					Content: claudeTextContent(m.Content),
				})
			}
		case "tool":
			// Claude expects tool results as role="user" with a tool_result block
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     m.Content,
			}
			b, _ := json.Marshal([]any{block})
			claudeMsgs = append(claudeMsgs, claudeMessage{
				Role:    "user",
				Content: json.RawMessage(b),
			})
		default:
			claudeMsgs = append(claudeMsgs, claudeMessage{
				Role:    m.Role,
				Content: claudeTextContent(m.Content),
			})
		}
	}
	if len(claudeMsgs) == 0 {
		return "", nil, fmt.Errorf("no non-system messages")
	}

	var claudeTools []claudeTool
	for _, t := range tools {
		claudeTools = append(claudeTools, claudeTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	payload := claudeRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  claudeMsgs,
		Tools:     claudeTools,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("claude request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var result claudeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", nil, fmt.Errorf("decode claude response: %w", err)
	}
	if result.Error != nil {
		return "", nil, fmt.Errorf("claude error: %s", result.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("claude returned %d", resp.StatusCode)
	}

	// Check for tool_use blocks
	var calls []ToolCall
	var textParts []string
	for _, block := range result.Content {
		switch block.Type {
		case "tool_use":
			argBytes := block.Input
			if argBytes == nil {
				argBytes = json.RawMessage("{}")
			}
			calls = append(calls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: argBytes,
			})
		case "text":
			textParts = append(textParts, block.Text)
		}
	}
	if len(calls) > 0 {
		return "", calls, nil
	}
	return strings.TrimSpace(strings.Join(textParts, "")), nil, nil
}

// Classify asks Claude to tag an email with a single category label.
func (c *ClaudeClient) Classify(sender, subject string) (Category, error) {
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

	reply, err := c.Chat([]ChatMessage{{Role: "user", Content: prompt}})
	if err != nil {
		return CategoryUnknown, err
	}
	return normalizeCategory(strings.TrimSpace(reply)), nil
}

// GenerateQuickReplies asks Claude for 3 short reply options.
func (c *ClaudeClient) GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error) {
	prompt := fmt.Sprintf(
		"Generate 3 very short (1–2 sentences max) reply options for this email.\nRespond with a JSON array of strings ONLY — no explanation, no markdown.\n\nFrom: %s\nSubject: %s\n\n%s",
		sender, subject, bodyPreview,
	)
	reply, err := c.Chat([]ChatMessage{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, err
	}
	reply = stripMarkdownFences(reply)
	var suggestions []string
	if err := json.Unmarshal([]byte(reply), &suggestions); err != nil {
		return nil, fmt.Errorf("parse quick replies: %w", err)
	}
	return suggestions, nil
}

// EnrichContact extracts company and topics from a list of email subjects.
func (c *ClaudeClient) EnrichContact(email string, subjects []string) (string, []string, error) {
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
	sb.WriteString(`{"company": "string or empty", "topics": ["topic1", "topic2"]}`)

	reply, err := c.Chat([]ChatMessage{{Role: "user", Content: sb.String()}})
	if err != nil {
		return "", nil, err
	}
	reply = stripMarkdownFences(reply)
	var result struct {
		Company string   `json:"company"`
		Topics  []string `json:"topics"`
	}
	if err := json.Unmarshal([]byte(reply), &result); err != nil {
		return "", nil, fmt.Errorf("parse enrichment: %w", err)
	}
	return result.Company, result.Topics, nil
}

// DescribeImage sends an image to Claude Vision and returns a description.
func (c *ClaudeClient) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(imageBytes)
	payload := claudeRequest{
		Model:     c.model,
		MaxTokens: 256,
		Messages: []claudeMessage{
			{
				Role: "user",
				Content: claudeBlocksContent([]claudeContentBlock{
					{
						Type: "image",
						Source: &claudeImgSource{
							Type:      "base64",
							MediaType: mimeType,
							Data:      b64,
						},
					},
					{Type: "text", Text: "Describe this image in one sentence, focusing on what it shows."},
				}),
			},
		},
	}
	return c.doRequest(ctx, payload)
}

// Ping checks Claude API reachability by sending a minimal request.
func (c *ClaudeClient) Ping() error {
	_, err := c.Chat([]ChatMessage{{Role: "user", Content: "ping"}})
	return err
}

// stripMarkdownFences removes opening and closing ``` fences from a string.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}
