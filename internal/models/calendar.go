package models

import (
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
	return strings.ToLower(strings.Join(parts, " "))
}
