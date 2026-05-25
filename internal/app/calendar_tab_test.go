package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type calendarAgendaStubBackend struct {
	stubBackend
	available bool
	events    []models.CalendarEvent
	getCalls  int
}

func (b *calendarAgendaStubBackend) CalendarAgendaAvailable() bool {
	return b.available
}

func (b *calendarAgendaStubBackend) ListCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	out := make([]models.CalendarEvent, 0, len(b.events))
	for _, event := range b.events {
		if !start.IsZero() && !event.End.IsZero() && !event.End.After(start) {
			continue
		}
		if !end.IsZero() && !event.Start.IsZero() && !event.Start.Before(end) {
			continue
		}
		out = append(out, event)
	}
	return out, nil
}

func (b *calendarAgendaStubBackend) GetCalendarEvent(ref models.EventRef) (*models.CalendarEvent, error) {
	b.getCalls++
	ref = ref.WithDefaults()
	for _, event := range b.events {
		if event.Ref.WithDefaults().LocalID == ref.LocalID {
			got := event
			return &got, nil
		}
	}
	return nil, nil
}

func TestCalendarTabHiddenForMailOnlyBackend(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline

	rendered := stripANSI(m.renderTabBar())
	if strings.Contains(rendered, "Calendar") {
		t.Fatalf("mail-only tab bar should not advertise Calendar:\n%s", rendered)
	}
	if got := stripANSI(m.renderKeyHints()); !strings.Contains(got, "1-2: tabs") || strings.Contains(got, "1-3: tabs") {
		t.Fatalf("mail-only hints = %q, want 1-2 tabs only", got)
	}

	model, _, handled := m.handleTabKey(keyRune('3'))
	if handled {
		t.Fatal("mail-only 3 key should not be handled as a calendar tab")
	}
	if model.(*Model).activeTab != tabTimeline {
		t.Fatalf("active tab changed to %d, want Timeline", model.(*Model).activeTab)
	}
}

func TestCalendarAgendaTabLoadsAndRendersReadOnlyDetail(t *testing.T) {
	events := testCalendarEvents()
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false

	model, cmd := m.handleKeyMsg(keyRunes("3"))
	m = model.(*Model)
	if m.activeTab != tabCalendar {
		t.Fatalf("activeTab = %d, want Calendar", m.activeTab)
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}

	if len(m.calendarEvents) != len(events) {
		t.Fatalf("calendar events = %d, want %d", len(m.calendarEvents), len(events))
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Calendar", "Agenda", "Design review", "Event Detail", "Herald planning room", "read-only"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("calendar view missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "etag") || strings.Contains(rendered, "oauth") || strings.Contains(rendered, "caldav") {
		t.Fatalf("calendar view exposed provider internals:\n%s", rendered)
	}

	model, _ = m.handleKeyMsg(keyRunes("j"))
	m = model.(*Model)
	if m.calendarCursor != 1 {
		t.Fatalf("calendar cursor = %d, want 1", m.calendarCursor)
	}
}

func TestCalendarEventDetailOpensAndEscReturnsToAgenda(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarCursor = 2

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Enter to open calendar detail")
	}
	if b.getCalls != 1 {
		t.Fatalf("GetCalendarEvent calls = %d, want 1", b.getCalls)
	}
	detail := stripANSI(m.renderMainView())
	if !strings.Contains(detail, "Event Detail") || !strings.Contains(detail, "Weekly planning") {
		t.Fatalf("detail view missing selected event:\n%s", detail)
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarDetailOpen {
		t.Fatal("expected Esc to return from detail to agenda")
	}
	if m.calendarCursor != 2 {
		t.Fatalf("calendar cursor = %d, want preserved index 2", m.calendarCursor)
	}
}

func TestCalendarDayAgendaSwitchesFromAgendaAndRendersDrawer(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = m.selectedCalendarEvent()

	model, _ := m.handleKeyMsg(keyRunes("d"))
	m = model.(*Model)
	if m.calendarView != calendarViewDay {
		t.Fatalf("calendarView = %q, want %q", m.calendarView, calendarViewDay)
	}

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Day Agenda", "Sun May 24", "Design review", "Daily standup", "Day Drawer", "Herald planning room", "Local", "Event TZ", "h/l: day", "a: agenda"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("day agenda missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Weekly planning") {
		t.Fatalf("day agenda should filter out events from other days:\n%s", rendered)
	}
}

