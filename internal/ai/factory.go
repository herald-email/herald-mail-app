package ai

import "mail-processor/internal/config"

// NewFromConfig constructs the right AIClient based on cfg.AI.Provider.
// Returns (nil, nil) when no AI credentials are configured.
//
// IMPORTANT: This function returns a true nil interface value when no AI is
// configured, NOT a typed nil pointer. Callers must check: if client == nil { ... }
func NewFromConfig(cfg *config.Config) (AIClient, error) {
	switch cfg.AI.Provider {
	case "claude":
		if cfg.Claude.APIKey == "" {
			return nil, nil
		}
		claude := NewClaudeClient(cfg.Claude.APIKey, cfg.Claude.Model)
		// If Ollama is also configured, use it for embedding only.
		if cfg.Ollama.Host != "" {
			ollama := New(cfg.Ollama.Host, cfg.Ollama.Model)
			if cfg.Ollama.EmbeddingModel != "" {
				ollama.SetEmbeddingModel(cfg.Ollama.EmbeddingModel)
			}
			return NewCompositeClient(claude, ollama), nil
		}
		return claude, nil

	case "openai":
		if cfg.OpenAI.APIKey == "" {
			return nil, nil
		}
		openai := NewOpenAICompatClient(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model)
		// If Ollama is also configured, use it for embedding only.
		if cfg.Ollama.Host != "" {
			ollama := New(cfg.Ollama.Host, cfg.Ollama.Model)
			if cfg.Ollama.EmbeddingModel != "" {
				ollama.SetEmbeddingModel(cfg.Ollama.EmbeddingModel)
			}
			return NewCompositeClient(openai, ollama), nil
		}
		return openai, nil

	default: // "ollama" or empty
		if cfg.Ollama.Host == "" {
			return nil, nil
		}
		c := New(cfg.Ollama.Host, cfg.Ollama.Model)
		if cfg.Ollama.EmbeddingModel != "" {
			c.SetEmbeddingModel(cfg.Ollama.EmbeddingModel)
		}
		return c, nil
	}
}
