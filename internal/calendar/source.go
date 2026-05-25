package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
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
	UpdateEvent(context.Context, models.CalendarEvent, models.CalendarMutationOptions) (*models.CalendarEvent, error)
	RespondToEvent(context.Context, models.EventRef, string, models.CalendarMutationOptions) (*models.CalendarEvent, error)
}

type GoogleCalendarSource struct {
	id            models.SourceID
	accountID     models.AccountID
	displayName   string
	accessToken   string
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
		accessToken:   strings.TrimSpace(cfg.Google.AccessToken),
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
			Ref:   ref,
			Color: item.BackgroundColor,
			ETag:  item.ETag,
		})
	}
	return out, nil
}

func (s *GoogleCalendarSource) ListEvents(ctx context.Context, ref models.CollectionRef) ([]models.CalendarEvent, error) {
	ref = s.normalizeCollection(ref)
	u := s.baseURL + "/calendars/" + url.PathEscape(ref.CollectionID) + "/events?singleEvents=true&showDeleted=false"
	req, err := s.newRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var payload googleEvents
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	out := make([]models.CalendarEvent, 0, len(payload.Items))
	for _, item := range payload.Items {
		out = append(out, googleEventToModel(s.id, s.accountID, ref.CollectionID, item))
	}
	return out, nil
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
	event := googleEventToModel(s.id, s.accountID, ref.CalendarID, payload)
	return &event, nil
}

