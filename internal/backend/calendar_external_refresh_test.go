package backend

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testcalendar"
)

func TestCalendarRefreshPicksUpExternalGoogleEditAfterTTL(t *testing.T) {
	start := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t, testcalendar.WithCalendar("work", "Work", "#123456"), testcalendar.WithEvent("work", testcalendar.Event{ID: "planning", UID: "planning", Summary: "Original title", Start: start, End: start.Add(time.Hour), TimeZone: "UTC", ETag: `"v1"`, Status: "confirmed"}))
	source := lab.GoogleSourceConfig("google-work", "default")
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	b := &LocalBackend{cache: store, cfg: &config.Config{Sources: []config.SourceConfig{source}}}
	events, _, err := b.RefreshCalendarAgenda(time.Time{}, time.Time{})
	if err != nil || len(events) != 1 || events[0].Title != "Original title" {
		t.Fatalf("initial sync = %#v, %v", events, err)
	}
	// Simulate a separate Google Calendar client, without changing Herald's cache.
	req, err := http.NewRequest(http.MethodPatch, source.Google.APIBaseURL+"/calendars/work/events/planning", strings.NewReader(`{"summary":"Externally edited title","start":{"dateTime":"2026-11-26T11:00:00Z","timeZone":"UTC"},"end":{"dateTime":"2026-11-26T12:00:00Z","timeZone":"UTC"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer local-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("external edit returned %s", resp.Status)
	}
	events, _, err = b.RefreshCalendarAgenda(time.Time{}, time.Time{})
	if err != nil || events[0].Title != "Original title" {
		t.Fatal("refresh bypassed existing provider TTL")
	}
	// Advance the sync deadline without a five-minute wall-clock wait.
	b.calendarLastSync[models.SourceID(source.ID)] = time.Now().Add(-calendarAgendaSyncTTL - time.Second)
	events, _, err = b.RefreshCalendarAgenda(time.Time{}, time.Time{})
	if err != nil || len(events) != 1 || events[0].Title != "Externally edited title" || !events[0].Start.Equal(start.Add(time.Hour)) {
		t.Fatalf("refresh on same backend = %#v, %v", events, err)
	}
}
