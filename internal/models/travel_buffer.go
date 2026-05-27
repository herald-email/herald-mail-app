package models

import (
	"fmt"
	"strings"
	"time"
)

// CalendarTravelBuffer is a read-only command-center summary assembled from
// cached mail and calendar rows for travel-aware planning around one event.
type CalendarTravelBuffer struct {
	Event           CalendarEvent
	QueryTerms      []string
	RelatedMail     []*EmailData
	NearbyEvents    []CalendarEvent
	Recommendations []CalendarTravelBufferRecommendation
	GeneratedAt     time.Time
}

type CalendarTravelBufferRecommendation struct {
	Label  string
	Window string
	Reason string
}

func CalendarTravelBufferQueries(event CalendarEvent) []string {
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
	add(event.Location)
	add(event.Organizer)
	add(event.OrganizerEmail)
	for _, attendee := range event.Attendees {
		add(attendee.Name)
		add(attendee.Email)
	}
	add("flight")
	add("airport")
	add("train")
	add("hotel")
	add("travel")
	return out
}

func CalendarTravelBufferRecommendations(event CalendarEvent, mail []*EmailData, nearby []CalendarEvent) []CalendarTravelBufferRecommendation {
	var recs []CalendarTravelBufferRecommendation
	if travel := firstTravelMailSignal(mail); travel != nil {
		recs = append(recs, CalendarTravelBufferRecommendation{
			Label:  "Arrive early",
			Window: "90 min before",
			Reason: strings.TrimSpace(travel.Subject),
		})
	} else if strings.TrimSpace(event.Location) != "" {
		recs = append(recs, CalendarTravelBufferRecommendation{
			Label:  "Hold arrival buffer",
			Window: "30 min before",
			Reason: "Location set for " + strings.TrimSpace(event.Location),
		})
	}

	for _, related := range nearby {
		if event.Start.IsZero() || related.End.IsZero() {
			continue
		}
		gap := event.Start.Sub(related.End)
		if gap < 0 || gap > 60*time.Minute {
			continue
		}
		label := "Tight nearby gap"
		window := fmt.Sprintf("%d min", int(gap.Minutes()))
		reason := strings.TrimSpace(related.Title)
		if reason == "" {
			reason = "Nearby cached event"
		}
		if strings.TrimSpace(related.Location) != "" && strings.TrimSpace(event.Location) != "" && !strings.EqualFold(strings.TrimSpace(related.Location), strings.TrimSpace(event.Location)) {
			reason += " ends at " + strings.TrimSpace(related.Location)
		}
		recs = append(recs, CalendarTravelBufferRecommendation{Label: label, Window: window, Reason: reason})
	}
	return recs
}

func firstTravelMailSignal(mail []*EmailData) *EmailData {
	for _, email := range mail {
		if email == nil {
			continue
		}
		if EmailLooksTravelRelated(email) {
			return email
		}
	}
	return nil
}

func EmailLooksTravelRelated(email *EmailData) bool {
	if email == nil {
		return false
	}
	text := strings.ToLower(strings.Join([]string{email.Sender, email.Subject, email.Folder}, " "))
	for _, token := range []string{
		"flight",
		"airport",
		"boarding",
		"itinerary",
		"train",
		"hotel",
		"reservation",
		"rideshare",
		"ride",
		"travel",
		"terminal",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}
