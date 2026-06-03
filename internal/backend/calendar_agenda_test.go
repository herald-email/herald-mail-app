package backend

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/calendar"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testcalendar"
)

func TestDemoBackendCalendarAgendaIsSortedAndFetchesDetail(t *testing.T) {
	b := NewDemoBackend()
	if !b.CalendarAgendaAvailable() {
		t.Fatal("demo backend should advertise calendar agenda data")
	}

	start := time.Now().Add(-time.Hour)
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

func TestDemoBackendSaveCalendarEventUpdatesMemory(t *testing.T) {
	b := NewDemoBackend()
	start := time.Now().Add(-time.Hour)
	events, err := b.ListCalendarAgenda(start, start.AddDate(0, 0, 14))
	if err != nil {
		t.Fatalf("ListCalendarAgenda: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("demo backend returned no events")
	}
	edited := events[0]
	edited.Title = "Edited cache-backed event"
	edited.Location = "Timezone lab"
	edited.TimeZone = "America/Los_Angeles"

	saved, err := b.SaveCalendarEvent(edited)
	if err != nil {
		t.Fatalf("SaveCalendarEvent: %v", err)
	}
	if saved.Title != edited.Title || saved.Location != edited.Location {
		t.Fatalf("saved = %#v, want edited title/location", saved)
	}

	detail, err := b.GetCalendarEvent(edited.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEvent: %v", err)
	}
	if detail.Title != edited.Title || detail.Location != edited.Location {
		t.Fatalf("detail = %#v, want saved event", detail)
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
		{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work", Google: config.GoogleConfig{RefreshToken: "refresh-token"}},
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

func TestLocalBackendCalendarAgendaExpandsRecurringCachedEvents(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "icloud-calendar", Kind: "calendar", Provider: "caldav", AccountID: "icloud", CalDAV: config.CalDAVConfig{URL: "https://calendar.example/"}},
	}}
	b := &LocalBackend{cache: store, cfg: cfg}
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:rsm-alg-sofia",
		"SUMMARY:RSM Alg Sofia",
		"DTSTART;TZID=America/Los_Angeles:20250825T153500",
		"DTEND;TZID=America/Los_Angeles:20250825T180500",
		"EXDATE;TZID=America/Los_Angeles:20260518T153500",
		"RRULE:FREQ=WEEKLY;UNTIL=20260602T065959Z",
		"END:VEVENT",
		"BEGIN:VTIMEZONE",
		"TZID:America/Los_Angeles",
		"BEGIN:STANDARD",
		"DTSTART:20071104T020000",
		"END:STANDARD",
		"END:VTIMEZONE",
		"END:VCALENDAR",
	}, "\r\n") + "\r\n"
	event, err := calendar.EventFromICS("icloud-calendar", "icloud", "home", "rsm-alg-sofia.ics", `"etag"`, ics)
	if err != nil {
		t.Fatalf("EventFromICS: %v", err)
	}
	if err := store.PutCalendarEvent(*event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	events, err := b.listCachedCalendarAgenda(
		time.Date(2026, 5, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
	)
	if err != nil {
		t.Fatalf("listCachedCalendarAgenda: %v", err)
	}

	var got []string
	for _, event := range events {
		got = append(got, event.Start.In(loc).Format("2006-01-02 15:04"))
	}
	want := []string{"2026-05-04 15:35", "2026-05-11 15:35", "2026-05-25 15:35"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Fatalf("expanded occurrences = %#v, want %#v", got, want)
	}
}

func TestLocalBackendCalendarAgendaSyncsProviderWhenCacheEmpty(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("family", "Family", "#63da38"),
		testcalendar.WithEvent("family", testcalendar.Event{
			ID:       "lesson",
			UID:      "lesson",
			Summary:  "Family lesson",
			Location: "Music room",
			Start:    start,
			End:      start.Add(time.Hour),
			TimeZone: "UTC",
			ETag:     `"g-v1"`,
			Status:   "confirmed",
		}),
	)
	sourceCfg := lab.GoogleSourceConfig("family-calendar", "family")

	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{sourceCfg}}}
	events, err := b.ListCalendarAgenda(start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("ListCalendarAgenda: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Family lesson" {
		t.Fatalf("events = %#v, want synced provider event", events)
	}

	cached, err := store.GetCalendarEventByRef(events[0].Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if cached.Title != "Family lesson" {
		t.Fatalf("cached = %#v, want synced provider event in cache", cached)
	}
}

func TestLocalBackendCalendarAgendaRefreshesCachedRowsWhenSyncTTLAllows(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("family", "Family", "#63da38"),
		testcalendar.WithEvent("family", testcalendar.Event{
			ID:       "lesson",
			UID:      "lesson",
			Summary:  "Family lesson",
			Start:    start,
			End:      start.Add(time.Hour),
			TimeZone: "UTC",
			ETag:     `"g-v1"`,
			Status:   "confirmed",
		}),
	)
	sourceCfg := lab.GoogleSourceConfig("icloud-calendar", "icloud")

	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	staleCollection := models.CalendarCollection{
		Ref: models.CollectionRef{
			SourceID:     "icloud-calendar",
			AccountID:    "icloud",
			Kind:         models.SourceKindCalendar,
			CollectionID: "reminders",
			DisplayName:  "Reminders ⚠",
		},
		Color:     "#7e57c2",
		SyncToken: "stale-sync-token",
	}
	if err := store.PutCalendarCollection(staleCollection); err != nil {
		t.Fatalf("PutCalendarCollection(stale): %v", err)
	}
	staleEvent := models.CalendarEvent{
		Ref:    models.EventRef{SourceID: "icloud-calendar", AccountID: "icloud", CalendarID: "reminders", EventID: "task-artifact"}.WithDefaults(),
		Title:  "Reminder task artifact",
		Start:  start,
		End:    start.Add(time.Hour),
		Status: "confirmed",
	}
	if err := store.PutCalendarEvent(staleEvent); err != nil {
		t.Fatalf("PutCalendarEvent(stale): %v", err)
	}

	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{sourceCfg}}}
	events, err := b.ListCalendarAgenda(start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("ListCalendarAgenda: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Family lesson" || events[0].Ref.CalendarID != "family" {
		t.Fatalf("events = %#v, want provider-refreshed family event only", events)
	}
	if _, err := store.GetCalendarCollection(staleCollection.Ref); err == nil {
		t.Fatal("stale reminder collection remained cached after provider refresh")
	}
	if _, err := store.GetCalendarEventByRef(staleEvent.Ref); err == nil {
		t.Fatal("stale reminder event remained cached after provider refresh")
	}
}

func TestLocalBackendCalendarAgendaHiddenForGoogleCalendarWithoutOAuth(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "default-mail", Kind: "mail", Provider: "imap", AccountID: "default"},
		{
			ID:        "work-calendar",
			Kind:      "calendar",
			Provider:  "google_calendar",
			AccountID: "work",
			Google:    config.GoogleConfig{Email: "work@example.test"},
		},
	}}
	b := &LocalBackend{cache: store, cfg: cfg}

	if b.CalendarAgendaAvailable() {
		t.Fatal("Google Calendar without OAuth tokens should not advertise the Calendar tab")
	}
}

