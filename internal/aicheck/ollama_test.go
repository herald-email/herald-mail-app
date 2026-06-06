package aicheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
)

func TestValidateOllamaModelsPassesWhenRequiredModelsAreInstalled(t *testing.T) {
	server := ollamaTagsServer(t, http.StatusOK, `{"models":[{"name":"llama3.2:1b"},{"model":"nomic-embed-text"}]}`)

	result := ValidateOllamaModels(context.Background(), ollamaConfig(server.URL, "llama3.2:1b", "nomic-embed-text"))

	if err := result.Err(); err != nil {
		t.Fatalf("ValidateOllamaModels returned error: %v", err)
	}
	if len(result.InstallCommands()) != 0 {
		t.Fatalf("expected no install commands, got %#v", result.InstallCommands())
	}
}

func TestValidateOllamaModelsReportsMissingModelsWithInstallCommands(t *testing.T) {
	server := ollamaTagsServer(t, http.StatusOK, `{"models":[{"name":"llama3.2:1b"}]}`)

	result := ValidateOllamaModels(context.Background(), ollamaConfig(server.URL, "missing-chat", "missing-embed"))

	if err := result.Err(); err == nil {
		t.Fatal("expected missing models to fail validation")
	}
	commands := strings.Join(result.InstallCommands(), "\n")
	for _, want := range []string{"ollama pull missing-chat", "ollama pull missing-embed"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("expected install commands to contain %q, got %q", want, commands)
		}
	}
	message := result.UserMessage("/tmp/herald.log", "/tmp/herald.yaml")
	for _, want := range []string{"missing-chat", "missing-embed", "Settings were not saved to /tmp/herald.yaml", "/tmp/herald.log"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected user message to contain %q, got %q", want, message)
		}
	}
}

func TestValidateOllamaModelsAcceptsBareNameWhenLatestTagIsInstalled(t *testing.T) {
	server := ollamaTagsServer(t, http.StatusOK, `{"models":[{"name":"llama3:latest"},{"name":"nomic-embed-text:latest"}]}`)

	result := ValidateOllamaModels(context.Background(), ollamaConfig(server.URL, "llama3", "nomic-embed-text"))

	if err := result.Err(); err != nil {
		t.Fatalf("expected bare model names to match installed latest tags: %v", err)
	}
}

func TestValidateOllamaModelsChecksOnlyLocalEmbeddingForExternalChat(t *testing.T) {
	server := ollamaTagsServer(t, http.StatusOK, `{"models":[{"name":"nomic-embed-text"}]}`)
	cfg := ollamaConfig(server.URL, "missing-chat", "nomic-embed-text")
	cfg.AI.Provider = "openai"
	cfg.OpenAI.APIKey = "sk-test"
	cfg.Semantic.Provider = config.EmbeddingProviderOllama

	result := ValidateOllamaModels(context.Background(), cfg)

	if err := result.Err(); err != nil {
		t.Fatalf("external chat with local embeddings should validate only embedding model: %v", err)
	}
}

func TestValidateOllamaModelsChecksOnlyLocalChatForExternalEmbeddings(t *testing.T) {
	server := ollamaTagsServer(t, http.StatusOK, `{"models":[{"name":"llama3.2:1b"}]}`)
	cfg := ollamaConfig(server.URL, "llama3.2:1b", "missing-embed")
	cfg.Semantic.Provider = config.EmbeddingProviderOpenAI
	cfg.Semantic.Model = "text-embedding-3-small"
	cfg.OpenAI.APIKey = "sk-test"

	result := ValidateOllamaModels(context.Background(), cfg)

	if err := result.Err(); err != nil {
		t.Fatalf("local chat with external embeddings should validate only chat model: %v", err)
	}
}

func TestValidateOllamaModelsReturnsReachabilityError(t *testing.T) {
	server := ollamaTagsServer(t, http.StatusInternalServerError, `{"error":"offline"}`)

	result := ValidateOllamaModels(context.Background(), ollamaConfig(server.URL, "llama3.2:1b", "nomic-embed-text"))

	if err := result.Err(); err == nil {
		t.Fatal("expected non-OK /api/tags response to fail validation")
	}
	if msg := result.UserMessage("", ""); !strings.Contains(msg, "Ollama is not reachable") {
		t.Fatalf("expected bounded reachability message, got %q", msg)
	}
}

func TestRequiresOllamaModelValidationOnlyForNewOrChangedOllama(t *testing.T) {
	prev := ollamaConfig("http://ollama.test", "llama3.2:1b", "nomic-embed-text")
	next := ollamaConfig("http://ollama.test", "llama3.2:1b", "nomic-embed-text")

	if RequiresOllamaModelValidation(prev, next) {
		t.Fatal("unchanged existing Ollama config should not revalidate on ordinary save")
	}

	changed := ollamaConfig("http://ollama.test", "gemma3:4b", "nomic-embed-text")
	if !RequiresOllamaModelValidation(prev, changed) {
		t.Fatal("changed Ollama model should require validation")
	}

	disabled := ollamaConfig("http://ollama.test", "gemma3:4b", "nomic-embed-text")
	disabled.AI.Provider = "disabled"
	if RequiresOllamaModelValidation(prev, disabled) {
		t.Fatal("disabled AI should not require Ollama validation")
	}
}

func ollamaTagsServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func ollamaConfig(host, chatModel, embedModel string) *config.Config {
	cfg := &config.Config{}
	cfg.AI.Provider = "ollama"
	cfg.Ollama.Host = host
	cfg.Ollama.Model = chatModel
	cfg.Ollama.EmbeddingModel = embedModel
	cfg.Semantic.Model = embedModel
	return cfg
}
