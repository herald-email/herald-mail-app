package ai

import (
	"mail-processor/internal/config"
	"testing"
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
	cfg.OpenAI.Model = "gpt-4o"

	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.(*ManagedClient); !ok {
		t.Errorf("expected *ManagedClient, got %T", client)
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
