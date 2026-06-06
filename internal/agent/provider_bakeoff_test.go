package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestProviderContractSearchPlusSummaryScenario(t *testing.T) {
	source := &fakeEmailToolSource{
		keywordResults: []*models.EmailData{
			toolEmail("msg-1", "alice@example.com", "Budget plan"),
			toolEmail("msg-2", "bob@example.com", "Budget numbers"),
		},
		emailsByID: map[string]*models.EmailData{
			"msg-1": toolEmail("msg-1", "alice@example.com", "Budget plan"),
			"msg-2": toolEmail("msg-2", "bob@example.com", "Budget numbers"),
		},
		bodyByID: map[string]string{
			"msg-1": "Alice asked Bob to review the budget by Friday.",
			"msg-2": "Bob said the action item is to send numbers by Monday.",
		},
	}
	model := core.NewTestModel(
		core.ToolCallResponse("find_emails", `{"query":"budget","mode":"keyword"}`),
		core.ToolCallResponse("summarize_email_set", `{"message_ids":["msg-1","msg-2"],"topic":"budget"}`),
		core.ToolCallResponse("final_result", `{"reply":"Budget summary ready.","summary":{"topic":"budget","summary":"Alice owns the budget update.","action_items":["Send numbers by Monday"],"cited_ids":["msg-1","msg-2"]},"timeline":{"mode":"explicit_ids","message_ids":["msg-1","msg-2"],"label":"budget"}}`),
	)
	runner := NewGollemRunnerWithEmailTools(model, source, EmailToolOptions{MaxResults: 5, MaxContextChars: 256})

	result, err := runner.Run(context.Background(), ChatInput{UserMessage: "summarize budget emails", CurrentFolder: "INBOX"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if source.searchCalls != 1 || source.getBodyTextCalls != 2 {
		t.Fatalf("tool calls search=%d body=%d", source.searchCalls, source.getBodyTextCalls)
	}
	if result.Summary == nil || result.Summary.CitedIDs[1] != "msg-2" {
		t.Fatalf("summary = %#v", result.Summary)
	}
	if result.Timeline == nil || result.Timeline.Mode != TimelineModeExplicitIDs {
		t.Fatalf("timeline = %#v", result.Timeline)
	}
}

func TestProviderContractComposeIntentScenario(t *testing.T) {
	model := core.NewTestModel(
		core.ToolCallResponse("final_result", `{"reply":"Draft edit ready.","compose":{"subject_suggestion":"Clearer subject","body_suggestion":"Thanks for the update.\n\nI can send the numbers Monday.","rationale":"Shorter and more direct."}}`),
	)
	runner := NewGollemRunner(model)

	result, err := runner.Run(context.Background(), ChatInput{
		UserMessage: "make this clearer",
		ActiveTab:   "compose",
		ComposeSnapshot: &ComposeSnapshot{
			Subject: "Budget",
			Body:    "Original draft",
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Compose == nil || result.Compose.SubjectSuggestion != "Clearer subject" {
		t.Fatalf("compose intent = %#v", result.Compose)
	}
	if !strings.Contains(result.Compose.BodySuggestion, "Monday") {
		t.Fatalf("compose body suggestion = %q", result.Compose.BodySuggestion)
	}
}

func TestProviderContractMalformedToolArgsCanRecover(t *testing.T) {
	source := &fakeEmailToolSource{
		keywordResults: []*models.EmailData{toolEmail("msg-1", "alice@example.com", "Invoice one")},
	}
	model := core.NewTestModel(
		core.ToolCallResponse("find_emails", `{broken`),
		core.ToolCallResponse("find_emails", `{"query":"invoice","mode":"keyword"}`),
		core.ToolCallResponse("final_result", `{"reply":"Recovered after malformed args.","timeline":{"mode":"explicit_ids","message_ids":["msg-1"],"label":"invoice"}}`),
	)
	runner := NewGollemRunnerWithEmailTools(model, source, EmailToolOptions{MaxResults: 5})

	result, err := runner.Run(context.Background(), ChatInput{UserMessage: "find invoice", CurrentFolder: "INBOX"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if source.searchCalls != 1 {
		t.Fatalf("searchCalls = %d, want 1 after recovery", source.searchCalls)
	}
	if result.Timeline == nil || result.Timeline.MessageIDs[0] != "msg-1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestProviderContractRunnerReturnsProviderFailure(t *testing.T) {
	model := core.NewTestModel()
	runner := NewGollemRunner(model)

	_, err := runner.Run(context.Background(), ChatInput{UserMessage: "hello", CurrentFolder: "INBOX"})
	if err == nil {
		t.Fatal("expected provider/model failure")
	}
	if !strings.Contains(err.Error(), "no responses configured") {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderContractProviderFactoryIsSideEffectFree(t *testing.T) {
	cfgs := []ProviderConfig{
		{Provider: ProviderOllama, Model: "llama3.1", BaseURL: "http://localhost:11434"},
		{Provider: ProviderAnthropic, Model: "claude-sonnet-4-6", APIKey: "test-key"},
		{Provider: ProviderOpenAI, Model: "gpt-5-mini", APIKey: "test-key"},
		{Provider: ProviderKimi, Model: "kimi-k2", APIKey: "test-key"},
		{Provider: ProviderFireworks, Model: "accounts/fireworks/models/kimi-k2-instruct", APIKey: "test-key"},
	}
	for _, cfg := range cfgs {
		t.Run(cfg.Provider, func(t *testing.T) {
			start := time.Now()
			model, err := BuildModel(cfg)
			if err != nil {
				t.Fatalf("BuildModel returned error: %v", err)
			}
			if model == nil {
				t.Fatal("BuildModel returned nil")
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("BuildModel took %s; provider construction should not call the network", elapsed)
			}
		})
	}
}
