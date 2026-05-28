package backend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/calendar"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	calendarAgendaSyncTTL     = 5 * time.Minute
	calendarAgendaSyncTimeout = 45 * time.Second
)

// CalendarAgendaBackend is an additive read-only calendar surface used by the
// TUI. Legacy mail backends do not need to implement it.
type CalendarAgendaBackend interface {
	CalendarAgendaAvailable() bool
	ListCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error)
	SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error)
	GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error)
}

// CalendarCollectionBackend exposes user-facing calendar lists for the TUI rail.
type CalendarCollectionBackend interface {
	ListCalendarCollections() ([]models.CalendarCollection, error)
}

// CalendarEventMutationBackend is the local/cache-backed calendar edit
// boundary. Live provider mutation adapters stay behind this interface until a
// later provider-write stage enables them explicitly.
type CalendarEventMutationBackend interface {
	SaveCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error)
	RespondCalendarEvent(ref models.EventRef, status string) (*models.CalendarEvent, error)
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

func (d *DemoBackend) ListCalendarCollections() ([]models.CalendarCollection, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]models.CalendarCollection, len(d.calendarCollections))
	copy(out, d.calendarCollections)
	sortCalendarCollections(out)
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

func (d *DemoBackend) SaveCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	event.Ref = event.Ref.WithDefaults()
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	for i := range d.calendarEvents {
		if d.calendarEvents[i].Ref.WithDefaults().LocalID == event.Ref.LocalID {
			d.calendarEvents[i] = event
			saved := event
			return &saved, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (d *DemoBackend) RespondCalendarEvent(ref models.EventRef, status string) (*models.CalendarEvent, error) {
	normalized, err := models.NormalizeCalendarRSVP(status)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	ref = ref.WithDefaults()
	for i := range d.calendarEvents {
		if d.calendarEvents[i].Ref.WithDefaults().LocalID != ref.LocalID {
			continue
		}
		if len(d.calendarEvents[i].Attendees) == 0 {
			return nil, fmt.Errorf("calendar event has no attendee response to update")
		}
		d.calendarEvents[i].Attendees[0].RSVP = normalized
		d.calendarEvents[i].UpdatedAt = time.Now().UTC()
		saved := d.calendarEvents[i]
		return &saved, nil
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
	cached, err := b.listCachedCalendarAgenda(start, end)
	if err != nil {
		return nil, err
	}
	if err := b.syncConfiguredCalendarSources(context.Background()); err != nil {
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	return b.listCachedCalendarAgenda(start, end)
}

func (b *LocalBackend) listCachedCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
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

func (b *LocalBackend) ListCalendarCollections() ([]models.CalendarCollection, error) {
	if b == nil || b.cache == nil {
		return nil, nil
	}
	cached, err := b.listCachedCalendarCollections()
	if err != nil {
		return nil, err
	}
	if err := b.syncConfiguredCalendarSources(context.Background()); err != nil {
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	return b.listCachedCalendarCollections()
}

func (b *LocalBackend) listCachedCalendarCollections() ([]models.CalendarCollection, error) {
	var out []models.CalendarCollection
	for _, source := range b.configuredCalendarSources() {
		collections, err := b.cache.ListCalendarCollections(models.SourceID(source.ID), models.AccountID(source.AccountID))
		if err != nil {
			return nil, err
		}
		out = append(out, collections...)
	}
	sortCalendarCollections(out)
	return out, nil
}

func (b *LocalBackend) GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, sql.ErrNoRows
	}
	return b.cache.GetCalendarEventByRef(ref)
}

func (b *LocalBackend) SaveCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, sql.ErrNoRows
	}
	event.Ref = event.Ref.WithDefaults()
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	if source, ok, err := b.calendarMutationSourceForRef(event.Ref); err != nil {
		return nil, err
	} else if ok {
		saved, err := source.UpdateEvent(context.Background(), event, models.CalendarMutationOptions{
			RecurrenceScope: models.CalendarMutationScopeThisEvent,
			IfMatch:         event.Ref.ETag,
		})
		if err != nil {
			return nil, calendarProviderMutationError("save", err)
		}
		if err := b.cache.PutCalendarEvent(*saved); err != nil {
			return nil, err
		}
		return b.cache.GetCalendarEventByRef(saved.Ref)
	}
	if err := b.cache.PutCalendarEvent(event); err != nil {
		return nil, err
	}
	return b.cache.GetCalendarEventByRef(event.Ref)
}

func (b *LocalBackend) RespondCalendarEvent(ref models.EventRef, status string) (*models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, sql.ErrNoRows
	}
	ref = ref.WithDefaults()
	source, ok, err := b.calendarMutationSourceForRef(ref)
	if err != nil {
		return nil, err
	}
	if ok {
		saved, err := source.RespondToEvent(context.Background(), ref, status, models.CalendarMutationOptions{
			RecurrenceScope: models.CalendarMutationScopeThisEvent,
			IfMatch:         ref.ETag,
		})
		if err != nil {
			return nil, calendarProviderMutationError("RSVP", err)
		}
		if err := b.cache.PutCalendarEvent(*saved); err != nil {
			return nil, err
		}
		return b.cache.GetCalendarEventByRef(saved.Ref)
	}
	event, err := b.cache.GetCalendarEventByRef(ref)
	if err != nil {
		return nil, err
	}
	normalized, err := models.NormalizeCalendarRSVP(status)
	if err != nil {
		return nil, err
	}
	if len(event.Attendees) == 0 {
		return nil, fmt.Errorf("calendar event has no attendee response to update")
	}
	event.Attendees[0].RSVP = normalized
	event.UpdatedAt = time.Now().UTC()
	if err := b.cache.PutCalendarEvent(*event); err != nil {
		return nil, err
	}
	return b.cache.GetCalendarEventByRef(event.Ref)
}

