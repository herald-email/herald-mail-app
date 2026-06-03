package app

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	calendarpkg "github.com/herald-email/herald-mail-app/internal/calendar"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type calendarInvitationPromptState struct {
	Active             bool
	Event              models.CalendarEvent
	Collections        []models.CalendarCollection
	Cursor             int
	Saving             bool
	Error              string
	Duplicate          *models.CalendarEvent
	DuplicateChecked   bool
	ReconnectAvailable bool
	OpenAfterLoad      bool
	PayloadLoadTried   bool
}

func (m *Model) handleCalendarInvitationPromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.calendarInvitation.Active {
		return m, nil, false
	}
	switch shortcutKey(msg) {
	case "esc":
		m.calendarInvitation = calendarInvitationPromptState{}
		m.statusMessage = "Invitation import cancelled"
		m.calendarStatus = m.statusMessage
		return m, nil, true
	case "s":
		if m.calendarInvitation.Duplicate != nil {
			m.calendarInvitation = calendarInvitationPromptState{}
			m.statusMessage = "Skipped existing calendar event"
			m.calendarStatus = m.statusMessage
		}
		return m, nil, true
	case "r":
		if m.calendarInvitation.ReconnectAvailable {
			return m, m.reconnectCalendarInvitationOAuthCmd(), true
		}
		return m, nil, true
	case "j", "down":
		if m.calendarInvitation.Duplicate != nil {
			return m, nil, true
		}
		if m.calendarInvitation.Cursor < len(m.calendarInvitation.Collections)-1 {
			m.calendarInvitation.Cursor++
		}
		return m, nil, true
	case "k", "up":
		if m.calendarInvitation.Duplicate != nil {
			return m, nil, true
		}
		if m.calendarInvitation.Cursor > 0 {
			m.calendarInvitation.Cursor--
		}
		return m, nil, true
	case "enter":
		return m, m.saveSelectedCalendarInvitation(), true
	}
	return m, nil, true
}

func (m *Model) openCalendarInvitationPrompt() tea.Cmd {
	if !m.calendarAvailable {
		m.statusMessage = "Create Calendar Event needs a configured calendar source"
		return nil
	}
	body := m.timeline.body
	payloadLoadTried := m.calendarInvitation.PayloadLoadTried
	event, ok, err := calendarInvitationEventFromBody(body, m.calendarCollections)
	if err != nil {
		m.statusMessage = "Create Calendar Event failed: " + err.Error()
		return nil
	}
	if !ok {
		if calendarBodyHasInvitation(body) && !payloadLoadTried {
			if m.timeline.selectedEmail == nil {
				m.statusMessage = "Loading calendar invitation needs a selected email"
				return nil
			}
			m.calendarInvitation = calendarInvitationPromptState{OpenAfterLoad: true, PayloadLoadTried: true}
			m.statusMessage = "Loading calendar invitation..."
			return m.loadEmailFullBodyForRefCmd(m.timeline.selectedEmail.MessageRef())
		}
		m.statusMessage = "No calendar invitation found in this email"
		return nil
	}
	collections := m.calendarInvitationCollections()
	if len(collections) == 0 {
		if cmd := m.loadCalendarInvitationCollectionsCmd(*event, payloadLoadTried); cmd != nil {
			m.statusMessage = "Loading calendars for invitation..."
			m.calendarStatus = m.statusMessage
			return cmd
		}
		m.statusMessage = "No writable calendar is available for invitation import"
		return nil
	}
	m.calendarInvitation = calendarInvitationPromptState{
		Active:           true,
		Event:            *event,
		Collections:      collections,
		PayloadLoadTried: payloadLoadTried,
	}
	m.statusMessage = "Choose a calendar for this invitation"
	return nil
}

func (m *Model) calendarInvitationCollections() []models.CalendarCollection {
	collections := m.calendarCollections
	if len(collections) == 0 {
		collections = m.mergeCalendarCollections(nil)
	}
	return writableCalendarInvitationCollections(collections)
}

