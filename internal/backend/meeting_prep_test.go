package backend

import (
	"reflect"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type fakeMeetingPrepSearch struct {
	resultsByQuery map[string][]models.CrossSourceSearchResult
	queries        []string
}

func (f *fakeMeetingPrepSearch) CrossSourceSearch(query string) ([]models.CrossSourceSearchResult, error) {
	f.queries = append(f.queries, query)
	return f.resultsByQuery[query], nil
}

func TestBuildCalendarMeetingPrepUsesCacheBackedCrossSourceSearch(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref:   models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "launch"},
		Title: "Launch planning",
		Start: start,
		End:   start.Add(time.Hour),
		Attendees: []models.CalendarAttendee{
			{Name: "Mina Park", Email: "mina@example.com"},
		},
	}
	event.Ref = event.Ref.WithDefaults()
	mail := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "mail-planning",
		Folder:    "INBOX",
		Sender:    "mina@example.com",
		Subject:   "Launch planning memo",
		Date:      start.Add(-time.Hour),
	}
	nearby := event
	nearby.Ref.EventID = "launch-followup"
	nearby.Ref.LocalID = ""
	nearby.Ref = nearby.Ref.WithDefaults()
	nearby.Title = "Launch follow-up"
	nearby.Start = start.Add(2 * time.Hour)
	otherAccountValue := *mail
	otherAccount := &otherAccountValue
	otherAccount.SourceID = "personal-mail"
	otherAccount.AccountID = "personal"
	otherAccount.MessageID = "mail-personal"
	search := &fakeMeetingPrepSearch{resultsByQuery: map[string][]models.CrossSourceSearchResult{
		"Launch planning": {
			models.NewMailCrossSourceSearchResult(mail, "Launch planning"),
			models.NewEventCrossSourceSearchResult(event, "Launch planning"),
			models.NewEventCrossSourceSearchResult(nearby, "Launch planning"),
		},
		"mina@example.com": {
			models.NewMailCrossSourceSearchResult(mail, "mina@example.com"),
			models.NewMailCrossSourceSearchResult(otherAccount, "mina@example.com"),
		},
	}}

	prep, err := buildCalendarMeetingPrep(search, event)
	if err != nil {
		t.Fatalf("buildCalendarMeetingPrep: %v", err)
	}
	if got, want := search.queries[:2], []string{"Launch planning", "Mina Park"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first queries = %#v, want %#v", got, want)
	}
	if prep.Event.Ref.LocalID != event.Ref.LocalID {
		t.Fatalf("prep event ref = %#v, want selected event", prep.Event.Ref)
	}
	if len(prep.RelatedMail) != 2 {
		t.Fatalf("related mail = %#v, want deduped scoped mail from two accounts", prep.RelatedMail)
	}
	if len(prep.RelatedEvents) != 1 || prep.RelatedEvents[0].Ref.WithDefaults().LocalID != nearby.Ref.LocalID {
		t.Fatalf("related events = %#v, want nearby event only", prep.RelatedEvents)
	}
}
