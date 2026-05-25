package calendar

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestEventServiceGetEventCacheFirstAvoidsSourceFetch(t *testing.T) {
	store := newCalendarTestCache(t)
	ref := calendarTestEventRef()
	cached := models.CalendarEvent{Ref: ref, Title: "cached event", Start: time.Now().UTC()}
	if err := store.PutCalendarEvent(cached); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	source := &fakeCalendarSource{event: models.CalendarEvent{Ref: ref, Title: "provider event"}}
	service := NewEventService(store, source)

	got, err := service.GetEvent(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Title != "cached event" {
		t.Fatalf("Title = %q, want cached event", got.Title)
	}
	if source.fetches != 0 {
		t.Fatalf("provider fetches = %d, want 0 for cache hit", source.fetches)
	}
}

func TestEventServiceGetEventNoCacheFetchesAndWritesThrough(t *testing.T) {
	store := newCalendarTestCache(t)
	ref := calendarTestEventRef()
	source := &fakeCalendarSource{event: models.CalendarEvent{Ref: ref, Title: "provider event"}}
	service := NewEventService(store, source)

	got, err := service.GetEventNoCache(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetEventNoCache: %v", err)
	}
	if got.Title != "provider event" || source.fetches != 1 {
		t.Fatalf("got %#v fetches=%d, want provider event and one fetch", got, source.fetches)
	}
	cached, err := store.GetCalendarEventByRef(ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef after NoCache: %v", err)
	}
	if cached.Title != "provider event" {
		t.Fatalf("cached Title = %q, want provider event", cached.Title)
	}
}

func TestEventServiceCoalescesDuplicateProviderFetches(t *testing.T) {
	store := newCalendarTestCache(t)
	ref := calendarTestEventRef()
	source := &fakeCalendarSource{
		event: models.CalendarEvent{Ref: ref, Title: "provider event"},
		block: make(chan struct{}),
	}
	service := NewEventService(store, source)

	var wg sync.WaitGroup
	results := make(chan string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := service.GetEvent(context.Background(), ref)
			if err != nil {
				results <- err.Error()
				return
			}
			results <- got.Title
		}()
	}
	source.waitForFetch(t)
	close(source.block)
	wg.Wait()
	close(results)

	for title := range results {
		if title != "provider event" {
			t.Fatalf("coalesced result = %q, want provider event", title)
		}
	}
	if source.fetches != 1 {
		t.Fatalf("provider fetches = %d, want 1 coalesced fetch", source.fetches)
	}
}

func TestEventServiceSyncCollectionUsesCachedSyncTokenAndStoresProviderToken(t *testing.T) {
	store := newCalendarTestCache(t)
	ref := models.CollectionRef{
		SourceID:     "work-calendar",
		AccountID:    "work",
		Kind:         models.SourceKindCalendar,
		CollectionID: "primary",
		DisplayName:  "Work",
	}
	if err := store.PutCalendarCollection(models.CalendarCollection{
		Ref:       ref,
		Color:     "#3367d6",
		SyncToken: "sync-primary-v1",
		ETag:      `"calendar-v1"`,
	}); err != nil {
		t.Fatalf("PutCalendarCollection: %v", err)
	}
	deleted := models.CalendarEvent{
		Ref:   models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "deleted-event"}.WithDefaults(),
		Title: "Deleted provider event",
		Start: time.Now().UTC(),
	}
	if err := store.PutCalendarEvent(deleted); err != nil {
		t.Fatalf("PutCalendarEvent deleted: %v", err)
	}
	updated := models.CalendarEvent{
		Ref:   models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "updated-event", ETag: `"updated-v2"`}.WithDefaults(),
		Title: "Updated provider event",
		Start: time.Now().UTC().Add(time.Hour),
	}
	source := &fakeCalendarSource{
		event: models.CalendarEvent{Ref: models.EventRef{SourceID: "work-calendar", AccountID: "work"}.WithDefaults()},
		syncResult: CalendarSyncResult{
			Events:        []models.CalendarEvent{updated},
			DeletedRefs:   []models.EventRef{deleted.Ref},
			NextSyncToken: "sync-primary-v2",
		},
	}
	service := NewEventService(store, source)

	events, err := service.SyncCollectionNoCache(context.Background(), ref)
	if err != nil {
		t.Fatalf("SyncCollectionNoCache: %v", err)
	}
	if len(source.syncTokens) != 1 || source.syncTokens[0] != "sync-primary-v1" {
		t.Fatalf("source sync tokens = %#v, want cached token", source.syncTokens)
	}
	if len(events) != 1 || events[0].Ref.EventID != "updated-event" {
		t.Fatalf("events = %#v, want updated provider event", events)
	}
	collection, err := store.GetCalendarCollection(ref)
	if err != nil {
		t.Fatalf("GetCalendarCollection: %v", err)
	}
	if collection.SyncToken != "sync-primary-v2" || collection.Color != "#3367d6" {
		t.Fatalf("collection = %#v, want refreshed sync token while preserving metadata", collection)
	}
	if _, err := store.GetCalendarEventByRef(deleted.Ref); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted event lookup err = %v, want sql.ErrNoRows after invalidation", err)
	}
	cached, err := store.GetCalendarEventByRef(updated.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef updated: %v", err)
	}
	if cached.Title != "Updated provider event" {
		t.Fatalf("cached updated event = %#v", cached)
	}
}

func newCalendarTestCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.New(filepath.Join(t.TempDir(), "calendar.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func calendarTestEventRef() models.EventRef {
	return models.EventRef{
		SourceID:   models.SourceID("work-calendar"),
		AccountID:  models.AccountID("work"),
		CalendarID: "primary",
		EventID:    "evt-1",
		ETag:       `"v1"`,
	}.WithDefaults()
}

type fakeCalendarSource struct {
	mu      sync.Mutex
	fetches int
	event   models.CalendarEvent
	block   chan struct{}
	seen    chan struct{}

	syncResult CalendarSyncResult
	syncTokens []string
}

func (s *fakeCalendarSource) ID() models.SourceID { return s.event.Ref.SourceID }

func (s *fakeCalendarSource) AccountID() models.AccountID { return s.event.Ref.AccountID }

func (s *fakeCalendarSource) DisplayName() string { return "fake" }

func (s *fakeCalendarSource) ListCalendars(context.Context) ([]models.CalendarCollection, error) {
	return nil, nil
}

func (s *fakeCalendarSource) ListEvents(context.Context, models.CollectionRef) ([]models.CalendarEvent, error) {
	return nil, nil
}

func (s *fakeCalendarSource) ListEventsWithSyncToken(_ context.Context, _ models.CollectionRef, syncToken string) (CalendarSyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncTokens = append(s.syncTokens, syncToken)
	return s.syncResult, nil
}

func (s *fakeCalendarSource) FetchEvent(context.Context, models.EventRef) (*models.CalendarEvent, error) {
	s.mu.Lock()
	s.fetches++
	if s.seen != nil {
		close(s.seen)
		s.seen = nil
	}
	block := s.block
	event := s.event
	s.mu.Unlock()
	if block != nil {
		<-block
	}
	return &event, nil
}

func (s *fakeCalendarSource) Close() error { return nil }

func (s *fakeCalendarSource) waitForFetch(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	if s.fetches > 0 {
		s.mu.Unlock()
		return
	}
	if s.seen == nil {
		s.seen = make(chan struct{})
	}
	seen := s.seen
	s.mu.Unlock()
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider fetch")
	}
}
