package models

import (
	"reflect"
	"testing"
	"time"
)

func TestCalendarMeetingPrepQueriesUseUserVisibleContextOnly(t *testing.T) {
	event := CalendarEvent{
		Ref: EventRef{
			SourceID:   "work-calendar",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "provider-secret",
			ETag:       `"provider-etag"`,
		},
		Title:          "Launch planning",
		Location:       "Tokyo Room",
		Start:          time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
		Organizer:      "Mina Park",
		OrganizerEmail: "mina@example.com",
		Attendees: []CalendarAttendee{
			{Name: "Ops Team", Email: "ops@example.com"},
			{Name: "Mina Park", Email: "mina@example.com"},
		},
		Raw: `{"syncToken":"secret"}`,
	}

	got := CalendarMeetingPrepQueries(event)
	want := []string{"Launch planning", "Mina Park", "mina@example.com", "Ops Team", "ops@example.com", "Tokyo Room"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CalendarMeetingPrepQueries = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"provider-secret", "provider-etag", "syncToken", "work-calendar", "primary"} {
		for _, query := range got {
			if query == forbidden {
				t.Fatalf("query leaked provider/internal value %q in %#v", forbidden, got)
			}
		}
	}
}
