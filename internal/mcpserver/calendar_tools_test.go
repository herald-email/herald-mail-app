package mcpserver

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func seedMCPCalendarCache(t *testing.T) (*cache.Cache, models.CalendarEvent) {
	t.Helper()
	c, err := cache.New(filepath.Join(t.TempDir(), "mcp-calendar-cache.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	start := time.Date(2026, 5, 28, 16, 30, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "work-calendar",
			AccountID:  "work",
			CalendarID: "primary",
			EventID:    "planning-123",
			ETag:       "etag-123",
		}.WithDefaults(),
		Title:             "Roadmap planning",
		Description:       "Discuss source platform next steps.",
		Location:          "Conference Room 2",
		Start:             start,
		End:               start.Add(time.Hour),
		TimeZone:          "America/Los_Angeles",
		Status:            "confirmed",
		Organizer:         "Avery",
		OrganizerEmail:    "avery@example.test",
		RecurrenceSummary: "Does not repeat",
		Attendees: []models.CalendarAttendee{{
			Name:  "Jordan",
			Email: "jordan@example.test",
			RSVP:  "accepted",
		}},
	}
	if err := c.PutCalendarEvent(event); err != nil {
		t.Fatalf("PutCalendarEvent work: %v", err)
	}

	other := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "personal-calendar",
			AccountID:  "personal",
			CalendarID: "home",
			EventID:    "private-456",
		}.WithDefaults(),
		Title: "Private dinner",
		Start: start,
		End:   start.Add(time.Hour),
	}
	if err := c.PutCalendarEvent(other); err != nil {
		t.Fatalf("PutCalendarEvent personal: %v", err)
	}
	return c, event
}

func TestMCPListsCalendarEventsWithScopedRefs(t *testing.T) {
	c, event := seedMCPCalendarCache(t)
	s := newMCPServer(c, nil)

	got := callVirtualLabTool(t, s, 1, "list_calendar_events", map[string]any{
		"source_id":  "work-calendar",
		"account_id": "work",
		"start":      "2026-05-28T00:00:00Z",
		"end":        "2026-05-29T00:00:00Z",
		"limit":      10,
	})
	for _, want := range []string{
		"Roadmap planning",
		"calendar_id=primary",
		"event_id=planning-123",
		"source_id=work-calendar",
		"account_id=work",
		"local_id=" + event.Ref.LocalID,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("list_calendar_events missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Private dinner") {
		t.Fatalf("list_calendar_events leaked another source:\n%s", got)
	}
}

func TestMCPSearchAndGetCalendarEventUseScopedRefs(t *testing.T) {
	c, event := seedMCPCalendarCache(t)
	s := newMCPServer(c, nil)

	search := callVirtualLabTool(t, s, 2, "search_calendar_events", map[string]any{
		"source_id":  "work-calendar",
		"account_id": "work",
		"query":      "source platform",
	})
	if !strings.Contains(search, "Roadmap planning") || !strings.Contains(search, "local_id="+event.Ref.LocalID) {
		t.Fatalf("search_calendar_events did not return scoped event ref:\n%s", search)
	}
	if strings.Contains(search, "Private dinner") {
		t.Fatalf("search_calendar_events leaked another source:\n%s", search)
	}

	detail := callVirtualLabTool(t, s, 3, "get_calendar_event", map[string]any{
		"local_id": event.Ref.LocalID,
	})
	for _, want := range []string{
		"Roadmap planning",
		"Conference Room 2",
		"Organizer: Avery",
		"avery@example.test",
		"Attendees: Jordan",
		"jordan@example.test",
		"(accepted)",
		"event_id=planning-123",
		"source_id=work-calendar",
		"account_id=work",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("get_calendar_event missing %q:\n%s", want, detail)
		}
	}
}

func TestMCPCalendarMutationToolsForwardToDaemon(t *testing.T) {
	localID := "calendar:work-calendar:work:primary:planning-123:"
	var calls []string
	withTestDaemonURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.WriteHeader(http.StatusOK)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.String())
		var body map[string]any
		if r.Body != nil && r.Method != http.MethodDelete {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode daemon body: %v", err)
			}
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/calendar/events":
			if body["calendar_id"] != "primary" || body["title"] != "Created by MCP" || body["start"] != "2026-06-03T16:00:00Z" {
				t.Fatalf("create body = %#v", body)
			}
			if body["start_timezone"] != "America/Los_Angeles" || body["end_timezone"] != "Asia/Tokyo" {
				t.Fatalf("create endpoint zones = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/calendar/events/planning-123":
			if r.URL.Query().Get("local_id") != localID || body["title"] != "Updated by MCP" || body["end_timezone"] != "Europe/London" {
				t.Fatalf("update path/body = %s %#v", r.URL.String(), body)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/calendar/events/planning-123":
			if r.URL.Query().Get("local_id") != localID {
				t.Fatalf("delete query = %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected daemon call: %s %s", r.Method, r.URL.String())
		}
	}))
	s := newMCPServer(newScopedMutationTestMCP(t), nil)

	created := callVirtualLabTool(t, s, 10, "create_calendar_event", map[string]any{
		"source_id":      "work-calendar",
		"account_id":     "work",
		"calendar_id":    "primary",
		"event_id":       "planning-123",
		"title":          "Created by MCP",
		"start":          "2026-06-03T16:00:00Z",
		"end":            "2026-06-03T16:30:00Z",
		"timezone":       "UTC",
		"start_timezone": "America/Los_Angeles",
		"end_timezone":   "Asia/Tokyo",
	})
	if !strings.Contains(created, "Calendar event created") {
		t.Fatalf("create response = %s", created)
	}
	updated := callVirtualLabTool(t, s, 11, "update_calendar_event", map[string]any{
		"local_id":     localID,
		"title":        "Updated by MCP",
		"end_timezone": "Europe/London",
	})
	if !strings.Contains(updated, "Calendar event updated") {
		t.Fatalf("update response = %s", updated)
	}
	deleted := callVirtualLabTool(t, s, 12, "delete_calendar_event", map[string]any{
		"local_id": localID,
	})
	if !strings.Contains(deleted, "Calendar event deleted") {
		t.Fatalf("delete response = %s", deleted)
	}
	if len(calls) != 3 {
		t.Fatalf("daemon calls = %d, want 3: %#v", len(calls), calls)
	}
}
