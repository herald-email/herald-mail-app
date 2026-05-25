package models

import (
	"strings"
	"testing"
	"time"
)

func TestCalendarEventEditDraftAppliesTimezoneAndPreview(t *testing.T) {
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, losAngeles)
	event := CalendarEvent{
		Ref:                EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "timezone-planning"}.WithDefaults(),
		Title:              "Timezone planning",
		Location:           "Video call",
		Start:              start,
		End:                start.Add(time.Hour),
		TimeZone:           "America/Los_Angeles",
		AlternateTimeZones: []string{"Asia/Tokyo"},
		Status:             "confirmed",
	}

	draft := NewCalendarEventEditDraft(event)
	draft.Title = "Timezone planning moved"
	draft.Location = "Tokyo room"
	draft.StartText = "2026-05-24 19:30"
	draft.EndText = "2026-05-24 20:30"
	draft.TimeZone = "America/Los_Angeles"

	updated, err := draft.Apply(event)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if updated.Title != "Timezone planning moved" || updated.Location != "Tokyo room" {
		t.Fatalf("updated event = %#v, want edited title and location", updated)
	}
	if updated.Start.Location().String() != "America/Los_Angeles" {
		t.Fatalf("updated start location = %q, want America/Los_Angeles", updated.Start.Location())
	}
	if updated.Start.Hour() != 19 || updated.End.Hour() != 20 {
		t.Fatalf("updated wall clock = %s-%s, want 19:30-20:30", updated.Start, updated.End)
	}

	preview, err := draft.TimezonePreview(event)
	if err != nil {
		t.Fatalf("TimezonePreview: %v", err)
	}
	if !strings.Contains(preview.Local, "Local") {
		t.Fatalf("preview local row = %q, want labeled local row", preview.Local)
	}
	if !strings.Contains(preview.Event, "America/Los_Angeles") {
		t.Fatalf("preview event row = %q, want event timezone", preview.Event)
	}
	if len(preview.Alternates) != 1 || !strings.Contains(preview.Alternates[0], "Asia/Tokyo") {
		t.Fatalf("preview alternates = %#v, want Asia/Tokyo", preview.Alternates)
	}
	if !strings.Contains(preview.DateCrossingNote, "date changes") {
		t.Fatalf("date crossing note = %q, want explicit date crossing", preview.DateCrossingNote)
	}
}

func TestCalendarEventEditDraftValidatesTimeAndTimezone(t *testing.T) {
	event := CalendarEvent{
		Ref:      EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "bad-time"}.WithDefaults(),
		Title:    "Bad time",
		Start:    time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		TimeZone: "UTC",
	}

	draft := NewCalendarEventEditDraft(event)
	draft.TimeZone = "Mars/Base"
	if _, err := draft.Apply(event); err == nil || !strings.Contains(err.Error(), "timezone") {
		t.Fatalf("Apply invalid timezone err=%v, want timezone validation", err)
	}

	draft = NewCalendarEventEditDraft(event)
	draft.StartText = "2026-05-24 11:00"
	draft.EndText = "2026-05-24 10:00"
	if _, err := draft.Apply(event); err == nil || !strings.Contains(err.Error(), "end") {
		t.Fatalf("Apply invalid range err=%v, want end-after-start validation", err)
	}
}
