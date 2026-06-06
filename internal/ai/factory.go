package ai

import "github.com/herald-email/herald-mail-app/internal/config"

// NewFromConfig constructs the right AIClient based on cfg.AI.Provider.
// Returns (nil, nil) when no AI credentials are configured.
//
// IMPORTANT: This function returns a true nil interface value when no AI is
// configured, NOT a typed nil pointer. Callers must check: if client == nil { ... }
func NewFromConfig(cfg *config.Config) (AIClient, error) {
	embeddingProvider := cfg.EffectiveEmbeddingProvider()
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
	newOllamaEmbedding := func() AIClient {
		if cfg.Ollama.Host == "" {
			return nil
		}
		ollama := New(cfg.Ollama.Host, cfg.Ollama.Model)
		if embeddingModel != "" {
			ollama.SetEmbeddingModel(embeddingModel)
		}
		return wrapLocal(ollama)
	}
	newOpenAIEmbedding := func() AIClient {
		if cfg.OpenAI.APIKey == "" {
			return nil
		}
		openai := NewOpenAICompatClient(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model)
		if embeddingModel != "" {
			openai.SetEmbeddingModel(embeddingModel)
		}
		return wrapExternal(openai)
	}
	embeddingClient := func() AIClient {
		switch embeddingProvider {
		case config.EmbeddingProviderOpenAI:
			return newOpenAIEmbedding()
		default:
			return newOllamaEmbedding()
		}
	}

	switch cfg.AI.Provider {
	case "disabled":
		return nil, nil

	case "claude":
		if cfg.Claude.APIKey == "" {
			return nil, nil
		}
		claude := wrapExternal(NewClaudeClient(cfg.Claude.APIKey, cfg.Claude.Model))
		if embedding := embeddingClient(); embedding != nil {
			return NewCompositeClient(claude, embedding), nil
		}
		return claude, nil

	case "openai":
		if cfg.OpenAI.APIKey == "" {
			return nil, nil
		}
		openaiBase := NewOpenAICompatClient(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model)
		if embeddingProvider == config.EmbeddingProviderOpenAI && embeddingModel != "" {
			openaiBase.SetEmbeddingModel(embeddingModel)
		}
		openai := wrapExternal(openaiBase)
		if embeddingProvider != config.EmbeddingProviderOpenAI {
			if embedding := embeddingClient(); embedding != nil {
				return NewCompositeClient(openai, embedding), nil
			}
		}
		return openai, nil

	default: // "ollama" or empty
		if cfg.Ollama.Host == "" {
			return nil, nil
		}
		c := New(cfg.Ollama.Host, cfg.Ollama.Model)
		if embeddingProvider == config.EmbeddingProviderOllama && embeddingModel != "" {
			c.SetEmbeddingModel(embeddingModel)
		}
		ollama := wrapLocal(c)
		if embeddingProvider != config.EmbeddingProviderOllama {
			if embedding := embeddingClient(); embedding != nil {
				return NewCompositeClient(ollama, embedding), nil
			}
		}
		return ollama, nil
	}
}
