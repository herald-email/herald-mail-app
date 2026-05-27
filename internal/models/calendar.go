package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type CalendarCollection struct {
	Ref       CollectionRef
	Color     string
	SyncToken string
	ETag      string
}

type CalendarEvent struct {
	Ref EventRef

	ProviderUID        string
	Title              string
	Description        string
	Location           string
	Start              time.Time
	End                time.Time
	AllDay             bool
	TimeZone           string
	Status             string
	Organizer          string
	OrganizerEmail     string
	Attendees          []CalendarAttendee
	Recurrence         []string
	RecurrenceSummary  string
	Attachments        []CalendarAttachment
	Reminders          []CalendarReminder
	AlternateTimeZones []string
	Revision           string
	UpdatedAt          time.Time
	Raw                string
}

type CalendarAttendee struct {
	Name     string
	Email    string
	RSVP     string
	Optional bool
}

type CalendarAttachment struct {
	Title    string
	URI      string
	MIMEType string
}

type CalendarReminder struct {
	Method        string
	MinutesBefore int
}

const (
	CalendarMutationScopeThisEvent = "this_event"
	CalendarMutationScopeAllEvents = "all_events"
)

var (
	ErrCalendarMutationConflict           = errors.New("calendar mutation conflict")
	ErrCalendarRecurrenceScopeUnsupported = errors.New("calendar recurrence scope unsupported")
)

type CalendarMutationOptions struct {
	RecurrenceScope string
	IfMatch         string
}

func NormalizeCalendarMutationOptions(opts CalendarMutationOptions) (CalendarMutationOptions, error) {
	scope := normalizeCalendarMutationScope(opts.RecurrenceScope)
	if scope == "" {
		scope = CalendarMutationScopeThisEvent
	}
	opts.RecurrenceScope = scope
	if scope != CalendarMutationScopeThisEvent {
		return opts, fmt.Errorf("%w: %s", ErrCalendarRecurrenceScopeUnsupported, CalendarMutationScopeLabel(scope))
	}
	return opts, nil
}

func CalendarMutationScopeLabel(scope string) string {
	switch normalizeCalendarMutationScope(scope) {
	case "", CalendarMutationScopeThisEvent:
		return "this event"
	case CalendarMutationScopeAllEvents:
		return "all events"
	default:
		return strings.ReplaceAll(strings.TrimSpace(scope), "_", " ")
	}
}

func normalizeCalendarMutationScope(scope string) string {
	scope = strings.TrimSpace(strings.ToLower(scope))
	scope = strings.ReplaceAll(scope, "-", "_")
	scope = strings.ReplaceAll(scope, " ", "_")
	return scope
}

func (e CalendarEvent) EventRef() EventRef {
	return e.Ref.WithDefaults()
}

func (e CalendarEvent) CanonicalTimeZone() string {
	if e.TimeZone != "" {
		return e.TimeZone
	}
	if !e.Start.IsZero() && e.Start.Location() != nil {
		return e.Start.Location().String()
	}
	return "Local"
}

func CalendarEventMatchesQuery(event CalendarEvent, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}
	return strings.Contains(CalendarEventSearchText(event), query)
}

func CalendarEventSearchText(event CalendarEvent) string {
	ref := event.Ref.WithDefaults()
	parts := []string{
		event.Title,
		event.Description,
		event.Location,
		event.TimeZone,
		event.Status,
		event.Organizer,
		event.OrganizerEmail,
		event.RecurrenceSummary,
		string(ref.SourceID),
		string(ref.AccountID),
		ref.CalendarID,
	}
	parts = append(parts, event.Recurrence...)
	for _, attendee := range event.Attendees {
		parts = append(parts, attendee.Name, attendee.Email, attendee.RSVP)
		if attendee.Optional {
			parts = append(parts, "optional")
		}
	}
	for _, attachment := range event.Attachments {
		parts = append(parts, attachment.Title, attachment.MIMEType)
	}
	for _, reminder := range event.Reminders {
		parts = append(parts, reminder.Method, fmt.Sprintf("%dm", reminder.MinutesBefore))
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func NormalizeCalendarRSVP(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(value, "_", "-")))
	switch value {
	case "accepted", "accept", "yes":
		return "accepted", nil
	case "tentative", "maybe":
		return "tentative", nil
	case "declined", "decline", "no":
		return "declined", nil
	case "needs-action", "needsaction", "none", "":
		return "needs-action", nil
	default:
		return "", fmt.Errorf("unsupported RSVP status %q", value)
	}
}

func NextCalendarRSVP(value string) string {
	normalized, err := NormalizeCalendarRSVP(value)
	if err != nil {
		return "accepted"
	}
	switch normalized {
	case "accepted":
		return "tentative"
	case "tentative":
		return "declined"
	case "declined":
		return "needs-action"
	default:
		return "accepted"
	}
}
