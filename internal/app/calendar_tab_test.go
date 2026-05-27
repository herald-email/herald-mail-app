package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type calendarAgendaStubBackend struct {
	stubBackend
	available        bool
	events           []models.CalendarEvent
	crossResults     []models.CrossSourceSearchResult
	getCalls         int
	searchCalls      []string
	crossSearchCalls []string
	savedEvents      []models.CalendarEvent
	saveErr          error
	rsvpEvents       []models.EventRef
	rsvpStatuses     []string
	rsvpErr          error
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

func (b *calendarAgendaStubBackend) SearchCalendarEvents(query string, start, end time.Time) ([]models.CalendarEvent, error) {
	b.searchCalls = append(b.searchCalls, query)
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]models.CalendarEvent, 0, len(b.events))
	for _, event := range b.events {
		if !start.IsZero() && !event.End.IsZero() && !event.End.After(start) {
			continue
		}
		if !end.IsZero() && !event.Start.IsZero() && !event.Start.Before(end) {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			event.Title,
			event.Description,
			event.Location,
			event.Organizer,
			event.OrganizerEmail,
			event.RecurrenceSummary,
			string(event.Ref.SourceID),
			event.Ref.CalendarID,
		}, " "))
		for _, attendee := range event.Attendees {
			haystack += " " + strings.ToLower(attendee.Name+" "+attendee.Email+" "+attendee.RSVP)
		}
		for _, attachment := range event.Attachments {
			haystack += " " + strings.ToLower(attachment.Title+" "+attachment.MIMEType)
		}
		if query != "" && strings.Contains(haystack, query) {
			out = append(out, event)
		}
	}
	return out, nil
}

func (b *calendarAgendaStubBackend) CrossSourceSearch(query string) ([]models.CrossSourceSearchResult, error) {
	b.crossSearchCalls = append(b.crossSearchCalls, query)
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]models.CrossSourceSearchResult, 0, len(b.crossResults))
	for _, result := range b.crossResults {
		haystack := strings.ToLower(result.MatchHint + " ")
		if result.Email != nil {
			haystack += strings.ToLower(result.Email.Sender + " " + result.Email.Subject + " " + result.Email.Folder)
		}
		if result.Event != nil {
			haystack += models.CalendarEventSearchText(*result.Event)
		}
		if query != "" && strings.Contains(haystack, query) {
			out = append(out, result)
		}
	}
	return out, nil
}

func (b *calendarAgendaStubBackend) SaveCalendarEvent(event models.CalendarEvent) (*models.CalendarEvent, error) {
	if b.saveErr != nil {
		return nil, b.saveErr
	}
	event.Ref = event.Ref.WithDefaults()
	b.savedEvents = append(b.savedEvents, event)
	for i := range b.events {
		if b.events[i].Ref.WithDefaults().LocalID == event.Ref.LocalID {
			b.events[i] = event
			saved := event
			return &saved, nil
		}
	}
	saved := event
	b.events = append(b.events, event)
	return &saved, nil
}

