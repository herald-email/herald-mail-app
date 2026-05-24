package testcalendar

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type Event struct {
	ID          string
	UID         string
	Summary     string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
	AllDay      bool
	ETag        string
	Updated     time.Time
	Status      string
}

type Option func(*options)

type options struct {
	calendars []calendarSpec
	events    []eventSpec
}

type calendarSpec struct {
	ID          string
	DisplayName string
	Color       string
}

type eventSpec struct {
	CalendarID string
	Event      Event
}

type Lab struct {
	t testing.TB

	mu        sync.RWMutex
	calendars map[string]calendarSpec
	events    map[string][]Event

	google *httptest.Server
	caldav *httptest.Server
}

func WithCalendar(id, displayName, color string) Option {
	return func(o *options) {
		o.calendars = append(o.calendars, calendarSpec{ID: id, DisplayName: displayName, Color: color})
	}
}

func WithEvent(calendarID string, event Event) Option {
	return func(o *options) {
		o.events = append(o.events, eventSpec{CalendarID: calendarID, Event: event})
	}
}

func Start(t testing.TB, opts ...Option) *Lab {
	t.Helper()

	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if len(cfg.calendars) == 0 {
		cfg.calendars = []calendarSpec{{ID: "primary", DisplayName: "Primary", Color: "#3367d6"}}
	}

	lab := &Lab{
		t:         t,
		calendars: make(map[string]calendarSpec),
		events:    make(map[string][]Event),
	}
	for _, cal := range cfg.calendars {
		lab.calendars[cal.ID] = cal
	}
	for _, evt := range cfg.events {
		if _, ok := lab.calendars[evt.CalendarID]; !ok {
			t.Fatalf("testcalendar: event calendar %q not found", evt.CalendarID)
		}
		event := evt.Event
		if event.ID == "" {
			event.ID = strings.TrimSpace(event.UID)
		}
		if event.UID == "" {
			event.UID = strings.TrimSuffix(event.ID, ".ics")
		}
		if event.ETag == "" {
			event.ETag = fmt.Sprintf(`"%s-v1"`, event.ID)
		}
		if event.Status == "" {
			event.Status = "confirmed"
		}
		lab.events[evt.CalendarID] = append(lab.events[evt.CalendarID], event)
	}

	lab.google = httptest.NewServer(http.HandlerFunc(lab.handleGoogle))
	lab.caldav = httptest.NewServer(http.HandlerFunc(lab.handleCalDAV))
	t.Cleanup(lab.Close)
	return lab
}

func (l *Lab) Close() {
	if l == nil {
		return
	}
	if l.google != nil {
		l.google.Close()
	}
	if l.caldav != nil {
		l.caldav.Close()
	}
}

func (l *Lab) GoogleSourceConfig(sourceID, accountID string) config.SourceConfig {
	return config.SourceConfig{
		ID:          sourceID,
		Kind:        string(models.SourceKindCalendar),
		Provider:    "google_calendar",
		DisplayName: sourceID,
		AccountID:   accountID,
		Google: config.GoogleConfig{
			AccessToken: "local-token",
			APIBaseURL:  l.google.URL + "/calendar/v3",
		},
	}
}

func (l *Lab) CalDAVSourceConfig(sourceID, accountID string) config.SourceConfig {
	return config.SourceConfig{
		ID:          sourceID,
		Kind:        string(models.SourceKindCalendar),
		Provider:    "caldav",
		DisplayName: sourceID,
		AccountID:   accountID,
		CalDAV: config.CalDAVConfig{
			URL:      l.caldav.URL + "/caldav/",
			Username: "local",
			Password: "password",
		},
	}
}

func (l *Lab) handleGoogle(w http.ResponseWriter, r *http.Request) {
	if auth := r.Header.Get("Authorization"); auth == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	p := strings.TrimPrefix(r.URL.Path, "/calendar/v3/")
	if p == "users/me/calendarList" {
		l.writeGoogleCalendarList(w)
		return
	}
	if strings.HasPrefix(p, "calendars/") {
		l.handleGoogleCalendarEvents(w, r, strings.TrimPrefix(p, "calendars/"))
		return
	}
	http.NotFound(w, r)
}

