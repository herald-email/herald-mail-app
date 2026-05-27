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

func TestCalendarEventEditDraftAppliesAttendeesAndRecurrence(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	event := CalendarEvent{
		Ref:      EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "selected-mutations"}.WithDefaults(),
		Title:    "Selected mutations",
		Start:    start,
		End:      start.Add(time.Hour),
		TimeZone: "UTC",
		Attendees: []CalendarAttendee{
			{Name: "Rae Stone", Email: "rae@example.com", RSVP: "accepted"},
			{Name: "Noor Patel", Email: "noor@example.com", RSVP: "tentative", Optional: true},
		},
		Recurrence:        []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
		RecurrenceSummary: "Weekly on Monday",
	}

	draft := NewCalendarEventEditDraft(event)
	if !strings.Contains(draft.AttendeesText, "Rae Stone <rae@example.com> accepted") {
		t.Fatalf("AttendeesText = %q, want existing attendee rendered for editing", draft.AttendeesText)
	}
	if draft.RecurrenceText != "RRULE:FREQ=WEEKLY;BYDAY=MO" {
		t.Fatalf("RecurrenceText = %q, want existing recurrence rule", draft.RecurrenceText)
	}

	draft.AttendeesText = "Mina Park <mina@example.com> accepted; ops@example.com tentative optional"
	draft.RecurrenceText = "RRULE:FREQ=WEEKLY;BYDAY=TU,TH"
	updated, err := draft.Apply(event)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(updated.Attendees) != 2 {
		t.Fatalf("attendees = %#v, want two edited attendees", updated.Attendees)
	}
	if updated.Attendees[0].Name != "Mina Park" || updated.Attendees[0].Email != "mina@example.com" || updated.Attendees[0].RSVP != "accepted" {
		t.Fatalf("first attendee = %#v, want parsed name/email/rsvp", updated.Attendees[0])
	}
	if updated.Attendees[1].Name != "" || updated.Attendees[1].Email != "ops@example.com" || updated.Attendees[1].RSVP != "tentative" || !updated.Attendees[1].Optional {
		t.Fatalf("second attendee = %#v, want optional bare-email attendee", updated.Attendees[1])
	}
	if len(updated.Recurrence) != 1 || updated.Recurrence[0] != "RRULE:FREQ=WEEKLY;BYDAY=TU,TH" {
		t.Fatalf("recurrence = %#v, want edited rule", updated.Recurrence)
	}
	if updated.RecurrenceSummary != "Weekly on Tuesday, Thursday" {
		t.Fatalf("recurrence summary = %q, want updated summary", updated.RecurrenceSummary)
	}
}

func TestCalendarEventEditDraftAppliesReminders(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	event := CalendarEvent{
		Ref:      EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "reminder-mutations"}.WithDefaults(),
		Title:    "Reminder mutations",
		Start:    start,
		End:      start.Add(time.Hour),
		TimeZone: "UTC",
		Reminders: []CalendarReminder{
			{Method: "popup", MinutesBefore: 30},
		},
	}

	draft := NewCalendarEventEditDraft(event)
	if draft.RemindersText != "popup 30m" {
		t.Fatalf("RemindersText = %q, want existing reminder rendered for editing", draft.RemindersText)
	}

	draft.RemindersText = "popup 10m; email 1h"
	updated, err := draft.Apply(event)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(updated.Reminders) != 2 {
		t.Fatalf("reminders = %#v, want two edited reminders", updated.Reminders)
	}
	if updated.Reminders[0] != (CalendarReminder{Method: "popup", MinutesBefore: 10}) {
		t.Fatalf("first reminder = %#v, want popup 10m", updated.Reminders[0])
	}
	if updated.Reminders[1] != (CalendarReminder{Method: "email", MinutesBefore: 60}) {
		t.Fatalf("second reminder = %#v, want email 60m", updated.Reminders[1])
	}
}