func TestCalendarDayAgendaCanReturnToAgendaList(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarView = calendarViewDay
	m.calendarDay = b.events[0].Start

	model, _ := m.handleKeyMsg(keyRunes("a"))
	m = model.(*Model)
	if m.calendarView != calendarViewAgenda {
		t.Fatalf("calendarView = %q, want %q", m.calendarView, calendarViewAgenda)
	}
	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "Agenda") || strings.Contains(rendered, "Day Drawer") {
		t.Fatalf("agenda view was not restored:\n%s", rendered)
	}
}

func TestCalendarDayAgendaNavigatesBetweenDaysAndPreservesDetailReturn(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarView = calendarViewDay
	m.calendarDay = b.events[0].Start
	m.calendarDetail = m.selectedCalendarEvent()

	model, _ := m.handleKeyMsg(keyRunes("l"))
	m = model.(*Model)
	if m.calendarDay.Local().Day() != 25 {
		t.Fatalf("calendarDay = %s, want May 25", m.calendarDay)
	}
	if got := m.selectedCalendarEvent(); got == nil || got.Title != "Weekly planning" {
		t.Fatalf("selected event after next day = %#v, want Weekly planning", got)
	}

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Enter to open full detail from Day view")
	}
	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarDetailOpen {
		t.Fatal("expected Esc to close full detail")
	}
	if m.calendarView != calendarViewDay {
		t.Fatalf("calendarView = %q, want Day view after closing detail", m.calendarView)
	}
}

func TestCalendarWeekGridSwitchesFromAgendaAndRendersInspector(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = m.selectedCalendarEvent()

	model, _ := m.handleKeyMsg(keyRunes("w"))
	m = model.(*Model)
	if m.calendarView != calendarViewWeek {
		t.Fatalf("calendarView = %q, want %q", m.calendarView, calendarViewWeek)
	}
	if m.calendarWeekStart.Local().Day() != 24 {
		t.Fatalf("calendarWeekStart = %s, want week starting May 24", m.calendarWeekStart)
	}

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Week Time-Grid", "Sun May 24", "Mon May 25", "Design review", "Weekly planning", "Week Inspector", "Herald planning room", "Local", "Event TZ", "h/l: week", "d: day", "a: agenda"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("week grid missing %q:\n%s", want, rendered)
		}
	}
	if lower := strings.ToLower(rendered); strings.Contains(lower, "etag") || strings.Contains(lower, "oauth") || strings.Contains(lower, "caldav") {
		t.Fatalf("week grid exposed provider internals:\n%s", rendered)
	}
}

func TestCalendarWeekGridNavigatesWeeksAndPreservesDetailReturn(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = m.selectedCalendarEvent()

	model, _ := m.handleKeyMsg(keyRunes("w"))
	m = model.(*Model)
	model, _ = m.handleKeyMsg(keyRunes("l"))
	m = model.(*Model)
	if m.calendarWeekStart.Local().Day() != 31 {
		t.Fatalf("calendarWeekStart = %s, want May 31", m.calendarWeekStart)
	}
	if got := m.selectedCalendarEvent(); got == nil || got.Title != "Roadmap sync" {
		t.Fatalf("selected event after next week = %#v, want Roadmap sync", got)
	}

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Enter to open full detail from Week view")
	}
	detail := stripANSI(m.renderMainView())
	if !strings.Contains(detail, "Event Detail") || !strings.Contains(detail, "Roadmap sync") {
		t.Fatalf("detail view missing selected week event:\n%s", detail)
	}
	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarDetailOpen {
		t.Fatal("expected Esc to close full detail")
	}
	if m.calendarView != calendarViewWeek {
		t.Fatalf("calendarView = %q, want Week view after closing detail", m.calendarView)
	}

	model, _ = m.handleKeyMsg(keyRunes("d"))
	m = model.(*Model)
	if m.calendarView != calendarViewDay {
		t.Fatalf("calendarView = %q, want Day view", m.calendarView)
	}
	if m.calendarDay.Local().Day() != 31 {
		t.Fatalf("calendarDay = %s, want selected event day May 31", m.calendarDay)
	}
	model, _ = m.handleKeyMsg(keyRunes("w"))
	m = model.(*Model)
	if m.calendarView != calendarViewWeek || m.calendarWeekStart.Local().Day() != 31 {
		t.Fatalf("week view did not restore selected event week, view=%q start=%s", m.calendarView, m.calendarWeekStart)
	}
}

