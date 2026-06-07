package app

import (
	"fmt"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/agent"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/logger"
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
		emailOptions := agent.EmailToolOptions{
			MaxResults:      20,
			MaxContextChars: 1200,
		}
		if memorySource, ok := m.backend.(agent.MemoryToolSource); ok {
			logger.Debug("Chat agent configured: provider=%s model=%s email_tools=true memory_tools=true max_email_results=%d max_memory_results=%d", providerCfg.Provider, providerCfg.Model, emailOptions.MaxResults, 12)
			m.chatAgent = agent.NewGollemRunnerWithEmailAndMemoryToolsAndOptions(model, m.backend, emailOptions, memorySource, agent.MemoryToolOptions{
				MaxResults:      12,
				ChatMinScore:    cfg.Memories.Thresholds.ChatRetrieval,
				ComposeMinScore: cfg.Memories.Thresholds.ComposeRadar,
			}, runnerOptions...)
			return
		}
		logger.Debug("Chat agent configured: provider=%s model=%s email_tools=true memory_tools=false max_email_results=%d", providerCfg.Provider, providerCfg.Model, emailOptions.MaxResults)
		m.chatAgent = agent.NewGollemRunnerWithEmailToolsAndOptions(model, m.backend, emailOptions, runnerOptions...)
		return
	}
	logger.Debug("Chat agent configured: provider=%s model=%s email_tools=false memory_tools=false", providerCfg.Provider, providerCfg.Model)
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
