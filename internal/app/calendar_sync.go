package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// Background reads use a separate result so they never reset navigation or an
// editor as a foreground agenda load would. A newer load/mutation supersedes them.
type calendarBackgroundRefreshedMsg struct {
	events      []models.CalendarEvent
	collections []models.CalendarCollection
	err         error
	generation  uint64
}

func (m *Model) invalidateCalendarBackgroundRefresh() {
	m.calendarRefreshGeneration++
	m.calendarBackgroundRefreshing = false
}

func (m *Model) calendarBackgroundRefreshBlocked() bool {
	return !m.calendarAvailable || m.calendarLoading || m.calendarEdit.Active ||
		m.calendarDelete.Active || m.calendarDetailLoading || m.calendarInvitation.Saving ||
		m.calendarRSVPSaving || m.calendarSearchLoading || m.crossSourceSearchLoading
}

func (m *Model) startCalendarBackgroundRefresh() tea.Cmd {
	if m.calendarBackgroundRefreshing || m.calendarBackgroundRefreshBlocked() {
		return nil
	}
	cached, ok := m.backend.(backend.CalendarAgendaCacheBackend)
	if !ok {
		return nil
	}
	m.calendarBackgroundRefreshing = true
	generation := m.calendarRefreshGeneration
	return func() tea.Msg {
		events, collections, err := cached.RefreshCalendarAgenda(time.Time{}, time.Time{})
		return calendarBackgroundRefreshedMsg{events: events, collections: collections, err: err, generation: generation}
	}
}

func (m *Model) applyCalendarBackgroundRefresh(msg calendarBackgroundRefreshedMsg) tea.Cmd {
	if msg.generation != m.calendarRefreshGeneration {
		return nil
	}
	m.calendarBackgroundRefreshing = false
	// A user may have started editing or loading a different view during the read.
	if m.calendarBackgroundRefreshBlocked() {
		return nil
	}
	if msg.err != nil {
		m.calendarStatus = "Calendar refresh failed; showing cached events: " + msg.err.Error()
		return nil
	}
	selected := m.selectedCalendarEvent()
	m.setCalendarEventsForDisplay(msg.events)
	m.setCalendarCollections(m.mergeCalendarCollections(msg.collections))
	m.pruneCalendarCollectionState()
	if strings.HasPrefix(m.calendarStatus, "Calendar refresh failed;") {
		m.calendarStatus = ""
	}
	switch m.calendarView {
	case calendarViewSearch:
		m.calendarSearchLoading = true
		return m.loadCalendarSearch()
	case calendarViewCrossSearch:
		return m.loadCrossSourceSearch()
	}
	visible := m.indexedCalendarEventsForActiveAnchorRange()
	if len(visible) == 0 {
		m.calendarCursor = 0
		m.calendarDetail = nil
		return nil
	}
	chosen := visible[0]
	for _, item := range visible {
		if selected != nil && item.event.Ref.WithDefaults().LocalID == selected.Ref.WithDefaults().LocalID {
			chosen = item
			break
		}
	}
	m.calendarCursor = chosen.index
	event := chosen.event
	m.calendarDetail = &event
	return nil
}
