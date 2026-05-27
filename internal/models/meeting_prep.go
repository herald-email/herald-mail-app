package models

import (
	"strings"
	"time"
)

// CalendarMeetingPrep is a read-only command-center summary assembled from
// cached mail and calendar rows for one selected event.
type CalendarMeetingPrep struct {
	Event         CalendarEvent
	QueryTerms    []string
	RelatedMail   []*EmailData
	RelatedEvents []CalendarEvent
	GeneratedAt   time.Time
}

func CalendarMeetingPrepQueries(event CalendarEvent) []string {
	var out []string
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}

	add(event.Title)
	add(event.Organizer)
	add(event.OrganizerEmail)
	for _, attendee := range event.Attendees {
		add(attendee.Name)
		add(attendee.Email)
	}
	add(event.Location)
	return out
}
