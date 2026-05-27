package models

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCalendarTravelBufferQueriesUseUserVisibleTravelContextOnly(t *testing.T) {
	event := CalendarEvent{
		Ref: EventRef{
			SourceID:   "work-calendar",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "provider-secret",
			ETag:       `"provider-etag"`,
		},
		Title:          "Partner onsite",
		Location:       "SFO Terminal 2",
		Start:          time.Date(2026, 5, 24, 14, 0, 0, 0, time.UTC),
		Organizer:      "Mina Park",
		OrganizerEmail: "mina@example.com",
		Attendees: []CalendarAttendee{
			{Name: "Ops Team", Email: "ops@example.com"},
			{Name: "Mina Park", Email: "mina@example.com"},
		},
		Raw: `{"syncToken":"secret"}`,
	}

	got := CalendarTravelBufferQueries(event)
	want := []string{
		"Partner onsite",
		"SFO Terminal 2",
		"Mina Park",
		"mina@example.com",
		"Ops Team",
		"ops@example.com",
		"flight",
		"airport",
		"train",
		"hotel",
		"travel",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CalendarTravelBufferQueries = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"provider-secret", "provider-etag", "syncToken", "work-calendar", "primary"} {
		for _, query := range got {
			if query == forbidden {
				t.Fatalf("query leaked provider/internal value %q in %#v", forbidden, got)
			}
		}
	}
}

func TestCalendarTravelBufferRecommendationsUseMailAndNearbyGaps(t *testing.T) {
	start := time.Date(2026, 5, 24, 14, 0, 0, 0, time.UTC)
	event := CalendarEvent{
		Title:    "Partner onsite",
		Location: "SFO Terminal 2",
		Start:    start,
		End:      start.Add(time.Hour),
	}
	mail := []*EmailData{{
		Sender:  "airline@example.com",
		Subject: "Flight itinerary for SFO",
		Folder:  "Travel",
		Date:    start.Add(-24 * time.Hour),
	}}
	nearby := []CalendarEvent{{
		Title:    "Team sync",
		Location: "Downtown office",
		Start:    start.Add(-45 * time.Minute),
		End:      start.Add(-15 * time.Minute),
	}}

	recs := CalendarTravelBufferRecommendations(event, mail, nearby)
	if len(recs) < 2 {
		t.Fatalf("recommendations = %#v, want travel-mail and nearby-gap suggestions", recs)
	}
	joined := strings.ToLower(recs[0].Label + " " + recs[0].Window + " " + recs[0].Reason)
	if !strings.Contains(joined, "arrive") || !strings.Contains(joined, "90 min") || !strings.Contains(joined, "flight itinerary") {
		t.Fatalf("first recommendation should name travel mail signal and arrival buffer: %#v", recs)
	}
	foundGap := false
	for _, rec := range recs {
		text := strings.ToLower(rec.Label + " " + rec.Reason)
		if strings.Contains(text, "gap") && strings.Contains(text, "team sync") {
			foundGap = true
		}
	}
	if !foundGap {
		t.Fatalf("recommendations = %#v, want nearby event gap warning", recs)
	}
}
