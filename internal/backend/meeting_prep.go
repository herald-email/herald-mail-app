package backend

import (
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// CalendarMeetingPrepBackend is a read-only command-center capability. It is
// assembled from existing cache-backed cross-source search results.
type CalendarMeetingPrepBackend interface {
	BuildCalendarMeetingPrep(event models.CalendarEvent) (models.CalendarMeetingPrep, error)
}

var _ CalendarMeetingPrepBackend = (*DemoBackend)(nil)
var _ CalendarMeetingPrepBackend = (*LocalBackend)(nil)
var _ CalendarMeetingPrepBackend = (*MultiBackend)(nil)

func (d *DemoBackend) BuildCalendarMeetingPrep(event models.CalendarEvent) (models.CalendarMeetingPrep, error) {
	return buildCalendarMeetingPrep(d, event)
}

func (b *LocalBackend) BuildCalendarMeetingPrep(event models.CalendarEvent) (models.CalendarMeetingPrep, error) {
	return buildCalendarMeetingPrep(b, event)
}

func (m *MultiBackend) BuildCalendarMeetingPrep(event models.CalendarEvent) (models.CalendarMeetingPrep, error) {
	return buildCalendarMeetingPrep(m, event)
}

func buildCalendarMeetingPrep(search CrossSourceSearchBackend, event models.CalendarEvent) (models.CalendarMeetingPrep, error) {
	event.Ref = event.Ref.WithDefaults()
	terms := models.CalendarMeetingPrepQueries(event)
	prep := models.CalendarMeetingPrep{
		Event:       event,
		QueryTerms:  terms,
		GeneratedAt: time.Now().UTC(),
	}
	if search == nil {
		return prep, nil
	}

	seenMail := make(map[string]struct{})
	seenEvents := make(map[string]struct{})
	selectedLocalID := event.Ref.WithDefaults().LocalID
	for _, query := range terms {
		results, err := search.CrossSourceSearch(query)
		if err != nil {
			return prep, err
		}
		for _, result := range results {
			switch result.Kind {
			case models.CrossSourceResultMail:
				if result.Email == nil {
					continue
				}
				key := result.Email.MessageRef().WithDefaults().LocalID
				if _, ok := seenMail[key]; ok {
					continue
				}
				seenMail[key] = struct{}{}
				prep.RelatedMail = append(prep.RelatedMail, models.CloneEmailData(result.Email))
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
				prep.RelatedEvents = append(prep.RelatedEvents, related)
			}
		}
	}
	if len(prep.RelatedMail) > 6 {
		prep.RelatedMail = prep.RelatedMail[:6]
	}
	if len(prep.RelatedEvents) > 4 {
		prep.RelatedEvents = prep.RelatedEvents[:4]
	}
	return prep, nil
}
