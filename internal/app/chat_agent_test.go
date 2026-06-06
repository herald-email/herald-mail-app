package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	"github.com/herald-email/herald-mail-app/internal/agent"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type fakeChatAgentRunner struct {
	input  agent.ChatInput
	result agent.ChatResult
	err    error
	calls  int
}

func (f *fakeChatAgentRunner) Run(_ context.Context, input agent.ChatInput) (agent.ChatResult, error) {
	f.calls++
	f.input = input
	return f.result, f.err
}

func TestSubmitChatUsesInjectedAgentRunner(t *testing.T) {
	runner := &fakeChatAgentRunner{result: agent.ChatResult{Reply: "agent reply"}}
	classifier := &stubClassifier{chatResponse: "legacy reply"}
	m := &Model{
		currentFolder: "INBOX",
		chatAgent:     runner,
		classifier:    classifier,
	}
	m.chatInput.SetValue("find invoices")

	cmd := m.submitChat()
	if cmd == nil {
		t.Fatal("submitChat returned nil command")
	}
	msg, ok := cmd().(ChatAgentResponseMsg)
	if !ok {
		t.Fatalf("submitChat command returned non-agent response")
	}
	if msg.Err != nil {
		t.Fatalf("ChatAgentResponseMsg error = %v", msg.Err)
	}
	if runner.calls != 1 {
		t.Fatalf("agent calls = %d, want 1", runner.calls)
	}
	if runner.input.UserMessage != "find invoices" {
		t.Fatalf("agent input user message = %q", runner.input.UserMessage)
	}
	if runner.input.CurrentFolder != "INBOX" {
		t.Fatalf("agent input current folder = %q", runner.input.CurrentFolder)
	}
	if len(classifier.chatMessages) != 0 {
		t.Fatalf("legacy classifier Chat was called with %d messages", len(classifier.chatMessages))
	}

	updatedModel, _ := m.Update(msg)
	updated := updatedModel.(*Model)
	if updated.chatWaiting {
		t.Fatal("chatWaiting should be false after agent response")
	}
	if len(updated.chatMessages) != 2 {
		t.Fatalf("chatMessages = %d, want user message plus agent reply", len(updated.chatMessages))
	}
	if got := updated.chatMessages[1].Content; got != "agent reply" {
		t.Fatalf("assistant message = %q", got)
	}
}

func TestSubmitChatReportsAgentUnavailableWithoutLegacyFallback(t *testing.T) {
	classifier := &stubClassifier{chatResponse: "legacy reply"}
	m := &Model{
		currentFolder: "INBOX",
		classifier:    classifier,
	}
	m.chatInput.SetValue("find invoices")

	cmd := m.submitChat()
	if cmd == nil {
		t.Fatal("submitChat returned nil command")
	}
	raw := cmd()
	msg, ok := raw.(ChatAgentResponseMsg)
	if !ok {
		t.Fatalf("submitChat command returned %T, want ChatAgentResponseMsg", raw)
	}
	if msg.Err == nil {
		t.Fatal("expected missing agent runner error")
	}
	if len(classifier.chatMessages) != 0 {
		t.Fatalf("legacy classifier Chat was called with %d messages", len(classifier.chatMessages))
	}

	updatedModel, _ := m.Update(msg)
	updated := updatedModel.(*Model)
	if updated.chatWaiting {
		t.Fatal("chatWaiting should be false after agent unavailable response")
	}
	if len(updated.chatMessages) != 2 {
		t.Fatalf("chatMessages = %d, want user message plus agent error", len(updated.chatMessages))
	}
	if updated.chatMessages[1].Role != "assistant" || !strings.Contains(updated.chatMessages[1].Content, "Chat agent unavailable") {
		t.Fatalf("assistant message = %#v", updated.chatMessages[1])
	}
}

