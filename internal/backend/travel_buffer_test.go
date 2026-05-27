package backend

import (
	"reflect"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestBuildCalendarTravelBufferUsesCachedCrossSourceContext(t *testing.T) {
	start := time.Date(2026, 5, 24, 14, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref:      models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "onsite"},
		Title:    "Partner onsite",
		Location: "SFO Terminal 2",
		Start:    start,
		End:      start.Add(time.Hour),
		Attendees: []models.CalendarAttendee{
			{Name: "Mina Park", Email: "mina@example.com"},
		},
	}
	event.Ref = event.Ref.WithDefaults()
	mail := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "flight-itinerary",
		Folder:    "Travel",
		Sender:    "airline@example.com",
		Subject:   "Flight itinerary for SFO",
		Date:      start.Add(-24 * time.Hour),
	}
	nearby := event
	nearby.Ref.EventID = "team-sync"
	nearby.Ref.LocalID = ""
	nearby.Ref = nearby.Ref.WithDefaults()
	nearby.Title = "Team sync"
	nearby.Location = "Downtown office"
	nearby.Start = start.Add(-45 * time.Minute)
	nearby.End = start.Add(-15 * time.Minute)
	otherAccountValue := *mail
	otherAccount := &otherAccountValue
	otherAccount.SourceID = "personal-mail"
	otherAccount.AccountID = "personal"
	otherAccount.MessageID = "hotel-confirmation"
	otherAccount.Subject = "Hotel confirmation near SFO"
	search := &fakeMeetingPrepSearch{resultsByQuery: map[string][]models.CrossSourceSearchResult{
		"Partner onsite": {
			models.NewMailCrossSourceSearchResult(mail, "Partner onsite"),
			models.NewEventCrossSourceSearchResult(event, "Partner onsite"),
			models.NewEventCrossSourceSearchResult(nearby, "Partner onsite"),
		},
		"flight": {
			models.NewMailCrossSourceSearchResult(mail, "flight"),
			models.NewMailCrossSourceSearchResult(otherAccount, "flight"),
		},
	}}

	buffer, err := buildCalendarTravelBuffer(search, event)
	if err != nil {
		t.Fatalf("buildCalendarTravelBuffer: %v", err)
	}
	if got, want := search.queries[:2], []string{"Partner onsite", "SFO Terminal 2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first queries = %#v, want %#v", got, want)
	}
	if buffer.Event.Ref.LocalID != event.Ref.LocalID {
		t.Fatalf("buffer event ref = %#v, want selected event", buffer.Event.Ref)
	}
	if len(buffer.RelatedMail) != 2 {
		t.Fatalf("related mail = %#v, want deduped scoped travel mail from two accounts", buffer.RelatedMail)
	}
	if len(buffer.NearbyEvents) != 1 || buffer.NearbyEvents[0].Ref.WithDefaults().LocalID != nearby.Ref.LocalID {
		t.Fatalf("nearby events = %#v, want nearby event only", buffer.NearbyEvents)
	}
	if len(buffer.Recommendations) == 0 {
		t.Fatalf("recommendations = %#v, want cache-backed travel suggestions", buffer.Recommendations)
	}
}
