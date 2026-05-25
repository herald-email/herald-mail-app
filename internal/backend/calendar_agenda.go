package backend

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// CalendarAgendaBackend is an additive read-only calendar surface used by the
// TUI. Legacy mail backends do not need to implement it.
type CalendarAgendaBackend interface {
	CalendarAgendaAvailable() bool
	ListCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error)
	SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error)
	GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error)
}

func (d *DemoBackend) CalendarAgendaAvailable() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calendarEvents) > 0
}

func (d *DemoBackend) ListCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]models.CalendarEvent, 0, len(d.calendarEvents))
	for _, event := range d.calendarEvents {
		event.Ref = event.Ref.WithDefaults()
		if calendarEventInRange(event, start, end) {
			out = append(out, event)
		}
	}
	sortCalendarEvents(out)
	return out, nil
}

func (d *DemoBackend) GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ref = ref.WithDefaults()
	for _, event := range d.calendarEvents {
		event.Ref = event.Ref.WithDefaults()
		if event.Ref.LocalID == ref.LocalID {
			got := event
			return &got, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (d *DemoBackend) SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]models.CalendarEvent, 0, len(d.calendarEvents))
	for _, event := range d.calendarEvents {
		event.Ref = event.Ref.WithDefaults()
		if calendarEventInRange(event, start, end) && models.CalendarEventMatchesQuery(event, query) {
			out = append(out, event)
		}
	}
	sortCalendarEvents(out)
	return out, nil
}

func (b *LocalBackend) CalendarAgendaAvailable() bool {
	return b != nil && b.cache != nil && len(b.configuredCalendarSources()) > 0
}

func (b *LocalBackend) ListCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, nil
	}
	var out []models.CalendarEvent
	for _, source := range b.configuredCalendarSources() {
		events, err := b.cache.ListCalendarAgendaEvents(models.SourceID(source.ID), models.AccountID(source.AccountID), start, end)
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	sortCalendarEvents(out)
	return out, nil
}

func (b *LocalBackend) GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, sql.ErrNoRows
	}
	return b.cache.GetCalendarEventByRef(ref)
}

func (b *LocalBackend) SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, nil
	}
	var out []models.CalendarEvent
	for _, source := range b.configuredCalendarSources() {
		events, err := b.cache.SearchCalendarEvents(models.SourceID(source.ID), models.AccountID(source.AccountID), query, start, end)
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	sortCalendarEvents(out)
	return out, nil
}

func (b *LocalBackend) configuredCalendarSources() []config.SourceConfig {
	if b == nil || b.cfg == nil {
		return nil
	}
	var sources []config.SourceConfig
	for _, source := range b.cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) == string(models.SourceKindCalendar) {
			sources = append(sources, source)
		}
	}
	return sources
}

func (m *MultiBackend) CalendarAgendaAvailable() bool {
	for _, backend := range m.calendarAgendaBackends() {
		if backend.CalendarAgendaAvailable() {
			return true
		}
	}
	return false
}

func (m *MultiBackend) ListCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	var out []models.CalendarEvent
	for _, backend := range m.calendarAgendaBackends() {
		if !backend.CalendarAgendaAvailable() {
			continue
		}
		events, err := backend.ListCalendarAgenda(start, end)
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	sortCalendarEvents(out)
	return out, nil
}

func (m *MultiBackend) GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error) {
	var lastErr error = sql.ErrNoRows
	for _, backend := range m.calendarAgendaBackends() {
		event, err := backend.GetCalendarEvent(ref)
		if err == nil {
			return event, nil
		}
		lastErr = err
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (m *MultiBackend) SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error) {
	var out []models.CalendarEvent
	for _, backend := range m.calendarAgendaBackends() {
		if !backend.CalendarAgendaAvailable() {
			continue
		}
		events, err := backend.SearchCalendarEvents(query, start, end)
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	sortCalendarEvents(out)
	return out, nil
}

func (m *MultiBackend) calendarAgendaBackends() []CalendarAgendaBackend {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	active := m.active
	if active == AllAccountsSourceID {
		slots := make([]*accountSlot, 0, len(m.order))
		for _, id := range m.order {
			slots = append(slots, m.slots[id])
		}
		m.mu.RUnlock()
		out := make([]CalendarAgendaBackend, 0, len(slots))
		for _, slot := range slots {
			if backend, ok := slot.backend.(CalendarAgendaBackend); ok {
				out = append(out, backend)
			}
		}
		return out
	}
	slot := m.slots[active]
	m.mu.RUnlock()
	if slot == nil {
		return nil
	}
	if backend, ok := slot.backend.(CalendarAgendaBackend); ok {
		return []CalendarAgendaBackend{backend}
	}
	return nil
}

func calendarEventInRange(event models.CalendarEvent, start, end time.Time) bool {
	if start.IsZero() && end.IsZero() {
		return true
	}
	eventStart := event.Start
	eventEnd := event.End
	if eventEnd.IsZero() {
		eventEnd = eventStart
	}
	if !end.IsZero() && !eventStart.IsZero() && !eventStart.Before(end) {
		return false
	}
	if !start.IsZero() && !eventEnd.IsZero() && !eventEnd.After(start) {
		return false
	}
	return true
}

func sortCalendarEvents(events []models.CalendarEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].Start.Equal(events[j].Start) {
			return events[i].Start.Before(events[j].Start)
		}
		if events[i].Title != events[j].Title {
			return events[i].Title < events[j].Title
		}
		return events[i].Ref.LocalID < events[j].Ref.LocalID
	})
}
