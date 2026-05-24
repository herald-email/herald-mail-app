package calendar

import (
	"context"
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