func (m *Model) loadCalendarInvitationCollectionsCmd(event models.CalendarEvent, payloadLoadTried bool) tea.Cmd {
	if cacheBackend, ok := m.backend.(backend.CalendarAgendaCacheBackend); ok {
		return func() tea.Msg {
			collections, err := cacheBackend.ListCachedCalendarCollections()
			if err == nil && len(collections) > 0 {
				return CalendarInvitationCollectionsMsg{Event: event, Collections: collections, PayloadLoadTried: payloadLoadTried}
			}
			if collectionBackend, ok := m.backend.(backend.CalendarCollectionBackend); ok {
				collections, err = collectionBackend.ListCalendarCollections()
				return CalendarInvitationCollectionsMsg{Event: event, Collections: collections, PayloadLoadTried: payloadLoadTried, Err: err}
			}
			return CalendarInvitationCollectionsMsg{Event: event, Collections: collections, PayloadLoadTried: payloadLoadTried, Err: err}
		}
	}
	if collectionBackend, ok := m.backend.(backend.CalendarCollectionBackend); ok {
		return func() tea.Msg {
			collections, err := collectionBackend.ListCalendarCollections()
			return CalendarInvitationCollectionsMsg{Event: event, Collections: collections, PayloadLoadTried: payloadLoadTried, Err: err}
		}
	}
	return nil
}

func (m *Model) saveSelectedCalendarInvitation() tea.Cmd {
	state := m.calendarInvitation
	if !state.Active || state.Saving || len(state.Collections) == 0 {
		return nil
	}
	if state.Cursor < 0 {
		state.Cursor = 0
	}
	if state.Cursor >= len(state.Collections) {
		state.Cursor = len(state.Collections) - 1
	}
	collection := state.Collections[state.Cursor]
	event := state.Event
	event.Ref.SourceID = models.NormalizeSourceID(collection.Ref.SourceID, models.DefaultCalendarSourceID)
	event.Ref.AccountID = models.NormalizeAccountID(collection.Ref.AccountID)
	event.Ref.CalendarID = collection.Ref.CollectionID
	if strings.TrimSpace(event.Ref.EventID) == "" {
		event.Ref.EventID = firstNonEmptyString(event.ProviderUID, "mail-invitation")
	}
	event.Ref.LocalID = ""
	event.Ref = event.Ref.WithDefaults()
	event.Status = firstNonEmptyString(event.Status, "confirmed")
	m.calendarInvitation.Saving = true
	m.calendarInvitation.Error = ""
	mutation, ok := m.calendarMutationBackend()
	if !ok {
		return func() tea.Msg {
			return CalendarInvitationSavedMsg{Ref: event.Ref, Err: fmt.Errorf("calendar mutation backend unavailable")}
		}
	}
	if state.Duplicate != nil {
		event.Ref = state.Duplicate.Ref.WithDefaults()
		m.calendarStatus = "Updating existing event in " + calendarCollectionDisplayName(collection) + "..."
		return func() tea.Msg {
			saved, err := mutation.SaveCalendarEvent(event)
			return CalendarInvitationSavedMsg{Ref: event.Ref, Event: saved, Err: err}
		}
	}
	collectionRef := models.CollectionRef{
		SourceID:     event.Ref.SourceID,
		AccountID:    event.Ref.AccountID,
		Kind:         models.SourceKindCalendar,
		CollectionID: event.Ref.CalendarID,
		DisplayName:  collection.Ref.DisplayName,
	}
	uid := strings.TrimSpace(event.ProviderUID)
	if uid != "" && !state.DuplicateChecked {
		m.calendarStatus = "Checking calendar for existing invitation..."
		return func() tea.Msg {
			duplicate, err := mutation.FindCalendarEventByUID(collectionRef, uid)
			if err != nil {
				return CalendarInvitationDuplicateMsg{Ref: collectionRef, UID: uid, Err: err}
			}
			if duplicate != nil {
				return CalendarInvitationDuplicateMsg{Ref: collectionRef, UID: uid, Duplicate: duplicate}
			}
			saved, err := mutation.CreateCalendarEvent(event)
			return CalendarInvitationSavedMsg{Ref: event.Ref, Event: saved, Err: err}
		}
	}
	m.calendarStatus = "Creating calendar event in " + calendarCollectionDisplayName(collection) + "..."
	return func() tea.Msg {
		saved, err := mutation.CreateCalendarEvent(event)
		return CalendarInvitationSavedMsg{Ref: event.Ref, Event: saved, Err: err}
	}
}

