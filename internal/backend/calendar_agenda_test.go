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

func TestLocalBackendCalendarSearchReadsScopedCache(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work"},
		{ID: "personal-calendar", Kind: "calendar", Provider: "caldav", AccountID: "personal"},
	}}
	b := &LocalBackend{cache: store, cfg: cfg}
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	workEvent := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "work-calendar",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "planning",
		}.WithDefaults(),
		Title:          "Launch planning",
		Description:    "Bring notes for the cache-first calendar slice.",
		Start:          start,
		End:            start.Add(time.Hour),
		Organizer:      "Mina Park",
		OrganizerEmail: "mina@example.com",
		Attendees:      []models.CalendarAttendee{{Name: "Rae Stone", Email: "rae@example.com"}},
		Attachments:    []models.CalendarAttachment{{Title: "Agenda", URI: "https://calendar.example/private.pdf"}},
		Raw:            `{"syncToken":"secret"}`,
	}
	personalEvent := workEvent
	personalEvent.Ref = models.EventRef{SourceID: "personal-calendar", AccountID: "personal", CalendarID: "primary", EventID: "planning"}.WithDefaults()
	personalEvent.Title = "Personal planning"
	for _, event := range []models.CalendarEvent{workEvent, personalEvent} {
		if err := store.PutCalendarEvent(event); err != nil {
			t.Fatalf("PutCalendarEvent: %v", err)
		}
	}

	results, err := b.SearchCalendarEvents("rae@example.com", start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("SearchCalendarEvents: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v, want both configured calendar source matches", results)
	}

	results, err = b.SearchCalendarEvents("calendar.example", start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("SearchCalendarEvents provider internals: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("provider-internal search results = %#v, want none", results)
	}
}

func TestMultiBackendCalendarSearchAggregatesVisibleAccounts(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	work.calendarEvents = []models.CalendarEvent{{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "planning"}.WithDefaults(),
		Title:       "Launch planning",
		Description: "Scoped work calendar result.",
		Start:       start,
		End:         start.Add(time.Hour),
	}}
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")
	personal.calendarEvents = []models.CalendarEvent{{
		Ref:         models.EventRef{SourceID: "personal-calendar", AccountID: "personal", CalendarID: "primary", EventID: "planning"}.WithDefaults(),
		Title:       "Travel planning",
		Description: "Scoped personal calendar result.",
		Start:       start.Add(30 * time.Minute),
		End:         start.Add(90 * time.Minute),
	}}

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-calendar", AccountID: "work", DisplayName: "Work Calendar", Provider: "google_calendar"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-calendar", AccountID: "personal", DisplayName: "Personal Calendar", Provider: "caldav"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}

	results, err := mb.SearchCalendarEvents("planning", start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("SearchCalendarEvents all: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("all-account results = %#v, want work and personal", results)
	}
	if results[0].Ref.LocalID == results[1].Ref.LocalID {
		t.Fatalf("duplicate provider event IDs should stay scoped, got %#v", results)
	}
	if len(work.calendarSearch) != 1 || len(personal.calendarSearch) != 1 {
		t.Fatalf("search calls work=%v personal=%v, want both accounts searched", work.calendarSearch, personal.calendarSearch)
	}

	if err := mb.SwitchAccount("personal-calendar"); err != nil {
		t.Fatalf("SwitchAccount(personal): %v", err)
	}
	results, err = mb.SearchCalendarEvents("planning", start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("SearchCalendarEvents active: %v", err)
	}
	if len(results) != 1 || results[0].Ref.SourceID != "personal-calendar" {
		t.Fatalf("active-account results = %#v, want personal only", results)
	}
}
