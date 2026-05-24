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

type GoogleCalendarSource struct {
	id          models.SourceID
	accountID   models.AccountID
	displayName string
	accessToken string
	baseURL     string
	client      *http.Client
}

func NewGoogleCalendarSource(cfg config.SourceConfig) (*GoogleCalendarSource, error) {
	id := models.NormalizeSourceID(models.SourceID(cfg.ID), models.DefaultCalendarSourceID)
	accountID := models.NormalizeAccountID(models.AccountID(cfg.AccountID))
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.Google.APIBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://www.googleapis.com/calendar/v3"
	}
	return &GoogleCalendarSource{
		id:          id,
		accountID:   accountID,
		displayName: cfg.DisplayName,
		accessToken: strings.TrimSpace(cfg.Google.AccessToken),
		baseURL:     baseURL,
		client:      http.DefaultClient,
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
	id          models.SourceID
	accountID   models.AccountID
	displayName string
	baseURL     string
	username    string
	password    string
	client      *http.Client
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
		id:          models.NormalizeSourceID(models.SourceID(cfg.ID), models.DefaultCalendarSourceID),
		accountID:   models.NormalizeAccountID(models.AccountID(cfg.AccountID)),
		displayName: cfg.DisplayName,
		baseURL:     baseURL,
		username:    cfg.CalDAV.Username,
		password:    cfg.CalDAV.Password,
		client:      http.DefaultClient,
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
	ID          string          `json:"id"`
	ICalUID     string          `json:"iCalUID"`
	ETag        string          `json:"etag"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Location    string          `json:"location"`
	Status      string          `json:"status"`
	Updated     string          `json:"updated"`
	Start       googleEventTime `json:"start"`
	End         googleEventTime `json:"end"`
	Sequence    int             `json:"sequence"`
	Extended    map[string]any  `json:"extendedProperties"`
}

type googleEventTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
	TimeZone string `json:"timeZone"`
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
		Ref:         ref,
		ProviderUID: item.ICalUID,
		Title:       item.Summary,
		Description: item.Description,
		Location:    item.Location,
		Start:       start,
		End:         end,
		AllDay:      allDay,
		Status:      item.Status,
		Revision:    fmt.Sprintf("%d", item.Sequence),
		UpdatedAt:   updated,
	}
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
	start, allDay := parseICSTime(fields["DTSTART"])
	end, _ := parseICSTime(fields["DTEND"])
	updated, _ := parseICSTime(fields["LAST-MODIFIED"])
	ref := models.EventRef{
		SourceID:   sourceID,
		AccountID:  accountID,
		CalendarID: calendarID,
		EventID:    eventID,
		ETag:       etag,
	}.WithDefaults()
	return &models.CalendarEvent{
		Ref:         ref,
		ProviderUID: fields["UID"],
		Title:       fields["SUMMARY"],
		Description: fields["DESCRIPTION"],
		Location:    fields["LOCATION"],
		Start:       start,
		End:         end,
		AllDay:      allDay,
		Status:      fields["STATUS"],
		UpdatedAt:   updated,
		Raw:         data,
	}, nil
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