func (b *LocalBackend) SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, nil
	}
	results, err := b.searchCachedCalendarEvents(query, start, end)
	if err != nil {
		return nil, err
	}
	if len(results) > 0 || strings.TrimSpace(query) == "" {
		return results, nil
	}
	cachedAgenda, err := b.listCachedCalendarAgenda(start, end)
	if err != nil {
		return nil, err
	}
	if len(cachedAgenda) > 0 {
		return results, nil
	}
	if err := b.syncConfiguredCalendarSources(context.Background()); err != nil {
		return nil, err
	}
	return b.searchCachedCalendarEvents(query, start, end)
}

func (b *LocalBackend) searchCachedCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error) {
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

func (b *LocalBackend) syncConfiguredCalendarSources(ctx context.Context) error {
	if b == nil || b.cache == nil {
		return nil
	}
	sources := b.configuredCalendarSources()
	if len(sources) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, calendarAgendaSyncTimeout)
	defer cancel()

	b.calendarSyncMu.Lock()
	defer b.calendarSyncMu.Unlock()

	if b.calendarLastSync == nil {
		b.calendarLastSync = make(map[models.SourceID]time.Time)
	}
	now := time.Now()
	for _, source := range sources {
		sourceID := models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID)
		accountID := models.NormalizeAccountID(models.AccountID(source.AccountID))
		if last := b.calendarLastSync[sourceID]; !last.IsZero() && now.Sub(last) < calendarAgendaSyncTTL {
			continue
		}
		opened, err := DefaultSourceRegistry().Open(ctx, source, SourceDeps{ProfileConfig: b.cfg})
		if err != nil {
			return fmt.Errorf("calendar source %s failed to open: %w", source.ID, err)
		}
		if opened.Calendar == nil {
			_ = opened.Close()
			return fmt.Errorf("calendar source %s did not provide a calendar adapter", source.ID)
		}
		collections, err := opened.Calendar.ListCalendars(ctx)
		if err != nil {
			_ = opened.Close()
			return fmt.Errorf("calendar source %s failed to list calendars: %w", source.ID, err)
		}
		keepRefs := make([]models.CollectionRef, 0, len(collections))
		for i := range collections {
			collections[i].Ref.Kind = models.SourceKindCalendar
			collections[i].Ref.SourceID = sourceID
			collections[i].Ref.AccountID = accountID
			keepRefs = append(keepRefs, collections[i].Ref)
		}
		if _, err := b.cache.PruneCalendarCollections(sourceID, accountID, keepRefs); err != nil {
			_ = opened.Close()
			return fmt.Errorf("calendar source %s failed to reconcile cached calendars: %w", source.ID, err)
		}
		service := calendar.NewEventService(b.cache, opened.Calendar)
		for _, collection := range collections {
			if _, err := service.SyncCollectionNoCache(ctx, collection.Ref); err != nil {
				service.Close()
				_ = opened.Close()
				return fmt.Errorf("calendar source %s failed to sync collection %s: %w", source.ID, collection.Ref.CollectionID, err)
			}
			if stored, err := b.cache.GetCalendarCollection(collection.Ref); err == nil && stored.SyncToken != "" {
				collection.SyncToken = stored.SyncToken
			}
			if err := b.cache.PutCalendarCollection(collection); err != nil {
				service.Close()
				_ = opened.Close()
				return err
			}
		}
		service.Close()
		_ = opened.Close()
		b.calendarLastSync[sourceID] = now
	}
	return nil
}

