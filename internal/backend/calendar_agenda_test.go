package backend

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestDemoBackendCalendarAgendaIsSortedAndFetchesDetail(t *testing.T) {
	b := NewDemoBackend()
	if !b.CalendarAgendaAvailable() {
		t.Fatal("demo backend should advertise calendar agenda data")
	}

	start := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)
	events, err := b.ListCalendarAgenda(start, start.AddDate(0, 0, 14))
	if err != nil {
		t.Fatalf("ListCalendarAgenda: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("events=%d, want at least 3 deterministic demo events", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].Start.Before(events[i-1].Start) {
			t.Fatalf("events not sorted by start: %s before %s", events[i].Start, events[i-1].Start)
		}
	}
	first := events[0]
	if first.Ref.SourceID == "" || first.Ref.AccountID == "" || first.Ref.CalendarID == "" || first.Ref.LocalID == "" {
		t.Fatalf("event ref not fully scoped: %#v", first.Ref)
	}

	detail, err := b.GetCalendarEvent(first.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEvent: %v", err)
	}
	if detail.Title != first.Title || detail.Ref.LocalID != first.Ref.LocalID {
		t.Fatalf("detail = %#v, want same event as %#v", detail, first)
	}
	if detail.Description == "" {
		t.Fatal("demo event detail should include a readable description")
	}
}

func TestLocalBackendCalendarAgendaReadsConfiguredSourceCache(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "default-mail", Kind: "mail", Provider: "imap", AccountID: "default"},
		{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work"},
	}}
	b := &LocalBackend{cache: store, cfg: cfg}
	if !b.CalendarAgendaAvailable() {
		t.Fatal("configured calendar source should make agenda available")
	}

	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "work-calendar",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "standup",
		}.WithDefaults(),
		Title:       "Cache-backed standup",
		Description: "Read from SQLite without a provider call.",
		Start:       start,
		End:         start.Add(30 * time.Minute),
		Status:      "confirmed",
	}
	if err := store.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}

	events, err := b.ListCalendarAgenda(start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListCalendarAgenda: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Cache-backed standup" {
		t.Fatalf("events = %#v, want cache-backed standup", events)
	}

	detail, err := b.GetCalendarEvent(event.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEvent: %v", err)
	}
	if detail.Title != event.Title {
		t.Fatalf("detail title = %q, want %q", detail.Title, event.Title)
	}
}
