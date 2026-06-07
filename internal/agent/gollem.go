package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
	"github.com/herald-email/herald-mail-app/internal/logger"
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
	agent     *core.Agent[ChatResult]
	model     core.Model
	tools     []core.Tool
	telemetry *chatAgentTelemetry
}

func NewGollemRunner(model core.Model, opts ...core.AgentOption[ChatResult]) *GollemRunner {
	return newGollemRunner(model, nil, opts...)
}

func newGollemRunner(model core.Model, tools []core.Tool, opts ...core.AgentOption[ChatResult]) *GollemRunner {
	modelName := ""
	if model != nil {
		modelName = model.ModelName()
	}
	telemetry := newChatAgentTelemetry(modelName, len(tools))
	options := append([]core.AgentOption[ChatResult]{
		core.WithSystemPrompt[ChatResult](systemPrompt()),
		core.WithMaxRetries[ChatResult](0),
		core.WithHooks[ChatResult](telemetry.hook()),
	}, opts...)
	if len(tools) > 0 {
		options = append(options, core.WithTools[ChatResult](tools...))
	}
	return &GollemRunner{
		agent:     core.NewAgent[ChatResult](model, options...),
		model:     model,
		tools:     tools,
		telemetry: telemetry,
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
		if isStructuredOutputError(err) {
			if fallbackText := r.telemetry.consumeStructuredFallbackText(); fallbackText != "" {
				return chatResultFromFallbackText(fallbackText), nil
			}
		}
		if reply, fallbackErr := r.runTextFallback(ctx, input, err); fallbackErr == nil {
			return chatResultFromFallbackText(reply), nil
		}
		return ChatResult{}, err
	}
	return result.Output, nil
}

func (r *GollemRunner) runTextFallback(ctx context.Context, input ChatInput, originalErr error) (string, error) {
	if !isStructuredOutputError(originalErr) || r.model == nil {
		return "", originalErr
	}
	modelName := r.model.ModelName()
	options := []core.AgentOption[string]{
		core.WithSystemPrompt[string](systemPrompt()),
		core.WithHooks[string](newChatAgentTelemetry(modelName, len(r.tools)).hook()),
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

func chatResultFromFallbackText(text string) ChatResult {
	text = strings.TrimSpace(text)
	if result, ok := tryParseChatResult(text); ok {
		return result
	}
	return ChatResult{Reply: text}
}

func tryParseChatResult(text string) (ChatResult, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ChatResult{}, false
	}
	if strings.HasPrefix(text, "```") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "```json"))
		text = strings.TrimSpace(strings.TrimPrefix(text, "```"))
		text = strings.TrimSpace(strings.TrimSuffix(text, "```"))
	}
	var result ChatResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return ChatResult{}, false
	}
	if strings.TrimSpace(result.Reply) == "" && result.Timeline == nil && result.Summary == nil && result.Compose == nil {
		return ChatResult{}, false
	}
	return result, true
}

type chatAgentTelemetry struct {
	modelName string
	toolCount int

	mu                     sync.Mutex
	runStarts              map[string]time.Time
	modelStarts            map[string]time.Time
	toolStarts             map[string]time.Time
	lastTextByRun          map[string]string
	structuredFallbackText string
}

func newChatAgentTelemetry(modelName string, toolCount int) *chatAgentTelemetry {
	return &chatAgentTelemetry{
		modelName:     modelName,
		toolCount:     toolCount,
		runStarts:     map[string]time.Time{},
		modelStarts:   map[string]time.Time{},
		toolStarts:    map[string]time.Time{},
		lastTextByRun: map[string]string{},
	}
}

func (t *chatAgentTelemetry) consumeStructuredFallbackText() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	text := t.structuredFallbackText
	t.structuredFallbackText = ""
	return text
}

