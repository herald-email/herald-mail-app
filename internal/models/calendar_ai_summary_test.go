package models

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCalendarAISummaryQueriesUseUserVisibleContextOnly(t *testing.T) {
	event := CalendarEvent{
		Title:          "Launch planning",
		Location:       "HQ Room 4",
		Organizer:      "Mina Park",
		OrganizerEmail: "mina@example.com",
		Attendees: []CalendarAttendee{
			{Name: "Ari Lane", Email: "ari@example.com"},
			{Name: "Mina Park", Email: "mina@example.com"},
		},
		ProviderUID: "provider-secret",
		Ref: EventRef{
			SourceID:   "work-calendar",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "provider-event-id",
			ETag:       `"provider-etag"`,
		},
		Raw: `{"syncToken":"secret"}`,
	}

	got := CalendarAISummaryQueries(event)
	want := []string{"Launch planning", "Mina Park", "mina@example.com", "Ari Lane", "ari@example.com", "HQ Room 4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CalendarAISummaryQueries = %#v, want %#v", got, want)
	}
	forbidden := strings.Join(got, " ")
	for _, token := range []string{"provider-secret", "provider-etag", "syncToken", "provider-event-id"} {
		if strings.Contains(forbidden, token) {
			t.Fatalf("summary queries leaked provider internals %q in %#v", token, got)
		}
	}
}

func TestCalendarAISummaryFallbackUsesCachedContext(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := CalendarEvent{
		Title:     "Launch planning",
		Organizer: "Mina Park",
		Start:     start,
		End:       start.Add(time.Hour),
		Ref:       EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "launch"}.WithDefaults(),
	}
	mail := []*EmailData{
		{
			SourceID:  "work-mail",
			AccountID: "work",
			MessageID: "memo",
			Folder:    "INBOX",
			Sender:    "mina@example.com",
			Subject:   "Launch planning risks",
			Date:      start.Add(-time.Hour),
		},
	}
	nearby := []CalendarEvent{
		{
			Title: "Launch retro",
			Start: start.Add(2 * time.Hour),
			End:   start.Add(3 * time.Hour),
			Ref:   EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "retro"}.WithDefaults(),
		},
	}

	summary := CalendarAISummaryFallback(event, mail, nearby, []string{"Launch planning"}, "AI unavailable")
	if summary.GeneratedBy != CalendarAISummaryGeneratedByFallback {
		t.Fatalf("GeneratedBy = %q, want fallback", summary.GeneratedBy)
	}
	if len(summary.Bullets) == 0 || len(summary.ActionItems) == 0 {
		t.Fatalf("fallback summary missing bullets/action items: %#v", summary)
	}
	rendered := strings.Join(append(summary.Bullets, summary.ActionItems...), " ")
	for _, want := range []string{"Launch planning", "1 cached related mail", "1 nearby event", "Launch planning risks"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("fallback summary missing %q in %q", want, rendered)
		}
	}
	if !strings.Contains(summary.AINote, "AI unavailable") {
		t.Fatalf("AINote = %q, want AI error context", summary.AINote)
	}
}