func (b *calendarAgendaStubBackend) RespondCalendarEvent(ref models.EventRef, status string) (*models.CalendarEvent, error) {
	if b.rsvpErr != nil {
		return nil, b.rsvpErr
	}
	ref = ref.WithDefaults()
	b.rsvpEvents = append(b.rsvpEvents, ref)
	b.rsvpStatuses = append(b.rsvpStatuses, status)
	for i := range b.events {
		if b.events[i].Ref.WithDefaults().LocalID != ref.LocalID {
			continue
		}
		event := b.events[i]
		if len(event.Attendees) == 0 {
			event.Attendees = []models.CalendarAttendee{{Name: "Me", Email: "me@example.com"}}
		}
		event.Attendees[0].RSVP = status
		b.events[i] = event
		saved := event
		return &saved, nil
	}
	return nil, errors.New("event not found")
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

func TestCalendarSearchViewFiltersAndPreservesDetailReturn(t *testing.T) {
	rich := richCalendarEventForTest()
	rich.ProviderUID = "provider-secret"
	rich.Ref.ETag = `"provider-etag"`
	rich.Raw = `{"syncToken":"secret"}`
	events := append(testCalendarEvents(), rich)
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = events
	m.calendarDetail = m.selectedCalendarEvent()

	model, cmd := m.handleKeyMsg(keyRunes("/"))
	m = model.(*Model)
	if m.calendarView != calendarViewSearch {
		t.Fatalf("calendarView = %q, want search", m.calendarView)
	}
	if cmd != nil {
		for _, msg := range calendarImmediateMessagesForTest(cmd) {
			model, _ = m.Update(msg)
			m = model.(*Model)
		}
	}

	model, cmd = m.handleKeyMsg(keyRunes("Mina"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.searchCalls) == 0 || b.searchCalls[len(b.searchCalls)-1] != "Mina" {
		t.Fatalf("search calls = %#v, want Mina", b.searchCalls)
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Calendar Search", "/ Mina", "Timezone planning", "Mina Park", "read-only"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("calendar search missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"provider-secret", "provider-etag", "syncToken", "calendar.example"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("calendar search leaked provider internals %q:\n%s", forbidden, rendered)
		}
	}

	model, cmd = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Enter to open full event detail from search")
	}
	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarDetailOpen {
		t.Fatal("expected first Esc to close detail")
	}
	if m.calendarView != calendarViewSearch || m.calendarSearchQuery != "Mina" {
		t.Fatalf("search state after detail Esc view=%q query=%q", m.calendarView, m.calendarSearchQuery)
	}
	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarView != calendarViewAgenda || m.calendarSearchQuery != "" {
		t.Fatalf("second Esc should clear search to agenda, view=%q query=%q", m.calendarView, m.calendarSearchQuery)
	}
}

