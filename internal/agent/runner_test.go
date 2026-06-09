package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type settingsCaptureModel struct {
	settings *core.ModelSettings
	params   *core.ModelRequestParameters
}

func (m *settingsCaptureModel) Request(_ context.Context, _ []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	m.settings = settings
	m.params = nil
	return core.ToolCallResponse("final_result", `{"reply":"ok"}`), nil
}

func (m *settingsCaptureModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, errors.New("streaming is not used in this test")
}

func (m *settingsCaptureModel) ModelName() string { return "settings-capture" }

type requestCaptureModel struct {
	settings *core.ModelSettings
	params   *core.ModelRequestParameters
}

func (m *requestCaptureModel) Request(_ context.Context, _ []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	m.settings = settings
	m.params = params
	return core.ToolCallResponse("final_result", `{"reply":"ok"}`), nil
}

func (m *requestCaptureModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, errors.New("streaming is not used in this test")
}

func (m *requestCaptureModel) ModelName() string { return "request-capture" }

type requestSnapshot struct {
	toolNames  []string
	toolChoice *core.ToolChoice
}

type sequenceCaptureModel struct {
	responses []*core.ModelResponse
	requests  []requestSnapshot
}

func (m *sequenceCaptureModel) Request(_ context.Context, _ []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	snapshot := requestSnapshot{}
	if params != nil {
		snapshot.toolNames = functionToolNames(params.FunctionTools)
	}
	if settings != nil && settings.ToolChoice != nil {
		choice := *settings.ToolChoice
		snapshot.toolChoice = &choice
	}
	m.requests = append(m.requests, snapshot)
	if len(m.requests) > len(m.responses) {
		return nil, errors.New("no canned response for request")
	}
	return m.responses[len(m.requests)-1], nil
}

func (m *sequenceCaptureModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, errors.New("streaming is not used in this test")
}

func (m *sequenceCaptureModel) ModelName() string { return "sequence-capture" }

