package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	CalendarAISummaryGeneratedByAI       = "ai"
	CalendarAISummaryGeneratedByFallback = "cached fallback"
)

// CalendarAISummary is a read-only command-center summary assembled from
// cached mail and calendar rows for one selected event.
type CalendarAISummary struct {
	Event        CalendarEvent
	QueryTerms   []string
	RelatedMail  []*EmailData
	NearbyEvents []CalendarEvent
	Bullets      []string
	ActionItems  []string
	GeneratedBy  string
	AINote       string
	GeneratedAt  time.Time
}

func CalendarAISummaryQueries(event CalendarEvent) []string {
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

func CalendarAISummaryFallback(event CalendarEvent, mail []*EmailData, nearby []CalendarEvent, queryTerms []string, note string) CalendarAISummary {
	event.Ref = event.Ref.WithDefaults()
	summary := CalendarAISummary{
		Event:        event,
		QueryTerms:   dedupeSummaryTerms(queryTerms),
		RelatedMail:  cloneSummaryMail(mail),
		NearbyEvents: cloneSummaryEvents(nearby),
		GeneratedBy:  CalendarAISummaryGeneratedByFallback,
		AINote:       strings.TrimSpace(note),
		GeneratedAt:  time.Now().UTC(),
	}
	title := strings.TrimSpace(event.Title)
	if title == "" {
		title = "Selected event"
	}
	summary.Bullets = append(summary.Bullets,
		fmt.Sprintf("%s has %s and %s.", title, summaryCountPhrase(len(mail), "cached related mail", "cached related mail messages"), summaryCountPhrase(len(nearby), "nearby event", "nearby events")),
	)
	if !event.Start.IsZero() {
		summary.Bullets = append(summary.Bullets, fmt.Sprintf("Event time: %s.", event.Start.Local().Format("Mon Jan 2 15:04")))
	}
	if first := firstSummaryMail(mail); first != nil {
		subject := strings.TrimSpace(first.Subject)
		if subject == "" {
			subject = "related cached mail"
		}
		summary.Bullets = append(summary.Bullets, fmt.Sprintf("Most relevant mail: %s from %s.", subject, strings.TrimSpace(first.Sender)))
		summary.ActionItems = append(summary.ActionItems, fmt.Sprintf("Review %s before the event.", subject))
	}
	if len(nearby) > 0 {
		name := strings.TrimSpace(nearby[0].Title)
		if name == "" {
			name = "nearby cached event"
		}
		summary.ActionItems = append(summary.ActionItems, fmt.Sprintf("Check schedule impact around %s.", name))
	}
	if len(summary.ActionItems) == 0 {
		summary.ActionItems = append(summary.ActionItems, "Review cached event details before the event.")
	}
	return summary
}

func ParseCalendarAISummaryAIResponse(raw string) (bullets []string, actions []string) {
	section := "summary"
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(strings.TrimRight(line, ":"))
		switch lower {
		case "summary", "summary bullets", "bullets":
			section = "summary"
			continue
		case "actions", "action", "action items", "next steps":
			section = "actions"
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "-"), "*"))
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if section == "actions" {
			actions = append(actions, line)
		} else {
			bullets = append(bullets, line)
		}
	}
	return bullets, actions
}

func dedupeSummaryTerms(terms []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, term)
	}
	return out
}

func cloneSummaryMail(mail []*EmailData) []*EmailData {
	out := make([]*EmailData, 0, len(mail))
	for _, email := range mail {
		if email == nil {
			continue
		}
		out = append(out, CloneEmailData(email))
	}
	return out
}

func cloneSummaryEvents(events []CalendarEvent) []CalendarEvent {
	out := make([]CalendarEvent, 0, len(events))
	for _, event := range events {
		event.Ref = event.Ref.WithDefaults()
		out = append(out, event)
	}
	return out
}

func firstSummaryMail(mail []*EmailData) *EmailData {
	for _, email := range mail {
		if email != nil {
			return email
		}
	}
	return nil
}

func summaryCountPhrase(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}
