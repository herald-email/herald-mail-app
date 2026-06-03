package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testcalendar"
)

func xmlEscapeForTest(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}

func testCalDAVICS(uid, summary string, start time.Time) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\nSUMMARY:" + summary + "\r\nDTSTART:" + start.UTC().Format("20060102T150405Z") + "\r\nDTEND:" + start.Add(time.Hour).UTC().Format("20060102T150405Z") + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

func TestGoogleCalendarSourceUsesLocalTestServer(t *testing.T) {
	start := time.Date(2026, 5, 24, 17, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:          "evt-1",
			Summary:     "Phase 6 Google proof",
			Description: "Fetched through local provider harness",
			Location:    "Localhost",
			Start:       start,
			End:         start.Add(time.Hour),
			ETag:        `"g-v1"`,
			Updated:     start.Add(-time.Hour),
			Status:      "confirmed",
		}),
	)

	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}

	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 1 || collections[0].Ref.CollectionID != "primary" || collections[0].Ref.SourceID != models.SourceID("work-calendar") || collections[0].AccessRole != "owner" {
		t.Fatalf("collections = %#v, want local primary collection scoped to work-calendar", collections)
	}

	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Phase 6 Google proof" || events[0].Ref.EventID != "evt-1" {
		t.Fatalf("events = %#v, want local event evt-1", events)
	}

	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.Ref.LocalID != events[0].Ref.LocalID || got.Ref.ETag != `"g-v1"` {
		t.Fatalf("FetchEvent = %#v, want same scoped event with etag", got)
	}
}

