package daemon

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type calendarEventMutationRequest struct {
	SourceID    string  `json:"source_id,omitempty"`
	AccountID   string  `json:"account_id,omitempty"`
	CalendarID  string  `json:"calendar_id,omitempty"`
	EventID     string  `json:"event_id,omitempty"`
	InstanceID  string  `json:"instance_id,omitempty"`
	LocalID     string  `json:"local_id,omitempty"`
	ProviderUID *string `json:"provider_uid,omitempty"`

	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Location    *string `json:"location,omitempty"`
	Start       *string `json:"start,omitempty"`
	End         *string `json:"end,omitempty"`
	TimeZone    *string `json:"timezone,omitempty"`
	Status      *string `json:"status,omitempty"`
	AllDay      *bool   `json:"all_day,omitempty"`

	Attendees          []models.CalendarAttendee   `json:"attendees,omitempty"`
	Recurrence         []string                    `json:"recurrence,omitempty"`
	Reminders          []models.CalendarReminder   `json:"reminders,omitempty"`
	Attachments        []models.CalendarAttachment `json:"attachments,omitempty"`
	AlternateTimeZones []string                    `json:"alternate_timezones,omitempty"`
}

func (s *Server) handleCreateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	mutation, ok := s.backend.(backend.CalendarEventMutationBackend)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "calendar mutation backend unavailable")
		return
	}
	req, err := decodeCalendarEventMutationRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.mergeScopeQuery(r)
	ref := req.eventRef("")
	if strings.TrimSpace(ref.CalendarID) == "" {
		writeError(w, http.StatusBadRequest, "calendar_id is required")
		return
	}
	event, err := req.createEvent(ref)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := mutation.CreateCalendarEvent(event)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if saved != nil {
		s.broadcastCalendarMutation("calendar_create", saved.Ref)
		writeJSON(w, http.StatusCreated, saved)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (s *Server) handleUpdateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	mutation, ok := s.backend.(backend.CalendarEventMutationBackend)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "calendar mutation backend unavailable")
		return
	}
	agenda, ok := s.backend.(backend.CalendarAgendaBackend)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "calendar agenda backend unavailable")
		return
	}
	req, err := decodeCalendarEventMutationRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.mergeScopeQuery(r)
	ref := req.eventRef(r.PathValue("id"))
	if !calendarEventRefUsable(ref) {
		writeError(w, http.StatusBadRequest, "local_id or calendar_id plus event_id is required")
		return
	}
	base, err := agenda.GetCalendarEvent(ref)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "calendar event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if base == nil {
		writeError(w, http.StatusNotFound, "calendar event not found")
		return
	}
	updated, err := req.applyPatch(*base)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := mutation.SaveCalendarEvent(updated)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if saved != nil {
		s.broadcastCalendarMutation("calendar_update", saved.Ref)
		writeJSON(w, http.StatusOK, saved)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteCalendarEvent(w http.ResponseWriter, r *http.Request) {
	mutation, ok := s.backend.(backend.CalendarEventMutationBackend)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "calendar mutation backend unavailable")
		return
	}
	req := calendarEventMutationRequest{
		SourceID:   r.URL.Query().Get("source_id"),
		AccountID:  r.URL.Query().Get("account_id"),
		CalendarID: r.URL.Query().Get("calendar_id"),
		EventID:    r.URL.Query().Get("event_id"),
		InstanceID: r.URL.Query().Get("instance_id"),
		LocalID:    r.URL.Query().Get("local_id"),
	}
	ref := req.eventRef(r.PathValue("id"))
	if !calendarEventRefUsable(ref) {
		writeError(w, http.StatusBadRequest, "local_id or calendar_id plus event_id is required")
		return
	}
	if err := mutation.DeleteCalendarEvent(ref); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "calendar event not found")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.broadcastCalendarMutation("calendar_delete", ref)
	w.WriteHeader(http.StatusNoContent)
}

