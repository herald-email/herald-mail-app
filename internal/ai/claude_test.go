package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func claudeOKResponse(text string) claudeResponse {
	return claudeResponse{
		Content: []claudeResponseBlock{{Type: "text", Text: text}},
	}
}

func newTestClaudeClient(srvURL string) *ClaudeClient {
	c := NewClaudeClient("test-key", "")
	c.baseURL = srvURL
	return c
}

func TestClaudeClientChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("missing anthropic-version header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeOKResponse("Hello from Claude"))
	}))
	defer srv.Close()

	c := newTestClaudeClient(srv.URL)
	reply, err := c.Chat([]ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "Hello from Claude" {
		t.Errorf("got %q", reply)
	}
}

func TestClaudeClientEmbedNotSupported(t *testing.T) {
	c := NewClaudeClient("key", "")
	_, err := c.Embed("text")
	if err != ErrEmbeddingNotSupported {
		t.Errorf("expected ErrEmbeddingNotSupported, got %v", err)
	}
}

func TestClaudeClientSystemPromotedToTopLevel(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeOKResponse("ok"))
	}))
	defer srv.Close()

	c := newTestClaudeClient(srv.URL)
	c.Chat([]ChatMessage{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "hello"},
	})

	// System message must appear as top-level "system" field, not in messages array
	if capturedBody["system"] != "you are helpful" {
		t.Errorf("expected system at top level, got: %v", capturedBody["system"])
	}
	messages, ok := capturedBody["messages"].([]any)
	if !ok {
		t.Fatal("messages field missing or wrong type")
	}
	for _, msg := range messages {
		m := msg.(map[string]any)
		if m["role"] == "system" {
			t.Error("system message must not appear in messages array")
		}
	}
}

func TestClaudeClientDescribeImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeOKResponse("A cat on a mat"))
	}))
	defer srv.Close()

	c := newTestClaudeClient(srv.URL)
	desc, err := c.DescribeImage(context.Background(), []byte("fake"), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if desc != "A cat on a mat" {
		t.Errorf("got %q", desc)
	}
}

func TestClaudeClientHasVisionModel(t *testing.T) {
	c := NewClaudeClient("key", "")
	if !c.HasVisionModel() {
		t.Error("Claude should always report HasVisionModel=true")
	}
}

func TestClaudeClientClassify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeOKResponse("spam"))
	}))
	defer srv.Close()

	c := newTestClaudeClient(srv.URL)
	cat, err := c.Classify("spammer@evil.com", "You won a prize!")
	if err != nil {
		t.Fatal(err)
	}
	if cat != CategorySpam {
		t.Errorf("expected spam, got %q", cat)
	}
}

func TestClaudeChatWithTools_ReturnsToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := claudeResponse{
			Content: []claudeResponseBlock{
				{
					Type:  "tool_use",
					ID:    "toolu_123",
					Name:  "search_emails",
					Input: json.RawMessage(`{"query":"invoice"}`),
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClaudeClient(srv.URL)
	tools := []Tool{
		{
			Name:        "search_emails",
			Description: "Search emails",
			Parameters: ToolParams{
				Type:       "object",
				Properties: map[string]ToolProp{"query": {Type: "string", Description: "query"}},
				Required:   []string{"query"},
			},
		},
	}

	text, calls, err := c.ChatWithTools([]ChatMessage{{Role: "user", Content: "find invoices"}}, tools)
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].ID != "toolu_123" {
		t.Errorf("expected ID toolu_123, got %q", calls[0].ID)
	}
	if calls[0].Name != "search_emails" {
		t.Errorf("expected search_emails, got %q", calls[0].Name)
	}
	var args map[string]string
	if err := json.Unmarshal(calls[0].Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if args["query"] != "invoice" {
		t.Errorf("expected query=invoice, got %q", args["query"])
	}
}

func TestClaudeChatWithTools_ReturnsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeOKResponse("Here are your results"))
	}))
	defer srv.Close()

	c := newTestClaudeClient(srv.URL)
	text, calls, err := c.ChatWithTools([]ChatMessage{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Here are your results" {
		t.Errorf("expected text, got %q", text)
	}
	if len(calls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(calls))
	}
}
