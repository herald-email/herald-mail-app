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
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: text}},
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
