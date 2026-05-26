package models

import (
	"fmt"
	"strings"
	"time"
)

const CalendarEventEditTimeLayout = "2006-01-02 15:04"

type CalendarEventEditDraft struct {
	Ref                EventRef
	Title              string
	Description        string
	Location           string
	StartText          string
	EndText            string
	TimeZone           string
	AttendeesText      string
	RecurrenceText     string
	Status             string
	AllDay             bool
	RecurrenceScope    string
	AlternateTimeZones []string
}

type CalendarTimezonePreview struct {
	Local            string
	Event            string
	Alternates       []string
	DateCrossingNote string
}

func NewCalendarEventEditDraft(event CalendarEvent) CalendarEventEditDraft {
	event.Ref = event.Ref.WithDefaults()
	loc := calendarEditLocation(event.CanonicalTimeZone(), event.Start.Location())
	start := event.Start
	end := event.End
	if !start.IsZero() {
		start = start.In(loc)
	}
	if !end.IsZero() {
		end = end.In(loc)
	}
	return CalendarEventEditDraft{
		Ref:                event.Ref,
		Title:              event.Title,
		Description:        event.Description,
		Location:           event.Location,
		StartText:          formatCalendarEditTime(start),
		EndText:            formatCalendarEditTime(end),
		TimeZone:           event.CanonicalTimeZone(),
		AttendeesText:      formatCalendarEditAttendees(event.Attendees),
		RecurrenceText:     formatCalendarEditRecurrence(event.Recurrence),
		Status:             event.Status,
		AllDay:             event.AllDay,
		RecurrenceScope:    CalendarMutationScopeThisEvent,
		AlternateTimeZones: append([]string(nil), event.AlternateTimeZones...),
	}
}

func (d CalendarEventEditDraft) Apply(base CalendarEvent) (CalendarEvent, error) {
	updated := base
	updated.Ref = firstEventRef(d.Ref, base.Ref).WithDefaults()
	updated.Title = strings.TrimSpace(d.Title)
	updated.Description = strings.TrimSpace(d.Description)
	updated.Location = strings.TrimSpace(d.Location)
	updated.TimeZone = strings.TrimSpace(d.TimeZone)
	if updated.TimeZone == "" {
		updated.TimeZone = base.CanonicalTimeZone()
	}
	loc, err := time.LoadLocation(updated.TimeZone)
	if err != nil {
		if strings.EqualFold(updated.TimeZone, "local") {
			loc = time.Local
		} else {
			return CalendarEvent{}, fmt.Errorf("timezone %q is not available", updated.TimeZone)
		}
	}
	start, err := parseCalendarEditTime(d.StartText, loc)
	if err != nil {
		return CalendarEvent{}, fmt.Errorf("start time: %w", err)
	}
	end, err := parseCalendarEditTime(d.EndText, loc)
	if err != nil {
		return CalendarEvent{}, fmt.Errorf("end time: %w", err)
	}
	if !start.IsZero() && !end.IsZero() && !end.After(start) {
		return CalendarEvent{}, fmt.Errorf("end time must be after start time")
	}
	updated.Start = start
	updated.End = end
	updated.AllDay = d.AllDay
	updated.Status = strings.TrimSpace(d.Status)
	updated.AlternateTimeZones = cleanCalendarEditTimeZones(d.AlternateTimeZones)
	attendees, err := parseCalendarEditAttendees(d.AttendeesText)
	if err != nil {
		return CalendarEvent{}, err
	}
	recurrence := parseCalendarEditRecurrence(d.RecurrenceText)
	updated.Attendees = attendees
	updated.Recurrence = recurrence
	updated.RecurrenceSummary = summarizeCalendarEditRecurrence(recurrence)
	if _, err := NormalizeCalendarMutationOptions(CalendarMutationOptions{RecurrenceScope: d.RecurrenceScope}); err != nil {
		return CalendarEvent{}, err
	}
	updated.UpdatedAt = time.Now().UTC()
	return updated, nil
}

