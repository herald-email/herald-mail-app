package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestLiveOpenAISmoke(t *testing.T) {
	if os.Getenv("HERALD_LIVE_OPENAI_SMOKE") != "1" {
		t.Skip("set HERALD_LIVE_OPENAI_SMOKE=1 to run the live OpenAI smoke")
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Fatal("OPENAI_API_KEY is required for live OpenAI smoke")
	}
	modelName := strings.TrimSpace(os.Getenv("HERALD_OPENAI_SMOKE_MODEL"))
	if modelName == "" {
		modelName = "gpt-5-mini"
	}
	reasoningEffort := NormalizeReasoningEffort(os.Getenv("HERALD_OPENAI_SMOKE_REASONING_EFFORT"))
	if reasoningEffort == "" {
		reasoningEffort = ReasoningEffortLow
	}

	providerCfg := ProviderConfig{
		Provider:        ProviderOpenAI,
		Model:           modelName,
		APIKey:          apiKey,
		ReasoningEffort: reasoningEffort,
	}
	model, err := BuildModel(providerCfg)
	if err != nil {
		t.Fatalf("BuildModel returned error: %v", err)
	}

	t.Run("plain reply", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		runner := NewGollemRunner(model, RunnerOptionsForProviderConfig(providerCfg)...)

		result, err := runner.Run(ctx, ChatInput{
			UserMessage:   "Reply briefly with the phrase herald-openai-smoke-ok and no private data.",
			CurrentFolder: "INBOX",
		})
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if !strings.Contains(strings.ToLower(result.Reply), "herald-openai-smoke-ok") {
			t.Fatalf("reply did not include smoke marker: %#v", result)
		}
	})

	t.Run("tool search summary timeline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		source := &fakeEmailToolSource{
			keywordResults: []*models.EmailData{
				toolEmail("budget-1", "alice@example.com", "Budget plan"),
				toolEmail("budget-2", "bob@example.com", "Budget numbers"),
			},
			emailsByID: map[string]*models.EmailData{
				"budget-1": toolEmail("budget-1", "alice@example.com", "Budget plan"),
				"budget-2": toolEmail("budget-2", "bob@example.com", "Budget numbers"),
			},
			bodyByID: map[string]string{
				"budget-1": "Alice asked Bob to review the budget by Friday.",
				"budget-2": "Bob said the action item is to send final numbers by Monday.",
			},
		}
		runner := NewGollemRunnerWithEmailToolsAndOptions(
			model,
			source,
			EmailToolOptions{MaxResults: 5, MaxContextChars: 512},
			RunnerOptionsForProviderConfig(providerCfg)...,
		)

		result, err := runner.Run(ctx, ChatInput{
			UserMessage:   "Use the email tools to find budget emails, summarize them, cite the message IDs, and return an explicit_ids timeline intent for the matching messages.",
			CurrentFolder: "INBOX",
			ActiveTab:     "timeline",
		})
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if source.searchCalls == 0 {
			t.Fatalf("expected find_emails tool call, got result %#v", result)
		}
		if result.Timeline == nil || result.Timeline.Mode != TimelineModeExplicitIDs || len(result.Timeline.MessageIDs) == 0 {
			t.Fatalf("expected explicit timeline intent, got %#v", result)
		}
		if result.Summary == nil || len(result.Summary.CitedIDs) == 0 {
			t.Fatalf("expected cited summary, got %#v", result)
		}
	})
}
