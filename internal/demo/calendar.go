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
			Color: "#a7df78",
			ETag:  `"demo-calendar-v1"`,
		},
		{
			Ref: models.CollectionRef{
				SourceID:     CalendarSourceID,
				AccountID:    CalendarAccountID,
				Kind:         models.SourceKindCalendar,
				CollectionID: "personal",
				DisplayName:  "Personal",
			},
			Color: "#76cce0",
			ETag:  `"demo-calendar-home-v1"`,
		},
		{
			Ref: models.CollectionRef{
				SourceID:     CalendarSourceID,
				AccountID:    CalendarAccountID,
				Kind:         models.SourceKindCalendar,
				CollectionID: "birthdays",
				DisplayName:  "Birthdays",
			},
			Color: "#e2b86b",
			ETag:  `"demo-calendar-family-v1"`,
		},
		{
			Ref: models.CollectionRef{
				SourceID:     CalendarSourceID,
				AccountID:    CalendarAccountID,
				Kind:         models.SourceKindCalendar,
				CollectionID: "travel",
				DisplayName:  "Travel",
			},
			Color: "#b39df3",
			ETag:  `"demo-calendar-holidays-v1"`,
		},
	}
}

func CalendarEvents() []models.CalendarEvent {
	week := time.Date(2026, 4, 20, 7, 0, 0, 0, time.Local)
	at := func(day, hour, minute int) time.Time {
		return week.AddDate(0, 0, day).Add(time.Duration(hour-7)*time.Hour + time.Duration(minute)*time.Minute)
	}
	events := []models.CalendarEvent{
		calendarEvent("work", "mon-standup", "Standup", "Daily team standup.", "Huddle room", at(0, 8, 0), 30*time.Minute, "confirmed"),
		calendarEvent("work", "tue-standup", "Standup", "Daily team standup.", "Huddle room", at(1, 8, 0), 30*time.Minute, "confirmed"),
		calendarEvent("work", "wed-standup", "Standup", "Daily team standup.", "Huddle room", at(2, 8, 0), 30*time.Minute, "confirmed"),
		calendarEvent("work", "thu-standup", "Standup", "Daily team standup.", "Huddle room", at(3, 8, 0), 30*time.Minute, "confirmed"),
		calendarEvent("work", "fri-standup", "Standup", "Daily team standup.", "Huddle room", at(4, 8, 0), 30*time.Minute, "confirmed"),
		calendarEvent("work", "tue-design-review", "Design Review", "<p>Weekly design review of ongoing projects and feedback.</p><ul><li>Prototype alignment</li><li>Accessibility notes</li></ul>", "Design Lab", at(1, 10, 0), time.Hour, "confirmed"),
		calendarEvent("work", "mon-design-review", "Design Review", "Review agenda layout with deterministic demo data.", "Design Lab", at(0, 10, 0), time.Hour, "confirmed"),
		calendarEvent("personal", "wed-focus-block", "Focus Block", "Deep work block.", "", at(2, 10, 0), 2*time.Hour, "busy"),
		calendarEvent("work", "thu-planning", "Planning", "Roadmap and launch sequencing.", "Planning room", at(3, 10, 0), time.Hour, "confirmed"),
		calendarEvent("personal", "fri-design-review", "Design Review", "Personal project review.", "", at(4, 10, 0), time.Hour, "confirmed"),
		calendarEvent("birthdays", "mon-lunch", "Lunch", "Lunch break.", "Cafe", at(0, 12, 0), time.Hour, "confirmed"),
		calendarEvent("birthdays", "tue-lunch", "Lunch", "Lunch break.", "Cafe", at(1, 12, 0), time.Hour, "confirmed"),
		calendarEvent("birthdays", "wed-lunch", "Lunch", "Lunch break.", "Cafe", at(2, 12, 0), time.Hour, "confirmed"),
		calendarEvent("birthdays", "thu-lunch", "Lunch", "Lunch break.", "Cafe", at(3, 12, 0), time.Hour, "confirmed"),
		calendarEvent("birthdays", "fri-lunch", "Lunch", "Lunch break.", "Cafe", at(4, 12, 0), time.Hour, "confirmed"),
		calendarEvent("personal", "mon-focus", "Focus Block", "Heads-down implementation time.", "", at(0, 13, 0), 2*time.Hour, "busy"),
		calendarEvent("work", "tue-project-sync", "Project Sync", "Status, blockers, decisions.", "Project room", at(1, 13, 0), time.Hour, "confirmed"),
		calendarEvent("personal", "thu-focus", "Focus Block", "Deep work block.", "", at(3, 13, 0), 2*time.Hour, "busy"),
		calendarEvent("work", "fri-one-one", "1:1 Alex", "Weekly check-in.", "Video", at(4, 13, 0), time.Hour, "confirmed"),
		calendarEvent("travel", "tue-travel-prep", "Travel Prep", "Confirm tickets, hotel, and packing list.", "", at(1, 14, 0), 2*time.Hour, "tentative"),
		calendarEvent("work", "thu-planning-pm", "Planning", "Implementation review.", "Planning room", at(3, 15, 0), time.Hour, "confirmed"),
		calendarEvent("work", "mon-ship-review", "Ship Review", "Release check.", "Ops room", at(0, 16, 0), time.Hour, "confirmed"),
		calendarEvent("work", "wed-ship-review", "Ship Review", "Release check.", "Ops room", at(2, 16, 0), time.Hour, "confirmed"),
		calendarEvent("work", "fri-ship-review", "Ship Review", "Release check.", "Ops room", at(4, 16, 0), time.Hour, "confirmed"),
	}
	events[5].TimeZone = "America/Los_Angeles"
	events[5].Organizer = "Mina Park"
	events[5].OrganizerEmail = "mina@example.com"
	events[5].Attendees = []models.CalendarAttendee{
		{Name: "Rae Stone", Email: "rae@example.com", RSVP: "accepted"},
		{Name: "Alex Chen", Email: "alex@example.com", RSVP: "accepted"},
		{Name: "Jordan Lee", Email: "jordan@example.com", RSVP: "needs-action"},
		{Name: "Sam Patel", Email: "sam@example.com", RSVP: "accepted"},
		{Name: "Taylor Fox", Email: "taylor@example.com", RSVP: "tentative"},
		{Name: "You", Email: "you@example.com", RSVP: "accepted"},
	}
	events[5].Recurrence = []string{"RRULE:FREQ=WEEKLY;BYDAY=TU"}
	events[5].RecurrenceSummary = "Weekly on Tuesday"
	events[5].Attachments = []models.CalendarAttachment{
		{Title: "Agenda", URI: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
	}
	events[5].Reminders = []models.CalendarReminder{
		{Method: "popup", MinutesBefore: 30},
	}
	events[5].AlternateTimeZones = []string{"Europe/London"}
	events[6].Attendees = []models.CalendarAttendee{
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
