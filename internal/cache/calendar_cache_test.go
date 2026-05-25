package cache

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestCalendarCacheTablesAreSourceScoped(t *testing.T) {
	c := newTestCache(t)

	calCols := tableColumns(t, c.db, "calendar_collections")
	for _, name := range []string{"source_id", "account_id", "calendar_id", "local_id", "display_name", "sync_token", "etag"} {
		if !calCols[name] {
			t.Fatalf("calendar_collections missing column %s", name)
		}
	}

	eventCols := tableColumns(t, c.db, "calendar_events")
	for _, name := range []string{"source_id", "account_id", "calendar_id", "event_id", "instance_id", "local_id", "etag", "revision", "starts_at", "ends_at", "timezone", "organizer", "attendees_json", "recurrence_json", "attachments_json", "alternate_timezones_json", "invalidated_at"} {
		if !eventCols[name] {
			t.Fatalf("calendar_events missing column %s", name)
		}
	}
}

func TestCacheCalendarCollectionRoundTrip(t *testing.T) {
	c := newTestCache(t)

	collection := models.CalendarCollection{
		Ref: models.CollectionRef{
			SourceID:     models.SourceID("work-calendar"),
			AccountID:    models.AccountID("work"),
			Kind:         models.SourceKindCalendar,
			CollectionID: "primary",
			DisplayName:  "Work",
		},
		Color:     "#3367d6",
		SyncToken: "sync-1",
		ETag:      `"cal-v1"`,
	}
	if err := c.PutCalendarCollection(collection); err != nil {
		t.Fatalf("PutCalendarCollection: %v", err)
	}

	got, err := c.GetCalendarCollection(collection.Ref)
	if err != nil {
		t.Fatalf("GetCalendarCollection: %v", err)
	}
	if got.Ref.SourceID != collection.Ref.SourceID || got.Ref.AccountID != collection.Ref.AccountID || got.Ref.CollectionID != "primary" {
		t.Fatalf("collection scope = %#v, want %#v", got.Ref, collection.Ref)
	}
	if got.Color != "#3367d6" || got.SyncToken != "sync-1" || got.ETag != `"cal-v1"` {
		t.Fatalf("collection metadata = %#v, want color/sync/etag", got)
	}
}

func TestCacheCalendarEventRoundTripAndInvalidate(t *testing.T) {
	c := newTestCache(t)
	start := time.Date(2026, 5, 24, 16, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   models.SourceID("work-calendar"),
			AccountID:  models.AccountID("work"),
			CalendarID: "primary",
			EventID:    "event-1",
			ETag:       `"event-v1"`,
		}.WithDefaults(),
		Title:       "Phase 6 review",
		Description: "Calendar cache foundation",
		Location:    "Herald",
		Start:       start,
		End:         start.Add(time.Hour),
		Status:      "confirmed",
		Revision:    "rev-1",
		UpdatedAt:   start.Add(-time.Hour),
		Raw:         `{"id":"event-1"}`,
	}

	if err := c.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	got, err := c.GetCalendarEventByRef(event.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if got.Ref.LocalID != event.Ref.LocalID || got.Title != event.Title || got.Ref.ETag != `"event-v1"` || got.Revision != "rev-1" {
		t.Fatalf("event roundtrip = %#v, want %#v", got, event)
	}

	if err := c.InvalidateCalendarEvent(event.Ref); err != nil {
		t.Fatalf("InvalidateCalendarEvent: %v", err)
	}
	if _, err := c.GetCalendarEventByRef(event.Ref); err == nil {
		t.Fatal("GetCalendarEventByRef succeeded after invalidation, want miss")
	}
}

