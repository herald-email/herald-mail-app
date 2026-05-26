package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type calendarViewMode string

const (
	calendarViewAgenda      calendarViewMode = "agenda"
	calendarViewDay         calendarViewMode = "day"
	calendarViewWeek        calendarViewMode = "week"
	calendarViewThreeDay    calendarViewMode = "three-day"
	calendarViewSearch      calendarViewMode = "search"
	calendarViewCrossSearch calendarViewMode = "cross-search"
)

type indexedCalendarEvent struct {
	index int
	event models.CalendarEvent
}

type calendarEditField int

const (
	calendarEditFieldTitle calendarEditField = iota
	calendarEditFieldLocation
	calendarEditFieldStart
	calendarEditFieldEnd
	calendarEditFieldTimeZone
	calendarEditFieldAttendees
	calendarEditFieldRecurrence
	calendarEditFieldDescription
)

type calendarEventEditState struct {
	Active bool
	Ref    models.EventRef
	Base   models.CalendarEvent
	Draft  models.CalendarEventEditDraft
	Field  calendarEditField
	Dirty  bool
	Saving bool
	Error  string
}

var calendarEditFields = []calendarEditField{
	calendarEditFieldTitle,
	calendarEditFieldLocation,
	calendarEditFieldStart,
	calendarEditFieldEnd,
	calendarEditFieldTimeZone,
	calendarEditFieldAttendees,
	calendarEditFieldRecurrence,
	calendarEditFieldDescription,
}

func (m *Model) refreshCalendarAvailability() {
	agenda, ok := m.backend.(backend.CalendarAgendaBackend)
	m.calendarAvailable = ok && agenda.CalendarAgendaAvailable()
	if !m.calendarAvailable {
		m.calendarEvents = nil
		m.calendarDetail = nil
		m.calendarView = calendarViewAgenda
		m.calendarDay = time.Time{}
		m.calendarWeekStart = time.Time{}
		m.calendarThreeDayStart = time.Time{}
		m.calendarSearchQuery = ""
		m.calendarSearchResults = nil
		m.calendarSearchCursor = 0
		m.calendarSearchLoading = false
		m.crossSourceSearchQuery = ""
		m.crossSourceSearchResults = nil
		m.crossSourceSearchCursor = 0
		m.crossSourceSearchLoading = false
		m.calendarDetailOpen = false
		m.calendarLoading = false
		m.calendarDetailLoading = false
		m.calendarEdit = calendarEventEditState{}
	}
}

func (m *Model) calendarAgendaBackend() (backend.CalendarAgendaBackend, bool) {
	agenda, ok := m.backend.(backend.CalendarAgendaBackend)
	if !ok || !agenda.CalendarAgendaAvailable() {
		return nil, false
	}
	return agenda, true
}

func (m *Model) calendarMutationBackend() (backend.CalendarEventMutationBackend, bool) {
	mutation, ok := m.backend.(backend.CalendarEventMutationBackend)
	return mutation, ok
}

func (m *Model) crossSourceSearchBackend() (backend.CrossSourceSearchBackend, bool) {
	search, ok := m.backend.(backend.CrossSourceSearchBackend)
	if !ok {
		return nil, false
	}
	return search, true
}

func (m *Model) crossSourceSearchAvailable() bool {
	_, ok := m.crossSourceSearchBackend()
	return ok
}

func (m *Model) loadCalendarAgenda() tea.Cmd {
	agenda, ok := m.calendarAgendaBackend()
	if !ok {
		return func() tea.Msg {
			return CalendarAgendaLoadedMsg{Err: fmt.Errorf("calendar agenda unavailable")}
		}
	}
	return func() tea.Msg {
		events, err := agenda.ListCalendarAgenda(time.Time{}, time.Time{})
		if err != nil {
			return CalendarAgendaLoadedMsg{Err: err}
		}
		return CalendarAgendaLoadedMsg{Events: events}
	}
}

func (m *Model) loadCalendarSearch() tea.Cmd {
	query := m.calendarSearchQuery
	if strings.TrimSpace(query) == "" {
		return func() tea.Msg {
			return CalendarSearchLoadedMsg{Query: query}
		}
	}
	agenda, ok := m.calendarAgendaBackend()
	if !ok {
		return func() tea.Msg {
			return CalendarSearchLoadedMsg{Query: query, Err: fmt.Errorf("calendar search unavailable")}
		}
	}
	return func() tea.Msg {
		events, err := agenda.SearchCalendarEvents(query, time.Time{}, time.Time{})
		return CalendarSearchLoadedMsg{Query: query, Events: events, Err: err}
	}
}

func (m *Model) loadCrossSourceSearch() tea.Cmd {
	query := m.crossSourceSearchQuery
	if strings.TrimSpace(query) == "" {
		return func() tea.Msg {
			return CrossSourceSearchLoadedMsg{Query: query}
		}
	}
	search, ok := m.crossSourceSearchBackend()
	if !ok {
		return func() tea.Msg {
			return CrossSourceSearchLoadedMsg{Query: query, Err: fmt.Errorf("cross-source search unavailable")}
		}
	}
	return func() tea.Msg {
		results, err := search.CrossSourceSearch(query)
		return CrossSourceSearchLoadedMsg{Query: query, Results: results, Err: err}
	}
}

func (m *Model) selectedCalendarEvent() *models.CalendarEvent {
	if m.calendarView == calendarViewCrossSearch {
		result := m.selectedCrossSourceSearchResult()
		if result == nil || result.Event == nil {
			return nil
		}
		event := *result.Event
		return &event
	}
	if m.calendarView == calendarViewSearch {
		if m.calendarSearchCursor < 0 || m.calendarSearchCursor >= len(m.calendarSearchResults) {
			return nil
		}
		event := m.calendarSearchResults[m.calendarSearchCursor]
		return &event
	}
	if m.calendarCursor < 0 || m.calendarCursor >= len(m.calendarEvents) {
		return nil
	}
	event := m.calendarEvents[m.calendarCursor]
	return &event
}

func (m *Model) selectedCalendarDay() time.Time {
	if !m.calendarDay.IsZero() {
		return m.calendarDay
	}
	if event := m.selectedCalendarEvent(); event != nil && !event.Start.IsZero() {
		return event.Start
	}
	if len(m.calendarEvents) > 0 && !m.calendarEvents[0].Start.IsZero() {
		return m.calendarEvents[0].Start
	}
	return time.Now()
}

func (m *Model) selectedCalendarWeekStart() time.Time {
	if !m.calendarWeekStart.IsZero() {
		return m.calendarWeekStart
	}
	return calendarWeekStartFor(m.selectedCalendarDay())
}

func (m *Model) selectedCalendarThreeDayStart() time.Time {
	if !m.calendarThreeDayStart.IsZero() {
		return m.calendarThreeDayStart
	}
	return calendarDayStartFor(m.selectedCalendarDay())
}

func (m *Model) setCalendarView(view calendarViewMode) {
	if view == "" {
		view = calendarViewAgenda
	}
	m.calendarView = view
	switch view {
	case calendarViewDay:
		m.calendarDay = m.selectedCalendarDay()
		m.selectFirstCalendarEventForDay(m.calendarDay)
	case calendarViewWeek:
		m.calendarWeekStart = calendarWeekStartFor(m.selectedCalendarDay())
		m.selectFirstCalendarEventForWeek(m.calendarWeekStart)
	case calendarViewThreeDay:
		m.calendarThreeDayStart = calendarDayStartFor(m.selectedCalendarDay())
		m.selectFirstCalendarEventForThreeDay(m.calendarThreeDayStart)
	case calendarViewSearch:
		m.calendarDetail = m.selectedCalendarEvent()
	case calendarViewCrossSearch:
		m.selectCrossSourceSearchResult()
	default:
		m.calendarDetail = m.selectedCalendarEvent()
	}
}

func (m *Model) selectFirstCalendarEventForDay(day time.Time) {
	events := m.calendarEventsForDay(day)
	if len(events) == 0 {
		m.calendarDetail = nil
		return
	}
	m.calendarCursor = events[0].index
	m.calendarDetail = &events[0].event
}

func (m *Model) selectFirstCalendarEventForWeek(start time.Time) {
	events := m.calendarEventsForWeek(start)
	if len(events) == 0 {
		m.calendarDetail = nil
		return
	}
	for _, item := range events {
		if item.index == m.calendarCursor {
			m.calendarDetail = &item.event
			return
		}
	}
	m.calendarCursor = events[0].index
	m.calendarDetail = &events[0].event
}

