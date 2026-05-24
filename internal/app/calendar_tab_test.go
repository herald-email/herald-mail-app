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
	m.calendarCursor = 1

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
	if m.calendarCursor != 1 {
		t.Fatalf("calendar cursor = %d, want preserved index 1", m.calendarCursor)
	}
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
			Ref:         models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "weekly-planning"}.WithDefaults(),
			Title:       "Weekly planning",
			Description: "Read-only detail should preserve the agenda cursor.",
			Location:    "Video call",
			Start:       base.Add(2 * time.Hour),
			End:         base.Add(3 * time.Hour),
			Status:      "tentative",
		},
	}
}