func TestCacheCalendarEventRichDetailRoundTrip(t *testing.T) {
	c := newTestCache(t)
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   models.SourceID("work-calendar"),
			AccountID:  models.AccountID("work"),
			CalendarID: "primary",
			EventID:    "event-rich",
		}.WithDefaults(),
		Title:              "Timezone planning",
		Start:              start,
		End:                start.Add(time.Hour),
		TimeZone:           "America/Los_Angeles",
		Organizer:          "Mina Park",
		OrganizerEmail:     "mina@example.com",
		Recurrence:         []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
		RecurrenceSummary:  "Weekly on Monday",
		AlternateTimeZones: []string{"Asia/Tokyo"},
		Attendees: []models.CalendarAttendee{
			{Name: "Rae Stone", Email: "rae@example.com", RSVP: "accepted"},
			{Name: "Noor Patel", Email: "noor@example.com", RSVP: "tentative", Optional: true},
		},
		Attachments: []models.CalendarAttachment{
			{Title: "Agenda", URI: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
		},
	}

	if err := c.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	got, err := c.GetCalendarEventByRef(event.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if got.TimeZone != "America/Los_Angeles" || got.Organizer != "Mina Park" || got.OrganizerEmail != "mina@example.com" {
		t.Fatalf("rich identity fields = %#v", got)
	}
	if got.RecurrenceSummary != "Weekly on Monday" || len(got.Recurrence) != 1 || got.Recurrence[0] != event.Recurrence[0] {
		t.Fatalf("recurrence = %#v / %q", got.Recurrence, got.RecurrenceSummary)
	}
	if len(got.Attendees) != 2 || got.Attendees[0].RSVP != "accepted" || !got.Attendees[1].Optional {
		t.Fatalf("attendees = %#v", got.Attendees)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Title != "Agenda" || got.Attachments[0].MIMEType != "application/pdf" {
		t.Fatalf("attachments = %#v", got.Attachments)
	}
	if len(got.AlternateTimeZones) != 1 || got.AlternateTimeZones[0] != "Asia/Tokyo" {
		t.Fatalf("alternate zones = %#v", got.AlternateTimeZones)
	}
}

func TestCacheSearchCalendarEventsMatchesScopedMetadata(t *testing.T) {
	c := newTestCache(t)
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   models.SourceID("work-calendar"),
			AccountID:  models.AccountID("work"),
			CalendarID: "primary",
			EventID:    "event-search",
			ETag:       `"provider-etag"`,
		}.WithDefaults(),
		ProviderUID:       "provider-secret",
		Title:             "Timezone planning",
		Description:       "Discuss follow-up notes for launch readiness.",
		Location:          "Video call",
		Start:             start,
		End:               start.Add(time.Hour),
		Organizer:         "Mina Park",
		OrganizerEmail:    "mina@example.com",
		RecurrenceSummary: "Weekly on Monday",
		Attendees: []models.CalendarAttendee{
			{Name: "Rae Stone", Email: "rae@example.com", RSVP: "accepted"},
		},
		Attachments: []models.CalendarAttachment{
			{Title: "Agenda", URI: "https://calendar.example/private/agenda.pdf", MIMEType: "application/pdf"},
		},
		Raw: `{"syncToken":"secret-sync-token"}`,
	}
	if err := c.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	otherScope := event
	otherScope.Ref = models.EventRef{SourceID: "personal-calendar", AccountID: "personal", CalendarID: "primary", EventID: "event-search"}.WithDefaults()
	if err := c.PutCalendarEvent(otherScope); err != nil {
		t.Fatalf("PutCalendarEvent other scope: %v", err)
	}

	for _, query := range []string{"mina", "rae@example.com", "weekly", "agenda", "work-calendar"} {
		results, err := c.SearchCalendarEvents("work-calendar", "work", query, start.Add(-time.Hour), start.Add(2*time.Hour))
		if err != nil {
			t.Fatalf("SearchCalendarEvents(%q): %v", query, err)
		}
		if len(results) != 1 || results[0].Ref.SourceID != "work-calendar" || results[0].Title != "Timezone planning" {
			t.Fatalf("SearchCalendarEvents(%q) = %#v, want scoped match", query, results)
		}
	}

	for _, query := range []string{"provider-secret", "secret-sync-token", "calendar.example"} {
		results, err := c.SearchCalendarEvents("work-calendar", "work", query, start.Add(-time.Hour), start.Add(2*time.Hour))
		if err != nil {
			t.Fatalf("SearchCalendarEvents(%q): %v", query, err)
		}
		if len(results) != 0 {
			t.Fatalf("SearchCalendarEvents(%q) = %#v, want provider internals excluded", query, results)
		}
	}

	if err := c.InvalidateCalendarEvent(event.Ref); err != nil {
		t.Fatalf("InvalidateCalendarEvent: %v", err)
	}
	results, err := c.SearchCalendarEvents("work-calendar", "work", "timezone", start.Add(-time.Hour), start.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("SearchCalendarEvents after invalidate: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("invalidated results = %#v, want none", results)
	}
}
