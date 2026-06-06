package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
)

const (
	ProviderOllama    = "ollama"
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderKimi      = "kimi"
	ProviderFireworks = "fireworks"

	ReasoningEffortLow    = "low"
	ReasoningEffortMedium = "medium"
	ReasoningEffortHigh   = "high"
	ReasoningEffortXHigh  = "xhigh"

	defaultOpenAIBaseURL    = "https://api.openai.com/v1"
	defaultKimiBaseURL      = "https://api.moonshot.ai/v1"
	defaultFireworksBaseURL = "https://api.fireworks.ai/inference/v1"
)

type ProviderConfig struct {
	Provider        string
	Model           string
	APIKey          string
	BaseURL         string
	ReasoningEffort string
}

type GollemRunner struct {
	agent *core.Agent[ChatResult]
	model core.Model
	tools []core.Tool
}

func NewGollemRunner(model core.Model, opts ...core.AgentOption[ChatResult]) *GollemRunner {
	return newGollemRunner(model, nil, opts...)
}

func newGollemRunner(model core.Model, tools []core.Tool, opts ...core.AgentOption[ChatResult]) *GollemRunner {
	options := append([]core.AgentOption[ChatResult]{
		core.WithSystemPrompt[ChatResult](systemPrompt()),
		core.WithMaxRetries[ChatResult](1),
	}, opts...)
	if len(tools) > 0 {
		options = append(options, core.WithTools[ChatResult](tools...))
	}
	return &GollemRunner{
		agent: core.NewAgent[ChatResult](model, options...),
		model: model,
		tools: tools,
	}
}

func NewGollemRunnerWithEmailTools(model core.Model, source EmailToolSource, toolOptions EmailToolOptions) *GollemRunner {
	return NewGollemRunnerWithEmailToolsAndOptions(model, source, toolOptions)
}

func NewGollemRunnerWithEmailToolsAndOptions(model core.Model, source EmailToolSource, toolOptions EmailToolOptions, opts ...core.AgentOption[ChatResult]) *GollemRunner {
	service := NewEmailToolService(source, toolOptions)
	return newGollemRunner(model, service.GollemTools(), opts...)
}

func NewGollemRunnerWithEmailAndMemoryToolsAndOptions(model core.Model, emailSource EmailToolSource, emailOptions EmailToolOptions, memorySource MemoryToolSource, memoryOptions MemoryToolOptions, opts ...core.AgentOption[ChatResult]) *GollemRunner {
	emailService := NewEmailToolService(emailSource, emailOptions)
	tools := append([]core.Tool{}, emailService.GollemTools()...)
	if memorySource != nil {
		memoryService := NewMemoryToolService(memorySource, memoryOptions)
		tools = append(tools, memoryService.GollemTools()...)
	}
	return newGollemRunner(model, tools, opts...)
}

func RunnerOptionsForProviderConfig(cfg ProviderConfig) []core.AgentOption[ChatResult] {
	if strings.ToLower(strings.TrimSpace(cfg.Provider)) != ProviderOpenAI {
		return nil
	}
	effort := NormalizeReasoningEffort(cfg.ReasoningEffort)
	if effort == "" {
		return nil
	}
	return []core.AgentOption[ChatResult]{core.WithReasoningEffort[ChatResult](effort)}
}

func NormalizeReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case ReasoningEffortLow:
		return ReasoningEffortLow
	case ReasoningEffortMedium:
		return ReasoningEffortMedium
	case ReasoningEffortHigh:
		return ReasoningEffortHigh
	case ReasoningEffortXHigh:
		return ReasoningEffortXHigh
	default:
		return ""
	}
}

func (r *GollemRunner) Run(ctx context.Context, input ChatInput) (ChatResult, error) {
	if r == nil || r.agent == nil {
		return ChatResult{}, fmt.Errorf("gollem runner is not configured")
	}
	result, err := r.agent.Run(ctx, buildPrompt(input), core.WithRunDeps(input))
	if err != nil {
		if reply, fallbackErr := r.runTextFallback(ctx, input, err); fallbackErr == nil {
			return ChatResult{Reply: reply}, nil
		}
		return ChatResult{}, err
	}
	return result.Output, nil
}

