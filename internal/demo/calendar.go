package demo

import (
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	CalendarSourceID  models.SourceID  = "demo-calendar"
	CalendarAccountID models.AccountID = models.DefaultAccountID
)

func CalendarCollections() []models.CalendarCollection {
	return []models.CalendarCollection{
		{
			Ref: models.CollectionRef{
				SourceID:     CalendarSourceID,
				AccountID:    CalendarAccountID,
				Kind:         models.SourceKindCalendar,
				CollectionID: "work",
				DisplayName:  "Demo Calendar",
			},
			Color: "#3367d6",
			ETag:  `"demo-calendar-v1"`,
		},
	}
}

func CalendarEvents() []models.CalendarEvent {
	events := []models.CalendarEvent{
		calendarEvent("design-review", "Design review", "Review agenda layout with deterministic demo data.", "Herald planning room", baseTime.Add(2*time.Hour), time.Hour, "confirmed"),
		calendarEvent("weekly-planning", "Weekly planning", "Confirm source-platform roadmap sequencing and next actions.", "Video call", baseTime.AddDate(0, 0, 1).Add(time.Hour), time.Hour, "tentative"),
		calendarEvent("cache-sync", "Calendar cache sync", "Check that read-only event details come from Herald cache rows first.", "Engineering desk", baseTime.AddDate(0, 0, 2).Add(30*time.Minute), 45*time.Minute, "confirmed"),
		calendarEvent("focus-block", "Focus block", "Protected work time for cleanup rules and source identity notes.", "", baseTime.AddDate(0, 0, 3).Add(3*time.Hour), 90*time.Minute, "busy"),
	}
	out := make([]models.CalendarEvent, len(events))
	copy(out, events)
	return out
}

func calendarEvent(id, title, description, location string, start time.Time, duration time.Duration, status string) models.CalendarEvent {
	return models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   CalendarSourceID,
			AccountID:  CalendarAccountID,
			CalendarID: "work",
			EventID:    id,
			ETag:       `"demo-` + id + `-v1"`,
		}.WithDefaults(),
		ProviderUID: id,
		Title:       title,
		Description: description,
		Location:    location,
		Start:       start,
		End:         start.Add(duration),
		Status:      status,
		Revision:    "1",
		UpdatedAt:   baseTime,
	}
}
