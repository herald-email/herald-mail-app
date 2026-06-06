package app

import (
	"fmt"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/agent"
	"github.com/herald-email/herald-mail-app/internal/config"
)

func (m *Model) applyChatAgentConfig(cfg *config.Config) {
	m.chatAgent = nil
	if cfg == nil || strings.EqualFold(strings.TrimSpace(cfg.AI.Provider), "disabled") {
		return
	}
	providerCfg := chatAgentProviderConfig(cfg)
	model, err := agent.BuildModel(providerCfg)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Chat agent unavailable: %v", err)
		return
	}
	runnerOptions := agent.RunnerOptionsForProviderConfig(providerCfg)
	if m.backend != nil {
		m.chatAgent = agent.NewGollemRunnerWithEmailToolsAndOptions(model, m.backend, agent.EmailToolOptions{
			MaxResults:      20,
			MaxContextChars: 1200,
		}, runnerOptions...)
		return
	}
	m.chatAgent = agent.NewGollemRunner(model, runnerOptions...)
}

func chatAgentProviderConfig(cfg *config.Config) agent.ProviderConfig {
	provider := strings.TrimSpace(cfg.AI.Agent.Provider)
	if provider == "" {
		provider = strings.TrimSpace(cfg.AI.Provider)
	}
	out := agent.ProviderConfig{
		Provider:        provider,
		Model:           strings.TrimSpace(cfg.AI.Agent.Model),
		APIKey:          strings.TrimSpace(cfg.AI.Agent.APIKey),
		BaseURL:         strings.TrimSpace(cfg.AI.Agent.BaseURL),
		ReasoningEffort: agent.NormalizeReasoningEffort(cfg.AI.Agent.ReasoningEffort),
	}
	switch strings.ToLower(out.Provider) {
	case "", agent.ProviderOllama:
		out.Provider = agent.ProviderOllama
		if out.Model == "" {
			out.Model = strings.TrimSpace(cfg.Ollama.Model)
		}
		if out.BaseURL == "" {
			out.BaseURL = strings.TrimSpace(cfg.Ollama.Host)
		}
		if out.Model == "" {
			out.Model = defaultOllamaModel
		}
		if out.BaseURL == "" {
			out.BaseURL = defaultOllamaHost
		}
	case "claude":
		out.Provider = agent.ProviderAnthropic
		fallthrough
	case agent.ProviderAnthropic:
		out.Provider = agent.ProviderAnthropic
		if out.Model == "" {
			out.Model = strings.TrimSpace(cfg.Claude.Model)
		}
		if out.APIKey == "" {
			out.APIKey = strings.TrimSpace(cfg.Claude.APIKey)
		}
		if out.Model == "" {
			out.Model = "claude-sonnet-4-6"
		}
	case agent.ProviderOpenAI:
		if out.Model == "" {
			out.Model = strings.TrimSpace(cfg.OpenAI.Model)
		}
		if out.APIKey == "" {
			out.APIKey = strings.TrimSpace(cfg.OpenAI.APIKey)
		}
		if out.BaseURL == "" {
			out.BaseURL = strings.TrimSpace(cfg.OpenAI.BaseURL)
		}
		if out.Model == "" {
			out.Model = "gpt-5-mini"
		}
		if out.BaseURL == "" {
			out.BaseURL = "https://api.openai.com/v1"
		}
		if out.ReasoningEffort == "" {
			out.ReasoningEffort = agent.ReasoningEffortLow
		}
	}
	return out
}
