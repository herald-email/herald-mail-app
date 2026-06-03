package testcalendar

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
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
	TimeZone    string
	ETag        string
	Updated     time.Time
	Status      string
	Organizer   Person
	Attendees   []Attendee
	Recurrence  []string
	Attachments []Attachment
	Reminders   []Reminder
}

type Person struct {
	Name  string
	Email string
}

type Attendee struct {
	Name           string
	Email          string
	ResponseStatus string
	Optional       bool
}

type Attachment struct {
	Title    string
	FileURL  string
	MIMEType string
}

type Reminder struct {
	Method        string
	MinutesBefore int
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
			"accessRole":      "owner",
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
		if r.Method != http.MethodGet {
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
			return
		}
		l.writeGoogleEvents(w, calendarID, r.URL.Query().Get("iCalUID"))
		return
	}
	eventID, err := url.PathUnescape(parts[2])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if eventID == "import" {
		if r.Method != http.MethodPost {
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
			return
		}
		imported, err := l.importGoogleEvent(calendarID, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(googleEvent(imported))
		return
	}
	event, ok := l.findEvent(calendarID, eventID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(googleEvent(event))
	case http.MethodPatch:
		updated, err := l.updateGoogleEvent(calendarID, eventID, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusPreconditionFailed)
			return
		}
		_ = json.NewEncoder(w).Encode(googleEvent(updated))
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func (l *Lab) writeGoogleEvents(w http.ResponseWriter, calendarID, iCalUID string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	iCalUID = strings.TrimSpace(iCalUID)
	events := l.events[calendarID]
	items := make([]map[string]any, 0, len(events))
	for _, event := range events {
		if iCalUID != "" && event.UID != iCalUID {
			continue
		}
		items = append(items, googleEvent(event))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "nextSyncToken": "sync-" + calendarID})
}

func googleEvent(event Event) map[string]any {
	startKey := "dateTime"
	endKey := "dateTime"
	startValue := event.Start.Format(time.RFC3339)
	endValue := event.End.Format(time.RFC3339)
	startPayload := map[string]string{startKey: startValue}
	endPayload := map[string]string{endKey: endValue}
	if event.AllDay {
		startKey = "date"
		endKey = "date"
		startValue = event.Start.Format("2006-01-02")
		endValue = event.End.Format("2006-01-02")
		startPayload = map[string]string{startKey: startValue}
		endPayload = map[string]string{endKey: endValue}
	} else if event.TimeZone != "" {
		startPayload["timeZone"] = event.TimeZone
		endPayload["timeZone"] = event.TimeZone
	}
	payload := map[string]any{
		"id":          event.ID,
		"iCalUID":     event.UID,
		"etag":        event.ETag,
		"summary":     event.Summary,
		"description": event.Description,
		"location":    event.Location,
		"status":      event.Status,
		"updated":     event.Updated.Format(time.RFC3339),
		"start":       startPayload,
		"end":         endPayload,
	}
	if event.Organizer.Email != "" || event.Organizer.Name != "" {
		payload["organizer"] = map[string]string{
			"displayName": event.Organizer.Name,
			"email":       event.Organizer.Email,
		}
	}
	if len(event.Attendees) > 0 {
		attendees := make([]map[string]any, 0, len(event.Attendees))
		for _, attendee := range event.Attendees {
			attendees = append(attendees, map[string]any{
				"displayName":    attendee.Name,
				"email":          attendee.Email,
				"responseStatus": attendee.ResponseStatus,
				"optional":       attendee.Optional,
			})
		}
		payload["attendees"] = attendees
	}
	if len(event.Recurrence) > 0 {
		payload["recurrence"] = append([]string(nil), event.Recurrence...)
	}
	if len(event.Attachments) > 0 {
		attachments := make([]map[string]string, 0, len(event.Attachments))
		for _, attachment := range event.Attachments {
			attachments = append(attachments, map[string]string{
				"title":    attachment.Title,
				"fileUrl":  attachment.FileURL,
				"mimeType": attachment.MIMEType,
			})
		}
		payload["attachments"] = attachments
	}
	if len(event.Reminders) > 0 {
		overrides := make([]map[string]any, 0, len(event.Reminders))
		for _, reminder := range event.Reminders {
			overrides = append(overrides, map[string]any{
				"method":  reminder.Method,
				"minutes": reminder.MinutesBefore,
			})
		}
		payload["reminders"] = map[string]any{"useDefault": false, "overrides": overrides}
	}
	return payload
}

type googleEventPatch struct {
	ID          string           `json:"id"`
	ICalUID     string           `json:"iCalUID"`
	Summary     string           `json:"summary"`
	Description string           `json:"description"`
	Location    string           `json:"location"`
	Status      string           `json:"status"`
	Start       googlePatchTime  `json:"start"`
	End         googlePatchTime  `json:"end"`
	Attendees   []googleAttendee `json:"attendees"`
	Recurrence  []string         `json:"recurrence"`
	Reminders   *googleReminders `json:"reminders,omitempty"`
}

type googlePatchTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
	TimeZone string `json:"timeZone"`
}

type googleAttendee struct {
	DisplayName    string `json:"displayName"`
	Email          string `json:"email"`
	ResponseStatus string `json:"responseStatus"`
	Optional       bool   `json:"optional"`
}

type googleReminders struct {
	UseDefault bool             `json:"useDefault"`
	Overrides  []googleReminder `json:"overrides"`
}

type googleReminder struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

func applyGooglePatch(event Event, patch googleEventPatch) Event {
	event.Summary = patch.Summary
	event.Description = patch.Description
	event.Location = patch.Location
	event.Status = patch.Status
	if start, allDay, timezone := parseGooglePatchTime(patch.Start); !start.IsZero() {
		event.Start = start
		event.AllDay = allDay
		event.TimeZone = timezone
	}
	if end, _, timezone := parseGooglePatchTime(patch.End); !end.IsZero() {
		event.End = end
		if event.TimeZone == "" {
			event.TimeZone = timezone
		}
	}
	event.Attendees = event.Attendees[:0]
	for _, attendee := range patch.Attendees {
		event.Attendees = append(event.Attendees, Attendee{
			Name:           attendee.DisplayName,
			Email:          attendee.Email,
			ResponseStatus: attendee.ResponseStatus,
			Optional:       attendee.Optional,
		})
	}
	event.Recurrence = append([]string(nil), patch.Recurrence...)
	if patch.Reminders != nil {
		event.Reminders = event.Reminders[:0]
		for _, reminder := range patch.Reminders.Overrides {
			event.Reminders = append(event.Reminders, Reminder{
				Method:        reminder.Method,
				MinutesBefore: reminder.Minutes,
			})
		}
	}
	return event
}

func parseGooglePatchTime(value googlePatchTime) (time.Time, bool, string) {
	if strings.TrimSpace(value.Date) != "" {
		parsed, _ := time.Parse("2006-01-02", value.Date)
		return parsed, true, value.TimeZone
	}
	if strings.TrimSpace(value.DateTime) == "" {
		return time.Time{}, false, value.TimeZone
	}
	parsed, _ := time.Parse(time.RFC3339, value.DateTime)
	return parsed, false, value.TimeZone
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (l *Lab) importGoogleEvent(calendarID string, r *http.Request) (Event, error) {
	var payload googleEventPatch
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return Event{}, err
	}
	start, allDay, timezone := parseGooglePatchTime(payload.Start)
	end, _, endTimezone := parseGooglePatchTime(payload.End)
	if timezone == "" {
		timezone = endTimezone
	}
	event := Event{
		ID:          firstNonEmpty(payload.ID, payload.ICalUID),
		UID:         firstNonEmpty(payload.ICalUID, payload.ID),
		Summary:     payload.Summary,
		Description: payload.Description,
		Location:    payload.Location,
		Start:       start,
		End:         end,
		AllDay:      allDay,
		TimeZone:    timezone,
		Status:      firstNonEmpty(payload.Status, "confirmed"),
		Recurrence:  append([]string(nil), payload.Recurrence...),
		Updated:     time.Now().UTC(),
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("event-%d", event.Updated.UnixNano())
	}
	if event.UID == "" {
		event.UID = event.ID
	}
	for _, attendee := range payload.Attendees {
		event.Attendees = append(event.Attendees, Attendee{
			Name:           attendee.DisplayName,
			Email:          attendee.Email,
			ResponseStatus: attendee.ResponseStatus,
			Optional:       attendee.Optional,
		})
	}
	if payload.Reminders != nil {
		for _, reminder := range payload.Reminders.Overrides {
			event.Reminders = append(event.Reminders, Reminder{
				Method:        reminder.Method,
				MinutesBefore: reminder.Minutes,
			})
		}
	}
	event.ETag = nextETag(event)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events[calendarID] = append(l.events[calendarID], event)
	return event, nil
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
	case "PUT":
		calendarID, eventID := splitCalDAVEventPath(r.URL.Path)
		updated, err := l.updateCalDAVEvent(calendarID, eventID, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusPreconditionFailed)
			return
		}
		w.Header().Set("ETag", updated.ETag)
		w.WriteHeader(http.StatusNoContent)
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