func TestNewMultiLocalKeepsStandaloneCalendarSourceAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Sources: []config.SourceConfig{
			{
				ID:          "default-mail",
				Kind:        string(models.SourceKindMail),
				Provider:    "imap",
				DisplayName: "Default Mail",
				AccountID:   "default",
				Credentials: config.CredentialsConfig{Username: "default@example.test", Password: "mail-pass"},
				IMAP:        config.ServerConfig{Host: "127.0.0.1", Port: 1143},
				SMTP:        config.ServerConfig{Host: "127.0.0.1", Port: 1025},
			},
			{
				ID:          "proton-mail",
				Kind:        string(models.SourceKindMail),
				Provider:    "protonmail",
				DisplayName: "Proton Mail",
				AccountID:   "proton",
				Credentials: config.CredentialsConfig{Username: "proton@example.test", Password: "mail-pass"},
				IMAP:        config.ServerConfig{Host: "127.0.0.1", Port: 1143},
				SMTP:        config.ServerConfig{Host: "127.0.0.1", Port: 1025},
			},
			{
				ID:          "icloud-calendar",
				Kind:        string(models.SourceKindCalendar),
				Provider:    "caldav",
				DisplayName: "iCloud Calendar",
				AccountID:   "icloud",
				CalDAV: config.CalDAVConfig{
					URL:      "https://caldav.icloud.com",
					Username: "icloud@example.test",
					Password: "app-specific-pass",
				},
			},
		},
	}
	cfg.Cache.DatabasePath = filepath.Join(tmpDir, "herald.db")

	b, err := NewMultiLocal(cfg, filepath.Join(tmpDir, "conf.yaml"), nil)
	if err != nil {
		t.Fatalf("NewMultiLocal: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	if !b.CalendarAgendaAvailable() {
		t.Fatal("standalone calendar source should make the Calendar tab available after startup")
	}
	if err := b.SwitchAccount("proton-mail"); err != nil {
		t.Fatalf("SwitchAccount(proton-mail): %v", err)
	}
	if !b.CalendarAgendaAvailable() {
		t.Fatal("standalone calendar source should remain available when a mail-only account is active")
	}
}

