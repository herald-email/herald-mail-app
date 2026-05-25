package calendar

import (
	"context"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testcalendar"
)

func TestGoogleCalendarSourceUsesLocalTestServer(t *testing.T) {
	start := time.Date(2026, 5, 24, 17, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:          "evt-1",
			Summary:     "Phase 6 Google proof",
			Description: "Fetched through local provider harness",
			Location:    "Localhost",
			Start:       start,
			End:         start.Add(time.Hour),
			ETag:        `"g-v1"`,
			Updated:     start.Add(-time.Hour),
			Status:      "confirmed",
		}),
	)

	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}

	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 1 || collections[0].Ref.CollectionID != "primary" || collections[0].Ref.SourceID != models.SourceID("work-calendar") {
		t.Fatalf("collections = %#v, want local primary collection scoped to work-calendar", collections)
	}

	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Phase 6 Google proof" || events[0].Ref.EventID != "evt-1" {
		t.Fatalf("events = %#v, want local event evt-1", events)
	}

	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.Ref.LocalID != events[0].Ref.LocalID || got.Ref.ETag != `"g-v1"` {
		t.Fatalf("FetchEvent = %#v, want same scoped event with etag", got)
	}
}

func TestGoogleCalendarSourceParsesRichEventDetail(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:          "rich-evt",
			Summary:     "Timezone planning",
			Description: "Review attendee status before editing is enabled.",
			Location:    "Video call",
			Start:       start,
			End:         start.Add(time.Hour),
			TimeZone:    "America/Los_Angeles",
			Organizer: testcalendar.Person{
				Name:  "Mina Park",
				Email: "mina@example.com",
			},
			Attendees: []testcalendar.Attendee{
				{Name: "Rae Stone", Email: "rae@example.com", ResponseStatus: "accepted"},
				{Name: "Noor Patel", Email: "noor@example.com", ResponseStatus: "tentative", Optional: true},
			},
			Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
			Attachments: []testcalendar.Attachment{
				{Title: "Agenda", FileURL: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
			},
		}),
	)

	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.TimeZone != "America/Los_Angeles" || got.Organizer != "Mina Park" || got.OrganizerEmail != "mina@example.com" {
		t.Fatalf("rich organizer/timezone = %#v", got)
	}
	if len(got.Attendees) != 2 || got.Attendees[0].RSVP != "accepted" || !got.Attendees[1].Optional {
		t.Fatalf("attendees = %#v", got.Attendees)
	}
	if got.RecurrenceSummary != "Weekly on Monday" || len(got.Recurrence) != 1 {
		t.Fatalf("recurrence = %#v summary=%q", got.Recurrence, got.RecurrenceSummary)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Title != "Agenda" || got.Attachments[0].MIMEType != "application/pdf" {
		t.Fatalf("attachments = %#v", got.Attachments)
	}
}

func TestCalDAVSourceUsesLocalTestServer(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("team", "Team Calendar", "#0b8043"),
		testcalendar.WithEvent("team", testcalendar.Event{
			ID:          "team-standup.ics",
			UID:         "team-standup",
			Summary:     "Phase 6 CalDAV proof",
			Description: "Fetched through local CalDAV harness",
			Location:    "Terminal",
			Start:       start,
			End:         start.Add(30 * time.Minute),
			ETag:        `"c-v1"`,
			Updated:     start.Add(-time.Hour),
			Status:      "CONFIRMED",
		}),
	)

	src, err := NewCalDAVSource(lab.CalDAVSourceConfig("family-caldav", "family"))
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}

	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 1 || collections[0].Ref.CollectionID != "team" || collections[0].Ref.DisplayName != "Team Calendar" {
		t.Fatalf("collections = %#v, want local team CalDAV collection", collections)
	}

	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Phase 6 CalDAV proof" || events[0].ProviderUID != "team-standup" {
		t.Fatalf("events = %#v, want parsed local CalDAV event", events)
	}

	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.Ref.LocalID != events[0].Ref.LocalID || got.Ref.ETag != `"c-v1"` {
		t.Fatalf("FetchEvent = %#v, want same scoped event with etag", got)
	}
}

func TestCalDAVSourceParsesRichEventDetail(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("team", "Team Calendar", "#0b8043"),
		testcalendar.WithEvent("team", testcalendar.Event{
			ID:          "rich-evt.ics",
			UID:         "rich-evt",
			Summary:     "Timezone planning",
			Description: "Review attendee status before editing is enabled.",
			Location:    "Video call",
			Start:       start,
			End:         start.Add(time.Hour),
			TimeZone:    "America/Los_Angeles",
			Status:      "CONFIRMED",
			Organizer: testcalendar.Person{
				Name:  "Mina Park",
				Email: "mina@example.com",
			},
			Attendees: []testcalendar.Attendee{
				{Name: "Rae Stone", Email: "rae@example.com", ResponseStatus: "ACCEPTED"},
				{Name: "Noor Patel", Email: "noor@example.com", ResponseStatus: "TENTATIVE", Optional: true},
			},
			Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
			Attachments: []testcalendar.Attachment{
				{Title: "Agenda", FileURL: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
			},
		}),
	)

	src, err := NewCalDAVSource(lab.CalDAVSourceConfig("family-caldav", "family"))
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.TimeZone != "America/Los_Angeles" || got.Organizer != "Mina Park" || got.OrganizerEmail != "mina@example.com" {
		t.Fatalf("rich organizer/timezone = %#v", got)
	}
	if len(got.Attendees) != 2 || got.Attendees[0].RSVP != "accepted" || !got.Attendees[1].Optional {
		t.Fatalf("attendees = %#v", got.Attendees)
	}
	if got.RecurrenceSummary != "Weekly on Monday" || len(got.Recurrence) != 1 {
		t.Fatalf("recurrence = %#v summary=%q", got.Recurrence, got.RecurrenceSummary)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Title != "Agenda" || got.Attachments[0].MIMEType != "application/pdf" {
		t.Fatalf("attachments = %#v", got.Attachments)
	}
}