func (d CalendarEventEditDraft) TimezonePreview(base CalendarEvent) (CalendarTimezonePreview, error) {
	event, err := d.Apply(base)
	if err != nil {
		return CalendarTimezonePreview{}, err
	}
	preview := CalendarTimezonePreview{
		Local: calendarEditPreviewLine("Local", "Local", event, time.Local),
		Event: calendarEditPreviewLine("Event TZ", event.CanonicalTimeZone(), event, calendarEditLocation(event.CanonicalTimeZone(), event.Start.Location())),
	}
	eventStart := event.Start.In(calendarEditLocation(event.CanonicalTimeZone(), event.Start.Location()))
	for _, timezone := range calendarEditAlternateTimeZones(event) {
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			continue
		}
		preview.Alternates = append(preview.Alternates, calendarEditPreviewLine("Alt TZ", timezone, event, loc))
		altStart := event.Start.In(loc)
		if preview.DateCrossingNote == "" && !sameCalendarEditDate(eventStart, altStart) {
			preview.DateCrossingNote = fmt.Sprintf("date changes in %s (%s)", timezone, altStart.Format("Mon Jan 2"))
		}
	}
	if preview.DateCrossingNote == "" {
		localStart := event.Start.In(time.Local)
		if !sameCalendarEditDate(eventStart, localStart) {
			preview.DateCrossingNote = fmt.Sprintf("date changes locally (%s)", localStart.Format("Mon Jan 2"))
		}
	}
	return preview, nil
}

func parseCalendarEditTime(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if loc == nil {
		loc = time.Local
	}
	parsed, err := time.ParseInLocation(CalendarEventEditTimeLayout, value, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("use %s", CalendarEventEditTimeLayout)
	}
	return parsed, nil
}

func formatCalendarEditTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(CalendarEventEditTimeLayout)
}

func calendarEditLocation(timezone string, fallback *time.Location) *time.Location {
	timezone = strings.TrimSpace(timezone)
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			return loc
		}
	}
	if fallback != nil {
		return fallback
	}
	return time.Local
}

func calendarEditPreviewLine(label, timezone string, event CalendarEvent, loc *time.Location) string {
	if strings.TrimSpace(timezone) == "" {
		timezone = "Local"
	}
	return fmt.Sprintf("%s  %s  %s", label, timezone, calendarEditTimeRange(event, loc))
}

func calendarEditTimeRange(event CalendarEvent, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	if event.Start.IsZero() {
		return "unscheduled"
	}
	if event.AllDay {
		return event.Start.In(loc).Format("Mon Jan 2") + " (all day)"
	}
	start := event.Start.In(loc)
	if event.End.IsZero() {
		return start.Format("Mon Jan 2 15:04 MST")
	}
	end := event.End.In(loc)
	if sameCalendarEditDate(start, end) {
		return start.Format("Mon Jan 2 15:04") + " - " + end.Format("15:04 MST")
	}
	return start.Format("Mon Jan 2 15:04 MST") + " - " + end.Format("Mon Jan 2 15:04 MST")
}

func sameCalendarEditDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func calendarEditAlternateTimeZones(event CalendarEvent) []string {
	seen := map[string]bool{"": true, event.CanonicalTimeZone(): true}
	out := make([]string, 0, len(event.AlternateTimeZones))
	for _, timezone := range event.AlternateTimeZones {
		timezone = strings.TrimSpace(timezone)
		if seen[timezone] {
			continue
		}
		seen[timezone] = true
		out = append(out, timezone)
	}
	return out
}