func TestGoogleCalendarSourceRefreshesExpiredOAuthTokenBeforeListCalendars(t *testing.T) {
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "calendar-client-id")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "calendar-client-secret")

	var tokenRefreshes int
	var calendarAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenRefreshes++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "calendar-refresh-token" {
				t.Fatalf("refresh form = %s, want refresh_token grant", r.Form.Encode())
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fresh-calendar-token",
				"expires_in":   3600,
				"token_type":   "Bearer",
			})
		case "/calendar/v3/users/me/calendarList":
			calendarAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]string{{"id": "primary", "summary": "Work"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	src, err := NewGoogleCalendarSource(config.SourceConfig{
		ID:        "work-calendar",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "google_calendar",
		AccountID: "work",
		Google: config.GoogleConfig{
			AccessToken:  "expired-calendar-token",
			RefreshToken: "calendar-refresh-token",
			TokenExpiry:  time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			APIBaseURL:   server.URL + "/calendar/v3",
			TokenURL:     server.URL + "/token",
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 1 || collections[0].Ref.CollectionID != "primary" {
		t.Fatalf("collections = %#v, want refreshed calendar list", collections)
	}
	if tokenRefreshes != 1 {
		t.Fatalf("token refreshes = %d, want 1", tokenRefreshes)
	}
	if calendarAuth != "Bearer fresh-calendar-token" {
		t.Fatalf("calendar Authorization = %q, want refreshed bearer token", calendarAuth)
	}
}

func TestGoogleCalendarSourceListEventsWithSyncTokenExposesNextAndDeletedRefs(t *testing.T) {
	var eventQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendar/v3/calendars/primary/events" {
			http.NotFound(w, r)
			return
		}
		eventQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"nextSyncToken": "sync-primary-v2",
			"items": []map[string]any{
				{
					"id":      "active-event",
					"etag":    `"active-v2"`,
					"summary": "Updated planning",
					"status":  "confirmed",
					"start":   map[string]string{"dateTime": "2026-05-25T10:00:00Z"},
					"end":     map[string]string{"dateTime": "2026-05-25T11:00:00Z"},
				},
				{
					"id":     "deleted-event",
					"etag":   `"deleted-v2"`,
					"status": "cancelled",
					"start":  map[string]string{"dateTime": "2026-05-25T12:00:00Z"},
					"end":    map[string]string{"dateTime": "2026-05-25T13:00:00Z"},
				},
			},
		})
	}))
	defer server.Close()

	src, err := NewGoogleCalendarSource(config.SourceConfig{
		ID:        "work-calendar",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "google_calendar",
		AccountID: "work",
		Google: config.GoogleConfig{
			AccessToken: "local-token",
			APIBaseURL:  server.URL + "/calendar/v3",
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	result, err := src.ListEventsWithSyncToken(context.Background(), models.CollectionRef{
		SourceID:     "work-calendar",
		AccountID:    "work",
		Kind:         models.SourceKindCalendar,
		CollectionID: "primary",
	}, "sync-primary-v1")
	if err != nil {
		t.Fatalf("ListEventsWithSyncToken: %v", err)
	}
	if eventQuery.Get("syncToken") != "sync-primary-v1" {
		t.Fatalf("syncToken query = %q, want cached token", eventQuery.Get("syncToken"))
	}
	if eventQuery.Get("showDeleted") != "true" {
		t.Fatalf("showDeleted query = %q, want true for incremental sync", eventQuery.Get("showDeleted"))
	}
	if result.NextSyncToken != "sync-primary-v2" {
		t.Fatalf("NextSyncToken = %q, want provider token", result.NextSyncToken)
	}
	if len(result.Events) != 1 || result.Events[0].Ref.EventID != "active-event" {
		t.Fatalf("Events = %#v, want only active event", result.Events)
	}
	if len(result.DeletedRefs) != 1 || result.DeletedRefs[0].EventID != "deleted-event" {
		t.Fatalf("DeletedRefs = %#v, want deleted-event ref", result.DeletedRefs)
	}
}

func TestGoogleCalendarSourceParsesRichEventDetail(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:          "rich-evt",
			Summary:     "Timezone planning",
			Description: "Review attendee status before editing is enabled.",
			Location:    "Video call",
			Start:       start,
			End:         start.Add(time.Hour),
			TimeZone:    "America/Los_Angeles",
			Organizer: testcalendar.Person{
				Name:  "Mina Park",
				Email: "mina@example.com",
			},
			Attendees: []testcalendar.Attendee{
				{Name: "Rae Stone", Email: "rae@example.com", ResponseStatus: "accepted"},
				{Name: "Noor Patel", Email: "noor@example.com", ResponseStatus: "tentative", Optional: true},
			},
			Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
			Attachments: []testcalendar.Attachment{
				{Title: "Agenda", FileURL: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
			},
			Reminders: []testcalendar.Reminder{
				{Method: "popup", MinutesBefore: 10},
				{Method: "email", MinutesBefore: 60},
			},
		}),
	)

	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.TimeZone != "America/Los_Angeles" || got.Organizer != "Mina Park" || got.OrganizerEmail != "mina@example.com" {
		t.Fatalf("rich organizer/timezone = %#v", got)
	}
	if len(got.Attendees) != 2 || got.Attendees[0].RSVP != "accepted" || !got.Attendees[1].Optional {
		t.Fatalf("attendees = %#v", got.Attendees)
	}
	if got.RecurrenceSummary != "Weekly on Monday" || len(got.Recurrence) != 1 {
		t.Fatalf("recurrence = %#v summary=%q", got.Recurrence, got.RecurrenceSummary)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Title != "Agenda" || got.Attachments[0].MIMEType != "application/pdf" {
		t.Fatalf("attachments = %#v", got.Attachments)
	}
	if len(got.Reminders) != 2 || got.Reminders[0].Method != "popup" || got.Reminders[0].MinutesBefore != 10 || got.Reminders[1].Method != "email" || got.Reminders[1].MinutesBefore != 60 {
		t.Fatalf("reminders = %#v", got.Reminders)
	}
}

func TestCalDAVSourceParsesICSDateVariantsWithoutZeroTime(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:offset-event\r\nSUMMARY:Offset event\r\nDTSTART;TZID=/freeassociation.sourceforge.net/America/Los_Angeles:20260524T1830\r\nDTEND:20260524T194500-0700\r\nLAST-MODIFIED:20260524T1200Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	event, err := eventFromICS("work-calendar", "work", "primary", "offset-event.ics", `"etag"`, ics)
	if err != nil {
		t.Fatalf("eventFromICS: %v", err)
	}
	if event.Start.IsZero() || event.End.IsZero() || event.UpdatedAt.IsZero() {
		t.Fatalf("parsed times should not be zero: start=%v end=%v updated=%v", event.Start, event.End, event.UpdatedAt)
	}
	if event.TimeZone != "/freeassociation.sourceforge.net/America/Los_Angeles" {
		t.Fatalf("TimeZone = %q, want raw provider TZID preserved", event.TimeZone)
	}
	if got := event.Start.In(time.FixedZone("PDT", -7*60*60)).Format("2006-01-02 15:04"); got != "2026-05-24 18:30" {
		t.Fatalf("start = %s, want 2026-05-24 18:30 PDT", got)
	}
	if got := event.End.In(time.FixedZone("PDT", -7*60*60)).Format("2006-01-02 15:04"); got != "2026-05-24 19:45" {
		t.Fatalf("end = %s, want 2026-05-24 19:45 PDT", got)
	}
	if got := event.UpdatedAt.UTC().Format("2006-01-02 15:04"); got != "2026-05-24 12:00" {
		t.Fatalf("updated = %s, want 2026-05-24 12:00 UTC", got)
	}
}

func TestCalDAVSourceParsesVEVENTFieldsWithoutTrailingTimezoneOverwrite(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:phantom",
		"SUMMARY:Phantom",
		"DTSTART;TZID=America/Los_Angeles:20260530T193000",
		"DTEND;TZID=America/Los_Angeles:20260530T223000",
		"END:VEVENT",
		"BEGIN:VTIMEZONE",
		"TZID:America/Los_Angeles",
		"BEGIN:STANDARD",
		"DTSTART:20071104T020000",
		"RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU",
		"END:STANDARD",
		"END:VTIMEZONE",
		"END:VCALENDAR",
	}, "\r\n") + "\r\n"

	event, err := eventFromICS("icloud-calendar", "icloud", "home", "phantom.ics", `"etag"`, ics)
	if err != nil {
		t.Fatalf("eventFromICS: %v", err)
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	if got := event.Start.In(loc).Format("2006-01-02 15:04 MST"); got != "2026-05-30 19:30 PDT" {
		t.Fatalf("start = %s, want VEVENT DTSTART 2026-05-30 19:30 PDT", got)
	}
	if got := event.End.In(loc).Format("2006-01-02 15:04 MST"); got != "2026-05-30 22:30 PDT" {
		t.Fatalf("end = %s, want VEVENT DTEND 2026-05-30 22:30 PDT", got)
	}
}

