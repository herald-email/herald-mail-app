package rules

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestAutomationLaneMailEventsCarrySourceScopeAndEvaluateRules(t *testing.T) {
	store := &mockStore{
		rules: []*models.Rule{
			makeRule(7, models.TriggerSender, "alice@example.com",
				models.RuleAction{Type: models.ActionMove, DestFolder: "Archive"}),
		},
	}
	exec := &mockExecutor{}
	lane := NewAutomationLane(New(store, exec, nil))
	defer lane.Close()

	email := makeEmail("alice@example.com", "Hello", "INBOX", "msg-1")
	email.SourceID = "mail-work"
	email.AccountID = "work"
	result := awaitAutomationResult(t, lane.Submit(context.Background(), models.NewMailAutomationEvent(email, "")))

	if result.Kind != models.AutomationEventMailMessageReceived {
		t.Fatalf("Kind = %q, want mail event", result.Kind)
	}
	if result.SourceID != "mail-work" || result.AccountID != "work" {
		t.Fatalf("scope = %q/%q, want mail-work/work", result.SourceID, result.AccountID)
	}
	if result.MessageID != "msg-1" || result.ItemID == "" {
		t.Fatalf("result ids = message %q item %q, want msg-1 and local item id", result.MessageID, result.ItemID)
	}
	if result.FiredCount != 1 {
		t.Fatalf("FiredCount = %d, want 1", result.FiredCount)
	}
	if len(exec.moved) != 1 || exec.moved[0] != "msg-1" {
		t.Fatalf("moved = %v, want msg-1", exec.moved)
	}
}

func TestAutomationLaneAcceptsCalendarEventChangeAsReadOnlyNoop(t *testing.T) {
	store := &mockStore{}
	exec := &mockExecutor{}
	lane := NewAutomationLane(New(store, exec, nil))
	defer lane.Close()

	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "calendar-work",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "event-1",
		},
		Title: "Planning",
	}
	result := awaitAutomationResult(t, lane.Submit(context.Background(), models.NewCalendarAutomationEvent(event)))

	if result.Kind != models.AutomationEventCalendarEventChanged {
		t.Fatalf("Kind = %q, want calendar event", result.Kind)
	}
	if result.SourceID != "calendar-work" || result.AccountID != "work" {
		t.Fatalf("scope = %q/%q, want calendar-work/work", result.SourceID, result.AccountID)
	}
	if result.EventID != "event-1" || result.ItemID == "" {
		t.Fatalf("result ids = event %q item %q, want event-1 and local item id", result.EventID, result.ItemID)
	}
	if result.FiredCount != 0 {
		t.Fatalf("FiredCount = %d, want 0", result.FiredCount)
	}
	if len(exec.moved) != 0 || len(store.log) != 0 {
		t.Fatalf("calendar no-op executed mail actions: moved=%v log=%v", exec.moved, store.log)
	}
}

func TestAutomationLaneSerializesMailEventsPerSource(t *testing.T) {
	store := &mockStore{
		rules: []*models.Rule{
			makeRule(7, models.TriggerDomain, "example.com",
				models.RuleAction{Type: models.ActionMove, DestFolder: "Archive"}),
		},
	}
	exec := newBlockingExecutor()
	lane := NewAutomationLane(New(store, exec, nil))
	defer lane.Close()

	first := makeEmail("first@example.com", "First", "INBOX", "msg-1")
	first.SourceID = "mail-work"
	second := makeEmail("second@example.com", "Second", "INBOX", "msg-2")
	second.SourceID = "mail-work"

	firstResult := lane.Submit(context.Background(), models.NewMailAutomationEvent(first, ""))
	waitForStarted(t, exec, "msg-1")
	secondResult := lane.Submit(context.Background(), models.NewMailAutomationEvent(second, ""))

	select {
	case id := <-exec.started:
		t.Fatalf("second same-source event started before first completed: %s", id)
	case <-time.After(30 * time.Millisecond):
	}

	exec.releaseOne()
	result := awaitAutomationResult(t, firstResult)
	if result.MessageID != "msg-1" {
		t.Fatalf("first result message = %q, want msg-1", result.MessageID)
	}
	waitForStarted(t, exec, "msg-2")
	exec.releaseOne()
	result = awaitAutomationResult(t, secondResult)
	if result.MessageID != "msg-2" {
		t.Fatalf("second result message = %q, want msg-2", result.MessageID)
	}
}

type blockingExecutor struct {
	started chan string
	release chan struct{}

	mu    sync.Mutex
	moved []string
}

func newBlockingExecutor() *blockingExecutor {
	return &blockingExecutor{
		started: make(chan string, 10),
		release: make(chan struct{}, 10),
	}
}

func (e *blockingExecutor) MoveEmail(messageID, from, to string) error {
	e.started <- messageID
	<-e.release
	e.mu.Lock()
	defer e.mu.Unlock()
	e.moved = append(e.moved, messageID)
	return nil
}

func (e *blockingExecutor) ArchiveEmail(messageID, folder string) error {
	return e.MoveEmail(messageID, folder, "Archive")
}

func (e *blockingExecutor) DeleteEmail(messageID, folder string) error {
	return e.MoveEmail(messageID, folder, "Trash")
}

func (e *blockingExecutor) releaseOne() {
	e.release <- struct{}{}
}

func waitForStarted(t *testing.T, exec *blockingExecutor, want string) {
	t.Helper()
	select {
	case got := <-exec.started:
		if got != want {
			t.Fatalf("started = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s to start", want)
	}
}

func awaitAutomationResult(t *testing.T, result AutomationResult) models.RuleResult {
	t.Helper()
	got, err := result.Await(context.Background())
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	return got
}
