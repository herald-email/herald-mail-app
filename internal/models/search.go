package models

import (
	"strings"
	"time"
)

type CrossSourceResultKind string

const (
	CrossSourceResultMail  CrossSourceResultKind = "mail"
	CrossSourceResultEvent CrossSourceResultKind = "event"
)

type CrossSourceSearchResult struct {
	Kind      CrossSourceResultKind
	Email     *EmailData
	Event     *CalendarEvent
	When      time.Time
	MatchHint string
}

func NewMailCrossSourceSearchResult(email *EmailData, query string) CrossSourceSearchResult {
	var when time.Time
	if email != nil {
		when = email.Date
	}
	return CrossSourceSearchResult{
		Kind:      CrossSourceResultMail,
		Email:     CloneEmailData(email),
		When:      when,
		MatchHint: EmailSearchMatchHint(email, query),
	}
}

func NewEventCrossSourceSearchResult(event CalendarEvent, query string) CrossSourceSearchResult {
	event.Ref = event.Ref.WithDefaults()
	eventCopy := event
	return CrossSourceSearchResult{
		Kind:      CrossSourceResultEvent,
		Event:     &eventCopy,
		When:      event.Start,
		MatchHint: CalendarEventSearchMatchHint(event, query),
	}
}

func CloneEmailData(email *EmailData) *EmailData {
	if email == nil {
		return nil
	}
	clone := *email
	return &clone
}

func EmailSearchMatchHint(email *EmailData, query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if email == nil || query == "" {
		return ""
	}
	contains := func(value string) bool {
		return strings.Contains(strings.ToLower(value), query)
	}
	if contains(email.Subject) {
		return "subject"
	}
	if contains(email.Sender) {
		return "sender"
	}
	if contains(email.Folder) {
		return "folder"
	}
	if contains(string(email.SourceID)) || contains(string(email.AccountID)) {
		return "account"
	}
	return "body"
}

func CalendarEventSearchMatchHint(event CalendarEvent, query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ""
	}
	fieldContains := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), query) {
				return true
			}
		}
		return false
	}
	if fieldContains(event.Title) {
		return "title"
	}
	if fieldContains(event.Location) {
		return "location"
	}
	if fieldContains(event.Organizer, event.OrganizerEmail) {
		return "organizer"
	}
	for _, attendee := range event.Attendees {
		if fieldContains(attendee.Name, attendee.Email, attendee.RSVP) {
			return "attendee"
		}
	}
	if fieldContains(event.Description) {
		return "notes"
	}
	if fieldContains(event.RecurrenceSummary) || fieldContains(event.Recurrence...) {
		return "recurrence"
	}
	for _, attachment := range event.Attachments {
		if fieldContains(attachment.Title, attachment.MIMEType) {
			return "attachment"
		}
	}
	ref := event.Ref.WithDefaults()
	if fieldContains(string(ref.SourceID), string(ref.AccountID), ref.CalendarID) {
		return "calendar"
	}
	return ""
}
