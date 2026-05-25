package models

import (
	"testing"
	"time"
)

func TestMailAutomationEventDefaultsSourceScope(t *testing.T) {
	email := &EmailData{
		SourceID:  "mail-work",
		AccountID: "work",
		MessageID: "msg-1",
		UID:       42,
		Sender:    "alice@example.com",
		Subject:   "Hello",
		Folder:    "INBOX",
		Date:      time.Now(),
	}

	event := NewMailAutomationEvent(email, "newsletter").WithDefaults()

	if event.Kind != AutomationEventMailMessageReceived {
		t.Fatalf("Kind = %q, want %q", event.Kind, AutomationEventMailMessageReceived)
	}
	if event.SourceID != "mail-work" {
		t.Fatalf("SourceID = %q, want mail-work", event.SourceID)
	}
	if event.AccountID != "work" {
		t.Fatalf("AccountID = %q, want work", event.AccountID)
	}
	if event.Collection.Kind != SourceKindMail || event.Collection.CollectionID != "INBOX" {
		t.Fatalf("Collection = %#v, want mail INBOX", event.Collection)
	}
	if event.MessageRef.MessageID != "msg-1" || event.MessageRef.LocalID == "" {
		t.Fatalf("MessageRef = %#v, want msg-1 with local id", event.MessageRef)
	}
	if event.Category != "newsletter" {
		t.Fatalf("Category = %q, want newsletter", event.Category)
	}
	if event.ItemID() != event.MessageRef.LocalID {
		t.Fatalf("ItemID = %q, want local message id %q", event.ItemID(), event.MessageRef.LocalID)
	}
}

func TestCalendarAutomationEventDefaultsSourceScope(t *testing.T) {
	calendarEvent := CalendarEvent{
		Ref: EventRef{
			SourceID:   "calendar-work",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "event-1",
		},
		Title: "Planning",
		Start: time.Now(),
		End:   time.Now().Add(time.Hour),
	}

	event := NewCalendarAutomationEvent(calendarEvent).WithDefaults()

	if event.Kind != AutomationEventCalendarEventChanged {
		t.Fatalf("Kind = %q, want %q", event.Kind, AutomationEventCalendarEventChanged)
	}
	if event.SourceID != "calendar-work" {
		t.Fatalf("SourceID = %q, want calendar-work", event.SourceID)
	}
	if event.AccountID != "work" {
		t.Fatalf("AccountID = %q, want work", event.AccountID)
	}
	if event.Collection.Kind != SourceKindCalendar || event.Collection.CollectionID != "primary" {
		t.Fatalf("Collection = %#v, want calendar primary", event.Collection)
	}
	if event.EventRef.EventID != "event-1" || event.EventRef.LocalID == "" {
		t.Fatalf("EventRef = %#v, want event-1 with local id", event.EventRef)
	}
	if event.ItemID() != event.EventRef.LocalID {
		t.Fatalf("ItemID = %q, want local event id %q", event.ItemID(), event.EventRef.LocalID)
	}
}
