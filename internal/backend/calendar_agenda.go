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

// CalendarAgendaCacheBackend lets the TUI show cached calendar rows immediately
// while a provider refresh runs in the background.
type CalendarAgendaCacheBackend interface {
	ListCachedCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error)
	ListCachedCalendarCollections() ([]models.CalendarCollection, error)
	RefreshCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, []models.CalendarCollection, error)
}

// CalendarCollectionBackend exposes user-facing calendar lists for the TUI rail.
type CalendarCollectionBackend interface {
	ListCalendarCollections() ([]models.CalendarCollection, error)
}

// CalendarEventMutationBackend is the local/cache-backed calendar edit
// boundary. Live provider mutation adapters stay behind this interface until a
// later provider-write stage enables them explicitly.
type CalendarEventMutationBackend interface {
	CreateCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error)
	SaveCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error)
	RespondCalendarEvent(ref models.EventRef, status string) (*models.CalendarEvent, error)
	FindCalendarEventByUID(ref models.CollectionRef, uid string) (*models.CalendarEvent, error)
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

func (d *DemoBackend) CreateCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.TrimSpace(event.Ref.EventID) == "" {
		event.Ref.EventID = firstNonEmptyString(event.ProviderUID, event.Title, "mail-invitation")
	}
	event.Ref = event.Ref.WithDefaults()
	if strings.TrimSpace(event.ProviderUID) == "" {
		event.ProviderUID = event.Ref.EventID
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	d.calendarEvents = append(d.calendarEvents, event)
	saved := event
	return &saved, nil
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

func (d *DemoBackend) FindCalendarEventByUID(ref models.CollectionRef, uid string) (*models.CalendarEvent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, nil
	}
	for _, event := range d.calendarEvents {
		event.Ref = event.Ref.WithDefaults()
		if event.Ref.SourceID == ref.SourceID && event.Ref.AccountID == ref.AccountID && event.Ref.CalendarID == ref.CollectionID && strings.TrimSpace(event.ProviderUID) == uid {
			got := event
			return &got, nil
		}
	}
	return nil, nil
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
	cached, err := b.ListCachedCalendarAgenda(start, end)
	if err != nil {
		return nil, err
	}
	if err := b.syncConfiguredCalendarSources(context.Background()); err != nil {
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	return b.ListCachedCalendarAgenda(start, end)
}

func (b *LocalBackend) ListCachedCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	var out []models.CalendarEvent
	for _, source := range b.configuredCalendarSources() {
		events, err := b.cache.ListCalendarAgendaEvents(models.SourceID(source.ID), models.AccountID(source.AccountID), start, end)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			out = append(out, calendar.ExpandEventOccurrences(event, start, end)...)
		}
	}
	sortCalendarEvents(out)
	return out, nil
}

func (b *LocalBackend) listCachedCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	return b.ListCachedCalendarAgenda(start, end)
}

