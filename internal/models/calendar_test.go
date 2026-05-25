package models

import (
	"testing"
	"time"
)

func TestCalendarEventCanonicalTimeZone(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	event := CalendarEvent{Start: time.Date(2026, 5, 24, 18, 30, 0, 0, loc)}
	if got := event.CanonicalTimeZone(); got != "PDT" {
		t.Fatalf("CanonicalTimeZone fallback = %q, want PDT", got)
	}

	event.TimeZone = "America/Los_Angeles"
	if got := event.CanonicalTimeZone(); got != "America/Los_Angeles" {
		t.Fatalf("CanonicalTimeZone explicit = %q, want America/Los_Angeles", got)
	}
}
