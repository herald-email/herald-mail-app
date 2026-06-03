package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/oauth"
)

type Source interface {
	ID() models.SourceID
	AccountID() models.AccountID
	DisplayName() string
	ListCalendars(context.Context) ([]models.CalendarCollection, error)
	ListEvents(context.Context, models.CollectionRef) ([]models.CalendarEvent, error)
	FetchEvent(context.Context, models.EventRef) (*models.CalendarEvent, error)
	Close() error
}

type MutationSource interface {
	CreateEvent(context.Context, models.CalendarEvent, models.CalendarMutationOptions) (*models.CalendarEvent, error)
	UpdateEvent(context.Context, models.CalendarEvent, models.CalendarMutationOptions) (*models.CalendarEvent, error)
	DeleteEvent(context.Context, models.EventRef, models.CalendarMutationOptions) error
	RespondToEvent(context.Context, models.EventRef, string, models.CalendarMutationOptions) (*models.CalendarEvent, error)
}

type UIDLookupSource interface {
	FindEventByUID(context.Context, models.CollectionRef, string) (*models.CalendarEvent, error)
}

type SyncTokenSource interface {
	ListEventsWithSyncToken(context.Context, models.CollectionRef, string) (CalendarSyncResult, error)
}

type CalendarSyncResult struct {
	Events        []models.CalendarEvent
	DeletedRefs   []models.EventRef
	NextSyncToken string
}

type GoogleCalendarSource struct {
	id            models.SourceID
	accountID     models.AccountID
	displayName   string
	google        config.GoogleConfig
	attendeeEmail string
	baseURL       string
	client        *http.Client
}

func NewGoogleCalendarSource(cfg config.SourceConfig) (*GoogleCalendarSource, error) {
	id := models.NormalizeSourceID(models.SourceID(cfg.ID), models.DefaultCalendarSourceID)
	accountID := models.NormalizeAccountID(models.AccountID(cfg.AccountID))
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.Google.APIBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://www.googleapis.com/calendar/v3"
	}
	return &GoogleCalendarSource{
		id:            id,
		accountID:     accountID,
		displayName:   cfg.DisplayName,
		google:        cfg.Google,
		attendeeEmail: strings.TrimSpace(cfg.Google.Email),
		baseURL:       baseURL,
		client:        http.DefaultClient,
	}, nil
}

func (s *GoogleCalendarSource) ID() models.SourceID         { return s.id }
func (s *GoogleCalendarSource) AccountID() models.AccountID { return s.accountID }
func (s *GoogleCalendarSource) DisplayName() string         { return s.displayName }
func (s *GoogleCalendarSource) Close() error                { return nil }

func (s *GoogleCalendarSource) ListCalendars(ctx context.Context) ([]models.CalendarCollection, error) {
	req, err := s.newRequest(ctx, http.MethodGet, s.baseURL+"/users/me/calendarList", nil)
	if err != nil {
		return nil, err
	}
	var payload googleCalendarList
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	out := make([]models.CalendarCollection, 0, len(payload.Items))
	for _, item := range payload.Items {
		ref := models.CollectionRef{
			SourceID:     s.id,
			AccountID:    s.accountID,
			Kind:         models.SourceKindCalendar,
			CollectionID: item.ID,
			DisplayName:  firstNonEmpty(item.Summary, item.ID),
		}
		out = append(out, models.CalendarCollection{
			Ref:        ref,
			Color:      item.BackgroundColor,
			ETag:       item.ETag,
			AccessRole: item.AccessRole,
		})
	}
	return out, nil
}

func (s *GoogleCalendarSource) ListEvents(ctx context.Context, ref models.CollectionRef) ([]models.CalendarEvent, error) {
	result, err := s.ListEventsWithSyncToken(ctx, ref, "")
	if err != nil {
		return nil, err
	}
	return result.Events, nil
}

func (s *GoogleCalendarSource) ListEventsWithSyncToken(ctx context.Context, ref models.CollectionRef, syncToken string) (CalendarSyncResult, error) {
	ref = s.normalizeCollection(ref)
	endpoint := s.baseURL + "/calendars/" + url.PathEscape(ref.CollectionID) + "/events"
	values := url.Values{}
	if strings.TrimSpace(syncToken) != "" {
		values.Set("syncToken", strings.TrimSpace(syncToken))
		values.Set("showDeleted", "true")
	} else {
		values.Set("singleEvents", "true")
		values.Set("showDeleted", "false")
	}
	var result CalendarSyncResult
	for {
		requestURL := endpoint + "?" + values.Encode()
		req, err := s.newRequest(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return CalendarSyncResult{}, err
		}
		var payload googleEvents
		if err := s.doJSON(req, &payload); err != nil {
			return CalendarSyncResult{}, err
		}
		if payload.NextSyncToken != "" {
			result.NextSyncToken = payload.NextSyncToken
		}
		for _, item := range payload.Items {
			if strings.TrimSpace(syncToken) != "" && normalizeCalendarStatus(item.Status) == "cancelled" {
				result.DeletedRefs = append(result.DeletedRefs, models.EventRef{
					SourceID:   s.id,
					AccountID:  s.accountID,
					CalendarID: ref.CollectionID,
					EventID:    item.ID,
					ETag:       item.ETag,
				}.WithDefaults())
				continue
			}
			event, err := googleEventToModel(s.id, s.accountID, ref.CollectionID, item)
			if err != nil {
				logger.Warn("Skipping Google calendar event %q from %s: %v", item.ID, ref.CollectionID, err)
				continue
			}
			result.Events = append(result.Events, event)
		}
		if payload.NextPageToken == "" {
			break
		}
		values.Set("pageToken", payload.NextPageToken)
	}
	return result, nil
}