func (b *LocalBackend) ListCalendarCollections() ([]models.CalendarCollection, error) {
	if b == nil || b.cache == nil {
		return nil, nil
	}
	cached, err := b.ListCachedCalendarCollections()
	if err != nil {
		return nil, err
	}
	if err := b.syncConfiguredCalendarSources(context.Background()); err != nil {
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	return b.ListCachedCalendarCollections()
}

func (b *LocalBackend) ListCachedCalendarCollections() ([]models.CalendarCollection, error) {
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

func (b *LocalBackend) listCachedCalendarCollections() ([]models.CalendarCollection, error) {
	return b.ListCachedCalendarCollections()
}

func (b *LocalBackend) RefreshCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, []models.CalendarCollection, error) {
	if b == nil || b.cache == nil {
		return nil, nil, nil
	}
	cachedEvents, cacheErr := b.ListCachedCalendarAgenda(start, end)
	cachedCollections, collectionsErr := b.ListCachedCalendarCollections()
	if cacheErr != nil {
		return nil, nil, cacheErr
	}
	if collectionsErr != nil {
		return nil, nil, collectionsErr
	}
	if err := b.syncConfiguredCalendarSources(context.Background()); err != nil {
		if len(cachedEvents) > 0 || len(cachedCollections) > 0 {
			return cachedEvents, cachedCollections, nil
		}
		return nil, nil, err
	}
	events, err := b.ListCachedCalendarAgenda(start, end)
	if err != nil {
		return nil, nil, err
	}
	collections, err := b.ListCachedCalendarCollections()
	if err != nil {
		return nil, nil, err
	}
	return events, collections, nil
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

func (b *LocalBackend) CreateCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, sql.ErrNoRows
	}
	event.Ref = event.Ref.WithDefaults()
	if strings.TrimSpace(event.ProviderUID) == "" {
		event.ProviderUID = strings.TrimSpace(event.Ref.EventID)
	}
	if strings.TrimSpace(event.Ref.EventID) == "" {
		event.Ref.EventID = firstNonEmptyString(event.ProviderUID, event.Title, "mail-invitation")
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = time.Now().UTC()
	}
	if source, ok, err := b.calendarMutationSourceForRef(event.Ref); err != nil {
		return nil, err
	} else if ok {
		saved, err := source.CreateEvent(context.Background(), event, models.CalendarMutationOptions{
			RecurrenceScope: models.CalendarMutationScopeThisEvent,
		})
		if err != nil {
			return nil, calendarProviderMutationError("create", err)
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

func (b *LocalBackend) FindCalendarEventByUID(ref models.CollectionRef, uid string) (*models.CalendarEvent, error) {
	if b == nil || b.cache == nil {
		return nil, nil
	}
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, nil
	}
	if source, ok, err := b.calendarUIDLookupSourceForRef(ref); err != nil {
		return nil, err
	} else if ok {
		found, err := source.FindEventByUID(context.Background(), ref, uid)
		if err != nil {
			return nil, calendarProviderMutationError("duplicate lookup", err)
		}
		if found != nil {
			return found, nil
		}
		return nil, nil
	}
	found, err := b.cache.FindCalendarEventByProviderUID(ref, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return found, err
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

func (b *LocalBackend) calendarUIDLookupSourceForRef(ref models.CollectionRef) (calendar.UIDLookupSource, bool, error) {
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
		if lookup, ok := opened.Calendar.(calendar.UIDLookupSource); ok {
			return lookup, true, nil
		}
		if lookup, ok := opened.CalendarMutation.(calendar.UIDLookupSource); ok {
			return lookup, true, nil
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
	if errors.Is(err, models.ErrCalendarAuthorizationRequired) {
		return fmt.Errorf("calendar provider %s authorization required; cache was not updated: %w", action, err)
	}
	if errors.Is(err, models.ErrCalendarWritePermission) {
		return fmt.Errorf("calendar provider %s write permission missing; cache was not updated: %w", action, err)
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

func (m *MultiBackend) ListCachedCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	var out []models.CalendarEvent
	for _, backend := range m.calendarAgendaBackends() {
		if !backend.CalendarAgendaAvailable() {
			continue
		}
		if cached, ok := backend.(CalendarAgendaCacheBackend); ok {
			events, err := cached.ListCachedCalendarAgenda(start, end)
			if err != nil {
				return nil, err
			}
			out = append(out, events...)
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

func (m *MultiBackend) ListCachedCalendarCollections() ([]models.CalendarCollection, error) {
	var out []models.CalendarCollection
	for _, agenda := range m.calendarAgendaBackends() {
		if cached, ok := agenda.(CalendarAgendaCacheBackend); ok {
			items, err := cached.ListCachedCalendarCollections()
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
			continue
		}
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

func (m *MultiBackend) RefreshCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, []models.CalendarCollection, error) {
	var events []models.CalendarEvent
	var collections []models.CalendarCollection
	for _, agenda := range m.calendarAgendaBackends() {
		if !agenda.CalendarAgendaAvailable() {
			continue
		}
		if cached, ok := agenda.(CalendarAgendaCacheBackend); ok {
			items, cols, err := cached.RefreshCalendarAgenda(start, end)
			if err != nil {
				return nil, nil, err
			}
			events = append(events, items...)
			collections = append(collections, cols...)
			continue
		}
		items, err := agenda.ListCalendarAgenda(start, end)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, items...)
		if collectionBackend, ok := agenda.(CalendarCollectionBackend); ok {
			cols, err := collectionBackend.ListCalendarCollections()
			if err != nil {
				return nil, nil, err
			}
			collections = append(collections, cols...)
		}
	}
	sortCalendarEvents(events)
	sortCalendarCollections(collections)
	return events, collections, nil
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

func (m *MultiBackend) CreateCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	ref := event.Ref.WithDefaults()
	var lastErr error = sql.ErrNoRows
	for _, backend := range m.calendarMutationBackendsForRef(ref) {
		saved, err := backend.CreateCalendarEvent(event)
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

func (m *MultiBackend) FindCalendarEventByUID(ref models.CollectionRef, uid string) (*models.CalendarEvent, error) {
	eventRef := models.EventRef{
		SourceID:   ref.SourceID,
		AccountID:  ref.AccountID,
		CalendarID: ref.CollectionID,
	}.WithDefaults()
	var lastErr error = sql.ErrNoRows
	for _, backend := range m.calendarMutationBackendsForRef(eventRef) {
		found, err := backend.FindCalendarEventByUID(ref, uid)
		if err == nil {
			if found != nil {
				return found, nil
			}
			lastErr = sql.ErrNoRows
			continue
		}
		lastErr = err
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if errors.Is(lastErr, sql.ErrNoRows) {
		return nil, nil
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
