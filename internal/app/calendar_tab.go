package app

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/backend"
	calendarpkg "github.com/herald-email/herald-mail-app/internal/calendar"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/render"
)

type calendarViewMode string

type calendarFocusPanel int

const (
	calendarViewAgenda      calendarViewMode = "agenda"
	calendarViewDay         calendarViewMode = "day"
	calendarViewWeek        calendarViewMode = "week"
	calendarViewThreeDay    calendarViewMode = "three-day"
	calendarViewSearch      calendarViewMode = "search"
	calendarViewCrossSearch calendarViewMode = "cross-search"
)

const (
	calendarFocusRail calendarFocusPanel = iota
	calendarFocusMain
	calendarFocusDetail
)

const calendarFrameSwitchHint = "(<-/->/h/l to switch)"

type calendarPanelFrameChrome struct {
	LeftTitle string
	RightHint string
}

var (
	calendarHTMLBreakPattern    = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/h[1-6])[^>]*>`)
	calendarHTMLListItemPattern = regexp.MustCompile(`(?i)<\s*li[^>]*>`)
	calendarHTMLTagPattern      = regexp.MustCompile(`(?s)<[^>]*>`)
)

type indexedCalendarEvent struct {
	index int
	event models.CalendarEvent
}

type calendarDerivedCache struct {
	EventsVersion  uint64
	FiltersVersion uint64
	View           calendarViewMode
	AgendaStart    time.Time
	AgendaEnd      time.Time
	AgendaShowPast bool
	AllVisible     []indexedCalendarEvent
	AllValid       bool
	Visible        []indexedCalendarEvent
	VisibleValid   bool
	Day            map[int64][]indexedCalendarEvent
	Week           map[int64][]indexedCalendarEvent
	ThreeDay       map[int64][]indexedCalendarEvent
	Offsets        map[int]int
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
	calendarEditFieldReminders
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
	calendarEditFieldReminders,
	calendarEditFieldDescription,
}

func (m *Model) refreshCalendarAvailability() {
	agenda, ok := m.backend.(backend.CalendarAgendaBackend)
	m.calendarAvailable = ok && agenda.CalendarAgendaAvailable()
	if !m.calendarAvailable {
		m.setCalendarEventsForDisplay(nil)
		m.setCalendarCollections(nil)
		m.calendarDetail = nil
		m.calendarView = calendarViewAgenda
		m.calendarFocus = calendarFocusMain
		m.calendarRailCursor = 0
		m.calendarHiddenCollections = make(map[string]bool)
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
		m.calendarAgendaStart = time.Time{}
		m.calendarAgendaEnd = time.Time{}
		m.calendarAgendaShowPast = false
		m.calendarDetailOpen = false
		m.calendarLoading = false
		m.calendarDetailLoading = false
		m.calendarMeetingPrepOpen = false
		m.calendarMeetingPrepLoading = false
		m.calendarMeetingPrep = nil
		m.calendarTravelBufferOpen = false
		m.calendarTravelBufferLoading = false
		m.calendarTravelBuffer = nil
		m.calendarAISummaryOpen = false
		m.calendarAISummaryLoading = false
		m.calendarAISummary = nil
		m.calendarEdit = calendarEventEditState{}
	}
}

func (m *Model) applyCalendarConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	m.applySelectedCalendarConfig()
}

func (m *Model) setCalendarEventsForDisplay(events []models.CalendarEvent) {
	m.calendarEvents = normalizeCalendarEventsForDisplay(events)
	m.calendarEventsVersion++
	m.calendarDerived = calendarDerivedCache{}
	m.invalidateSettingsBackdrop()
}

func (m *Model) setCalendarCollections(collections []models.CalendarCollection) {
	m.calendarCollections = collections
	m.calendarFiltersVersion++
	m.calendarDerived = calendarDerivedCache{}
	m.invalidateSettingsBackdrop()
}

func (m *Model) invalidateCalendarFilterDerivations() {
	m.calendarFiltersVersion++
	m.calendarDerived = calendarDerivedCache{}
	m.invalidateSettingsBackdrop()
}

func (m *Model) ensureCalendarDerivedCache() *calendarDerivedCache {
	agendaStart, agendaEnd := m.calendarAgendaWindow()
	cache := &m.calendarDerived
	if cache.EventsVersion != m.calendarEventsVersion ||
		cache.FiltersVersion != m.calendarFiltersVersion ||
		cache.View != m.calendarView ||
		!cache.AgendaStart.Equal(agendaStart) ||
		!cache.AgendaEnd.Equal(agendaEnd) ||
		cache.AgendaShowPast != m.calendarAgendaShowPast {
		*cache = calendarDerivedCache{
			EventsVersion:  m.calendarEventsVersion,
			FiltersVersion: m.calendarFiltersVersion,
			View:           m.calendarView,
			AgendaStart:    agendaStart,
			AgendaEnd:      agendaEnd,
			AgendaShowPast: m.calendarAgendaShowPast,
		}
	}
	return cache
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

func (m *Model) calendarMeetingPrepBackend() (backend.CalendarMeetingPrepBackend, bool) {
	prep, ok := m.backend.(backend.CalendarMeetingPrepBackend)
	return prep, ok
}

func (m *Model) calendarTravelBufferBackend() (backend.CalendarTravelBufferBackend, bool) {
	buffer, ok := m.backend.(backend.CalendarTravelBufferBackend)
	return buffer, ok
}

func (m *Model) calendarAISummaryBackend() (backend.CalendarAISummaryBackend, bool) {
	summary, ok := m.backend.(backend.CalendarAISummaryBackend)
	return summary, ok
}

func (m *Model) loadCalendarAgenda() tea.Cmd {
	agenda, ok := m.calendarAgendaBackend()
	if !ok {
		return func() tea.Msg {
			return CalendarAgendaLoadedMsg{Err: fmt.Errorf("calendar agenda unavailable")}
		}
	}
	if cacheBackend, ok := m.backend.(backend.CalendarAgendaCacheBackend); ok {
		return tea.Batch(
			m.loadCachedCalendarAgenda(cacheBackend),
			m.refreshCalendarAgenda(cacheBackend),
		)
	}
	return func() tea.Msg {
		events, err := agenda.ListCalendarAgenda(time.Time{}, time.Time{})
		if err != nil {
			return CalendarAgendaLoadedMsg{Err: err}
		}
		var collections []models.CalendarCollection
		if collectionBackend, ok := m.backend.(backend.CalendarCollectionBackend); ok {
			collections, err = collectionBackend.ListCalendarCollections()
			if err != nil {
				return CalendarAgendaLoadedMsg{Err: err}
			}
		}
		return CalendarAgendaLoadedMsg{Events: events, Collections: collections}
	}
}

func (m *Model) loadCachedCalendarAgenda(cacheBackend backend.CalendarAgendaCacheBackend) tea.Cmd {
	return func() tea.Msg {
		events, err := cacheBackend.ListCachedCalendarAgenda(time.Time{}, time.Time{})
		if err != nil {
			return CalendarAgendaLoadedMsg{Err: err, Cached: true, Refreshing: true}
		}
		collections, err := cacheBackend.ListCachedCalendarCollections()
		if err != nil {
			return CalendarAgendaLoadedMsg{Err: err, Cached: true, Refreshing: true}
		}
		return CalendarAgendaLoadedMsg{Events: events, Collections: collections, Cached: true, Refreshing: true}
	}
}

func (m *Model) refreshCalendarAgenda(cacheBackend backend.CalendarAgendaCacheBackend) tea.Cmd {
	return func() tea.Msg {
		events, collections, err := cacheBackend.RefreshCalendarAgenda(time.Time{}, time.Time{})
		return CalendarAgendaLoadedMsg{Events: events, Collections: collections, Err: err}
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
	if m.calendarView == "" || m.calendarView == calendarViewAgenda {
		for _, item := range m.indexedVisibleCalendarEvents() {
			if item.index == m.calendarCursor {
				event := item.event
				return &event
			}
		}
		return nil
	}
	if m.calendarCursor < 0 || m.calendarCursor >= len(m.calendarEvents) {
		return nil
	}
	event := m.calendarEvents[m.calendarCursor]
	return &event
}

func (m *Model) selectedCalendarDay() time.Time {
	return calendarDayStartFor(m.calendarAnchorDate())
}

func (m *Model) selectedCalendarWeekStart() time.Time {
	if !m.calendarWeekStart.IsZero() {
		return m.calendarWeekStart
	}
	return m.calendarWeekStartFor(m.selectedCalendarDay())
}

func (m *Model) selectedCalendarThreeDayStart() time.Time {
	if !m.calendarThreeDayStart.IsZero() {
		return m.calendarThreeDayStart
	}
	return calendarDayStartFor(m.selectedCalendarDay())
}

func (m *Model) calendarAnchorDate() time.Time {
	if event := m.selectedCalendarEventForAnchor(); event != nil && !event.Start.IsZero() {
		return event.Start
	}
	if day := m.calendarDayAnchorInActiveRange(); !day.IsZero() {
		return day
	}
	if start := m.calendarActiveRangeStartForAnchor(); !start.IsZero() {
		return start
	}
	today := calendarDayStartFor(m.calendarCurrentTime())
	if m.calendarDateInActiveRange(today) {
		return today
	}
	if event := m.nearestCalendarEventForAnchor(today, true); event != nil {
		return event.Start
	}
	if event := m.nearestCalendarEventForAnchor(today, false); event != nil {
		return event.Start
	}
	return today
}

func (m *Model) calendarDayAnchorInActiveRange() time.Time {
	if m.calendarDay.IsZero() {
		return time.Time{}
	}
	day := calendarDayStartFor(m.calendarDay)
	switch m.calendarView {
	case calendarViewDay:
		return day
	case calendarViewWeek:
		start := m.calendarWeekStart
		if start.IsZero() {
			start = m.calendarWeekStartFor(day)
		}
		start = calendarDayStartFor(start)
		if !day.Before(start) && day.Before(start.AddDate(0, 0, 7)) {
			return day
		}
	case calendarViewThreeDay:
		start := m.calendarThreeDayStart
		if start.IsZero() {
			start = day
		}
		start = calendarDayStartFor(start)
		if !day.Before(start) && day.Before(start.AddDate(0, 0, 3)) {
			return day
		}
	case calendarViewAgenda:
		if !m.calendarAgendaStart.IsZero() && !m.calendarAgendaEnd.IsZero() {
			start := calendarDayStartFor(m.calendarAgendaStart)
			end := calendarDayStartFor(m.calendarAgendaEnd)
			if !day.Before(start) && day.Before(end) {
				return day
			}
		}
	}
	return time.Time{}
}

func (m *Model) selectedCalendarEventForAnchor() *models.CalendarEvent {
	if m.calendarView == calendarViewSearch || m.calendarView == calendarViewCrossSearch {
		event := m.selectedCalendarEvent()
		if event == nil || event.Start.IsZero() || m.calendarEventHidden(*event) {
			return nil
		}
		return event
	}
	for _, item := range m.indexedCalendarEventsForActiveAnchorRange() {
		if item.index == m.calendarCursor {
			event := item.event
			return &event
		}
	}
	return nil
}

func (m *Model) indexedCalendarEventsForActiveAnchorRange() []indexedCalendarEvent {
	switch m.calendarView {
	case calendarViewDay:
		if m.calendarDay.IsZero() {
			return m.indexedAllVisibleCalendarEvents()
		}
		return m.indexedCalendarEventsInRange(m.calendarDay, m.calendarDay.AddDate(0, 0, 1))
	case calendarViewWeek:
		if m.calendarWeekStart.IsZero() {
			return m.indexedAllVisibleCalendarEvents()
		}
		return m.indexedCalendarEventsInRange(m.calendarWeekStart, m.calendarWeekStart.AddDate(0, 0, 7))
	case calendarViewThreeDay:
		if m.calendarThreeDayStart.IsZero() {
			return m.indexedAllVisibleCalendarEvents()
		}
		return m.indexedCalendarEventsInRange(m.calendarThreeDayStart, m.calendarThreeDayStart.AddDate(0, 0, 3))
	case calendarViewAgenda:
		return m.indexedVisibleCalendarEvents()
	default:
		return m.indexedAllVisibleCalendarEvents()
	}
}

func (m *Model) indexedCalendarEventsInRange(start, end time.Time) []indexedCalendarEvent {
	out := make([]indexedCalendarEvent, 0)
	for _, item := range m.indexedAllVisibleCalendarEvents() {
		if eventOccursInCalendarRange(item.event, start, end) {
			out = append(out, item)
		}
	}
	return out
}

func (m *Model) calendarActiveRangeStartForAnchor() time.Time {
	switch m.calendarView {
	case calendarViewDay:
		if m.calendarDay.IsZero() {
			return time.Time{}
		}
		return calendarDayStartFor(m.calendarDay)
	case calendarViewWeek:
		if m.calendarWeekStart.IsZero() {
			return time.Time{}
		}
		return calendarDayStartFor(m.calendarWeekStart)
	case calendarViewThreeDay:
		if m.calendarThreeDayStart.IsZero() {
			return time.Time{}
		}
		return calendarDayStartFor(m.calendarThreeDayStart)
	case calendarViewAgenda:
		if !m.calendarAgendaStart.IsZero() && !m.calendarAgendaEnd.IsZero() && m.calendarAgendaEnd.After(m.calendarAgendaStart) {
			return calendarDayStartFor(m.calendarAgendaStart)
		}
	}
	return time.Time{}
}

func (m *Model) calendarDateInActiveRange(day time.Time) bool {
	if day.IsZero() {
		return false
	}
	day = calendarDayStartFor(day)
	start := m.calendarActiveRangeStartForAnchor()
	if start.IsZero() {
		return false
	}
	end := start.AddDate(0, 0, 1)
	switch m.calendarView {
	case calendarViewWeek:
		end = start.AddDate(0, 0, 7)
	case calendarViewThreeDay:
		end = start.AddDate(0, 0, 3)
	case calendarViewAgenda:
		end = calendarDayStartFor(m.calendarAgendaEnd)
	}
	return !day.Before(start) && day.Before(end)
}

func (m *Model) nearestCalendarEventForAnchor(today time.Time, future bool) *models.CalendarEvent {
	var best *models.CalendarEvent
	for _, item := range m.indexedAllVisibleCalendarEvents() {
		event := item.event
		day := calendarDayStartFor(event.Start)
		if future {
			if day.Before(today) {
				continue
			}
			if best == nil || day.Before(calendarDayStartFor(best.Start)) {
				candidate := event
				best = &candidate
			}
			continue
		}
		if !day.Before(today) {
			continue
		}
		if best == nil || day.After(calendarDayStartFor(best.Start)) {
			candidate := event
			best = &candidate
		}
	}
	return best
}

func (m *Model) setCalendarView(view calendarViewMode) {
	if view == "" {
		view = calendarViewAgenda
	}
	anchor := m.calendarAnchorDate()
	m.calendarView = view
	switch view {
	case calendarViewDay:
		m.calendarDay = calendarDayStartFor(anchor)
		m.selectFirstCalendarEventForDay(m.calendarDay)
	case calendarViewWeek:
		m.calendarWeekStart = m.calendarWeekStartFor(anchor)
		m.selectFirstCalendarEventForWeek(m.calendarWeekStart)
	case calendarViewThreeDay:
		m.calendarThreeDayStart = calendarDayStartFor(anchor)
		m.selectFirstCalendarEventForThreeDay(m.calendarThreeDayStart)
	case calendarViewSearch:
		m.calendarDetail = m.selectedCalendarEvent()
	case calendarViewCrossSearch:
		m.selectCrossSourceSearchResult()
	default:
		m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(anchor)
		m.ensureCalendarSelectionVisible()
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
	cache := m.ensureCalendarDerivedCache()
	key := calendarDayStartFor(day).Unix()
	if cache.Day == nil {
		cache.Day = make(map[int64][]indexedCalendarEvent)
	}
	if events, ok := cache.Day[key]; ok {
		return events
	}
	out := make([]indexedCalendarEvent, 0)
	for _, item := range m.indexedVisibleCalendarEvents() {
		if eventOccursOnCalendarDate(item.event, day) {
			out = append(out, item)
		}
	}
	cache.Day[key] = out
	return out
}

func (m *Model) calendarEventsForWeek(start time.Time) []indexedCalendarEvent {
	if start.IsZero() {
		start = m.selectedCalendarWeekStart()
	}
	cache := m.ensureCalendarDerivedCache()
	key := calendarDayStartFor(start).Unix()
	if cache.Week == nil {
		cache.Week = make(map[int64][]indexedCalendarEvent)
	}
	if events, ok := cache.Week[key]; ok {
		return events
	}
	out := make([]indexedCalendarEvent, 0)
	for _, item := range m.indexedVisibleCalendarEvents() {
		if eventOccursInCalendarRange(item.event, start, start.AddDate(0, 0, 7)) {
			out = append(out, item)
		}
	}
	cache.Week[key] = out
	return out
}

func (m *Model) calendarEventsForThreeDay(start time.Time) []indexedCalendarEvent {
	if start.IsZero() {
		start = m.selectedCalendarThreeDayStart()
	}
	cache := m.ensureCalendarDerivedCache()
	key := calendarDayStartFor(start).Unix()
	if cache.ThreeDay == nil {
		cache.ThreeDay = make(map[int64][]indexedCalendarEvent)
	}
	if events, ok := cache.ThreeDay[key]; ok {
		return events
	}
	out := make([]indexedCalendarEvent, 0)
	for _, item := range m.indexedVisibleCalendarEvents() {
		if eventOccursInCalendarRange(item.event, start, start.AddDate(0, 0, 3)) {
			out = append(out, item)
		}
	}
	cache.ThreeDay[key] = out
	return out
}

func (m *Model) openCalendarDetail() tea.Cmd {
	event := m.selectedCalendarEvent()
	if event == nil {
		return nil
	}
	m.calendarEdit = calendarEventEditState{}
	m.calendarMeetingPrepOpen = false
	m.calendarMeetingPrepLoading = false
	m.calendarMeetingPrep = nil
	m.calendarTravelBufferOpen = false
	m.calendarTravelBufferLoading = false
	m.calendarTravelBuffer = nil
	m.calendarAISummaryOpen = false
	m.calendarAISummaryLoading = false
	m.calendarAISummary = nil
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
	m.calendarMeetingPrepOpen = false
	m.calendarMeetingPrepLoading = false
	m.calendarMeetingPrep = nil
	m.calendarTravelBufferOpen = false
	m.calendarTravelBufferLoading = false
	m.calendarTravelBuffer = nil
	m.calendarAISummaryOpen = false
	m.calendarAISummaryLoading = false
	m.calendarAISummary = nil
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
	m.calendarMeetingPrepOpen = false
	m.calendarMeetingPrepLoading = false
	m.calendarMeetingPrep = nil
	m.calendarTravelBufferOpen = false
	m.calendarTravelBufferLoading = false
	m.calendarTravelBuffer = nil
	m.calendarAISummaryOpen = false
	m.calendarAISummaryLoading = false
	m.calendarAISummary = nil
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
	m.calendarStatus = ""
	m.setCalendarView(calendarViewAgenda)
}

func (m *Model) clearCrossSourceSearch() {
	m.crossSourceSearchQuery = ""
	m.crossSourceSearchResults = nil
	m.crossSourceSearchCursor = 0
	m.crossSourceSearchLoading = false
	m.calendarDetail = m.selectedCalendarEvent()
	m.calendarStatus = ""
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
	m.calendarMeetingPrepOpen = false
	m.calendarMeetingPrepLoading = false
	m.calendarMeetingPrep = nil
	m.calendarTravelBufferOpen = false
	m.calendarTravelBufferLoading = false
	m.calendarTravelBuffer = nil
	m.calendarAISummaryOpen = false
	m.calendarAISummaryLoading = false
	m.calendarAISummary = nil
	m.calendarDetailOpen = true
	m.calendarDetailLoading = false
	m.calendarStatus = "Editing cached calendar event"
}

func (m *Model) openCalendarMeetingPrep() tea.Cmd {
	event := m.calendarDetail
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	if event == nil {
		return nil
	}
	prep, ok := m.calendarMeetingPrepBackend()
	if !ok {
		m.calendarStatus = "Meeting prep unavailable for this backend"
		return nil
	}
	ref := event.Ref.WithDefaults()
	selected := *event
	selected.Ref = ref
	m.calendarMeetingPrepOpen = true
	m.calendarMeetingPrepLoading = true
	m.calendarMeetingPrep = nil
	m.calendarTravelBufferOpen = false
	m.calendarTravelBufferLoading = false
	m.calendarTravelBuffer = nil
	m.calendarAISummaryOpen = false
	m.calendarAISummaryLoading = false
	m.calendarAISummary = nil
	m.calendarDetailOpen = false
	m.calendarStatus = "Preparing cached meeting context..."
	return func() tea.Msg {
		result, err := prep.BuildCalendarMeetingPrep(selected)
		return CalendarMeetingPrepMsg{Ref: ref, Prep: result, Err: err}
	}
}

func (m *Model) handleCalendarMeetingPrepKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch shortcutKey(msg) {
	case "esc":
		m.calendarMeetingPrepOpen = false
		m.calendarMeetingPrepLoading = false
		m.calendarDetailOpen = true
		m.calendarStatus = "Returned to event detail"
		return m, nil
	case "r", "ctrl+r":
		return m, m.openCalendarMeetingPrep()
	}
	return m, nil
}

func (m *Model) openCalendarTravelBuffer() tea.Cmd {
	event := m.calendarDetail
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	if event == nil {
		return nil
	}
	buffer, ok := m.calendarTravelBufferBackend()
	if !ok {
		m.calendarStatus = "Travel buffer unavailable for this backend"
		return nil
	}
	ref := event.Ref.WithDefaults()
	selected := *event
	selected.Ref = ref
	m.calendarTravelBufferOpen = true
	m.calendarTravelBufferLoading = true
	m.calendarTravelBuffer = nil
	m.calendarMeetingPrepOpen = false
	m.calendarMeetingPrepLoading = false
	m.calendarMeetingPrep = nil
	m.calendarAISummaryOpen = false
	m.calendarAISummaryLoading = false
	m.calendarAISummary = nil
	m.calendarDetailOpen = false
	m.calendarStatus = "Preparing cached travel context..."
	return func() tea.Msg {
		result, err := buffer.BuildCalendarTravelBuffer(selected)
		return CalendarTravelBufferMsg{Ref: ref, Buffer: result, Err: err}
	}
}

func (m *Model) handleCalendarTravelBufferKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch shortcutKey(msg) {
	case "esc":
		m.calendarTravelBufferOpen = false
		m.calendarTravelBufferLoading = false
		m.calendarDetailOpen = true
		m.calendarStatus = "Returned to event detail"
		return m, nil
	case "r", "ctrl+r":
		return m, m.openCalendarTravelBuffer()
	}
	return m, nil
}

func (m *Model) openCalendarAISummary() tea.Cmd {
	event := m.calendarDetail
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	if event == nil {
		return nil
	}
	summary, ok := m.calendarAISummaryBackend()
	if !ok {
		m.calendarStatus = "AI summary unavailable for this backend"
		return nil
	}
	ref := event.Ref.WithDefaults()
	selected := *event
	selected.Ref = ref
	m.calendarAISummaryOpen = true
	m.calendarAISummaryLoading = true
	m.calendarAISummary = nil
	m.calendarMeetingPrepOpen = false
	m.calendarMeetingPrepLoading = false
	m.calendarMeetingPrep = nil
	m.calendarTravelBufferOpen = false
	m.calendarTravelBufferLoading = false
	m.calendarTravelBuffer = nil
	m.calendarDetailOpen = false
	m.calendarStatus = "Preparing cached AI summary..."
	return func() tea.Msg {
		result, err := summary.BuildCalendarAISummary(selected)
		return CalendarAISummaryMsg{Ref: ref, Summary: result, Err: err}
	}
}

func (m *Model) handleCalendarAISummaryKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch shortcutKey(msg) {
	case "esc":
		m.calendarAISummaryOpen = false
		m.calendarAISummaryLoading = false
		m.calendarDetailOpen = true
		m.calendarStatus = "Returned to event detail"
		return m, nil
	case "r", "ctrl+r":
		return m, m.openCalendarAISummary()
	}
	return m, nil
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
	return m.saveCalendarRSVPStatus(models.NextCalendarRSVP(firstCalendarAttendeeRSVP(*event)))
}

func (m *Model) saveCalendarRSVPStatus(status string) tea.Cmd {
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
	normalized, err := models.NormalizeCalendarRSVP(status)
	if err != nil {
		m.calendarStatus = "RSVP failed: " + err.Error()
		return nil
	}
	ref := event.Ref.WithDefaults()
	m.calendarStatus = "Saving RSVP " + normalized + "..."
	return func() tea.Msg {
		saved, err := mutation.RespondCalendarEvent(ref, normalized)
		return CalendarEventRSVPMsg{Ref: ref, Status: normalized, Event: saved, Err: err}
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
	case calendarEditFieldReminders:
		return m.calendarEdit.Draft.RemindersText
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
	case calendarEditFieldReminders:
		m.calendarEdit.Draft.RemindersText = value
	case calendarEditFieldDescription:
		m.calendarEdit.Draft.Description = value
	}
	m.calendarEdit.Dirty = true
	m.calendarEdit.Error = ""
}

func (m *Model) applySavedCalendarEvent(event models.CalendarEvent) {
	event.Ref = event.Ref.WithDefaults()
	changedEvents := false
	for i := range m.calendarEvents {
		if m.calendarEvents[i].Ref.WithDefaults().LocalID == event.Ref.LocalID {
			m.calendarEvents[i] = event
			changedEvents = true
		}
	}
	if changedEvents {
		m.calendarEventsVersion++
		m.calendarDerived = calendarDerivedCache{}
		m.invalidateSettingsBackdrop()
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
	if key == "S" {
		return m, m.openSettingsPanel()
	}
	if m.calendarMeetingPrepOpen {
		return m.handleCalendarMeetingPrepKey(msg)
	}
	if m.calendarTravelBufferOpen {
		return m.handleCalendarTravelBufferKey(msg)
	}
	if m.calendarAISummaryOpen {
		return m.handleCalendarAISummaryKey(msg)
	}
	switch key {
	case "q":
		m.cleanup()
		return m, tea.Quit
	case "m":
		return m, m.toggleMouseCaptureMode()
	case "tab":
		if !m.calendarDetailOpen {
			m.cycleCalendarFocus(1)
		}
		return m, nil
	case "shift+tab":
		if !m.calendarDetailOpen {
			m.cycleCalendarFocus(-1)
		}
		return m, nil
	case " ", "space":
		if !m.calendarDetailOpen && m.calendarFocus == calendarFocusRail {
			m.toggleFocusedCalendarCollection()
		}
		return m, nil
	case "ctrl+d", "pgdown":
		if !m.calendarDetailOpen {
			m.moveCalendarSelectionPage(1)
		}
		return m, nil
	case "ctrl+u", "pgup":
		if !m.calendarDetailOpen {
			m.moveCalendarSelectionPage(-1)
		}
		return m, nil
	case "j", "down":
		if !m.calendarDetailOpen {
			if m.calendarFocus == calendarFocusRail {
				m.moveCalendarRailSelection(1)
			} else if m.calendarView == calendarViewDay {
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
			if m.calendarFocus == calendarFocusRail {
				m.moveCalendarRailSelection(-1)
			} else if m.calendarView == calendarViewDay {
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
	case "p":
		if m.calendarView == "" || m.calendarView == calendarViewAgenda {
			m.calendarAgendaShowPast = !m.calendarAgendaShowPast
			if m.calendarAgendaShowPast {
				m.calendarStatus = "Showing past agenda events"
			} else {
				m.calendarStatus = "Hiding past agenda events"
			}
			m.ensureCalendarSelectionVisible()
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
		if !m.calendarDetailOpen {
			m.moveCalendarRange(-1)
		}
		return m, nil
	case "l", "right":
		if !m.calendarDetailOpen {
			m.moveCalendarRange(1)
		}
		return m, nil
	case "enter":
		if !m.calendarDetailOpen {
			return m, m.openCalendarDetail()
		}
		return m, nil
	case "e":
		m.openCalendarEdit()
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
	if m.calendarMeetingPrepOpen {
		return m.renderCalendarMeetingPrepFullView()
	}
	if m.calendarTravelBufferOpen {
		return m.renderCalendarTravelBufferFullView()
	}
	if m.calendarAISummaryOpen {
		return m.renderCalendarAISummaryFullView()
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

	return m.renderCalendarSplitView(
		func(width, height int) string { return m.renderCalendarAgendaList(width, height) },
		func(width, height int) string { return m.renderCalendarEventDetail(width, height, false) },
		26,
		"Event Detail",
	)
}

func (m *Model) renderCalendarSearchView() string {
	return m.renderCalendarSplitView(
		func(width, height int) string { return m.renderCalendarSearchResults(width, height) },
		func(width, height int) string {
			return m.renderCalendarEventDetailWithHeader(width, height, false, "Search Detail")
		},
		28,
		"Search Detail",
	)
}

func (m *Model) renderCrossSourceSearchView() string {
	return m.renderCalendarSplitView(
		func(width, height int) string { return m.renderCrossSourceSearchResults(width, height) },
		func(width, height int) string { return m.renderCrossSourceSearchDetail(width, height) },
		30,
		m.crossSourceSearchDetailFrameTitle(),
	)
}

func (m *Model) renderCalendarDayView() string {
	return m.renderCalendarSplitView(
		func(width, height int) string { return m.renderCalendarDayAgenda(width, height) },
		func(width, height int) string { return m.renderCalendarDayDrawer(width, height) },
		26,
		"Day Drawer",
	)
}

func (m *Model) renderCalendarWeekView() string {
	return m.renderCalendarSplitView(
		func(width, height int) string { return m.renderCalendarWeekGrid(width, height) },
		func(width, height int) string { return m.renderCalendarWeekInspector(width, height) },
		28,
		"Week Inspector",
	)
}

func (m *Model) renderCalendarThreeDayView() string {
	return m.renderCalendarSplitView(
		func(width, height int) string { return m.renderCalendarThreeDayLanes(width, height) },
		func(width, height int) string { return m.renderCalendarThreeDayCommandPanel(width, height) },
		30,
		"Command Panel",
	)
}

func (m *Model) renderCalendarSplitView(mainFn, detailFn func(int, int) string, mainMin int, detailTitle string) string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	contentH := calendarPanelOuterHeight(plan)
	if contentH < 4 {
		contentH = 4
	}

	railW := 32
	if contentW < 118 {
		railW = 28
	}
	if contentW < 76 {
		railW = 0
	}
	gaps := panelGapWidth
	if railW > 0 {
		gaps += panelGapWidth
	}
	available := clamp(contentW-gaps, 40)
	remaining := available - railW
	if remaining < mainMin+24 {
		remaining = available
		railW = 0
	}
	mainW, detailW := splitWidth(remaining, 0, mainMin, 24, remaining*56/100)
	mainInnerW, detailInnerW, innerH := clamp(mainW-4, 1), clamp(detailW-4, 1), clamp(contentH-2, 1)
	mainPanel := m.calendarPanel(mainW, contentH, m.calendarFocus == calendarFocusMain).
		Render(fitCalendarPanelContent(mainFn(mainInnerW, innerH), mainInnerW, innerH))
	mainPanel = m.renderCalendarPanelFrameChrome(mainPanel, m.calendarMainPanelFrameChrome())
	detailPanel := m.calendarPanel(detailW, contentH, m.calendarFocus == calendarFocusDetail).
		Render(fitCalendarPanelContent(detailFn(detailInnerW, innerH), detailInnerW, innerH))
	detailPanel = m.renderCalendarPanelFrameChrome(detailPanel, calendarPanelFrameChrome{LeftTitle: detailTitle})
	if railW <= 0 {
		return lipgloss.JoinHorizontal(lipgloss.Top, mainPanel, panelGap, detailPanel)
	}
	railInnerW := clamp(railW-4, 1)
	railPanel := m.calendarPanel(railW, contentH, m.calendarFocus == calendarFocusRail).
		Render(fitCalendarPanelContent(m.renderCalendarLeftPanel(railInnerW, innerH), railInnerW, innerH))
	railPanel = m.renderCalendarPanelFrameChrome(railPanel, m.calendarRailPanelFrameChrome())
	return lipgloss.JoinHorizontal(lipgloss.Top, railPanel, panelGap, mainPanel, panelGap, detailPanel)
}

func calendarPanelOuterHeight(plan LayoutPlan) int {
	// LayoutPlan.ContentHeight is the inner content budget used by table-like
	// views. Calendar renders bordered panels, so the outer panel height needs
	// to include the top and bottom frame rows to keep bottom chrome anchored.
	return plan.ContentHeight + 2
}

func (m *Model) calendarMainPanelFrameChrome() calendarPanelFrameChrome {
	title := m.calendarMainPanelFrameTitle()
	if title == "" {
		return calendarPanelFrameChrome{}
	}
	return calendarPanelFrameChrome{
		LeftTitle: title,
		RightHint: calendarFrameSwitchHint,
	}
}

func (m *Model) calendarMainPanelFrameTitle() string {
	switch m.calendarView {
	case "", calendarViewAgenda:
		visible := m.indexedVisibleCalendarEvents()
		count := fmt.Sprintf("%d", len(visible))
		if m.calendarLoading {
			count = "loading"
		}
		start, end := m.calendarAgendaDisplayRange()
		return fmt.Sprintf("Agenda (%s) for %s", count, calendarFrameDateRange(start, end))
	case calendarViewDay:
		day := m.selectedCalendarDay()
		return fmt.Sprintf("Day Agenda for %s", calendarFrameDateRange(day, day))
	case calendarViewWeek:
		start := m.selectedCalendarWeekStart()
		return fmt.Sprintf("Week Agenda for %s", calendarFrameDateRange(start, start.AddDate(0, 0, 6)))
	case calendarViewThreeDay:
		start := m.selectedCalendarThreeDayStart()
		return fmt.Sprintf("3-Day Window for %s", calendarFrameDateRange(start, start.AddDate(0, 0, 2)))
	default:
		return ""
	}
}

func (m *Model) calendarRailPanelFrameChrome() calendarPanelFrameChrome {
	_, _, rangeLabel := m.calendarActiveRange()
	return calendarPanelFrameChrome{LeftTitle: rangeLabel}
}

func (m *Model) crossSourceSearchDetailFrameTitle() string {
	result := m.selectedCrossSourceSearchResult()
	if result == nil {
		return "Search Detail"
	}
	if result.Event != nil {
		return "Event Detail"
	}
	if result.Email != nil {
		return "Mail Detail"
	}
	return "Search Detail"
}

func (m *Model) renderCalendarPanelFrameChrome(view string, chrome calendarPanelFrameChrome) string {
	leftTitle := strings.TrimSpace(chrome.LeftTitle)
	rightHint := strings.TrimSpace(chrome.RightHint)
	if leftTitle == "" && rightHint == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return view
	}
	lineWidth := ansi.StringWidth(lines[0])
	if lineWidth < 8 {
		return view
	}

	leftStart := 1
	rightEnd := lineWidth - 1
	var rightNotice string
	rightWidth := 0
	if rightHint != "" {
		rightMax := lineWidth - leftStart - 3
		if leftTitle != "" {
			const reservedLeftTitleWidth = 12
			if maxRightWithLeft := lineWidth - leftStart - reservedLeftTitleWidth - 2; maxRightWithLeft < rightMax {
				rightMax = maxRightWithLeft
			}
		}
		if rightMax >= 8 {
			rightText := ansi.Truncate(rightHint, rightMax-2, "...")
			rightNotice = m.theme.Text.Dim.Style().Render(" " + rightText + " ")
			rightWidth = ansi.StringWidth(rightNotice)
			if rightWidth > rightMax {
				rightWidth = 0
				rightNotice = ""
			}
		}
	}

	rightStart := rightEnd - rightWidth
	if rightWidth > 0 && rightStart < leftStart+6 {
		rightWidth = 0
		rightNotice = ""
		rightStart = rightEnd
	}

	var leftNotice string
	leftWidth := 0
	if leftTitle != "" {
		leftMax := rightEnd - leftStart
		if rightWidth > 0 {
			leftMax = rightStart - leftStart - 1
		}
		if leftMax >= 5 {
			leftText := ansi.Truncate(leftTitle, leftMax-2, "...")
			leftNotice = m.theme.Text.Primary.Style().Bold(true).Render(" " + leftText + " ")
			leftWidth = ansi.StringWidth(leftNotice)
		}
	}

	line := lines[0]
	if leftWidth > 0 {
		line = overlayCalendarFrameText(line, lineWidth, leftStart, leftNotice, leftWidth)
	}
	if rightWidth > 0 {
		line = overlayCalendarFrameText(line, lineWidth, rightStart, rightNotice, rightWidth)
	}
	lines[0] = ansi.Cut(line, 0, lineWidth)
	return strings.Join(lines, "\n")
}

func overlayCalendarFrameText(line string, lineWidth, start int, text string, textWidth int) string {
	if start < 0 || textWidth <= 0 || start >= lineWidth {
		return line
	}
	if start+textWidth > lineWidth {
		textWidth = lineWidth - start
		text = ansi.Cut(text, 0, textWidth)
	}
	left := padANSIToWidth(ansi.Cut(line, 0, start), start)
	right := ansi.Cut(line, start+textWidth, lineWidth)
	return ansi.Cut(left+text+right, 0, lineWidth)
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
	contentH := calendarPanelOuterHeight(plan)
	if contentH < 4 {
		contentH = 4
	}
	panelW, innerW, innerH := clamp(contentW, 40), clamp(contentW-4, 20), clamp(contentH-2, 1)
	panel := m.calendarPanel(panelW, contentH, true).
		Render(fitCalendarPanelContent(m.renderCalendarEventDetail(innerW, innerH, true), innerW, innerH))
	return m.renderCalendarPanelFrameChrome(panel, calendarPanelFrameChrome{LeftTitle: "Event Detail"})
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
	contentH := calendarPanelOuterHeight(plan)
	if contentH < 4 {
		contentH = 4
	}
	panelW, innerW, innerH := clamp(contentW, 40), clamp(contentW-4, 20), clamp(contentH-2, 1)
	return m.calendarPanel(panelW, contentH, true).Render(fitCalendarPanelContent(m.renderCalendarEventEdit(innerW, innerH), innerW, innerH))
}

func (m *Model) renderCalendarMeetingPrepFullView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	contentH := calendarPanelOuterHeight(plan)
	if contentH < 4 {
		contentH = 4
	}
	panelW, innerW, innerH := clamp(contentW, 40), clamp(contentW-4, 20), clamp(contentH-2, 1)
	return m.calendarPanel(panelW, contentH, true).Render(fitCalendarPanelContent(m.renderCalendarMeetingPrep(innerW, innerH), innerW, innerH))
}

func (m *Model) renderCalendarTravelBufferFullView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	contentH := calendarPanelOuterHeight(plan)
	if contentH < 4 {
		contentH = 4
	}
	panelW, innerW, innerH := clamp(contentW, 40), clamp(contentW-4, 20), clamp(contentH-2, 1)
	return m.calendarPanel(panelW, contentH, true).Render(fitCalendarPanelContent(m.renderCalendarTravelBuffer(innerW, innerH), innerW, innerH))
}

func (m *Model) renderCalendarAISummaryFullView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
	}
	contentH := calendarPanelOuterHeight(plan)
	if contentH < 4 {
		contentH = 4
	}
	panelW, innerW, innerH := clamp(contentW, 40), clamp(contentW-4, 20), clamp(contentH-2, 1)
	return m.calendarPanel(panelW, contentH, true).Render(fitCalendarPanelContent(m.renderCalendarAISummary(innerW, innerH), innerW, innerH))
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

func (m *Model) renderCalendarRail(width, height int) string {
	return m.renderCalendarLeftPanel(width, height)
}

func fitCalendarPanelContent(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		line = ansi.Cut(line, 0, width)
		if missing := width - ansi.StringWidth(line); missing > 0 {
			line += strings.Repeat(" ", missing)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderCalendarLeftPanel(width, height int) string {
	if width < 10 {
		width = 10
	}
	collections := m.calendarCollections
	if len(collections) == 0 {
		collections = m.mergeCalendarCollections(nil)
	}
	var lines []string
	start, end, _ := m.calendarActiveRange()
	lines = append(lines, m.renderCalendarMiniMonth(width, start, end)...)
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarRule(width)))
	lines = append(lines, m.theme.Severity.Info.Style().Render(calendarFit("Calendars", width)))
	if len(collections) == 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No calendars", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}
	lastGroup := ""
	for i, collection := range collections {
		group := calendarCollectionGroupLabel(collection)
		if group != lastGroup {
			if len(lines) > 1 {
				lines = append(lines, "")
			}
			lines = append(lines, m.theme.Text.Dim.Style().Bold(true).Render(calendarFit(group, width)))
			lastGroup = group
		}
		ref := collection.Ref
		hidden := m.calendarHiddenCollections[calendarCollectionRefKey(ref)]
		box := "[x]"
		if hidden {
			box = "[ ]"
		}
		name := calendarCollectionDisplayName(collection)
		pending := m.calendarCollectionPendingCount(ref)
		if pending > 0 {
			name += fmt.Sprintf("  %d", pending)
		}
		color := calendarCollectionColor(collection, i)
		swatch := dynamicCalendarBlockStyle("#181819", color).Bold(true).Render(box)
		plainText := box + " " + name
		line := swatch + " " + dynamicForegroundStyle(color).Render(calendarFit(name, width-6))
		if m.calendarFocus == calendarFocusRail && i == m.calendarRailCursor {
			line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+plainText, width))
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarRule(width)))
	lines = append(lines, m.theme.Severity.Info.Style().Render(calendarFit("Filter", width)))
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit("All Events          v", width)))
	lines = append(lines, "")
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("space: show/hide", width)))
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarAgendaList(width, height int) string {
	if width < 10 {
		width = 10
	}
	var lines []string
	visibleEvents := m.indexedVisibleCalendarEvents()
	if status := m.visibleCalendarStatus(); status != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(status, width)))
	}
	hiddenPast := m.calendarAgendaHiddenPastCount()
	if hiddenPast > 0 {
		for _, notice := range m.calendarAgendaPastNoticeLines(hiddenPast) {
			lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(notice, width)))
		}
	}
	if len(visibleEvents) == 0 {
		lines = append(lines, "")
		empty := "No cached calendar events"
		if hiddenPast > 0 && !m.calendarAgendaShowPast {
			empty = "No upcoming calendar events"
		}
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(empty, width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	selectedOffset := m.calendarVisibleOffset()
	start := 0
	if selectedOffset >= maxRows {
		start = selectedOffset - maxRows + 1
	}
	end := start + maxRows
	if end > len(visibleEvents) {
		end = len(visibleEvents)
	}
	var lastDay time.Time
	for _, item := range visibleEvents[start:end] {
		day := calendarDayStartFor(item.event.Start)
		if lastDay.IsZero() || !sameCalendarDate(day, lastDay) {
			lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit(item.event.Start.Local().Format("Mon Jan 2"), width)))
			lastDay = day
		}
		line := m.calendarAgendaLine(item.event, width)
		if item.index == m.calendarCursor {
			line = m.theme.Focus.SelectionActive.Style().Render(calendarFit("> "+line, width))
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) visibleCalendarStatus() string {
	status := strings.TrimSpace(m.calendarStatus)
	if status == "" {
		return ""
	}
	if strings.HasPrefix(status, "Loaded ") && strings.Contains(status, "calendar event") {
		return ""
	}
	return status
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
	if status := m.visibleCalendarStatus(); status != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(status, width)))
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
	if status := m.visibleCalendarStatus(); status != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(status, width)))
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
	lines = append(lines, m.renderCalendarEditField("Reminders", calendarEditFieldReminders, state.Draft.RemindersText, width))
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

func (m *Model) renderCalendarMeetingPrep(width, height int) string {
	if width < 12 {
		width = 12
	}
	event := m.calendarDetail
	if m.calendarMeetingPrep != nil {
		event = &m.calendarMeetingPrep.Event
	}
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("Meeting Prep", width)))
	subtitle := "read-only cached context"
	if m.calendarMeetingPrepLoading {
		subtitle = "loading cached mail and event context..."
	}
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(subtitle, width)))
	lines = append(lines, "")
	if event == nil {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No event selected", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Selected Event", width)))
	lines = append(lines, m.theme.Metadata.Subject.Style().Render(calendarFit(event.Title, width)))
	lines = append(lines, calendarDetailRow(m, "Time", calendarTimeRange(*event), width))
	lines = append(lines, calendarDetailRow(m, "Calendar", calendarSourceLabel(*event), width))

	prep := m.calendarMeetingPrep
	if prep == nil {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached meeting context loaded yet", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Related Mail", width)))
	if len(prep.RelatedMail) == 0 {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached related mail found", width)))
	} else {
		for _, email := range prep.RelatedMail {
			if email == nil {
				continue
			}
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarMeetingPrepMailLine(m, email, width), width)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Nearby Events", width)))
	if len(prep.RelatedEvents) == 0 {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached nearby events found", width)))
	} else {
		for _, related := range prep.RelatedEvents {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarMeetingPrepEventLine(related, width), width)))
		}
	}

	if len(prep.QueryTerms) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Query Terms", width)))
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(strings.Join(prep.QueryTerms, ", "), width)))
	}
	lines = append(lines, "")
	lines = append(lines, calendarDetailRow(m, "Mode", "read-only cached context", width))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("esc: event detail  r: refresh prep", width)))
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarTravelBuffer(width, height int) string {
	if width < 12 {
		width = 12
	}
	event := m.calendarDetail
	if m.calendarTravelBuffer != nil {
		event = &m.calendarTravelBuffer.Event
	}
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("Travel Buffer", width)))
	subtitle := "read-only cached travel context"
	if m.calendarTravelBufferLoading {
		subtitle = "loading cached travel context..."
	}
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(subtitle, width)))
	lines = append(lines, "")
	if event == nil {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No event selected", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Selected Event", width)))
	lines = append(lines, m.theme.Metadata.Subject.Style().Render(calendarFit(event.Title, width)))
	lines = append(lines, calendarDetailRow(m, "Time", calendarTimeRange(*event), width))
	lines = append(lines, calendarDetailRow(m, "Location", event.Location, width))
	lines = append(lines, calendarDetailRow(m, "Calendar", calendarSourceLabel(*event), width))

	buffer := m.calendarTravelBuffer
	if buffer == nil {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached travel context loaded yet", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Buffer Suggestions", width)))
	if len(buffer.Recommendations) == 0 {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached buffer suggestions found", width)))
	} else {
		for _, rec := range buffer.Recommendations {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarTravelBufferRecommendationLine(rec, width), width)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Travel Mail", width)))
	if len(buffer.RelatedMail) == 0 {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached travel mail found", width)))
	} else {
		for _, email := range buffer.RelatedMail {
			if email == nil {
				continue
			}
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarMeetingPrepMailLine(m, email, width), width)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Nearby Gaps", width)))
	if len(buffer.NearbyEvents) == 0 {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached nearby events found", width)))
	} else {
		for _, related := range buffer.NearbyEvents {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarMeetingPrepEventLine(related, width), width)))
		}
	}

	if len(buffer.QueryTerms) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Query Terms", width)))
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(strings.Join(buffer.QueryTerms, ", "), width)))
	}
	lines = append(lines, "")
	lines = append(lines, calendarDetailRow(m, "Mode", "read-only cached travel context", width))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("esc: event detail  r: refresh buffer", width)))
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) renderCalendarAISummary(width, height int) string {
	if width < 12 {
		width = 12
	}
	event := m.calendarDetail
	if m.calendarAISummary != nil {
		event = &m.calendarAISummary.Event
	}
	if event == nil {
		event = m.selectedCalendarEvent()
	}
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit("AI Summary", width)))
	subtitle := "read-only cached AI summary"
	if m.calendarAISummaryLoading {
		subtitle = "loading cached AI summary..."
	}
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(subtitle, width)))
	lines = append(lines, "")
	if event == nil {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No event selected", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Selected Event", width)))
	lines = append(lines, m.theme.Metadata.Subject.Style().Render(calendarFit(event.Title, width)))
	lines = append(lines, calendarDetailRow(m, "Time", calendarTimeRange(*event), width))
	lines = append(lines, calendarDetailRow(m, "Calendar", calendarSourceLabel(*event), width))

	summary := m.calendarAISummary
	if summary == nil {
		lines = append(lines, "")
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached AI summary loaded yet", width)))
		return fitPanelContentHeight(strings.Join(lines, "\n"), height)
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Summary Bullets", width)))
	if len(summary.Bullets) == 0 {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No summary bullets generated", width)))
	} else {
		for _, bullet := range summary.Bullets {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit("- "+bullet, width)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Action Items", width)))
	if len(summary.ActionItems) == 0 {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("No cached action items generated", width)))
	} else {
		for _, item := range summary.ActionItems {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit("- "+item, width)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Related Sources", width)))
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarAISummarySourceLine(summary), width)))
	if strings.TrimSpace(summary.AINote) != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(summary.AINote, width)))
	}

	if len(summary.QueryTerms) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Query Terms", width)))
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(strings.Join(summary.QueryTerms, ", "), width)))
	}
	lines = append(lines, "")
	lines = append(lines, calendarDetailRow(m, "Mode", "read-only cached AI summary", width))
	lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("esc: event detail  r: refresh summary", width)))
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
	if errors.Is(err, models.ErrCalendarAuthorizationRequired) {
		return prefix + ": calendar authorization expired; reconnect Google Calendar in Settings"
	}
	if errors.Is(err, models.ErrCalendarWritePermission) {
		return prefix + ": calendar write permission missing; reconnect Google Calendar in Settings"
	}
	return prefix + ": " + err.Error()
}

func (m *Model) renderCalendarDayAgenda(width, height int) string {
	day := m.selectedCalendarDay()
	return m.renderCalendarTimeGrid(width, height, calendarTimeGridConfig{
		Title:        "Day Agenda",
		RangeLabel:   day.Local().Format("Mon Jan 2, 2006"),
		Start:        day,
		DayCount:     1,
		Events:       m.calendarEventsForDay(day),
		EmptyMessage: "No events on this day",
		EmptyHint:    "h/l: previous/next day",
	})
}

func (m *Model) renderCalendarDayDrawer(width, height int) string {
	return m.renderCalendarEventDetailWithHeader(width, height, false, "Day Drawer")
}

func (m *Model) renderCalendarWeekGrid(width, height int) string {
	start := m.selectedCalendarWeekStart()
	return m.renderCalendarTimeGrid(width, height, calendarTimeGridConfig{
		Title:          "Week Time-Grid",
		RangeLabel:     m.calendarWeekRange(start),
		DaySummary:     m.calendarWeekDaySummary(start),
		Start:          start,
		DayCount:       7,
		Events:         m.calendarEventsForWeek(start),
		CompactSummary: m.calendarWeekEventSummary(start, width),
		ShowNowMarker:  true,
	})
}

func (m *Model) calendarWeekGridRows(start time.Time, width, maxRows int) []string {
	return m.calendarTimeGridRows(calendarTimeGridConfig{Start: start, DayCount: 7, Events: m.calendarEventsForWeek(start), ShowNowMarker: true}, width, maxRows)
}

type calendarTimeGridConfig struct {
	Title          string
	RangeLabel     string
	DaySummary     string
	Start          time.Time
	DayCount       int
	Events         []indexedCalendarEvent
	CompactSummary string
	EmptyMessage   string
	EmptyHint      string
	ShowNowMarker  bool
}

func (m *Model) renderCalendarTimeGrid(width, height int, cfg calendarTimeGridConfig) string {
	if width < 10 {
		width = 10
	}
	if cfg.DayCount < 1 {
		cfg.DayCount = 1
	}
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit(cfg.Title, width)))
	if cfg.RangeLabel != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(cfg.RangeLabel, width)))
	}
	if cfg.DaySummary != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(cfg.DaySummary, width)))
	}
	if summary := calendarAllDayAndSpanningSummary(cfg.Events, width); summary != "" {
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit(summary, width)))
	}
	if cfg.CompactSummary != "" {
		lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(cfg.CompactSummary, width)))
	}
	if status := m.visibleCalendarStatus(); status != "" {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit(status, width)))
	}
	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	rows := m.calendarTimeGridRows(cfg, width, maxRows)
	if len(cfg.Events) == 0 {
		if cfg.EmptyMessage != "" {
			rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit(cfg.EmptyMessage, width)))
		}
		if cfg.EmptyHint != "" {
			rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit(cfg.EmptyHint, width)))
		}
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	lines = append(lines, rows...)
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) calendarTimeGridRows(cfg calendarTimeGridConfig, width, maxRows int) []string {
	if cfg.DayCount < 1 {
		cfg.DayCount = 1
	}
	timeW := 7
	dayW := (width - timeW - (cfg.DayCount - 1)) / cfg.DayCount
	if dayW < 4 {
		dayW = 4
	}
	rows := make([]string, 0, 28)
	var header strings.Builder
	header.WriteString(calendarFit("", timeW))
	for i := 0; i < cfg.DayCount; i++ {
		day := cfg.Start.AddDate(0, 0, i)
		label := calendarTimeGridDayLabel(day, dayW, i, cfg.DayCount)
		if sameCalendarDate(day, m.selectedCalendarDay()) {
			header.WriteString(m.theme.Focus.VisualSelection.Style().Render(calendarFit(label, dayW)))
		} else {
			header.WriteString(m.theme.Metadata.Label.Style().Render(calendarFit(label, dayW)))
		}
		if i < cfg.DayCount-1 {
			header.WriteString(m.theme.Text.Dim.Style().Render("│"))
		}
	}
	rows = append(rows, calendarFit(header.String(), width))
	rows = append(rows, m.theme.Text.Dim.Style().Render(calendarFit(strings.Repeat("─", width), width)))
	startHour, endHour := calendarVisibleHourRange(cfg.Events)
	markerTime, showMarker := m.calendarTimeGridCurrentTimeMarker(cfg.Start, cfg.DayCount, cfg.ShowNowMarker)
	if showMarker {
		if hour := markerTime.Hour(); hour < startHour {
			startHour = hour
		} else if hour > endHour {
			endHour = hour
		}
	}
	showHalfHours := calendarWeekGridShouldShowHalfHours(maxRows, startHour, endHour)
	for hour := startHour; hour <= endHour; hour++ {
		rows = append(rows, m.calendarTimeGridSlotRow(cfg.Start, cfg.DayCount, hour, 0, timeW, dayW, width))
		if showMarker && markerTime.Hour() == hour {
			rows = append(rows, m.calendarWeekCurrentTimeMarkerRow(markerTime, width))
			if showHalfHours && hour < endHour {
				rows = append(rows, m.calendarTimeGridSlotRow(cfg.Start, cfg.DayCount, hour, 30, timeW, dayW, width))
			}
			continue
		}
		if hour >= endHour {
			continue
		}
		if showHalfHours {
			rows = append(rows, m.calendarTimeGridSlotRow(cfg.Start, cfg.DayCount, hour, 30, timeW, dayW, width))
		} else {
			rows = append(rows, m.calendarTimeGridGuideRow(cfg.Start, cfg.DayCount, hour, timeW, dayW, width))
		}
	}
	return rows
}

func calendarTimeGridDayLabel(day time.Time, width, offset, dayCount int) string {
	if dayCount == 3 {
		label := day.Local().Format("Mon Jan 2")
		if offset == 0 {
			label += " today"
		} else if offset == 1 {
			label += " tomorrow"
		}
		return label
	}
	return calendarWeekGridDayLabel(day, width)
}

func (m *Model) calendarTimeGridCurrentTimeMarker(start time.Time, dayCount int, enabled bool) (time.Time, bool) {
	if !enabled {
		return time.Time{}, false
	}
	now := m.calendarNow
	if now.IsZero() {
		now = time.Now()
	}
	now = now.Local()
	start = calendarDayStartFor(start)
	today := calendarDayStartFor(now)
	end := start.AddDate(0, 0, dayCount)
	if today.Before(start) || !today.Before(end) {
		return time.Time{}, false
	}
	return now, true
}

func (m *Model) calendarWeekCurrentTimeMarker(start time.Time) (time.Time, bool) {
	return m.calendarTimeGridCurrentTimeMarker(m.calendarWeekStartFor(start), 7, true)
}

func (m *Model) calendarWeekCurrentTimeMarkerRow(now time.Time, width int) string {
	label := " " + now.Local().Format("15:04") + " ◀"
	line := label + strings.Repeat("─", clamp(width-ansi.StringWidth(label), 1))
	return m.theme.Severity.Error.Style().Render(calendarFit(line, width))
}

func (m *Model) calendarClockTick() tea.Cmd {
	now := time.Now().Local()
	next := now.Truncate(time.Minute).Add(time.Minute)
	delay := time.Until(next)
	if delay <= 0 {
		delay = time.Minute
	}
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return CalendarClockTickMsg{Now: t.Local()}
	})
}

func calendarWeekGridShouldShowHalfHours(maxRows, startHour, endHour int) bool {
	if maxRows <= 0 || endHour < startHour {
		return false
	}
	const headerRows = 2
	slotRows := ((endHour - startHour) * 2) + 1
	return maxRows >= headerRows+slotRows
}

func (m *Model) calendarWeekEventSummary(start time.Time, width int) string {
	if width >= 86 {
		return ""
	}
	events := m.calendarEventsForWeek(start)
	if len(events) == 0 {
		return ""
	}
	titles := make([]string, 0, len(events))
	seen := make(map[string]bool, len(events))
	for _, item := range events {
		title := strings.TrimSpace(item.event.Title)
		if title == "" || seen[title] {
			continue
		}
		seen[title] = true
		titles = append(titles, title)
		if len(titles) >= 4 {
			break
		}
	}
	if len(titles) == 0 {
		return ""
	}
	return "Events: " + strings.Join(titles, " | ")
}

func calendarAllDayAndSpanningSummary(events []indexedCalendarEvent, width int) string {
	if len(events) == 0 {
		return ""
	}
	allDayTitles := make([]string, 0, 2)
	spanningTitles := make([]string, 0, 2)
	seen := make(map[string]bool, len(events))
	for _, item := range events {
		event := item.event
		if !event.AllDay && !calendarEventSpansMultipleDates(event) {
			continue
		}
		title := strings.TrimSpace(event.Title)
		if title == "" {
			title = "Untitled"
		}
		key := title + "|" + event.Start.Format(time.RFC3339)
		if seen[key] {
			continue
		}
		seen[key] = true
		if event.AllDay {
			if len(allDayTitles) < 2 {
				allDayTitles = append(allDayTitles, title)
			}
			continue
		}
		if len(spanningTitles) < 2 {
			spanningTitles = append(spanningTitles, title)
		}
	}
	parts := make([]string, 0, 2)
	if len(allDayTitles) > 0 {
		parts = append(parts, "All-day: "+strings.Join(allDayTitles, " | "))
	}
	if len(spanningTitles) > 0 {
		parts = append(parts, "Multi-day: "+strings.Join(spanningTitles, " | "))
	}
	if len(parts) == 0 {
		return ""
	}
	return calendarFit(strings.Join(parts, "  "), width)
}

func (m *Model) calendarWeekGridSlotRow(start time.Time, hour, minute, timeW, dayW, width int) string {
	return m.calendarTimeGridSlotRow(start, 7, hour, minute, timeW, dayW, width)
}

func (m *Model) calendarTimeGridSlotRow(start time.Time, dayCount, hour, minute, timeW, dayW, width int) string {
	var row strings.Builder
	row.WriteString(m.theme.Text.Primary.Style().Render(calendarFit(fmt.Sprintf("%02d:%02d", hour, minute), timeW)))
	for dayIdx := 0; dayIdx < dayCount; dayIdx++ {
		day := start.AddDate(0, 0, dayIdx)
		event, continuation := m.calendarEventInSlot(day, hour, minute)
		cell := m.calendarGridCell(event, dayW)
		if continuation {
			cell = m.calendarGridContinuationCell(event, dayW)
		} else if event == nil && minute > 0 {
			cell = m.theme.Text.Dim.Style().Render(calendarFit(strings.Repeat("·", dayW), dayW))
		}
		row.WriteString(cell)
		if dayIdx < dayCount-1 {
			row.WriteString(m.theme.Text.Dim.Style().Render("│"))
		}
	}
	return calendarFit(row.String(), width)
}

func (m *Model) calendarWeekGridGuideRow(start time.Time, hour, timeW, dayW, width int) string {
	return m.calendarTimeGridGuideRow(start, 7, hour, timeW, dayW, width)
}

func (m *Model) calendarTimeGridGuideRow(start time.Time, dayCount, hour, timeW, dayW, width int) string {
	var row strings.Builder
	row.WriteString(m.theme.Text.Dim.Style().Render(calendarFit("", timeW)))
	for dayIdx := 0; dayIdx < dayCount; dayIdx++ {
		day := start.AddDate(0, 0, dayIdx)
		event, continuation := m.calendarEventInSlot(day, hour, 30)
		if event != nil {
			if continuation {
				row.WriteString(m.calendarGridContinuationCell(event, dayW))
			} else {
				row.WriteString(m.calendarGridCell(event, dayW))
			}
		} else {
			row.WriteString(m.theme.Text.Dim.Style().Render(calendarFit(strings.Repeat("·", dayW), dayW)))
		}
		if dayIdx < dayCount-1 {
			row.WriteString(m.theme.Text.Dim.Style().Render("│"))
		}
	}
	return calendarFit(row.String(), width)
}

func (m *Model) calendarEventInSlot(day time.Time, hour, minute int) (*models.CalendarEvent, bool) {
	slotStart := calendarDayStartFor(day).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	slotEnd := slotStart.Add(30 * time.Minute)
	var continuation *models.CalendarEvent
	for _, item := range m.calendarEventsForDay(day) {
		event := item.event
		if event.AllDay || event.Start.IsZero() || calendarEventSpansMultipleDates(event) {
			continue
		}
		start := event.Start.Local()
		end := event.End.Local()
		if event.End.IsZero() {
			end = start
		}
		if !start.Before(slotStart) && start.Before(slotEnd) {
			candidate := event
			return &candidate, false
		}
		if start.Before(slotEnd) && end.After(slotStart) && continuation == nil {
			candidate := event
			continuation = &candidate
		}
	}
	if continuation != nil {
		return continuation, true
	}
	return nil, false
}

func (m *Model) calendarEventInHour(day time.Time, hour int) (*models.CalendarEvent, bool) {
	var best *models.CalendarEvent
	for _, item := range m.calendarEventsForDay(day) {
		event := item.event
		if event.AllDay || event.Start.IsZero() || calendarEventSpansMultipleDates(event) {
			continue
		}
		start := event.Start.Local()
		if start.Hour() == hour {
			candidate := event
			best = &candidate
			break
		}
		if start.Hour() < hour && !event.End.IsZero() && event.End.Local().Hour() > hour {
			candidate := event
			best = &candidate
			return best, true
		}
	}
	return best, false
}

func (m *Model) renderCalendarWeekInspector(width, height int) string {
	return m.renderCalendarEventDetailWithHeader(width, height, false, "Week Inspector")
}

func (m *Model) renderCalendarThreeDayLanes(width, height int) string {
	start := m.selectedCalendarThreeDayStart()
	return m.renderCalendarTimeGrid(width, height, calendarTimeGridConfig{
		Title:        "3-Day Command",
		RangeLabel:   calendarThreeDayRange(start),
		Start:        start,
		DayCount:     3,
		Events:       m.calendarEventsForThreeDay(start),
		EmptyMessage: "No events in this 3-day window",
		EmptyHint:    "h/l: slide 3-day window",
	})
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

	if event != nil && strings.TrimSpace(calendarRenderedNotes(event.Description)) != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Notes", width)))
		for _, line := range wrapText(calendarRenderedNotes(event.Description), width) {
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
	if m.calendarDetailLoading {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Loading latest cached detail...", width)))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, m.calendarDetailTitleLine(*event, width))
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
		lines = append(lines, calendarDetailRow(m, "Attendees", fmt.Sprintf("%d", len(event.Attendees)), width))
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
	if len(event.Reminders) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Reminders", width)))
		for _, reminder := range event.Reminders {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(calendarReminderLabel(reminder), width)))
		}
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
	lines = append(lines, calendarDetailRow(m, "Mode", "read-only list / provider-backed edit", width))
	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Actions", width)))
	lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit("e: edit event", width)))
	if strings.TrimSpace(calendarRenderedNotes(event.Description)) != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Notes", width)))
		for _, line := range calendarDetailNoteLines(event.Description, width) {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(line, width)))
		}
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
}

func (m *Model) moveCalendarDaySelection(delta int) {
	events := m.indexedVisibleCalendarEvents()
	if len(events) == 0 {
		return
	}
	selectedOffset := m.calendarVisibleOffset()
	selectedOffset += delta
	if selectedOffset < 0 {
		selectedOffset = 0
	}
	if selectedOffset >= len(events) {
		selectedOffset = len(events) - 1
	}
	m.calendarCursor = events[selectedOffset].index
	m.calendarDetail = &events[selectedOffset].event
	if !events[selectedOffset].event.Start.IsZero() {
		m.calendarDay = calendarDayStartFor(events[selectedOffset].event.Start)
	}
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

func (m *Model) moveCalendarSelectionPage(direction int) {
	step := 6
	if m.windowHeight > 0 {
		step = m.windowHeight / 3
	}
	if step < 3 {
		step = 3
	}
	if direction < 0 {
		step = -step
	}
	switch m.calendarView {
	case calendarViewDay:
		m.moveCalendarDaySelection(step)
	case calendarViewWeek:
		m.moveCalendarWeekSelection(step)
	case calendarViewThreeDay:
		m.moveCalendarThreeDaySelection(step)
	default:
		events := m.indexedVisibleCalendarEvents()
		if len(events) == 0 {
			return
		}
		offset := m.calendarVisibleOffset() + step
		if offset < 0 {
			offset = 0
		}
		if offset >= len(events) {
			offset = len(events) - 1
		}
		m.calendarCursor = events[offset].index
		m.calendarDetail = &events[offset].event
	}
}

func (m *Model) moveCalendarRange(delta int) {
	if delta == 0 {
		return
	}
	switch m.calendarView {
	case calendarViewDay:
		m.calendarDay = m.selectedCalendarDay().AddDate(0, 0, delta)
		m.selectFirstCalendarEventForDay(m.calendarDay)
	case calendarViewWeek:
		m.calendarWeekStart = m.selectedCalendarWeekStart().AddDate(0, 0, 7*delta)
		m.selectFirstCalendarEventForWeek(m.calendarWeekStart)
	case calendarViewThreeDay:
		m.calendarThreeDayStart = m.selectedCalendarThreeDayStart().AddDate(0, 0, delta)
		m.selectFirstCalendarEventForThreeDay(m.calendarThreeDayStart)
	default:
		start, _ := m.calendarAgendaWindow()
		m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(start.AddDate(0, delta, 0))
		m.ensureCalendarSelectionVisible()
	}
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

func (m *Model) calendarViewTitle() string {
	switch m.calendarView {
	case calendarViewWeek:
		return "Week"
	case calendarViewDay:
		return "Day"
	case calendarViewThreeDay:
		return "3-Day"
	case calendarViewSearch:
		return "Search"
	case calendarViewCrossSearch:
		return "All Search"
	default:
		return "Agenda"
	}
}

func (m *Model) calendarActiveRange() (time.Time, time.Time, string) {
	switch m.calendarView {
	case calendarViewWeek:
		start := m.selectedCalendarWeekStart()
		end := start.AddDate(0, 0, 6)
		return start, end, calendarCompactDateRange(start, end)
	case calendarViewDay:
		day := m.selectedCalendarDay()
		return day, day, day.Local().Format("Jan 2, 2006")
	case calendarViewThreeDay:
		start := m.selectedCalendarThreeDayStart()
		end := start.AddDate(0, 0, 2)
		return start, end, calendarCompactDateRange(start, end)
	default:
		start, end := m.calendarAgendaDisplayRange()
		return start, end, calendarCompactDateRange(start, end)
	}
}

func (m *Model) setDefaultCalendarAgendaRange(now time.Time) {
	start := calendarDefaultAgendaStart(m.calendarEvents, now)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(start)
}

func (m *Model) calendarCurrentTime() time.Time {
	if !m.calendarNow.IsZero() {
		return m.calendarNow
	}
	return time.Now().Local()
}

func calendarDefaultAgendaStart(events []models.CalendarEvent, now time.Time) time.Time {
	today := calendarDayStartFor(now)
	defaultStart, defaultEnd := calendarAgendaWindowFor(today)
	for _, event := range events {
		if calendarEventOccursInAgendaWindow(event, defaultStart, defaultEnd) {
			return defaultStart
		}
	}
	var nearestFuture time.Time
	for _, event := range events {
		if event.Start.IsZero() {
			continue
		}
		day := calendarDayStartFor(event.Start)
		if !day.Before(today) {
			if nearestFuture.IsZero() || day.Before(nearestFuture) {
				nearestFuture = day
			}
			continue
		}
	}
	if !nearestFuture.IsZero() {
		return nearestFuture
	}
	return defaultStart
}

func calendarAgendaWindowFor(day time.Time) (time.Time, time.Time) {
	day = calendarDayStartFor(day)
	start := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	return start, start.AddDate(0, 1, 0)
}

func (m *Model) calendarAgendaWindow() (time.Time, time.Time) {
	if m.calendarAgendaStart.IsZero() || m.calendarAgendaEnd.IsZero() || !m.calendarAgendaEnd.After(m.calendarAgendaStart) {
		return calendarAgendaWindowFor(m.calendarCurrentTime())
	}
	return calendarDayStartFor(m.calendarAgendaStart), calendarDayStartFor(m.calendarAgendaEnd)
}

func (m *Model) calendarAgendaDisplayRange() (time.Time, time.Time) {
	start, end := m.calendarAgendaWindow()
	return start, end.AddDate(0, 0, -1)
}

func calendarCompactDateRange(start, end time.Time) string {
	start = calendarDayStartFor(start)
	end = calendarDayStartFor(end)
	if sameCalendarDate(start, end) {
		return start.Local().Format("Jan 2, 2006")
	}
	if start.Year() == end.Year() && start.Month() == end.Month() {
		return start.Local().Format("Jan 2") + "-" + end.Local().Format("2, 2006")
	}
	return start.Local().Format("Jan 2") + " - " + end.Local().Format("Jan 2, 2006")
}

func calendarRule(width int) string {
	if width < 1 {
		return ""
	}
	return strings.Repeat("─", width)
}

func (m *Model) renderCalendarMiniMonth(width int, start, end time.Time) []string {
	if width < 14 {
		return nil
	}
	start = calendarDayStartFor(start)
	end = calendarDayStartFor(end)
	month := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	gridStart := m.calendarWeekStartFor(month)
	header := month.Format("Jan 2006")
	leftPad := (width - ansi.StringWidth(header)) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	lines := []string{
		m.theme.Text.Primary.Style().Render(calendarFit(strings.Repeat(" ", leftPad)+header, width)),
		m.theme.Text.Dim.Style().Render(calendarFit(m.calendarMiniMonthWeekdayHeader(), width)),
	}
	for week := 0; week < 6; week++ {
		var row strings.Builder
		for dayIdx := 0; dayIdx < 7; dayIdx++ {
			day := gridStart.AddDate(0, 0, week*7+dayIdx)
			row.WriteString(m.renderCalendarMiniMonthDayCell(day, month, start, end))
			if dayIdx < 6 {
				row.WriteByte(' ')
			}
		}
		lines = append(lines, calendarFit(row.String(), width))
	}
	return lines
}

func (m *Model) calendarMiniMonthWeekdayHeader() string {
	if m != nil && m.cfg != nil && config.NormalizeCalendarWeekStart(m.cfg.Calendar.WeekStart) == config.CalendarWeekStartSunday {
		return "Su Mo Tu We Th Fr Sa"
	}
	return "Mo Tu We Th Fr Sa Su"
}

func (m *Model) renderCalendarMiniMonthDayCell(day, month, start, end time.Time) string {
	cellText := fmt.Sprintf("%2d", day.Day())
	inMonth := day.Month() == month.Month()
	inRange := !day.Before(start) && !day.After(end)
	selected := sameCalendarDate(day, m.selectedCalendarDay())
	_, hasEvent := m.calendarMiniMonthEventForDay(day)
	switch {
	case selected:
		style := m.theme.Focus.SelectionActive.Style()
		if hasEvent {
			style = style.Bold(true)
		}
		return style.Render(cellText)
	case hasEvent:
		style := m.theme.Text.Primary.Style().Bold(true)
		if !inMonth || !inRange {
			style = m.theme.Text.Dim.Style().Bold(true)
		}
		return style.Render(cellText)
	case !inMonth:
		return m.theme.Text.Dim.Style().Render(cellText)
	default:
		return m.theme.Text.Primary.Style().Render(cellText)
	}
}

func (m *Model) calendarMiniMonthEventForDay(day time.Time) (*models.CalendarEvent, bool) {
	day = calendarDayStartFor(day)
	for _, event := range m.calendarEvents {
		if event.Start.IsZero() || m.calendarEventHidden(event) {
			continue
		}
		if eventOccursOnCalendarDate(event, day) {
			visible := event
			return &visible, true
		}
	}
	return nil, false
}

func (m *Model) calendarStatusLine() string {
	start, end, rangeLabel := m.calendarActiveRange()
	mode := m.calendarViewTitle()
	if mode == "3-Day" {
		mode = "3-Day"
	}
	zone := time.Local.String()
	if zone == "Local" || zone == "" {
		zone = start.Local().Format("MST")
	}
	visible := len(m.indexedCalendarEventsForActiveAnchorRange())
	today := m.selectedCalendarDay()
	if today.IsZero() {
		today = start
	}
	week := ""
	if !start.IsZero() {
		_, w := start.ISOWeek()
		week = fmt.Sprintf("Week %d", w)
	}
	updated := "Updated: cached"
	if len(m.calendarEvents) > 0 && !m.calendarEvents[0].UpdatedAt.IsZero() {
		updated = "Updated: " + m.calendarEvents[0].UpdatedAt.Local().Format("15:04")
	}
	dayCount := 0
	for _, item := range m.indexedCalendarEventsForActiveAnchorRange() {
		if eventOccursOnCalendarDate(item.event, today) {
			dayCount++
		}
	}
	if sameCalendarDate(start, end) {
		return strings.Join([]string{mode, today.Local().Format("Jan 2, 2006"), zone, updated, fmt.Sprintf("%d events", dayCount)}, "  │  ")
	}
	return strings.Join([]string{week, rangeLabel, zone, updated, fmt.Sprintf("%d events visible", visible)}, "  │  ")
}

func (m *Model) calendarAgendaLine(event models.CalendarEvent, width int) string {
	timeText := calendarShortTime(event)
	source := calendarSourceLabel(event)
	marker := calendarRSVPMarker(event)
	sourceMarker := dynamicCalendarBlockStyle("#181819", m.calendarEventColor(event)).Render(" ")
	prefixW := 18
	if width < 48 {
		prefixW = 12
	}
	titleW := width - prefixW - 4
	if titleW < 8 {
		titleW = 8
	}
	prefix := calendarFit(timeText, prefixW)
	return calendarFit(sourceMarker+" "+prefix+"  "+ansi.Truncate(marker+event.Title+" - "+source, titleW, "..."), width-2)
}

func (m *Model) calendarGridCell(event *models.CalendarEvent, width int) string {
	if width < 4 {
		width = 4
	}
	if event == nil {
		return m.theme.Text.Dim.Style().Render(calendarFit(" ", width))
	}
	ref := event.Ref.WithDefaults()
	selected := m.calendarDetail != nil && m.calendarDetail.Ref.WithDefaults().LocalID == ref.LocalID
	text := calendarRSVPMarker(*event) + event.Title
	if width > 18 {
		text += " " + calendarDayTime(*event)
	}
	text = calendarFit(text, width)
	if selected {
		return m.theme.Focus.SelectionActive.Style().Render(text)
	}
	return dynamicCalendarBlockStyle("#181819", m.calendarEventColor(*event)).Render(text)
}

func (m *Model) calendarGridContinuationCell(event *models.CalendarEvent, width int) string {
	if width < 4 {
		width = 4
	}
	if event == nil {
		return m.theme.Text.Dim.Style().Render(calendarFit(" ", width))
	}
	return dynamicCalendarBlockStyle("#181819", m.calendarEventColor(*event)).Render(strings.Repeat(" ", width))
}

func (m *Model) calendarEventColor(event models.CalendarEvent) string {
	key := calendarEventCollectionKey(event)
	for i, collection := range m.calendarCollections {
		if calendarCollectionRefKey(collection.Ref) == key {
			return calendarCollectionColor(collection, i)
		}
	}
	for i, collection := range m.mergeCalendarCollections(nil) {
		if calendarCollectionRefKey(collection.Ref) == key {
			return calendarCollectionColor(collection, i)
		}
	}
	return "#76cce0"
}

func calendarSearchLine(event models.CalendarEvent, query string, width int) string {
	timeText := calendarShortTime(event)
	source := calendarSourceLabel(event)
	hint := calendarSearchMatchHint(event, query)
	marker := calendarRSVPMarker(event)
	prefixW := 18
	if width < 48 {
		prefixW = 12
	}
	titleW := width - prefixW - 2
	if titleW < 8 {
		titleW = 8
	}
	summary := marker + event.Title + " - " + source
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

func calendarMeetingPrepMailLine(m *Model, email *models.EmailData, width int) string {
	if email == nil {
		return ""
	}
	account := m.accountBadgeForEmail(email)
	if strings.TrimSpace(account) == "" {
		account = string(email.SourceID)
	}
	if strings.TrimSpace(account) == "" {
		account = "mail"
	}
	when := ""
	if !email.Date.IsZero() {
		when = email.Date.Local().Format("Mon 15:04")
	}
	prefix := strings.TrimSpace(when + " " + account)
	summary := email.Sender + " - " + email.Subject
	if prefix == "" {
		return ansi.Truncate(summary, width, "...")
	}
	return ansi.Truncate(prefix+"  "+summary, width, "...")
}

func calendarMeetingPrepEventLine(event models.CalendarEvent, width int) string {
	summary := calendarShortTime(event) + "  " + event.Title + " - " + calendarSourceLabel(event)
	return ansi.Truncate(summary, width, "...")
}

func calendarTravelBufferRecommendationLine(rec models.CalendarTravelBufferRecommendation, width int) string {
	summary := rec.Label
	if strings.TrimSpace(rec.Window) != "" {
		summary += " - " + rec.Window
	}
	if strings.TrimSpace(rec.Reason) != "" {
		summary += " (" + rec.Reason + ")"
	}
	return ansi.Truncate(summary, width, "...")
}

func calendarAISummarySourceLine(summary *models.CalendarAISummary) string {
	if summary == nil {
		return "0 cached mail  |  0 nearby events  |  mode unknown"
	}
	mailLabel := "cached mail"
	if len(summary.RelatedMail) != 1 {
		mailLabel = "cached mail"
	}
	eventLabel := "nearby event"
	if len(summary.NearbyEvents) != 1 {
		eventLabel = "nearby events"
	}
	generatedBy := strings.TrimSpace(summary.GeneratedBy)
	if generatedBy == "" {
		generatedBy = "cached fallback"
	}
	return fmt.Sprintf("%d %s  |  %d %s  |  %s", len(summary.RelatedMail), mailLabel, len(summary.NearbyEvents), eventLabel, generatedBy)
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
	return calendarFit(prefix+"  "+ansi.Truncate(calendarRSVPMarker(event)+event.Title+" - "+calendarStatusLabel(event), titleW, "..."), width-2)
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
	return calendarFit(prefix+"  "+ansi.Truncate(calendarRSVPMarker(event)+event.Title+" - "+calendarSourceLabel(event), titleW, "..."), width-2)
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
	return calendarFit(prefix+"  "+ansi.Truncate(calendarRSVPMarker(event)+event.Title+" - "+calendarSourceLabel(event), titleW, "..."), width-2)
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
	return m.theme.Metadata.Label.Style().Render(label) + " " + m.theme.Text.Primary.Style().Render(calendarFit(linkifyURLs(value), valueW))
}

func (m *Model) calendarDetailTitleLine(event models.CalendarEvent, width int) string {
	color := m.calendarEventColor(event)
	swatch := dynamicCalendarBlockStyle("#181819", color).Render("  ")
	titleW := width - ansi.StringWidth(swatch) - 1
	if titleW < 8 {
		titleW = 8
	}
	title := dynamicForegroundStyle(color).Bold(true).Render(calendarFit(event.Title, titleW))
	return swatch + " " + title
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
		title += " (" + mimeType + ")"
	}
	if uri := strings.TrimSpace(attachment.URI); uri != "" {
		return render.TerminalHyperlink(title, uri)
	}
	return title
}

func calendarDetailNoteLines(value string, width int) []string {
	notes := calendarRenderedNotes(value)
	if strings.TrimSpace(notes) == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(notes, "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if render.URLRe.MatchString(line) {
			out = append(out, calendarFit(linkifyURLs(line), width))
			continue
		}
		out = append(out, wrapText(line, width)...)
	}
	return out
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

func calendarReminderLabel(reminder models.CalendarReminder) string {
	method := strings.TrimSpace(strings.ToLower(reminder.Method))
	if method == "" {
		method = "popup"
	}
	minutes := reminder.MinutesBefore
	if minutes < 0 {
		minutes = 0
	}
	if minutes%1440 == 0 && minutes != 0 {
		return fmt.Sprintf("%s %dd before", method, minutes/1440)
	}
	if minutes%60 == 0 && minutes != 0 {
		return fmt.Sprintf("%s %dh before", method, minutes/60)
	}
	return fmt.Sprintf("%s %dm before", method, minutes)
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
	return calendarWeekStartForWithFirstDay(day, time.Monday)
}

func calendarWeekStartForWithFirstDay(day time.Time, firstDay time.Weekday) time.Time {
	if day.IsZero() {
		day = time.Now()
	}
	dayStart := calendarDayStartFor(day)
	daysSinceFirst := (int(dayStart.Weekday()) - int(firstDay) + 7) % 7
	return dayStart.AddDate(0, 0, -daysSinceFirst)
}

func (m *Model) calendarWeekStartFor(day time.Time) time.Time {
	firstDay := time.Monday
	if m != nil && m.cfg != nil && config.NormalizeCalendarWeekStart(m.cfg.Calendar.WeekStart) == config.CalendarWeekStartSunday {
		firstDay = time.Sunday
	}
	return calendarWeekStartForWithFirstDay(day, firstDay)
}

func calendarWeekRange(start time.Time) string {
	start = calendarWeekStartFor(start)
	end := start.AddDate(0, 0, 6)
	return calendarWeekRangeFromStart(start, end)
}

func (m *Model) calendarWeekRange(start time.Time) string {
	start = m.calendarWeekStartFor(start)
	end := start.AddDate(0, 0, 6)
	return calendarWeekRangeFromStart(start, end)
}

func calendarWeekRangeFromStart(start, end time.Time) string {
	if start.Year() == end.Year() {
		if start.Month() == end.Month() {
			return start.Format("Mon Jan 2") + " - " + end.Format("Mon Jan 2, 2006")
		}
		return start.Format("Mon Jan 2") + " - " + end.Format("Mon Jan 2, 2006")
	}
	return start.Format("Mon Jan 2, 2006") + " - " + end.Format("Mon Jan 2, 2006")
}

func calendarWeekDaySummary(start time.Time) string {
	start = calendarWeekStartFor(start)
	return calendarWeekDaySummaryFromStart(start)
}

func (m *Model) calendarWeekDaySummary(start time.Time) string {
	start = m.calendarWeekStartFor(start)
	return calendarWeekDaySummaryFromStart(start)
}

func calendarWeekDaySummaryFromStart(start time.Time) string {
	labels := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		labels = append(labels, start.AddDate(0, 0, i).Local().Format("Mon Jan 2"))
	}
	return strings.Join(labels, " | ")
}

func calendarWeekGridDayLabel(day time.Time, width int) string {
	day = day.Local()
	if width >= 10 {
		return day.Format("Mon Jan 2")
	}
	if width >= 6 {
		return day.Format("Mon 2")
	}
	return day.Format("02")
}

func calendarVisibleHourRange(events []indexedCalendarEvent) (int, int) {
	startHour := 7
	endHour := 18
	for _, item := range events {
		event := item.event
		if event.AllDay || event.Start.IsZero() || calendarEventSpansMultipleDates(event) {
			continue
		}
		start := event.Start.Local()
		if start.Hour() < startHour {
			startHour = start.Hour()
		}
		eventEnd := event.End.Local()
		if event.End.IsZero() {
			eventEnd = start
		}
		hour := eventEnd.Hour()
		if eventEnd.Minute() > 0 || eventEnd.Second() > 0 || eventEnd.Nanosecond() > 0 {
			hour++
		}
		if hour > 23 {
			hour = 23
		}
		if hour > endHour {
			endHour = hour
		}
	}
	if startHour < 0 {
		startHour = 0
	}
	if endHour > 23 {
		endHour = 23
	}
	if endHour < startHour {
		endHour = startHour
	}
	return startHour, endHour
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
	dayStart := calendarDayStartFor(day)
	dayEnd := dayStart.AddDate(0, 0, 1)
	eventStart, eventEnd := calendarEventDisplaySpan(event)
	return eventStart.Before(dayEnd) && eventEnd.After(dayStart)
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
	eventStart, eventEnd := calendarEventDisplaySpan(event)
	return eventStart.Before(end) && eventEnd.After(start)
}

func calendarEventOccursInAgendaWindow(event models.CalendarEvent, start, end time.Time) bool {
	if event.Start.IsZero() {
		return false
	}
	start = calendarDayStartFor(start)
	end = calendarDayStartFor(end)
	eventStart, eventEnd := calendarEventDisplaySpan(event)
	if !eventStart.Before(end) || !eventEnd.After(start) {
		return false
	}
	return !eventStart.Before(start)
}

func calendarEventEndedBeforeDay(event models.CalendarEvent, day time.Time) bool {
	if event.Start.IsZero() {
		return false
	}
	dayStart := calendarDayStartFor(day)
	_, eventEnd := calendarEventDisplaySpan(event)
	return !eventEnd.After(dayStart)
}

func calendarEventDisplaySpan(event models.CalendarEvent) (time.Time, time.Time) {
	start := event.Start.Local()
	if event.AllDay {
		start = calendarDayStartFor(start)
		end := calendarDayStartFor(event.End)
		if event.End.IsZero() || !end.After(start) {
			end = start.AddDate(0, 0, 1)
		}
		return start, end
	}
	end := event.End.Local()
	if event.End.IsZero() || !end.After(start) {
		end = start.Add(time.Nanosecond)
	}
	return start, end
}

func calendarEventSpansMultipleDates(event models.CalendarEvent) bool {
	if event.Start.IsZero() || event.End.IsZero() {
		return false
	}
	start, end := calendarEventDisplaySpan(event)
	if event.AllDay {
		return end.After(start.AddDate(0, 0, 1))
	}
	latestOccupied := end.Add(-time.Nanosecond)
	return !sameCalendarDate(start, latestOccupied)
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
	if ref.CalendarID != "" && !looksLikeCalendarProviderID(ref.CalendarID) {
		return calendarHumanLabel(ref.CalendarID)
	}
	if ref.AccountID != "" && ref.AccountID != models.DefaultAccountID {
		return calendarHumanLabel(string(ref.AccountID))
	}
	if ref.SourceID != "" && ref.SourceID != models.DefaultCalendarSourceID {
		return calendarHumanLabel(string(ref.SourceID))
	}
	return "calendar"
}

func normalizeCalendarEventsForDisplay(events []models.CalendarEvent) []models.CalendarEvent {
	out := make([]models.CalendarEvent, 0, len(events))
	for _, event := range events {
		if event.Start.IsZero() {
			continue
		}
		event = normalizeCachedCalendarRawForDisplay(event)
		event.Ref = event.Ref.WithDefaults()
		if event.AllDay {
			event.Start = normalizeAllDayCalendarDateForDisplay(event.Start)
			if !event.End.IsZero() {
				event.End = normalizeAllDayCalendarDateForDisplay(event.End)
			}
		}
		if !event.Start.IsZero() && !event.End.IsZero() && event.End.Before(event.Start) {
			event.End = event.Start
		}
		out = append(out, event)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			if out[i].Start.IsZero() {
				return false
			}
			if out[j].Start.IsZero() {
				return true
			}
			return out[i].Start.Before(out[j].Start)
		}
		if out[i].AllDay != out[j].AllDay {
			return out[i].AllDay
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].Ref.LocalID < out[j].Ref.LocalID
	})
	return out
}

func normalizeCachedCalendarRawForDisplay(event models.CalendarEvent) models.CalendarEvent {
	return calendarpkg.NormalizeEventFromRaw(event)
}

func normalizeAllDayCalendarDateForDisplay(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	if value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0 {
		return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.Local)
	}
	return calendarDayStartFor(value)
}

func (m *Model) mergeCalendarCollections(collections []models.CalendarCollection) []models.CalendarCollection {
	seen := make(map[string]bool)
	out := make([]models.CalendarCollection, 0, len(collections)+4)
	for _, collection := range collections {
		collection.Ref.Kind = models.SourceKindCalendar
		collection.Ref.SourceID = models.NormalizeSourceID(collection.Ref.SourceID, models.DefaultCalendarSourceID)
		collection.Ref.AccountID = models.NormalizeAccountID(collection.Ref.AccountID)
		if strings.TrimSpace(collection.Ref.CollectionID) == "" {
			continue
		}
		if strings.TrimSpace(collection.Ref.DisplayName) == "" {
			collection.Ref.DisplayName = calendarHumanLabel(collection.Ref.CollectionID)
		}
		key := calendarCollectionRefKey(collection.Ref)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, collection)
	}
	for _, event := range m.calendarEvents {
		ref := event.Ref.WithDefaults()
		collectionRef := models.CollectionRef{
			SourceID:     ref.SourceID,
			AccountID:    ref.AccountID,
			Kind:         models.SourceKindCalendar,
			CollectionID: ref.CalendarID,
			DisplayName:  calendarSourceLabel(event),
		}
		if strings.TrimSpace(collectionRef.CollectionID) == "" {
			continue
		}
		key := calendarCollectionRefKey(collectionRef)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, models.CalendarCollection{Ref: collectionRef})
	}
	return out
}

func (m *Model) pruneCalendarCollectionState() {
	if m.calendarHiddenCollections == nil {
		m.calendarHiddenCollections = make(map[string]bool)
	}
	known := make(map[string]bool)
	for _, collection := range m.calendarCollections {
		known[calendarCollectionRefKey(collection.Ref)] = true
	}
	for key := range m.calendarHiddenCollections {
		if !known[key] {
			delete(m.calendarHiddenCollections, key)
		}
	}
	m.applySelectedCalendarConfig()
	if m.calendarRailCursor >= len(m.calendarCollections) {
		m.calendarRailCursor = len(m.calendarCollections) - 1
	}
	if m.calendarRailCursor < 0 {
		m.calendarRailCursor = 0
	}
	m.ensureCalendarSelectionVisible()
}

func (m *Model) indexedVisibleCalendarEvents() []indexedCalendarEvent {
	cache := m.ensureCalendarDerivedCache()
	if cache.VisibleValid {
		return cache.Visible
	}
	out := m.indexedAllVisibleCalendarEvents()
	if m.calendarView != calendarViewAgenda {
		cache.Visible = out
		cache.VisibleValid = true
		return out
	}
	filtered := m.filterAgendaVisibleEvents(out, m.calendarAgendaShowPast)
	cache.Visible = filtered
	cache.VisibleValid = true
	return filtered
}

func (m *Model) filterAgendaVisibleEvents(events []indexedCalendarEvent, showPast bool) []indexedCalendarEvent {
	agendaStart, agendaEnd := m.calendarAgendaWindow()
	filtered := make([]indexedCalendarEvent, 0, len(events))
	today := calendarDayStartFor(m.calendarCurrentTime())
	for _, item := range events {
		if !calendarEventOccursInAgendaWindow(item.event, agendaStart, agendaEnd) {
			continue
		}
		if !showPast && calendarEventEndedBeforeDay(item.event, today) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (m *Model) calendarAgendaWindowEvents() []indexedCalendarEvent {
	out := m.indexedAllVisibleCalendarEvents()
	agendaStart, agendaEnd := m.calendarAgendaWindow()
	filtered := make([]indexedCalendarEvent, 0, len(out))
	for _, item := range out {
		if calendarEventOccursInAgendaWindow(item.event, agendaStart, agendaEnd) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m *Model) calendarAgendaHiddenPastCount() int {
	if m.calendarView != "" && m.calendarView != calendarViewAgenda {
		return 0
	}
	today := calendarDayStartFor(m.calendarCurrentTime())
	count := 0
	for _, item := range m.calendarAgendaWindowEvents() {
		if calendarEventEndedBeforeDay(item.event, today) {
			count++
		}
	}
	return count
}

func (m *Model) calendarAgendaPastNoticeLines(count int) []string {
	if count <= 0 {
		return nil
	}
	noun := "events"
	if count == 1 {
		noun = "event"
	}
	action := "Show past"
	state := "hidden"
	if m.calendarAgendaShowPast {
		action = "Hide past"
		state = "shown"
	}
	return []string{
		fmt.Sprintf("%d past %s %s before %s", count, noun, state, calendarDayStartFor(m.calendarCurrentTime()).Local().Format("Jan 2")),
		fmt.Sprintf("[p] %s", action),
	}
}

func (m *Model) indexedAllVisibleCalendarEvents() []indexedCalendarEvent {
	cache := m.ensureCalendarDerivedCache()
	if cache.AllValid {
		return cache.AllVisible
	}
	out := make([]indexedCalendarEvent, 0, len(m.calendarEvents))
	for i, event := range m.calendarEvents {
		if m.calendarEventHidden(event) || event.Start.IsZero() {
			continue
		}
		out = append(out, indexedCalendarEvent{index: i, event: event})
	}
	cache.AllVisible = out
	cache.AllValid = true
	return out
}

func (m *Model) calendarEventHidden(event models.CalendarEvent) bool {
	if m.calendarHiddenCollections == nil {
		return false
	}
	return m.calendarHiddenCollections[calendarEventCollectionKey(event)]
}

func (m *Model) calendarVisibleOffset() int {
	cache := m.ensureCalendarDerivedCache()
	if cache.Offsets == nil {
		cache.Offsets = make(map[int]int)
	}
	if offset, ok := cache.Offsets[m.calendarCursor]; ok {
		return offset
	}
	events := m.indexedVisibleCalendarEvents()
	for i, item := range events {
		if item.index == m.calendarCursor {
			cache.Offsets[m.calendarCursor] = i
			return i
		}
	}
	cache.Offsets[m.calendarCursor] = 0
	return 0
}

func (m *Model) ensureCalendarSelectionVisible() {
	events := m.indexedVisibleCalendarEvents()
	if len(events) == 0 {
		m.calendarDetail = nil
		return
	}
	if m.calendarDetail == nil {
		for _, item := range events {
			if calendarEventNeedsAction(item.event) {
				m.calendarCursor = item.index
				m.calendarDetail = &item.event
				return
			}
		}
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

func (m *Model) cycleCalendarFocus(delta int) {
	order := []calendarFocusPanel{calendarFocusRail, calendarFocusMain, calendarFocusDetail}
	idx := 1
	for i, panel := range order {
		if panel == m.calendarFocus {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = len(order) - 1
	}
	if idx >= len(order) {
		idx = 0
	}
	m.calendarFocus = order[idx]
}

func (m *Model) moveCalendarRailSelection(delta int) {
	if len(m.calendarCollections) == 0 {
		return
	}
	m.calendarRailCursor += delta
	if m.calendarRailCursor < 0 {
		m.calendarRailCursor = 0
	}
	if m.calendarRailCursor >= len(m.calendarCollections) {
		m.calendarRailCursor = len(m.calendarCollections) - 1
	}
}

func (m *Model) toggleFocusedCalendarCollection() {
	if len(m.calendarCollections) == 0 || m.calendarRailCursor < 0 || m.calendarRailCursor >= len(m.calendarCollections) {
		return
	}
	if m.calendarHiddenCollections == nil {
		m.calendarHiddenCollections = make(map[string]bool)
	}
	key := calendarCollectionRefKey(m.calendarCollections[m.calendarRailCursor].Ref)
	if m.calendarHiddenCollections[key] {
		delete(m.calendarHiddenCollections, key)
		m.calendarStatus = "Calendar shown: " + calendarCollectionDisplayName(m.calendarCollections[m.calendarRailCursor])
	} else {
		m.calendarHiddenCollections[key] = true
		m.calendarStatus = "Calendar hidden: " + calendarCollectionDisplayName(m.calendarCollections[m.calendarRailCursor])
	}
	m.invalidateCalendarFilterDerivations()
	m.ensureCalendarSelectionVisible()
	m.persistSelectedCalendarConfig()
}

func (m *Model) applySelectedCalendarConfig() {
	if m.cfg == nil || len(m.calendarCollections) == 0 {
		return
	}
	selected := make(map[string]bool, len(m.cfg.Calendar.SelectedCalendars))
	for _, key := range m.cfg.Calendar.SelectedCalendars {
		key = strings.TrimSpace(key)
		if key != "" {
			selected[key] = true
		}
	}
	if len(selected) == 0 {
		return
	}
	if m.calendarHiddenCollections == nil {
		m.calendarHiddenCollections = make(map[string]bool)
	}
	known := make(map[string]bool, len(m.calendarCollections))
	matchedKnownSelection := false
	for _, collection := range m.calendarCollections {
		key := calendarCollectionRefKey(collection.Ref)
		known[key] = true
		if selected[key] {
			matchedKnownSelection = true
		}
	}
	if !matchedKnownSelection {
		return
	}
	for key := range known {
		if selected[key] {
			delete(m.calendarHiddenCollections, key)
		} else {
			m.calendarHiddenCollections[key] = true
		}
	}
	for key := range m.calendarHiddenCollections {
		if !known[key] {
			delete(m.calendarHiddenCollections, key)
		}
	}
	m.invalidateCalendarFilterDerivations()
}

func (m *Model) persistSelectedCalendarConfig() {
	if m.cfg == nil || len(m.calendarCollections) == 0 {
		return
	}
	selected := make([]string, 0, len(m.calendarCollections))
	for _, collection := range m.calendarCollections {
		key := calendarCollectionRefKey(collection.Ref)
		if !m.calendarHiddenCollections[key] {
			selected = append(selected, key)
		}
	}
	sort.Strings(selected)
	m.cfg.Calendar.SelectedCalendars = selected
	if strings.TrimSpace(m.configPath) == "" {
		return
	}
	if err := m.cfg.Save(m.configPath); err != nil {
		m.calendarStatus = "Calendar selection save failed: " + err.Error()
		return
	}
}

func (m *Model) calendarCollectionPendingCount(ref models.CollectionRef) int {
	count := 0
	key := calendarCollectionRefKey(ref)
	for _, event := range m.calendarEvents {
		if calendarEventCollectionKey(event) == key && calendarEventNeedsAction(event) {
			count++
		}
	}
	return count
}

func calendarCollectionRefKey(ref models.CollectionRef) string {
	ref.Kind = models.SourceKindCalendar
	ref.SourceID = models.NormalizeSourceID(ref.SourceID, models.DefaultCalendarSourceID)
	ref.AccountID = models.NormalizeAccountID(ref.AccountID)
	return ref.CacheKey()
}

func calendarEventCollectionKey(event models.CalendarEvent) string {
	ref := event.Ref.WithDefaults()
	return calendarCollectionRefKey(models.CollectionRef{
		SourceID:     ref.SourceID,
		AccountID:    ref.AccountID,
		Kind:         models.SourceKindCalendar,
		CollectionID: ref.CalendarID,
	})
}

func calendarCollectionGroupLabel(collection models.CalendarCollection) string {
	ref := collection.Ref
	if ref.AccountID != "" && ref.AccountID != models.DefaultAccountID {
		return calendarHumanLabel(string(ref.AccountID))
	}
	if ref.SourceID != "" && ref.SourceID != models.DefaultCalendarSourceID {
		return calendarHumanLabel(string(ref.SourceID))
	}
	return "Calendars"
}

func calendarCollectionDisplayName(collection models.CalendarCollection) string {
	if name := strings.TrimSpace(collection.Ref.DisplayName); name != "" {
		return name
	}
	if id := strings.TrimSpace(collection.Ref.CollectionID); id != "" {
		return calendarHumanLabel(id)
	}
	return "Calendar"
}

func calendarCollectionColor(collection models.CalendarCollection, idx int) string {
	if color := strings.TrimSpace(collection.Color); color != "" {
		return color
	}
	palette := []string{"#5fd7ff", "#ff5f87", "#87d75f", "#ffd75f", "#af87ff", "#ff875f"}
	if idx < 0 {
		idx = 0
	}
	return palette[idx%len(palette)]
}

func calendarFrameDateRange(start, end time.Time) string {
	start = calendarDayStartFor(start)
	end = calendarDayStartFor(end)
	rangeText := start.Format("Mon Jan 2, 2006")
	if !sameCalendarDate(start, end) {
		rangeText = start.Format("Mon Jan 2") + " - " + end.Format("Mon Jan 2, 2006")
	}
	return rangeText
}

func calendarRenderedNotes(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = calendarHTMLListItemPattern.ReplaceAllString(value, "\n- ")
	value = calendarHTMLBreakPattern.ReplaceAllString(value, "\n")
	value = calendarHTMLTagPattern.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	value = strings.NewReplacer("**", "", "__", "", "`", "").Replace(value)
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func calendarEventNeedsAction(event models.CalendarEvent) bool {
	for _, attendee := range event.Attendees {
		status, _ := models.NormalizeCalendarRSVP(attendee.RSVP)
		if status == "needs-action" {
			return true
		}
	}
	return false
}

func calendarRSVPMarker(event models.CalendarEvent) string {
	if calendarEventNeedsAction(event) {
		return "! "
	}
	return ""
}

func calendarRSVPActionLabel(event models.CalendarEvent) string {
	status := firstCalendarAttendeeRSVP(event)
	normalized, err := models.NormalizeCalendarRSVP(status)
	if err != nil {
		normalized = "needs-action"
	}
	if normalized == "needs-action" {
		return "needs response"
	}
	return normalized
}

func looksLikeCalendarProviderID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) >= 24 && strings.Count(value, "-") >= 3 {
		return true
	}
	if len(value) >= 32 && strings.IndexFunc(value, func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F')
	}) == -1 {
		return true
	}
	return strings.Contains(value, "@group.calendar.google.com")
}

func calendarHumanLabel(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "-calendar")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.Join(strings.Fields(value), " ")
	switch strings.ToLower(value) {
	case "":
		return "calendar"
	case "icloud":
		return "iCloud"
	case "google":
		return "Google"
	case "us holidays":
		return "US Holidays"
	}
	words := strings.Fields(value)
	for i, word := range words {
		lower := strings.ToLower(word)
		if len(lower) == 0 {
			continue
		}
		words[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(words, " ")
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
