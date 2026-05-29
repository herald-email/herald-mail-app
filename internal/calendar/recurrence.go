package calendar

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type recurrenceRule struct {
	Freq     string
	Interval int
	Until    time.Time
	ByDays   []time.Weekday
}

// NormalizeEventFromRaw reparses cached CalDAV/iCalendar payloads so stale rows
// produced by older parsers use the VEVENT fields rather than VTIMEZONE fields.
func NormalizeEventFromRaw(event models.CalendarEvent) models.CalendarEvent {
	event.Ref = event.Ref.WithDefaults()
	if strings.TrimSpace(event.Ref.InstanceID) != "" {
		return event
	}
	if !strings.Contains(event.Raw, "BEGIN:VEVENT") {
		return event
	}
	ref := event.Ref
	parsed, err := EventFromICS(ref.SourceID, ref.AccountID, ref.CalendarID, ref.EventID, ref.ETag, event.Raw)
	if err != nil || parsed == nil || parsed.Start.IsZero() {
		event.Ref = ref
		return event
	}
	out := *parsed
	out.Ref = ref
	out.Raw = event.Raw
	if out.UpdatedAt.IsZero() {
		out.UpdatedAt = event.UpdatedAt
	}
	return out
}

// ExpandEventOccurrences returns concrete event instances that overlap the
// requested range. Unsupported recurrence rules fall back to the master event.
func ExpandEventOccurrences(event models.CalendarEvent, start, end time.Time) []models.CalendarEvent {
	event = NormalizeEventFromRaw(event)
	event.Ref = event.Ref.WithDefaults()
	if event.Start.IsZero() {
		return nil
	}
	if start.IsZero() {
		start = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if end.IsZero() {
		end = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	rule, ok := recurrenceRuleFromEvent(event)
	if !ok {
		if eventOverlapsRange(event, start, end) {
			return []models.CalendarEvent{event}
		}
		return nil
	}

	duration := eventDuration(event)
	excluded := recurrenceExclusionSet(event)
	var out []models.CalendarEvent
	addOccurrence := func(occurrenceStart time.Time) {
		if recurrenceDateExcluded(event, excluded, occurrenceStart) {
			return
		}
		occurrence := recurrenceOccurrenceEvent(event, occurrenceStart, duration)
		if eventOverlapsRange(occurrence, start, end) {
			out = append(out, occurrence)
		}
	}

	switch rule.Freq {
	case "DAILY":
		for occurrence, iterations := firstDailyOccurrence(event.Start, start, rule.Interval, duration), 0; occurrence.Before(end) && recurrenceBeforeUntil(occurrence, rule.Until) && iterations < 10000; occurrence, iterations = occurrence.AddDate(0, 0, rule.Interval), iterations+1 {
			addOccurrence(occurrence)
		}
	case "WEEKLY":
		if len(rule.ByDays) == 0 {
			intervalDays := 7 * rule.Interval
			for occurrence, iterations := firstDailyOccurrence(event.Start, start, intervalDays, duration), 0; occurrence.Before(end) && recurrenceBeforeUntil(occurrence, rule.Until) && iterations < 10000; occurrence, iterations = occurrence.AddDate(0, 0, intervalDays), iterations+1 {
				addOccurrence(occurrence)
			}
			break
		}
		for _, occurrence := range weeklyByDayOccurrences(event.Start, start, end, rule) {
			if recurrenceBeforeUntil(occurrence, rule.Until) {
				addOccurrence(occurrence)
			}
		}
	default:
		if eventOverlapsRange(event, start, end) {
			out = append(out, event)
		}
	}
	for _, occurrence := range recurrenceRDates(event) {
		addOccurrence(occurrence)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func recurrenceRuleFromEvent(event models.CalendarEvent) (recurrenceRule, bool) {
	for _, recurrence := range event.Recurrence {
		name, value, ok := strings.Cut(recurrence, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "RRULE") {
			continue
		}
		rule := recurrenceRule{Interval: 1}
		for _, part := range strings.Split(value, ";") {
			key, val, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			key = strings.ToUpper(strings.TrimSpace(key))
			val = strings.TrimSpace(val)
			switch key {
			case "FREQ":
				rule.Freq = strings.ToUpper(val)
			case "INTERVAL":
				if interval, err := strconv.Atoi(val); err == nil && interval > 0 {
					rule.Interval = interval
				}
			case "UNTIL":
				if until, _, err := parseICSTimeWithZone(val, event.TimeZone); err == nil {
					rule.Until = until
				}
			case "BYDAY":
				rule.ByDays = parseRecurrenceByDays(val)
			}
		}
		if rule.Freq == "DAILY" || rule.Freq == "WEEKLY" {
			return rule, true
		}
	}
	return recurrenceRule{}, false
}

func parseRecurrenceByDays(value string) []time.Weekday {
	seen := make(map[time.Weekday]bool)
	var days []time.Weekday
	for _, part := range strings.Split(value, ",") {
		part = strings.ToUpper(strings.TrimSpace(part))
		if len(part) > 2 {
			part = part[len(part)-2:]
		}
		day, ok := map[string]time.Weekday{
			"SU": time.Sunday,
			"MO": time.Monday,
			"TU": time.Tuesday,
			"WE": time.Wednesday,
			"TH": time.Thursday,
			"FR": time.Friday,
			"SA": time.Saturday,
		}[part]
		if ok && !seen[day] {
			seen[day] = true
			days = append(days, day)
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	return days
}

func firstDailyOccurrence(first, rangeStart time.Time, intervalDays int, duration time.Duration) time.Time {
	if intervalDays <= 0 || !first.Before(rangeStart) {
		return first
	}
	days := int(rangeStart.Sub(first).Hours() / 24)
	steps := days / intervalDays
	occurrence := first.AddDate(0, 0, steps*intervalDays)
	for occurrence.Add(duration).Before(rangeStart) {
		occurrence = occurrence.AddDate(0, 0, intervalDays)
	}
	return occurrence
}

func weeklyByDayOccurrences(first, rangeStart, rangeEnd time.Time, rule recurrenceRule) []time.Time {
	week := recurrenceWeekStart(first)
	if week.Before(rangeStart) {
		weeks := int(rangeStart.Sub(week).Hours() / (24 * 7))
		steps := weeks / rule.Interval
		week = week.AddDate(0, 0, steps*7*rule.Interval)
	}
	var out []time.Time
	for iterations := 0; week.Before(rangeEnd) && iterations < 10000; week, iterations = week.AddDate(0, 0, 7*rule.Interval), iterations+1 {
		for _, weekday := range rule.ByDays {
			day := week.AddDate(0, 0, int(weekday-time.Monday+7)%7)
			occurrence := time.Date(day.Year(), day.Month(), day.Day(), first.Hour(), first.Minute(), first.Second(), first.Nanosecond(), first.Location())
			if occurrence.Before(first) {
				continue
			}
			out = append(out, occurrence)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func recurrenceWeekStart(value time.Time) time.Time {
	day := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	offset := (int(day.Weekday()) - int(time.Monday) + 7) % 7
	return day.AddDate(0, 0, -offset)
}

func eventDuration(event models.CalendarEvent) time.Duration {
	if event.End.After(event.Start) {
		return event.End.Sub(event.Start)
	}
	if event.AllDay {
		return 24 * time.Hour
	}
	return time.Nanosecond
}

func recurrenceBeforeUntil(occurrence, until time.Time) bool {
	return until.IsZero() || !occurrence.After(until)
}

func recurrenceOccurrenceEvent(event models.CalendarEvent, occurrenceStart time.Time, duration time.Duration) models.CalendarEvent {
	occurrence := event
	occurrence.Start = occurrenceStart
	occurrence.End = occurrenceStart.Add(duration)
	occurrence.Ref.InstanceID = occurrenceStart.UTC().Format("20060102T150405Z")
	occurrence.Ref.LocalID = ""
	occurrence.Ref = occurrence.Ref.WithDefaults()
	return occurrence
}

func recurrenceExclusionSet(event models.CalendarEvent) map[string]bool {
	out := make(map[string]bool)
	for _, recurrence := range event.Recurrence {
		name, value, ok := strings.Cut(recurrence, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "EXDATE") {
			continue
		}
		for _, part := range strings.Split(value, ",") {
			exdate, _, err := parseICSTimeWithZone(part, event.TimeZone)
			if err == nil {
				out[recurrenceOccurrenceKey(event, exdate)] = true
			}
		}
	}
	return out
}

func recurrenceRDates(event models.CalendarEvent) []time.Time {
	var out []time.Time
	for _, recurrence := range event.Recurrence {
		name, value, ok := strings.Cut(recurrence, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "RDATE") {
			continue
		}
		for _, part := range strings.Split(value, ",") {
			rdate, _, err := parseICSTimeWithZone(part, event.TimeZone)
			if err == nil {
				out = append(out, rdate)
			}
		}
	}
	return out
}

func recurrenceDateExcluded(event models.CalendarEvent, excluded map[string]bool, occurrence time.Time) bool {
	return excluded[recurrenceOccurrenceKey(event, occurrence)]
}

func recurrenceOccurrenceKey(event models.CalendarEvent, value time.Time) string {
	loc := event.Start.Location()
	if loc == nil {
		loc = time.Local
	}
	if event.AllDay {
		return value.In(loc).Format("20060102")
	}
	return value.In(loc).Format("20060102T150405")
}

func eventOverlapsRange(event models.CalendarEvent, start, end time.Time) bool {
	eventEnd := event.End
	if eventEnd.IsZero() || !eventEnd.After(event.Start) {
		eventEnd = event.Start.Add(eventDuration(event))
	}
	return event.Start.Before(end) && eventEnd.After(start)
}