func (b *LocalBackend) configuredCalendarSources() []config.SourceConfig {
	if b == nil || b.cfg == nil {
		return nil
	}
	var sources []config.SourceConfig
	for _, source := range b.cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) == string(models.SourceKindCalendar) {
			if calendarSourceHasProviderConfig(source) {
				sources = append(sources, source)
			}
		}
	}
	return sources
}

func (b *LocalBackend) calendarMutationSourceForRef(ref models.EventRef) (calendar.MutationSource, bool, error) {
	for _, source := range b.configuredCalendarSources() {
		if models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID) != ref.SourceID {
			continue
		}
		if !calendarSourceHasProviderConfig(source) {
			continue
		}
		opened, err := DefaultSourceRegistry().Open(context.Background(), source, SourceDeps{ProfileConfig: b.cfg})
		if err != nil {
			return nil, false, calendarProviderMutationError("open", err)
		}
		if opened.CalendarMutation != nil {
			return opened.CalendarMutation, true, nil
		}
	}
	return nil, false, nil
}

func calendarProviderMutationError(action string, err error) error {
	if err == nil {
		return nil
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if errors.Is(err, models.ErrCalendarMutationConflict) {
		return fmt.Errorf("calendar provider %s conflict; cache was not updated: %w", action, err)
	}
	if errors.Is(err, models.ErrCalendarRecurrenceScopeUnsupported) {
		return fmt.Errorf("calendar provider %s recurrence scope unsupported; cache was not updated: %w", action, err)
	}
	return fmt.Errorf("calendar provider %s failed; cache was not updated: %w", action, err)
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

func (m *MultiBackend) ListCalendarCollections() ([]models.CalendarCollection, error) {
	var out []models.CalendarCollection
	for _, agenda := range m.calendarAgendaBackends() {
		collections, ok := agenda.(CalendarCollectionBackend)
		if !ok {
			continue
		}
		items, err := collections.ListCalendarCollections()
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sortCalendarCollections(out)
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

func (m *MultiBackend) SaveCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	ref := event.Ref.WithDefaults()
	var lastErr error = sql.ErrNoRows
	for _, backend := range m.calendarMutationBackendsForRef(ref) {
		if agenda, ok := backend.(CalendarAgendaBackend); ok {
			if _, err := agenda.GetCalendarEvent(ref); err != nil {
				lastErr = err
				if !errors.Is(err, sql.ErrNoRows) {
					continue
				}
				continue
			}
		}
		saved, err := backend.SaveCalendarEvent(event)
		if err == nil {
			return saved, nil
		}
		lastErr = err
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (m *MultiBackend) RespondCalendarEvent(ref models.EventRef, status string) (*models.CalendarEvent, error) {
	ref = ref.WithDefaults()
	var lastErr error = sql.ErrNoRows
	for _, backend := range m.calendarMutationBackendsForRef(ref) {
		if agenda, ok := backend.(CalendarAgendaBackend); ok {
			if _, err := agenda.GetCalendarEvent(ref); err != nil {
				lastErr = err
				if !errors.Is(err, sql.ErrNoRows) {
					continue
				}
				continue
			}
		}
		saved, err := backend.RespondCalendarEvent(ref, status)
		if err == nil {
			return saved, nil
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
	slots := make([]*accountSlot, 0, len(m.order))
	for _, id := range m.order {
		slots = append(slots, m.slots[id])
	}
	slot := m.slots[active]
	m.mu.RUnlock()

	if active == AllAccountsSourceID {
		return calendarAgendaBackendsFromSlots(slots, false)
	}
	if slot == nil {
		return calendarAgendaBackendsFromSlots(slots, true)
	}
	if backend, ok := slot.backend.(CalendarAgendaBackend); ok {
		if backend.CalendarAgendaAvailable() {
			return []CalendarAgendaBackend{backend}
		}
		if fallback := calendarAgendaBackendsFromSlots(slots, true); len(fallback) > 0 {
			return fallback
		}
		return []CalendarAgendaBackend{backend}
	}
	return calendarAgendaBackendsFromSlots(slots, true)
}

func calendarAgendaBackendsFromSlots(slots []*accountSlot, requireAvailable bool) []CalendarAgendaBackend {
	out := make([]CalendarAgendaBackend, 0, len(slots))
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		backend, ok := slot.backend.(CalendarAgendaBackend)
		if !ok {
			continue
		}
		if requireAvailable && !backend.CalendarAgendaAvailable() {
			continue
		}
		out = append(out, backend)
	}
	return out
}

func (m *MultiBackend) calendarMutationBackendsForRef(ref models.EventRef) []CalendarEventMutationBackend {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	active := m.active
	slots := make([]*accountSlot, 0, len(m.order))
	if active == AllAccountsSourceID {
		for _, id := range m.order {
			slots = append(slots, m.slots[id])
		}
	} else if slot := m.slots[active]; slot != nil {
		slots = append(slots, slot)
	}
	if active != AllAccountsSourceID {
		for _, id := range m.order {
			slot := m.slots[id]
			if slot != nil && slot.info.SourceID == ref.SourceID {
				alreadyIncluded := false
				for _, existing := range slots {
					if existing == slot {
						alreadyIncluded = true
						break
					}
				}
				if !alreadyIncluded {
					slots = append(slots, slot)
				}
			}
		}
	}
	m.mu.RUnlock()

	out := make([]CalendarEventMutationBackend, 0, len(slots))
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		if backend, ok := slot.backend.(CalendarEventMutationBackend); ok {
			out = append(out, backend)
		}
	}
	return out
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
		leftStart, leftEnd := normalizedCalendarSortRange(events[i])
		rightStart, rightEnd := normalizedCalendarSortRange(events[j])
		if !leftStart.Equal(rightStart) {
			return leftStart.Before(rightStart)
		}
		if !leftEnd.Equal(rightEnd) {
			return leftEnd.Before(rightEnd)
		}
		if events[i].Title != events[j].Title {
			return events[i].Title < events[j].Title
		}
		return events[i].Ref.LocalID < events[j].Ref.LocalID
	})
}

func normalizedCalendarSortRange(event models.CalendarEvent) (time.Time, time.Time) {
	start := event.Start
	end := event.End
	if start.IsZero() {
		start = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	if end.IsZero() || end.Before(start) {
		end = start
	}
	return start, end
}

func sortCalendarCollections(collections []models.CalendarCollection) {
	sort.SliceStable(collections, func(i, j int) bool {
		left := collections[i].Ref
		right := collections[j].Ref
		if left.AccountID != right.AccountID {
			return left.AccountID < right.AccountID
		}
		if left.SourceID != right.SourceID {
			return left.SourceID < right.SourceID
		}
		if left.DisplayName != right.DisplayName {
			return left.DisplayName < right.DisplayName
		}
		return left.CollectionID < right.CollectionID
	})
}
