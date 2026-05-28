package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	calendarpkg "github.com/herald-email/herald-mail-app/internal/calendar"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type calendarInvitationPromptState struct {
	Active      bool
	Event       models.CalendarEvent
	Collections []models.CalendarCollection
	Cursor      int
	Saving      bool
	Error       string
}

func (m *Model) handleCalendarInvitationPromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.calendarInvitation.Active {
		return m, nil, false
	}
	switch shortcutKey(msg) {
	case "esc":
		m.calendarInvitation = calendarInvitationPromptState{}
		m.calendarStatus = "Invitation import cancelled"
		return m, nil, true
	case "j", "down":
		if m.calendarInvitation.Cursor < len(m.calendarInvitation.Collections)-1 {
			m.calendarInvitation.Cursor++
		}
		return m, nil, true
	case "k", "up":
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
		m.statusMessage = "Add to calendar needs a configured calendar source"
		return nil
	}
	body := m.timeline.body
	event, ok, err := calendarInvitationEventFromBody(body, m.calendarCollections)
	if err != nil {
		m.statusMessage = "Invitation import failed: " + err.Error()
		return nil
	}
	if !ok {
		m.statusMessage = "No calendar invitation found in this email"
		return nil
	}
	collections := m.calendarCollections
	if len(collections) == 0 {
		collections = m.mergeCalendarCollections(nil)
	}
	if len(collections) == 0 {
		m.statusMessage = "No calendar is available for invitation import"
		return nil
	}
	m.calendarInvitation = calendarInvitationPromptState{
		Active:      true,
		Event:       *event,
		Collections: collections,
	}
	m.statusMessage = "Choose a calendar for this invitation"
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
	if existing, ok := m.calendarEventWithProviderUID(event.Ref.SourceID, event.Ref.AccountID, event.Ref.CalendarID, event.ProviderUID); ok {
		event.Ref = existing.Ref.WithDefaults()
	}
	if strings.TrimSpace(event.Ref.EventID) == "" {
		event.Ref.EventID = firstNonEmptyString(event.ProviderUID, "mail-invitation")
	}
	event.Ref.LocalID = ""
	event.Ref = event.Ref.WithDefaults()
	event.Status = firstNonEmptyString(event.Status, "confirmed")
	m.calendarInvitation.Saving = true
	m.calendarInvitation.Error = ""
	m.calendarStatus = "Adding invitation to " + calendarCollectionDisplayName(collection) + "..."
	mutation, ok := m.calendarMutationBackend()
	if !ok {
		return func() tea.Msg {
			return CalendarInvitationSavedMsg{Ref: event.Ref, Err: fmt.Errorf("calendar mutation backend unavailable")}
		}
	}
	return func() tea.Msg {
		saved, err := mutation.SaveCalendarEvent(event)
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
		m.theme.Severity.Info.Style().Render(calendarFit("Add invitation to calendar", width)),
	}
	title := strings.TrimSpace(m.calendarInvitation.Event.Title)
	if title == "" {
		title = "Untitled invitation"
	}
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(title, width)))
	if strings.TrimSpace(m.calendarInvitation.Error) != "" {
		lines = append(lines, m.theme.Severity.Error.Style().Render(calendarFit(m.calendarInvitation.Error, width)))
	}
	for i, collection := range m.calendarInvitation.Collections {
		line := calendarCollectionDisplayName(collection)
		if i == m.calendarInvitation.Cursor {
			line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
		} else {
			line = "  " + calendarFit(line, width-2)
		}
		lines = append(lines, line)
	}
	if m.calendarInvitation.Saving {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Saving...", width)))
	} else {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("enter: add  esc: cancel", width)))
	}
	return lines
}
