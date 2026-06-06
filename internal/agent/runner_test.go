package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

type settingsCaptureModel struct {
	settings *core.ModelSettings
}

func (m *settingsCaptureModel) Request(_ context.Context, _ []core.ModelMessage, settings *core.ModelSettings, _ *core.ModelRequestParameters) (*core.ModelResponse, error) {
	m.settings = settings
	return core.ToolCallResponse("final_result", `{"reply":"ok"}`), nil
}

func (m *settingsCaptureModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, errors.New("streaming is not used in this test")
}

func (m *settingsCaptureModel) ModelName() string { return "settings-capture" }

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

func TestGollemRunnerFallsBackToPlainTextWhenTypedResultFails(t *testing.T) {
	model := core.NewTestModel(
		core.TextResponse("plain answer"),
		core.TextResponse("plain answer after repair"),
		core.TextResponse("plain answer from fallback"),
	)
	runner := NewGollemRunner(model)

	result, err := runner.Run(context.Background(), ChatInput{
		UserMessage:   "say hello",
		CurrentFolder: "INBOX",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Reply != "plain answer from fallback" {
		t.Fatalf("Reply = %q, want fallback text", result.Reply)
	}
}

func TestSystemPromptKeepsUIChatBriefAndReadOnly(t *testing.T) {
	prompt := systemPrompt()
	for _, want := range []string{
		"Herald's UI chat agent",
		"Default to 2-5 short sentences",
		"Do not send email, delete email, archive email, or mutate calendar events",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
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