func TestLocalBackendSaveCalendarEventWritesScopedCache(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work"},
	}}
	b := &LocalBackend{cache: store, cfg: cfg}
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "planning"}.WithDefaults(),
		Title:       "Launch planning",
		Description: "Initial cached event.",
		Location:    "Room A",
		Start:       start,
		End:         start.Add(time.Hour),
		TimeZone:    "UTC",
		Status:      "confirmed",
	}
	if err := store.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}

	edited := event
	edited.Title = "Launch planning moved"
	edited.Location = "Room B"
	edited.TimeZone = "America/Los_Angeles"
	saved, err := b.SaveCalendarEvent(edited)
	if err != nil {
		t.Fatalf("SaveCalendarEvent: %v", err)
	}
	if saved.Ref.LocalID != event.Ref.LocalID {
		t.Fatalf("saved ref = %#v, want same scoped local id %q", saved.Ref, event.Ref.LocalID)
	}

	got, err := store.GetCalendarEventByRef(event.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if got.Title != edited.Title || got.Location != edited.Location || got.TimeZone != edited.TimeZone {
		t.Fatalf("cached event = %#v, want edited event", got)
	}
}

func TestLocalBackendSaveCalendarEventWritesProviderBeforeCache(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:          "planning",
			UID:         "planning",
			Summary:     "Provider planning",
			Description: "Original provider event.",
			Location:    "Room A",
			Start:       start,
			End:         start.Add(time.Hour),
			TimeZone:    "UTC",
			ETag:        `"g-v1"`,
			Status:      "confirmed",
		}),
	)
	sourceCfg := lab.GoogleSourceConfig("work-calendar", "work")
	provider, err := calendar.NewGoogleCalendarSource(sourceCfg)
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	sourceEvents, err := provider.ListEvents(context.Background(), models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.PutCalendarEvent(sourceEvents[0]); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{sourceCfg}}}

	edited := sourceEvents[0]
	edited.Title = "Provider planning moved"
	edited.Location = "Room B"
	saved, err := b.SaveCalendarEvent(edited)
	if err != nil {
		t.Fatalf("SaveCalendarEvent: %v", err)
	}
	if saved.Title != edited.Title || saved.Ref.ETag == edited.Ref.ETag {
		t.Fatalf("saved = %#v, want provider-updated event with fresh etag", saved)
	}

	providerFresh, err := provider.FetchEvent(context.Background(), saved.Ref)
	if err != nil {
		t.Fatalf("FetchEvent provider: %v", err)
	}
	if providerFresh.Title != edited.Title || providerFresh.Location != edited.Location {
		t.Fatalf("providerFresh = %#v, want edited provider event", providerFresh)
	}
	cached, err := store.GetCalendarEventByRef(saved.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if cached.Title != edited.Title || cached.Ref.ETag != saved.Ref.ETag {
		t.Fatalf("cached = %#v, want provider-saved event", cached)
	}
}