func cleanCalendarEditTimeZones(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{"": true}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func formatCalendarEditAttendees(attendees []CalendarAttendee) string {
	parts := make([]string, 0, len(attendees))
	for _, attendee := range attendees {
		email := strings.TrimSpace(attendee.Email)
		if email == "" {
			continue
		}
		var b strings.Builder
		if name := strings.TrimSpace(attendee.Name); name != "" {
			b.WriteString(name)
			b.WriteString(" <")
			b.WriteString(email)
			b.WriteString(">")
		} else {
			b.WriteString(email)
		}
		if rsvp := strings.TrimSpace(attendee.RSVP); rsvp != "" {
			b.WriteString(" ")
			b.WriteString(rsvp)
		}
		if attendee.Optional {
			b.WriteString(" optional")
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "; ")
}

func parseCalendarEditAttendees(value string) ([]CalendarAttendee, error) {
	value = strings.ReplaceAll(value, "\n", ";")
	value = strings.ReplaceAll(value, "\r", ";")
	entries := strings.Split(value, ";")
	attendees := make([]CalendarAttendee, 0, len(entries))
	for _, entry := range entries {
		attendee, ok, err := parseCalendarEditAttendee(entry)
		if err != nil {
			return nil, err
		}
		if ok {
			attendees = append(attendees, attendee)
		}
	}
	return attendees, nil
}

func parseCalendarEditAttendee(entry string) (CalendarAttendee, bool, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return CalendarAttendee{}, false, nil
	}
	var attendee CalendarAttendee
	rest := ""
	if open := strings.LastIndex(entry, "<"); open >= 0 {
		if close := strings.Index(entry[open:], ">"); close >= 0 {
			close += open
			attendee.Name = strings.TrimSpace(entry[:open])
			attendee.Email = strings.TrimSpace(entry[open+1 : close])
			rest = strings.TrimSpace(entry[close+1:])
		}
	}
	if attendee.Email == "" {
		fields := strings.Fields(entry)
		if len(fields) == 0 {
			return CalendarAttendee{}, false, nil
		}
		attendee.Email = strings.TrimSpace(fields[0])
		if len(fields) > 1 {
			rest = strings.Join(fields[1:], " ")
		}
	}
	if attendee.Email == "" {
		return CalendarAttendee{}, false, fmt.Errorf("attendee is missing an email address")
	}
	for _, token := range strings.Fields(rest) {
		if strings.EqualFold(token, "optional") {
			attendee.Optional = true
			continue
		}
		rsvp, err := NormalizeCalendarRSVP(token)
		if err != nil {
			return CalendarAttendee{}, false, fmt.Errorf("attendee %s: %w", attendee.Email, err)
		}
		attendee.RSVP = rsvp
	}
	if attendee.RSVP == "" {
		attendee.RSVP = "needs-action"
	}
	return attendee, true, nil
}

func formatCalendarEditRecurrence(rules []string) string {
	out := cleanCalendarEditRecurrence(rules)
	return strings.Join(out, " | ")
}

func parseCalendarEditRecurrence(value string) []string {
	value = strings.ReplaceAll(value, "\n", "|")
	value = strings.ReplaceAll(value, "\r", "|")
	return cleanCalendarEditRecurrence(strings.Split(value, "|"))
}

func cleanCalendarEditRecurrence(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{"": true}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func summarizeCalendarEditRecurrence(rules []string) string {
	for _, rule := range rules {
		key, value, ok := strings.Cut(rule, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "RRULE") {
			continue
		}
		attrs := calendarEditRecurrenceAttrs(value)
		switch attrs["FREQ"] {
		case "DAILY":
			return "Daily"
		case "WEEKLY":
			if days := summarizeCalendarEditRecurrenceDays(attrs["BYDAY"]); days != "" {
				return "Weekly on " + days
			}
			return "Weekly"
		case "MONTHLY":
			return "Monthly"
		case "YEARLY":
			return "Yearly"
		}
	}
	if len(rules) > 0 {
		return rules[0]
	}
	return ""
}

func calendarEditRecurrenceAttrs(value string) map[string]string {
	attrs := map[string]string{}
	for _, part := range strings.Split(value, ";") {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		attrs[strings.ToUpper(strings.TrimSpace(key))] = strings.ToUpper(strings.TrimSpace(val))
	}
	return attrs
}

func summarizeCalendarEditRecurrenceDays(byDay string) string {
	if byDay == "" {
		return ""
	}
	labels := map[string]string{
		"MO": "Monday",
		"TU": "Tuesday",
		"WE": "Wednesday",
		"TH": "Thursday",
		"FR": "Friday",
		"SA": "Saturday",
		"SU": "Sunday",
	}
	parts := strings.Split(byDay, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if label := labels[strings.TrimSpace(part)]; label != "" {
			out = append(out, label)
		}
	}
	return strings.Join(out, ", ")
}

func firstEventRef(primary, fallback EventRef) EventRef {
	if primary.SourceID != "" || primary.AccountID != "" || primary.CalendarID != "" || primary.EventID != "" || primary.InstanceID != "" || primary.LocalID != "" {
		return primary
	}
	return fallback
}
