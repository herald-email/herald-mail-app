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

// OpenAICompatClient implements AIClient using OpenAI-compatible APIs.
// Works with OpenAI, LM Studio, llama.cpp, and any /v1/chat/completions endpoint.
type OpenAICompatClient struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewOpenAICompatClient creates an OpenAICompatClient.
// baseURL defaults to "https://api.openai.com/v1" if empty.
// model defaults to "gpt-4o" if empty.
func NewOpenAICompatClient(apiKey, baseURL, model string) *OpenAICompatClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAICompatClient{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// SetEmbeddingModel is a no-op; OpenAI uses "text-embedding-3-small" hardcoded.
func (c *OpenAICompatClient) SetEmbeddingModel(_ string) {}

// HasVisionModel returns true (OpenAI gpt-4o supports vision).
func (c *OpenAICompatClient) HasVisionModel() bool { return true }

type openAIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []openAIContentBlock
}

type openAIContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAITool struct {
	Type     string         `json:"type"` // "function"
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  ToolParams `json:"parameters"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type openAIChatRequest struct {
	Model    string       `json:"model"`
	Messages []any        `json:"messages"`
	Tools    []openAITool `json:"tools,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type openAIEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompatClient) doPost(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// Chat sends a multi-turn conversation using /v1/chat/completions.
func (c *OpenAICompatClient) Chat(messages []ChatMessage) (string, error) {
	var oaMsgs []any
	for _, m := range messages {
		oaMsgs = append(oaMsgs, openAIMessage{Role: m.Role, Content: m.Content})
	}
	respBody, err := c.doPost(context.Background(), "/chat/completions", openAIChatRequest{
		Model:    c.model,
		Messages: oaMsgs,
	})
	if err != nil {
		return "", err
	}
	var result openAIChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("openai error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in openai response")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// openAIToolCallOut is the tool_call object sent inside an assistant message.
type openAIToolCallOut struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON-encoded string
	} `json:"function"`
}

// openAIAssistantMsg is an assistant message that may include tool_calls.
type openAIAssistantMsg struct {
	Role      string              `json:"role"`
	Content   string              `json:"content,omitempty"`
	ToolCalls []openAIToolCallOut `json:"tool_calls,omitempty"`
}

// openAIToolResultMsg is a tool result message.
type openAIToolResultMsg struct {
	Role       string `json:"role"`        // "tool"
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ChatWithTools sends a conversation with tool definitions using /v1/chat/completions.
// Returns either a text response OR tool calls (not both).
func (c *OpenAICompatClient) ChatWithTools(messages []ChatMessage, tools []Tool) (string, []ToolCall, error) {
	// Build a []any so we can mix different message struct shapes.
	var oaMsgs []any
	for _, m := range messages {
		switch m.Role {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var tcs []openAIToolCallOut
				for _, tc := range m.ToolCalls {
					tcs = append(tcs, openAIToolCallOut{
						ID:   tc.ID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      tc.Name,
							Arguments: string(tc.Arguments),
						},
					})
				}
				oaMsgs = append(oaMsgs, openAIAssistantMsg{Role: "assistant", ToolCalls: tcs})
			} else {
				oaMsgs = append(oaMsgs, openAIMessage{Role: "assistant", Content: m.Content})
			}
		case "tool":
			oaMsgs = append(oaMsgs, openAIToolResultMsg{
				Role:       "tool",
				ToolCallID: m.ToolCallID,
				Content:    m.Content,
			})
		default:
			oaMsgs = append(oaMsgs, openAIMessage{Role: m.Role, Content: m.Content})
		}
	}
	var oaTools []openAITool
	for _, t := range tools {
		oaTools = append(oaTools, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	respBody, err := c.doPost(context.Background(), "/chat/completions", openAIChatRequest{
		Model:    c.model,
		Messages: oaMsgs,
		Tools:    oaTools,
	})
	if err != nil {
		return "", nil, err
	}
	var result openAIChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", nil, fmt.Errorf("decode openai response: %w", err)
	}
	if result.Error != nil {
		return "", nil, fmt.Errorf("openai error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", nil, fmt.Errorf("no choices in openai response")
	}
	msg := result.Choices[0].Message
	if len(msg.ToolCalls) > 0 {
		var calls []ToolCall
		for _, tc := range msg.ToolCalls {
			// OpenAI returns arguments as a JSON-encoded string (e.g. `"{\"q\":\"foo\"}"`).
			// Try to double-decode; if that fails the raw value is already a JSON object.
			rawArgs := json.RawMessage(tc.Function.Arguments)
			var argsStr string
			if json.Unmarshal(rawArgs, &argsStr) == nil {
				rawArgs = json.RawMessage(argsStr)
			}
			calls = append(calls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: rawArgs,
			})
		}
		return "", calls, nil
	}
	return strings.TrimSpace(msg.Content), nil, nil
}

// Embed returns a float32 embedding using /v1/embeddings with text-embedding-3-small.
func (c *OpenAICompatClient) Embed(text string) ([]float32, error) {
	respBody, err := c.doPost(context.Background(), "/embeddings", openAIEmbedRequest{
		Model: "text-embedding-3-small",
		Input: text,
	})
	if err != nil {
		return nil, err
	}
	var result openAIEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode embedding: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openai embed error: %s", result.Error.Message)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding")
	}
	return result.Data[0].Embedding, nil
}

// Classify asks the model to tag an email with a single category label.
func (c *OpenAICompatClient) Classify(sender, subject string) (Category, error) {
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

// GenerateQuickReplies asks the model for 3 short reply options.
func (c *OpenAICompatClient) GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error) {
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

// EnrichContact extracts company and topics from email subjects.
func (c *OpenAICompatClient) EnrichContact(email string, subjects []string) (string, []string, error) {
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
	comp := strings.ToLower(strings.TrimSpace(result.Company))
	if comp == "" || comp == "string or empty" || comp == "acme corp" || comp == "unknown" {
		result.Company = ""
	}
	return result.Company, result.Topics, nil
}

// DescribeImage sends an image to the OpenAI vision endpoint.
func (c *OpenAICompatClient) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(imageBytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)
	msgs := []any{
		openAIMessage{
			Role: "user",
			Content: []openAIContentBlock{
				{Type: "text", Text: "Describe this image in one sentence, focusing on what it shows."},
				{Type: "image_url", ImageURL: &openAIImageURL{URL: dataURL}},
			},
		},
	}
	respBody, err := c.doPost(ctx, "/chat/completions", openAIChatRequest{
		Model:    c.model,
		Messages: msgs,
	})
	if err != nil {
		return "", err
	}
	var result openAIChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// Ping checks reachability by listing models.
func (c *OpenAICompatClient) Ping() error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("openai not reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openai returned %d", resp.StatusCode)
	}
	return nil
}
