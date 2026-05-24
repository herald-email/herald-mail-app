package cache

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestCalendarCacheTablesAreSourceScoped(t *testing.T) {
	c := newTestCache(t)

	calCols := tableColumns(t, c.db, "calendar_collections")
	for _, name := range []string{"source_id", "account_id", "calendar_id", "local_id", "display_name", "sync_token", "etag"} {
		if !calCols[name] {
			t.Fatalf("calendar_collections missing column %s", name)
		}
	}

	eventCols := tableColumns(t, c.db, "calendar_events")
	for _, name := range []string{"source_id", "account_id", "calendar_id", "event_id", "instance_id", "local_id", "etag", "revision", "starts_at", "ends_at", "invalidated_at"} {
		if !eventCols[name] {
			t.Fatalf("calendar_events missing column %s", name)
		}
	}
}

func TestCacheCalendarCollectionRoundTrip(t *testing.T) {
	c := newTestCache(t)

	collection := models.CalendarCollection{
		Ref: models.CollectionRef{
			SourceID:     models.SourceID("work-calendar"),
			AccountID:    models.AccountID("work"),
			Kind:         models.SourceKindCalendar,
			CollectionID: "primary",
			DisplayName:  "Work",
		},
		Color:     "#3367d6",
		SyncToken: "sync-1",
		ETag:      `"cal-v1"`,
	}
	if err := c.PutCalendarCollection(collection); err != nil {
		t.Fatalf("PutCalendarCollection: %v", err)
	}

	got, err := c.GetCalendarCollection(collection.Ref)
	if err != nil {
		t.Fatalf("GetCalendarCollection: %v", err)
	}
	if got.Ref.SourceID != collection.Ref.SourceID || got.Ref.AccountID != collection.Ref.AccountID || got.Ref.CollectionID != "primary" {
		t.Fatalf("collection scope = %#v, want %#v", got.Ref, collection.Ref)
	}
	if got.Color != "#3367d6" || got.SyncToken != "sync-1" || got.ETag != `"cal-v1"` {
		t.Fatalf("collection metadata = %#v, want color/sync/etag", got)
	}
}

func TestCacheCalendarEventRoundTripAndInvalidate(t *testing.T) {
	c := newTestCache(t)
	start := time.Date(2026, 5, 24, 16, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   models.SourceID("work-calendar"),
			AccountID:  models.AccountID("work"),
			CalendarID: "primary",
			EventID:    "event-1",
			ETag:       `"event-v1"`,
		}.WithDefaults(),
		Title:       "Phase 6 review",
		Description: "Calendar cache foundation",
		Location:    "Herald",
		Start:       start,
		End:         start.Add(time.Hour),
		Status:      "confirmed",
		Revision:    "rev-1",
		UpdatedAt:   start.Add(-time.Hour),
		Raw:         `{"id":"event-1"}`,
	}

	if err := c.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent: %v", err)
	}
	got, err := c.GetCalendarEventByRef(event.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEventByRef: %v", err)
	}
	if got.Ref.LocalID != event.Ref.LocalID || got.Title != event.Title || got.Ref.ETag != `"event-v1"` || got.Revision != "rev-1" {
		t.Fatalf("event roundtrip = %#v, want %#v", got, event)
	}

	if err := c.InvalidateCalendarEvent(event.Ref); err != nil {
		t.Fatalf("InvalidateCalendarEvent: %v", err)
	}
	if _, err := c.GetCalendarEventByRef(event.Ref); err == nil {
		t.Fatal("GetCalendarEventByRef succeeded after invalidation, want miss")
	}
}
