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
	"github.com/herald-email/herald-mail-app/internal/searchquery"
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
		core.WithToolsPrepare[ChatResult](prepareChatToolsForRun),
		core.WithAgentMiddleware[ChatResult](chatToolPolicyMiddleware()),
		core.WithToolChoiceAutoReset[ChatResult](),
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

type chatToolPolicy struct {
	filter      bool
	allowed     map[string]bool
	requireTool bool
}

type chatCapability string

const (
	chatCapabilityMailboxSearch  chatCapability = "mailbox_search"
	chatCapabilityMailboxContext chatCapability = "mailbox_context"
	chatCapabilityMailboxSummary chatCapability = "mailbox_summary"
	chatCapabilityPeople         chatCapability = "people"
)

type chatToolSpec struct {
	capabilities       map[chatCapability]bool
	requiresMessageIDs bool
	providesMessageIDs bool
}

var chatToolSpecs = map[string]chatToolSpec{
	"find_emails": {
		capabilities:       chatCapabilitySet(chatCapabilityMailboxSearch),
		providesMessageIDs: true,
	},
	"get_email_context": {
		capabilities:       chatCapabilitySet(chatCapabilityMailboxContext),
		requiresMessageIDs: true,
	},
	"summarize_email_set": {
		capabilities:       chatCapabilitySet(chatCapabilityMailboxSummary),
		requiresMessageIDs: true,
	},
	"explain_people": {
		capabilities:       chatCapabilitySet(chatCapabilityPeople),
		requiresMessageIDs: true,
	},
}

func chatCapabilitySet(capabilities ...chatCapability) map[chatCapability]bool {
	out := make(map[chatCapability]bool, len(capabilities))
	for _, capability := range capabilities {
		out[capability] = true
	}
	return out
}

type chatToolFacts struct {
	messageIDs            bool
	queryScopedMessageIDs bool
	completed             map[chatCapability]bool
}

