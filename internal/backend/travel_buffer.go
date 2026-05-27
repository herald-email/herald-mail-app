package backend

import (
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// CalendarTravelBufferBackend is a read-only command-center capability
// assembled from existing cache-backed cross-source search results.
type CalendarTravelBufferBackend interface {
	BuildCalendarTravelBuffer(event models.CalendarEvent) (models.CalendarTravelBuffer, error)
}

var _ CalendarTravelBufferBackend = (*DemoBackend)(nil)
var _ CalendarTravelBufferBackend = (*LocalBackend)(nil)
var _ CalendarTravelBufferBackend = (*MultiBackend)(nil)

func (d *DemoBackend) BuildCalendarTravelBuffer(event models.CalendarEvent) (models.CalendarTravelBuffer, error) {
	return buildCalendarTravelBuffer(d, event)
}

func (b *LocalBackend) BuildCalendarTravelBuffer(event models.CalendarEvent) (models.CalendarTravelBuffer, error) {
	return buildCalendarTravelBuffer(b, event)
}

func (m *MultiBackend) BuildCalendarTravelBuffer(event models.CalendarEvent) (models.CalendarTravelBuffer, error) {
	return buildCalendarTravelBuffer(m, event)
}

func buildCalendarTravelBuffer(search CrossSourceSearchBackend, event models.CalendarEvent) (models.CalendarTravelBuffer, error) {
	event.Ref = event.Ref.WithDefaults()
	terms := models.CalendarTravelBufferQueries(event)
	buffer := models.CalendarTravelBuffer{
		Event:       event,
		QueryTerms:  terms,
		GeneratedAt: time.Now().UTC(),
	}
	if search == nil {
		buffer.Recommendations = models.CalendarTravelBufferRecommendations(event, nil, nil)
		return buffer, nil
	}

	seenMail := make(map[string]struct{})
	seenEvents := make(map[string]struct{})
	selectedLocalID := event.Ref.WithDefaults().LocalID
	for _, query := range terms {
		results, err := search.CrossSourceSearch(query)
		if err != nil {
			return buffer, err
		}
		for _, result := range results {
			switch result.Kind {
			case models.CrossSourceResultMail:
				if result.Email == nil {
					continue
				}
				if !models.EmailLooksTravelRelated(result.Email) && !travelQueryMatchesEmail(query, result.Email) {
					continue
				}
				key := result.Email.MessageRef().WithDefaults().LocalID
				if _, ok := seenMail[key]; ok {
					continue
				}
				seenMail[key] = struct{}{}
				buffer.RelatedMail = append(buffer.RelatedMail, models.CloneEmailData(result.Email))
			case models.CrossSourceResultEvent:
				if result.Event == nil {
					continue
				}
				related := *result.Event
				related.Ref = related.Ref.WithDefaults()
				if related.Ref.LocalID == selectedLocalID {
					continue
				}
				if _, ok := seenEvents[related.Ref.LocalID]; ok {
					continue
				}
				seenEvents[related.Ref.LocalID] = struct{}{}
				buffer.NearbyEvents = append(buffer.NearbyEvents, related)
			}
		}
	}
	if len(buffer.RelatedMail) > 6 {
		buffer.RelatedMail = buffer.RelatedMail[:6]
	}
	if len(buffer.NearbyEvents) > 4 {
		buffer.NearbyEvents = buffer.NearbyEvents[:4]
	}
	buffer.Recommendations = models.CalendarTravelBufferRecommendations(event, buffer.RelatedMail, buffer.NearbyEvents)
	return buffer, nil
}

func travelQueryMatchesEmail(query string, email *models.EmailData) bool {
	if email == nil {
		return false
	}
	query = stringsTrimLower(query)
	if query == "" {
		return false
	}
	text := stringsTrimLower(email.Sender + " " + email.Subject + " " + email.Folder)
	return text != "" && strings.Contains(text, query)
}

func stringsTrimLower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