func (t *chatAgentTelemetry) hook() core.Hook {
	key := func(runID string, n int) string {
		return runID + ":" + fmt.Sprint(n)
	}

	return core.Hook{
		OnRunStart: func(_ context.Context, rc *core.RunContext, prompt string) {
			started := time.Now()
			t.mu.Lock()
			t.runStarts[rc.RunID] = started
			t.mu.Unlock()
			logger.Debug("Chat agent run started: run=%s model=%s tools=%d prompt_chars=%d", rc.RunID, t.modelName, t.toolCount, len([]rune(prompt)))
		},
		OnRunEnd: func(_ context.Context, rc *core.RunContext, _ []core.ModelMessage, err error) {
			t.mu.Lock()
			started, ok := t.runStarts[rc.RunID]
			if err != nil && isStructuredOutputError(err) {
				t.structuredFallbackText = t.lastTextByRun[rc.RunID]
			}
			delete(t.runStarts, rc.RunID)
			delete(t.lastTextByRun, rc.RunID)
			t.mu.Unlock()
			duration := time.Duration(0)
			if ok {
				duration = time.Since(started)
			}
			logger.Debug("Chat agent run completed: run=%s model=%s duration=%s error=%t", rc.RunID, t.modelName, duration.Round(time.Millisecond), err != nil)
		},
		OnTurnStart: func(_ context.Context, rc *core.RunContext, turnNumber int) {
			logger.Debug("Chat agent turn started: run=%s turn=%d", rc.RunID, turnNumber)
		},
		OnModelRequest: func(_ context.Context, rc *core.RunContext, messages []core.ModelMessage) {
			t.mu.Lock()
			t.modelStarts[key(rc.RunID, rc.RunStep)] = time.Now()
			t.mu.Unlock()
			logger.Debug("Chat agent model request started: run=%s turn=%d messages=%d", rc.RunID, rc.RunStep, len(messages))
		},
		OnModelResponse: func(_ context.Context, rc *core.RunContext, response *core.ModelResponse) {
			t.mu.Lock()
			started, ok := t.modelStarts[key(rc.RunID, rc.RunStep)]
			delete(t.modelStarts, key(rc.RunID, rc.RunStep))
			t.mu.Unlock()
			duration := time.Duration(0)
			if ok {
				duration = time.Since(started)
			}
			inputTokens := 0
			outputTokens := 0
			finishReason := ""
			hasToolCalls := false
			hasText := false
			if response != nil {
				inputTokens = response.Usage.InputTokens
				outputTokens = response.Usage.OutputTokens
				finishReason = string(response.FinishReason)
				hasToolCalls = len(response.ToolCalls()) > 0
				text := strings.TrimSpace(response.TextContent())
				hasText = text != ""
				if hasText {
					t.mu.Lock()
					t.lastTextByRun[rc.RunID] = text
					t.mu.Unlock()
				}
			}
			logger.Debug("Chat agent model response completed: run=%s turn=%d model=%s duration=%s input_tokens=%d output_tokens=%d has_tool_calls=%t has_text=%t finish=%s", rc.RunID, rc.RunStep, t.modelName, duration.Round(time.Millisecond), inputTokens, outputTokens, hasToolCalls, hasText, finishReason)
		},
		OnToolStart: func(_ context.Context, rc *core.RunContext, toolCallID string, toolName string, _ string) {
			t.mu.Lock()
			t.toolStarts[rc.RunID+":"+toolCallID] = time.Now()
			t.mu.Unlock()
			logger.Debug("Chat agent tool started: run=%s tool=%s", rc.RunID, toolName)
		},
		OnToolEnd: func(_ context.Context, rc *core.RunContext, toolCallID string, toolName string, _ string, err error) {
			t.mu.Lock()
			toolKey := rc.RunID + ":" + toolCallID
			started, ok := t.toolStarts[toolKey]
			delete(t.toolStarts, toolKey)
			t.mu.Unlock()
			duration := time.Duration(0)
			if ok {
				duration = time.Since(started)
			}
			logger.Debug("Chat agent tool completed: run=%s tool=%s duration=%s error=%t", rc.RunID, toolName, duration.Round(time.Millisecond), err != nil)
		},
		OnTurnEnd: func(_ context.Context, rc *core.RunContext, turnNumber int, response *core.ModelResponse) {
			hasToolCalls := false
			hasText := false
			if response != nil {
				hasToolCalls = len(response.ToolCalls()) > 0
				hasText = strings.TrimSpace(response.TextContent()) != ""
			}
			logger.Debug("Chat agent turn completed: run=%s turn=%d has_tool_calls=%t has_text=%t", rc.RunID, turnNumber, hasToolCalls, hasText)
		},
		OnOutputValidation: func(_ context.Context, rc *core.RunContext, passed bool, err error) {
			logger.Debug("Chat agent output validation: run=%s passed=%t error=%t", rc.RunID, passed, err != nil)
		},
	}
}

func isStructuredOutputError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "result validation") ||
		strings.Contains(message, "failed to parse text output")
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
		"Return a JSON object matching the ChatResult schema.",
		"When no UI action is needed, set reply to the helpful answer and omit timeline, summary, and compose.",
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
