package backend

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/memory"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type memorySchedulerStubAI struct{}

func (s *memorySchedulerStubAI) Chat(_ []ai.ChatMessage) (string, error) { return "", nil }
func (s *memorySchedulerStubAI) ChatWithTools(_ []ai.ChatMessage, _ []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, ai.ErrToolsNotSupported
}
func (s *memorySchedulerStubAI) Classify(_, _ string) (ai.Category, error) { return "", nil }
func (s *memorySchedulerStubAI) Embed(_ string) ([]float32, error)         { return nil, nil }
func (s *memorySchedulerStubAI) SetEmbeddingModel(_ string)                {}
func (s *memorySchedulerStubAI) GenerateQuickReplies(_, _, _ string) ([]string, error) {
	return nil, nil
}
func (s *memorySchedulerStubAI) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (s *memorySchedulerStubAI) HasVisionModel() bool { return false }
func (s *memorySchedulerStubAI) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (s *memorySchedulerStubAI) Ping() error { return nil }

type countingMemoryEmailSource struct {
	calls int32
}

func (s *countingMemoryEmailSource) MemoryEmails(_ string, _ int) ([]memory.EmailSnapshot, error) {
	atomic.AddInt32(&s.calls, 1)
	return nil, nil
}

func TestLocalMemoryEmailSourceReadsCachedHeadersAndBodies(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	email := &models.EmailData{
		SourceID:         "work-mail",
		AccountID:        "work",
		LocalID:          "mail:work:memory-msg-1",
		MessageID:        "memory-msg-1",
		Sender:           "Sergey <sergey@example.com>",
		Subject:          "Interview follow-up",
		ProviderThreadID: "thread-123",
		Folder:           "INBOX",
		Date:             time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	}
	if err := store.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	body := strings.Repeat("Can you send availability by Friday? ", 180)
	if err := store.CacheBodyTextByRef(email.MessageRef(), body); err != nil {
		t.Fatalf("CacheBodyText: %v", err)
	}
	if err := store.SetClassificationByRef(email.MessageRef(), "important"); err != nil {
		t.Fatalf("SetClassificationByRef: %v", err)
	}
	if err := store.UpsertContacts([]models.ContactAddr{{Email: "sergey@example.com", Name: "Sergey Petrov"}}, "from"); err != nil {
		t.Fatalf("UpsertContacts: %v", err)
	}
	if err := store.UpdateContactEnrichment("sergey@example.com", "Cobalt Systems", []string{"interview", "platform"}); err != nil {
		t.Fatalf("UpdateContactEnrichment: %v", err)
	}
	if err := store.StoreEmbeddingChunksByRef(email.MessageRef(), []models.EmbeddingChunk{{
		MessageID:   email.MessageID,
		ChunkIndex:  0,
		Embedding:   []float32{1, 0},
		ContentHash: "memory-hash",
	}}); err != nil {
		t.Fatalf("StoreEmbeddingChunksByRef: %v", err)
	}

	source := localMemoryEmailSource{cache: store}
	got, err := source.MemoryEmails("INBOX", 10)
	if err != nil {
		t.Fatalf("MemoryEmails: %v", err)
	}
	if len(got) != 1 || got[0].Email.MessageID != email.MessageID || got[0].BodyText == "" {
		t.Fatalf("memory source rows = %#v", got)
	}
	row := got[0]
	if row.Direction != "inbound" || row.Classification != "important" || !row.HasBodyCache || !row.HasEmbedding {
		t.Fatalf("memory source metadata = %#v", row)
	}
	if row.ContactDisplayName != "Sergey Petrov" || row.ContactCompany != "Cobalt Systems" || len(row.ContactTopics) != 2 {
		t.Fatalf("memory source contact metadata = %#v", row)
	}
	if len([]rune(row.BodyText)) > 4003 {
		t.Fatalf("memory source body was not bounded: %d runes", len([]rune(row.BodyText)))
	}
}

func TestLocalMemoryCalendarSourceReadsCachedScopedEvents(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	start := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   models.DefaultCalendarSourceID,
			AccountID:  models.DefaultAccountID,
			CalendarID: "primary",
			EventID:    "evt-memory-calendar",
		},
		Title:          "Sergey interview loop",
		Description:    "Discuss next steps.",
		Start:          start,
		End:            start.Add(time.Hour),
		OrganizerEmail: "sergey@example.com",
	}
	if err := store.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	source := localMemoryEmailSource{cache: store}

	got, err := source.MemoryCalendarEvents(start.Add(-time.Hour), start.Add(2*time.Hour), 10)
	if err != nil {
		t.Fatalf("MemoryCalendarEvents: %v", err)
	}
	if len(got) != 1 || got[0].Ref.EventID != event.Ref.EventID || got[0].Title != event.Title {
		t.Fatalf("calendar memory events = %#v", got)
	}
}