func prepareChatToolsForRun(_ context.Context, rc *core.RunContext, defs []core.ToolDefinition) []core.ToolDefinition {
	input, ok := core.TryGetDeps[ChatInput](rc)
	if !ok {
		return defs
	}
	policy := chatToolPolicyForRun(input, rc.Messages)
	if !policy.filter {
		return defs
	}
	filtered := make([]core.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if policy.allowed[def.Name] {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func chatToolPolicyMiddleware() core.AgentMiddleware {
	return core.RequestOnlyMiddleware(func(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters, next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error)) (*core.ModelResponse, error) {
		input := ChatInput{UserMessage: latestUserMessageFromModelMessages(messages)}
		if modelMessagesContainMessageIDContext(messages) {
			input.VisibleIDs = []string{"context"}
		}
		policy := chatToolPolicyForRun(input, messages)
		if policy.requireTool {
			if settings == nil {
				settings = &core.ModelSettings{}
			}
			if shouldSetRequiredToolChoice(settings.ToolChoice) {
				settings.ToolChoice = core.ToolChoiceRequired()
			}
		}
		return next(ctx, messages, settings, params)
	})
}

func chatToolPolicyForInput(input ChatInput) chatToolPolicy {
	return chatToolPolicyForRun(input, nil)
}

func chatToolPolicyForRun(input ChatInput, messages []core.ModelMessage) chatToolPolicy {
	terms := searchquery.Terms(input.UserMessage)
	if len(terms) == 0 {
		return chatToolPolicy{filter: true, allowed: map[string]bool{}}
	}
	if isPlainChatTurn(terms) {
		return chatToolPolicy{filter: true, allowed: map[string]bool{}}
	}
	facts := chatToolFactsForRun(input, messages)
	requested := requestedChatCapabilities(terms, facts)
	if len(requested) == 0 {
		return chatToolPolicy{}
	}
	allowed := make(map[string]bool)
	for name, spec := range chatToolSpecs {
		if !chatToolCanAdvanceRequestedCapabilities(spec, requested, facts) {
			continue
		}
		if spec.requiresMessageIDs && !chatToolHasRequiredMessageIDs(requested, facts) {
			continue
		}
		allowed[name] = true
	}
	return chatToolPolicy{
		filter:      true,
		allowed:     allowed,
		requireTool: len(allowed) > 0,
	}
}

func shouldSetRequiredToolChoice(choice *core.ToolChoice) bool {
	if choice == nil {
		return true
	}
	return choice.ToolName == "" && (choice.Mode == "" || choice.Mode == "auto")
}

func requestedChatCapabilities(terms []string, facts chatToolFacts) map[chatCapability]bool {
	requested := map[chatCapability]bool{}
	if hasMailboxSearchIntent(terms) {
		requested[chatCapabilityMailboxSearch] = true
	}
	if hasAnyTerm(terms, "summarize", "summary", "digest", "recap") {
		requested[chatCapabilityMailboxSummary] = true
	}
	if hasAnyTerm(terms, "context", "inspect", "details", "detail") {
		requested[chatCapabilityMailboxContext] = true
	}
	if hasAnyTerm(terms, "people", "person", "who") {
		requested[chatCapabilityPeople] = true
	}
	if chatRequiresMessageSet(requested) && !facts.queryScopedMessageIDs && hasSearchTopicTerm(terms) {
		requested[chatCapabilityMailboxSearch] = true
	}
	if chatRequiresMessageSet(requested) && !facts.messageIDs && hasSearchScopeOrTopic(terms) {
		requested[chatCapabilityMailboxSearch] = true
	}
	return requested
}

func chatRequiresMessageSet(requested map[chatCapability]bool) bool {
	return requested[chatCapabilityMailboxContext] ||
		requested[chatCapabilityMailboxSummary] ||
		requested[chatCapabilityPeople]
}

func chatToolCanAdvanceRequestedCapabilities(spec chatToolSpec, requested map[chatCapability]bool, facts chatToolFacts) bool {
	for capability := range spec.capabilities {
		if requested[capability] && !facts.completed[capability] {
			return true
		}
	}
	return false
}

func chatToolHasRequiredMessageIDs(requested map[chatCapability]bool, facts chatToolFacts) bool {
	if !facts.messageIDs {
		return false
	}
	if requested[chatCapabilityMailboxSearch] {
		return facts.queryScopedMessageIDs
	}
	return true
}

func chatToolFactsForRun(input ChatInput, messages []core.ModelMessage) chatToolFacts {
	facts := chatToolFacts{
		messageIDs: hasSummaryContext(input) || modelMessagesContainMessageIDContext(messages),
		completed:  map[chatCapability]bool{},
	}
	for _, msg := range messages {
		req, ok := msg.(core.ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			toolReturn, ok := part.(core.ToolReturnPart)
			if !ok {
				continue
			}
			spec, ok := chatToolSpecs[toolReturn.ToolName]
			if !ok {
				continue
			}
			for capability := range spec.capabilities {
				facts.completed[capability] = true
			}
			if spec.providesMessageIDs && toolReturnContainsMessageIDs(toolReturn.Content) {
				facts.messageIDs = true
				facts.queryScopedMessageIDs = true
			}
		}
	}
	return facts
}

func isPlainChatTurn(terms []string) bool {
	if len(terms) > 3 {
		return false
	}
	for _, term := range terms {
		switch term {
		case "hey", "hi", "hello", "yo", "thanks", "thank", "you", "ok", "okay":
		default:
			return false
		}
	}
	return true
}

func hasSummaryContext(input ChatInput) bool {
	if len(input.SelectedIDs) > 0 || len(input.VisibleIDs) > 0 {
		return true
	}
	for _, turn := range input.History {
		content := strings.ToLower(turn.Content)
		if strings.Contains(content, "message_id=") ||
			strings.Contains(content, `"message_id"`) ||
			strings.Contains(content, "sources:") {
			return true
		}
	}
	return false
}

func hasMailboxSearchIntent(terms []string) bool {
	hasRetrievalVerb := hasAnyTerm(terms, "find", "search", "pull", "show", "list", "get", "open", "locate", "look", "latest", "recent")
	return hasRetrievalVerb && hasSearchScopeOrTopic(terms)
}

func hasSearchScopeOrTopic(terms []string) bool {
	return hasMailboxHint(terms) || hasSearchTopicTerm(terms)
}

func hasMailboxHint(terms []string) bool {
	return hasAnyTerm(terms, "email", "emails", "mail", "mails", "message", "messages", "newsletter", "newsletters", "inbox", "sender", "senders", "subject", "unread", "invoice", "invoices", "receipt", "receipts", "alert", "alerts")
}

func hasSearchTopicTerm(terms []string) bool {
	for _, term := range terms {
		if !isChatIntentStopTerm(term) {
			return true
		}
	}
	return false
}

func isChatIntentStopTerm(term string) bool {
	switch term {
	case "a", "an", "and", "about", "for", "from", "in", "my", "of", "or", "the", "to", "with", "your":
		return true
	case "email", "emails", "mail", "mails", "message", "messages", "inbox", "sender", "senders", "subject", "unread":
		return true
	case "find", "search", "pull", "show", "list", "get", "open", "locate", "look", "latest", "recent":
		return true
	case "summarize", "summary", "digest", "recap", "context", "inspect", "details", "detail", "people", "person", "who":
		return true
	default:
		return false
	}
}

func hasAnyTerm(terms []string, candidates ...string) bool {
	for _, term := range terms {
		for _, candidate := range candidates {
			if term == candidate {
				return true
			}
		}
	}
	return false
}

func toolReturnContainsMessageIDs(content any) bool {
	if content == nil {
		return false
	}
	switch value := content.(type) {
	case string:
		if !strings.Contains(value, "message_id") && !strings.Contains(value, "message_ids") {
			return false
		}
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return strings.Contains(value, "message_id") || strings.Contains(value, "message_ids")
		}
		return valueContainsMessageIDs(decoded)
	default:
		return valueContainsMessageIDs(value)
	}
}

func valueContainsMessageIDs(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			switch strings.ToLower(key) {
			case "message_id", "messageid":
				if strings.TrimSpace(fmt.Sprint(nested)) != "" {
					return true
				}
			case "message_ids", "messageids":
				if valueContainsNonEmptyItem(nested) {
					return true
				}
			default:
				if valueContainsMessageIDs(nested) {
					return true
				}
			}
		}
	case []any:
		for _, nested := range typed {
			if valueContainsMessageIDs(nested) {
				return true
			}
		}
	}
	return false
}