func (l *Lab) writeGoogleCalendarList(w http.ResponseWriter) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	items := make([]map[string]string, 0, len(l.calendars))
	for _, cal := range l.calendars {
		items = append(items, map[string]string{
			"id":              cal.ID,
			"summary":         cal.DisplayName,
			"backgroundColor": cal.Color,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (l *Lab) handleGoogleCalendarEvents(w http.ResponseWriter, r *http.Request, tail string) {
	parts := strings.Split(tail, "/")
	if len(parts) < 2 || parts[1] != "events" {
		http.NotFound(w, r)
		return
	}
	calendarID, err := url.PathUnescape(parts[0])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(parts) == 2 {
		l.writeGoogleEvents(w, calendarID)
		return
	}
	eventID, err := url.PathUnescape(parts[2])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	event, ok := l.findEvent(calendarID, eventID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(googleEvent(event))
}

func (l *Lab) writeGoogleEvents(w http.ResponseWriter, calendarID string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	events := l.events[calendarID]
	items := make([]map[string]any, 0, len(events))
	for _, event := range events {
		items = append(items, googleEvent(event))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "nextSyncToken": "sync-" + calendarID})
}

func googleEvent(event Event) map[string]any {
	startKey := "dateTime"
	endKey := "dateTime"
	startValue := event.Start.Format(time.RFC3339)
	endValue := event.End.Format(time.RFC3339)
	if event.AllDay {
		startKey = "date"
		endKey = "date"
		startValue = event.Start.Format("2006-01-02")
		endValue = event.End.Format("2006-01-02")
	}
	return map[string]any{
		"id":          event.ID,
		"iCalUID":     event.UID,
		"etag":        event.ETag,
		"summary":     event.Summary,
		"description": event.Description,
		"location":    event.Location,
		"status":      event.Status,
		"updated":     event.Updated.Format(time.RFC3339),
		"start":       map[string]string{startKey: startValue},
		"end":         map[string]string{endKey: endValue},
	}
}

func (l *Lab) handleCalDAV(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "PROPFIND":
		l.writeCalDAVCalendars(w)
	case "REPORT":
		calendarID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/caldav/"), "/")
		l.writeCalDAVEvents(w, calendarID)
	case "GET":
		calendarID, eventID := splitCalDAVEventPath(r.URL.Path)
		event, ok := l.findEvent(calendarID, eventID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("ETag", event.ETag)
		_, _ = w.Write([]byte(ics(event)))
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func (l *Lab) writeCalDAVCalendars(w http.ResponseWriter) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/">`)
	for _, cal := range l.calendars {
		href := "/caldav/" + path.Clean(cal.ID) + "/"
		b.WriteString(`<d:response><d:href>`)
		b.WriteString(html.EscapeString(href))
		b.WriteString(`</d:href><d:propstat><d:prop><d:displayname>`)
		b.WriteString(html.EscapeString(cal.DisplayName))
		b.WriteString(`</d:displayname><cs:calendar-color>`)
		b.WriteString(html.EscapeString(cal.Color))
		b.WriteString(`</cs:calendar-color></d:prop></d:propstat></d:response>`)
	}
	b.WriteString(`</d:multistatus>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(b.String()))
}

func (l *Lab) writeCalDAVEvents(w http.ResponseWriter, calendarID string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav">`)
	for _, event := range l.events[calendarID] {
		href := "/caldav/" + path.Clean(calendarID) + "/" + path.Clean(event.ID)
		b.WriteString(`<d:response><d:href>`)
		b.WriteString(html.EscapeString(href))
		b.WriteString(`</d:href><d:propstat><d:prop><d:getetag>`)
		b.WriteString(html.EscapeString(event.ETag))
		b.WriteString(`</d:getetag><cal:calendar-data>`)
		b.WriteString(html.EscapeString(ics(event)))
		b.WriteString(`</cal:calendar-data></d:prop></d:propstat></d:response>`)
	}
	b.WriteString(`</d:multistatus>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(b.String()))
}

func (l *Lab) findEvent(calendarID, eventID string) (Event, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, event := range l.events[calendarID] {
		if event.ID == eventID || strings.TrimSuffix(event.ID, ".ics") == eventID || event.UID == eventID {
			return event, true
		}
	}
	return Event{}, false
}

func splitCalDAVEventPath(p string) (string, string) {
	tail := strings.Trim(strings.TrimPrefix(p, "/caldav/"), "/")
	parts := strings.Split(tail, "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], parts[len(parts)-1]
}

func ics(event Event) string {
	uid := event.UID
	if uid == "" {
		uid = strings.TrimSuffix(event.ID, ".ics")
	}
	status := strings.ToUpper(event.Status)
	if status == "" {
		status = "CONFIRMED"
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Herald//Test Calendar//EN\r\nBEGIN:VEVENT\r\n")
	writeICSLine(&b, "UID", uid)
	writeICSLine(&b, "SUMMARY", event.Summary)
	writeICSLine(&b, "DESCRIPTION", event.Description)
	writeICSLine(&b, "LOCATION", event.Location)
	writeICSLine(&b, "STATUS", status)
	if event.AllDay {
		writeICSLine(&b, "DTSTART;VALUE=DATE", event.Start.Format("20060102"))
		writeICSLine(&b, "DTEND;VALUE=DATE", event.End.Format("20060102"))
	} else {
		writeICSLine(&b, "DTSTART", event.Start.UTC().Format("20060102T150405Z"))
		writeICSLine(&b, "DTEND", event.End.UTC().Format("20060102T150405Z"))
	}
	if !event.Updated.IsZero() {
		writeICSLine(&b, "LAST-MODIFIED", event.Updated.UTC().Format("20060102T150405Z"))
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

func writeICSLine(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	b.WriteString(key)
	b.WriteByte(':')
	b.WriteString(strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "", ",", "\\,", ";", "\\;").Replace(value))
	b.WriteString("\r\n")
}