func (m *Model) selectFirstCalendarEventForThreeDay(start time.Time) {
	events := m.calendarEventsForThreeDay(start)
	if len(events) == 0 {
		m.calendarDetail = nil
		return
	}
	for _, item := range events {
		if item.index == m.calendarCursor {
			m.calendarDetail = &item.event
			return
		}
	}
	m.calendarCursor = events[0].index
	m.calendarDetail = &events[0].event
}

func (m *Model) calendarEventsForDay(day time.Time) []indexedCalendarEvent {
	if day.IsZero() {
		day = m.selectedCalendarDay()
	}
	out := make([]indexedCalendarEvent, 0)
	for i, event := range m.calendarEvents {
		if eventOccursOnCalendarDate(event, day) {
			out = append(out, indexedCalendarEvent{index: i, event: event})
		}
	}
	return out
}

func (m *Model) calendarEventsForWeek(start time.Time) []indexedCalendarEvent {
	if start.IsZero() {
		start = m.selectedCalendarWeekStart()
	}
	out := make([]indexedCalendarEvent, 0)
	for i, event := range m.calendarEvents {
		if eventOccursInCalendarWeek(event, start) {
			out = append(out, indexedCalendarEvent{index: i, event: event})
		}
	}
	return out
}

func (m *Model) calendarEventsForThreeDay(start time.Time) []indexedCalendarEvent {
	if start.IsZero() {
		start = m.selectedCalendarThreeDayStart()
	}
	out := make([]indexedCalendarEvent, 0)
	for i, event := range m.calendarEvents {
		if eventOccursInCalendarRange(event, start, start.AddDate(0, 0, 3)) {
			out = append(out, indexedCalendarEvent{index: i, event: event})
		}
	}
	return out
}

func (m *Model) openCalendarDetail() tea.Cmd {
	event := m.selectedCalendarEvent()
	if event == nil {
		return nil
	}
	m.calendarEdit = calendarEventEditState{}
	m.calendarDetailOpen = true
	m.calendarDetailLoading = true
	m.calendarDetail = event
	agenda, ok := m.calendarAgendaBackend()
	if !ok {
		return nil
	}
	ref := event.Ref.WithDefaults()
	return func() tea.Msg {
		detail, err := agenda.GetCalendarEvent(ref)
		return CalendarEventDetailMsg{Ref: ref, Event: detail, Err: err}
	}
}

func (m *Model) openCalendarSearch() {
	m.calendarView = calendarViewSearch
	m.calendarDetailOpen = false
	m.calendarDetailLoading = false
	m.calendarEdit = calendarEventEditState{}
	m.calendarSearchQuery = ""
	m.calendarSearchResults = nil
	m.calendarSearchCursor = 0
	m.calendarSearchLoading = false
	m.calendarDetail = nil
	m.calendarStatus = "Type to search cached calendar events"
}

func (m *Model) openCrossSourceSearch() {
	m.calendarView = calendarViewCrossSearch
	m.calendarDetailOpen = false
	m.calendarDetailLoading = false
	m.calendarEdit = calendarEventEditState{}
	m.crossSourceSearchQuery = ""
	m.crossSourceSearchResults = nil
	m.crossSourceSearchCursor = 0
	m.crossSourceSearchLoading = false
	m.calendarDetail = nil
	m.calendarStatus = "Type to search cached mail and calendar events"
}

func (m *Model) clearCalendarSearch() {
	m.calendarSearchQuery = ""
	m.calendarSearchResults = nil
	m.calendarSearchCursor = 0
	m.calendarSearchLoading = false
	m.calendarStatus = fmt.Sprintf("Loaded %d calendar event(s)", len(m.calendarEvents))
	m.setCalendarView(calendarViewAgenda)
}

func (m *Model) clearCrossSourceSearch() {
	m.crossSourceSearchQuery = ""
	m.crossSourceSearchResults = nil
	m.crossSourceSearchCursor = 0
	m.crossSourceSearchLoading = false
	m.calendarDetail = m.selectedCalendarEvent()
	m.calendarStatus = fmt.Sprintf("Loaded %d calendar event(s)", len(m.calendarEvents))
	m.setCalendarView(calendarViewAgenda)
}

func (m *Model) handleCalendarSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := shortcutKey(msg)
	switch key {
	case "j", "down":
		if m.calendarSearchCursor < len(m.calendarSearchResults)-1 {
			m.calendarSearchCursor++
			m.calendarDetail = m.selectedCalendarEvent()
		}
		return m, nil
	case "k", "up":
		if m.calendarSearchCursor > 0 {
			m.calendarSearchCursor--
			m.calendarDetail = m.selectedCalendarEvent()
		}
		return m, nil
	case "enter":
		return m, m.openCalendarDetail()
	case "esc":
		m.clearCalendarSearch()
		return m, nil
	case "backspace":
		runes := []rune(m.calendarSearchQuery)
		if len(runes) > 0 {
			m.calendarSearchQuery = string(runes[:len(runes)-1])
			m.calendarSearchCursor = 0
			m.calendarSearchLoading = true
			return m, m.loadCalendarSearch()
		}
		return m, nil
	case "ctrl+u":
		m.calendarSearchQuery = ""
		m.calendarSearchResults = nil
		m.calendarSearchCursor = 0
		m.calendarSearchLoading = false
		m.calendarDetail = nil
		m.calendarStatus = "Type to search cached calendar events"
		return m, nil
	case "ctrl+r":
		m.calendarSearchLoading = true
		m.calendarStatus = "Searching cached calendar events..."
		return m, m.loadCalendarSearch()
	}

	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		m.calendarSearchQuery += msg.Text
		m.calendarSearchCursor = 0
		m.calendarSearchLoading = true
		m.calendarStatus = "Searching cached calendar events..."
		return m, m.loadCalendarSearch()
	}
	return m, nil
}

func (m *Model) selectedCrossSourceSearchResult() *models.CrossSourceSearchResult {
	if m.crossSourceSearchCursor < 0 || m.crossSourceSearchCursor >= len(m.crossSourceSearchResults) {
		return nil
	}
	result := m.crossSourceSearchResults[m.crossSourceSearchCursor]
	return &result
}

func (m *Model) selectCrossSourceSearchResult() {
	result := m.selectedCrossSourceSearchResult()
	if result == nil || result.Event == nil {
		m.calendarDetail = nil
		return
	}
	event := *result.Event
	m.calendarDetail = &event
}

func (m *Model) handleCrossSourceSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := shortcutKey(msg)
	switch key {
	case "j", "down":
		if m.crossSourceSearchCursor < len(m.crossSourceSearchResults)-1 {
			m.crossSourceSearchCursor++
			m.selectCrossSourceSearchResult()
		}
		return m, nil
	case "k", "up":
		if m.crossSourceSearchCursor > 0 {
			m.crossSourceSearchCursor--
			m.selectCrossSourceSearchResult()
		}
		return m, nil
	case "enter":
		if result := m.selectedCrossSourceSearchResult(); result != nil && result.Event != nil {
			return m, m.openCalendarDetail()
		}
		if result := m.selectedCrossSourceSearchResult(); result != nil && result.Email != nil {
			m.calendarStatus = "Mail results are shown read-only in this cross-source slice"
		}
		return m, nil
	case "esc":
		m.clearCrossSourceSearch()
		return m, nil
	case "backspace":
		runes := []rune(m.crossSourceSearchQuery)
		if len(runes) > 0 {
			m.crossSourceSearchQuery = string(runes[:len(runes)-1])
			m.crossSourceSearchCursor = 0
			m.crossSourceSearchLoading = true
			m.calendarStatus = "Searching cached mail and calendar events..."
			return m, m.loadCrossSourceSearch()
		}
		return m, nil
	case "ctrl+u":
		m.crossSourceSearchQuery = ""
		m.crossSourceSearchResults = nil
		m.crossSourceSearchCursor = 0
		m.crossSourceSearchLoading = false
		m.calendarDetail = nil
		m.calendarStatus = "Type to search cached mail and calendar events"
		return m, nil
	case "ctrl+r":
		m.crossSourceSearchLoading = true
		m.calendarStatus = "Searching cached mail and calendar events..."
		return m, m.loadCrossSourceSearch()
	}

	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		m.crossSourceSearchQuery += msg.Text
		m.crossSourceSearchCursor = 0
		m.crossSourceSearchLoading = true
		m.calendarStatus = "Searching cached mail and calendar events..."
		return m, m.loadCrossSourceSearch()
	}
	return m, nil
}

