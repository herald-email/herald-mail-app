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
		calendarEvent("daily-standup", "Daily standup", "Walk the day plan and identify calendar conflicts.", "Huddle room", baseTime.Add(90*time.Minute), 30*time.Minute, "confirmed"),
		calendarEvent("design-review", "Design review", "Review agenda layout with deterministic demo data.", "Herald planning room", baseTime.Add(2*time.Hour), time.Hour, "confirmed"),
		calendarEvent("launch-window", "Launch window", "Keep a protected block for release notes and follow-up review.", "", baseTime.Add(4*time.Hour), 75*time.Minute, "busy"),
		calendarEvent("weekly-planning", "Weekly planning", "Confirm source-platform roadmap sequencing and next actions.", "Video call", baseTime.AddDate(0, 0, 1).Add(time.Hour), time.Hour, "tentative"),
		calendarEvent("cache-sync", "Calendar cache sync", "Check that read-only event details come from Herald cache rows first.", "Engineering desk", baseTime.AddDate(0, 0, 2).Add(30*time.Minute), 45*time.Minute, "confirmed"),
		calendarEvent("focus-block", "Focus block", "Protected work time for cleanup rules and source identity notes.", "", baseTime.AddDate(0, 0, 3).Add(3*time.Hour), 90*time.Minute, "busy"),
	}
	events[0].TimeZone = "America/Los_Angeles"
	events[0].Organizer = "Mina Park"
	events[0].OrganizerEmail = "mina@example.com"
	events[0].Attendees = []models.CalendarAttendee{
		{Name: "Rae Stone", Email: "rae@example.com", RSVP: "accepted"},
		{Name: "Noor Patel", Email: "noor@example.com", RSVP: "tentative", Optional: true},
	}
	events[0].Recurrence = []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"}
	events[0].RecurrenceSummary = "Weekly on Monday"
	events[0].Attachments = []models.CalendarAttachment{
		{Title: "Agenda", URI: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
	}
	events[0].Reminders = []models.CalendarReminder{
		{Method: "popup", MinutesBefore: 30},
	}
	events[0].AlternateTimeZones = []string{"Asia/Tokyo"}
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