func TestCalendarSearchNoMatchesAndProviderInternalsHidden(t *testing.T) {
	rich := richCalendarEventForTest()
	rich.Raw = `{"syncToken":"secret"}`
	rich.Ref.ETag = `"provider-etag"`
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events

	model, cmd := m.handleKeyMsg(keyRunes("/"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	model, cmd = m.handleKeyMsg(keyRunes("Atlantis"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Calendar Search", "/ Atlantis", "No cached event matches"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("no-match search missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"provider-etag", "syncToken", "https://calendar.example", "RSVP", "Edit"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("no-match search leaked or advertised %q:\n%s", forbidden, rendered)
		}
	}
}

func TestCrossSourceSearchViewBlendsMailAndCalendarResults(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := richCalendarEventForTest()
	event.Title = "Launch planning"
	event.Description = "Discuss the product launch plan."
	event.Start = start
	event.End = start.Add(time.Hour)
	event.ProviderUID = "provider-secret"
	event.Ref.ETag = `"provider-etag"`
	event.Raw = `{"syncToken":"secret"}`
	mail := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "mail-planning",
		UID:       42,
		Folder:    "INBOX",
		Sender:    "mina@example.com",
		Subject:   "Launch planning memo",
		Date:      start.Add(-time.Hour),
	}
	b := &calendarAgendaStubBackend{
		available: true,
		events:    []models.CalendarEvent{event},
		crossResults: []models.CrossSourceSearchResult{
			{Kind: models.CrossSourceResultMail, Email: mail, When: mail.Date, MatchHint: "subject"},
			{Kind: models.CrossSourceResultEvent, Event: &event, When: event.Start, MatchHint: "title"},
		},
	}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = m.selectedCalendarEvent()

	model, cmd := m.handleKeyMsg(keyRunes("x"))
	m = model.(*Model)
	if m.calendarView != calendarViewCrossSearch {
		t.Fatalf("calendarView = %q, want cross-source search", m.calendarView)
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}

	model, cmd = m.handleKeyMsg(keyRunes("planning"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.crossSearchCalls) == 0 || b.crossSearchCalls[len(b.crossSearchCalls)-1] != "planning" {
		t.Fatalf("cross search calls = %#v, want planning", b.crossSearchCalls)
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Cross-Source Search", "mail", "event", "Launch planning memo", "Launch planning", "mina@example.com", "read-only"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("cross-source search missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"provider-secret", "provider-etag", "syncToken", "OAuth", "Edit"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("cross-source search leaked or advertised %q:\n%s", forbidden, rendered)
		}
	}

	model, _ = m.handleKeyMsg(keyRunes("j"))
	m = model.(*Model)
	moved := stripANSI(m.renderMainView())
	if moved == rendered {
		t.Fatalf("expected j to move cross-source detail selection:\n%s", moved)
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarView != calendarViewAgenda || m.crossSourceSearchQuery != "" {
		t.Fatalf("Esc should clear cross-source search to agenda, view=%q query=%q", m.calendarView, m.crossSourceSearchQuery)
	}
}

func TestCrossSourceSearchDoesNotReplaceCalendarSearchOrAcceptStaleResults(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := richCalendarEventForTest()
	event.Title = "Launch planning"
	event.Start = start
	mail := &models.EmailData{
		MessageID: "mail-planning",
		Folder:    "INBOX",
		Sender:    "mina@example.com",
		Subject:   "Launch planning memo",
		Date:      start.Add(-time.Hour),
	}
	b := &calendarAgendaStubBackend{
		available: true,
		events:    []models.CalendarEvent{event},
		crossResults: []models.CrossSourceSearchResult{
			{Kind: models.CrossSourceResultMail, Email: mail, When: mail.Date, MatchHint: "subject"},
			{Kind: models.CrossSourceResultEvent, Event: &event, When: event.Start, MatchHint: "title"},
		},
	}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events

	model, cmd := m.handleKeyMsg(keyRunes("/"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	model, cmd = m.handleKeyMsg(keyRunes("planning"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if m.calendarView != calendarViewSearch {
		t.Fatalf("calendar search view = %q, want event-only search", m.calendarView)
	}
	if len(m.calendarSearchResults) != 1 {
		t.Fatalf("calendar search results = %#v, want one event-only result", m.calendarSearchResults)
	}
	if len(b.crossSearchCalls) != 0 {
		t.Fatalf("calendar search should not call cross-source search: %#v", b.crossSearchCalls)
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	model, cmd = m.handleKeyMsg(keyRunes("x"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	m.crossSourceSearchQuery = "newer"
	stale := CrossSourceSearchLoadedMsg{
		Query: "older",
		Results: []models.CrossSourceSearchResult{
			{Kind: models.CrossSourceResultMail, Email: mail, When: mail.Date},
		},
	}
	model, _ = m.Update(stale)
	m = model.(*Model)
	if len(m.crossSourceSearchResults) != 0 {
		t.Fatalf("stale cross-source results repainted newer query: %#v", m.crossSourceSearchResults)
	}
}

func TestCrossSourceSearchShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("x"))
		m = model.(*Model)
		if m.activeTab != tabCompose {
			t.Fatalf("activeTab = %d, want Compose", m.activeTab)
		}
		if got := m.composeBody.Value(); got != "x" {
			t.Fatalf("compose body=%q, want literal x", got)
		}
	})

	t.Run("timeline prompt", func(t *testing.T) {
		m := New(b, nil, "", nil, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(*Model)
		m.loading = false
		m.activeTab = tabTimeline
		m.timeline.searchMode = true
		m.timeline.searchFocus = timelineSearchFocusInput
		m.timeline.searchInput.Focus()

		model, _ := m.handleKeyMsg(keyRunes("x"))
		m = model.(*Model)
		if m.activeTab != tabTimeline {
			t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
		}
		if got := m.timeline.searchInput.Value(); got != "x" {
			t.Fatalf("timeline search=%q, want literal x", got)
		}
	})

	t.Run("calendar search", func(t *testing.T) {
		m := New(b, nil, "", nil, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(*Model)
		m.loading = false
		m.activeTab = tabCalendar
		m.calendarEvents = b.events
		m.openCalendarSearch()

		model, cmd := m.handleKeyMsg(keyRunes("x"))
		m = model.(*Model)
		for _, msg := range calendarImmediateMessagesForTest(cmd) {
			model, _ = m.Update(msg)
			m = model.(*Model)
		}
		if m.calendarView != calendarViewSearch {
			t.Fatalf("calendarView = %q, want Calendar Search", m.calendarView)
		}
		if m.calendarSearchQuery != "x" {
			t.Fatalf("calendar search query=%q, want literal x", m.calendarSearchQuery)
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

		model, _ := m.Update(keyRunes("x"))
		m = model.(*Model)
		if !m.showPromptEditor {
			t.Fatal("prompt editor should remain active after typing x")
		}
		if got := m.promptEditor.name; got != "x" {
			t.Fatalf("prompt editor name=%q, want literal x", got)
		}
	})
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

func TestCalendarThreeDayCommandSwitchesFromAgendaAndRendersPanel(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = m.selectedCalendarEvent()

	model, _ := m.handleKeyMsg(keyRunes("t"))
	m = model.(*Model)
	if m.calendarView != calendarViewThreeDay {
		t.Fatalf("calendarView = %q, want %q", m.calendarView, calendarViewThreeDay)
	}
	if m.calendarThreeDayStart.Local().Day() != 24 {
		t.Fatalf("calendarThreeDayStart = %s, want May 24", m.calendarThreeDayStart)
	}

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"3-Day Command", "Sun May 24", "Mon May 25", "Tue May 26", "Design review", "Weekly planning", "Command Panel", "Next Up", "Open Slots", "Conflicts", "Mode", "read-only", "h/l: 3-day", "w: week", "d: day", "a: agenda"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("3-day command missing %q:\n%s", want, rendered)
		}
	}
	if lower := strings.ToLower(rendered); strings.Contains(lower, "etag") || strings.Contains(lower, "oauth") || strings.Contains(lower, "caldav") {
		t.Fatalf("3-day command exposed provider internals:\n%s", rendered)
	}
}

func TestCalendarThreeDayCommandSlidesWindowAndPreservesDetailReturn(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = m.selectedCalendarEvent()

	model, _ := m.handleKeyMsg(keyRunes("t"))
	m = model.(*Model)
	model, _ = m.handleKeyMsg(keyRunes("l"))
	m = model.(*Model)
	if m.calendarThreeDayStart.Local().Day() != 25 {
		t.Fatalf("calendarThreeDayStart = %s, want May 25", m.calendarThreeDayStart)
	}
	if got := m.selectedCalendarEvent(); got == nil || got.Title != "Weekly planning" {
		t.Fatalf("selected event after sliding 3-day window = %#v, want Weekly planning", got)
	}

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Enter to open full detail from 3-Day view")
	}
	detail := stripANSI(m.renderMainView())
	if !strings.Contains(detail, "Event Detail") || !strings.Contains(detail, "Weekly planning") {
		t.Fatalf("detail view missing selected 3-day event:\n%s", detail)
	}
	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarDetailOpen {
		t.Fatal("expected Esc to close full detail")
	}
	if m.calendarView != calendarViewThreeDay {
		t.Fatalf("calendarView = %q, want 3-Day view after closing detail", m.calendarView)
	}

	model, _ = m.handleKeyMsg(keyRunes("w"))
	m = model.(*Model)
	if m.calendarView != calendarViewWeek {
		t.Fatalf("calendarView = %q, want Week view", m.calendarView)
	}
	model, _ = m.handleKeyMsg(keyRunes("t"))
	m = model.(*Model)
	if m.calendarView != calendarViewThreeDay || m.calendarThreeDayStart.Local().Day() != 25 {
		t.Fatalf("3-day view did not restore selected event window, view=%q start=%s", m.calendarView, m.calendarThreeDayStart)
	}
}

func TestCalendarFullEventDetailRendersRichMetadataAndTimezones(t *testing.T) {
	rich := richCalendarEventForTest()
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{
		"Event Detail",
		"Timezone planning",
		"Local",
		"Event TZ",
		"America/Los_Angeles",
		"Asia/Tokyo",
		"Organizer",
		"Mina Park <mina@example.com>",
		"Attendees",
		"Rae Stone <rae@example.com> accepted",
		"Noor Patel <noor@example.com> tentative optional",
		"Recurrence",
		"Weekly on Monday",
		"Attachments",
		"Agenda (application/pdf)",
		"Scope",
		"this event",
		"Mode",
		"provider-backed edit/RSVP",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rich event detail missing %q:\n%s", want, rendered)
		}
	}
	lower := strings.ToLower(rendered)
	for _, forbidden := range []string{"etag", "oauth", "caldav", "sync token", "https://calendar.example"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("rich event detail leaked provider internals %q:\n%s", forbidden, rendered)
		}
	}
}

func TestCalendarEventEditOpensFromDetailAndRendersTimezonePreview(t *testing.T) {
	rich := richCalendarEventForTest()
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	model, _ := m.handleKeyMsg(keyRunes("e"))
	m = model.(*Model)
	if !m.calendarEdit.Active {
		t.Fatal("expected e from Event Detail to open Event Edit")
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{
		"Event Edit",
		"Title",
		"Timezone planning",
		"Start",
		"End",
		"Event TZ",
		"America/Los_Angeles",
		"Attendees",
		"Rae Stone <rae@example.com> accepted",
		"Recurrence",
		"RRULE:FREQ=WEEKLY;BYDAY=MO",
		"Reminders",
		"popup 30m",
		"Alt TZ",
		"Asia/Tokyo",
		"Preview",
		"Alt TZ rows are preview only; Event TZ saves.",
		"ctrl+s: save",
		"esc: cancel",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("event edit missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"provider-etag", "syncToken", "RSVP", "Create event"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("event edit leaked or advertised %q:\n%s", forbidden, rendered)
		}
	}
}

func TestCalendarEventEditSaveUpdatesCachedEventAndReturnsToDetail(t *testing.T) {
	rich := richCalendarEventForTest()
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	model, _ := m.handleKeyMsg(keyRunes("e"))
	m = model.(*Model)
	m.calendarEdit.Draft.Title = "Timezone planning moved"
	m.calendarEdit.Draft.Location = "Tokyo room"
	m.calendarEdit.Draft.StartText = "2026-05-24 19:30"
	m.calendarEdit.Draft.EndText = "2026-05-24 20:30"
	m.calendarEdit.Draft.TimeZone = "America/Los_Angeles"
	m.calendarEdit.Draft.AttendeesText = "Mina Park <mina@example.com> accepted; ops@example.com tentative optional"
	m.calendarEdit.Draft.RecurrenceText = "RRULE:FREQ=WEEKLY;BYDAY=TU,TH"
	m.calendarEdit.Draft.RemindersText = "popup 10m; email 1h"
	m.calendarEdit.Dirty = true

	model, cmd := m.handleKeyMsg(keyCtrl('s'))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if m.calendarEdit.Active {
		t.Fatal("expected successful save to close Event Edit")
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected successful save to return to Event Detail")
	}
	if len(b.savedEvents) != 1 {
		t.Fatalf("saved events = %#v, want one cache-backed save", b.savedEvents)
	}
	if m.calendarDetail == nil || m.calendarDetail.Title != "Timezone planning moved" || m.calendarDetail.Location != "Tokyo room" {
		t.Fatalf("calendar detail = %#v, want saved event", m.calendarDetail)
	}
	if len(m.calendarDetail.Attendees) != 2 || m.calendarDetail.Attendees[0].Email != "mina@example.com" || !m.calendarDetail.Attendees[1].Optional {
		t.Fatalf("calendar detail attendees = %#v, want edited attendees", m.calendarDetail.Attendees)
	}
	if got := m.calendarDetail.RecurrenceSummary; got != "Weekly on Tuesday, Thursday" {
		t.Fatalf("recurrence summary = %q, want edited recurrence summary", got)
	}
	if len(m.calendarDetail.Reminders) != 2 || m.calendarDetail.Reminders[0].MinutesBefore != 10 || m.calendarDetail.Reminders[1].Method != "email" {
		t.Fatalf("calendar detail reminders = %#v, want edited reminders", m.calendarDetail.Reminders)
	}
	if got := m.calendarEvents[0].Title; got != "Timezone planning moved" {
		t.Fatalf("calendar list title = %q, want saved title", got)
	}
	if !strings.Contains(m.calendarStatus, "Saved") {
		t.Fatalf("calendar status = %q, want save success", m.calendarStatus)
	}
}

func TestCalendarEventEditProviderFailureKeepsEditorOpen(t *testing.T) {
	rich := richCalendarEventForTest()
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}, saveErr: errors.New("provider save failed")}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	model, _ := m.handleKeyMsg(keyRunes("e"))
	m = model.(*Model)
	m.calendarEdit.Draft.Title = "Unsaved provider title"
	m.calendarEdit.Dirty = true
	model, cmd := m.handleKeyMsg(keyCtrl('s'))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if !m.calendarEdit.Active {
		t.Fatal("expected provider failure to keep Event Edit open")
	}
	if m.calendarEdit.Draft.Title != "Unsaved provider title" {
		t.Fatalf("draft title = %q, want unsaved value preserved", m.calendarEdit.Draft.Title)
	}
	if m.calendarEvents[0].Title == "Unsaved provider title" {
		t.Fatalf("calendar list updated after provider failure: %#v", m.calendarEvents[0])
	}
	if !strings.Contains(m.calendarEdit.Error, "Save failed") {
		t.Fatalf("calendar edit error = %q, want save failure", m.calendarEdit.Error)
	}
}

func TestCalendarEventEditProviderConflictNamesConflictAndKeepsEditorOpen(t *testing.T) {
	rich := richCalendarEventForTest()
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}, saveErr: models.ErrCalendarMutationConflict}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	model, _ := m.handleKeyMsg(keyRunes("e"))
	m = model.(*Model)
	m.calendarEdit.Draft.Title = "Unsaved conflict title"
	m.calendarEdit.Dirty = true
	model, cmd := m.handleKeyMsg(keyCtrl('s'))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if !m.calendarEdit.Active {
		t.Fatal("expected provider conflict to keep Event Edit open")
	}
	if m.calendarEdit.Draft.Title != "Unsaved conflict title" {
		t.Fatalf("draft title = %q, want unsaved value preserved", m.calendarEdit.Draft.Title)
	}
	if m.calendarEvents[0].Title == "Unsaved conflict title" {
		t.Fatalf("calendar list updated after provider conflict: %#v", m.calendarEvents[0])
	}
	if !strings.Contains(strings.ToLower(m.calendarStatus), "conflict") {
		t.Fatalf("calendar status = %q, want conflict", m.calendarStatus)
	}
}

func TestCalendarEventRSVPShortcutUpdatesAttendeeAndDetail(t *testing.T) {
	rich := richCalendarEventForTest()
	rich.Attendees[0].RSVP = "needs-action"
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	model, cmd := m.handleKeyMsg(keyRunes("v"))
	m = model.(*Model)
	if cmd == nil {
		t.Fatal("expected RSVP shortcut to produce mutation command")
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.rsvpEvents) != 1 || len(b.rsvpStatuses) != 1 || b.rsvpStatuses[0] != "accepted" {
		t.Fatalf("RSVP calls refs=%#v statuses=%#v, want accepted response", b.rsvpEvents, b.rsvpStatuses)
	}
	if m.calendarDetail == nil || len(m.calendarDetail.Attendees) == 0 || m.calendarDetail.Attendees[0].RSVP != "accepted" {
		t.Fatalf("calendar detail attendees = %#v, want accepted RSVP", m.calendarDetail)
	}
	if !strings.Contains(m.calendarStatus, "Saved RSVP accepted") {
		t.Fatalf("calendar status = %q, want RSVP success", m.calendarStatus)
	}
}

func TestCalendarEventRSVPConflictLeavesCachedAttendeeUnchanged(t *testing.T) {
	rich := richCalendarEventForTest()
	rich.Attendees[0].RSVP = "tentative"
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}, rsvpErr: models.ErrCalendarMutationConflict}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	model, cmd := m.handleKeyMsg(keyRunes("v"))
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if m.calendarDetail == nil || len(m.calendarDetail.Attendees) == 0 || m.calendarDetail.Attendees[0].RSVP != "tentative" {
		t.Fatalf("calendar detail attendees = %#v, want unchanged tentative RSVP", m.calendarDetail)
	}
	if !strings.Contains(strings.ToLower(m.calendarStatus), "conflict") {
		t.Fatalf("calendar status = %q, want conflict", m.calendarStatus)
	}
}