func TestCalendarWeekShortcutDoesNotStealTextEntry(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}

	t.Run("compose", func(t *testing.T) {
		m := New(b, nil, "", nil, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(*Model)
		m.loading = false
		m.activeTab = tabCompose
		m.composeField = composeFieldBody
		m.composeTo.Blur()
		m.composeBody.Focus()

		model, _ := m.handleKeyMsg(keyRunes("w"))
		m = model.(*Model)
		if m.activeTab != tabCompose {
			t.Fatalf("activeTab = %d, want Compose", m.activeTab)
		}
		if got := m.composeBody.Value(); got != "w" {
			t.Fatalf("compose body=%q, want literal w", got)
		}
	})

	t.Run("prompt", func(t *testing.T) {
		m := New(b, nil, "", nil, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(*Model)
		m.loading = false
		m.activeTab = tabTimeline
		m.timeline.searchMode = true
		m.timeline.searchFocus = timelineSearchFocusInput
		m.timeline.searchInput.Focus()

		model, _ := m.handleKeyMsg(keyRunes("w"))
		m = model.(*Model)
		if m.activeTab != tabTimeline {
			t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
		}
		if got := m.timeline.searchInput.Value(); got != "w" {
			t.Fatalf("timeline search=%q, want literal w", got)
		}
	})

	t.Run("editor", func(t *testing.T) {
		m := New(b, nil, "", nil, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(*Model)
		m.loading = false
		m.activeTab = tabTimeline
		m.showPromptEditor = true
		m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
		_ = m.promptEditor.Init()

		model, _ := m.Update(keyRunes("w"))
		m = model.(*Model)
		if !m.showPromptEditor {
			t.Fatal("prompt editor should remain active after typing w")
		}
		if got := m.promptEditor.name; got != "w" {
			t.Fatalf("prompt editor name=%q, want literal w", got)
		}
	})
}

func calendarImmediateMessagesForTest(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		messages := make([]tea.Msg, 0, len(batch))
		for _, child := range batch {
			if child == nil {
				continue
			}
			if childMsg := child(); childMsg != nil {
				messages = append(messages, childMsg)
			}
		}
		return messages
	}
	return []tea.Msg{msg}
}

func testCalendarEvents() []models.CalendarEvent {
	base := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	return []models.CalendarEvent{
		{
			Ref:         models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "design-review"}.WithDefaults(),
			Title:       "Design review",
			Description: "Review agenda layout with deterministic demo data.",
			Location:    "Herald planning room",
			Start:       base,
			End:         base.Add(time.Hour),
			Status:      "confirmed",
		},
		{
			Ref:         models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "daily-standup"}.WithDefaults(),
			Title:       "Daily standup",
			Description: "Walk the day plan and identify calendar conflicts.",
			Location:    "Huddle room",
			Start:       base.Add(90 * time.Minute),
			End:         base.Add(2 * time.Hour),
			Status:      "confirmed",
		},
		{
			Ref:         models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "weekly-planning"}.WithDefaults(),
			Title:       "Weekly planning",
			Description: "Read-only detail should preserve the agenda cursor.",
			Location:    "Video call",
			Start:       base.AddDate(0, 0, 1).Add(2 * time.Hour),
			End:         base.AddDate(0, 0, 1).Add(3 * time.Hour),
			Status:      "tentative",
		},
		{
			Ref:         models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "roadmap-sync"}.WithDefaults(),
			Title:       "Roadmap sync",
			Description: "Confirm week navigation keeps selected event detail stable.",
			Location:    "Planning call",
			Start:       base.AddDate(0, 0, 7).Add(time.Hour),
			End:         base.AddDate(0, 0, 7).Add(2 * time.Hour),
			Status:      "confirmed",
		},
	}
}
