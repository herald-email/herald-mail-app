package ai

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsVisionCapable(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gemma3:4b", true},
		{"gemma3:1b", true},
		{"gemma3:12b", true},
		{"gemma3n:4b", true},
		{"llava", true},
		{"llava:7b", true},
		{"bakllava", true},
		{"bakllava:7b", true},
		{"moondream", true},
		{"moondream:1.8b", true},
		{"minicpm-v", true},
		{"minicpm-v:8b", true},
		// case-insensitive
		{"Gemma3:4b", true},
		{"LLAVA:13b", true},
		// non-vision models
		{"gemma2:2b", false},
		{"gemma2", false},
		{"llama3", false},
		{"mistral", false},
		{"nomic-embed-text", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsVisionCapable(tt.model)
		if got != tt.want {
			t.Errorf("IsVisionCapable(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func newTestClassifier(srvURL string) *Classifier {
	c := New(srvURL, "test-model")
	return c
}

func TestChatWithTools_ReturnsToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []map[string]any{
					{
						"function": map[string]any{
							"name":      "search_emails",
							"arguments": json.RawMessage(`{"query":"invoice"}`),
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClassifier(srv.URL)
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

func TestChatWithTools_ReturnsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": "Here are your results",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClassifier(srv.URL)
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

func TestEmbed_FallsBackToSupportedEndpointAndCachesIt(t *testing.T) {
	var embeddingsCalls int
	var embedCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embeddings":
			embeddingsCalls++
			http.NotFound(w, r)
		case "/api/embed":
			embedCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"embeddings": [][]float32{{0.1, 0.2, 0.3}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClassifier(srv.URL)

	vec, err := c.Embed("hello")
	if err != nil {
		t.Fatalf("first Embed() failed: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3-d embedding from fallback endpoint, got %d", len(vec))
	}

	vec, err = c.Embed("world")
	if err != nil {
		t.Fatalf("second Embed() failed: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3-d embedding on cached endpoint, got %d", len(vec))
	}

	if embeddingsCalls != 1 {
		t.Fatalf("expected /api/embeddings to be probed once, got %d", embeddingsCalls)
	}
	if embedCalls != 2 {
		t.Fatalf("expected /api/embed to serve both requests, got %d", embedCalls)
	}
}

func TestMissingModelInstallHint(t *testing.T) {
	err := errors.New(`ollama /api/embed returned 404: {"error":"model \"nomic-embed-text\" not found, try pulling it first"}`)

	hint := MissingModelInstallHint(err)
	if !strings.Contains(hint, "ollama pull nomic-embed-text") {
		t.Fatalf("expected install hint for missing model, got %q", hint)
	}
}