func (m *Model) openCalendarEdit() {
	event := m.calendarDetail
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	if event == nil {
		return
	}
	draft := models.NewCalendarEventEditDraft(*event)
	m.calendarEdit = calendarEventEditState{
		Active: true,
		Ref:    event.Ref.WithDefaults(),
		Base:   *event,
		Draft:  draft,
		Field:  calendarEditFieldTitle,
	}
	m.calendarDetailOpen = true
	m.calendarDetailLoading = false
	m.calendarStatus = "Editing cached calendar event"
}

func (m *Model) saveCalendarEdit() tea.Cmd {
	if !m.calendarEdit.Active || m.calendarEdit.Saving {
		return nil
	}
	updated, err := m.calendarEdit.Draft.Apply(m.calendarEdit.Base)
	if err != nil {
		m.calendarEdit.Error = "Validation: " + err.Error()
		m.calendarStatus = m.calendarEdit.Error
		return nil
	}
	mutation, ok := m.calendarMutationBackend()
	if !ok {
		m.calendarEdit.Error = "Save failed: calendar edit backend unavailable"
		m.calendarStatus = m.calendarEdit.Error
		return nil
	}
	ref := updated.Ref.WithDefaults()
	m.calendarEdit.Ref = ref
	m.calendarEdit.Saving = true
	m.calendarEdit.Error = ""
	m.calendarStatus = "Saving calendar event..."
	return func() tea.Msg {
		saved, err := mutation.SaveCalendarEvent(updated)
		return CalendarEventSavedMsg{Ref: ref, Event: saved, Err: err}
	}
}

func (m *Model) saveCalendarRSVP() tea.Cmd {
	event := m.calendarDetail
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	if event == nil {
		return nil
	}
	mutation, ok := m.calendarMutationBackend()
	if !ok {
		m.calendarStatus = "RSVP failed: calendar mutation backend unavailable"
		return nil
	}
	ref := event.Ref.WithDefaults()
	status := models.NextCalendarRSVP(firstCalendarAttendeeRSVP(*event))
	m.calendarStatus = "Saving RSVP " + status + "..."
	return func() tea.Msg {
		saved, err := mutation.RespondCalendarEvent(ref, status)
		return CalendarEventRSVPMsg{Ref: ref, Status: status, Event: saved, Err: err}
	}
}

func (m *Model) handleCalendarEditKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := shortcutKey(msg)
	switch key {
	case "ctrl+s":
		return m, m.saveCalendarEdit()
	case "esc":
		m.calendarEdit = calendarEventEditState{}
		m.calendarStatus = "Calendar edit cancelled"
		return m, nil
	case "tab":
		m.moveCalendarEditField(1)
		return m, nil
	case "shift+tab":
		m.moveCalendarEditField(-1)
		return m, nil
	case "up":
		m.moveCalendarEditField(-1)
		return m, nil
	case "down":
		m.moveCalendarEditField(1)
		return m, nil
	case "backspace":
		m.backspaceCalendarEditField()
		return m, nil
	case "ctrl+u":
		m.setCalendarEditFieldValue("")
		return m, nil
	}
	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		m.appendCalendarEditFieldValue(msg.Text)
		return m, nil
	}
	return m, nil
}

func (m *Model) moveCalendarEditField(delta int) {
	if len(calendarEditFields) == 0 {
		return
	}
	idx := 0
	for i, field := range calendarEditFields {
		if field == m.calendarEdit.Field {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = len(calendarEditFields) - 1
	}
	if idx >= len(calendarEditFields) {
		idx = 0
	}
	m.calendarEdit.Field = calendarEditFields[idx]
}

func (m *Model) appendCalendarEditFieldValue(value string) {
	m.setCalendarEditFieldValue(m.calendarEditFieldValue() + value)
}

func (m *Model) backspaceCalendarEditField() {
	value := []rune(m.calendarEditFieldValue())
	if len(value) == 0 {
		return
	}
	m.setCalendarEditFieldValue(string(value[:len(value)-1]))
}

func (m *Model) calendarEditFieldValue() string {
	switch m.calendarEdit.Field {
	case calendarEditFieldTitle:
		return m.calendarEdit.Draft.Title
	case calendarEditFieldLocation:
		return m.calendarEdit.Draft.Location
	case calendarEditFieldStart:
		return m.calendarEdit.Draft.StartText
	case calendarEditFieldEnd:
		return m.calendarEdit.Draft.EndText
	case calendarEditFieldTimeZone:
		return m.calendarEdit.Draft.TimeZone
	case calendarEditFieldAttendees:
		return m.calendarEdit.Draft.AttendeesText
	case calendarEditFieldRecurrence:
		return m.calendarEdit.Draft.RecurrenceText
	case calendarEditFieldDescription:
		return m.calendarEdit.Draft.Description
	default:
		return ""
	}
}

func (m *Model) setCalendarEditFieldValue(value string) {
	switch m.calendarEdit.Field {
	case calendarEditFieldTitle:
		m.calendarEdit.Draft.Title = value
	case calendarEditFieldLocation:
		m.calendarEdit.Draft.Location = value
	case calendarEditFieldStart:
		m.calendarEdit.Draft.StartText = value
	case calendarEditFieldEnd:
		m.calendarEdit.Draft.EndText = value
	case calendarEditFieldTimeZone:
		m.calendarEdit.Draft.TimeZone = value
	case calendarEditFieldAttendees:
		m.calendarEdit.Draft.AttendeesText = value
	case calendarEditFieldRecurrence:
		m.calendarEdit.Draft.RecurrenceText = value
	case calendarEditFieldDescription:
		m.calendarEdit.Draft.Description = value
	}
	m.calendarEdit.Dirty = true
	m.calendarEdit.Error = ""
}

func (m *Model) applySavedCalendarEvent(event models.CalendarEvent) {
	event.Ref = event.Ref.WithDefaults()
	for i := range m.calendarEvents {
		if m.calendarEvents[i].Ref.WithDefaults().LocalID == event.Ref.LocalID {
			m.calendarEvents[i] = event
		}
	}
	for i := range m.calendarSearchResults {
		if m.calendarSearchResults[i].Ref.WithDefaults().LocalID == event.Ref.LocalID {
			m.calendarSearchResults[i] = event
		}
	}
	for i := range m.crossSourceSearchResults {
		result := &m.crossSourceSearchResults[i]
		if result.Event != nil && result.Event.Ref.WithDefaults().LocalID == event.Ref.LocalID {
			updated := event
			result.Event = &updated
			result.When = event.Start
		}
	}
	m.calendarDetail = &event
}

func (m *Model) handleCalendarKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := shortcutKey(msg)
	if m.calendarEdit.Active {
		return m.handleCalendarEditKey(msg)
	}
	if m.calendarView == calendarViewSearch && !m.calendarDetailOpen {
		return m.handleCalendarSearchKey(msg)
	}
	if m.calendarView == calendarViewCrossSearch && !m.calendarDetailOpen {
		return m.handleCrossSourceSearchKey(msg)
	}
	switch key {
	case "j", "down":
		if !m.calendarDetailOpen {
			if m.calendarView == calendarViewDay {
				m.moveCalendarDaySelection(1)
			} else if m.calendarView == calendarViewWeek {
				m.moveCalendarWeekSelection(1)
			} else if m.calendarView == calendarViewThreeDay {
				m.moveCalendarThreeDaySelection(1)
			} else if m.calendarCursor < len(m.calendarEvents)-1 {
				m.calendarCursor++
				m.calendarDetail = m.selectedCalendarEvent()
			}
		}
		return m, nil
	case "k", "up":
		if !m.calendarDetailOpen {
			if m.calendarView == calendarViewDay {
				m.moveCalendarDaySelection(-1)
			} else if m.calendarView == calendarViewWeek {
				m.moveCalendarWeekSelection(-1)
			} else if m.calendarView == calendarViewThreeDay {
				m.moveCalendarThreeDaySelection(-1)
			} else if m.calendarCursor > 0 {
				m.calendarCursor--
				m.calendarDetail = m.selectedCalendarEvent()
			}
		}
		return m, nil
	case "d":
		if !m.calendarDetailOpen {
			m.setCalendarView(calendarViewDay)
		}
		return m, nil
	case "a":
		if !m.calendarDetailOpen {
			m.setCalendarView(calendarViewAgenda)
		}
		return m, nil
	case "w":
		if !m.calendarDetailOpen {
			m.setCalendarView(calendarViewWeek)
		}
		return m, nil
	case "t":
		if !m.calendarDetailOpen {
			m.setCalendarView(calendarViewThreeDay)
		}
		return m, nil
	case "/":
		if !m.calendarDetailOpen {
			m.openCalendarSearch()
		}
		return m, nil
	case "x":
		if !m.calendarDetailOpen && m.crossSourceSearchAvailable() {
			m.openCrossSourceSearch()
		}
		return m, nil
	case "h", "left":
		if !m.calendarDetailOpen && m.calendarView == calendarViewDay {
			m.calendarDay = m.selectedCalendarDay().AddDate(0, 0, -1)
			m.selectFirstCalendarEventForDay(m.calendarDay)
		} else if !m.calendarDetailOpen && m.calendarView == calendarViewWeek {
			m.calendarWeekStart = m.selectedCalendarWeekStart().AddDate(0, 0, -7)
			m.selectFirstCalendarEventForWeek(m.calendarWeekStart)
		} else if !m.calendarDetailOpen && m.calendarView == calendarViewThreeDay {
			m.calendarThreeDayStart = m.selectedCalendarThreeDayStart().AddDate(0, 0, -1)
			m.selectFirstCalendarEventForThreeDay(m.calendarThreeDayStart)
		}
		return m, nil
	case "l", "right":
		if !m.calendarDetailOpen && m.calendarView == calendarViewDay {
			m.calendarDay = m.selectedCalendarDay().AddDate(0, 0, 1)
			m.selectFirstCalendarEventForDay(m.calendarDay)
		} else if !m.calendarDetailOpen && m.calendarView == calendarViewWeek {
			m.calendarWeekStart = m.selectedCalendarWeekStart().AddDate(0, 0, 7)
			m.selectFirstCalendarEventForWeek(m.calendarWeekStart)
		} else if !m.calendarDetailOpen && m.calendarView == calendarViewThreeDay {
			m.calendarThreeDayStart = m.selectedCalendarThreeDayStart().AddDate(0, 0, 1)
			m.selectFirstCalendarEventForThreeDay(m.calendarThreeDayStart)
		}
		return m, nil
	case "enter":
		if !m.calendarDetailOpen {
			return m, m.openCalendarDetail()
		}
		return m, nil
	case "e":
		if m.calendarDetailOpen {
			m.openCalendarEdit()
		}
		return m, nil
	case "v":
		if m.calendarDetailOpen {
			return m, m.saveCalendarRSVP()
		}
		return m, nil
	case "esc":
		if m.calendarDetailOpen {
			m.calendarEdit = calendarEventEditState{}
			m.calendarDetailOpen = false
			m.calendarDetailLoading = false
			return m, nil
		}
		return m, nil
	case "r", "ctrl+r":
		m.calendarLoading = true
		m.calendarStatus = "Refreshing calendar agenda..."
		return m, m.loadCalendarAgenda()
	}
	return m, nil
}