func TestCalendarDateOnlyProviderTimesUseLocalCalendarDate(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	oldLocal := time.Local
	time.Local = loc
	defer func() { time.Local = oldLocal }()

	googleStart, allDay, ok := parseGoogleTime(googleEventTime{Date: "2026-05-25"})
	if !ok {
		t.Fatal("parseGoogleTime failed")
	}
	if !allDay {
		t.Fatal("parseGoogleTime date should be all-day")
	}
	if got := googleStart.Local().Format("2006-01-02 15:04 MST"); got != "2026-05-25 00:00 PDT" {
		t.Fatalf("Google date parsed as %s, want 2026-05-25 00:00 PDT", got)
	}

	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:all-day\r\nSUMMARY:No school\r\nDTSTART;VALUE=DATE:20260525\r\nDTEND;VALUE=DATE:20260526\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	event, err := eventFromICS("icloud-calendar", "icloud", "home", "all-day.ics", `"etag"`, ics)
	if err != nil {
		t.Fatalf("eventFromICS: %v", err)
	}
	if !event.AllDay {
		t.Fatal("ICS VALUE=DATE should be all-day")
	}
	if got := event.Start.Local().Format("2006-01-02 15:04 MST"); got != "2026-05-25 00:00 PDT" {
		t.Fatalf("ICS DTSTART parsed as %s, want 2026-05-25 00:00 PDT", got)
	}
	if got := event.End.Local().Format("2006-01-02 15:04 MST"); got != "2026-05-26 00:00 PDT" {
		t.Fatalf("ICS exclusive DTEND parsed as %s, want 2026-05-26 00:00 PDT", got)
	}
}

func TestCalDAVSourceRejectsInvalidICSStart(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:bad-event\r\nSUMMARY:Bad event\r\nDTSTART:THIS-IS-NOT-A-DATE\r\nDTEND:20260524T194500Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := eventFromICS("work-calendar", "work", "primary", "bad-event.ics", `"etag"`, ics); err == nil {
		t.Fatal("eventFromICS succeeded with invalid DTSTART, want error")
	}
}

