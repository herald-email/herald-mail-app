package backend

import (
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// CrossSourceSearchBackend is an additive cache-backed search capability. It
// intentionally stays outside the legacy Backend interface so daemon/MCP wire
// shapes do not need to change for this TUI slice.
type CrossSourceSearchBackend interface {
	CrossSourceSearch(query string) ([]models.CrossSourceSearchResult, error)
}

var _ CrossSourceSearchBackend = (*DemoBackend)(nil)
var _ CrossSourceSearchBackend = (*LocalBackend)(nil)
var _ CrossSourceSearchBackend = (*MultiBackend)(nil)

func (d *DemoBackend) CrossSourceSearch(query string) ([]models.CrossSourceSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	emails, err := d.SearchEmailsCrossFolder(query)
	if err != nil {
		return nil, err
	}
	events, err := d.SearchCalendarEvents(query, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	return buildCrossSourceSearchResults(emails, events, query), nil
}

func (b *LocalBackend) CrossSourceSearch(query string) ([]models.CrossSourceSearchResult, error) {
	if b == nil || b.cache == nil || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	emails, err := b.SearchEmailsCrossFolder(query)
	if err != nil {
		return nil, err
	}
	var events []models.CalendarEvent
	if b.CalendarAgendaAvailable() {
		events, err = b.SearchCalendarEvents(query, time.Time{}, time.Time{})
		if err != nil {
			return nil, err
		}
	}
	return buildCrossSourceSearchResults(emails, events, query), nil
}

func (m *MultiBackend) CrossSourceSearch(query string) ([]models.CrossSourceSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if !m.allAccountsActive() {
		active := m.activeBackend()
		if active == nil {
			return nil, nil
		}
		return crossSourceSearchFromBackend(active, query)
	}
	var out []models.CrossSourceSearchResult
	for _, slot := range m.snapshotSlots() {
		results, err := crossSourceSearchFromBackend(slot.backend, query)
		if err != nil {
			return out, err
		}
		for _, result := range results {
			out = append(out, scopeCrossSourceResultForSlot(slot, result))
		}
	}
	sortCrossSourceSearchResults(out)
	return out, nil
}

func crossSourceSearchFromBackend(b Backend, query string) ([]models.CrossSourceSearchResult, error) {
	if search, ok := b.(CrossSourceSearchBackend); ok {
		return search.CrossSourceSearch(query)
	}
	emails, err := b.SearchEmailsCrossFolder(query)
	if err != nil {
		return nil, err
	}
	var events []models.CalendarEvent
	if agenda, ok := b.(CalendarAgendaBackend); ok && agenda.CalendarAgendaAvailable() {
		events, err = agenda.SearchCalendarEvents(query, time.Time{}, time.Time{})
		if err != nil {
			return nil, err
		}
	}
	return buildCrossSourceSearchResults(emails, events, query), nil
}

func buildCrossSourceSearchResults(emails []*models.EmailData, events []models.CalendarEvent, query string) []models.CrossSourceSearchResult {
	results := make([]models.CrossSourceSearchResult, 0, len(emails)+len(events))
	for _, email := range emails {
		if email == nil {
			continue
		}
		results = append(results, models.NewMailCrossSourceSearchResult(email, query))
	}
	for _, event := range events {
		results = append(results, models.NewEventCrossSourceSearchResult(event, query))
	}
	sortCrossSourceSearchResults(results)
	return results
}

func sortCrossSourceSearchResults(results []models.CrossSourceSearchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		a := results[i]
		b := results[j]
		if !a.When.Equal(b.When) {
			if a.When.IsZero() {
				return false
			}
			if b.When.IsZero() {
				return true
			}
			return a.When.After(b.When)
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return crossSourceResultTitle(a) < crossSourceResultTitle(b)
	})
}

func scopeCrossSourceResultForSlot(slot *accountSlot, result models.CrossSourceSearchResult) models.CrossSourceSearchResult {
	if slot == nil {
		return result
	}
	if result.Email != nil {
		result.Email = emailForAccountSlot(slot, result.Email)
		result.When = result.Email.Date
	}
	if result.Event != nil {
		event := *result.Event
		event.Ref = event.Ref.WithDefaults()
		if event.Ref.AccountID == "" || event.Ref.AccountID == models.DefaultAccountID {
			event.Ref.AccountID = slot.info.AccountID
			event.Ref.LocalID = ""
			event.Ref = event.Ref.WithDefaults()
		}
		result.Event = &event
		result.When = event.Start
	}
	return result
}

func crossSourceResultTitle(result models.CrossSourceSearchResult) string {
	switch result.Kind {
	case models.CrossSourceResultMail:
		if result.Email != nil {
			return result.Email.Subject
		}
	case models.CrossSourceResultEvent:
		if result.Event != nil {
			return result.Event.Title
		}
	}
	return ""
}
