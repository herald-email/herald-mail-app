package ai

import "mail-processor/internal/config"

// NewFromConfig constructs the right AIClient based on cfg.AI.Provider.
// Returns (nil, nil) when no AI credentials are configured.
//
// IMPORTANT: This function returns a true nil interface value when no AI is
// configured, NOT a typed nil pointer. Callers must check: if client == nil { ... }
func NewFromConfig(cfg *config.Config) (AIClient, error) {
	embeddingModel := cfg.EffectiveEmbeddingModel()
	wrapLocal := func(client AIClient) AIClient {
		if client == nil {
			return nil
		}
		return NewManagedClient(client, ManagedConfig{
			MaxConcurrency:                  cfg.AI.LocalMaxConcurrency,
			QueueLimit:                      cfg.AI.BackgroundQueueLimit,
			PauseBackgroundWhileInteractive: cfg.AI.PauseBackgroundWhileInteractive,
		})
	}
	wrapExternal := func(client AIClient) AIClient {
		if client == nil {
			return nil
		}
		return NewManagedClient(client, ManagedConfig{
			MaxConcurrency:                  cfg.AI.ExternalMaxConcurrency,
			QueueLimit:                      cfg.AI.BackgroundQueueLimit,
			PauseBackgroundWhileInteractive: false,
		})
	}

	switch cfg.AI.Provider {
	case "claude":
		if cfg.Claude.APIKey == "" {
			return nil, nil
		}
		claude := wrapExternal(NewClaudeClient(cfg.Claude.APIKey, cfg.Claude.Model))
		// If Ollama is also configured, use it for embedding only.
		if cfg.Ollama.Host != "" {
			ollama := New(cfg.Ollama.Host, cfg.Ollama.Model)
			if embeddingModel != "" {
				ollama.SetEmbeddingModel(embeddingModel)
			}
			return NewCompositeClient(claude, wrapLocal(ollama)), nil
		}
		return claude, nil

	case "openai":
		if cfg.OpenAI.APIKey == "" {
			return nil, nil
		}
		openai := wrapExternal(NewOpenAICompatClient(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model))
		// If Ollama is also configured, use it for embedding only.
		if cfg.Ollama.Host != "" {
			ollama := New(cfg.Ollama.Host, cfg.Ollama.Model)
			if embeddingModel != "" {
				ollama.SetEmbeddingModel(embeddingModel)
			}
			return NewCompositeClient(openai, wrapLocal(ollama)), nil
		}
		return openai, nil

	default: // "ollama" or empty
		if cfg.Ollama.Host == "" {
			return nil, nil
		}
		c := New(cfg.Ollama.Host, cfg.Ollama.Model)
		if embeddingModel != "" {
			c.SetEmbeddingModel(embeddingModel)
		}
		return wrapLocal(c), nil
	}
}
