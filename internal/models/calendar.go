package models

import "time"

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