func TestGoogleCalendarSourceUpdateEventWritesThroughProvider(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:          "update-evt",
			UID:         "update-evt",
			Summary:     "Provider planning",
			Description: "Original notes",
			Location:    "Room A",
			Start:       start,
			End:         start.Add(time.Hour),
			TimeZone:    "UTC",
			ETag:        `"g-v1"`,
			Status:      "confirmed",
		}),
	)
	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	events, err := src.ListEvents(context.Background(), models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	edited := events[0]
	edited.Title = "Provider planning moved"
	edited.Description = "Updated through PATCH"
	edited.Location = "Room B"
	edited.Start = start.Add(2 * time.Hour)
	edited.End = start.Add(3 * time.Hour)
	edited.TimeZone = "America/Los_Angeles"
	edited.Reminders = []models.CalendarReminder{{Method: "popup", MinutesBefore: 15}}

	saved, err := src.UpdateEvent(context.Background(), edited, models.CalendarMutationOptions{
		RecurrenceScope: models.CalendarMutationScopeThisEvent,
		IfMatch:         edited.Ref.ETag,
	})
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if saved.Title != edited.Title || saved.Location != edited.Location || saved.TimeZone != edited.TimeZone {
		t.Fatalf("saved = %#v, want edited fields", saved)
	}
	if len(saved.Reminders) != 1 || saved.Reminders[0].Method != "popup" || saved.Reminders[0].MinutesBefore != 15 {
		t.Fatalf("saved reminders = %#v, want edited reminder override", saved.Reminders)
	}
	if saved.Ref.ETag == edited.Ref.ETag {
		t.Fatalf("saved etag = %q, want provider freshness to advance", saved.Ref.ETag)
	}

	fetched, err := src.FetchEvent(context.Background(), saved.Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if fetched.Title != edited.Title || fetched.Location != edited.Location {
		t.Fatalf("fetched = %#v, want provider-updated event", fetched)
	}
	if len(fetched.Reminders) != 1 || fetched.Reminders[0].MinutesBefore != 15 {
		t.Fatalf("fetched reminders = %#v, want provider-updated reminder", fetched.Reminders)
	}
}

func TestGoogleCalendarSourceImportsEventAndFindsDuplicateUID(t *testing.T) {
	start := time.Date(2026, 6, 2, 16, 0, 0, 0, time.UTC)
	lab := testcalendar.Start(t, testcalendar.WithCalendar("primary", "Work", "#3367d6"))
	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	event := models.CalendarEvent{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "email-invite@example.test"}.WithDefaults(),
		ProviderUID: "email-invite@example.test",
		Title:       "Email invite",
		Start:       start,
		End:         start.Add(30 * time.Minute),
		TimeZone:    "UTC",
		Status:      "confirmed",
		Raw: strings.Join([]string{
			"BEGIN:VCALENDAR",
			"VERSION:2.0",
			"BEGIN:VEVENT",
			"UID:email-invite@example.test",
			"SUMMARY:Email invite",
			"DTSTART:20260602T160000Z",
			"DTEND:20260602T163000Z",
			"END:VEVENT",
			"END:VCALENDAR",
		}, "\r\n"),
	}

	saved, err := src.CreateEvent(context.Background(), event, models.CalendarMutationOptions{})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if saved.ProviderUID != event.ProviderUID || saved.Title != event.Title {
		t.Fatalf("saved event = %#v, want imported event", saved)
	}
	duplicate, err := src.FindEventByUID(context.Background(), models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"}, event.ProviderUID)
	if err != nil {
		t.Fatalf("FindEventByUID: %v", err)
	}
	if duplicate == nil || duplicate.Ref.EventID != saved.Ref.EventID {
		t.Fatalf("duplicate = %#v, want saved event %q", duplicate, saved.Ref.EventID)
	}
}

func TestGoogleCalendarSourceCreateEventSurfacesMissingWritePermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/events/import") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Request had insufficient authentication scopes.","status":"PERMISSION_DENIED","errors":[{"reason":"insufficientPermissions","message":"Insufficient Permission"}]}}`))
	}))
	defer server.Close()
	src, err := NewGoogleCalendarSource(config.SourceConfig{
		ID:        "work-calendar",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "google_calendar",
		AccountID: "work",
		Google: config.GoogleConfig{
			AccessToken: "token-without-calendar-events",
			APIBaseURL:  server.URL + "/calendar/v3",
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	start := time.Date(2026, 6, 2, 16, 0, 0, 0, time.UTC)
	_, err = src.CreateEvent(context.Background(), models.CalendarEvent{
		Ref:         models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "missing-scope@example.test"}.WithDefaults(),
		ProviderUID: "missing-scope@example.test",
		Title:       "Missing scope",
		Start:       start,
		End:         start.Add(30 * time.Minute),
		Status:      "confirmed",
	}, models.CalendarMutationOptions{})
	if !errors.Is(err, models.ErrCalendarWritePermission) {
		t.Fatalf("error = %v, want ErrCalendarWritePermission", err)
	}
	if strings.Contains(err.Error(), "{") || strings.Contains(err.Error(), "insufficientPermissions") {
		t.Fatalf("error leaked raw Google response: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "reconnect") {
		t.Fatalf("error = %v, want reconnect guidance", err)
	}
}

func TestGoogleCalendarSourceRespondToEventWritesAttendeeRSVP(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:       "rsvp-evt",
			Summary:  "Provider RSVP",
			Start:    start,
			End:      start.Add(time.Hour),
			TimeZone: "UTC",
			ETag:     `"g-v1"`,
			Attendees: []testcalendar.Attendee{
				{Name: "Rae Stone", Email: "rae@example.com", ResponseStatus: "tentative"},
			},
		}),
	)
	cfg := lab.GoogleSourceConfig("work-calendar", "work")
	cfg.Google.Email = "rae@example.com"
	src, err := NewGoogleCalendarSource(cfg)
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	events, err := src.ListEvents(context.Background(), models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	saved, err := src.RespondToEvent(context.Background(), events[0].Ref, "accepted", models.CalendarMutationOptions{
		RecurrenceScope: models.CalendarMutationScopeThisEvent,
		IfMatch:         events[0].Ref.ETag,
	})
	if err != nil {
		t.Fatalf("RespondToEvent: %v", err)
	}
	if len(saved.Attendees) != 1 || saved.Attendees[0].RSVP != "accepted" {
		t.Fatalf("attendees = %#v, want accepted RSVP", saved.Attendees)
	}
}

func TestGoogleCalendarSourceUpdateEventConflictIsTyped(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:       "conflict-evt",
			Summary:  "Provider conflict",
			Start:    start,
			End:      start.Add(time.Hour),
			TimeZone: "UTC",
			ETag:     `"g-v1"`,
		}),
	)
	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	events, err := src.ListEvents(context.Background(), models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	edited := events[0]
	edited.Title = "Should conflict"

	_, err = src.UpdateEvent(context.Background(), edited, models.CalendarMutationOptions{
		RecurrenceScope: models.CalendarMutationScopeThisEvent,
		IfMatch:         `"stale"`,
	})
	if !errors.Is(err, models.ErrCalendarMutationConflict) {
		t.Fatalf("error = %v, want ErrCalendarMutationConflict", err)
	}
	if strings.Contains(err.Error(), "/calendar/v3/") || strings.Contains(strings.ToLower(err.Error()), "etag") {
		t.Fatalf("error leaked provider internals: %v", err)
	}
	fetched, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if fetched.Title != events[0].Title {
		t.Fatalf("provider event title = %q, want unchanged %q", fetched.Title, events[0].Title)
	}
}

func TestGoogleCalendarSourceRejectsUnsupportedRecurrenceScopeBeforeProviderWrite(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("primary", "Work", "#3367d6"),
		testcalendar.WithEvent("primary", testcalendar.Event{
			ID:         "recurrence-evt",
			Summary:    "Provider recurrence",
			Start:      start,
			End:        start.Add(time.Hour),
			TimeZone:   "UTC",
			ETag:       `"g-v1"`,
			Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
		}),
	)
	src, err := NewGoogleCalendarSource(lab.GoogleSourceConfig("work-calendar", "work"))
	if err != nil {
		t.Fatalf("NewGoogleCalendarSource: %v", err)
	}
	events, err := src.ListEvents(context.Background(), models.CollectionRef{SourceID: "work-calendar", AccountID: "work", Kind: models.SourceKindCalendar, CollectionID: "primary"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	edited := events[0]
	edited.Title = "Unsupported broad edit"

	_, err = src.UpdateEvent(context.Background(), edited, models.CalendarMutationOptions{
		RecurrenceScope: models.CalendarMutationScopeAllEvents,
		IfMatch:         edited.Ref.ETag,
	})
	if !errors.Is(err, models.ErrCalendarRecurrenceScopeUnsupported) {
		t.Fatalf("error = %v, want ErrCalendarRecurrenceScopeUnsupported", err)
	}
	fetched, err := src.FetchEvent(context.Background(), edited.Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if fetched.Title != events[0].Title {
		t.Fatalf("provider event title = %q, want unchanged %q", fetched.Title, events[0].Title)
	}
}

func TestCalDAVSourceUsesLocalTestServer(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("team", "Team Calendar", "#0b8043"),
		testcalendar.WithEvent("team", testcalendar.Event{
			ID:          "team-standup.ics",
			UID:         "team-standup",
			Summary:     "Phase 6 CalDAV proof",
			Description: "Fetched through local CalDAV harness",
			Location:    "Terminal",
			Start:       start,
			End:         start.Add(30 * time.Minute),
			ETag:        `"c-v1"`,
			Updated:     start.Add(-time.Hour),
			Status:      "CONFIRMED",
		}),
	)

	src, err := NewCalDAVSource(lab.CalDAVSourceConfig("family-caldav", "family"))
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}

	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 1 || collections[0].Ref.CollectionID != "team" || collections[0].Ref.DisplayName != "Team Calendar" {
		t.Fatalf("collections = %#v, want local team CalDAV collection", collections)
	}

	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Phase 6 CalDAV proof" || events[0].ProviderUID != "team-standup" {
		t.Fatalf("events = %#v, want parsed local CalDAV event", events)
	}

	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.Ref.LocalID != events[0].Ref.LocalID || got.Ref.ETag != `"c-v1"` {
		t.Fatalf("FetchEvent = %#v, want same scoped event with etag", got)
	}
}

func TestCalDAVSourceDiscoversCalendarHomeSetAndCollectionSyncToken(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Method != "PROPFIND" {
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/</d:href><d:propstat><d:prop><d:current-user-principal><d:href>/principals/rae/</d:href></d:current-user-principal></d:prop></d:propstat></d:response></d:multistatus>`))
		case "/principals/rae/":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/principals/rae/</d:href><d:propstat><d:prop><cal:calendar-home-set><d:href>/caldav/rae/</d:href></cal:calendar-home-set></d:prop></d:propstat></d:response></d:multistatus>`))
		case "/caldav/rae/":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/"><d:response><d:href>/caldav/rae/</d:href><d:propstat><d:prop><d:displayname>Calendar Home</d:displayname></d:prop></d:propstat></d:response><d:response><d:href>/caldav/rae/team/</d:href><d:propstat><d:prop><d:displayname>Team</d:displayname><d:resourcetype><d:collection/><cal:calendar/></d:resourcetype><cs:calendar-color>#0b8043</cs:calendar-color><d:sync-token>sync-team-v1</d:sync-token></d:prop></d:propstat></d:response></d:multistatus>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	src, err := NewCalDAVSource(config.SourceConfig{
		ID:        "family-caldav",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "caldav",
		AccountID: "family",
		CalDAV: config.CalDAVConfig{
			URL:      server.URL + "/",
			Username: "rae@example.com",
			Password: "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("collections = %#v, want discovered team calendar only", collections)
	}
	if got := collections[0]; got.Ref.CollectionID != "team" || got.Ref.DisplayName != "Team" || got.Color != "#0b8043" || got.SyncToken != "sync-team-v1" {
		t.Fatalf("collection = %#v, want discovered scoped calendar with sync token", got)
	}
	joined := strings.Join(seen, ",")
	for _, want := range []string{"PROPFIND /", "PROPFIND /principals/rae/", "PROPFIND /caldav/rae/"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("seen requests = %v, missing %q", seen, want)
		}
	}
}

func TestCalDAVSourceSkipsCollectionsThatDoNotSupportEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/</d:href><d:propstat><d:prop><d:current-user-principal><d:href>/principals/rae/</d:href></d:current-user-principal></d:prop></d:propstat></d:response></d:multistatus>`))
		case "/principals/rae/":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/principals/rae/</d:href><d:propstat><d:prop><cal:calendar-home-set><d:href>/caldav/rae/</d:href></cal:calendar-home-set></d:prop></d:propstat></d:response></d:multistatus>`))
		case "/caldav/rae/":
			_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/"><d:response><d:href>/caldav/rae/team/</d:href><d:propstat><d:prop><d:displayname>Team</d:displayname><d:resourcetype><d:collection/><cal:calendar/></d:resourcetype><cal:supported-calendar-component-set><cal:comp name="VEVENT"/></cal:supported-calendar-component-set><cs:calendar-color>#0b8043</cs:calendar-color></d:prop></d:propstat></d:response><d:response><d:href>/caldav/rae/reminders/</d:href><d:propstat><d:prop><d:displayname>Reminders ⚠</d:displayname><d:resourcetype><d:collection/><cal:calendar/></d:resourcetype><cal:supported-calendar-component-set><cal:comp name="VTODO"/></cal:supported-calendar-component-set><cs:calendar-color>#ffcc00</cs:calendar-color></d:prop></d:propstat></d:response><d:response><d:href>/caldav/rae/legacy/</d:href><d:propstat><d:prop><d:displayname>Legacy</d:displayname><d:resourcetype><d:collection/><cal:calendar/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	src, err := NewCalDAVSource(config.SourceConfig{
		ID:        "icloud-calendar",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "caldav",
		AccountID: "icloud",
		CalDAV: config.CalDAVConfig{
			URL:      server.URL + "/",
			Username: "rae@example.com",
			Password: "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}

	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 2 {
		t.Fatalf("collections = %#v, want only VEVENT-capable and legacy calendars", collections)
	}
	if got := collections[0].Ref.DisplayName; got != "Team" {
		t.Fatalf("first collection = %q, want Team", got)
	}
	if got := collections[1].Ref.DisplayName; got != "Legacy" {
		t.Fatalf("second collection = %q, want Legacy", got)
	}
	for _, collection := range collections {
		if strings.Contains(collection.Ref.DisplayName, "Reminders") {
			t.Fatalf("collections include non-event reminder artifact: %#v", collections)
		}
	}
}

func TestCalDAVSourceICloudRedirectPreservesBasicAuth(t *testing.T) {
	var redirectedRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			t.Fatalf("method = %q, want PROPFIND", r.Method)
		}
		if r.Header.Get("Depth") == "" {
			t.Fatal("redirected CalDAV request dropped Depth header")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll body: %v", err)
		}
		if !strings.Contains(string(body), "<d:propfind") {
			t.Fatalf("redirected body = %q, want propfind XML", string(body))
		}
		switch r.Host {
		case "caldav.icloud.com":
			if r.Header.Get("Authorization") == "" {
				t.Fatal("initial iCloud CalDAV request missing Basic Auth")
			}
			w.Header().Set("Location", "https://p123-caldav.icloud.com/caldav/user/")
			w.WriteHeader(http.StatusFound)
		case "p123-caldav.icloud.com":
			redirectedRequests++
			if got := r.Header.Get("Authorization"); got == "" {
				t.Fatal("trusted iCloud redirect dropped Basic Auth")
			}
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/caldav/user/work/</d:href><d:propstat><d:prop><d:displayname>Work</d:displayname><d:resourcetype><d:collection/><cal:calendar/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`))
		default:
			t.Fatalf("unexpected host %q", r.Host)
		}
	}))
	defer server.Close()

	src, err := NewCalDAVSource(config.SourceConfig{
		ID:        "icloud-calendar",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "caldav",
		AccountID: "icloud",
		CalDAV: config.CalDAVConfig{
			URL:      "https://caldav.icloud.com/",
			Username: "me@icloud.com",
			Password: "app-specific-password",
		},
	})
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	src.client = &http.Client{Transport: rewriteHostTransportForTest(t, server.URL)}

	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if len(collections) != 1 || collections[0].Ref.CollectionID != "work" {
		t.Fatalf("collections = %#v, want redirected iCloud calendar", collections)
	}
	if redirectedRequests == 0 {
		t.Fatal("expected at least one trusted iCloud redirect request")
	}
}