func (m *Model) calendarEventWithProviderUID(sourceID models.SourceID, accountID models.AccountID, calendarID, providerUID string) (models.CalendarEvent, bool) {
	providerUID = strings.TrimSpace(providerUID)
	if providerUID == "" {
		return models.CalendarEvent{}, false
	}
	for _, event := range m.calendarEvents {
		ref := event.Ref.WithDefaults()
		if ref.SourceID == sourceID && ref.AccountID == accountID && ref.CalendarID == calendarID && strings.TrimSpace(event.ProviderUID) == providerUID {
			return event, true
		}
	}
	return models.CalendarEvent{}, false
}

func calendarInvitationEventFromBody(body *models.EmailBody, collections []models.CalendarCollection) (*models.CalendarEvent, bool, error) {
	if body == nil {
		return nil, false, nil
	}
	data := calendarInvitationICS(body)
	if strings.TrimSpace(data) == "" {
		return nil, false, nil
	}
	sourceID := models.DefaultCalendarSourceID
	accountID := models.DefaultAccountID
	calendarID := "calendar"
	if len(collections) > 0 {
		sourceID = models.NormalizeSourceID(collections[0].Ref.SourceID, models.DefaultCalendarSourceID)
		accountID = models.NormalizeAccountID(collections[0].Ref.AccountID)
		calendarID = firstNonEmptyString(collections[0].Ref.CollectionID, calendarID)
	}
	event, err := calendarpkg.EventFromInvitationICS(sourceID, accountID, calendarID, data)
	if err != nil {
		return nil, true, err
	}
	return event, true, nil
}

func calendarInvitationICS(body *models.EmailBody) string {
	for _, invitation := range body.CalendarInvitations {
		if data := extractCalendarICS(invitation.Data); data != "" {
			return data
		}
	}
	for _, attachment := range body.Attachments {
		if !calendarAttachmentLooksLikeInvitation(attachment) || len(attachment.Data) == 0 {
			continue
		}
		if data := extractCalendarICS(string(attachment.Data)); data != "" {
			return data
		}
	}
	if data := extractCalendarICS(body.TextPlain); data != "" {
		return data
	}
	if data := extractCalendarICS(body.TextHTML); data != "" {
		return data
	}
	return ""
}

func calendarBodyHasInvitation(body *models.EmailBody) bool {
	if body == nil {
		return false
	}
	if calendarInvitationICS(body) != "" {
		return true
	}
	for _, invitation := range body.CalendarInvitations {
		if calendarInvitationPartLooksLikeInvitation(invitation) {
			return true
		}
	}
	for _, attachment := range body.Attachments {
		if calendarAttachmentLooksLikeInvitation(attachment) {
			return true
		}
	}
	return false
}

func calendarAttachmentLooksLikeInvitation(attachment models.Attachment) bool {
	mimeType := strings.ToLower(strings.TrimSpace(attachment.MIMEType))
	filename := strings.ToLower(strings.TrimSpace(attachment.Filename))
	return strings.Contains(mimeType, "text/calendar") || strings.HasSuffix(filename, ".ics") || strings.Contains(mimeType, "application/ics")
}