func (m *Model) renderCalendarView() string {
	if !m.calendarAvailable {
		return m.emptyStateView("Calendar agenda is not configured for this session")
	}
	if m.calendarEdit.Active {
		return m.renderCalendarEditFullView()
	}
	if m.calendarDetailOpen {
		return m.renderCalendarDetailFullView()
	}
	if m.calendarView == "" {
		m.calendarView = calendarViewAgenda
	}
	if m.calendarView == calendarViewDay {
		return m.renderCalendarDayView()
	}
	if m.calendarView == calendarViewWeek {
		return m.renderCalendarWeekView()
	}
	if m.calendarView == calendarViewThreeDay {
		return m.renderCalendarThreeDayView()
	}
	if m.calendarView == calendarViewSearch {
		return m.renderCalendarSearchView()
	}
	if m.calendarView == calendarViewCrossSearch {
		return m.renderCrossSourceSearchView()
	}

	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	available := clamp(contentW-panelGapWidth, 40)
	leftW, rightW := splitWidth(available, 0, 26, 28, available*48/100)
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}

	leftPanel := m.calendarPanel(leftW, contentH, true).Render(m.renderCalendarAgendaList(leftW-4, contentH-2))
	rightPanel := m.calendarPanel(rightW, contentH, false).Render(m.renderCalendarEventDetail(rightW-4, contentH-2, false))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, panelGap, rightPanel)
}

func (m *Model) renderCalendarSearchView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	available := clamp(contentW-panelGapWidth, 40)
	leftW, rightW := splitWidth(available, 0, 28, 28, available*50/100)
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}

	leftPanel := m.calendarPanel(leftW, contentH, true).Render(m.renderCalendarSearchResults(leftW-4, contentH-2))
	rightPanel := m.calendarPanel(rightW, contentH, false).Render(m.renderCalendarEventDetailWithHeader(rightW-4, contentH-2, false, "Search Detail"))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, panelGap, rightPanel)
}

func (m *Model) renderCrossSourceSearchView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	available := clamp(contentW-panelGapWidth, 40)
	leftW, rightW := splitWidth(available, 0, 30, 30, available*50/100)
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}

	leftPanel := m.calendarPanel(leftW, contentH, true).Render(m.renderCrossSourceSearchResults(leftW-4, contentH-2))
	rightPanel := m.calendarPanel(rightW, contentH, false).Render(m.renderCrossSourceSearchDetail(rightW-4, contentH-2))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, panelGap, rightPanel)
}

func (m *Model) renderCalendarDayView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	available := clamp(contentW-panelGapWidth, 40)
	leftW, rightW := splitWidth(available, 0, 26, 28, available*54/100)
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}

	leftPanel := m.calendarPanel(leftW, contentH, true).Render(m.renderCalendarDayAgenda(leftW-4, contentH-2))
	rightPanel := m.calendarPanel(rightW, contentH, false).Render(m.renderCalendarDayDrawer(rightW-4, contentH-2))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, panelGap, rightPanel)
}

func (m *Model) renderCalendarWeekView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	available := clamp(contentW-panelGapWidth, 40)
	leftW, rightW := splitWidth(available, 0, 28, 28, available*58/100)
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}

	leftPanel := m.calendarPanel(leftW, contentH, true).Render(m.renderCalendarWeekGrid(leftW-4, contentH-2))
	rightPanel := m.calendarPanel(rightW, contentH, false).Render(m.renderCalendarWeekInspector(rightW-4, contentH-2))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, panelGap, rightPanel)
}

func (m *Model) renderCalendarThreeDayView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	available := clamp(contentW-panelGapWidth, 40)
	leftW, rightW := splitWidth(available, 0, 30, 30, available*58/100)
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}

	leftPanel := m.calendarPanel(leftW, contentH, true).Render(m.renderCalendarThreeDayLanes(leftW-4, contentH-2))
	rightPanel := m.calendarPanel(rightW, contentH, false).Render(m.renderCalendarThreeDayCommandPanel(rightW-4, contentH-2))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, panelGap, rightPanel)
}

func (m *Model) renderCalendarDetailFullView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}
	return m.calendarPanel(clamp(contentW, 40), contentH, true).Render(m.renderCalendarEventDetail(clamp(contentW-4, 20), contentH-2, true))
}

func (m *Model) renderCalendarEditFullView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	contentH := plan.ContentHeight
	if contentH < 4 {
		contentH = 4
	}
	return m.calendarPanel(clamp(contentW, 40), contentH, true).Render(m.renderCalendarEventEdit(clamp(contentW-4, 20), contentH-2))
}

func (m *Model) calendarPanel(width, height int, active bool) lipgloss.Style {
	border := m.theme.Focus.PanelBorder.ForegroundColor()
	if active {
		border = m.theme.Focus.PanelBorderFocused.ForegroundColor()
	}
	return m.baseStyle.
		Width(width).
		Height(height).
		PaddingLeft(1).
		PaddingRight(1).
		BorderForeground(border)
}