func TestCalDAVSourceDoesNotForwardBasicAuthToUntrustedRedirect(t *testing.T) {
	var untrustedAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			t.Fatalf("method = %q, want PROPFIND", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll body: %v", err)
		}
		if !strings.Contains(string(body), "<d:propfind") {
			t.Fatalf("redirected body = %q, want propfind XML", string(body))
		}
		switch r.Host {
		case "caldav.icloud.com":
			if r.Header.Get("Authorization") == "" {
				t.Fatal("initial iCloud CalDAV request missing Basic Auth")
			}
			w.Header().Set("Location", "https://calendar.attacker.test/caldav/")
			w.WriteHeader(http.StatusFound)
		case "calendar.attacker.test":
			untrustedAuthorization = r.Header.Get("Authorization")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		default:
			t.Fatalf("unexpected host %q", r.Host)
		}
	}))
	defer server.Close()

	src, err := NewCalDAVSource(config.SourceConfig{
		ID:        "icloud-calendar",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "caldav",
		AccountID: "icloud",
		CalDAV: config.CalDAVConfig{
			URL:      "https://caldav.icloud.com/",
			Username: "me@icloud.com",
			Password: "app-specific-password",
		},
	})
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	src.client = &http.Client{Transport: rewriteHostTransportForTest(t, server.URL)}

	if _, err := src.ListCalendars(context.Background()); err == nil {
		t.Fatal("ListCalendars succeeded after untrusted redirect, want auth failure")
	}
	if untrustedAuthorization != "" {
		t.Fatalf("untrusted redirect Authorization = %q, want empty", untrustedAuthorization)
	}
}