func TestSubmitChatIgnoresEmptyInputWithAgentRunner(t *testing.T) {
	runner := &fakeChatAgentRunner{result: agent.ChatResult{Reply: "agent reply"}}
	m := &Model{chatAgent: runner}
	m.chatInput.SetValue("   ")

	if cmd := m.submitChat(); cmd != nil {
		t.Fatal("empty chat input returned command")
	}
	if runner.calls != 0 {
		t.Fatalf("agent calls = %d, want 0", runner.calls)
	}
	if m.chatWaiting {
		t.Fatal("chatWaiting changed for empty input")
	}
}

func TestRenderChatPanelUsesEffectiveWidthForWrappingAndInput(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.showChat = true
	m.chatMessages = []ai.ChatMessage{{
		Role:    "assistant",
		Content: strings.Repeat("wide response ", 12),
	}}
	m.chatWrappedWidth = chatPanelMinWidth
	m.chatInput.SetValue("follow up")

	rendered := stripANSI(m.renderChatPanel())

	if got, want := m.chatWrappedWidth, chatPanelMaxWidth; got != want {
		t.Fatalf("chat wrap width = %d, want effective width %d", got, want)
	}
	minInputWidth := chatPanelMinWidth - len(m.chatInput.Prompt)
	if got := m.chatInput.Width(); got <= minInputWidth {
		t.Fatalf("chat input width = %d, want wider than fixed-width baseline %d", got, minInputWidth)
	}
	if !strings.Contains(rendered, strings.Repeat("─", chatPanelMaxWidth)) {
		t.Fatalf("rendered chat panel did not use effective divider width %d:\n%s", chatPanelMaxWidth, rendered)
	}
}

func TestChatAgentResponseErrorAppendsBoundedAssistantMessage(t *testing.T) {
	m := &Model{}
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{Role: "user", Content: "hello"})
	m.chatWaiting = true

	updatedModel, _ := m.Update(ChatAgentResponseMsg{Err: context.Canceled})
	updated := updatedModel.(*Model)
	if updated.chatWaiting {
		t.Fatal("chatWaiting should be false after agent error")
	}
	if len(updated.chatMessages) != 2 {
		t.Fatalf("chatMessages = %d, want user message plus error", len(updated.chatMessages))
	}
	if got := updated.chatMessages[1].Role; got != "assistant" {
		t.Fatalf("assistant role = %q", got)
	}
	if got := updated.chatMessages[1].Content; got != "Error: context canceled" {
		t.Fatalf("assistant error content = %q", got)
	}
}

func TestChatAgentResponseUsesFallbackForEmptyReply(t *testing.T) {
	m := &Model{}
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{Role: "user", Content: "hello"})
	m.chatWaiting = true

	updatedModel, _ := m.Update(ChatAgentResponseMsg{Result: agent.ChatResult{}})
	updated := updatedModel.(*Model)
	if updated.chatWaiting {
		t.Fatal("chatWaiting should be false after empty agent response")
	}
	if len(updated.chatMessages) != 2 {
		t.Fatalf("chatMessages = %d, want user message plus fallback reply", len(updated.chatMessages))
	}
	if got := updated.chatMessages[1].Content; got != "No agent response." {
		t.Fatalf("assistant fallback content = %q", got)
	}
}

func TestSetConfigEnablesGollemChatAgentByDefaultWhenAIConfigured(t *testing.T) {
	m := &Model{chatAgent: &fakeChatAgentRunner{}}

	m.SetConfig(&config.Config{})

	if m.chatAgent == nil {
		t.Fatal("chatAgent should default to Gollem when global AI is configured")
	}
}

func TestSetConfigKeepsChatAgentDisabledWhenAIDisabled(t *testing.T) {
	m := &Model{chatAgent: &fakeChatAgentRunner{}}

	cfg := &config.Config{}
	cfg.AI.Provider = "disabled"
	m.SetConfig(cfg)

	if m.chatAgent != nil {
		t.Fatal("chatAgent should be nil when global AI is disabled")
	}
}