func calendarInvitationPartLooksLikeInvitation(invitation models.CalendarInvitationPart) bool {
	mimeType := strings.ToLower(strings.TrimSpace(invitation.MIMEType))
	filename := strings.ToLower(strings.TrimSpace(invitation.Filename))
	return strings.Contains(mimeType, "text/calendar") || strings.HasSuffix(filename, ".ics") || strings.Contains(mimeType, "application/ics")
}

func extractCalendarICS(value string) string {
	upper := strings.ToUpper(value)
	start := strings.Index(upper, "BEGIN:VCALENDAR")
	end := strings.Index(upper, "END:VCALENDAR")
	if start < 0 || end < start {
		return ""
	}
	end += len("END:VCALENDAR")
	return strings.TrimSpace(value[start:end])
}

func (m *Model) renderCalendarInvitationPrompt(width int) []string {
	if !m.calendarInvitation.Active {
		return nil
	}
	if width < 12 {
		width = 12
	}
	lines := []string{
		m.theme.Severity.Info.Style().Render(calendarFit("Create Calendar Event", width)),
	}
	title := strings.TrimSpace(m.calendarInvitation.Event.Title)
	if title == "" {
		title = "Untitled invitation"
	}
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(title, width)))
	if strings.TrimSpace(m.calendarInvitation.Error) != "" {
		lines = append(lines, m.theme.Severity.Error.Style().Render(calendarFit(m.calendarInvitation.Error, width)))
	}
	if m.calendarInvitation.Duplicate != nil {
		lines = append(lines, m.theme.Severity.Warning.Style().Render(calendarFit("Event already exists", width)))
		hint := "enter: update  s: skip  esc: cancel"
		if m.calendarInvitation.ReconnectAvailable {
			hint = "enter: update  s: skip  r: reconnect  esc: cancel"
		}
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(hint, width)))
		return lines
	}
	if !m.calendarInvitation.Saving {
		accountLabels := calendarInvitationAccountLabels(m.calendarInvitation.Collections)
		for i, collection := range m.calendarInvitation.Collections {
			line := calendarInvitationCollectionChoiceLabel(collection, accountLabels[calendarInvitationCollectionScopeKey(collection)])
			if i == m.calendarInvitation.Cursor {
				line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
			} else {
				line = "  " + calendarFit(line, width-2)
			}
			lines = append(lines, line)
		}
	}
	if m.calendarInvitation.Saving {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Saving...", width)))
	} else {
		hint := "enter: create  esc: cancel"
		if m.calendarInvitation.ReconnectAvailable {
			hint = "enter: create  r: reconnect  esc: cancel"
		}
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(hint, width)))
	}
	return lines
}

func (m *Model) calendarInvitationReconnectAvailable(err error) bool {
	return calendarMutationNeedsReconnect(err) && m.calendarInvitationGoogleSource() != nil
}

func (m *Model) reconnectCalendarInvitationOAuthCmd() tea.Cmd {
	source := m.calendarInvitationGoogleSource()
	if source == nil || m.cfg == nil {
		m.statusMessage = "Google Calendar reconnect is unavailable for this calendar source"
		m.calendarStatus = m.statusMessage
		return nil
	}
	candidate := cloneConfigForOAuth(m.cfg)
	sourceID := models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID)
	email := strings.TrimSpace(source.Google.Email)
	m.statusMessage = "Starting Google Calendar reconnect..."
	m.calendarStatus = m.statusMessage
	return func() tea.Msg {
		return OAuthRequiredMsg{
			Email:             email,
			ServiceLabel:      "Google Calendar OAuth",
			Config:            candidate,
			ValidateCalendar:  true,
			CalendarSourceIDs: []models.SourceID{sourceID},
			SourceIDs:         []models.SourceID{sourceID},
		}
	}
}