func TestCalDAVDebugHelpersDoNotExposeSensitivePathOrAuthValues(t *testing.T) {
	if got := caldavDebugPathKind("/123456789/principal/"); got != "principal" {
		t.Fatalf("path kind = %q, want principal", got)
	}
	if got := caldavDebugPathKind("/123456789/calendars/private-calendar/event.ics"); got != "calendars" {
		t.Fatalf("path kind = %q, want calendars", got)
	}
	header := http.Header{}
	header.Add("WWW-Authenticate", `x-mobileme-authtoken realm="MMCalDav"`)
	header.Add("WWW-Authenticate", `Basic realm="MMCalDav"`)
	if got := caldavAuthChallengeSchemes(header); got != "basic,x-mobileme-authtoken" {
		t.Fatalf("auth challenge schemes = %q, want sanitized schemes", got)
	}
	base, err := url.Parse("https://caldav.icloud.com/")
	if err != nil {
		t.Fatal(err)
	}
	if got := caldavRedirectHost(base, "https://p123-caldav.icloud.com/private/path"); got != "p123-caldav.icloud.com" {
		t.Fatalf("redirect host = %q, want host only", got)
	}
}

func rewriteHostTransportForTest(t *testing.T, target string) http.RoundTripper {
	t.Helper()
	targetURL, err := url.Parse(target)
	if err != nil {
		t.Fatalf("Parse target URL: %v", err)
	}
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.Host = req.URL.Host
		clone.URL.Scheme = targetURL.Scheme
		clone.URL.Host = targetURL.Host
		return http.DefaultTransport.RoundTrip(clone)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestCalDAVSourceListEventsWithSyncTokenUsesSyncCollection(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	var reportBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" || r.URL.Path != "/caldav/team/" {
			http.NotFound(w, r)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		reportBody = string(data)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav"><d:sync-token>sync-team-v2</d:sync-token><d:response><d:href>/caldav/team/updated.ics</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:getetag>"updated-v2"</d:getetag><cal:calendar-data>` + xmlEscapeForTest(testCalDAVICS("updated", "Updated planning", start)) + `</cal:calendar-data></d:prop></d:propstat></d:response><d:response><d:href>/caldav/team/deleted.ics</d:href><d:status>HTTP/1.1 404 Not Found</d:status></d:response></d:multistatus>`))
	}))
	defer server.Close()

	src, err := NewCalDAVSource(config.SourceConfig{
		ID:        "family-caldav",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "caldav",
		AccountID: "family",
		CalDAV: config.CalDAVConfig{
			URL:      server.URL + "/caldav/",
			Username: "local",
			Password: "password",
		},
	})
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	result, err := src.ListEventsWithSyncToken(context.Background(), models.CollectionRef{
		SourceID:     "family-caldav",
		AccountID:    "family",
		Kind:         models.SourceKindCalendar,
		CollectionID: "team",
	}, "sync-team-v1")
	if err != nil {
		t.Fatalf("ListEventsWithSyncToken: %v", err)
	}
	if !strings.Contains(reportBody, "sync-collection") || !strings.Contains(reportBody, "sync-team-v1") {
		t.Fatalf("sync REPORT body = %q, want sync-collection with cached token", reportBody)
	}
	if result.NextSyncToken != "sync-team-v2" {
		t.Fatalf("NextSyncToken = %q, want sync-team-v2", result.NextSyncToken)
	}
	if len(result.Events) != 1 || result.Events[0].Ref.EventID != "updated.ics" || result.Events[0].Title != "Updated planning" {
		t.Fatalf("Events = %#v, want updated event", result.Events)
	}
	if len(result.DeletedRefs) != 1 || result.DeletedRefs[0].EventID != "deleted.ics" {
		t.Fatalf("DeletedRefs = %#v, want deleted.ics", result.DeletedRefs)
	}
}

