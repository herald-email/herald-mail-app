package models

import (
	"testing"
	"time"
)

func TestEventRefWithDefaultsBuildsScopedLocalID(t *testing.T) {
	ref := EventRef{
		SourceID:   SourceID("work-calendar"),
		AccountID:  AccountID("work"),
		CalendarID: "primary",
		EventID:    "event-123",
		InstanceID: "20260524T160000Z",
		ETag:       `"abc"`,
	}.WithDefaults()

	want := "calendar:work-calendar:work:primary:event-123:20260524T160000Z"
	if ref.LocalID != want {
		t.Fatalf("LocalID = %q, want %q", ref.LocalID, want)
	}
	if ref.SourceID != SourceID("work-calendar") || ref.AccountID != AccountID("work") {
		t.Fatalf("scope = %q/%q, want work-calendar/work", ref.SourceID, ref.AccountID)
	}
}

func TestEventRefWithDefaultsUsesCalendarFallbacks(t *testing.T) {
	ref := EventRef{
		CalendarID: "personal",
		EventID:    "event-1",
	}.WithDefaults()

	if ref.SourceID != DefaultCalendarSourceID {
		t.Fatalf("SourceID = %q, want %q", ref.SourceID, DefaultCalendarSourceID)
	}
	if ref.AccountID != DefaultAccountID {
		t.Fatalf("AccountID = %q, want %q", ref.AccountID, DefaultAccountID)
	}
	if ref.LocalID != "calendar:default-calendar:default:personal:event-1:" {
		t.Fatalf("LocalID = %q, want default scoped calendar local ID", ref.LocalID)
	}
}

func TestCalendarEventRefRoundTrip(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := CalendarEvent{
		Ref: EventRef{
			SourceID:   SourceID("team-calendar"),
			AccountID:  AccountID("team"),
			CalendarID: "release",
			EventID:    "launch",
			ETag:       `"v1"`,
		}.WithDefaults(),
		Title:       "Launch review",
		Description: "Read-only provider proof",
		Location:    "Terminal",
		Start:       start,
		End:         start.Add(time.Hour),
		Status:      "confirmed",
		UpdatedAt:   start.Add(-time.Hour),
	}

	if event.EventRef().LocalID != event.Ref.LocalID {
		t.Fatalf("EventRef LocalID = %q, want %q", event.EventRef().LocalID, event.Ref.LocalID)
	}
	if event.EventRef().ETag != `"v1"` {
		t.Fatalf("EventRef ETag = %q, want %q", event.EventRef().ETag, `"v1"`)
	}
}