func (r *GollemRunner) runTextFallback(ctx context.Context, input ChatInput, originalErr error) (string, error) {
	if !isResultValidationError(originalErr) || r.model == nil {
		return "", originalErr
	}
	options := []core.AgentOption[string]{
		core.WithSystemPrompt[string](systemPrompt()),
	}
	if len(r.tools) > 0 {
		options = append(options, core.WithTools[string](r.tools...))
	}
	result, err := core.NewAgent[string](r.model, options...).Run(ctx, buildPrompt(input), core.WithRunDeps(input))
	if err != nil {
		return "", err
	}
	reply := strings.TrimSpace(result.Output)
	if reply == "" {
		return "", originalErr
	}
	return reply, nil
}

func isResultValidationError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "result validation")
}

func BuildModel(cfg ProviderConfig) (core.Model, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = ProviderOllama
	}
	switch provider {
	case ProviderOllama:
		opts := []openai.Option{}
		if cfg.Model != "" {
			opts = append(opts, openai.WithModel(cfg.Model))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, openai.WithBaseURL(cfg.BaseURL))
		}
		return openai.NewOllama(opts...), nil
	case ProviderAnthropic:
		opts := []anthropic.Option{}
		if cfg.APIKey != "" {
			opts = append(opts, anthropic.WithAPIKey(cfg.APIKey))
		}
		if cfg.Model != "" {
			opts = append(opts, anthropic.WithModel(cfg.Model))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, anthropic.WithBaseURL(cfg.BaseURL))
		}
		return anthropic.New(opts...), nil
	case ProviderOpenAI:
		return buildOpenAICompatibleModel(cfg, defaultOpenAIBaseURL), nil
	case ProviderKimi:
		return buildOpenAICompatibleModel(cfg, defaultKimiBaseURL), nil
	case ProviderFireworks:
		return buildOpenAICompatibleModel(cfg, defaultFireworksBaseURL), nil
	default:
		return nil, fmt.Errorf("unsupported Gollem chat provider: %s", cfg.Provider)
	}
}

func buildOpenAICompatibleModel(cfg ProviderConfig, defaultBaseURL string) core.Model {
	opts := []openai.Option{}
	if cfg.APIKey != "" {
		opts = append(opts, openai.WithAPIKey(cfg.APIKey))
	}
	if cfg.Model != "" {
		opts = append(opts, openai.WithModel(cfg.Model))
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	opts = append(opts, openai.WithBaseURL(baseURL))
	return openai.New(opts...)
}

func systemPrompt() string {
	return strings.Join([]string{
		"You are Herald's UI chat agent.",
		"Return concise, source-grounded answers for the user.",
		"Default to 2-5 short sentences or at most 5 bullets unless the user asks for detail.",
		"Do not send email, delete email, archive email, or mutate calendar events.",
		"When answering from Herald Memories, cite source evidence and distinguish email, Obsidian, public research, and inference.",
		"No evidence means no factual memory answer.",
		"When no UI action is needed, return only a helpful reply.",
	}, "\n")
}

func buildPrompt(input ChatInput) string {
	var sb strings.Builder
	if input.CurrentFolder != "" {
		sb.WriteString("Current folder: ")
		sb.WriteString(input.CurrentFolder)
		sb.WriteString("\n")
	}
	if input.ActiveTab != "" {
		sb.WriteString("Active tab: ")
		sb.WriteString(input.ActiveTab)
		sb.WriteString("\n")
	}
	if len(input.VisibleIDs) > 0 {
		sb.WriteString("Visible message IDs: ")
		sb.WriteString(strings.Join(input.VisibleIDs, ", "))
		sb.WriteString("\n")
	}
	if len(input.SelectedIDs) > 0 {
		sb.WriteString("Selected message IDs: ")
		sb.WriteString(strings.Join(input.SelectedIDs, ", "))
		sb.WriteString("\n")
	}
	if input.ComposeSnapshot != nil {
		sb.WriteString("Compose is active.\n")
		if input.ComposeSnapshot.Subject != "" {
			sb.WriteString("Compose subject: ")
			sb.WriteString(input.ComposeSnapshot.Subject)
			sb.WriteString("\n")
		}
	}
	for _, turn := range input.History {
		if strings.TrimSpace(turn.Content) == "" {
			continue
		}
		sb.WriteString(strings.ToLower(strings.TrimSpace(turn.Role)))
		sb.WriteString(": ")
		sb.WriteString(strings.TrimSpace(turn.Content))
		sb.WriteString("\n")
	}
	sb.WriteString("User: ")
	sb.WriteString(strings.TrimSpace(input.UserMessage))
	return sb.String()
}