func TestGollemRunnerReturnsTypedReply(t *testing.T) {
	model := core.NewTestModel(
		core.ToolCallResponse("final_result", `{"reply":"I found the answer."}`),
	)
	runner := NewGollemRunner(model)

	result, err := runner.Run(context.Background(), ChatInput{
		UserMessage:   "summarize invoices",
		CurrentFolder: "INBOX",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Reply != "I found the answer." {
		t.Fatalf("Reply = %q", result.Reply)
	}
}

func TestGollemRunnerParsesJSONTextReplyWithoutFallback(t *testing.T) {
	model := core.NewTestModel(
		core.TextResponse(`{"reply":"I found the answer."}`),
	)
	runner := NewGollemRunner(model)

	result, err := runner.Run(context.Background(), ChatInput{
		UserMessage:   "say hello",
		CurrentFolder: "INBOX",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Reply != "I found the answer." {
		t.Fatalf("Reply = %q", result.Reply)
	}
}

func TestGollemRunnerUsesPlainTextFromFirstStructuredAttempt(t *testing.T) {
	model := core.NewTestModel(
		core.TextResponse("plain answer"),
	)
	runner := NewGollemRunner(model)

	result, err := runner.Run(context.Background(), ChatInput{
		UserMessage:   "say hello",
		CurrentFolder: "INBOX",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Reply != "plain answer" {
		t.Fatalf("Reply = %q, want first text response", result.Reply)
	}
}

func TestSystemPromptKeepsUIChatBriefAndReadOnly(t *testing.T) {
	prompt := systemPrompt()
	for _, want := range []string{
		"Herald's UI chat agent",
		"Default to 2-5 short sentences",
		"Do not send email, delete email, archive email, or mutate calendar events",
		"Use tool outputs as grounding, not as the final answer format",
		"No evidence means no factual memory answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestSystemPromptRequiresStructuredReplyObject(t *testing.T) {
	prompt := systemPrompt()
	for _, want := range []string{
		"Return a JSON object",
		"set reply",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "return only a helpful reply") {
		t.Fatalf("system prompt invites plain-text output that triggers result validation fallback:\n%s", prompt)
	}
}

func TestBuildModelSupportsInitialProviders(t *testing.T) {
	tests := []ProviderConfig{
		{Provider: "ollama", Model: "llama3.1"},
		{Provider: "anthropic", Model: "claude-sonnet-4-6", APIKey: "test-key"},
		{Provider: "openai", Model: "gpt-5-mini", APIKey: "test-key"},
		{Provider: "kimi", Model: "kimi-k2", APIKey: "test-key"},
		{Provider: "fireworks", Model: "accounts/fireworks/models/kimi-k2-instruct", APIKey: "test-key"},
	}
	for _, tt := range tests {
		t.Run(tt.Provider, func(t *testing.T) {
			model, err := BuildModel(tt)
			if err != nil {
				t.Fatalf("BuildModel returned error: %v", err)
			}
			if model == nil {
				t.Fatal("BuildModel returned nil model")
			}
		})
	}
}

func TestBuildModelRejectsUnknownProvider(t *testing.T) {
	_, err := BuildModel(ProviderConfig{Provider: "mystery"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRunnerOptionsApplyOpenAIReasoningEffort(t *testing.T) {
	model := &settingsCaptureModel{}
	runner := NewGollemRunner(
		model,
		RunnerOptionsForProviderConfig(ProviderConfig{Provider: ProviderOpenAI, ReasoningEffort: "High"})...,
	)

	if _, err := runner.Run(context.Background(), ChatInput{UserMessage: "hello"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if model.settings == nil || model.settings.ReasoningEffort == nil {
		t.Fatalf("expected reasoning effort in model settings, got %#v", model.settings)
	}
	if *model.settings.ReasoningEffort != ReasoningEffortHigh {
		t.Fatalf("reasoning effort = %q, want %q", *model.settings.ReasoningEffort, ReasoningEffortHigh)
	}
}

func TestRunnerOptionsIgnoreUnsupportedReasoningEffort(t *testing.T) {
	if got := NormalizeReasoningEffort("warp-speed"); got != "" {
		t.Fatalf("unexpected normalized effort: %q", got)
	}
	if opts := RunnerOptionsForProviderConfig(ProviderConfig{Provider: ProviderAnthropic, ReasoningEffort: ReasoningEffortHigh}); len(opts) != 0 {
		t.Fatalf("expected no reasoning-effort options for non-OpenAI provider, got %d", len(opts))
	}
}

func TestGollemRunnerHidesMailboxToolsForPlainGreeting(t *testing.T) {
	model := &requestCaptureModel{}
	source := &fakeEmailToolSource{}
	runner := NewGollemRunnerWithEmailTools(model, source, EmailToolOptions{CurrentFolder: "INBOX"})

	if _, err := runner.Run(context.Background(), ChatInput{UserMessage: "hey", CurrentFolder: "INBOX"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if model.params == nil {
		t.Fatal("model params were not captured")
	}
	if got := len(model.params.FunctionTools); got != 0 {
		t.Fatalf("plain greeting exposed %d mailbox tools, want 0", got)
	}
	if model.settings != nil && model.settings.ToolChoice != nil && model.settings.ToolChoice.ToolName != "" {
		t.Fatalf("plain greeting forced tool choice: %#v", model.settings.ToolChoice)
	}
}

func TestGollemRunnerRequiresFindEmailsForRetrievalRequest(t *testing.T) {
	model := &requestCaptureModel{}
	source := &fakeEmailToolSource{}
	runner := NewGollemRunnerWithEmailTools(model, source, EmailToolOptions{CurrentFolder: "INBOX"})

	if _, err := runner.Run(context.Background(), ChatInput{UserMessage: "pull latest Herald newsletter", CurrentFolder: "INBOX"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if model.params == nil {
		t.Fatal("model params were not captured")
	}
	if got := functionToolNames(model.params.FunctionTools); len(got) != 1 || got[0] != "find_emails" {
		t.Fatalf("retrieval request function tools = %#v, want only find_emails", got)
	}
	if model.settings == nil || model.settings.ToolChoice == nil || model.settings.ToolChoice.Mode != "required" || model.settings.ToolChoice.ToolName != "" {
		t.Fatalf("retrieval request tool choice = %#v, want required tool without a hardcoded tool name", model.settings)
	}
}

func TestGollemRunnerRequiresSummaryToolForBareSummarizeWithVisibleIDs(t *testing.T) {
	model := &requestCaptureModel{}
	source := &fakeEmailToolSource{}
	runner := NewGollemRunnerWithEmailTools(model, source, EmailToolOptions{CurrentFolder: "INBOX"})

	if _, err := runner.Run(context.Background(), ChatInput{
		UserMessage:   "summarize",
		CurrentFolder: "INBOX",
		VisibleIDs:    []string{"msg-1", "msg-2"},
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if model.params == nil {
		t.Fatal("model params were not captured")
	}
	if got := functionToolNames(model.params.FunctionTools); len(got) != 1 || got[0] != "summarize_email_set" {
		t.Fatalf("summary request function tools = %#v, want only summarize_email_set", got)
	}
	if model.settings == nil || model.settings.ToolChoice == nil || model.settings.ToolChoice.Mode != "required" || model.settings.ToolChoice.ToolName != "" {
		t.Fatalf("summary request tool choice = %#v, want required tool without a hardcoded tool name", model.settings)
	}
}

func TestGollemRunnerAdvancesCombinedRequestByToolDataflow(t *testing.T) {
	model := &sequenceCaptureModel{
		responses: []*core.ModelResponse{
			core.ToolCallResponse("find_emails", `{"query":"herald newsletter","mode":"keyword"}`),
			core.ToolCallResponse("summarize_email_set", `{"message_ids":["msg-1"],"topic":"herald newsletter"}`),
			core.ToolCallResponse("final_result", `{"reply":"Herald sent a welcome and confirmation flow."}`),
		},
	}
	source := &fakeEmailToolSource{
		keywordResults: []*models.EmailData{toolEmail("msg-1", "Herald Mail App", "Welcome to Herald Mail App Newsletter")},
		emailsByID: map[string]*models.EmailData{
			"msg-1": toolEmail("msg-1", "Herald Mail App", "Welcome to Herald Mail App Newsletter"),
		},
		bodyByID: map[string]string{
			"msg-1": "Welcome note confirming newsletter subscription.",
		},
	}
	runner := NewGollemRunnerWithEmailTools(model, source, EmailToolOptions{CurrentFolder: "INBOX"})

	result, err := runner.Run(context.Background(), ChatInput{
		UserMessage:   "find and summarize herald newsletter",
		CurrentFolder: "INBOX",
		VisibleIDs:    []string{"visible-but-not-query-scoped"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Reply == "" {
		t.Fatalf("expected final reply, got %#v", result)
	}
	if len(model.requests) != 3 {
		t.Fatalf("request count = %d, want 3: %#v", len(model.requests), model.requests)
	}
	if got := model.requests[0].toolNames; len(got) != 1 || got[0] != "find_emails" {
		t.Fatalf("first request tools = %#v, want only find_emails because no message IDs are available yet", got)
	}
	if got := model.requests[1].toolNames; len(got) != 1 || got[0] != "summarize_email_set" {
		t.Fatalf("second request tools = %#v, want only summarize_email_set after search provided message IDs", got)
	}
	if got := model.requests[2].toolNames; len(got) != 0 {
		t.Fatalf("final request tools = %#v, want none after requested capabilities are complete", got)
	}
	for i, request := range model.requests[:2] {
		if request.toolChoice == nil || request.toolChoice.Mode != "required" || request.toolChoice.ToolName != "" {
			t.Fatalf("request %d tool choice = %#v, want generic required tool choice", i, request.toolChoice)
		}
	}
}

func functionToolNames(defs []core.ToolDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}
