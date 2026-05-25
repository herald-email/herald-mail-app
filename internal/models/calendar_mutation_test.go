package models

import (
	"errors"
	"testing"
)

func TestNormalizeCalendarMutationOptionsDefaultsToThisEvent(t *testing.T) {
	opts, err := NormalizeCalendarMutationOptions(CalendarMutationOptions{})
	if err != nil {
		t.Fatalf("NormalizeCalendarMutationOptions: %v", err)
	}
	if opts.RecurrenceScope != CalendarMutationScopeThisEvent {
		t.Fatalf("RecurrenceScope = %q, want %q", opts.RecurrenceScope, CalendarMutationScopeThisEvent)
	}
	if got := CalendarMutationScopeLabel(opts.RecurrenceScope); got != "this event" {
		t.Fatalf("CalendarMutationScopeLabel = %q, want this event", got)
	}
}

func TestNormalizeCalendarMutationOptionsRejectsUnsupportedBroadRecurrenceScope(t *testing.T) {
	_, err := NormalizeCalendarMutationOptions(CalendarMutationOptions{RecurrenceScope: CalendarMutationScopeAllEvents})
	if !errors.Is(err, ErrCalendarRecurrenceScopeUnsupported) {
		t.Fatalf("error = %v, want ErrCalendarRecurrenceScopeUnsupported", err)
	}
}