func (m *Model) calendarInvitationGoogleSource() *config.SourceConfig {
	if m == nil || m.cfg == nil || !m.calendarInvitation.Active || len(m.calendarInvitation.Collections) == 0 {
		return nil
	}
	cursor := m.calendarInvitation.Cursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(m.calendarInvitation.Collections) {
		cursor = len(m.calendarInvitation.Collections) - 1
	}
	collection := m.calendarInvitation.Collections[cursor]
	sourceID := models.NormalizeSourceID(collection.Ref.SourceID, models.DefaultCalendarSourceID)
	for _, source := range m.cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) != string(models.SourceKindCalendar) {
			continue
		}
		if strings.TrimSpace(source.Provider) != "google_calendar" {
			continue
		}
		if models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID) != sourceID {
			continue
		}
		copy := source
		return &copy
	}
	return nil
}

func calendarMutationNeedsReconnect(err error) bool {
	return errors.Is(err, models.ErrCalendarAuthorizationRequired) || errors.Is(err, models.ErrCalendarWritePermission)
}

func cloneConfigForOAuth(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	clone.Sources = append([]config.SourceConfig(nil), cfg.Sources...)
	return &clone
}

func writableCalendarInvitationCollections(collections []models.CalendarCollection) []models.CalendarCollection {
	out := make([]models.CalendarCollection, 0, len(collections))
	for _, collection := range collections {
		if calendarCollectionWritableForInvitation(collection) {
			out = append(out, collection)
		}
	}
	return out
}

func calendarCollectionWritableForInvitation(collection models.CalendarCollection) bool {
	switch strings.ToLower(strings.TrimSpace(collection.AccessRole)) {
	case "freebusyreader", "reader":
		return false
	default:
		return true
	}
}

func calendarInvitationAccountLabels(collections []models.CalendarCollection) map[string]string {
	labels := make(map[string]string, len(collections))
	scopes := make(map[string]bool, len(collections))
	for _, collection := range collections {
		scopes[calendarInvitationCollectionScopeKey(collection)] = true
	}
	if len(scopes) <= 1 {
		return labels
	}
	for _, collection := range collections {
		key := calendarInvitationCollectionScopeKey(collection)
		displayName := strings.TrimSpace(collection.Ref.DisplayName)
		if calendarInvitationLooksLikeEmail(displayName) {
			labels[key] = displayName
		}
	}
	for _, collection := range collections {
		key := calendarInvitationCollectionScopeKey(collection)
		if strings.TrimSpace(labels[key]) != "" {
			continue
		}
		if label := calendarInvitationAccountFallbackLabel(collection); label != "" {
			labels[key] = label
		}
	}
	return labels
}

func calendarInvitationCollectionChoiceLabel(collection models.CalendarCollection, accountLabel string) string {
	name := calendarCollectionDisplayName(collection)
	if calendarInvitationLooksLikeEmail(name) || strings.EqualFold(strings.TrimSpace(collection.Ref.CollectionID), "primary") {
		name = "Primary calendar"
	}
	accountLabel = strings.TrimSpace(accountLabel)
	if accountLabel == "" || strings.EqualFold(name, accountLabel) {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, accountLabel)
}

func calendarInvitationCollectionScopeKey(collection models.CalendarCollection) string {
	ref := collection.Ref
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	return string(ref.SourceID) + "\x00" + string(ref.AccountID)
}

func calendarInvitationAccountFallbackLabel(collection models.CalendarCollection) string {
	ref := collection.Ref
	if ref.AccountID != "" && ref.AccountID != models.DefaultAccountID {
		return calendarInvitationAccountIDLabel(string(ref.AccountID))
	}
	if ref.SourceID != "" && ref.SourceID != models.DefaultCalendarSourceID {
		return calendarHumanLabel(string(ref.SourceID))
	}
	return ""
}

func calendarInvitationAccountIDLabel(value string) string {
	value = strings.TrimSpace(value)
	if calendarInvitationLooksLikeEmail(value) {
		return value
	}
	return calendarHumanLabel(value)
}

func calendarInvitationLooksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	at := strings.Index(value, "@")
	return at > 0 && strings.Contains(value[at+1:], ".")
}
