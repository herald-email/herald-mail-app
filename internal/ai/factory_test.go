package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
)

func TestNewFromConfigOllama(t *testing.T) {
	cfg := &config.Config{}
	cfg.Ollama.Host = "http://localhost:11434"
	cfg.Ollama.Model = "gemma2"
	cfg.AI.Provider = "ollama"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if _, ok := client.(*ManagedClient); !ok {
		t.Errorf("expected *ManagedClient, got %T", client)
	}
}

func TestNewFromConfigNoAI(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Provider = "ollama"
	// No Ollama host = no AI

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if client != nil {
		t.Errorf("expected nil client when no AI configured, got %T", client)
	}
}

func TestNewFromConfigDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Provider = "disabled"
	cfg.Ollama.Host = "http://localhost:11434"
	cfg.Claude.APIKey = "sk-ant-test"
	cfg.OpenAI.APIKey = "sk-test"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if client != nil {
		t.Errorf("expected nil client when AI is disabled, got %T", client)
	}
}

func TestNewFromConfigClaude(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Provider = "claude"
	cfg.Claude.APIKey = "sk-ant-test"
	cfg.Claude.Model = "claude-sonnet-4-6"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if _, ok := client.(*ManagedClient); !ok {
		t.Errorf("expected *ManagedClient, got %T", client)
	}
}

func TestNewFromConfigClaudeWithOllamaEmbedding(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Provider = "claude"
	cfg.Claude.APIKey = "sk-ant-test"
	cfg.Ollama.Host = "http://localhost:11434"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.(*CompositeClient); !ok {
		t.Errorf("expected *CompositeClient when both claude+ollama configured, got %T", client)
	}
}

func TestNewFromConfigOpenAI(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Provider = "openai"
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = "https://api.openai.com/v1"
	cfg.OpenAI.Model = "gpt-5.4-mini"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.(*ManagedClient); !ok {
		t.Errorf("expected *ManagedClient, got %T", client)
	}
}

func TestNewFromConfigOpenAIUsesConfiguredEmbeddingModel(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIEmbedOKResponse([]float32{0.1, 0.2}))
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.AI.Provider = "openai"
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = srv.URL
	cfg.OpenAI.Model = "gpt-5.4-mini"
	cfg.OpenAI.EmbeddingModel = "text-embedding-3-large"
	cfg.Semantic.Provider = config.EmbeddingProviderOpenAI
	cfg.Semantic.Model = "text-embedding-3-large"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Embed("hello"); err != nil {
		t.Fatal(err)
	}
	if gotModel != "text-embedding-3-large" {
		t.Fatalf("embedding model = %q, want text-embedding-3-large", gotModel)
	}
}

func TestNewFromConfigClaudeCanUseOpenAIEmbeddingProvider(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req openAIEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIEmbedOKResponse([]float32{0.1, 0.2}))
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.AI.Provider = "claude"
	cfg.Claude.APIKey = "sk-ant-test"
	cfg.OpenAI.APIKey = "sk-test"
	cfg.OpenAI.BaseURL = srv.URL
	cfg.OpenAI.EmbeddingModel = "text-embedding-3-small"
	cfg.Semantic.Provider = config.EmbeddingProviderOpenAI
	cfg.Semantic.Model = "text-embedding-3-small"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.(*CompositeClient); !ok {
		t.Fatalf("expected *CompositeClient, got %T", client)
	}
	if _, err := client.Embed("hello"); err != nil {
		t.Fatal(err)
	}
	if gotModel != "text-embedding-3-small" {
		t.Fatalf("embedding model = %q, want text-embedding-3-small", gotModel)
	}
}

// TestNewFromConfigReturnsNilInterface verifies we never return a typed nil
// (the classic Go interface nil pitfall).
func TestNewFromConfigReturnsNilInterface(t *testing.T) {
	cfg := &config.Config{} // no AI configured
	cfg.AI.Provider = "ollama"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// This must be a true nil interface, not a typed nil
	var expected AIClient
	if client != expected {
		t.Error("expected nil interface, got non-nil")
	}
}
