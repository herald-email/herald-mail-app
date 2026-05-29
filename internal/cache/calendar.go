package cache

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func calendarCollectionLocalID(ref models.CollectionRef) string {
	ref.Kind = models.SourceKindCalendar
	return ref.CacheKey()
}

func (c *Cache) PutCalendarCollection(collection models.CalendarCollection) error {
	ref := collection.Ref
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	localID := calendarCollectionLocalID(ref)
	now := time.Now().UTC()
	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO calendar_collections
		(local_id, source_id, account_id, calendar_id, display_name, color, sync_token, etag, last_synced, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, localID, string(ref.SourceID), string(ref.AccountID), ref.CollectionID, ref.DisplayName, collection.Color, collection.SyncToken, collection.ETag, now, now)
	return err
}

func (c *Cache) GetCalendarCollection(ref models.CollectionRef) (*models.CalendarCollection, error) {
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	localID := calendarCollectionLocalID(ref)
	row := c.db.QueryRow(`
		SELECT source_id, account_id, calendar_id, display_name, color, sync_token, etag
		FROM calendar_collections
		WHERE local_id = ?
	`, localID)

	var sourceID, accountID, calendarID, displayName, color, syncToken, etag string
	if err := row.Scan(&sourceID, &accountID, &calendarID, &displayName, &color, &syncToken, &etag); err != nil {
		return nil, err
	}
	return &models.CalendarCollection{
		Ref: models.CollectionRef{
			SourceID:     models.SourceID(sourceID),
			AccountID:    models.AccountID(accountID),
			Kind:         models.SourceKindCalendar,
			CollectionID: calendarID,
			DisplayName:  displayName,
		},
		Color:     color,
		SyncToken: syncToken,
		ETag:      etag,
	}, nil
}