func TestSetConfigEnablesGollemChatAgentWhenConfigured(t *testing.T) {
	cfg := &config.Config{}
	cfg.AI.Agent.Provider = "ollama"
	cfg.AI.Agent.Model = "llama3.1"

	m := &Model{}
	m.SetConfig(cfg)

	if m.chatAgent == nil {
		t.Fatal("chatAgent should be configured from ai.agent provider overrides")
	}
}

func TestChatAgentProviderConfigInheritsGlobalAIProviders(t *testing.T) {
	claudeCfg := &config.Config{}
	claudeCfg.AI.Provider = "claude"
	claudeCfg.Claude.Model = "claude-sonnet-4-6"
	claudeCfg.Claude.APIKey = "anthropic-key"
	claudeProvider := chatAgentProviderConfig(claudeCfg)
	if claudeProvider.Provider != agent.ProviderAnthropic {
		t.Fatalf("claude provider = %q, want anthropic", claudeProvider.Provider)
	}
	if claudeProvider.Model != "claude-sonnet-4-6" || claudeProvider.APIKey != "anthropic-key" {
		t.Fatalf("claude provider config = %#v", claudeProvider)
	}

	openAICfg := &config.Config{}
	openAICfg.AI.Provider = "openai"
	openAICfg.OpenAI.Model = "gpt-5-mini"
	openAICfg.OpenAI.APIKey = "openai-key"
	openAICfg.OpenAI.BaseURL = "https://example.test/v1"
	openAICfg.AI.Agent.ReasoningEffort = "medium"
	openAIProvider := chatAgentProviderConfig(openAICfg)
	if openAIProvider.Provider != agent.ProviderOpenAI {
		t.Fatalf("openai provider = %q, want openai", openAIProvider.Provider)
	}
	if openAIProvider.Model != "gpt-5-mini" || openAIProvider.APIKey != "openai-key" || openAIProvider.BaseURL != "https://example.test/v1" {
		t.Fatalf("openai provider config = %#v", openAIProvider)
	}
	if openAIProvider.ReasoningEffort != agent.ReasoningEffortMedium {
		t.Fatalf("openai reasoning effort = %q, want %q", openAIProvider.ReasoningEffort, agent.ReasoningEffortMedium)
	}
}

func TestChatAgentProviderConfigDefaultsOpenAIReasoningEffortLow(t *testing.T) {
	openAICfg := &config.Config{}
	openAICfg.AI.Provider = "openai"

	openAIProvider := chatAgentProviderConfig(openAICfg)
	if openAIProvider.Model != "gpt-5-mini" {
		t.Fatalf("openai model = %q, want gpt-5-mini", openAIProvider.Model)
	}
	if openAIProvider.ReasoningEffort != agent.ReasoningEffortLow {
		t.Fatalf("openai reasoning effort = %q, want %q", openAIProvider.ReasoningEffort, agent.ReasoningEffortLow)
	}
}

