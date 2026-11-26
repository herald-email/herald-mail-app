package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// Execute the refresh command without waiting for the real minute timer.
func calendarTickRefreshForTest(t *testing.T, m *Model) tea.Msg {
	t.Helper()
	_, cmd := m.Update(CalendarClockTickMsg{Now: m.calendarCurrentTime().Add(time.Minute)})
	if cmd == nil {
		t.Fatal("calendar tick did not schedule work")
	}
	ready := make(chan tea.Msg, 1)
	go func() { ready <- cmd() }()
	select {
	case msg := <-ready:
		batch, ok := msg.(tea.BatchMsg)
		if !ok || len(batch) != 2 {
			t.Fatalf("tick commands = %T, want clock and refresh", msg)
		}
		return batch[1]()
	case <-time.After(200 * time.Millisecond):
		t.Fatal("calendar tick only scheduled the clock; no provider refresh")
	}
	return nil
}

func TestCalendarTickPicksUpExternalEdit(t *testing.T) {
	for _, view := range []calendarViewMode{calendarViewAgenda, calendarViewDay, calendarViewWeek, calendarViewThreeDay} {
		t.Run(string(view), func(t *testing.T) {
			now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.Local)
			first := models.CalendarEvent{Ref: models.EventRef{SourceID: "google-calendar", AccountID: "default", CalendarID: "work", EventID: "first"}.WithDefaults(), Title: "First event", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}
			selected := first
			selected.Ref = models.EventRef{SourceID: "google-calendar", AccountID: "default", CalendarID: "work", EventID: "selected"}.WithDefaults()
			selected.Title = "Old title"
			selected.Start = now.Add(3 * time.Hour)
			selected.End = now.Add(4 * time.Hour)
			edited := selected
			edited.Title = "Edited in Google Calendar"
			edited.Start = now.Add(30 * time.Minute)
			edited.End = now.Add(45 * time.Minute)
			b := &calendarAgendaStubBackend{available: true, refreshEvents: []models.CalendarEvent{edited, first}}
			m := New(b, nil, "", nil, false)
			m.loading = false
			m.activeTab = tabCalendar
			m.calendarNow = now
			m.setCalendarEventsForDisplay([]models.CalendarEvent{first, selected})
			m.calendarCursor = 1
			m.setCalendarView(view)
			m.calendarCursor = 1
			m.calendarDetail = &selected
			m.calendarDetailOpen = true
			msg := calendarTickRefreshForTest(t, m)
			if b.refreshAgendaCalls != 1 {
				t.Fatalf("provider refresh calls = %d", b.refreshAgendaCalls)
			}
			m.Update(msg)
			got := m.selectedCalendarEvent()
			if got == nil || got.Title != edited.Title || !got.Start.Equal(edited.Start) {
				t.Fatalf("selected event after refresh = %#v", got)
			}
			if m.calendarDetail == nil || m.calendarDetail.Title != edited.Title || !m.calendarDetailOpen {
				t.Fatalf("open detail did not update: %#v", m.calendarDetail)
			}
		})
	}
}

func TestCalendarBackgroundRefreshGuards(t *testing.T) {
	for name, block := range map[string]func(*Model){
		"unavailable":         func(m *Model) { m.calendarAvailable = false },
		"foreground load":     func(m *Model) { m.calendarLoading = true },
		"editor":              func(m *Model) { m.calendarEdit.Active = true },
		"delete confirmation": func(m *Model) { m.calendarDelete.Active = true },
		"detail load":         func(m *Model) { m.calendarDetailLoading = true },
		"RSVP":                func(m *Model) { m.calendarRSVPSaving = true },
		"invitation save":     func(m *Model) { m.calendarInvitation.Saving = true },
		"search":              func(m *Model) { m.calendarSearchLoading = true },
	} {
		t.Run(name, func(t *testing.T) {
			m := New(&calendarAgendaStubBackend{available: true}, nil, "", nil, false)
			block(m)
			if cmd := m.startCalendarBackgroundRefresh(); cmd != nil {
				t.Fatal("refresh started during protected operation")
			}
		})
	}
	m := New(&calendarAgendaStubBackend{available: true}, nil, "", nil, false)
	first := m.startCalendarBackgroundRefresh()
	if first == nil {
		t.Fatal("first refresh missing")
	}
	if m.startCalendarBackgroundRefresh() != nil {
		t.Fatal("overlapping refresh")
	}
	m.Update(first())
	if m.startCalendarBackgroundRefresh() == nil {
		t.Fatal("refresh did not rearm")
	}
}

func TestCalendarBackgroundRefreshKeepsDraftAndRejectsSupersededRead(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, refreshEvents: []models.CalendarEvent{{Title: "Stale response"}}}
	m := New(b, nil, "", nil, false)
	cmd := m.startCalendarBackgroundRefresh()
	m.calendarEdit.Active = true
	m.calendarEdit.Draft.Title = "Unsaved title"
	m.Update(cmd())
	if m.calendarEdit.Draft.Title != "Unsaved title" || len(m.calendarEvents) != 0 {
		t.Fatal("background refresh modified an active draft or agenda")
	}
	m.calendarEdit.Active = false
	cmd = m.startCalendarBackgroundRefresh()
	m.loadCalendarAgenda() // A foreground load supersedes the earlier read.
	m.Update(cmd())
	if len(m.calendarEvents) != 0 || !m.calendarLoading {
		t.Fatal("superseded read changed foreground state")
	}
}

