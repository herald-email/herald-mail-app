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

	ProviderUID string
	Title       string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Status      string
	Revision    string
	UpdatedAt   time.Time
	Raw         string
}

func (e CalendarEvent) EventRef() EventRef {
	return e.Ref.WithDefaults()
}
