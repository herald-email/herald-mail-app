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
	Status             string
	AllDay             bool
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
		Status:             event.Status,
		AllDay:             event.AllDay,
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

func firstEventRef(primary, fallback EventRef) EventRef {
	if primary.SourceID != "" || primary.AccountID != "" || primary.CalendarID != "" || primary.EventID != "" || primary.InstanceID != "" || primary.LocalID != "" {
		return primary
	}
	return fallback
}