func TestCalendarBackgroundRefreshFailureAndRetry(t *testing.T) {
	m := New(&calendarAgendaStubBackend{available: true}, nil, "", nil, false)
	m.setCalendarEventsForDisplay([]models.CalendarEvent{{Ref: models.EventRef{SourceID: "google", CalendarID: "work", EventID: "cached"}.WithDefaults(), Title: "Cached event", Start: time.Now().Add(time.Hour), End: time.Now().Add(2 * time.Hour)}})
	m.calendarBackgroundRefreshing = true
	m.Update(calendarBackgroundRefreshedMsg{generation: m.calendarRefreshGeneration, err: fmt.Errorf("offline")})
	if len(m.calendarEvents) != 1 || m.calendarEvents[0].Title != "Cached event" {
		t.Fatal("failure discarded cached events")
	}
	if !strings.Contains(m.calendarStatus, "offline") {
		t.Fatal("failure not reported")
	}
	cmd := m.startCalendarBackgroundRefresh()
	if cmd == nil {
		t.Fatal("failure prevented retry")
	}
	m.Update(cmd())
	if m.calendarStatus != "" {
		t.Fatalf("recovery left stale status %q", m.calendarStatus)
	}
}

func TestCalendarBackgroundRefreshDeletesAndFilters(t *testing.T) {
	now := time.Now().Add(time.Hour)
	event := models.CalendarEvent{Ref: models.EventRef{SourceID: "google", CalendarID: "work", EventID: "deleted"}.WithDefaults(), Title: "Deleted externally", Start: now, End: now.Add(time.Hour)}
	b := &calendarAgendaStubBackend{available: true}
	m := New(b, nil, "", nil, false)
	m.setCalendarEventsForDisplay([]models.CalendarEvent{event})
	m.calendarDetail = &event
	m.Update(m.startCalendarBackgroundRefresh()())
	if len(m.calendarEvents) != 0 || m.calendarDetail != nil {
		t.Fatal("externally deleted event remains displayed")
	}
	b.refreshEvents = []models.CalendarEvent{event}
	m.calendarHiddenCollections = map[string]bool{calendarEventCollectionKey(event): true}
	m.Update(m.startCalendarBackgroundRefresh()())
	if m.calendarDetail != nil {
		t.Fatal("refresh selected a hidden calendar event")
	}
}

func TestCalendarBackgroundRefreshWhileAnotherTabIsOpen(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true}
	m := New(b, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.Update(calendarTickRefreshForTest(t, m))
	if b.refreshAgendaCalls != 1 || m.activeTab != tabTimeline {
		t.Fatal("refresh should run without switching tabs")
	}
}

func TestCalendarLateCacheReadDoesNotBlockBackgroundRefresh(t *testing.T) {
	m := New(&calendarAgendaStubBackend{available: true}, nil, "", nil, false)
	m.loadCalendarAgenda()
	// Concurrent cache/provider commands are allowed to finish in either order.
	m.Update(CalendarAgendaLoadedMsg{})
	m.Update(CalendarAgendaLoadedMsg{Cached: true, Refreshing: true})
	if m.startCalendarBackgroundRefresh() == nil {
		t.Fatal("late cache read left calendar permanently loading")
	}
}

func TestCalendarBackgroundRefreshReloadsSearch(t *testing.T) {
	now := time.Now().Add(time.Hour)
	event := models.CalendarEvent{Ref: models.EventRef{SourceID: "google", CalendarID: "work", EventID: "edited"}.WithDefaults(), Title: "Search updated", Start: now, End: now.Add(time.Hour)}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event}}
	m := New(b, nil, "", nil, false)
	m.calendarView = calendarViewSearch
	m.calendarSearchQuery = "Search"
	_, cmd := m.Update(m.startCalendarBackgroundRefresh()())
	if cmd == nil {
		t.Fatal("refresh did not reload active search")
	}
	m.Update(cmd())
	if got := m.selectedCalendarEvent(); got == nil || got.Title != event.Title {
		t.Fatalf("search remained stale: %#v", got)
	}
}

func TestCalendarBackgroundRefreshCannotUndoCompletedSave(t *testing.T) {
	now := time.Now().Add(time.Hour)
	event := models.CalendarEvent{Ref: models.EventRef{SourceID: "google", CalendarID: "work", EventID: "edited"}.WithDefaults(), Title: "Old title", Start: now, End: now.Add(time.Hour)}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event}}
	m := New(b, nil, "", nil, false)
	m.setCalendarEventsForDisplay(b.events)
	pending := m.startCalendarBackgroundRefresh()()
	m.calendarEdit.Ref = event.Ref
	event.Title = "Saved title"
	m.Update(CalendarEventSavedMsg{Ref: event.Ref, Event: &event})
	m.Update(pending)
	if got := m.selectedCalendarEvent(); got == nil || got.Title != "Saved title" {
		t.Fatalf("background read undid completed save: %#v", got)
	}
}