func TestCalendarEventEditValidationKeepsEditorOpen(t *testing.T) {
	rich := richCalendarEventForTest()
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{rich}}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	model, _ := m.handleKeyMsg(keyRunes("e"))
	m = model.(*Model)
	m.calendarEdit.Draft.StartText = "2026-05-24 21:00"
	m.calendarEdit.Draft.EndText = "2026-05-24 20:00"
	model, cmd := m.handleKeyMsg(keyCtrl('s'))
	m = model.(*Model)
	if cmd != nil {
		t.Fatalf("invalid edit produced command %T, want local validation only", cmd)
	}
	if !m.calendarEdit.Active {
		t.Fatal("expected validation failure to keep Event Edit open")
	}
	if !strings.Contains(m.calendarEdit.Error, "end") {
		t.Fatalf("calendar edit error = %q, want end-time validation", m.calendarEdit.Error)
	}
	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "Validation") || !strings.Contains(rendered, "end") {
		t.Fatalf("event edit validation not rendered:\n%s", rendered)
	}
}

func TestCalendarEditShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("e"))
		m = model.(*Model)
		if m.activeTab != tabCompose {
			t.Fatalf("activeTab = %d, want Compose", m.activeTab)
		}
		if got := m.composeBody.Value(); got != "e" {
			t.Fatalf("compose body=%q, want literal e", got)
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

		model, _ := m.handleKeyMsg(keyRunes("e"))
		m = model.(*Model)
		if m.activeTab != tabTimeline {
			t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
		}
		if got := m.timeline.searchInput.Value(); got != "e" {
			t.Fatalf("timeline search=%q, want literal e", got)
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

		model, _ := m.Update(keyRunes("e"))
		m = model.(*Model)
		if !m.showPromptEditor {
			t.Fatal("prompt editor should remain active after typing e")
		}
		if got := m.promptEditor.name; got != "e" {
			t.Fatalf("prompt editor name=%q, want literal e", got)
		}
	})
}

func TestCalendarRSVPShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("v"))
		m = model.(*Model)
		if m.activeTab != tabCompose {
			t.Fatalf("activeTab = %d, want Compose", m.activeTab)
		}
		if got := m.composeBody.Value(); got != "v" {
			t.Fatalf("compose body=%q, want literal v", got)
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

		model, _ := m.handleKeyMsg(keyRunes("v"))
		m = model.(*Model)
		if m.activeTab != tabTimeline {
			t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
		}
		if got := m.timeline.searchInput.Value(); got != "v" {
			t.Fatalf("timeline search=%q, want literal v", got)
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

		model, _ := m.Update(keyRunes("v"))
		m = model.(*Model)
		if !m.showPromptEditor {
			t.Fatal("prompt editor should remain active after typing v")
		}
		if got := m.promptEditor.name; got != "v" {
			t.Fatalf("prompt editor name=%q, want literal v", got)
		}
	})
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

func TestCalendarThreeDayShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("t"))
		m = model.(*Model)
		if m.activeTab != tabCompose {
			t.Fatalf("activeTab = %d, want Compose", m.activeTab)
		}
		if got := m.composeBody.Value(); got != "t" {
			t.Fatalf("compose body=%q, want literal t", got)
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

		model, _ := m.handleKeyMsg(keyRunes("t"))
		m = model.(*Model)
		if m.activeTab != tabTimeline {
			t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
		}
		if got := m.timeline.searchInput.Value(); got != "t" {
			t.Fatalf("timeline search=%q, want literal t", got)
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

		model, _ := m.Update(keyRunes("t"))
		m = model.(*Model)
		if !m.showPromptEditor {
			t.Fatal("prompt editor should remain active after typing t")
		}
		if got := m.promptEditor.name; got != "t" {
			t.Fatalf("prompt editor name=%q, want literal t", got)
		}
	})
}

func TestCalendarSearchShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("/"))
		m = model.(*Model)
		if m.activeTab != tabCompose {
			t.Fatalf("activeTab = %d, want Compose", m.activeTab)
		}
		if got := m.composeBody.Value(); got != "/" {
			t.Fatalf("compose body=%q, want literal /", got)
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

		model, _ := m.handleKeyMsg(keyRunes("/"))
		m = model.(*Model)
		if m.activeTab != tabTimeline {
			t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
		}
		if got := m.timeline.searchInput.Value(); got != "/" {
			t.Fatalf("timeline search=%q, want literal /", got)
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

		model, _ := m.Update(keyRunes("/"))
		m = model.(*Model)
		if !m.showPromptEditor {
			t.Fatal("prompt editor should remain active after typing /")
		}
		if got := m.promptEditor.name; got != "/" {
			t.Fatalf("prompt editor name=%q, want literal /", got)
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

func richCalendarEventForTest() models.CalendarEvent {
	loc := time.FixedZone("PDT", -7*60*60)
	start := time.Date(2026, 5, 24, 18, 30, 0, 0, loc)
	return models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "demo-calendar",
			AccountID:  "default",
			CalendarID: "work",
			EventID:    "timezone-planning",
		}.WithDefaults(),
		Title:              "Timezone planning",
		Description:        "Review attendee status before editing is enabled.",
		Location:           "Video call",
		Start:              start,
		End:                start.Add(time.Hour),
		TimeZone:           "America/Los_Angeles",
		Status:             "confirmed",
		Organizer:          "Mina Park",
		OrganizerEmail:     "mina@example.com",
		Recurrence:         []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
		RecurrenceSummary:  "Weekly on Monday",
		AlternateTimeZones: []string{"Asia/Tokyo"},
		Attendees: []models.CalendarAttendee{
			{Name: "Rae Stone", Email: "rae@example.com", RSVP: "accepted"},
			{Name: "Noor Patel", Email: "noor@example.com", RSVP: "tentative", Optional: true},
		},
		Attachments: []models.CalendarAttachment{
			{Title: "Agenda", URI: "https://calendar.example/agenda.pdf", MIMEType: "application/pdf"},
		},
		Reminders: []models.CalendarReminder{
			{Method: "popup", MinutesBefore: 30},
		},
	}
}
