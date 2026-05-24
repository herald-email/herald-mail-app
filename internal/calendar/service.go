package calendar

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/work"
)

type EventService struct {
	cache       *cache.Cache
	source      Source
	coordinator *work.Coordinator
}

func NewEventService(cacheStore *cache.Cache, source Source) *EventService {
	return &EventService{
		cache:       cacheStore,
		source:      source,
		coordinator: work.NewCoordinator(),
	}
}

func (s *EventService) Close() {
	if s != nil && s.coordinator != nil {
		s.coordinator.Close()
	}
}

func (s *EventService) GetEvent(ctx context.Context, ref models.EventRef) (*models.CalendarEvent, error) {
	if s == nil {
		return nil, fmt.Errorf("calendar event service is nil")
	}
	ref = s.normalizeRef(ref)
	if s.cache != nil {
		event, err := s.cache.GetCalendarEventByRef(ref)
		if err == nil {
			return event, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	result := s.coordinator.Submit(ctx, work.Spec{
		SourceID: work.SourceID(ref.SourceID),
		ResourceKey: work.ResourceKey{
			SourceID:     string(ref.SourceID),
			AccountID:    string(ref.AccountID),
			CollectionID: ref.CalendarID,
			ItemID:       ref.LocalID,
			Operation:    "calendar_event_fetch",
			Freshness:    ref.ETag,
		},
		Policy: work.PolicyCoalesceByResource | work.PolicyReplayCompletedResource,
		Run: func(runCtx context.Context) (any, error) {
			if s.cache != nil {
				event, err := s.cache.GetCalendarEventByRef(ref)
				if err == nil {
					return event, nil
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return nil, err
				}
			}
			return s.GetEventNoCache(runCtx, ref)
		},
	})
	value, err := result.Await(ctx)
	if err != nil {
		return nil, err
	}
	event, ok := value.(*models.CalendarEvent)
	if !ok {
		return nil, fmt.Errorf("calendar event service returned %T", value)
	}
	return event, nil
}

func (s *EventService) GetEventNoCache(ctx context.Context, ref models.EventRef) (*models.CalendarEvent, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("calendar source is nil")
	}
	ref = s.normalizeRef(ref)
	event, err := s.source.FetchEvent(ctx, ref)
	if err != nil {
		return nil, err
	}
	normalized := normalizeFetchedEvent(ref, event)
	if s.cache != nil {
		if err := s.cache.PutCalendarEvent(normalized); err != nil {
			return nil, err
		}
	}
	return &normalized, nil
}

func (s *EventService) PutEvent(event models.CalendarEvent) error {
	if s == nil || s.cache == nil {
		return nil
	}
	event.Ref = s.normalizeRef(event.Ref)
	return s.cache.PutCalendarEvent(event)
}

func (s *EventService) InvalidateEvent(ref models.EventRef) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.InvalidateCalendarEvent(s.normalizeRef(ref))
}

func (s *EventService) SyncCollectionNoCache(ctx context.Context, ref models.CollectionRef) ([]models.CalendarEvent, error) {
	if s == nil || s.source == nil {
		return nil, fmt.Errorf("calendar source is nil")
	}
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.source.ID())
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	events, err := s.source.ListEvents(ctx, ref)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		for _, event := range events {
			event.Ref = s.normalizeRef(event.Ref)
			if err := s.cache.PutCalendarEvent(event); err != nil {
				return nil, err
			}
		}
	}
	return events, nil
}

func (s *EventService) normalizeRef(ref models.EventRef) models.EventRef {
	if s != nil && s.source != nil {
		ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.source.ID())
		if ref.AccountID == "" {
			ref.AccountID = s.source.AccountID()
		}
	}
	return ref.WithDefaults()
}

func normalizeFetchedEvent(request models.EventRef, event *models.CalendarEvent) models.CalendarEvent {
	if event == nil {
		return models.CalendarEvent{Ref: request}
	}
	out := *event
	if out.Ref.SourceID == "" {
		out.Ref.SourceID = request.SourceID
	}
	if out.Ref.AccountID == "" {
		out.Ref.AccountID = request.AccountID
	}
	if out.Ref.CalendarID == "" {
		out.Ref.CalendarID = request.CalendarID
	}
	if out.Ref.EventID == "" {
		out.Ref.EventID = request.EventID
	}
	if out.Ref.ETag == "" {
		out.Ref.ETag = request.ETag
	}
	out.Ref = out.Ref.WithDefaults()
	return out
}