func decodeCalendarEventMutationRequest(r *http.Request) (calendarEventMutationRequest, error) {
	var req calendarEventMutationRequest
	if r.Body == nil {
		return req, nil
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func (req *calendarEventMutationRequest) mergeScopeQuery(r *http.Request) {
	if r == nil {
		return
	}
	query := r.URL.Query()
	if req.SourceID == "" {
		req.SourceID = query.Get("source_id")
	}
	if req.AccountID == "" {
		req.AccountID = query.Get("account_id")
	}
	if req.CalendarID == "" {
		req.CalendarID = query.Get("calendar_id")
	}
	if req.EventID == "" {
		req.EventID = query.Get("event_id")
	}
	if req.InstanceID == "" {
		req.InstanceID = query.Get("instance_id")
	}
	if req.LocalID == "" {
		req.LocalID = query.Get("local_id")
	}
}

func (req calendarEventMutationRequest) eventRef(pathEventID string) models.EventRef {
	ref := models.EventRef{
		SourceID:   models.SourceID(strings.TrimSpace(req.SourceID)),
		AccountID:  models.AccountID(strings.TrimSpace(req.AccountID)),
		CalendarID: strings.TrimSpace(req.CalendarID),
		EventID:    strings.TrimSpace(firstNonEmptyDaemon(req.EventID, pathEventID)),
		InstanceID: strings.TrimSpace(req.InstanceID),
		LocalID:    strings.TrimSpace(req.LocalID),
	}
	if parsed, ok := models.EventRefFromLocalID(ref.LocalID); ok {
		if ref.SourceID == "" {
			ref.SourceID = parsed.SourceID
		}
		if ref.AccountID == "" {
			ref.AccountID = parsed.AccountID
		}
		if ref.CalendarID == "" {
			ref.CalendarID = parsed.CalendarID
		}
		if ref.EventID == "" {
			ref.EventID = parsed.EventID
		}
		if ref.InstanceID == "" {
			ref.InstanceID = parsed.InstanceID
		}
	}
	return ref.WithDefaults()
}

func calendarEventRefUsable(ref models.EventRef) bool {
	ref = ref.WithDefaults()
	return strings.TrimSpace(ref.LocalID) != "" || (strings.TrimSpace(ref.CalendarID) != "" && strings.TrimSpace(ref.EventID) != "")
}

func (req calendarEventMutationRequest) createEvent(ref models.EventRef) (models.CalendarEvent, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(ref.EventID) == "" {
		ref.EventID = fmt.Sprintf("daemon-%d", now.UnixNano())
		ref.LocalID = ""
		ref = ref.WithDefaults()
	}
	event := models.CalendarEvent{Ref: ref, Status: "confirmed", UpdatedAt: now}
	if req.ProviderUID != nil {
		event.ProviderUID = strings.TrimSpace(*req.ProviderUID)
	}
	if event.ProviderUID == "" {
		event.ProviderUID = ref.EventID
	}
	if req.Title != nil {
		event.Title = strings.TrimSpace(*req.Title)
	}
	if event.Title == "" {
		return models.CalendarEvent{}, fmt.Errorf("title is required")
	}
	if err := req.applyCalendarEventFields(&event, true); err != nil {
		return models.CalendarEvent{}, err
	}
	return event, nil
}

func (req calendarEventMutationRequest) applyPatch(base models.CalendarEvent) (models.CalendarEvent, error) {
	base.Ref = req.eventRef(base.Ref.EventID)
	if req.ProviderUID != nil {
		base.ProviderUID = strings.TrimSpace(*req.ProviderUID)
	}
	if err := req.applyCalendarEventFields(&base, false); err != nil {
		return models.CalendarEvent{}, err
	}
	base.UpdatedAt = time.Now().UTC()
	return base, nil
}

func (req calendarEventMutationRequest) applyCalendarEventFields(event *models.CalendarEvent, requireTimes bool) error {
	if req.Title != nil {
		event.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		event.Description = strings.TrimSpace(*req.Description)
	}
	if req.Location != nil {
		event.Location = strings.TrimSpace(*req.Location)
	}
	if req.TimeZone != nil {
		event.TimeZone = strings.TrimSpace(*req.TimeZone)
	}
	if req.Status != nil {
		event.Status = strings.TrimSpace(*req.Status)
	}
	if event.Status == "" {
		event.Status = "confirmed"
	}
	if req.AllDay != nil {
		event.AllDay = *req.AllDay
	}
	loc := time.Local
	if strings.TrimSpace(event.TimeZone) != "" {
		loaded, err := time.LoadLocation(event.TimeZone)
		if err != nil {
			return fmt.Errorf("timezone %q is not available", event.TimeZone)
		}
		loc = loaded
	}
	if req.Start != nil {
		start, err := parseDaemonCalendarTime(*req.Start, loc)
		if err != nil {
			return fmt.Errorf("start: %w", err)
		}
		event.Start = start
	}
	if req.End != nil {
		end, err := parseDaemonCalendarTime(*req.End, loc)
		if err != nil {
			return fmt.Errorf("end: %w", err)
		}
		event.End = end
	}
	if requireTimes && (event.Start.IsZero() || event.End.IsZero()) {
		return fmt.Errorf("start and end are required")
	}
	if !event.Start.IsZero() && !event.End.IsZero() && !event.End.After(event.Start) {
		return fmt.Errorf("end must be after start")
	}
	if len(req.Attendees) > 0 {
		event.Attendees = append([]models.CalendarAttendee(nil), req.Attendees...)
	}
	if len(req.Recurrence) > 0 {
		event.Recurrence = append([]string(nil), req.Recurrence...)
	}
	if len(req.Reminders) > 0 {
		event.Reminders = append([]models.CalendarReminder(nil), req.Reminders...)
	}
	if len(req.Attachments) > 0 {
		event.Attachments = append([]models.CalendarAttachment(nil), req.Attachments...)
	}
	if len(req.AlternateTimeZones) > 0 {
		event.AlternateTimeZones = append([]string(nil), req.AlternateTimeZones...)
	}
	return nil
}

func parseDaemonCalendarTime(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	if loc == nil {
		loc = time.Local
	}
	if parsed, err := time.ParseInLocation(models.CalendarEventEditTimeLayout, value, loc); err == nil {
		return parsed, nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, loc); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("use RFC3339, YYYY-MM-DD, or %s", models.CalendarEventEditTimeLayout)
}

func (s *Server) broadcastCalendarMutation(operation string, ref models.EventRef) {
	if s == nil || s.broadcaster == nil {
		return
	}
	ref = ref.WithDefaults()
	s.broadcastJSON("mutation", mutationEvent{
		Operation:    operation,
		SourceID:     ref.SourceID,
		AccountID:    ref.AccountID,
		CollectionID: ref.CalendarID,
		ItemID:       ref.LocalID,
		LocalID:      ref.LocalID,
	})
}

func firstNonEmptyDaemon(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