func TestCalDAVSourceListEventsWithSyncTokenFallsBackToCalendarQueryWhenUnsupported(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	var reportBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" || r.URL.Path != "/caldav/team/" {
			http.NotFound(w, r)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		body := string(data)
		reportBodies = append(reportBodies, body)
		if strings.Contains(body, "sync-collection") {
			http.Error(w, "sync-collection unsupported", http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/caldav/team/full.ics</d:href><d:propstat><d:prop><d:getetag>"full-v1"</d:getetag><cal:calendar-data>` + xmlEscapeForTest(testCalDAVICS("full", "Full polling event", start)) + `</cal:calendar-data></d:prop></d:propstat></d:response></d:multistatus>`))
	}))
	defer server.Close()

	src, err := NewCalDAVSource(config.SourceConfig{
		ID:        "family-caldav",
		Kind:      string(models.SourceKindCalendar),
		Provider:  "caldav",
		AccountID: "family",
		CalDAV: config.CalDAVConfig{
			URL:      server.URL + "/caldav/",
			Username: "local",
			Password: "password",
		},
	})
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	result, err := src.ListEventsWithSyncToken(context.Background(), models.CollectionRef{
		SourceID:     "family-caldav",
		AccountID:    "family",
		Kind:         models.SourceKindCalendar,
		CollectionID: "team",
	}, "sync-team-v1")
	if err != nil {
		t.Fatalf("ListEventsWithSyncToken fallback: %v", err)
	}
	if len(reportBodies) != 2 || !strings.Contains(reportBodies[0], "sync-collection") || !strings.Contains(reportBodies[1], "calendar-query") {
		t.Fatalf("report bodies = %#v, want sync attempt then calendar-query fallback", reportBodies)
	}
	if result.NextSyncToken != "" {
		t.Fatalf("NextSyncToken = %q, want empty token for polling fallback", result.NextSyncToken)
	}
	if len(result.Events) != 1 || result.Events[0].Title != "Full polling event" {
		t.Fatalf("Events = %#v, want polling fallback event", result.Events)
	}
}

func TestCalDAVSourceParsesRichEventDetail(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("team", "Team Calendar", "#0b8043"),
		testcalendar.WithEvent("team", testcalendar.Event{
			ID:          "rich-evt.ics",
			UID:         "rich-evt",
			Summary:     "Timezone planning",
			Description: "Review attendee status before editing is enabled.",
			Location:    "Video call",
			Start:       start,
			End:         start.Add(time.Hour),
			TimeZone:    "America/Los_Angeles",
			Status:      "CONFIRMED",
			Organizer: testcalendar.Person{
				Name:  "Mina Park",
				Email: "mina@example.com",
			},
			Attendees: []testcalendar.Attendee{
				{Name: "Rae Stone", Email: "rae@example.com", ResponseStatus: "ACCEPTED"},
				{Name: "Noor Patel", Email: "noor@example.com", ResponseStatus: "TENTATIVE", Optional: true},
			},
			Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
			Attachments: []testcalendar.Attachment{
				{Title: "Agenda", FileURL: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
			},
			Reminders: []testcalendar.Reminder{
				{Method: "popup", MinutesBefore: 10},
				{Method: "email", MinutesBefore: 60},
			},
		}),
	)

	src, err := NewCalDAVSource(lab.CalDAVSourceConfig("family-caldav", "family"))
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	collections, err := src.ListCalendars(context.Background())
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	events, err := src.ListEvents(context.Background(), collections[0].Ref)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	got, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if got.TimeZone != "America/Los_Angeles" || got.Organizer != "Mina Park" || got.OrganizerEmail != "mina@example.com" {
		t.Fatalf("rich organizer/timezone = %#v", got)
	}
	if len(got.Attendees) != 2 || got.Attendees[0].RSVP != "accepted" || !got.Attendees[1].Optional {
		t.Fatalf("attendees = %#v", got.Attendees)
	}
	if got.RecurrenceSummary != "Weekly on Monday" || len(got.Recurrence) != 1 {
		t.Fatalf("recurrence = %#v summary=%q", got.Recurrence, got.RecurrenceSummary)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Title != "Agenda" || got.Attachments[0].MIMEType != "application/pdf" {
		t.Fatalf("attachments = %#v", got.Attachments)
	}
	if len(got.Reminders) != 2 || got.Reminders[0].Method != "popup" || got.Reminders[0].MinutesBefore != 10 || got.Reminders[1].Method != "email" || got.Reminders[1].MinutesBefore != 60 {
		t.Fatalf("reminders = %#v", got.Reminders)
	}
}

func TestCalDAVSourceUpdateEventWritesThroughProvider(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("team", "Team Calendar", "#0b8043"),
		testcalendar.WithEvent("team", testcalendar.Event{
			ID:          "update-evt.ics",
			UID:         "update-evt",
			Summary:     "CalDAV planning",
			Description: "Original notes",
			Location:    "Room A",
			Start:       start,
			End:         start.Add(time.Hour),
			TimeZone:    "UTC",
			ETag:        `"c-v1"`,
			Status:      "CONFIRMED",
		}),
	)
	src, err := NewCalDAVSource(lab.CalDAVSourceConfig("family-caldav", "family"))
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	events, err := src.ListEvents(context.Background(), models.CollectionRef{SourceID: "family-caldav", AccountID: "family", Kind: models.SourceKindCalendar, CollectionID: "team"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	edited := events[0]
	edited.Title = "CalDAV planning moved"
	edited.Description = "Updated through PUT"
	edited.Location = "Room B"
	edited.Start = start.Add(2 * time.Hour)
	edited.End = start.Add(3 * time.Hour)
	edited.TimeZone = "America/Los_Angeles"
	edited.Reminders = []models.CalendarReminder{{Method: "popup", MinutesBefore: 15}}

	saved, err := src.UpdateEvent(context.Background(), edited, models.CalendarMutationOptions{
		RecurrenceScope: models.CalendarMutationScopeThisEvent,
		IfMatch:         edited.Ref.ETag,
	})
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if saved.Title != edited.Title || saved.Location != edited.Location || saved.TimeZone != edited.TimeZone {
		t.Fatalf("saved = %#v, want edited fields", saved)
	}
	if len(saved.Reminders) != 1 || saved.Reminders[0].Method != "popup" || saved.Reminders[0].MinutesBefore != 15 {
		t.Fatalf("saved reminders = %#v, want edited reminder override", saved.Reminders)
	}
	if saved.Ref.ETag == edited.Ref.ETag {
		t.Fatalf("saved etag = %q, want provider freshness to advance", saved.Ref.ETag)
	}
}

func TestCalDAVSourceRespondToEventWritesAttendeeRSVP(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("team", "Team Calendar", "#0b8043"),
		testcalendar.WithEvent("team", testcalendar.Event{
			ID:       "rsvp-evt.ics",
			UID:      "rsvp-evt",
			Summary:  "CalDAV RSVP",
			Start:    start,
			End:      start.Add(time.Hour),
			TimeZone: "UTC",
			ETag:     `"c-v1"`,
			Attendees: []testcalendar.Attendee{
				{Name: "Rae Stone", Email: "rae@example.com", ResponseStatus: "TENTATIVE"},
			},
		}),
	)
	cfg := lab.CalDAVSourceConfig("family-caldav", "family")
	cfg.CalDAV.Username = "rae@example.com"
	src, err := NewCalDAVSource(cfg)
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	events, err := src.ListEvents(context.Background(), models.CollectionRef{SourceID: "family-caldav", AccountID: "family", Kind: models.SourceKindCalendar, CollectionID: "team"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	saved, err := src.RespondToEvent(context.Background(), events[0].Ref, "accepted", models.CalendarMutationOptions{
		RecurrenceScope: models.CalendarMutationScopeThisEvent,
		IfMatch:         events[0].Ref.ETag,
	})
	if err != nil {
		t.Fatalf("RespondToEvent: %v", err)
	}
	if len(saved.Attendees) != 1 || saved.Attendees[0].RSVP != "accepted" {
		t.Fatalf("attendees = %#v, want accepted RSVP", saved.Attendees)
	}
}

func TestCalDAVSourceUpdateEventConflictIsTyped(t *testing.T) {
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, time.UTC)
	lab := testcalendar.Start(t,
		testcalendar.WithCalendar("team", "Team Calendar", "#0b8043"),
		testcalendar.WithEvent("team", testcalendar.Event{
			ID:       "conflict-evt.ics",
			UID:      "conflict-evt",
			Summary:  "CalDAV conflict",
			Start:    start,
			End:      start.Add(time.Hour),
			TimeZone: "UTC",
			ETag:     `"c-v1"`,
		}),
	)
	src, err := NewCalDAVSource(lab.CalDAVSourceConfig("family-caldav", "family"))
	if err != nil {
		t.Fatalf("NewCalDAVSource: %v", err)
	}
	events, err := src.ListEvents(context.Background(), models.CollectionRef{SourceID: "family-caldav", AccountID: "family", Kind: models.SourceKindCalendar, CollectionID: "team"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	edited := events[0]
	edited.Title = "Should conflict"

	_, err = src.UpdateEvent(context.Background(), edited, models.CalendarMutationOptions{
		RecurrenceScope: models.CalendarMutationScopeThisEvent,
		IfMatch:         `"stale"`,
	})
	if !errors.Is(err, models.ErrCalendarMutationConflict) {
		t.Fatalf("error = %v, want ErrCalendarMutationConflict", err)
	}
	if strings.Contains(err.Error(), "/caldav/") || strings.Contains(strings.ToLower(err.Error()), "etag") {
		t.Fatalf("error leaked provider internals: %v", err)
	}
	fetched, err := src.FetchEvent(context.Background(), events[0].Ref)
	if err != nil {
		t.Fatalf("FetchEvent: %v", err)
	}
	if fetched.Title != events[0].Title {
		t.Fatalf("provider event title = %q, want unchanged %q", fetched.Title, events[0].Title)
	}
}
