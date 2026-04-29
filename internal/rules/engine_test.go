package rules

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// --- mock Store ---

type mockStore struct {
	rules           []*models.Rule
	prompts         map[int64]*models.CustomPrompt
	log             []*models.RuleActionLogEntry
	savedCategories map[string]string // "messageID:promptID" -> result
}

func (m *mockStore) GetEnabledRules() ([]*models.Rule, error) { return m.rules, nil }

func (m *mockStore) GetCustomPrompt(id int64) (*models.CustomPrompt, error) {
	if m.prompts != nil {
		if p, ok := m.prompts[id]; ok {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockStore) SaveCustomCategory(messageID string, promptID int64, result string) error {
	if m.savedCategories == nil {
		m.savedCategories = make(map[string]string)
	}
	key := fmt.Sprintf("%s:%d", messageID, promptID)
	m.savedCategories[key] = result
	return nil
}

func (m *mockStore) AppendActionLog(e *models.RuleActionLogEntry) error {
	m.log = append(m.log, e)
	return nil
}

func (m *mockStore) TouchRuleLastTriggered(int64) error { return nil }

// --- mock Executor ---

type mockExecutor struct {
	moved     []string
	archived  []string
	deleted   []string
	errOnMove error
}

func (m *mockExecutor) MoveEmail(messageID, from, to string) error {
	if m.errOnMove != nil {
		return m.errOnMove
	}
	m.moved = append(m.moved, messageID)
	return nil
}

func (m *mockExecutor) ArchiveEmail(messageID, folder string) error {
	m.archived = append(m.archived, messageID)
	return nil
}

func (m *mockExecutor) DeleteEmail(messageID, folder string) error {
	m.deleted = append(m.deleted, messageID)
	return nil
}

// --- helpers ---

func makeEmail(sender, subject, folder, messageID string) *models.EmailData {
	return &models.EmailData{
		MessageID: messageID,
		Sender:    sender,
		Subject:   subject,
		Folder:    folder,
		Date:      time.Now(),
	}
}

func makeRule(id int64, triggerType models.RuleTriggerType, triggerValue string, actions ...models.RuleAction) *models.Rule {
	return &models.Rule{
		ID:           id,
		Name:         "test-rule",
		Enabled:      true,
		TriggerType:  triggerType,
		TriggerValue: triggerValue,
		Actions:      actions,
	}
}

// --- matchRule tests ---

func TestMatchRule_Sender(t *testing.T) {
	email := makeEmail("Alice <alice@example.com>", "Hi", "INBOX", "msg1")
	rule := makeRule(1, models.TriggerSender, "Alice <alice@example.com>")

	if !MatchRule(rule, email, "") {
		t.Error("expected exact sender match to return true")
	}

	// case-insensitive
	rule.TriggerValue = "alice <ALICE@EXAMPLE.COM>"
	if !MatchRule(rule, email, "") {
		t.Error("expected case-insensitive sender match to return true")
	}

	rule.TriggerValue = "bob@example.com"
	if MatchRule(rule, email, "") {
		t.Error("expected different sender to return false")
	}
}

func TestMatchRule_Domain(t *testing.T) {
	email := makeEmail("Name <user@example.com>", "Hi", "INBOX", "msg1")
	rule := makeRule(1, models.TriggerDomain, "example.com")

	if !MatchRule(rule, email, "") {
		t.Error("expected domain match to return true")
	}

	// case-insensitive
	rule.TriggerValue = "EXAMPLE.COM"
	if !MatchRule(rule, email, "") {
		t.Error("expected case-insensitive domain match to return true")
	}

	rule.TriggerValue = "other.com"
	if MatchRule(rule, email, "") {
		t.Error("expected different domain to return false")
	}
}

func TestMatchRule_Category(t *testing.T) {
	email := makeEmail("sender@example.com", "Hi", "INBOX", "msg1")
	rule := makeRule(1, models.TriggerCategory, "news")

	if !MatchRule(rule, email, "news") {
		t.Error("expected category match to return true")
	}

	// case-insensitive
	if !MatchRule(rule, email, "NEWS") {
		t.Error("expected case-insensitive category match to return true")
	}

	if MatchRule(rule, email, "spam") {
		t.Error("expected different category to return false")
	}
}

func TestMatchRule_Unknown(t *testing.T) {
	email := makeEmail("sender@example.com", "Hi", "INBOX", "msg1")
	rule := makeRule(1, models.RuleTriggerType("bogus"), "anything")

	if MatchRule(rule, email, "") {
		t.Error("expected unknown trigger type to return false")
	}
}

// --- EvaluateEmail tests ---

func TestEvaluateEmail_FiresMatchingRule(t *testing.T) {
	store := &mockStore{
		rules: []*models.Rule{
			makeRule(1, models.TriggerSender, "alice@example.com",
				models.RuleAction{Type: models.ActionMove, DestFolder: "Archive"}),
		},
	}
	exec := &mockExecutor{}
	engine := New(store, exec, nil)

	email := makeEmail("alice@example.com", "Hello", "INBOX", "msg-1")
	fired, err := engine.EvaluateEmail(email, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fired != 1 {
		t.Errorf("expected fired=1, got %d", fired)
	}
	if len(exec.moved) != 1 || exec.moved[0] != "msg-1" {
		t.Errorf("expected MoveEmail called with msg-1, got %v", exec.moved)
	}
	if len(store.log) != 1 {
		t.Errorf("expected 1 action log entry, got %d", len(store.log))
	}
	if store.log[0].Status != "ok" {
		t.Errorf("expected log status ok, got %s", store.log[0].Status)
	}
}

func TestEvaluateEmail_NoMatch(t *testing.T) {
	store := &mockStore{
		rules: []*models.Rule{
			makeRule(1, models.TriggerSender, "bob@example.com",
				models.RuleAction{Type: models.ActionMove, DestFolder: "Archive"}),
		},
	}
	exec := &mockExecutor{}
	engine := New(store, exec, nil)

	email := makeEmail("alice@example.com", "Hello", "INBOX", "msg-1")
	fired, err := engine.EvaluateEmail(email, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fired != 0 {
		t.Errorf("expected fired=0, got %d", fired)
	}
	if len(exec.moved) != 0 {
		t.Errorf("expected no moves, got %v", exec.moved)
	}
}

func TestEvaluateEmail_MultipleRules(t *testing.T) {
	store := &mockStore{
		rules: []*models.Rule{
			// matches
			makeRule(1, models.TriggerSender, "alice@example.com",
				models.RuleAction{Type: models.ActionMove, DestFolder: "Archive"}),
			// does not match
			makeRule(2, models.TriggerDomain, "other.com",
				models.RuleAction{Type: models.ActionDelete}),
		},
	}
	exec := &mockExecutor{}
	engine := New(store, exec, nil)

	email := makeEmail("alice@example.com", "Hello", "INBOX", "msg-1")
	fired, err := engine.EvaluateEmail(email, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fired != 1 {
		t.Errorf("expected fired=1, got %d", fired)
	}
	if len(exec.moved) != 1 {
		t.Errorf("expected 1 move, got %d", len(exec.moved))
	}
	if len(exec.deleted) != 0 {
		t.Errorf("expected 0 deletes, got %d", len(exec.deleted))
	}
}

func TestEvaluateEmail_ActionError(t *testing.T) {
	store := &mockStore{
		rules: []*models.Rule{
			makeRule(1, models.TriggerSender, "alice@example.com",
				models.RuleAction{Type: models.ActionMove, DestFolder: "Archive"}),
		},
	}
	moveErr := errors.New("IMAP move failed")
	exec := &mockExecutor{errOnMove: moveErr}
	engine := New(store, exec, nil)

	email := makeEmail("alice@example.com", "Hello", "INBOX", "msg-1")
	fired, err := engine.EvaluateEmail(email, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, moveErr) {
		t.Errorf("expected moveErr, got %v", err)
	}
	if fired != 1 {
		t.Errorf("expected fired=1 even on error, got %d", fired)
	}
	if len(store.log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.log))
	}
	if store.log[0].Status != "error" {
		t.Errorf("expected log status error, got %s", store.log[0].Status)
	}
	if store.log[0].Detail == "" {
		t.Error("expected non-empty detail in log entry")
	}
}

// --- extractDomain tests ---

func TestExtractDomain(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"user@example.com", "example.com"},
		{"Name <user@example.com>", "example.com"},
		{"CAPS@EXAMPLE.COM", "example.com"},
		{"nodomain", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := extractDomain(tc.input)
		if got != tc.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- mock AI client ---

type mockAI struct {
	response string
	err      error
}

func (m *mockAI) Chat(messages []ai.ChatMessage) (string, error) {
	return m.response, m.err
}
func (m *mockAI) ChatWithTools(messages []ai.ChatMessage, tools []ai.Tool) (string, []ai.ToolCall, error) {
	return m.response, nil, m.err
}
func (m *mockAI) Classify(sender, subject string) (ai.Category, error) { return "", nil }
func (m *mockAI) Embed(text string) ([]float32, error)                 { return nil, nil }
func (m *mockAI) SetEmbeddingModel(model string)                       {}
func (m *mockAI) GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error) {
	return nil, nil
}
func (m *mockAI) EnrichContact(email string, subjects []string) (string, []string, error) {
	return "", nil, nil
}
func (m *mockAI) HasVisionModel() bool { return false }
func (m *mockAI) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	return "", nil
}
func (m *mockAI) Ping() error { return nil }

// --- TestRunCustomPrompt ---

func TestRunCustomPrompt(t *testing.T) {
	const wantResult = "high urgency"
	mockClient := &mockAI{response: wantResult}

	promptID := int64(1)
	store := &mockStore{
		rules: []*models.Rule{
			{
				ID:             1,
				Name:           "urgency-rule",
				Enabled:        true,
				TriggerType:    models.TriggerSender,
				TriggerValue:   "alice@example.com",
				CustomPromptID: &promptID,
				Actions:        []models.RuleAction{{Type: models.ActionMove, DestFolder: "Urgent"}},
			},
		},
		prompts: map[int64]*models.CustomPrompt{
			1: {
				ID:           1,
				Name:         "urgency",
				SystemText:   "Rate urgency as high, medium, or low.",
				UserTemplate: "From: {{.Sender}}\nSubject: {{.Subject}}",
				OutputVar:    "urgency",
			},
		},
	}
	exec := &mockExecutor{}
	engine := New(store, exec, mockClient)

	email := makeEmail("alice@example.com", "Urgent: action required", "INBOX", "msg-custom-1")
	fired, err := engine.EvaluateEmail(email, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fired != 1 {
		t.Errorf("expected fired=1, got %d", fired)
	}

	// Verify the result was saved in the mock store
	key := fmt.Sprintf("%s:%d", email.MessageID, promptID)
	got, ok := store.savedCategories[key]
	if !ok {
		t.Fatalf("expected custom category to be saved for key %q", key)
	}
	if got != wantResult {
		t.Errorf("saved result: got %q, want %q", got, wantResult)
	}
}

func TestDryRunRulesEngine(t *testing.T) {
	store := &mockStore{
		rules: []*models.Rule{
			makeRule(1, models.TriggerSender, "alice@example.com",
				models.RuleAction{Type: models.ActionDelete}),
		},
	}
	exec := &mockExecutor{}
	engine := New(store, exec, nil)
	engine.DryRun = true

	email := makeEmail("alice@example.com", "Hello", "INBOX", "msg-dry-1")
	fired, err := engine.EvaluateEmail(email, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Rule should still be counted as fired (condition matched)
	if fired != 1 {
		t.Errorf("expected fired=1, got %d", fired)
	}
	// Backend delete must NOT have been called
	if len(exec.deleted) != 0 {
		t.Errorf("dry-run: DeleteEmail should not be called, got %v", exec.deleted)
	}
	// Action log entry should record status "ok" (dry-run returns nil error)
	if len(store.log) != 1 {
		t.Fatalf("expected 1 action log entry, got %d", len(store.log))
	}
	if store.log[0].Status != "ok" {
		t.Errorf("expected log status ok in dry-run, got %s", store.log[0].Status)
	}
}

func TestRunCustomPrompt_AIError(t *testing.T) {
	aiErr := errors.New("AI unavailable")
	mockClient := &mockAI{err: aiErr}

	promptID := int64(1)
	store := &mockStore{
		rules: []*models.Rule{
			{
				ID:             1,
				Name:           "urgency-rule",
				Enabled:        true,
				TriggerType:    models.TriggerSender,
				TriggerValue:   "alice@example.com",
				CustomPromptID: &promptID,
				Actions:        []models.RuleAction{{Type: models.ActionMove, DestFolder: "Urgent"}},
			},
		},
		prompts: map[int64]*models.CustomPrompt{
			1: {ID: 1, Name: "urgency", UserTemplate: "Subject: {{.Subject}}", OutputVar: "urgency"},
		},
	}
	exec := &mockExecutor{}
	engine := New(store, exec, mockClient)

	email := makeEmail("alice@example.com", "Hello", "INBOX", "msg-ai-err")
	// Even when AI fails, the rule still fires (best-effort prompt execution)
	fired, err := engine.EvaluateEmail(email, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fired != 1 {
		t.Errorf("expected fired=1 even on AI error, got %d", fired)
	}
	// No category should be saved
	if len(store.savedCategories) != 0 {
		t.Errorf("expected no saved categories on AI error, got %v", store.savedCategories)
	}
}