func valueContainsNonEmptyItem(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if strings.TrimSpace(fmt.Sprint(item)) != "" {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
		return false
	default:
		return strings.TrimSpace(fmt.Sprint(typed)) != ""
	}
}

func modelMessagesContainMessageIDContext(messages []core.ModelMessage) bool {
	for _, msg := range messages {
		req, ok := msg.(core.ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			user, ok := part.(core.UserPromptPart)
			if !ok {
				continue
			}
			content := strings.ToLower(user.Content)
			if strings.Contains(content, "visible message ids:") ||
				strings.Contains(content, "selected message ids:") ||
				strings.Contains(content, "message_id=") ||
				strings.Contains(content, `"message_id"`) ||
				strings.Contains(content, "sources:") {
				return true
			}
		}
	}
	return false
}

func latestUserMessageFromModelMessages(messages []core.ModelMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		req, ok := messages[i].(core.ModelRequest)
		if !ok {
			continue
		}
		for j := len(req.Parts) - 1; j >= 0; j-- {
			part, ok := req.Parts[j].(core.UserPromptPart)
			if !ok {
				continue
			}
			content := strings.TrimSpace(part.Content)
			if idx := strings.LastIndex(content, "\nUser: "); idx >= 0 {
				return strings.TrimSpace(content[idx+len("\nUser: "):])
			}
			if strings.HasPrefix(content, "User: ") {
				return strings.TrimSpace(strings.TrimPrefix(content, "User: "))
			}
			return content
		}
	}
	return ""
}

func (r *GollemRunner) runTextFallback(ctx context.Context, input ChatInput, originalErr error) (string, error) {
	if !isStructuredOutputError(originalErr) || r.model == nil {
		return "", originalErr
	}
	modelName := r.model.ModelName()
	options := []core.AgentOption[string]{
		core.WithSystemPrompt[string](systemPrompt()),
		core.WithHooks[string](newChatAgentTelemetry(modelName, len(r.tools)).hook()),
		core.WithToolsPrepare[string](prepareChatToolsForRun),
		core.WithAgentMiddleware[string](chatToolPolicyMiddleware()),
		core.WithToolChoiceAutoReset[string](),
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
		"Use tool outputs as grounding, not as the final answer format: synthesize summaries and avoid raw message IDs, email addresses, and body snippets unless the user asks for rows or evidence.",
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
