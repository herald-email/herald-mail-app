package rules

import (
	"errors"
	"testing"
	"time"

	"mail-processor/internal/models"
)

// --- mock Store ---

type mockStore struct {
	rules   []*models.Rule
	prompts map[int64]*models.CustomPrompt
	log     []*models.RuleActionLogEntry
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

func (m *mockStore) AppendActionLog(e *models.RuleActionLogEntry) error {
	m.log = append(m.log, e)
	return nil
}

func (m *mockStore) TouchRuleLastTriggered(int64) error { return nil }

// --- mock Executor ---

type mockExecutor struct {
	moved    []string
	archived []string
	deleted  []string
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