func (s *GoogleCalendarSource) FetchEvent(ctx context.Context, ref models.EventRef) (*models.CalendarEvent, error) {
	ref = s.normalizeEvent(ref)
	u := s.baseURL + "/calendars/" + url.PathEscape(ref.CalendarID) + "/events/" + url.PathEscape(ref.EventID)
	req, err := s.newRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var payload googleEventPayload
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	event, err := googleEventToModel(s.id, s.accountID, ref.CalendarID, payload)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *GoogleCalendarSource) CreateEvent(ctx context.Context, event models.CalendarEvent, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	var err error
	opts, err = validateCalendarMutationOptions(opts)
	if err != nil {
		return nil, err
	}
	event.Ref = s.normalizeEvent(event.Ref)
	if strings.TrimSpace(event.ProviderUID) == "" {
		event.ProviderUID = strings.TrimSpace(event.Ref.EventID)
	}
	payload := googleEventFromModel(event)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	u := s.baseURL + "/calendars/" + url.PathEscape(event.Ref.CalendarID) + "/events/import?sendUpdates=none"
	req, err := s.newRequest(ctx, http.MethodPost, u, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	var imported googleEventPayload
	if err := s.doJSON(req, &imported); err != nil {
		return nil, err
	}
	out, err := googleEventToModel(s.id, s.accountID, event.Ref.CalendarID, imported)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *GoogleCalendarSource) UpdateEvent(ctx context.Context, event models.CalendarEvent, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	var err error
	opts, err = validateCalendarMutationOptions(opts)
	if err != nil {
		return nil, err
	}
	event.Ref = s.normalizeEvent(event.Ref)
	payload := googleEventFromModel(event)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	u := s.baseURL + "/calendars/" + url.PathEscape(event.Ref.CalendarID) + "/events/" + url.PathEscape(event.Ref.EventID) + "?sendUpdates=none"
	req, err := s.newRequest(ctx, http.MethodPatch, u, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if ifMatch := firstNonEmpty(opts.IfMatch, event.Ref.ETag); ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	var updated googleEventPayload
	if err := s.doJSON(req, &updated); err != nil {
		return nil, err
	}
	out, err := googleEventToModel(s.id, s.accountID, event.Ref.CalendarID, updated)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *GoogleCalendarSource) DeleteEvent(ctx context.Context, ref models.EventRef, opts models.CalendarMutationOptions) error {
	var err error
	opts, err = validateCalendarMutationOptions(opts)
	if err != nil {
		return err
	}
	ref = s.normalizeEvent(ref)
	u := s.baseURL + "/calendars/" + url.PathEscape(ref.CalendarID) + "/events/" + url.PathEscape(ref.EventID) + "?sendUpdates=none"
	req, err := s.newRequest(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	if ifMatch := firstNonEmpty(opts.IfMatch, ref.ETag); ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	return s.doNoContent(req)
}

func (s *GoogleCalendarSource) FindEventByUID(ctx context.Context, ref models.CollectionRef, uid string) (*models.CalendarEvent, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, nil
	}
	ref = s.normalizeCollection(ref)
	endpoint := s.baseURL + "/calendars/" + url.PathEscape(ref.CollectionID) + "/events"
	values := url.Values{}
	values.Set("iCalUID", uid)
	values.Set("singleEvents", "false")
	values.Set("showDeleted", "false")
	req, err := s.newRequest(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var payload googleEvents
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	for _, item := range payload.Items {
		event, err := googleEventToModel(s.id, s.accountID, ref.CollectionID, item)
		if err != nil {
			logger.Warn("Skipping Google calendar duplicate candidate %q from %s: %v", item.ID, ref.CollectionID, err)
			continue
		}
		return &event, nil
	}
	return nil, nil
}

func (s *GoogleCalendarSource) RespondToEvent(ctx context.Context, ref models.EventRef, status string, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	normalized, err := models.NormalizeCalendarRSVP(status)
	if err != nil {
		return nil, err
	}
	event, err := s.FetchEvent(ctx, ref)
	if err != nil {
		return nil, err
	}
	if opts.IfMatch == "" {
		opts.IfMatch = event.Ref.ETag
	}
	if err := setCalendarAttendeeRSVP(event, s.attendeeEmail, normalized); err != nil {
		return nil, err
	}
	return s.UpdateEvent(ctx, *event, opts)
}

func (s *GoogleCalendarSource) normalizeCollection(ref models.CollectionRef) models.CollectionRef {
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	return ref
}

func (s *GoogleCalendarSource) normalizeEvent(ref models.EventRef) models.EventRef {
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	return ref.WithDefaults()
}

func (s *GoogleCalendarSource) newRequest(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	token, err := oauth.RefreshGoogleConfigIfNeeded(ctx, &s.google)
	if err != nil {
		return nil, calendarProviderOAuthError("google calendar", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func (s *GoogleCalendarSource) doJSON(req *http.Request, out any) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return calendarProviderHTTPError("google calendar", req.Method, resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *GoogleCalendarSource) doNoContent(req *http.Request) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return calendarProviderHTTPError("google calendar", req.Method, resp.StatusCode, body)
	}
	return nil
}

type CalDAVSource struct {
	id             models.SourceID
	accountID      models.AccountID
	displayName    string
	baseURL        string
	discoveredHome string
	username       string
	password       string
	attendeeEmail  string
	client         *http.Client
}

func NewCalDAVSource(cfg config.SourceConfig) (*CalDAVSource, error) {
	baseURL := strings.TrimSpace(cfg.CalDAV.URL)
	if baseURL == "" {
		return nil, fmt.Errorf("caldav URL is required")
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &CalDAVSource{
		id:            models.NormalizeSourceID(models.SourceID(cfg.ID), models.DefaultCalendarSourceID),
		accountID:     models.NormalizeAccountID(models.AccountID(cfg.AccountID)),
		displayName:   cfg.DisplayName,
		baseURL:       baseURL,
		username:      cfg.CalDAV.Username,
		password:      cfg.CalDAV.Password,
		attendeeEmail: strings.TrimSpace(cfg.CalDAV.Username),
		client:        http.DefaultClient,
	}, nil
}

func (s *CalDAVSource) ID() models.SourceID         { return s.id }
func (s *CalDAVSource) AccountID() models.AccountID { return s.accountID }
func (s *CalDAVSource) DisplayName() string         { return s.displayName }
func (s *CalDAVSource) Close() error                { return nil }

func (s *CalDAVSource) ListCalendars(ctx context.Context) ([]models.CalendarCollection, error) {
	homeURL, err := s.calendarHomeURL(ctx)
	if err != nil {
		return nil, err
	}
	ms, err := s.propfind(ctx, homeURL, "1", caldavCalendarListPropfind)
	if err != nil {
		return nil, err
	}
	out := make([]models.CalendarCollection, 0, len(ms.Responses))
	for _, response := range ms.Responses {
		prop := response.FirstProp()
		if prop.ResourceType.IsSet() && !prop.ResourceType.IsCalendar() {
			continue
		}
		responseURL := resolveCalDAVHref(homeURL, response.Href)
		if sameCalDAVURL(responseURL, homeURL) && !prop.ResourceType.IsCalendar() {
			continue
		}
		if prop.SupportedComponents.IsSet() && !prop.SupportedComponents.Supports("VEVENT") {
			continue
		}
		calendarID := calendarIDFromHref(response.Href)
		if calendarID == "" {
			continue
		}
		ref := models.CollectionRef{
			SourceID:     s.id,
			AccountID:    s.accountID,
			Kind:         models.SourceKindCalendar,
			CollectionID: calendarID,
			DisplayName:  firstNonEmpty(prop.DisplayName, calendarID),
		}
		out = append(out, models.CalendarCollection{
			Ref:       ref,
			Color:     prop.CalendarColor,
			SyncToken: prop.SyncToken,
			ETag:      prop.GetETag,
		})
	}
	return out, nil
}

func (s *CalDAVSource) ListEvents(ctx context.Context, ref models.CollectionRef) ([]models.CalendarEvent, error) {
	events, err := s.listEventsByCalendarQuery(ctx, ref)
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (s *CalDAVSource) ListEventsWithSyncToken(ctx context.Context, ref models.CollectionRef, syncToken string) (CalendarSyncResult, error) {
	syncToken = strings.TrimSpace(syncToken)
	if syncToken == "" {
		events, err := s.listEventsByCalendarQuery(ctx, ref)
		if err != nil {
			return CalendarSyncResult{}, err
		}
		return CalendarSyncResult{Events: events}, nil
	}
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	collectionURL, err := s.collectionURL(ctx, ref.CollectionID)
	if err != nil {
		return CalendarSyncResult{}, err
	}
	req, err := s.newRequest(ctx, "REPORT", collectionURL, bytes.NewBufferString(caldavSyncCollectionReport(syncToken)))
	if err != nil {
		return CalendarSyncResult{}, err
	}
	req.Header.Set("Depth", "1")
	var ms davMultiStatus
	if err := s.doXML(req, &ms); err != nil {
		if isCalDAVSyncUnsupported(err) {
			events, fallbackErr := s.listEventsByCalendarQuery(ctx, ref)
			if fallbackErr != nil {
				return CalendarSyncResult{}, fallbackErr
			}
			return CalendarSyncResult{Events: events}, nil
		}
		return CalendarSyncResult{}, err
	}
	events, deletedRefs, err := s.eventsFromMultiStatus(ref, ms)
	if err != nil {
		return CalendarSyncResult{}, err
	}
	return CalendarSyncResult{
		Events:        events,
		DeletedRefs:   deletedRefs,
		NextSyncToken: strings.TrimSpace(ms.SyncToken),
	}, nil
}

func (s *CalDAVSource) listEventsByCalendarQuery(ctx context.Context, ref models.CollectionRef) ([]models.CalendarEvent, error) {
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	collectionURL, err := s.collectionURL(ctx, ref.CollectionID)
	if err != nil {
		return nil, err
	}
	req, err := s.newRequest(ctx, "REPORT", collectionURL, bytes.NewBufferString(caldavCalendarQuery))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	var ms davMultiStatus
	if err := s.doXML(req, &ms); err != nil {
		return nil, err
	}
	events, _, err := s.eventsFromMultiStatus(ref, ms)
	return events, err
}

func (s *CalDAVSource) eventsFromMultiStatus(ref models.CollectionRef, ms davMultiStatus) ([]models.CalendarEvent, []models.EventRef, error) {
	out := make([]models.CalendarEvent, 0, len(ms.Responses))
	deleted := make([]models.EventRef, 0)
	for _, response := range ms.Responses {
		eventID := path.Base(strings.TrimRight(response.Href, "/"))
		if response.IsNotFound() {
			deleted = append(deleted, models.EventRef{
				SourceID:   s.id,
				AccountID:  s.accountID,
				CalendarID: ref.CollectionID,
				EventID:    eventID,
			}.WithDefaults())
			continue
		}
		prop := response.FirstProp()
		if strings.TrimSpace(prop.CalendarData) == "" {
			continue
		}
		event, err := eventFromICS(s.id, s.accountID, ref.CollectionID, eventID, prop.GetETag, prop.CalendarData)
		if err != nil {
			logger.Warn("Skipping CalDAV calendar event %q from %s: %v", eventID, ref.CollectionID, err)
			continue
		}
		out = append(out, *event)
	}
	return out, deleted, nil
}

func (s *CalDAVSource) FetchEvent(ctx context.Context, ref models.EventRef) (*models.CalendarEvent, error) {
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	ref = ref.WithDefaults()
	eventURL, err := s.eventURL(ctx, ref.CalendarID, ref.EventID)
	if err != nil {
		return nil, err
	}
	req, err := s.newRequest(ctx, http.MethodGet, eventURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("caldav GET %s: %s", req.URL.Path, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return eventFromICS(s.id, s.accountID, ref.CalendarID, ref.EventID, resp.Header.Get("ETag"), string(data))
}

func (s *CalDAVSource) CreateEvent(ctx context.Context, event models.CalendarEvent, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	var err error
	opts, err = validateCalendarMutationOptions(opts)
	if err != nil {
		return nil, err
	}
	event.Ref.SourceID = models.NormalizeSourceID(event.Ref.SourceID, s.id)
	event.Ref.AccountID = models.NormalizeAccountID(event.Ref.AccountID)
	if strings.TrimSpace(event.Ref.EventID) == "" {
		event.Ref.EventID = safeCalDAVEventID(firstNonEmpty(event.ProviderUID, event.Title, "mail-invitation"))
	}
	event.Ref = event.Ref.WithDefaults()
	eventURL, err := s.eventURL(ctx, event.Ref.CalendarID, event.Ref.EventID)
	if err != nil {
		return nil, err
	}
	req, err := s.newRequest(ctx, http.MethodPut, eventURL, strings.NewReader(eventToICS(event)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	req.Header.Set("If-None-Match", "*")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, s.calendarHTTPError(req.Method, resp.StatusCode, body)
	}
	freshRef := event.Ref
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
		freshRef.ETag = etag
	}
	return s.FetchEvent(ctx, freshRef)
}

func (s *CalDAVSource) UpdateEvent(ctx context.Context, event models.CalendarEvent, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	var err error
	opts, err = validateCalendarMutationOptions(opts)
	if err != nil {
		return nil, err
	}
	event.Ref.SourceID = models.NormalizeSourceID(event.Ref.SourceID, s.id)
	event.Ref.AccountID = models.NormalizeAccountID(event.Ref.AccountID)
	event.Ref = event.Ref.WithDefaults()
	eventURL, err := s.eventURL(ctx, event.Ref.CalendarID, event.Ref.EventID)
	if err != nil {
		return nil, err
	}
	req, err := s.newRequest(ctx, http.MethodPut, eventURL, strings.NewReader(eventToICS(event)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	if ifMatch := firstNonEmpty(opts.IfMatch, event.Ref.ETag); ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, calendarProviderHTTPError("caldav", req.Method, resp.StatusCode, body)
	}
	freshRef := event.Ref
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
		freshRef.ETag = etag
	}
	return s.FetchEvent(ctx, freshRef)
}

func (s *CalDAVSource) DeleteEvent(ctx context.Context, ref models.EventRef, opts models.CalendarMutationOptions) error {
	var err error
	opts, err = validateCalendarMutationOptions(opts)
	if err != nil {
		return err
	}
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	ref = ref.WithDefaults()
	eventURL, err := s.eventURL(ctx, ref.CalendarID, ref.EventID)
	if err != nil {
		return err
	}
	req, err := s.newRequest(ctx, http.MethodDelete, eventURL, nil)
	if err != nil {
		return err
	}
	if ifMatch := firstNonEmpty(opts.IfMatch, ref.ETag); ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return calendarProviderHTTPError("caldav", req.Method, resp.StatusCode, body)
	}
	return nil
}

func (s *CalDAVSource) FindEventByUID(ctx context.Context, ref models.CollectionRef, uid string) (*models.CalendarEvent, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, nil
	}
	events, err := s.ListEvents(ctx, ref)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if strings.TrimSpace(event.ProviderUID) == uid {
			found := event
			return &found, nil
		}
	}
	return nil, nil
}

func (s *CalDAVSource) RespondToEvent(ctx context.Context, ref models.EventRef, status string, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	normalized, err := models.NormalizeCalendarRSVP(status)
	if err != nil {
		return nil, err
	}
	event, err := s.FetchEvent(ctx, ref)
	if err != nil {
		return nil, err
	}
	if opts.IfMatch == "" {
		opts.IfMatch = event.Ref.ETag
	}
	if err := setCalendarAttendeeRSVP(event, s.attendeeEmail, normalized); err != nil {
		return nil, err
	}
	return s.UpdateEvent(ctx, *event, opts)
}

func (s *CalDAVSource) newRequest(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	var bodyReader io.Reader
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, err
	}
	if bodyBytes != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		req.ContentLength = int64(len(bodyBytes))
	}
	if s.username != "" || s.password != "" {
		req.SetBasicAuth(s.username, s.password)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	return req, nil
}

func (s *CalDAVSource) calendarHomeURL(ctx context.Context) (string, error) {
	if strings.TrimSpace(s.discoveredHome) != "" {
		return s.discoveredHome, nil
	}
	homeURL, err := s.discoverCalendarHome(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(homeURL) == "" {
		homeURL = s.baseURL
	}
	if !strings.HasSuffix(homeURL, "/") {
		homeURL += "/"
	}
	s.discoveredHome = homeURL
	return homeURL, nil
}

func (s *CalDAVSource) discoverCalendarHome(ctx context.Context) (string, error) {
	ms, err := s.propfind(ctx, s.baseURL, "0", caldavDiscoveryPropfind)
	if err != nil {
		if isCalDAVDiscoveryOptionalError(err) {
			return "", nil
		}
		return "", err
	}
	for _, response := range ms.Responses {
		prop := response.FirstProp()
		if href := strings.TrimSpace(prop.CalendarHomeSet.Href); href != "" {
			return resolveCalDAVHref(s.baseURL, href), nil
		}
		if href := strings.TrimSpace(prop.CurrentUserPrincipal.Href); href != "" {
			principalURL := resolveCalDAVHref(s.baseURL, href)
			principal, err := s.propfind(ctx, principalURL, "0", caldavDiscoveryPropfind)
			if err != nil {
				return "", err
			}
			for _, principalResponse := range principal.Responses {
				if homeHref := strings.TrimSpace(principalResponse.FirstProp().CalendarHomeSet.Href); homeHref != "" {
					return resolveCalDAVHref(principalURL, homeHref), nil
				}
			}
		}
	}
	return "", nil
}

func (s *CalDAVSource) propfind(ctx context.Context, targetURL, depth, body string) (davMultiStatus, error) {
	req, err := s.newRequest(ctx, "PROPFIND", targetURL, strings.NewReader(body))
	if err != nil {
		return davMultiStatus{}, err
	}
	req.Header.Set("Depth", depth)
	var ms davMultiStatus
	if err := s.doXML(req, &ms); err != nil {
		return davMultiStatus{}, err
	}
	return ms, nil
}

func (s *CalDAVSource) doXML(req *http.Request, out any) error {
	resp, finalReq, err := s.doDAVXMLRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return s.calendarHTTPError(finalReq.Method, resp.StatusCode, body)
	}
	return xml.NewDecoder(resp.Body).Decode(out)
}

func (s *CalDAVSource) doDAVXMLRequest(req *http.Request) (*http.Response, *http.Request, error) {
	current := req
	for redirects := 0; ; redirects++ {
		logger.Debug(
			"CalDAV XML request: source=%s method=%s host=%s path_kind=%s auth_present=%t",
			s.id,
			current.Method,
			normalizedCalDAVHost(current.URL.Host),
			caldavDebugPathKind(current.URL.Path),
			current.Header.Get("Authorization") != "",
		)
		resp, err := s.noRedirectClient().Do(current)
		if err != nil {
			logger.Debug(
				"CalDAV XML request error: source=%s method=%s host=%s error=%s",
				s.id,
				current.Method,
				normalizedCalDAVHost(current.URL.Host),
				err,
			)
			return nil, current, err
		}
		logger.Debug(
			"CalDAV XML response: source=%s method=%s host=%s status=%d auth_challenges=%s redirect_host=%s",
			s.id,
			current.Method,
			normalizedCalDAVHost(current.URL.Host),
			resp.StatusCode,
			caldavAuthChallengeSchemes(resp.Header),
			caldavRedirectHost(current.URL, resp.Header.Get("Location")),
		)
		if !isCalDAVRedirect(resp.StatusCode) || resp.Header.Get("Location") == "" {
			return resp, current, nil
		}
		if redirects >= 10 {
			_ = resp.Body.Close()
			return nil, current, fmt.Errorf("caldav %s failed: stopped after 10 redirects", strings.ToLower(current.Method))
		}
		next, err := redirectedCalDAVRequest(current, resp.Header.Get("Location"))
		_ = resp.Body.Close()
		if err != nil {
			return nil, current, err
		}
		logger.Debug(
			"CalDAV XML redirect: source=%s method=%s from_host=%s to_host=%s auth_forwarded=%t",
			s.id,
			current.Method,
			normalizedCalDAVHost(current.URL.Host),
			normalizedCalDAVHost(next.URL.Host),
			next.Header.Get("Authorization") != "",
		)
		current = next
	}
}

func (s *CalDAVSource) noRedirectClient() *http.Client {
	base := http.DefaultClient
	if s.client != nil {
		base = s.client
	}
	next := *base
	next.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &next
}

func redirectedCalDAVRequest(previous *http.Request, location string) (*http.Request, error) {
	ref, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	nextURL := previous.URL.ResolveReference(ref)
	var bodyBytes []byte
	if previous.GetBody != nil {
		body, err := previous.GetBody()
		if err != nil {
			return nil, err
		}
		bodyBytes, err = io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			return nil, err
		}
	}
	next, err := http.NewRequestWithContext(previous.Context(), previous.Method, nextURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	if previous.GetBody != nil {
		next.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		next.ContentLength = int64(len(bodyBytes))
	}
	next.Header = previous.Header.Clone()
	if !shouldForwardCalDAVAuthorization(previous.URL.Host, nextURL.Host) {
		next.Header.Del("Authorization")
	}
	return next, nil
}

func isCalDAVRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func shouldForwardCalDAVAuthorization(fromHost, toHost string) bool {
	fromHost = normalizedCalDAVHost(fromHost)
	toHost = normalizedCalDAVHost(toHost)
	if fromHost == "" || toHost == "" {
		return false
	}
	if fromHost == toHost {
		return true
	}
	return isTrustedICloudCalDAVHost(fromHost) && isTrustedICloudCalDAVHost(toHost)
}

func normalizedCalDAVHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return strings.TrimSuffix(host, ".")
}

func isTrustedICloudCalDAVHost(host string) bool {
	host = normalizedCalDAVHost(host)
	return host == "caldav.icloud.com" || strings.HasSuffix(host, ".icloud.com")
}

func caldavDebugPathKind(rawPath string) string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || rawPath == "/" {
		return "root"
	}
	segments := strings.Split(strings.Trim(rawPath, "/"), "/")
	for _, segment := range segments {
		switch strings.ToLower(segment) {
		case "principal":
			return "principal"
		case "calendars":
			return "calendars"
		}
	}
	return "other"
}

func caldavAuthChallengeSchemes(header http.Header) string {
	values := header.Values("WWW-Authenticate")
	if len(values) == 0 {
		return "none"
	}
	seen := make(map[string]bool, len(values))
	var schemes []string
	for _, value := range values {
		scheme := strings.TrimSpace(value)
		if scheme == "" {
			continue
		}
		if fields := strings.Fields(scheme); len(fields) > 0 {
			scheme = fields[0]
		}
		scheme = strings.Trim(strings.ToLower(scheme), `"'`)
		if scheme != "" && !seen[scheme] {
			seen[scheme] = true
			schemes = append(schemes, scheme)
		}
	}
	if len(schemes) == 0 {
		return "none"
	}
	sort.Strings(schemes)
	return strings.Join(schemes, ",")
}

func caldavRedirectHost(base *url.URL, location string) string {
	if strings.TrimSpace(location) == "" {
		return "none"
	}
	ref, err := url.Parse(location)
	if err != nil {
		return "invalid"
	}
	if base != nil {
		ref = base.ResolveReference(ref)
	}
	host := normalizedCalDAVHost(ref.Host)
	if host == "" {
		return "none"
	}
	return host
}

func (s *CalDAVSource) collectionURL(ctx context.Context, calendarID string) (string, error) {
	homeURL, err := s.calendarHomeURL(ctx)
	if err != nil {
		return "", err
	}
	return homeURL + url.PathEscape(calendarID) + "/", nil
}

func (s *CalDAVSource) eventURL(ctx context.Context, calendarID, eventID string) (string, error) {
	collectionURL, err := s.collectionURL(ctx, calendarID)
	if err != nil {
		return "", err
	}
	return collectionURL + url.PathEscape(eventID), nil
}

const caldavDiscoveryPropfind = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:current-user-principal/><cal:calendar-home-set/></d:prop>
</d:propfind>`

const caldavCalendarListPropfind = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/">
  <d:prop><d:displayname/><d:resourcetype/><cal:supported-calendar-component-set/><cs:calendar-color/><d:getetag/><d:sync-token/></d:prop>
</d:propfind>`

const caldavCalendarQuery = `<?xml version="1.0" encoding="utf-8"?>
<cal:calendar-query xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:getetag/><cal:calendar-data/></d:prop>
  <cal:filter><cal:comp-filter name="VCALENDAR"><cal:comp-filter name="VEVENT"/></cal:comp-filter></cal:filter>
</cal:calendar-query>`

func caldavSyncCollectionReport(syncToken string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<d:sync-collection xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav">
  <d:sync-token>` + escapeXML(syncToken) + `</d:sync-token>
  <d:sync-level>1</d:sync-level>
  <d:prop><d:getetag/><cal:calendar-data/></d:prop>
</d:sync-collection>`
}

type googleCalendarList struct {
	Items []struct {
		ID              string `json:"id"`
		Summary         string `json:"summary"`
		BackgroundColor string `json:"backgroundColor"`
		ETag            string `json:"etag"`
		AccessRole      string `json:"accessRole"`
	} `json:"items"`
}

type googleEvents struct {
	Items         []googleEventPayload `json:"items"`
	NextPageToken string               `json:"nextPageToken"`
	NextSyncToken string               `json:"nextSyncToken"`
}

type googleEventPayload struct {
	ID          string             `json:"id"`
	ICalUID     string             `json:"iCalUID"`
	ETag        string             `json:"etag"`
	Summary     string             `json:"summary"`
	Description string             `json:"description"`
	Location    string             `json:"location"`
	Status      string             `json:"status"`
	Updated     string             `json:"updated"`
	Start       googleEventTime    `json:"start"`
	End         googleEventTime    `json:"end"`
	Sequence    int                `json:"sequence"`
	Organizer   googlePerson       `json:"organizer"`
	Attendees   []googleAttendee   `json:"attendees"`
	Recurrence  []string           `json:"recurrence"`
	Attachments []googleAttachment `json:"attachments"`
	Reminders   *googleReminders   `json:"reminders,omitempty"`
	Extended    map[string]any     `json:"extendedProperties"`
}

type googleEventTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
	TimeZone string `json:"timeZone"`
}

type googlePerson struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type googleAttendee struct {
	DisplayName    string `json:"displayName"`
	Email          string `json:"email"`
	ResponseStatus string `json:"responseStatus"`
	Optional       bool   `json:"optional"`
}

type googleAttachment struct {
	Title    string `json:"title"`
	FileURL  string `json:"fileUrl"`
	MIMEType string `json:"mimeType"`
}

type googleReminders struct {
	UseDefault bool             `json:"useDefault"`
	Overrides  []googleReminder `json:"overrides"`
}

type googleReminder struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

func googleEventToModel(sourceID models.SourceID, accountID models.AccountID, calendarID string, item googleEventPayload) (models.CalendarEvent, error) {
	start, allDay, ok := parseGoogleTime(item.Start)
	if !ok {
		return models.CalendarEvent{}, fmt.Errorf("calendar event %s has invalid start time", firstNonEmpty(item.ID, item.ICalUID, "(unknown)"))
	}
	var end time.Time
	if !googleTimeEmpty(item.End) {
		var endOK bool
		end, _, endOK = parseGoogleTime(item.End)
		if !endOK {
			return models.CalendarEvent{}, fmt.Errorf("calendar event %s has invalid end time", firstNonEmpty(item.ID, item.ICalUID, "(unknown)"))
		}
	}
	updated, _ := parseRFC3339(item.Updated)
	ref := models.EventRef{
		SourceID:   sourceID,
		AccountID:  accountID,
		CalendarID: calendarID,
		EventID:    item.ID,
		ETag:       item.ETag,
	}.WithDefaults()
	return models.CalendarEvent{
		Ref:               ref,
		ProviderUID:       item.ICalUID,
		Title:             item.Summary,
		Description:       item.Description,
		Location:          item.Location,
		Start:             start,
		End:               end,
		AllDay:            allDay,
		TimeZone:          firstNonEmpty(item.Start.TimeZone, item.End.TimeZone),
		Status:            item.Status,
		Organizer:         item.Organizer.DisplayName,
		OrganizerEmail:    item.Organizer.Email,
		Attendees:         googleAttendeesToModel(item.Attendees),
		Recurrence:        append([]string(nil), item.Recurrence...),
		RecurrenceSummary: summarizeRecurrence(item.Recurrence),
		Attachments:       googleAttachmentsToModel(item.Attachments),
		Reminders:         googleRemindersToModel(item.Reminders),
		Revision:          fmt.Sprintf("%d", item.Sequence),
		UpdatedAt:         updated,
	}, nil
}

func googleEventFromModel(event models.CalendarEvent) googleEventPayload {
	payload := googleEventPayload{
		ID:          event.Ref.EventID,
		ICalUID:     event.ProviderUID,
		ETag:        event.Ref.ETag,
		Summary:     event.Title,
		Description: event.Description,
		Location:    event.Location,
		Status:      event.Status,
		Sequence:    0,
		Recurrence:  append([]string(nil), event.Recurrence...),
		Reminders:   googleRemindersFromModel(event.Reminders),
	}
	payload.Start = googleTimeFromModel(event.Start, event.TimeZone, event.AllDay)
	payload.End = googleTimeFromModel(event.End, event.TimeZone, event.AllDay)
	if event.Organizer != "" || event.OrganizerEmail != "" {
		payload.Organizer = googlePerson{DisplayName: event.Organizer, Email: event.OrganizerEmail}
	}
	for _, attendee := range event.Attendees {
		payload.Attendees = append(payload.Attendees, googleAttendee{
			DisplayName:    attendee.Name,
			Email:          attendee.Email,
			ResponseStatus: googleRSVPStatus(attendee.RSVP),
			Optional:       attendee.Optional,
		})
	}
	for _, attachment := range event.Attachments {
		payload.Attachments = append(payload.Attachments, googleAttachment{
			Title:    attachment.Title,
			FileURL:  attachment.URI,
			MIMEType: attachment.MIMEType,
		})
	}
	return payload
}

func googleTimeFromModel(value time.Time, timezone string, allDay bool) googleEventTime {
	if value.IsZero() {
		return googleEventTime{}
	}
	if allDay {
		return googleEventTime{Date: value.Format("2006-01-02")}
	}
	timezone = strings.TrimSpace(timezone)
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			return googleEventTime{DateTime: value.In(loc).Format(time.RFC3339), TimeZone: timezone}
		}
	}
	return googleEventTime{DateTime: value.Format(time.RFC3339), TimeZone: timezone}
}

func googleAttendeesToModel(attendees []googleAttendee) []models.CalendarAttendee {
	out := make([]models.CalendarAttendee, 0, len(attendees))
	for _, attendee := range attendees {
		out = append(out, models.CalendarAttendee{
			Name:     attendee.DisplayName,
			Email:    attendee.Email,
			RSVP:     normalizeCalendarStatus(attendee.ResponseStatus),
			Optional: attendee.Optional,
		})
	}
	return out
}

func googleAttachmentsToModel(attachments []googleAttachment) []models.CalendarAttachment {
	out := make([]models.CalendarAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, models.CalendarAttachment{
			Title:    attachment.Title,
			URI:      attachment.FileURL,
			MIMEType: attachment.MIMEType,
		})
	}
	return out
}

func googleRemindersToModel(reminders *googleReminders) []models.CalendarReminder {
	if reminders == nil {
		return nil
	}
	out := make([]models.CalendarReminder, 0, len(reminders.Overrides))
	for _, reminder := range reminders.Overrides {
		if reminder.Minutes < 0 {
			continue
		}
		out = append(out, models.CalendarReminder{
			Method:        strings.TrimSpace(strings.ToLower(reminder.Method)),
			MinutesBefore: reminder.Minutes,
		})
	}
	return out
}

func googleRemindersFromModel(reminders []models.CalendarReminder) *googleReminders {
	if reminders == nil {
		return nil
	}
	out := googleReminders{UseDefault: false}
	for _, reminder := range reminders {
		if reminder.MinutesBefore < 0 {
			continue
		}
		method := strings.TrimSpace(strings.ToLower(reminder.Method))
		if method == "" {
			method = "popup"
		}
		out.Overrides = append(out.Overrides, googleReminder{
			Method:  method,
			Minutes: reminder.MinutesBefore,
		})
	}
	return &out
}

func googleTimeEmpty(t googleEventTime) bool {
	return strings.TrimSpace(t.Date) == "" && strings.TrimSpace(t.DateTime) == ""
}

func parseGoogleTime(t googleEventTime) (time.Time, bool, bool) {
	if strings.TrimSpace(t.Date) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(t.Date), time.Local)
		return parsed, true, err == nil
	}
	parsed, ok := parseRFC3339(t.DateTime)
	return parsed, false, ok
}

type davMultiStatus struct {
	Responses []davResponse `xml:"response"`
	SyncToken string        `xml:"sync-token"`
}

type davResponse struct {
	Href     string        `xml:"href"`
	Status   string        `xml:"status"`
	Propstat []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop   davProp `xml:"prop"`
	Status string  `xml:"status"`
}

type davProp struct {
	DisplayName          string                           `xml:"displayname"`
	CalendarColor        string                           `xml:"calendar-color"`
	GetETag              string                           `xml:"getetag"`
	SyncToken            string                           `xml:"sync-token"`
	CalendarData         string                           `xml:"calendar-data"`
	ResourceType         davResourceType                  `xml:"resourcetype"`
	SupportedComponents  davSupportedCalendarComponentSet `xml:"supported-calendar-component-set"`
	CurrentUserPrincipal davHref                          `xml:"current-user-principal"`
	CalendarHomeSet      davHref                          `xml:"calendar-home-set"`
}

type davHref struct {
	Href string `xml:"href"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection"`
	Calendar   *struct{} `xml:"calendar"`
}

func (r davResourceType) IsSet() bool {
	return r.Collection != nil || r.Calendar != nil
}

func (r davResourceType) IsCalendar() bool {
	return r.Calendar != nil
}

type davSupportedCalendarComponentSet struct {
	Components []davCalendarComponent `xml:"comp"`
}

type davCalendarComponent struct {
	Name string `xml:"name,attr"`
}

func (s davSupportedCalendarComponentSet) IsSet() bool {
	return len(s.Components) > 0
}

func (s davSupportedCalendarComponentSet) Supports(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, component := range s.Components {
		if strings.EqualFold(strings.TrimSpace(component.Name), name) {
			return true
		}
	}
	return false
}

func (r davResponse) FirstProp() davProp {
	if len(r.Propstat) == 0 {
		return davProp{}
	}
	return r.Propstat[0].Prop
}

func (r davResponse) IsNotFound() bool {
	if strings.Contains(r.Status, "404") {
		return true
	}
	for _, propstat := range r.Propstat {
		if strings.Contains(propstat.Status, "404") {
			return true
		}
	}
	return false
}

func calendarIDFromHref(href string) string {
	parsed, err := url.Parse(href)
	if err == nil && parsed.Path != "" {
		href = parsed.Path
	}
	trimmed := strings.Trim(href, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	id := parts[len(parts)-1]
	if id == "caldav" {
		return ""
	}
	return id
}

func resolveCalDAVHref(baseURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func sameCalDAVURL(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

func safeCalDAVEventID(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		uid = "mail-invitation"
	}
	var b strings.Builder
	for _, r := range uid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case strings.ContainsRune("-_.@", r):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	id := strings.Trim(b.String(), "._-")
	if id == "" {
		id = "mail-invitation"
	}
	if !strings.HasSuffix(strings.ToLower(id), ".ics") {
		id += ".ics"
	}
	return id
}

// EventFromICS parses the first VEVENT in an iCalendar payload into Herald's
// calendar event model.
func EventFromICS(sourceID models.SourceID, accountID models.AccountID, calendarID, eventID, etag, data string) (*models.CalendarEvent, error) {
	return eventFromICS(sourceID, accountID, calendarID, eventID, etag, data)
}

func eventFromICS(sourceID models.SourceID, accountID models.AccountID, calendarID, eventID, etag, data string) (*models.CalendarEvent, error) {
	fields := parseICS(data)
	details := parseICSRichDetails(data)
	start, allDay, err := parseICSTimeWithZone(fields["DTSTART"], details.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("calendar event %s has invalid DTSTART %q: %w", firstNonEmpty(eventID, fields["UID"], "(unknown)"), fields["DTSTART"], err)
	}
	var end time.Time
	if strings.TrimSpace(fields["DTEND"]) != "" {
		end, _, err = parseICSTimeWithZone(fields["DTEND"], details.TimeZone)
		if err != nil {
			return nil, fmt.Errorf("calendar event %s has invalid DTEND %q: %w", firstNonEmpty(eventID, fields["UID"], "(unknown)"), fields["DTEND"], err)
		}
	}
	updated, _ := parseICSTime(fields["LAST-MODIFIED"])
	ref := models.EventRef{
		SourceID:   sourceID,
		AccountID:  accountID,
		CalendarID: calendarID,
		EventID:    eventID,
		ETag:       etag,
	}.WithDefaults()
	return &models.CalendarEvent{
		Ref:               ref,
		ProviderUID:       fields["UID"],
		Title:             fields["SUMMARY"],
		Description:       fields["DESCRIPTION"],
		Location:          fields["LOCATION"],
		Start:             start,
		End:               end,
		AllDay:            allDay,
		TimeZone:          details.TimeZone,
		Status:            fields["STATUS"],
		Organizer:         details.Organizer,
		OrganizerEmail:    details.OrganizerEmail,
		Attendees:         details.Attendees,
		Recurrence:        details.Recurrence,
		RecurrenceSummary: summarizeRecurrence(details.Recurrence),
		Attachments:       details.Attachments,
		Reminders:         details.Reminders,
		UpdatedAt:         updated,
		Raw:               data,
	}, nil
}

func EventFromInvitationICS(sourceID models.SourceID, accountID models.AccountID, calendarID, data string) (*models.CalendarEvent, error) {
	fields := parseICS(data)
	eventID := strings.TrimSpace(fields["UID"])
	if eventID == "" {
		eventID = fmt.Sprintf("invitation-%d", time.Now().UnixNano())
	}
	return eventFromICS(sourceID, accountID, calendarID, eventID, "", data)
}

func eventToICS(event models.CalendarEvent) string {
	event.Ref = event.Ref.WithDefaults()
	uid := firstNonEmpty(event.ProviderUID, event.Ref.EventID)
	status := strings.ToUpper(strings.TrimSpace(event.Status))
	if status == "" {
		status = "CONFIRMED"
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Herald//Calendar Mutation//EN\r\nBEGIN:VEVENT\r\n")
	writeCalendarICSLine(&b, "UID", uid)
	writeCalendarICSLine(&b, "SUMMARY", event.Title)
	writeCalendarICSLine(&b, "DESCRIPTION", event.Description)
	writeCalendarICSLine(&b, "LOCATION", event.Location)
	writeCalendarICSLine(&b, "STATUS", status)
	if event.AllDay {
		writeCalendarICSLine(&b, "DTSTART;VALUE=DATE", event.Start.Format("20060102"))
		writeCalendarICSLine(&b, "DTEND;VALUE=DATE", event.End.Format("20060102"))
	} else if timezone := strings.TrimSpace(event.TimeZone); timezone != "" {
		loc := event.Start.Location()
		if loaded, err := time.LoadLocation(timezone); err == nil {
			loc = loaded
		}
		writeCalendarICSLine(&b, "DTSTART;TZID="+timezone, event.Start.In(loc).Format("20060102T150405"))
		writeCalendarICSLine(&b, "DTEND;TZID="+timezone, event.End.In(loc).Format("20060102T150405"))
	} else {
		writeCalendarICSLine(&b, "DTSTART", event.Start.UTC().Format("20060102T150405Z"))
		writeCalendarICSLine(&b, "DTEND", event.End.UTC().Format("20060102T150405Z"))
	}
	if event.Organizer != "" || event.OrganizerEmail != "" {
		writeCalendarICSLine(&b, "ORGANIZER;CN="+event.Organizer, "mailto:"+event.OrganizerEmail)
	}
	for _, attendee := range event.Attendees {
		role := "REQ-PARTICIPANT"
		if attendee.Optional {
			role = "OPT-PARTICIPANT"
		}
		key := "ATTENDEE;CN=" + attendee.Name + ";PARTSTAT=" + icsRSVPStatus(attendee.RSVP) + ";ROLE=" + role
		writeCalendarICSLine(&b, key, "mailto:"+attendee.Email)
	}
	for _, rule := range event.Recurrence {
		key, value, ok := strings.Cut(rule, ":")
		if !ok {
			continue
		}
		writeCalendarICSLine(&b, key, value)
	}
	for _, attachment := range event.Attachments {
		key := "ATTACH;FILENAME=" + attachment.Title
		if attachment.MIMEType != "" {
			key += ";FMTTYPE=" + attachment.MIMEType
		}
		writeCalendarICSLine(&b, key, attachment.URI)
	}
	for _, reminder := range event.Reminders {
		method := strings.TrimSpace(strings.ToLower(reminder.Method))
		action := "DISPLAY"
		if method == "email" {
			action = "EMAIL"
		}
		b.WriteString("BEGIN:VALARM\r\n")
		writeCalendarICSLine(&b, "ACTION", action)
		writeCalendarICSLine(&b, "TRIGGER", formatICSReminderTrigger(reminder.MinutesBefore))
		if action == "EMAIL" {
			writeCalendarICSLine(&b, "SUMMARY", event.Title)
			writeCalendarICSLine(&b, "DESCRIPTION", event.Description)
		}
		b.WriteString("END:VALARM\r\n")
	}
	if !event.UpdatedAt.IsZero() {
		writeCalendarICSLine(&b, "LAST-MODIFIED", event.UpdatedAt.UTC().Format("20060102T150405Z"))
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

func writeCalendarICSLine(b *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	b.WriteString(key)
	b.WriteString(":")
	b.WriteString(escapeICSValue(value))
	b.WriteString("\r\n")
}

func escapeICSValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", "", ",", `\,`, ";", `\;`).Replace(value)
}

type icsRichDetails struct {
	TimeZone       string
	Organizer      string
	OrganizerEmail string
	Attendees      []models.CalendarAttendee
	Recurrence     []string
	Attachments    []models.CalendarAttachment
	Reminders      []models.CalendarReminder
}

func parseICSRichDetails(data string) icsRichDetails {
	var details icsRichDetails
	inAlarm := false
	alarmAction := ""
	alarmTrigger := ""
	for _, line := range firstVEVENTLines(data) {
		nameAndParams, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		parts := strings.Split(nameAndParams, ";")
		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		params := parseICSParams(parts[1:])
		value = unescapeICSValue(value)
		if key == "BEGIN" && strings.EqualFold(value, "VALARM") {
			inAlarm = true
			alarmAction = ""
			alarmTrigger = ""
			continue
		}
		if key == "END" && strings.EqualFold(value, "VALARM") {
			if reminder, ok := calendarReminderFromAlarm(alarmAction, alarmTrigger); ok {
				details.Reminders = append(details.Reminders, reminder)
			}
			inAlarm = false
			continue
		}
		if inAlarm {
			switch key {
			case "ACTION":
				alarmAction = value
			case "TRIGGER":
				alarmTrigger = value
			}
			continue
		}
		switch key {
		case "DTSTART":
			if details.TimeZone == "" {
				details.TimeZone = params["TZID"]
			}
		case "ORGANIZER":
			details.Organizer = firstNonEmpty(params["CN"], value)
			details.OrganizerEmail = calendarMailtoAddress(value)
		case "ATTENDEE":
			details.Attendees = append(details.Attendees, models.CalendarAttendee{
				Name:     params["CN"],
				Email:    calendarMailtoAddress(value),
				RSVP:     normalizeCalendarStatus(params["PARTSTAT"]),
				Optional: strings.EqualFold(params["ROLE"], "OPT-PARTICIPANT"),
			})
		case "RRULE", "RDATE", "EXDATE":
			details.Recurrence = append(details.Recurrence, key+":"+value)
		case "ATTACH":
			details.Attachments = append(details.Attachments, models.CalendarAttachment{
				Title:    firstNonEmpty(params["FILENAME"], params["X-FILENAME"], "Attachment"),
				URI:      value,
				MIMEType: params["FMTTYPE"],
			})
		}
	}
	return details
}

func calendarReminderFromAlarm(action, trigger string) (models.CalendarReminder, bool) {
	minutes, ok := parseICSReminderTrigger(trigger)
	if !ok {
		return models.CalendarReminder{}, false
	}
	method := "popup"
	if strings.EqualFold(strings.TrimSpace(action), "EMAIL") {
		method = "email"
	}
	return models.CalendarReminder{Method: method, MinutesBefore: minutes}, true
}

func parseICSReminderTrigger(trigger string) (int, bool) {
	trigger = strings.TrimSpace(strings.ToUpper(trigger))
	if !strings.HasPrefix(trigger, "-P") {
		return 0, false
	}
	trigger = strings.TrimPrefix(trigger, "-P")
	if strings.HasPrefix(trigger, "T") {
		trigger = strings.TrimPrefix(trigger, "T")
	}
	multiplier := 1
	switch {
	case strings.HasSuffix(trigger, "M"):
		trigger = strings.TrimSuffix(trigger, "M")
	case strings.HasSuffix(trigger, "H"):
		trigger = strings.TrimSuffix(trigger, "H")
		multiplier = 60
	case strings.HasSuffix(trigger, "D"):
		trigger = strings.TrimSuffix(trigger, "D")
		multiplier = 24 * 60
	default:
		return 0, false
	}
	value, err := strconv.Atoi(trigger)
	if err != nil || value < 0 {
		return 0, false
	}
	return value * multiplier, true
}

func formatICSReminderTrigger(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	if minutes%1440 == 0 && minutes != 0 {
		return fmt.Sprintf("-P%dD", minutes/1440)
	}
	if minutes%60 == 0 && minutes != 0 {
		return fmt.Sprintf("-PT%dH", minutes/60)
	}
	return fmt.Sprintf("-PT%dM", minutes)
}

func parseICSParams(parts []string) map[string]string {
	params := make(map[string]string, len(parts))
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		params[strings.ToUpper(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(unescapeICSValue(value)), `"`)
	}
	return params
}

func calendarMailtoAddress(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "mailto:") {
		return strings.TrimSpace(value[len("mailto:"):])
	}
	if strings.Contains(value, "@") {
		return value
	}
	return ""
}

func parseICS(data string) map[string]string {
	lines := firstVEVENTLines(data)
	out := make(map[string]string)
	for _, line := range lines {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.Split(key, ";")[0])
		out[key] = unescapeICSValue(value)
	}
	return out
}

func firstVEVENTLines(data string) []string {
	lines := unfoldICSLines(data)
	var eventLines []string
	inEvent := false
	nestedDepth := 0
	for _, line := range lines {
		nameAndParams, value, ok := strings.Cut(line, ":")
		if !ok {
			if inEvent {
				eventLines = append(eventLines, line)
			}
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(strings.Split(nameAndParams, ";")[0]))
		component := strings.ToUpper(strings.TrimSpace(value))
		switch key {
		case "BEGIN":
			if component == "VEVENT" && !inEvent {
				inEvent = true
				nestedDepth = 0
				continue
			}
			if inEvent {
				nestedDepth++
				eventLines = append(eventLines, line)
			}
			continue
		case "END":
			if !inEvent {
				continue
			}
			if component == "VEVENT" && nestedDepth == 0 {
				return eventLines
			}
			if nestedDepth > 0 {
				nestedDepth--
			}
			eventLines = append(eventLines, line)
			continue
		}
		if inEvent {
			eventLines = append(eventLines, line)
		}
	}
	if len(eventLines) > 0 {
		return eventLines
	}
	return lines
}

func unfoldICSLines(data string) []string {
	raw := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if line == "" {
			continue
		}
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += strings.TrimLeft(line, " \t")
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func parseICSTime(value string) (time.Time, bool) {
	parsed, _, err := parseICSTimeWithZone(value, "")
	return parsed, err == nil
}

func parseICSTimeWithZone(value, timezone string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false, fmt.Errorf("empty timestamp")
	}
	if len(value) == len("20060102") {
		parsed, err := time.ParseInLocation("20060102", value, time.Local)
		if err != nil {
			return time.Time{}, false, err
		}
		return parsed, true, nil
	}
	if parsed, ok := parseICSTimeWithOffset(value); ok {
		return parsed, false, nil
	}
	if timezone != "" {
		if loc, ok := loadICSTimeLocation(timezone); ok {
			if parsed, ok := parseICSFloatingTimeInLocation(value, loc); ok {
				return parsed, false, nil
			}
		}
	}
	if parsed, ok := parseICSFloatingTimeInLocation(value, time.UTC); ok {
		return parsed, false, nil
	}
	return time.Time{}, false, fmt.Errorf("unsupported timestamp format")
}

func parseICSTimeWithOffset(value string) (time.Time, bool) {
	for _, layout := range []string{
		"20060102T150405Z",
		"20060102T1504Z",
		"20060102T150405-0700",
		"20060102T1504-0700",
		"20060102T150405-07:00",
		"20060102T1504-07:00",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseICSFloatingTimeInLocation(value string, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.UTC
	}
	for _, layout := range []string{"20060102T150405", "20060102T1504"} {
		if parsed, err := time.ParseInLocation(layout, value, loc); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func loadICSTimeLocation(timezone string) (*time.Location, bool) {
	timezone = strings.Trim(strings.TrimSpace(timezone), `"`)
	if timezone == "" {
		return nil, false
	}
	for _, prefix := range []string{
		"/freeassociation.sourceforge.net/",
		"/mozilla.org/20050126_1/",
	} {
		timezone = strings.TrimPrefix(timezone, prefix)
	}
	timezone = strings.TrimPrefix(timezone, "/")
	if loc, err := time.LoadLocation(timezone); err == nil {
		return loc, true
	}
	for _, marker := range []string{
		"Africa/",
		"America/",
		"Antarctica/",
		"Asia/",
		"Atlantic/",
		"Australia/",
		"Europe/",
		"Indian/",
		"Pacific/",
		"Etc/",
	} {
		if idx := strings.Index(timezone, marker); idx >= 0 {
			if loc, err := time.LoadLocation(timezone[idx:]); err == nil {
				return loc, true
			}
		}
	}
	return nil, false
}

func parseRFC3339(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	return parsed, err == nil
}

func unescapeICSValue(value string) string {
	return strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`).Replace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func escapeXML(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}

func normalizeCalendarStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", "-")
	return strings.ToLower(value)
}

func validateCalendarMutationOptions(opts models.CalendarMutationOptions) (models.CalendarMutationOptions, error) {
	return models.NormalizeCalendarMutationOptions(opts)
}

func (s *CalDAVSource) calendarHTTPError(method string, status int, body []byte) error {
	err := calendarProviderHTTPError("caldav", method, status, body)
	if s != nil && isCalDAVAuthStatus(status) && isTrustedICloudCalDAVURL(s.baseURL) {
		return fmt.Errorf("%w. %s", err, iCloudCalDAVAuthGuidance())
	}
	return err
}

func calendarProviderHTTPError(provider, method string, status int, body []byte) error {
	if status == http.StatusConflict || status == http.StatusPreconditionFailed {
		return fmt.Errorf("%w: provider calendar item changed before %s", models.ErrCalendarMutationConflict, strings.ToLower(method))
	}
	message := calendarProviderBodyMessage(status, body)
	if message == "" {
		message = http.StatusText(status)
	}
	if message == "" {
		message = fmt.Sprintf("status %d", status)
	}
	if status == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s authorization expired; reconnect this calendar account", models.ErrCalendarAuthorizationRequired, provider)
	}
	if status == http.StatusForbidden {
		action := "access"
		if strings.TrimSpace(method) != "" && !strings.EqualFold(method, http.MethodGet) {
			action = "write access"
		}
		return fmt.Errorf("%w: %s %s denied; reconnect this calendar account to approve Calendar access", models.ErrCalendarWritePermission, provider, action)
	}
	return fmt.Errorf("%s %s failed: %s", provider, strings.ToLower(method), message)
}

func calendarProviderOAuthError(provider string, err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "re-authenticate") || strings.Contains(message, "refresh token") || strings.Contains(message, "oauth") {
		return fmt.Errorf("%w: %s needs OAuth; reconnect this calendar account", models.ErrCalendarAuthorizationRequired, provider)
	}
	return err
}

func calendarProviderBodyMessage(status int, body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return ""
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Errors  []struct {
				Message string `json:"message"`
				Reason  string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if payload.Error.Message != "" {
			return payload.Error.Message
		}
		for _, item := range payload.Error.Errors {
			if item.Message != "" {
				return item.Message
			}
			if item.Reason != "" {
				return item.Reason
			}
		}
		if payload.Error.Status != "" {
			return strings.ReplaceAll(strings.ToLower(payload.Error.Status), "_", " ")
		}
	}
	return strings.TrimSpace(string(body))
}

func isCalDAVAuthStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func isTrustedICloudCalDAVURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return isTrustedICloudCalDAVHost(parsed.Host)
}

func iCloudCalDAVAuthGuidance() string {
	return "For iCloud Calendar, use your Apple Account email and an Apple app-specific password. If you changed your Apple Account password, generate a new app-specific password. Apple Account two-factor authentication must be enabled."
}

func isCalDAVSyncUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unsupported") ||
		strings.Contains(message, "not implemented") ||
		strings.Contains(message, "method not allowed")
}

func isCalDAVDiscoveryOptionalError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "method not allowed") ||
		strings.Contains(message, "not implemented")
}

func setCalendarAttendeeRSVP(event *models.CalendarEvent, attendeeEmail, status string) error {
	if event == nil {
		return fmt.Errorf("calendar event is nil")
	}
	attendeeEmail = strings.TrimSpace(strings.ToLower(attendeeEmail))
	if attendeeEmail == "" {
		return fmt.Errorf("calendar attendee identity is not configured")
	}
	for i := range event.Attendees {
		if strings.EqualFold(strings.TrimSpace(event.Attendees[i].Email), attendeeEmail) {
			event.Attendees[i].RSVP = status
			event.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("calendar attendee %s is not on this event", attendeeEmail)
}

func googleRSVPStatus(value string) string {
	normalized, err := models.NormalizeCalendarRSVP(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	if normalized == "needs-action" {
		return "needsAction"
	}
	return normalized
}

func icsRSVPStatus(value string) string {
	normalized, err := models.NormalizeCalendarRSVP(value)
	if err != nil {
		return strings.ToUpper(strings.TrimSpace(value))
	}
	return strings.ToUpper(normalized)
}

func summarizeRecurrence(rules []string) string {
	if len(rules) == 0 {
		return ""
	}
	for _, rule := range rules {
		key, value, ok := strings.Cut(rule, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "RRULE") {
			continue
		}
		parts := strings.Split(value, ";")
		attrs := make(map[string]string, len(parts))
		for _, part := range parts {
			k, v, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			attrs[strings.ToUpper(strings.TrimSpace(k))] = strings.ToUpper(strings.TrimSpace(v))
		}
		switch attrs["FREQ"] {
		case "DAILY":
			return "Daily"
		case "WEEKLY":
			if days := summarizeRecurrenceDays(attrs["BYDAY"]); days != "" {
				return "Weekly on " + days
			}
			return "Weekly"
		case "MONTHLY":
			return "Monthly"
		case "YEARLY":
			return "Yearly"
		}
	}
	return rules[0]
}

func summarizeRecurrenceDays(byDay string) string {
	if byDay == "" {
		return ""
	}
	labels := map[string]string{
		"MO": "Monday",
		"TU": "Tuesday",
		"WE": "Wednesday",
		"TH": "Thursday",
		"FR": "Friday",
		"SA": "Saturday",
		"SU": "Sunday",
	}
	parts := strings.Split(byDay, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimLeft(strings.TrimSpace(part), "+-0123456789")
		if label := labels[part]; label != "" {
			out = append(out, label)
		}
	}
	if len(out) == 0 {
		return ""
	}
	if len(out) == 1 {
		return out[0]
	}
	if len(out) == 2 {
		return out[0] + " and " + out[1]
	}
	return strings.Join(out[:len(out)-1], ", ") + ", and " + out[len(out)-1]
}