func TestBuildChatAgentInputSnapshotsBoundedUIContext(t *testing.T) {
	m := &Model{
		activeTab: tabCompose,
		timeline: TimelineState{
			selectedMessageIDs: map[string]bool{
				"msg-2": true,
				"msg-1": true,
				"msg-3": false,
			},
		},
	}
	m.timeline.emails = []*models.EmailData{
		{MessageID: "visible-1", Date: time.Now()},
		{MessageID: "visible-2", Date: time.Now()},
	}
	m.composeTo.SetValue("to@example.com")
	m.composeCC.SetValue("cc@example.com")
	m.composeBCC.SetValue("bcc@example.com")
	m.composeSubject.SetValue("Draft subject")
	m.composeBody = textarea.New()
	m.composeBody.SetValue("Draft body")
	history := []ai.ChatMessage{
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
	}

	input := m.buildChatAgentInput("new question", "INBOX", history)

	if input.UserMessage != "new question" {
		t.Fatalf("UserMessage = %q", input.UserMessage)
	}
	if input.CurrentFolder != "INBOX" {
		t.Fatalf("CurrentFolder = %q", input.CurrentFolder)
	}
	if input.ActiveTab != "compose" {
		t.Fatalf("ActiveTab = %q", input.ActiveTab)
	}
	if len(input.VisibleIDs) != 2 || input.VisibleIDs[0] != "visible-1" || input.VisibleIDs[1] != "visible-2" {
		t.Fatalf("VisibleIDs = %#v", input.VisibleIDs)
	}
	if len(input.SelectedIDs) != 2 || input.SelectedIDs[0] != "msg-1" || input.SelectedIDs[1] != "msg-2" {
		t.Fatalf("SelectedIDs = %#v", input.SelectedIDs)
	}
	if input.ComposeSnapshot == nil {
		t.Fatal("ComposeSnapshot is nil")
	}
	if input.ComposeSnapshot.Subject != "Draft subject" || input.ComposeSnapshot.Body != "Draft body" {
		t.Fatalf("ComposeSnapshot = %#v", input.ComposeSnapshot)
	}
	if len(input.History) != 2 || input.History[0].Content != "old question" || input.History[1].Content != "old answer" {
		t.Fatalf("History = %#v", input.History)
	}
}

func TestBuildChatAgentInputBoundsLargeContext(t *testing.T) {
	m := &Model{activeTab: tabCompose}
	for i := 0; i < chatAgentMaxVisibleIDs+10; i++ {
		m.timeline.emails = append(m.timeline.emails, &models.EmailData{MessageID: "visible"})
	}
	m.composeBody = textarea.New()
	m.composeBody.SetValue(strings.Repeat("b", chatAgentMaxComposeBodyChars+100))
	history := []ai.ChatMessage{
		{Role: "user", Content: strings.Repeat("h", chatAgentMaxHistoryChars+100)},
	}

	input := m.buildChatAgentInput(strings.Repeat("q", chatAgentMaxUserMessageChars+100), "INBOX", history)

	if len(input.VisibleIDs) != chatAgentMaxVisibleIDs {
		t.Fatalf("VisibleIDs length = %d, want %d", len(input.VisibleIDs), chatAgentMaxVisibleIDs)
	}
	if got := len([]rune(strings.TrimSuffix(input.UserMessage, "...[truncated]"))); got != chatAgentMaxUserMessageChars {
		t.Fatalf("bounded user message length = %d, want %d", got, chatAgentMaxUserMessageChars)
	}
	if got := len([]rune(strings.TrimSuffix(input.ComposeSnapshot.Body, "...[truncated]"))); got != chatAgentMaxComposeBodyChars {
		t.Fatalf("bounded compose body length = %d, want %d", got, chatAgentMaxComposeBodyChars)
	}
	if got := len([]rune(strings.TrimSuffix(input.History[0].Content, "...[truncated]"))); got != chatAgentMaxHistoryChars {
		t.Fatalf("bounded history length = %d, want %d", got, chatAgentMaxHistoryChars)
	}
}

func TestBuildChatAgentInputUsesDisplayedTimelineRows(t *testing.T) {
	m := &Model{activeTab: tabTimeline}
	m.timeline.emails = []*models.EmailData{{MessageID: "base-1"}, {MessageID: "base-2"}}
	m.timeline.searchMode = true
	m.timeline.searchResults = []*models.EmailData{{MessageID: "search-1"}}

	input := m.buildChatAgentInput("question", "INBOX", nil)
	if len(input.VisibleIDs) != 1 || input.VisibleIDs[0] != "search-1" {
		t.Fatalf("VisibleIDs from search mode = %#v", input.VisibleIDs)
	}

	m.timeline.chatFilterMode = true
	m.timeline.chatFilteredEmails = []*models.EmailData{{MessageID: "filter-1"}, {MessageID: "filter-2"}}
	input = m.buildChatAgentInput("question", "INBOX", nil)
	if len(input.VisibleIDs) != 2 || input.VisibleIDs[0] != "filter-1" || input.VisibleIDs[1] != "filter-2" {
		t.Fatalf("VisibleIDs from chat filter = %#v", input.VisibleIDs)
	}
}