func (m *Model) renderCalendarAgendaList(width, height int) string {
	if width < 10 {
		width = 10
	}
	var lines []string
	title := fmt.Sprintf("Agenda (%d)", len(m.calendarEvents))
	if m.calendarLoading {
		title = "Agenda (loading)"
	}
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit(title, width)))
	if strings.TrimSpace(m.calendarStatus) != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(m.calendarStatus, width)))
	}
	if len(m.calendarEvents) == 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached calendar events", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if m.calendarCursor >= maxRows {
		start = m.calendarCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.calendarEvents) {
		end = len(m.calendarEvents)
	}
	for i := start; i < end; i++ {
		event := m.calendarEvents[i]
		line := calendarAgendaLine(event, width)
		if i == m.calendarCursor {
			line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarSearchResults(width, height int) string {
	if width < 10 {
		width = 10
	}
	var lines []string
	title := fmt.Sprintf("Calendar Search (%d)", len(m.calendarSearchResults))
	if m.calendarSearchLoading {
		title = "Calendar Search (loading)"
	}
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit(title, width)))
	queryLine := "/ " + m.calendarSearchQuery
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(queryLine, width)))
	if strings.TrimSpace(m.calendarStatus) != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(m.calendarStatus, width)))
	}
	if strings.TrimSpace(m.calendarSearchQuery) == "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Type to search cached calendar events", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}
	if len(m.calendarSearchResults) == 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached event matches", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if m.calendarSearchCursor >= maxRows {
		start = m.calendarSearchCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.calendarSearchResults) {
		end = len(m.calendarSearchResults)
	}
	for i := start; i < end; i++ {
		event := m.calendarSearchResults[i]
		line := calendarSearchLine(event, m.calendarSearchQuery, width)
		if i == m.calendarSearchCursor {
			line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCrossSourceSearchResults(width, height int) string {
	if width < 10 {
		width = 10
	}
	var lines []string
	title := fmt.Sprintf("Cross-Source Search (%d)", len(m.crossSourceSearchResults))
	if m.crossSourceSearchLoading {
		title = "Cross-Source Search (loading)"
	}
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit(title, width)))
	queryLine := "x " + m.crossSourceSearchQuery
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(queryLine, width)))
	if strings.TrimSpace(m.calendarStatus) != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(m.calendarStatus, width)))
	}
	if strings.TrimSpace(m.crossSourceSearchQuery) == "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Type to search cached mail and events", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}
	if len(m.crossSourceSearchResults) == 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached mail or event matches", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if m.crossSourceSearchCursor >= maxRows {
		start = m.crossSourceSearchCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.crossSourceSearchResults) {
		end = len(m.crossSourceSearchResults)
	}
	for i := start; i < end; i++ {
		result := m.crossSourceSearchResults[i]
		line := m.crossSourceSearchLine(result, width)
		if i == m.crossSourceSearchCursor {
			line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCrossSourceSearchDetail(width, height int) string {
	if width < 12 {
		width = 12
	}
	result := m.selectedCrossSourceSearchResult()
	if result == nil {
		return fitPanelContentHeight(m.theme.Text.Dim.Style().Render("No result selected"), height)
	}
	if result.Event != nil {
		m.selectCrossSourceSearchResult()
		return m.renderCalendarEventDetailWithHeader(width, height, false, "Event Detail")
	}
	if result.Email == nil {
		return fitPanelContentHeight(m.theme.Text.Dim.Style().Render("No result selected"), height)
	}
	email := result.Email
	account := m.accountBadgeForEmail(email)
	if strings.TrimSpace(account) == "" {
		account = string(email.SourceID)
	}
	if strings.TrimSpace(account) == "" {
		account = string(email.AccountID)
	}
	if strings.TrimSpace(account) == "" {
		account = "mail"
	}
	match := result.MatchHint
	if strings.TrimSpace(match) == "" {
		match = models.EmailSearchMatchHint(email, m.crossSourceSearchQuery)
	}
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("Mail Detail", width)))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("read-only cached mail result", width)))
	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Subject.Style().Render(calendarFit(email.Subject, width)))
	lines = append(lines, calendarDetailRow(m, "From", email.Sender, width))
	lines = append(lines, calendarDetailRow(m, "When", email.Date.Local().Format("Mon Jan 2 15:04"), width))
	lines = append(lines, calendarDetailRow(m, "Folder", email.Folder, width))
	lines = append(lines, calendarDetailRow(m, "Account", account, width))
	if strings.TrimSpace(match) != "" {
		lines = append(lines, calendarDetailRow(m, "Match", match, width))
	}
	lines = append(lines, calendarDetailRow(m, "Mode", "read-only", width))
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarEventDetail(width, height int, full bool) string {
	return m.renderCalendarEventDetailWithHeader(width, height, full, "Event Detail")
}

func (m *Model) renderCalendarEventEdit(width, height int) string {
	if width < 12 {
		width = 12
	}
	state := m.calendarEdit
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("Event Edit", width)))
	subtitle := "provider save-through when available"
	if state.Dirty {
		subtitle += " - unsaved"
	}
	if state.Saving {
		subtitle = "saving calendar event..."
	}
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(subtitle, width)))
	lines = append(lines, "")
	lines = append(lines, m.renderCalendarEditField("Title", calendarEditFieldTitle, state.Draft.Title, width))
	lines = append(lines, m.renderCalendarEditField("Location", calendarEditFieldLocation, state.Draft.Location, width))
	lines = append(lines, m.renderCalendarEditField("Start", calendarEditFieldStart, state.Draft.StartText, width))
	lines = append(lines, m.renderCalendarEditField("End", calendarEditFieldEnd, state.Draft.EndText, width))
	lines = append(lines, m.renderCalendarEditField("Event TZ", calendarEditFieldTimeZone, state.Draft.TimeZone, width))
	lines = append(lines, m.renderCalendarEditField("Attendees", calendarEditFieldAttendees, state.Draft.AttendeesText, width))
	lines = append(lines, m.renderCalendarEditField("Recurrence", calendarEditFieldRecurrence, state.Draft.RecurrenceText, width))
	lines = append(lines, m.renderCalendarEditField("Notes", calendarEditFieldDescription, state.Draft.Description, width))
	if strings.TrimSpace(state.Error) != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Validation", width)))
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(state.Error, width)))
	}
	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Preview", width)))
	preview, err := state.Draft.TimezonePreview(state.Base)
	if err != nil {
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit("Validation: "+err.Error(), width)))
	} else {
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(preview.Local, width)))
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(preview.Event, width)))
		for _, alt := range preview.Alternates {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(alt, width)))
		}
		if strings.TrimSpace(preview.DateCrossingNote) != "" {
			lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(preview.DateCrossingNote, width)))
		}
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Alt TZ rows are preview only; Event TZ saves.", width)))
	}
	lines = append(lines, "")
	lines = append(lines, calendarDetailRow(m, "Scope", models.CalendarMutationScopeLabel(state.Draft.RecurrenceScope), width))
	lines = append(lines, calendarDetailRow(m, "Mode", "provider save-through, cache after success", width))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("tab: next field  ctrl+s: save  esc: cancel", width)))
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarEditField(label string, field calendarEditField, value string, width int) string {
	prefix := "  "
	if m.calendarEdit.Field == field {
		prefix = "> "
	}
	if strings.TrimSpace(value) == "" {
		value = "--"
	}
	labelText := calendarFit(label+":", 11)
	valueW := width - ansi.StringWidth(labelText) - 3
	if valueW < 4 {
		valueW = 4
	}
	line := prefix + labelText + " " + calendarFit(value, valueW)
	if m.calendarEdit.Field == field {
		return m.theme.Focus.SelectionActive.Style().Render(calendarFit(line, width))
	}
	return m.theme.Text.Primary.Style().Render(calendarFit(line, width))
}

func calendarMutationErrorStatus(prefix string, err error) string {
	if err == nil {
		return prefix
	}
	if errors.Is(err, models.ErrCalendarMutationConflict) {
		return prefix + ": provider conflict; refresh calendar and try again"
	}
	if errors.Is(err, models.ErrCalendarRecurrenceScopeUnsupported) {
		return prefix + ": recurrence scope unsupported"
	}
	return prefix + ": " + err.Error()
}

