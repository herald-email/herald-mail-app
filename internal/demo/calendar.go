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
				DisplayName:  "Work",
			},
			Color: "#5fd7ff",
			ETag:  `"demo-calendar-v1"`,
		},
		{
			Ref: models.CollectionRef{
				SourceID:     CalendarSourceID,
				AccountID:    CalendarAccountID,
				Kind:         models.SourceKindCalendar,
				CollectionID: "home",
				DisplayName:  "Home",
			},
			Color: "#ff5f87",
			ETag:  `"demo-calendar-home-v1"`,
		},
		{
			Ref: models.CollectionRef{
				SourceID:     CalendarSourceID,
				AccountID:    CalendarAccountID,
				Kind:         models.SourceKindCalendar,
				CollectionID: "family",
				DisplayName:  "Family",
			},
			Color: "#87d75f",
			ETag:  `"demo-calendar-family-v1"`,
		},
		{
			Ref: models.CollectionRef{
				SourceID:     CalendarSourceID,
				AccountID:    CalendarAccountID,
				Kind:         models.SourceKindCalendar,
				CollectionID: "holidays",
				DisplayName:  "Holidays",
			},
			Color: "#ffd75f",
			ETag:  `"demo-calendar-holidays-v1"`,
		},
	}
}

func CalendarEvents() []models.CalendarEvent {
	events := []models.CalendarEvent{
		calendarEvent("work", "daily-standup", "Daily standup", "<p>Walk the day plan and identify <strong>calendar conflicts</strong>.</p><ul><li>Confirm owners</li><li>Call out blockers</li></ul>", "Huddle room", baseTime.Add(90*time.Minute), 30*time.Minute, "confirmed"),
		calendarEvent("work", "design-review", "Design review", "Review agenda layout with deterministic demo data.", "Herald planning room", baseTime.Add(2*time.Hour), time.Hour, "confirmed"),
		calendarEvent("work", "launch-window", "Launch window", "Keep a protected block for release notes and follow-up review.", "", baseTime.Add(4*time.Hour), 75*time.Minute, "busy"),
		calendarEvent("home", "weekly-planning", "Weekly planning", "Confirm source-platform roadmap sequencing and next actions.", "Video call", baseTime.AddDate(0, 0, 1).Add(time.Hour), time.Hour, "tentative"),
		calendarEvent("family", "school-pickup", "School pickup", "Bring the signed field trip form.", "North gate", baseTime.AddDate(0, 0, 1).Add(6*time.Hour), 30*time.Minute, "confirmed"),
		calendarEvent("work", "cache-sync", "Calendar cache sync", "Check that read-only event details come from Herald cache rows first.", "Engineering desk", baseTime.AddDate(0, 0, 2).Add(30*time.Minute), 45*time.Minute, "confirmed"),
		calendarEvent("holidays", "regional-holiday", "Regional holiday", "Public holiday shown from the subscribed calendar.", "", baseTime.AddDate(0, 0, 2), 24*time.Hour, "confirmed"),
		calendarEvent("home", "focus-block", "Focus block", "Protected work time for cleanup rules and source identity notes.", "", baseTime.AddDate(0, 0, 3).Add(3*time.Hour), 90*time.Minute, "busy"),
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
	events[1].Attendees = []models.CalendarAttendee{
		{Name: "Rae Stone", Email: "rae@example.com", RSVP: "needs-action"},
		{Name: "Mina Park", Email: "mina@example.com", RSVP: "accepted"},
	}
	out := make([]models.CalendarEvent, len(events))
	copy(out, events)
	return out
}

func calendarEvent(calendarID, id, title, description, location string, start time.Time, duration time.Duration, status string) models.CalendarEvent {
	return models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   CalendarSourceID,
			AccountID:  CalendarAccountID,
			CalendarID: calendarID,
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