func TestChatAgentTimelineIntentExplicitIDsProjectsValidatedEmails(t *testing.T) {
	m := makeModelWithEmails()
	m.activeTab = tabContacts

	updatedModel, cmd := m.Update(ChatAgentResponseMsg{Result: agent.ChatResult{
		Reply: "Found the invoice.",
		Timeline: &agent.TimelineIntent{
			Mode:       agent.TimelineModeExplicitIDs,
			MessageIDs: []string{"msg-2", "missing", "msg-1"},
			Label:      "agent picks",
		},
	}})
	if cmd != nil {
		t.Fatalf("explicit ID projection should be synchronous, got command %T", cmd)
	}
	updated := updatedModel.(*Model)
	if !updated.timeline.chatFilterMode {
		t.Fatal("expected typed Timeline intent to enable projected results")
	}
	if updated.timeline.chatFilterLabel != "agent picks" {
		t.Fatalf("chatFilterLabel = %q", updated.timeline.chatFilterLabel)
	}
	if len(updated.timeline.chatFilteredEmails) != 2 {
		t.Fatalf("filtered emails = %#v", updated.timeline.chatFilteredEmails)
	}
	if updated.timeline.chatFilteredEmails[0].MessageID != "msg-2" || updated.timeline.chatFilteredEmails[1].MessageID != "msg-1" {
		t.Fatalf("filtered IDs = %#v", updated.timeline.chatFilteredEmails)
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab = %d, want Timeline", updated.activeTab)
	}
}

func TestChatAgentTimelineIntentKeywordRoutesThroughSearchPipeline(t *testing.T) {
	backend := &stubBackend{
		searchResult: []*models.EmailData{
			{MessageID: "invoice-1", Sender: "billing@example.com", Subject: "Invoice", Date: time.Now(), Folder: "INBOX"},
		},
	}
	m := New(backend, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabTimeline
	m.currentFolder = "INBOX"
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updatedModel, cmd := m.Update(ChatAgentResponseMsg{Result: agent.ChatResult{
		Reply: "Searching invoices.",
		Timeline: &agent.TimelineIntent{
			Mode:  agent.TimelineModeKeyword,
			Query: "invoice",
			Label: "invoices",
		},
	}})
	if cmd == nil {
		t.Fatal("keyword Timeline intent should return a search command")
	}
	updated := updatedModel.(*Model)
	if !updated.timeline.searchMode {
		t.Fatal("expected search mode to open for keyword intent")
	}
	if got := updated.timeline.searchInput.Value(); got != "/k invoice" {
		t.Fatalf("search query = %q", got)
	}

	msg := cmd()
	searchMsg, ok := msg.(SearchResultMsg)
	if !ok {
		t.Fatalf("search command returned %T", msg)
	}
	if searchMsg.Token != updated.timeline.searchToken {
		t.Fatalf("search token = %d, want %d", searchMsg.Token, updated.timeline.searchToken)
	}
	if searchMsg.Source != "local" {
		t.Fatalf("search source = %q", searchMsg.Source)
	}
	finalModel, _ := updated.Update(searchMsg)
	final := finalModel.(*Model)
	if len(final.timeline.searchResults) != 1 || final.timeline.searchResults[0].MessageID != "invoice-1" {
		t.Fatalf("search results = %#v", final.timeline.searchResults)
	}
	if final.timeline.searchFocus != timelineSearchFocusResults {
		t.Fatalf("searchFocus = %d, want results", final.timeline.searchFocus)
	}
}

func TestChatAgentSummaryFormatsSourceBackedDetails(t *testing.T) {
	m := &Model{}
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{Role: "user", Content: "summarize"})
	m.chatWaiting = true

	updatedModel, _ := m.Update(ChatAgentResponseMsg{Result: agent.ChatResult{
		Reply: "Here is the short version.",
		Summary: &agent.EmailSummary{
			Topic:       "budget",
			Summary:     "Alice owns the budget update.",
			ActionItems: []string{"Send numbers by Monday"},
			OpenQuestions: []string{
				"Who approves the final total?",
			},
			CitedIDs: []string{"msg-1", "msg-2"},
			People: []agent.Person{{
				NameOrEmail: "alice@example.com",
				Role:        "sender",
				EvidenceID:  "msg-1",
			}},
		},
	}})
	updated := updatedModel.(*Model)
	content := updated.chatMessages[len(updated.chatMessages)-1].Content
	for _, want := range []string{"Here is the short version.", "Summary: Alice owns", "People:", "Action items:", "Open questions:", "Sources: msg-1, msg-2"} {
		if !strings.Contains(content, want) {
			t.Fatalf("assistant content missing %q:\n%s", want, content)
		}
	}
}