func (m *Model) renderCalendarDayAgenda(width, height int) string {
	if width < 10 {
		width = 10
	}
	day := m.selectedCalendarDay()
	events := m.calendarEventsForDay(day)
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("Day Agenda", width)))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(day.Local().Format("Mon Jan 2, 2006"), width)))
	if strings.TrimSpace(m.calendarStatus) != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(m.calendarStatus, width)))
	}
	if len(events) == 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No events on this day", width)))
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("h/l: previous/next day", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	selectedOffset := 0
	for i, item := range events {
		if item.index == m.calendarCursor {
			selectedOffset = i
			break
		}
	}
	start := 0
	if selectedOffset >= maxRows {
		start = selectedOffset - maxRows + 1
	}
	end := start + maxRows
	if end > len(events) {
		end = len(events)
	}
	for _, item := range events[start:end] {
		line := calendarDayAgendaLine(item.event, width)
		if item.index == m.calendarCursor {
			line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarDayDrawer(width, height int) string {
	return m.renderCalendarEventDetailWithHeader(width, height, false, "Day Drawer")
}

func (m *Model) renderCalendarWeekGrid(width, height int) string {
	if width < 10 {
		width = 10
	}
	start := m.selectedCalendarWeekStart()
	events := m.calendarEventsForWeek(start)
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("Week Time-Grid", width)))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(calendarWeekRange(start), width)))
	if strings.TrimSpace(m.calendarStatus) != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(m.calendarStatus, width)))
	}

	rows, selectedOffset := m.calendarWeekRows(start, width)
	if len(events) == 0 {
		rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit("No events this week", width)))
		rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit("h/l: previous/next week", width)))
	}
	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	if selectedOffset < 0 {
		selectedOffset = 0
	}
	startRow := 0
	if selectedOffset >= maxRows {
		startRow = selectedOffset - maxRows + 1
	}
	endRow := startRow + maxRows
	if endRow > len(rows) {
		endRow = len(rows)
	}
	lines = append(lines, rows[startRow:endRow]...)
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) calendarWeekRows(start time.Time, width int) ([]string, int) {
	rows := make([]string, 0, 14)
	selectedOffset := -1
	for i := 0; i < 7; i++ {
		day := start.AddDate(0, 0, i)
		rows = append(rows, m.theme.Metadata.Label.Style().Render(calendarFit(day.Local().Format("Mon Jan 2"), width)))
		events := m.calendarEventsForDay(day)
		if len(events) == 0 {
			rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit("  --", width)))
			continue
		}
		for _, item := range events {
			line := calendarWeekEventLine(item.event, width)
			if item.index == m.calendarCursor {
				selectedOffset = len(rows)
				line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
			} else {
				line = "  " + line
			}
			rows = append(rows, line)
		}
	}
	return rows, selectedOffset
}

func (m *Model) renderCalendarWeekInspector(width, height int) string {
	return m.renderCalendarEventDetailWithHeader(width, height, false, "Week Inspector")
}

func (m *Model) renderCalendarThreeDayLanes(width, height int) string {
	if width < 10 {
		width = 10
	}
	start := m.selectedCalendarThreeDayStart()
	events := m.calendarEventsForThreeDay(start)
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("3-Day Command", width)))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(calendarThreeDayRange(start), width)))
	if strings.TrimSpace(m.calendarStatus) != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(m.calendarStatus, width)))
	}

	rows, selectedOffset := m.calendarThreeDayRows(start, width)
	if len(events) == 0 {
		rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit("No events in this 3-day window", width)))
		rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit("h/l: slide 3-day window", width)))
	}
	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	if selectedOffset < 0 {
		selectedOffset = 0
	}
	startRow := 0
	if selectedOffset >= maxRows {
		startRow = selectedOffset - maxRows + 1
	}
	endRow := startRow + maxRows
	if endRow > len(rows) {
		endRow = len(rows)
	}
	lines = append(lines, rows[startRow:endRow]...)
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) calendarThreeDayRows(start time.Time, width int) ([]string, int) {
	rows := make([]string, 0, 12)
	selectedOffset := -1
	for i := 0; i < 3; i++ {
		day := start.AddDate(0, 0, i)
		label := day.Local().Format("Mon Jan 2")
		if i == 0 {
			label += "  today"
		} else if i == 1 {
			label += "  tomorrow"
		}
		rows = append(rows, m.theme.Metadata.Label.Style().Render(calendarFit(label, width)))
		events := m.calendarEventsForDay(day)
		if len(events) == 0 {
			rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit("  open day", width)))
			continue
		}
		for _, item := range events {
			line := calendarThreeDayEventLine(item.event, width)
			if item.index == m.calendarCursor {
				selectedOffset = len(rows)
				line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
			} else {
				line = "  " + line
			}
			rows = append(rows, line)
		}
	}
	return rows, selectedOffset
}

func (m *Model) renderCalendarThreeDayCommandPanel(width, height int) string {
	if width < 12 {
		width = 12
	}
	start := m.selectedCalendarThreeDayStart()
	events := m.calendarEventsForThreeDay(start)
	event := m.calendarDetail
	if event == nil {
		event = m.selectedCalendarEvent()
	}

	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("Command Panel", width)))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("read-only 3-day planning", width)))
	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Selected", width)))
	if event == nil {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No event selected", width)))
	} else {
		lines = append(lines, m.theme.Metadata.Subject.Style().Render(calendarFit(event.Title, width)))
		lines = append(lines, calendarDetailRow(m, "Time", calendarTimeRange(*event), width))
		lines = append(lines, calendarDetailRow(m, "Calendar", calendarSourceLabel(*event), width))
		lines = append(lines, calendarDetailRow(m, "Mode", "read-only", width))
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Next Up", width)))
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(m.calendarNextUpSummary(events), width)))

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Open Slots", width)))
	for _, slot := range calendarOpenSlotSummaries(start, events, width) {
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(slot, width)))
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Conflicts", width)))
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarConflictSummary(events), width)))

	if event != nil && strings.TrimSpace(event.Description) != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Notes", width)))
		for _, line := range wrapText(event.Description, width) {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(line, width)))
		}
	}

	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarEventDetailWithHeader(width, height int, full bool, header string) string {
	if width < 12 {
		width = 12
	}
	event := m.calendarDetail
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	if event == nil {
		return fitPanelContentHeight(m.theme.Text.Dim.Style().Render("No event selected"), height)
	}

	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit(header, width)))
	if m.calendarDetailLoading {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Loading latest cached detail...", width)))
	}
	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Subject.Style().Render(calendarFit(event.Title, width)))
	lines = append(lines, calendarDetailRow(m, "Time", calendarTimeRange(*event), width))
	if !event.AllDay && !event.Start.IsZero() {
		lines = append(lines, calendarDetailRow(m, "Local", calendarTimeRangeInLocation(*event, time.Local), width))
		lines = append(lines, calendarDetailRow(m, "Event TZ", calendarTimeRangeInNamedLocation(*event, event.CanonicalTimeZone()), width))
		for _, timezone := range calendarAlternateTimeZones(*event) {
			lines = append(lines, calendarDetailRow(m, "Alt TZ", calendarTimeRangeInNamedLocation(*event, timezone), width))
		}
	}
	if strings.TrimSpace(event.Location) != "" {
		lines = append(lines, calendarDetailRow(m, "Location", event.Location, width))
	}
	if organizer := calendarOrganizerLabel(*event); organizer != "" {
		lines = append(lines, calendarDetailRow(m, "Organizer", organizer, width))
	}
	if len(event.Attendees) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Attendees", width)))
		for _, attendee := range event.Attendees {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarAttendeeLabel(attendee), width)))
		}
	}
	if recurrence := calendarRecurrenceLabel(*event); recurrence != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Recurrence", width)))
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(recurrence, width)))
	}
	if len(event.Attachments) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Attachments", width)))
		for _, attachment := range event.Attachments {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarAttachmentLabel(attachment), width)))
		}
	}
	lines = append(lines, calendarDetailRow(m, "Status", calendarStatusLabel(*event), width))
	lines = append(lines, calendarDetailRow(m, "Calendar", calendarSourceLabel(*event), width))
	lines = append(lines, calendarDetailRow(m, "Scope", "this event", width))
	lines = append(lines, calendarDetailRow(m, "Mode", "provider-backed edit/RSVP", width))
	if strings.TrimSpace(event.Description) != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Notes", width)))
		for _, line := range wrapText(event.Description, width) {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(line, width)))
		}
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) moveCalendarDaySelection(delta int) {
	events := m.calendarEventsForDay(m.selectedCalendarDay())
	if len(events) == 0 {
		return
	}
	selectedOffset := 0
	for i, item := range events {
		if item.index == m.calendarCursor {
			selectedOffset = i
			break
		}
	}
	selectedOffset += delta
	if selectedOffset < 0 {
		selectedOffset = 0
	}
	if selectedOffset >= len(events) {
		selectedOffset = len(events) - 1
	}
	m.calendarCursor = events[selectedOffset].index
	m.calendarDetail = &events[selectedOffset].event
}