func (l *Lab) updateGoogleEvent(calendarID, eventID string, r *http.Request) (Event, error) {
	var patch googleEventPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		return Event{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	events := l.events[calendarID]
	for i := range events {
		if events[i].ID != eventID && strings.TrimSuffix(events[i].ID, ".ics") != eventID && events[i].UID != eventID {
			continue
		}
		if ifMatch := strings.TrimSpace(r.Header.Get("If-Match")); ifMatch != "" && ifMatch != events[i].ETag {
			return Event{}, fmt.Errorf("etag mismatch")
		}
		events[i] = applyGooglePatch(events[i], patch)
		events[i].ETag = nextETag(events[i])
		events[i].Updated = time.Now().UTC()
		l.events[calendarID] = events
		return events[i], nil
	}
	return Event{}, fmt.Errorf("event not found")
}

func (l *Lab) updateCalDAVEvent(calendarID, eventID string, r *http.Request) (Event, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return Event{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	events := l.events[calendarID]
	for i := range events {
		if events[i].ID != eventID && strings.TrimSuffix(events[i].ID, ".ics") != eventID && events[i].UID != eventID {
			continue
		}
		if strings.TrimSpace(r.Header.Get("If-None-Match")) == "*" {
			return Event{}, fmt.Errorf("event already exists")
		}
		if ifMatch := strings.TrimSpace(r.Header.Get("If-Match")); ifMatch != "" && ifMatch != events[i].ETag {
			return Event{}, fmt.Errorf("etag mismatch")
		}
		events[i] = applyICSPatch(events[i], string(data))
		events[i].ETag = nextETag(events[i])
		events[i].Updated = time.Now().UTC()
		l.events[calendarID] = events
		return events[i], nil
	}
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == "*" {
		event := Event{ID: eventID, UID: strings.TrimSuffix(eventID, ".ics"), Status: "confirmed"}
		event = applyICSPatch(event, string(data))
		if event.UID == "" {
			event.UID = strings.TrimSuffix(eventID, ".ics")
		}
		event.ETag = nextETag(event)
		event.Updated = time.Now().UTC()
		l.events[calendarID] = append(l.events[calendarID], event)
		return event, nil
	}
	return Event{}, fmt.Errorf("event not found")
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
	} else if event.TimeZone != "" {
		writeICSLine(&b, "DTSTART;TZID="+event.TimeZone, event.Start.Format("20060102T150405"))
		writeICSLine(&b, "DTEND;TZID="+event.TimeZone, event.End.Format("20060102T150405"))
	} else {
		writeICSLine(&b, "DTSTART", event.Start.UTC().Format("20060102T150405Z"))
		writeICSLine(&b, "DTEND", event.End.UTC().Format("20060102T150405Z"))
	}
	if event.Organizer.Email != "" || event.Organizer.Name != "" {
		writeICSLine(&b, `ORGANIZER;CN=`+event.Organizer.Name, "mailto:"+event.Organizer.Email)
	}
	for _, attendee := range event.Attendees {
		role := "REQ-PARTICIPANT"
		if attendee.Optional {
			role = "OPT-PARTICIPANT"
		}
		key := `ATTENDEE;CN=` + attendee.Name + `;PARTSTAT=` + attendee.ResponseStatus + `;ROLE=` + role
		writeICSLine(&b, key, "mailto:"+attendee.Email)
	}
	for _, rule := range event.Recurrence {
		key, value, ok := strings.Cut(rule, ":")
		if !ok {
			continue
		}
		writeICSLine(&b, key, value)
	}
	for _, attachment := range event.Attachments {
		key := `ATTACH;FILENAME=` + attachment.Title
		if attachment.MIMEType != "" {
			key += `;FMTTYPE=` + attachment.MIMEType
		}
		writeICSLine(&b, key, attachment.FileURL)
	}
	for _, reminder := range event.Reminders {
		action := "DISPLAY"
		if strings.EqualFold(reminder.Method, "email") {
			action = "EMAIL"
		}
		b.WriteString("BEGIN:VALARM\r\n")
		writeICSLine(&b, "ACTION", action)
		writeICSLine(&b, "TRIGGER", formatICSReminderTrigger(reminder.MinutesBefore))
		if action == "EMAIL" {
			writeICSLine(&b, "SUMMARY", event.Summary)
			writeICSLine(&b, "DESCRIPTION", event.Description)
		}
		b.WriteString("END:VALARM\r\n")
	}
	if !event.Updated.IsZero() {
		writeICSLine(&b, "LAST-MODIFIED", event.Updated.UTC().Format("20060102T150405Z"))
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

func applyICSPatch(event Event, data string) Event {
	seenAttendee := false
	inAlarm := false
	alarmAction := ""
	alarmTrigger := ""
	seenAlarm := false
	for _, line := range unfoldICSLines(data) {
		nameAndParams, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		parts := strings.Split(nameAndParams, ";")
		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		params := parseICSParams(parts[1:])
		value = unescapeICSValue(value)
		if key == "BEGIN" && strings.EqualFold(value, "VALARM") {
			inAlarm = true
			alarmAction = ""
			alarmTrigger = ""
			if !seenAlarm {
				event.Reminders = nil
				seenAlarm = true
			}
			continue
		}
		if key == "END" && strings.EqualFold(value, "VALARM") {
			if reminder, ok := reminderFromAlarm(alarmAction, alarmTrigger); ok {
				event.Reminders = append(event.Reminders, reminder)
			}
			inAlarm = false
			continue
		}
		if inAlarm {
			switch key {
			case "ACTION":
				alarmAction = value
			case "TRIGGER":
				alarmTrigger = value
			}
			continue
		}
		switch key {
		case "UID":
			event.UID = value
		case "SUMMARY":
			event.Summary = value
		case "DESCRIPTION":
			event.Description = value
		case "LOCATION":
			event.Location = value
		case "STATUS":
			event.Status = value
		case "DTSTART":
			start, allDay := parseICSTime(value, params["TZID"])
			event.Start = start
			event.AllDay = allDay
			if params["TZID"] != "" {
				event.TimeZone = params["TZID"]
			}
		case "DTEND":
			end, _ := parseICSTime(value, params["TZID"])
			event.End = end
			if event.TimeZone == "" && params["TZID"] != "" {
				event.TimeZone = params["TZID"]
			}
		case "ATTENDEE":
			if !seenAttendee {
				event.Attendees = nil
				seenAttendee = true
			}
			event.Attendees = append(event.Attendees, Attendee{
				Name:           params["CN"],
				Email:          calendarMailtoAddress(value),
				ResponseStatus: params["PARTSTAT"],
				Optional:       strings.EqualFold(params["ROLE"], "OPT-PARTICIPANT"),
			})
		}
	}
	return event
}

func reminderFromAlarm(action, trigger string) (Reminder, bool) {
	minutes, ok := parseICSReminderTrigger(trigger)
	if !ok {
		return Reminder{}, false
	}
	method := "popup"
	if strings.EqualFold(action, "EMAIL") {
		method = "email"
	}
	return Reminder{Method: method, MinutesBefore: minutes}, true
}

func formatICSReminderTrigger(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	if minutes%1440 == 0 && minutes != 0 {
		return fmt.Sprintf("-P%dD", minutes/1440)
	}
	if minutes%60 == 0 && minutes != 0 {
		return fmt.Sprintf("-PT%dH", minutes/60)
	}
	return fmt.Sprintf("-PT%dM", minutes)
}

func parseICSReminderTrigger(trigger string) (int, bool) {
	trigger = strings.TrimSpace(strings.ToUpper(trigger))
	if !strings.HasPrefix(trigger, "-P") {
		return 0, false
	}
	trigger = strings.TrimPrefix(trigger, "-P")
	if strings.HasPrefix(trigger, "T") {
		trigger = strings.TrimPrefix(trigger, "T")
	}
	multiplier := 1
	switch {
	case strings.HasSuffix(trigger, "M"):
		trigger = strings.TrimSuffix(trigger, "M")
	case strings.HasSuffix(trigger, "H"):
		trigger = strings.TrimSuffix(trigger, "H")
		multiplier = 60
	case strings.HasSuffix(trigger, "D"):
		trigger = strings.TrimSuffix(trigger, "D")
		multiplier = 24 * 60
	default:
		return 0, false
	}
	value, err := strconv.Atoi(trigger)
	if err != nil || value < 0 {
		return 0, false
	}
	return value * multiplier, true
}

func unfoldICSLines(data string) []string {
	raw := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(lines) > 0 {
				lines[len(lines)-1] += strings.TrimLeft(line, " \t")
			}
			continue
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseICSParams(parts []string) map[string]string {
	params := make(map[string]string, len(parts))
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		params[strings.ToUpper(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(unescapeICSValue(value)), `"`)
	}
	return params
}

func parseICSTime(value, timezone string) (time.Time, bool) {
	if len(value) == len("20060102") {
		parsed, _ := time.Parse("20060102", value)
		return parsed, true
	}
	if strings.HasSuffix(value, "Z") {
		parsed, _ := time.Parse("20060102T150405Z", value)
		return parsed, false
	}
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			parsed, _ := time.ParseInLocation("20060102T150405", value, loc)
			return parsed, false
		}
	}
	parsed, _ := time.Parse("20060102T150405", value)
	return parsed, false
}

func calendarMailtoAddress(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "mailto:") {
		return strings.TrimSpace(value[len("mailto:"):])
	}
	return value
}

func unescapeICSValue(value string) string {
	return strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`).Replace(value)
}

func nextETag(event Event) string {
	id := strings.TrimSuffix(event.ID, ".ics")
	if id == "" {
		id = event.UID
	}
	return fmt.Sprintf(`"%s-v%d"`, id, time.Now().UnixNano())
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