func (s *GoogleCalendarSource) UpdateEvent(ctx context.Context, event models.CalendarEvent, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	if err := validateCalendarMutationOptions(opts); err != nil {
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
	out := googleEventToModel(s.id, s.accountID, event.Ref.CalendarID, updated)
	return &out, nil
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
	if s.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.accessToken)
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
		return fmt.Errorf("google calendar %s %s: %s", req.Method, req.URL.Path, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type CalDAVSource struct {
	id            models.SourceID
	accountID     models.AccountID
	displayName   string
	baseURL       string
	username      string
	password      string
	attendeeEmail string
	client        *http.Client
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
	req, err := s.newRequest(ctx, "PROPFIND", s.baseURL, strings.NewReader(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:displayname/></d:prop></d:propfind>`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	var ms davMultiStatus
	if err := s.doXML(req, &ms); err != nil {
		return nil, err
	}
	out := make([]models.CalendarCollection, 0, len(ms.Responses))
	for _, response := range ms.Responses {
		calendarID := calendarIDFromHref(response.Href)
		if calendarID == "" {
			continue
		}
		prop := response.FirstProp()
		ref := models.CollectionRef{
			SourceID:     s.id,
			AccountID:    s.accountID,
			Kind:         models.SourceKindCalendar,
			CollectionID: calendarID,
			DisplayName:  firstNonEmpty(prop.DisplayName, calendarID),
		}
		out = append(out, models.CalendarCollection{
			Ref:   ref,
			Color: prop.CalendarColor,
			ETag:  prop.GetETag,
		})
	}
	return out, nil
}

func (s *CalDAVSource) ListEvents(ctx context.Context, ref models.CollectionRef) ([]models.CalendarEvent, error) {
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	req, err := s.newRequest(ctx, "REPORT", s.collectionURL(ref.CollectionID), bytes.NewBufferString(caldavCalendarQuery))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	var ms davMultiStatus
	if err := s.doXML(req, &ms); err != nil {
		return nil, err
	}
	out := make([]models.CalendarEvent, 0, len(ms.Responses))
	for _, response := range ms.Responses {
		prop := response.FirstProp()
		if strings.TrimSpace(prop.CalendarData) == "" {
			continue
		}
		eventID := path.Base(strings.TrimRight(response.Href, "/"))
		event, err := eventFromICS(s.id, s.accountID, ref.CollectionID, eventID, prop.GetETag, prop.CalendarData)
		if err != nil {
			return nil, err
		}
		out = append(out, *event)
	}
	return out, nil
}

func (s *CalDAVSource) FetchEvent(ctx context.Context, ref models.EventRef) (*models.CalendarEvent, error) {
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, s.id)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	ref = ref.WithDefaults()
	req, err := s.newRequest(ctx, http.MethodGet, s.eventURL(ref.CalendarID, ref.EventID), nil)
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

func (s *CalDAVSource) UpdateEvent(ctx context.Context, event models.CalendarEvent, opts models.CalendarMutationOptions) (*models.CalendarEvent, error) {
	if err := validateCalendarMutationOptions(opts); err != nil {
		return nil, err
	}
	event.Ref.SourceID = models.NormalizeSourceID(event.Ref.SourceID, s.id)
	event.Ref.AccountID = models.NormalizeAccountID(event.Ref.AccountID)
	event.Ref = event.Ref.WithDefaults()
	req, err := s.newRequest(ctx, http.MethodPut, s.eventURL(event.Ref.CalendarID, event.Ref.EventID), strings.NewReader(eventToICS(event)))
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
		return nil, fmt.Errorf("caldav PUT %s: %s", req.URL.Path, strings.TrimSpace(string(body)))
	}
	freshRef := event.Ref
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
		freshRef.ETag = etag
	}
	return s.FetchEvent(ctx, freshRef)
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
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if s.username != "" || s.password != "" {
		req.SetBasicAuth(s.username, s.password)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	return req, nil
}

func (s *CalDAVSource) doXML(req *http.Request, out any) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("caldav %s %s: %s", req.Method, req.URL.Path, strings.TrimSpace(string(body)))
	}
	return xml.NewDecoder(resp.Body).Decode(out)
}

func (s *CalDAVSource) collectionURL(calendarID string) string {
	return s.baseURL + url.PathEscape(calendarID) + "/"
}

func (s *CalDAVSource) eventURL(calendarID, eventID string) string {
	return s.collectionURL(calendarID) + url.PathEscape(eventID)
}

const caldavCalendarQuery = `<?xml version="1.0" encoding="utf-8"?>
<cal:calendar-query xmlns:d="DAV:" xmlns:cal="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:getetag/><cal:calendar-data/></d:prop>
  <cal:filter><cal:comp-filter name="VCALENDAR"><cal:comp-filter name="VEVENT"/></cal:comp-filter></cal:filter>
</cal:calendar-query>`

type googleCalendarList struct {
	Items []struct {
		ID              string `json:"id"`
		Summary         string `json:"summary"`
		BackgroundColor string `json:"backgroundColor"`
		ETag            string `json:"etag"`
	} `json:"items"`
}

type googleEvents struct {
	Items         []googleEventPayload `json:"items"`
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

func googleEventToModel(sourceID models.SourceID, accountID models.AccountID, calendarID string, item googleEventPayload) models.CalendarEvent {
	start, allDay := parseGoogleTime(item.Start)
	end, _ := parseGoogleTime(item.End)
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
		Revision:          fmt.Sprintf("%d", item.Sequence),
		UpdatedAt:         updated,
	}
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

func parseGoogleTime(t googleEventTime) (time.Time, bool) {
	if strings.TrimSpace(t.Date) != "" {
		parsed, _ := time.Parse("2006-01-02", t.Date)
		return parsed, true
	}
	parsed, _ := parseRFC3339(t.DateTime)
	return parsed, false
}

type davMultiStatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string        `xml:"href"`
	Propstat []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop davProp `xml:"prop"`
}

type davProp struct {
	DisplayName   string `xml:"displayname"`
	CalendarColor string `xml:"calendar-color"`
	GetETag       string `xml:"getetag"`
	CalendarData  string `xml:"calendar-data"`
}

func (r davResponse) FirstProp() davProp {
	if len(r.Propstat) == 0 {
		return davProp{}
	}
	return r.Propstat[0].Prop
}

func calendarIDFromHref(href string) string {
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

func eventFromICS(sourceID models.SourceID, accountID models.AccountID, calendarID, eventID, etag, data string) (*models.CalendarEvent, error) {
	fields := parseICS(data)
	details := parseICSRichDetails(data)
	start, allDay := parseICSTimeWithZone(fields["DTSTART"], details.TimeZone)
	end, _ := parseICSTimeWithZone(fields["DTEND"], details.TimeZone)
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
		UpdatedAt:         updated,
		Raw:               data,
	}, nil
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
}

func parseICSRichDetails(data string) icsRichDetails {
	var details icsRichDetails
	for _, line := range unfoldICSLines(data) {
		nameAndParams, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		parts := strings.Split(nameAndParams, ";")
		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		params := parseICSParams(parts[1:])
		value = unescapeICSValue(value)
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
	lines := unfoldICSLines(data)
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
	return parseICSTimeWithZone(value, "")
}

func parseICSTimeWithZone(value, timezone string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	if len(value) == len("20060102") {
		parsed, _ := time.Parse("20060102", value)
		return parsed, true
	}
	if strings.HasSuffix(value, "Z") {
		parsed, _ := time.Parse("20060102T150405Z", value)
		return parsed, false
	}
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			parsed, _ := time.ParseInLocation("20060102T150405", value, loc)
			return parsed, false
		}
	}
	parsed, _ := time.Parse("20060102T150405", value)
	return parsed, false
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

func normalizeCalendarStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", "-")
	return strings.ToLower(value)
}

func validateCalendarMutationOptions(opts models.CalendarMutationOptions) error {
	scope := strings.TrimSpace(opts.RecurrenceScope)
	if scope == "" || scope == models.CalendarMutationScopeThisEvent {
		return nil
	}
	return fmt.Errorf("calendar recurrence scope %q is not supported yet", scope)
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