func (m *Model) moveCalendarWeekSelection(delta int) {
	events := m.calendarEventsForWeek(m.selectedCalendarWeekStart())
	if len(events) == 0 {
		return
	}
	selectedOffset := 0
	for i, item := range events {
		if item.index == m.calendarCursor {
			selectedOffset = i
			break
		}
	}
	selectedOffset += delta
	if selectedOffset < 0 {
		selectedOffset = 0
	}
	if selectedOffset >= len(events) {
		selectedOffset = len(events) - 1
	}
	m.calendarCursor = events[selectedOffset].index
	m.calendarDetail = &events[selectedOffset].event
}

func (m *Model) moveCalendarThreeDaySelection(delta int) {
	events := m.calendarEventsForThreeDay(m.selectedCalendarThreeDayStart())
	if len(events) == 0 {
		return
	}
	selectedOffset := 0
	for i, item := range events {
		if item.index == m.calendarCursor {
			selectedOffset = i
			break
		}
	}
	selectedOffset += delta
	if selectedOffset < 0 {
		selectedOffset = 0
	}
	if selectedOffset >= len(events) {
		selectedOffset = len(events) - 1
	}
	m.calendarCursor = events[selectedOffset].index
	m.calendarDetail = &events[selectedOffset].event
}

func (m *Model) calendarNextUpSummary(events []indexedCalendarEvent) string {
	if len(events) == 0 {
		return "No events in window"
	}
	selectedOffset := -1
	for i, item := range events {
		if item.index == m.calendarCursor {
			selectedOffset = i
			break
		}
	}
	if selectedOffset < 0 {
		return calendarCompactEventSummary(events[0].event)
	}
	if selectedOffset+1 < len(events) {
		return calendarCompactEventSummary(events[selectedOffset+1].event)
	}
	return "No later events in window"
}

func calendarAgendaLine(event models.CalendarEvent, width int) string {
	timeText := calendarShortTime(event)
	source := calendarSourceLabel(event)
	prefixW := 18
	if width < 48 {
		prefixW = 12
	}
	titleW := width - prefixW - 2
	if titleW < 8 {
		titleW = 8
	}
	prefix := calendarFit(timeText, prefixW)
	return calendarFit(prefix+"  "+ansi.Truncate(event.Title+" - "+source, titleW, "..."), width-2)
}

func calendarSearchLine(event models.CalendarEvent, query string, width int) string {
	timeText := calendarShortTime(event)
	source := calendarSourceLabel(event)
	hint := calendarSearchMatchHint(event, query)
	prefixW := 18
	if width < 48 {
		prefixW = 12
	}
	titleW := width - prefixW - 2
	if titleW < 8 {
		titleW = 8
	}
	summary := event.Title + " - " + source
	if hint != "" {
		summary += " [" + hint + "]"
	}
	prefix := calendarFit(timeText, prefixW)
	return calendarFit(prefix+"  "+ansi.Truncate(summary, titleW, "..."), width-2)
}

func (m *Model) crossSourceSearchLine(result models.CrossSourceSearchResult, width int) string {
	timeText := crossSourceShortTime(result.When)
	kind := string(result.Kind)
	summary := ""
	hint := strings.TrimSpace(result.MatchHint)
	switch result.Kind {
	case models.CrossSourceResultMail:
		if result.Email != nil {
			summary = result.Email.Sender + " - " + result.Email.Subject
			if account := m.accountBadgeForEmail(result.Email); strings.TrimSpace(account) != "" {
				summary += " - " + account
			}
			if hint == "" {
				hint = models.EmailSearchMatchHint(result.Email, m.crossSourceSearchQuery)
			}
		}
	case models.CrossSourceResultEvent:
		if result.Event != nil {
			summary = result.Event.Title + " - " + calendarSourceLabel(*result.Event)
			if hint == "" {
				hint = models.CalendarEventSearchMatchHint(*result.Event, m.crossSourceSearchQuery)
			}
		}
	}
	if summary == "" {
		summary = "Untitled result"
	}
	if hint != "" {
		summary += " [" + hint + "]"
	}
	prefixW := 24
	if width < 56 {
		prefixW = 18
	}
	titleW := width - prefixW - 2
	if titleW < 8 {
		titleW = 8
	}
	prefix := calendarFit(kind+" "+timeText, prefixW)
	return calendarFit(prefix+"  "+ansi.Truncate(summary, titleW, "..."), width-2)
}

func calendarSearchMatchHint(event models.CalendarEvent, query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ""
	}
	fieldContains := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), query) {
				return true
			}
		}
		return false
	}
	if fieldContains(event.Title) {
		return "title"
	}
	if fieldContains(event.Location) {
		return "location"
	}
	if fieldContains(event.Organizer, event.OrganizerEmail) {
		return "organizer"
	}
	for _, attendee := range event.Attendees {
		if fieldContains(attendee.Name, attendee.Email, attendee.RSVP) {
			return "attendee"
		}
	}
	if fieldContains(event.Description) {
		return "notes"
	}
	if fieldContains(event.RecurrenceSummary) || fieldContains(event.Recurrence...) {
		return "recurrence"
	}
	for _, attachment := range event.Attachments {
		if fieldContains(attachment.Title, attachment.MIMEType) {
			return "attachment"
		}
	}
	if fieldContains(calendarSourceLabel(event), string(event.Ref.WithDefaults().AccountID)) {
		return "calendar"
	}
	return ""
}

func calendarDayAgendaLine(event models.CalendarEvent, width int) string {
	timeText := calendarDayTime(event)
	prefixW := 13
	if width < 44 {
		prefixW = 9
	}
	titleW := width - prefixW - 2
	if titleW < 8 {
		titleW = 8
	}
	prefix := calendarFit(timeText, prefixW)
	return calendarFit(prefix+"  "+ansi.Truncate(event.Title+" - "+calendarStatusLabel(event), titleW, "..."), width-2)
}

func calendarWeekEventLine(event models.CalendarEvent, width int) string {
	timeText := calendarDayTime(event)
	prefixW := 13
	if width < 44 {
		prefixW = 9
	}
	titleW := width - prefixW - 2
	if titleW < 8 {
		titleW = 8
	}
	prefix := calendarFit(timeText, prefixW)
	return calendarFit(prefix+"  "+ansi.Truncate(event.Title+" - "+calendarSourceLabel(event), titleW, "..."), width-2)
}

func calendarThreeDayEventLine(event models.CalendarEvent, width int) string {
	timeText := calendarDayTime(event)
	prefixW := 13
	if width < 44 {
		prefixW = 9
	}
	titleW := width - prefixW - 2
	if titleW < 8 {
		titleW = 8
	}
	prefix := calendarFit(timeText, prefixW)
	return calendarFit(prefix+"  "+ansi.Truncate(event.Title+" - "+calendarSourceLabel(event), titleW, "..."), width-2)
}

func calendarCompactEventSummary(event models.CalendarEvent) string {
	return event.Start.Local().Format("Mon 15:04") + " " + event.Title
}

func calendarDetailRow(m *Model, label, value string, width int) string {
	label = calendarFit(label+":", 10)
	valueW := width - ansi.StringWidth(label) - 1
	if valueW < 4 {
		valueW = 4
	}
	return m.theme.Metadata.Label.Style().Render(label) + " " + m.theme.Text.Primary.Style().Render(calendarFit(value, valueW))
}

func calendarOrganizerLabel(event models.CalendarEvent) string {
	name := strings.TrimSpace(event.Organizer)
	email := strings.TrimSpace(event.OrganizerEmail)
	if name != "" && email != "" {
		return name + " <" + email + ">"
	}
	return firstNonEmptyString(name, email)
}

func calendarAttendeeLabel(attendee models.CalendarAttendee) string {
	name := strings.TrimSpace(attendee.Name)
	email := strings.TrimSpace(attendee.Email)
	label := firstNonEmptyString(name, email)
	if name != "" && email != "" {
		label = name + " <" + email + ">"
	}
	if status := strings.TrimSpace(attendee.RSVP); status != "" {
		label += " " + strings.ToLower(status)
	}
	if attendee.Optional {
		label += " optional"
	}
	return label
}

func firstCalendarAttendeeRSVP(event models.CalendarEvent) string {
	if len(event.Attendees) == 0 {
		return ""
	}
	return event.Attendees[0].RSVP
}

func calendarAttachmentLabel(attachment models.CalendarAttachment) string {
	title := strings.TrimSpace(attachment.Title)
	if title == "" {
		title = "Attachment"
	}
	if mimeType := strings.TrimSpace(attachment.MIMEType); mimeType != "" {
		return title + " (" + mimeType + ")"
	}
	return title
}