func TestDemoBackendMemoryReplyPrepReturnsSourceBackedNudges(t *testing.T) {
	demoBackend := NewDemoBackend()

	results, err := demoBackend.SearchMemories(context.Background(), memory.Query{
		People:        []string{"sergey@example.com"},
		MinConfidence: 0.35,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected demo memory results for Sergey")
	}

	prep, err := demoBackend.BuildReplyMemoryContext(context.Background(), memory.ReplyPrepQuery{
		Recipient: "sergey@example.com",
		Subject:   "Senior engineer interview",
	})
	if err != nil {
		t.Fatalf("BuildReplyMemoryContext: %v", err)
	}
	if len(prep.Nudges) == 0 {
		t.Fatalf("expected demo Compose Radar nudges, got %#v", prep)
	}
	if prep.Nudges[0].Evidence[0].MessageID == "" {
		t.Fatalf("nudge missing source evidence: %#v", prep.Nudges[0])
	}
}

func TestLocalMemoryRefreshUsesBackgroundAndInteractiveAISchedulerPriorities(t *testing.T) {
	store, err := memory.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	settings := memory.DefaultSettings()
	settings.Directory = store.Root()
	settings.Sources.Folders = []string{"INBOX"}
	source := &countingMemoryEmailSource{}
	classifier := ai.NewManagedClient(&memorySchedulerStubAI{}, ai.ManagedConfig{
		MaxConcurrency:                  1,
		QueueLimit:                      8,
		PauseBackgroundWhileInteractive: true,
	})
	backend := &LocalBackend{
		classifier:    classifier,
		memoryService: memory.NewServiceWithStore(settings, store, source),
	}

	holdStarted := make(chan struct{})
	releaseHold := make(chan struct{})
	holdDone := make(chan struct{})
	go func() {
		_ = ai.Schedule(classifier, ai.PriorityBackground, ai.TaskKindEmbedding, "hold", func() error {
			close(holdStarted)
			<-releaseHold
			return nil
		})
		close(holdDone)
	}()
	<-holdStarted

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		_, err := backend.SearchMemories(ctx, memory.Query{Limit: 1})
		errCh <- err
	}()
	waitForMemorySchedulerStatus(t, classifier, func(status ai.SchedulerStatus) bool {
		return status.QueuedBackgroundKind == ai.TaskKindMemoryExtraction
	})

	go func() {
		_, err := backend.BuildReplyMemoryContext(ctx, memory.ReplyPrepQuery{
			Recipient: "sergey@example.com",
			Subject:   "Interview follow-up",
		})
		errCh <- err
	}()
	waitForMemorySchedulerStatus(t, classifier, func(status ai.SchedulerStatus) bool {
		return status.QueuedInteractiveKind == ai.TaskKindMemoryExtraction &&
			status.QueuedBackgroundKind == ai.TaskKindMemoryExtraction
	})

	close(releaseHold)
	<-holdDone
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("memory refresh call returned error: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for scheduled memory refresh calls: %v", ctx.Err())
		}
	}
	if got := atomic.LoadInt32(&source.calls); got != 1 {
		t.Fatalf("memory source calls = %d, want one winning refresh", got)
	}
}

func TestLocalMemoryCadenceManualDoesNotRefreshAfterSync(t *testing.T) {
	backend, source := newTestLocalMemoryCadenceBackend(t, "manual")

	backend.refreshMemoriesAfterSuccessfulSync()
	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&source.calls); got != 0 {
		t.Fatalf("memory source calls = %d, want no proactive manual refresh", got)
	}
}

func TestLocalMemoryCadenceAfterSyncTriggersBackgroundRefresh(t *testing.T) {
	backend, source := newTestLocalMemoryCadenceBackend(t, "after_sync")

	backend.refreshMemoriesAfterSuccessfulSync()

	waitForMemorySourceCalls(t, source, 1)
}

func TestLocalMemoryCadenceBackgroundIdleTriggersBackgroundRefresh(t *testing.T) {
	backend, source := newTestLocalMemoryCadenceBackend(t, "background_idle")

	backend.refreshMemoriesAfterSuccessfulSync()

	waitForMemorySourceCalls(t, source, 1)
}

func TestLocalMemoryCadenceComposeOpenForcesReplyRefresh(t *testing.T) {
	backend, source := newTestLocalMemoryCadenceBackend(t, "compose_open")

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := backend.BuildReplyMemoryContext(ctx, memory.ReplyPrepQuery{Recipient: "sergey@example.com", Limit: 2}); err != nil {
			t.Fatalf("BuildReplyMemoryContext #%d: %v", i+1, err)
		}
	}

	if got := atomic.LoadInt32(&source.calls); got != 2 {
		t.Fatalf("memory source calls = %d, want compose_open to force each reply-prep refresh", got)
	}
}

func newTestLocalMemoryCadenceBackend(t *testing.T, cadence string) (*LocalBackend, *countingMemoryEmailSource) {
	t.Helper()
	store, err := memory.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	settings := memory.DefaultSettings()
	settings.Directory = store.Root()
	settings.Sources.Folders = []string{"INBOX"}
	settings.UpdateRules.Cadence = cadence
	source := &countingMemoryEmailSource{}
	return &LocalBackend{
		cfg:           &config.Config{Memories: settings},
		memoryService: memory.NewServiceWithStore(settings, store, source),
	}, source
}

func waitForMemorySourceCalls(t *testing.T, source *countingMemoryEmailSource, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := atomic.LoadInt32(&source.calls); got >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for memory source calls >= %d, got %d", want, atomic.LoadInt32(&source.calls))
}

func waitForMemorySchedulerStatus(t *testing.T, reporter ai.StatusReporter, match func(ai.SchedulerStatus) bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := reporter.AIStatus()
		if match(status) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for AI scheduler status")
}