func TestLocalBackendCalendarCreateEventWritesProviderBeforeCache(t *testing.T) {
	start := time.Date(2026, 6, 2, 16, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t, testcalendar.WithCalendar("primary", "Work", "#3367d6"))
	sourceCfg := lab.GoogleSourceConfig("work-calendar", "work")
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{sourceCfg}}}
	event := models.CalendarEvent{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "backend-invite@example.test"}.WithDefaults(),
		ProviderUID: "backend-invite@example.test",
		Title:       "Backend invite",
		Start:       start,
		End:         start.Add(30 * time.Minute),
		TimeZone:    "UTC",
		Status:      "confirmed",
		Raw: strings.Join([]string{
			"BEGIN:VCALENDAR",
			"VERSION:2.0",
			"BEGIN:VEVENT",
			"UID:backend-invite@example.test",
			"SUMMARY:Backend invite",
			"DTSTART:20260602T160000Z",
			"DTEND:20260602T163000Z",
			"END:VEVENT",
			"END:VCALENDAR",
		}, "\r\n"),
	}

	saved, err := b.CreateCalendarEvent(event)
	if err != nil {
		t.Fatalf("CreateCalendarEvent: %v", err)
	}
	if saved.ProviderUID != event.ProviderUID || saved.Ref.ETag == "" {
		t.Fatalf("saved = %#v, want provider-created event with etag", saved)
	}
	cached, err := store.GetCalendarEventByRef(saved.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if cached.Title != event.Title || cached.ProviderUID != event.ProviderUID {
		t.Fatalf("cached = %#v, want created event", cached)
	}
	duplicate, err := b.FindCalendarEventByUID(models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"}, event.ProviderUID)
	if err != nil {
		t.Fatalf("FindCalendarEventByUID: %v", err)
	}
	if duplicate == nil || duplicate.Ref.LocalID != saved.Ref.LocalID {
		t.Fatalf("duplicate = %#v, want saved event", duplicate)
	}
}

func TestLocalBackendFindCalendarEventByUIDTrustsProviderOverStaleCache(t *testing.T) {
	start := time.Date(2026, 6, 2, 16, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t, testcalendar.WithCalendar("primary", "Work", "#3367d6"))
	sourceCfg := lab.GoogleSourceConfig("work-calendar", "work")
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	stale := models.CalendarEvent{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "deleted-provider-event"}.WithDefaults(),
		ProviderUID: "deleted-invite@example.test",
		Title:       "Deleted provider invite",
		Start:       start,
		End:         start.Add(30 * time.Minute),
		Status:      "confirmed",
	}
	if err := store.PutCalendarEvent(stale); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{sourceCfg}}}

	duplicate, err := b.FindCalendarEventByUID(models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"}, stale.ProviderUID)
	if err != nil {
		t.Fatalf("FindCalendarEventByUID: %v", err)
	}
	if duplicate != nil {
		t.Fatalf("duplicate = %#v, want provider miss to ignore stale cache row", duplicate)
	}
}

func TestLocalBackendSaveCalendarEventProviderFailureKeepsCachedEvent(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:       "planning",
			UID:      "planning",
			Summary:  "Provider planning",
			Location: "Room A",
			Start:    start,
			End:      start.Add(time.Hour),
			ETag:     `"g-v1"`,
		}),
	)
	sourceCfg := lab.GoogleSourceConfig("work-calendar", "work")
	event := models.CalendarEvent{
		Ref:      models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "planning", ETag: `"stale"`}.WithDefaults(),
		Title:    "Provider planning",
		Location: "Room A",
		Start:    start,
		End:      start.Add(time.Hour),
	}

	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{sourceCfg}}}

	edited := event
	edited.Title = "Should not reach cache"
	_, err = b.SaveCalendarEvent(edited)
	if err == nil {
		t.Fatal("SaveCalendarEvent succeeded with stale provider etag, want provider failure")
	}
	if !errors.Is(err, models.ErrCalendarMutationConflict) {
		t.Fatalf("error = %v, want ErrCalendarMutationConflict", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "etag") || strings.Contains(err.Error(), "/calendar/v3/") {
		t.Fatalf("error leaked provider internals: %v", err)
	}
	cached, err := store.GetCalendarEventByRef(event.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if cached.Title != event.Title || cached.Location != event.Location {
		t.Fatalf("cached = %#v, want original cached event after provider failure", cached)
	}
}

func TestLocalBackendRespondCalendarEventWritesProviderAndCache(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:      "rsvp",
			UID:     "rsvp",
			Summary: "Provider RSVP",
			Start:   start,
			End:     start.Add(time.Hour),
			ETag:    `"g-v1"`,
			Attendees: []testcalendar.Attendee{
				{Name: "Rae Stone", Email: "rae@example.com", ResponseStatus: "tentative"},
			},
		}),
	)
	sourceCfg := lab.GoogleSourceConfig("work-calendar", "work")
	sourceCfg.Google.Email = "rae@example.com"
	provider, err := calendar.NewGoogleCalendarSource(sourceCfg)
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	sourceEvents, err := provider.ListEvents(context.Background(), models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.PutCalendarEvent(sourceEvents[0]); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{sourceCfg}}}

	saved, err := b.RespondCalendarEvent(sourceEvents[0].Ref, "accepted")
	if err != nil {
		t.Fatalf("RespondCalendarEvent: %v", err)
	}
	if len(saved.Attendees) != 1 || saved.Attendees[0].RSVP != "accepted" {
		t.Fatalf("saved attendees = %#v, want accepted", saved.Attendees)
	}
	cached, err := store.GetCalendarEventByRef(saved.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if len(cached.Attendees) != 1 || cached.Attendees[0].RSVP != "accepted" {
		t.Fatalf("cached attendees = %#v, want accepted", cached.Attendees)
	}
}

