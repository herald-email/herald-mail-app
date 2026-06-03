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
	week := calendarFixtureDayStart(time.Now()).Add(7 * time.Hour)
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		losAngeles = time.Local
	}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		tokyo = time.Local
	}
	at := func(day, hour, minute int) time.Time {
		return week.AddDate(0, 0, day).Add(time.Duration(hour-7)*time.Hour + time.Duration(minute)*time.Minute)
	}
	events := []models.CalendarEvent{
		calendarEvent("work", "demo-product-review-existing", "Product review", "Existing event imported from the demo invitation UID.", "Video", at(2, 15, 0), 45*time.Minute, "confirmed"),
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
	events[0].ProviderUID = "demo-product-review-invite@herald.demo"
	events[6].TimeZone = "America/Los_Angeles"
	events[6].Organizer = "Mina Park"
	events[6].OrganizerEmail = "mina@example.com"
	events[6].Attendees = []models.CalendarAttendee{
		{Name: "Rae Stone", Email: "rae@example.com", RSVP: "accepted"},
		{Name: "Alex Chen", Email: "alex@example.com", RSVP: "accepted"},
		{Name: "Jordan Lee", Email: "jordan@example.com", RSVP: "needs-action"},
		{Name: "Sam Patel", Email: "sam@example.com", RSVP: "accepted"},
		{Name: "Taylor Fox", Email: "taylor@example.com", RSVP: "tentative"},
		{Name: "You", Email: "you@example.com", RSVP: "accepted"},
	}
	events[6].Recurrence = []string{"RRULE:FREQ=WEEKLY;BYDAY=TU"}
	events[6].RecurrenceSummary = "Weekly on Tuesday"
	events[6].Attachments = []models.CalendarAttachment{
		{Title: "Agenda", URI: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
	}
	events[6].Reminders = []models.CalendarReminder{
		{Method: "popup", MinutesBefore: 30},
	}
	events[6].AlternateTimeZones = []string{"Europe/London"}
	events[7].Attendees = []models.CalendarAttendee{
		{Name: "Rae Stone", Email: "rae@example.com", RSVP: "needs-action"},
		{Name: "Mina Park", Email: "mina@example.com", RSVP: "accepted"},
	}
	flightStartDay := week.AddDate(0, 0, 1)
	events[20].Title = "LAX -> HND Flight"
	events[20].Description = "<p>Flight notes from the travel desk.</p><ul><li><strong>Bring</strong> passport and headphones</li><li>Check rail transfer after landing</li></ul>\n\n**Markdown note:** confirm hotel check-in."
	events[20].Location = "LAX TBIT -> HND T3"
	events[20].Start = time.Date(flightStartDay.Year(), flightStartDay.Month(), flightStartDay.Day(), 22, 30, 0, 0, losAngeles)
	events[20].End = time.Date(flightStartDay.Year(), flightStartDay.Month(), flightStartDay.Day()+2, 5, 30, 0, 0, tokyo)
	events[20].TimeZone = "America/Los_Angeles"
	events[20].StartTimeZone = "America/Los_Angeles"
	events[20].EndTimeZone = "Asia/Tokyo"
	events[20].AlternateTimeZones = []string{"Europe/London"}
	events[20].Reminders = []models.CalendarReminder{
		{Method: "popup", MinutesBefore: 180},
		{Method: "popup", MinutesBefore: 45},
	}
	out := make([]models.CalendarEvent, len(events))
	copy(out, events)
	return out
}

func calendarFixtureDayStart(t time.Time) time.Time {
	if t.IsZero() {
		t = time.Now()
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
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