func (c *Cache) ListCalendarCollections(sourceID models.SourceID, accountID models.AccountID) ([]models.CalendarCollection, error) {
	sourceID = models.NormalizeSourceID(sourceID, models.DefaultCalendarSourceID)
	accountID = models.NormalizeAccountID(accountID)
	rows, err := c.db.Query(`
		SELECT source_id, account_id, calendar_id, display_name, color, sync_token, etag
		FROM calendar_collections
		WHERE source_id = ? AND account_id = ?
		ORDER BY display_name ASC, calendar_id ASC
	`, string(sourceID), string(accountID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.CalendarCollection
	for rows.Next() {
		var sourceIDValue, accountIDValue, calendarID, displayName, color, syncToken, etag string
		if err := rows.Scan(&sourceIDValue, &accountIDValue, &calendarID, &displayName, &color, &syncToken, &etag); err != nil {
			return nil, err
		}
		out = append(out, models.CalendarCollection{
			Ref: models.CollectionRef{
				SourceID:     models.SourceID(sourceIDValue),
				AccountID:    models.AccountID(accountIDValue),
				Kind:         models.SourceKindCalendar,
				CollectionID: calendarID,
				DisplayName:  displayName,
			},
			Color:     color,
			SyncToken: syncToken,
			ETag:      etag,
		})
	}
	return out, rows.Err()
}

func (c *Cache) PruneCalendarCollections(sourceID models.SourceID, accountID models.AccountID, keep []models.CollectionRef) ([]models.CollectionRef, error) {
	sourceID = models.NormalizeSourceID(sourceID, models.DefaultCalendarSourceID)
	accountID = models.NormalizeAccountID(accountID)
	keepLocalIDs := make(map[string]struct{}, len(keep))
	for _, ref := range keep {
		ref.Kind = models.SourceKindCalendar
		ref.SourceID = sourceID
		ref.AccountID = accountID
		keepLocalIDs[calendarCollectionLocalID(ref)] = struct{}{}
	}

	existing, err := c.ListCalendarCollections(sourceID, accountID)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		return nil, nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC()
	removed := make([]models.CollectionRef, 0)
	for _, collection := range existing {
		ref := collection.Ref
		ref.Kind = models.SourceKindCalendar
		ref.SourceID = sourceID
		ref.AccountID = accountID
		localID := calendarCollectionLocalID(ref)
		if _, ok := keepLocalIDs[localID]; ok {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM calendar_collections WHERE local_id = ?`, localID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`
			UPDATE calendar_events
			SET invalidated_at = ?
			WHERE source_id = ? AND account_id = ? AND calendar_id = ? AND invalidated_at IS NULL
		`, now, string(sourceID), string(accountID), ref.CollectionID); err != nil {
			return nil, err
		}
		removed = append(removed, ref)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return removed, nil
}

func (c *Cache) PutCalendarEvent(event models.CalendarEvent) error {
	ref := event.Ref.WithDefaults()
	now := time.Now().UTC()
	attendeesJSON, err := json.Marshal(event.Attendees)
	if err != nil {
		return err
	}
	recurrenceJSON, err := json.Marshal(event.Recurrence)
	if err != nil {
		return err
	}
	attachmentsJSON, err := json.Marshal(event.Attachments)
	if err != nil {
		return err
	}
	remindersJSON, err := json.Marshal(event.Reminders)
	if err != nil {
		return err
	}
	alternateTimeZonesJSON, err := json.Marshal(event.AlternateTimeZones)
	if err != nil {
		return err
	}
	_, err = c.db.Exec(`
		INSERT OR REPLACE INTO calendar_events
		(local_id, source_id, account_id, calendar_id, event_id, instance_id, provider_uid, etag, revision,
		 title, description, location, starts_at, ends_at, all_day, timezone, status, organizer, organizer_email,
		 attendees_json, recurrence_json, recurrence_summary, attachments_json, reminders_json, alternate_timezones_json,
		 updated_at, raw, cached_at, invalidated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, ref.LocalID, string(ref.SourceID), string(ref.AccountID), ref.CalendarID, ref.EventID, ref.InstanceID, event.ProviderUID,
		ref.ETag, event.Revision, event.Title, event.Description, event.Location, event.Start, event.End,
		boolToInt(event.AllDay), event.TimeZone, event.Status, event.Organizer, event.OrganizerEmail,
		string(attendeesJSON), string(recurrenceJSON), event.RecurrenceSummary, string(attachmentsJSON), string(remindersJSON), string(alternateTimeZonesJSON),
		event.UpdatedAt, event.Raw, now)
	return err
}

func (c *Cache) GetCalendarEventByRef(ref models.EventRef) (*models.CalendarEvent, error) {
	ref = ref.WithDefaults()
	row := c.db.QueryRow(`
		SELECT source_id, account_id, calendar_id, event_id, instance_id, provider_uid, etag, revision,
		       title, description, location, starts_at, ends_at, all_day, timezone, status, organizer, organizer_email,
		       attendees_json, recurrence_json, recurrence_summary, attachments_json, reminders_json, alternate_timezones_json,
		       updated_at, raw, local_id
		FROM calendar_events
		WHERE local_id = ? AND invalidated_at IS NULL
	`, ref.LocalID)
	return scanCalendarEvent(row)
}

func (c *Cache) InvalidateCalendarEvent(ref models.EventRef) error {
	ref = ref.WithDefaults()
	now := time.Now().UTC()
	result, err := c.db.Exec(`UPDATE calendar_events SET invalidated_at = ? WHERE local_id = ?`, now, ref.LocalID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err == nil && rows > 0 {
		return nil
	}
	_, err = c.db.Exec(`
		UPDATE calendar_events
		SET invalidated_at = ?
		WHERE source_id = ? AND account_id = ? AND calendar_id = ? AND event_id = ? AND instance_id = ?
	`, now, string(ref.SourceID), string(ref.AccountID), ref.CalendarID, ref.EventID, ref.InstanceID)
	return err
}

func (c *Cache) ListCalendarEvents(ref models.CollectionRef, start, end time.Time) ([]models.CalendarEvent, error) {
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	if start.IsZero() {
		start = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if end.IsZero() {
		end = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	rows, err := c.db.Query(`
		SELECT source_id, account_id, calendar_id, event_id, instance_id, provider_uid, etag, revision,
		       title, description, location, starts_at, ends_at, all_day, timezone, status, organizer, organizer_email,
		       attendees_json, recurrence_json, recurrence_summary, attachments_json, reminders_json, alternate_timezones_json,
		       updated_at, raw, local_id
		FROM calendar_events
		WHERE source_id = ? AND account_id = ? AND calendar_id = ? AND invalidated_at IS NULL
		  AND (
		    ((starts_at IS NULL OR starts_at < ?) AND (ends_at IS NULL OR ends_at > ?))
		    OR COALESCE(recurrence_json, '[]') NOT IN ('[]', 'null', '')
		    OR raw LIKE '%RRULE%'
		    OR raw LIKE '%RDATE%'
		  )
		ORDER BY starts_at ASC, title ASC
	`, string(ref.SourceID), string(ref.AccountID), ref.CollectionID, end, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.CalendarEvent
	for rows.Next() {
		event, err := scanCalendarEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}
	return events, rows.Err()
}

func (c *Cache) ListCalendarAgendaEvents(sourceID models.SourceID, accountID models.AccountID, start, end time.Time) ([]models.CalendarEvent, error) {
	sourceID = models.NormalizeSourceID(sourceID, models.DefaultCalendarSourceID)
	accountID = models.NormalizeAccountID(accountID)
	if start.IsZero() {
		start = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if end.IsZero() {
		end = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	rows, err := c.db.Query(`
		SELECT source_id, account_id, calendar_id, event_id, instance_id, provider_uid, etag, revision,
		       title, description, location, starts_at, ends_at, all_day, timezone, status, organizer, organizer_email,
		       attendees_json, recurrence_json, recurrence_summary, attachments_json, reminders_json, alternate_timezones_json,
		       updated_at, raw, local_id
		FROM calendar_events
		WHERE source_id = ? AND account_id = ? AND invalidated_at IS NULL
		  AND (
		    ((starts_at IS NULL OR starts_at < ?) AND (ends_at IS NULL OR ends_at > ?))
		    OR COALESCE(recurrence_json, '[]') NOT IN ('[]', 'null', '')
		    OR raw LIKE '%RRULE%'
		    OR raw LIKE '%RDATE%'
		  )
		ORDER BY starts_at ASC, title ASC
	`, string(sourceID), string(accountID), end, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.CalendarEvent
	for rows.Next() {
		event, err := scanCalendarEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}
	return events, rows.Err()
}

func (c *Cache) SearchCalendarEvents(sourceID models.SourceID, accountID models.AccountID, query string, start, end time.Time) ([]models.CalendarEvent, error) {
	events, err := c.ListCalendarAgendaEvents(sourceID, accountID, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]models.CalendarEvent, 0, len(events))
	for _, event := range events {
		if models.CalendarEventMatchesQuery(event, query) {
			out = append(out, event)
		}
	}
	return out, nil
}

type calendarEventScanner interface {
	Scan(dest ...any) error
}

func scanCalendarEvent(scanner calendarEventScanner) (*models.CalendarEvent, error) {
	var sourceID, accountID, calendarID, eventID, instanceID, providerUID, etag, revision string
	var title, description, location, timeZone, status, organizer, organizerEmail, raw, localID string
	var attendeesJSON, recurrenceJSON, recurrenceSummary, attachmentsJSON, remindersJSON, alternateTimeZonesJSON string
	var start, end, updatedAt sql.NullTime
	var allDay int
	if err := scanner.Scan(
		&sourceID, &accountID, &calendarID, &eventID, &instanceID, &providerUID, &etag, &revision,
		&title, &description, &location, &start, &end, &allDay, &timeZone, &status, &organizer, &organizerEmail,
		&attendeesJSON, &recurrenceJSON, &recurrenceSummary, &attachmentsJSON, &remindersJSON, &alternateTimeZonesJSON,
		&updatedAt, &raw, &localID,
	); err != nil {
		return nil, err
	}
	var attendees []models.CalendarAttendee
	_ = json.Unmarshal([]byte(attendeesJSON), &attendees)
	var recurrence []string
	_ = json.Unmarshal([]byte(recurrenceJSON), &recurrence)
	var attachments []models.CalendarAttachment
	_ = json.Unmarshal([]byte(attachmentsJSON), &attachments)
	var reminders []models.CalendarReminder
	_ = json.Unmarshal([]byte(remindersJSON), &reminders)
	var alternateTimeZones []string
	_ = json.Unmarshal([]byte(alternateTimeZonesJSON), &alternateTimeZones)
	event := &models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   models.SourceID(sourceID),
			AccountID:  models.AccountID(accountID),
			CalendarID: calendarID,
			EventID:    eventID,
			InstanceID: instanceID,
			ETag:       etag,
			LocalID:    localID,
		}.WithDefaults(),
		ProviderUID:        providerUID,
		Title:              title,
		Description:        description,
		Location:           location,
		AllDay:             allDay != 0,
		TimeZone:           timeZone,
		Status:             status,
		Organizer:          organizer,
		OrganizerEmail:     organizerEmail,
		Attendees:          attendees,
		Recurrence:         recurrence,
		RecurrenceSummary:  recurrenceSummary,
		Attachments:        attachments,
		Reminders:          reminders,
		AlternateTimeZones: alternateTimeZones,
		Revision:           revision,
		Raw:                raw,
	}
	if start.Valid {
		event.Start = start.Time
	}
	if end.Valid {
		event.End = end.Time
	}
	if updatedAt.Valid {
		event.UpdatedAt = updatedAt.Time
	}
	return event, nil
}