func TestLocalBackendCalendarSearchReadsScopedCache(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work", Google: config.GoogleConfig{RefreshToken: "refresh-token"}},
		{ID: "personal-calendar", Kind: "calendar", Provider: "caldav", AccountID: "personal", CalDAV: config.CalDAVConfig{URL: "https://caldav.example/personal"}},
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

func TestMultiBackendSaveCalendarEventRoutesByScopedRef(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	work := newRecordingAccountBackend("work", []string{"INBOX"}, nil, "")
	work.calendarEvents = []models.CalendarEvent{{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "planning"}.WithDefaults(),
		Title:       "Work planning",
		Description: "Scoped work calendar result.",
		Start:       start,
		End:         start.Add(time.Hour),
	}}
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, nil, "")
	personal.calendarEvents = []models.CalendarEvent{{
		Ref:         models.EventRef{SourceID: "personal-calendar", AccountID: "personal", CalendarID: "primary", EventID: "planning"}.WithDefaults(),
		Title:       "Personal planning",
		Description: "Scoped personal calendar result.",
		Start:       start,
		End:         start.Add(time.Hour),
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

	edited := personal.calendarEvents[0]
	edited.Title = "Personal planning moved"
	saved, err := mb.SaveCalendarEvent(edited)
	if err != nil {
		t.Fatalf("SaveCalendarEvent: %v", err)
	}
	if saved.Title != edited.Title {
		t.Fatalf("saved = %#v, want edited personal event", saved)
	}
	if len(work.savedCalendarEvents) != 0 {
		t.Fatalf("work saved events = %#v, want no cross-account write", work.savedCalendarEvents)
	}
	if len(personal.savedCalendarEvents) != 1 || personal.savedCalendarEvents[0].Ref.SourceID != "personal-calendar" {
		t.Fatalf("personal saved events = %#v, want one scoped save", personal.savedCalendarEvents)
	}
}

func TestLocalBackendCrossSourceSearchReadsMailAndCalendarCache(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "default-mail", Kind: "mail", Provider: "imap", AccountID: "default"},
		{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work", Google: config.GoogleConfig{RefreshToken: "refresh-token"}},
	}}
	b := &LocalBackend{cache: store, cfg: cfg}
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	if err := store.CacheEmail(&models.EmailData{
		SourceID:  "default-mail",
		AccountID: "default",
		MessageID: "mail-planning",
		UID:       41,
		Folder:    "INBOX",
		Sender:    "mina@example.com",
		Subject:   "Launch planning memo",
		Date:      start.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "work-calendar",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "planning",
		}.WithDefaults(),
		Title:       "Launch planning",
		Description: "Cached calendar event.",
		Start:       start,
		End:         start.Add(time.Hour),
		Attachments: []models.CalendarAttachment{{
			Title: "Agenda",
			URI:   "https://calendar.example/private.pdf",
		}},
		Raw: `{"syncToken":"secret"}`,
	}
	if err := store.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}

	results, err := b.CrossSourceSearch("planning")
	if err != nil {
		t.Fatalf("CrossSourceSearch: %v", err)
	}
	if got := crossSourceKinds(results); got != "event,mail" && got != "mail,event" {
		t.Fatalf("result kinds = %q, want mail and event: %#v", got, results)
	}
	for _, result := range results {
		switch result.Kind {
		case models.CrossSourceResultMail:
			if result.Email == nil || result.Email.MessageID != "mail-planning" {
				t.Fatalf("mail result = %#v, want cached planning mail", result)
			}
		case models.CrossSourceResultEvent:
			if result.Event == nil || result.Event.Ref.SourceID != "work-calendar" {
				t.Fatalf("event result = %#v, want scoped cached event", result)
			}
		default:
			t.Fatalf("unexpected result kind %q", result.Kind)
		}
	}

	providerResults, err := b.CrossSourceSearch("calendar.example")
	if err != nil {
		t.Fatalf("CrossSourceSearch provider internals: %v", err)
	}
	if len(providerResults) != 0 {
		t.Fatalf("provider-internal results = %#v, want none", providerResults)
	}
}