func TestChatAgentResponseTreatsFilterMarkupAsAssistantText(t *testing.T) {
	m := makeModelWithEmails()
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{Role: "user", Content: "show invoices"})
	m.chatWaiting = true

	updatedModel, cmd := m.Update(ChatAgentResponseMsg{Result: agent.ChatResult{
		Reply: `Here is literal markup: <filter>{"ids":["msg-1"],"label":"invoices"}</filter>`,
	}})
	if cmd != nil {
		t.Fatalf("plain assistant markup should not produce Timeline command, got %T", cmd)
	}
	updated := updatedModel.(*Model)
	if updated.timeline.chatFilterMode {
		t.Fatal("literal filter markup should not activate Timeline chat filtering")
	}
	content := updated.chatMessages[len(updated.chatMessages)-1].Content
	if !strings.Contains(content, "<filter>") {
		t.Fatalf("assistant content should preserve literal markup, got %q", content)
	}
}

func TestChatAgentComposeIntentOpensReviewWithoutMutatingDraft(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeBody.SetValue("Original body")
	m.composeSubject.SetValue("Original subject")

	updatedModel, _ := m.Update(ChatAgentResponseMsg{Result: agent.ChatResult{
		Reply: "I drafted an edit for review.",
		Compose: &agent.ComposeIntent{
			SubjectSuggestion: "Better subject",
			BodySuggestion:    "Suggested body",
			Rationale:         "Clearer and shorter.",
		},
	}})
	updated := updatedModel.(*Model)
	if got := updated.composeBody.Value(); got != "Original body" {
		t.Fatalf("compose body mutated to %q", got)
	}
	if got := updated.composeSubject.Value(); got != "Original subject" {
		t.Fatalf("compose subject mutated to %q", got)
	}
	if !updated.composeAIReviewActive() {
		t.Fatal("expected compose AI review to open")
	}
	if got := updated.composeAIResponse.Value(); got != "Suggested body" {
		t.Fatalf("compose AI response = %q", got)
	}
	if got := updated.composeAISubjectHint; got != "Better subject" {
		t.Fatalf("compose subject hint = %q", got)
	}
	if !strings.Contains(updated.statusMessage, "review") {
		t.Fatalf("statusMessage = %q", updated.statusMessage)
	}
}

func TestChatAgentComposeIntentOutsideComposeShowsNotice(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline

	updatedModel, _ := m.Update(ChatAgentResponseMsg{Result: agent.ChatResult{
		Reply: "I can draft this when Compose is active.",
		Compose: &agent.ComposeIntent{
			BodySuggestion: "Suggested body",
		},
	}})
	updated := updatedModel.(*Model)
	if updated.composeAIReviewActive() {
		t.Fatal("compose review should not open outside Compose")
	}
	if !strings.Contains(updated.statusMessage, "Open Compose") {
		t.Fatalf("statusMessage = %q", updated.statusMessage)
	}
}