func calendarRecurrenceLabel(event models.CalendarEvent) string {
	if summary := strings.TrimSpace(event.RecurrenceSummary); summary != "" {
		return summary
	}
	if len(event.Recurrence) > 0 {
		return event.Recurrence[0]
	}
	return ""
}

func calendarAlternateTimeZones(event models.CalendarEvent) []string {
	seen := map[string]bool{
		event.CanonicalTimeZone(): true,
		"":                        true,
	}
	out := make([]string, 0, len(event.AlternateTimeZones))
	for _, timezone := range event.AlternateTimeZones {
		timezone = strings.TrimSpace(timezone)
		if seen[timezone] {
			continue
		}
		seen[timezone] = true
		out = append(out, timezone)
	}
	return out
}

func calendarDayTime(event models.CalendarEvent) string {
	if event.AllDay {
		return "all day"
	}
	if event.Start.IsZero() {
		return "unsched"
	}
	start := event.Start.Local()
	if event.End.IsZero() {
		return start.Format("15:04")
	}
	return start.Format("15:04") + "-" + event.End.Local().Format("15:04")
}

func calendarShortTime(event models.CalendarEvent) string {
	if event.AllDay {
		return event.Start.Local().Format("Mon Jan 2")
	}
	if event.Start.IsZero() {
		return "unscheduled"
	}
	return event.Start.Local().Format("Mon 15:04")
}

func crossSourceShortTime(when time.Time) string {
	if when.IsZero() {
		return "unscheduled"
	}
	return when.Local().Format("Mon 15:04")
}

func calendarTimeRange(event models.CalendarEvent) string {
	if event.Start.IsZero() {
		return "unscheduled"
	}
	if event.AllDay {
		return event.Start.Local().Format("Mon Jan 2") + " (all day)"
	}
	start := event.Start.Local()
	if event.End.IsZero() {
		return start.Format("Mon Jan 2 15:04")
	}
	end := event.End.Local()
	if sameCalendarDate(start, end) {
		return start.Format("Mon Jan 2 15:04") + " - " + end.Format("15:04")
	}
	return start.Format("Mon Jan 2 15:04") + " - " + end.Format("Mon Jan 2 15:04")
}

func calendarTimeRangeInLocation(event models.CalendarEvent, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	if event.Start.IsZero() {
		return "unscheduled"
	}
	start := event.Start.In(loc)
	if event.End.IsZero() {
		return start.Format("Mon Jan 2 15:04 MST")
	}
	end := event.End.In(loc)
	if sameCalendarDate(start, end) {
		return start.Format("Mon Jan 2 15:04") + " - " + end.Format("15:04 MST")
	}
	return start.Format("Mon Jan 2 15:04 MST") + " - " + end.Format("Mon Jan 2 15:04 MST")
}

func calendarTimeRangeInNamedLocation(event models.CalendarEvent, timezone string) string {
	timezone = strings.TrimSpace(timezone)
	loc := event.Start.Location()
	if timezone != "" {
		if loaded, err := time.LoadLocation(timezone); err == nil {
			loc = loaded
		}
	}
	label := timezone
	if label == "" {
		label = "Local"
	}
	return label + "  " + calendarTimeRangeInLocation(event, loc)
}

func sameCalendarDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func calendarDayStartFor(day time.Time) time.Time {
	if day.IsZero() {
		day = time.Now()
	}
	day = day.Local()
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
}

func calendarWeekStartFor(day time.Time) time.Time {
	if day.IsZero() {
		day = time.Now()
	}
	dayStart := calendarDayStartFor(day)
	return dayStart.AddDate(0, 0, -int(dayStart.Weekday()))
}

func calendarWeekRange(start time.Time) string {
	start = calendarWeekStartFor(start)
	end := start.AddDate(0, 0, 6)
	if start.Year() == end.Year() {
		if start.Month() == end.Month() {
			return start.Format("Mon Jan 2") + " - " + end.Format("Mon Jan 2, 2006")
		}
		return start.Format("Mon Jan 2") + " - " + end.Format("Mon Jan 2, 2006")
	}
	return start.Format("Mon Jan 2, 2006") + " - " + end.Format("Mon Jan 2, 2006")
}

func calendarThreeDayRange(start time.Time) string {
	start = calendarDayStartFor(start)
	end := start.AddDate(0, 0, 2)
	if start.Year() == end.Year() {
		return start.Format("Mon Jan 2") + " - " + end.Format("Mon Jan 2, 2006")
	}
	return start.Format("Mon Jan 2, 2006") + " - " + end.Format("Mon Jan 2, 2006")
}

func eventOccursOnCalendarDate(event models.CalendarEvent, day time.Time) bool {
	if event.Start.IsZero() {
		return false
	}
	day = day.Local()
	start := event.Start.Local()
	if sameCalendarDate(start, day) {
		return true
	}
	if event.End.IsZero() {
		return false
	}
	end := event.End.Local()
	if sameCalendarDate(end, day) {
		return true
	}
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)
	return start.Before(dayEnd) && end.After(dayStart)
}

func eventOccursInCalendarWeek(event models.CalendarEvent, start time.Time) bool {
	start = calendarWeekStartFor(start)
	return eventOccursInCalendarRange(event, start, start.AddDate(0, 0, 7))
}

func eventOccursInCalendarRange(event models.CalendarEvent, start, end time.Time) bool {
	if event.Start.IsZero() {
		return false
	}
	start = calendarDayStartFor(start)
	end = calendarDayStartFor(end)
	eventStart := event.Start.Local()
	eventEnd := event.End.Local()
	if event.End.IsZero() {
		eventEnd = eventStart
	}
	return eventStart.Before(end) && eventEnd.After(start)
}

func calendarOpenSlotSummaries(start time.Time, events []indexedCalendarEvent, width int) []string {
	start = calendarDayStartFor(start)
	summaries := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		day := start.AddDate(0, 0, i)
		dayEvents := make([]models.CalendarEvent, 0)
		for _, item := range events {
			if eventOccursOnCalendarDate(item.event, day) {
				dayEvents = append(dayEvents, item.event)
			}
		}
		label := day.Format("Mon")
		if len(dayEvents) == 0 {
			summaries = append(summaries, label+": open day")
			continue
		}
		dayStart := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, day.Location())
		dayEnd := time.Date(day.Year(), day.Month(), day.Day(), 17, 0, 0, 0, day.Location())
		lastEnd := dayStart
		found := ""
		for _, event := range dayEvents {
			eventStart := event.Start.Local()
			eventEnd := event.End.Local()
			if eventStart.After(lastEnd) {
				found = lastEnd.Format("15:04") + "-" + eventStart.Format("15:04")
				break
			}
			if eventEnd.After(lastEnd) {
				lastEnd = eventEnd
			}
		}
		if found == "" && lastEnd.Before(dayEnd) {
			found = lastEnd.Format("15:04") + "-" + dayEnd.Format("15:04")
		}
		if found == "" {
			found = "packed"
		}
		summaries = append(summaries, calendarFit(label+": "+found, width))
	}
	return summaries
}

func calendarConflictSummary(events []indexedCalendarEvent) string {
	for i := 0; i < len(events); i++ {
		a := events[i].event
		if a.Start.IsZero() {
			continue
		}
		aEnd := a.End
		if aEnd.IsZero() {
			aEnd = a.Start
		}
		for j := i + 1; j < len(events); j++ {
			b := events[j].event
			if b.Start.IsZero() {
				continue
			}
			bEnd := b.End
			if bEnd.IsZero() {
				bEnd = b.Start
			}
			if a.Start.Before(bEnd) && b.Start.Before(aEnd) {
				return a.Title + " overlaps " + b.Title
			}
		}
	}
	return "No conflicts"
}

func calendarStatusLabel(event models.CalendarEvent) string {
	status := strings.TrimSpace(event.Status)
	if status == "" {
		return "confirmed"
	}
	return status
}

func calendarSourceLabel(event models.CalendarEvent) string {
	ref := event.Ref.WithDefaults()
	if ref.CalendarID != "" && ref.SourceID != "" {
		return ref.CalendarID + " - " + string(ref.SourceID)
	}
	if ref.CalendarID != "" {
		return ref.CalendarID
	}
	if ref.SourceID != "" {
		return string(ref.SourceID)
	}
	return "calendar"
}

func calendarFit(text string, width int) string {
	if width <= 0 {
		return ""
	}
	out := ansi.Truncate(text, width, "...")
	if missing := width - ansi.StringWidth(out); missing > 0 {
		out += strings.Repeat(" ", missing)
	}
	return out
}