func TestMultiBackendCrossSourceSearchAggregatesVisibleAccounts(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	work := newRecordingAccountBackend("work", []string{"INBOX"}, &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "shared-id",
		UID:       11,
		Folder:    "INBOX",
		Sender:    "work@example.test",
		Subject:   "planning",
		Date:      start.Add(-time.Hour),
	}, "")
	work.calendarEvents = []models.CalendarEvent{{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "planning"}.WithDefaults(),
		Title:       "Planning event",
		Description: "Scoped work calendar result.",
		Start:       start,
		End:         start.Add(time.Hour),
	}}
	personal := newRecordingAccountBackend("personal", []string{"INBOX"}, &models.EmailData{
		SourceID:  "personal-mail",
		AccountID: "personal",
		MessageID: "shared-id",
		UID:       22,
		Folder:    "INBOX",
		Sender:    "personal@example.test",
		Subject:   "planning",
		Date:      start.Add(-2 * time.Hour),
	}, "")
	personal.calendarEvents = []models.CalendarEvent{{
		Ref:         models.EventRef{SourceID: "personal-calendar", AccountID: "personal", CalendarID: "primary", EventID: "planning"}.WithDefaults(),
		Title:       "Personal planning",
		Description: "Scoped personal calendar result.",
		Start:       start.Add(30 * time.Minute),
		End:         start.Add(90 * time.Minute),
	}}

	mb, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"}, Backend: work},
		{Info: AccountInfo{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"}, Backend: personal},
	})
	if err != nil {
		t.Fatalf("NewMultiBackend: %v", err)
	}
	if err := mb.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatalf("SwitchAccount(all): %v", err)
	}

	results, err := mb.CrossSourceSearch("planning")
	if err != nil {
		t.Fatalf("CrossSourceSearch all: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("all-account results = %#v, want two mail and two event results", results)
	}
	seenMailSources := map[models.SourceID]bool{}
	seenEventSources := map[models.SourceID]bool{}
	for _, result := range results {
		if result.Email != nil {
			seenMailSources[result.Email.SourceID] = true
		}
		if result.Event != nil {
			seenEventSources[result.Event.Ref.SourceID] = true
		}
	}
	if !seenMailSources["work-mail"] || !seenMailSources["personal-mail"] {
		t.Fatalf("mail sources = %#v, want work and personal", seenMailSources)
	}
	if !seenEventSources["work-calendar"] || !seenEventSources["personal-calendar"] {
		t.Fatalf("event sources = %#v, want work and personal calendars", seenEventSources)
	}

	if err := mb.SwitchAccount("personal-mail"); err != nil {
		t.Fatalf("SwitchAccount(personal): %v", err)
	}
	results, err = mb.CrossSourceSearch("planning")
	if err != nil {
		t.Fatalf("CrossSourceSearch active: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("active-account results = %#v, want personal mail and event", results)
	}
	for _, result := range results {
		if result.Email != nil && result.Email.SourceID != "personal-mail" {
			t.Fatalf("active mail result = %#v, want personal-mail", result)
		}
		if result.Event != nil && result.Event.Ref.AccountID != "personal" {
			t.Fatalf("active event result = %#v, want personal account", result)
		}
	}
}

func crossSourceKinds(results []models.CrossSourceSearchResult) string {
	kinds := make([]string, 0, len(results))
	for _, result := range results {
		kinds = append(kinds, string(result.Kind))
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ",")
}

func (b *recordingAccountBackend) CrossSourceSearch(query string) ([]models.CrossSourceSearchResult, error) {
	emails, err := b.SearchEmails("INBOX", query, false)
	if err != nil {
		return nil, err
	}
	events, err := b.SearchCalendarEvents(query, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	results := make([]models.CrossSourceSearchResult, 0, len(emails)+len(events))
	for _, email := range emails {
		if email == nil {
			continue
		}
		results = append(results, models.CrossSourceSearchResult{
			Kind:      models.CrossSourceResultMail,
			Email:     email,
			When:      email.Date,
			MatchHint: models.EmailSearchMatchHint(email, query),
		})
	}
	for _, event := range events {
		eventCopy := event
		results = append(results, models.CrossSourceSearchResult{
			Kind:      models.CrossSourceResultEvent,
			Event:     &eventCopy,
			When:      event.Start,
			MatchHint: models.CalendarEventSearchMatchHint(event, query),
		})
	}
	return results, nil
}
