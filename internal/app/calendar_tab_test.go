package app

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type calendarAgendaStubBackend struct {
	stubBackend
	available             bool
	events                []models.CalendarEvent
	cachedEvents          []models.CalendarEvent
	refreshEvents         []models.CalendarEvent
	collections           []models.CalendarCollection
	cachedCollections     []models.CalendarCollection
	refreshCollections    []models.CalendarCollection
	crossResults          []models.CrossSourceSearchResult
	meetingPrep           *models.CalendarMeetingPrep
	meetingPrepCalls      []models.EventRef
	travelBuffer          *models.CalendarTravelBuffer
	travelCalls           []models.EventRef
	aiSummary             *models.CalendarAISummary
	aiSummaryCalls        []models.EventRef
	getCalls              int
	searchCalls           []string
	crossSearchCalls      []string
	savedEvents           []models.CalendarEvent
	saveErr               error
	rsvpEvents            []models.EventRef
	rsvpStatuses          []string
	rsvpErr               error
	cachedAgendaCalls     int
	cachedCollectionCalls int
	refreshAgendaCalls    int
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

func (b *calendarAgendaStubBackend) ListCalendarCollections() ([]models.CalendarCollection, error) {
	out := make([]models.CalendarCollection, len(b.collections))
	copy(out, b.collections)
	return out, nil
}

func (b *calendarAgendaStubBackend) ListCachedCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, error) {
	b.cachedAgendaCalls++
	events := b.cachedEvents
	if events == nil {
		events = b.events
	}
	return filterCalendarEventsForTest(events, start, end), nil
}

func (b *calendarAgendaStubBackend) ListCachedCalendarCollections() ([]models.CalendarCollection, error) {
	b.cachedCollectionCalls++
	collections := b.cachedCollections
	if collections == nil {
		collections = b.collections
	}
	out := make([]models.CalendarCollection, len(collections))
	copy(out, collections)
	return out, nil
}

func (b *calendarAgendaStubBackend) RefreshCalendarAgenda(start, end time.Time) ([]models.CalendarEvent, []models.CalendarCollection, error) {
	b.refreshAgendaCalls++
	events := b.refreshEvents
	if events == nil {
		events = b.events
	}
	collections := b.refreshCollections
	if collections == nil {
		collections = b.collections
	}
	outCollections := make([]models.CalendarCollection, len(collections))
	copy(outCollections, collections)
	return filterCalendarEventsForTest(events, start, end), outCollections, nil
}

func filterCalendarEventsForTest(events []models.CalendarEvent, start, end time.Time) []models.CalendarEvent {
	out := make([]models.CalendarEvent, 0, len(events))
	for _, event := range events {
		if !start.IsZero() && !event.End.IsZero() && !event.End.After(start) {
			continue
		}
		if !end.IsZero() && !event.Start.IsZero() && !event.Start.Before(end) {
			continue
		}
		out = append(out, event)
	}
	return out
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

func (b *calendarAgendaStubBackend) BuildCalendarMeetingPrep(event models.CalendarEvent) (models.CalendarMeetingPrep, error) {
	event.Ref = event.Ref.WithDefaults()
	b.meetingPrepCalls = append(b.meetingPrepCalls, event.Ref)
	if b.meetingPrep != nil {
		prep := *b.meetingPrep
		prep.Event.Ref = prep.Event.Ref.WithDefaults()
		return prep, nil
	}
	return models.CalendarMeetingPrep{Event: event, QueryTerms: models.CalendarMeetingPrepQueries(event)}, nil
}

func (b *calendarAgendaStubBackend) BuildCalendarTravelBuffer(event models.CalendarEvent) (models.CalendarTravelBuffer, error) {
	event.Ref = event.Ref.WithDefaults()
	b.travelCalls = append(b.travelCalls, event.Ref)
	if b.travelBuffer != nil {
		buffer := *b.travelBuffer
		buffer.Event.Ref = buffer.Event.Ref.WithDefaults()
		return buffer, nil
	}
	return models.CalendarTravelBuffer{Event: event, QueryTerms: models.CalendarTravelBufferQueries(event)}, nil
}

func (b *calendarAgendaStubBackend) BuildCalendarAISummary(event models.CalendarEvent) (models.CalendarAISummary, error) {
	event.Ref = event.Ref.WithDefaults()
	b.aiSummaryCalls = append(b.aiSummaryCalls, event.Ref)
	if b.aiSummary != nil {
		summary := *b.aiSummary
		summary.Event.Ref = summary.Event.Ref.WithDefaults()
		return summary, nil
	}
	return models.CalendarAISummary{Event: event, QueryTerms: models.CalendarAISummaryQueries(event)}, nil
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

func TestCalendarMeetingPrepOpensFromDetailWithRelatedCachedMail(t *testing.T) {
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
	nearby := event
	nearby.Title = "Launch retro"
	nearby.Ref.EventID = "launch-retro"
	nearby.Ref.LocalID = ""
	nearby.Ref = nearby.Ref.WithDefaults()
	nearby.Start = start.Add(2 * time.Hour)
	nearby.End = nearby.Start.Add(time.Hour)
	prep := &models.CalendarMeetingPrep{
		Event:         event,
		QueryTerms:    []string{"Launch planning", "mina@example.com"},
		RelatedMail:   []*models.EmailData{mail},
		RelatedEvents: []models.CalendarEvent{nearby},
	}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event, nearby}, meetingPrep: prep}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &event
	m.calendarDetailOpen = true

	model, cmd := m.handleKeyMsg(keyRunes("p"))
	m = model.(*Model)
	if !m.calendarMeetingPrepOpen || !m.calendarMeetingPrepLoading {
		t.Fatalf("meeting prep state open=%v loading=%v, want open loading", m.calendarMeetingPrepOpen, m.calendarMeetingPrepLoading)
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.meetingPrepCalls) != 1 || b.meetingPrepCalls[0].LocalID != event.Ref.WithDefaults().LocalID {
		t.Fatalf("meeting prep calls = %#v, want selected event ref", b.meetingPrepCalls)
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{
		"Meeting Prep",
		"read-only cached context",
		"Launch planning",
		"Related Mail",
		"Launch planning memo",
		"mina@example.com",
		"Nearby Events",
		"Launch retro",
		"Query Terms",
		"Launch planning",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("meeting prep missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"provider-secret", "provider-etag", "syncToken", "OAuth", "Save", "Edit"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("meeting prep leaked or advertised %q:\n%s", forbidden, rendered)
		}
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarMeetingPrepOpen {
		t.Fatal("expected Esc to close meeting prep")
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Esc from meeting prep to return to Event Detail")
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

func TestCalendarMeetingPrepShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("p"))
		m = model.(*Model)
		if got := m.composeBody.Value(); got != "p" {
			t.Fatalf("compose body=%q, want literal p", got)
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

		model, cmd := m.handleKeyMsg(keyRunes("p"))
		m = model.(*Model)
		for _, msg := range calendarImmediateMessagesForTest(cmd) {
			model, _ = m.Update(msg)
			m = model.(*Model)
		}
		if m.calendarSearchQuery != "p" {
			t.Fatalf("calendar search query=%q, want literal p", m.calendarSearchQuery)
		}
		if m.calendarMeetingPrepOpen {
			t.Fatal("meeting prep should not open while typing in calendar search")
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

		model, _ := m.handleKeyMsg(keyRunes("p"))
		m = model.(*Model)
		if m.activeTab != tabTimeline {
			t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
		}
		if got := m.timeline.searchInput.Value(); got != "p" {
			t.Fatalf("timeline search=%q, want literal p", got)
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

		model, _ := m.Update(keyRunes("p"))
		m = model.(*Model)
		if got := m.promptEditor.name; got != "p" {
			t.Fatalf("prompt editor name=%q, want literal p", got)
		}
	})
}

func TestCalendarTravelBufferOpensFromDetailWithCachedTravelMail(t *testing.T) {
	start := time.Date(2026, 5, 24, 14, 0, 0, 0, time.UTC)
	event := richCalendarEventForTest()
	event.Title = "Partner onsite"
	event.Description = "Discuss partner rollout."
	event.Location = "SFO Terminal 2"
	event.Start = start
	event.End = start.Add(time.Hour)
	event.ProviderUID = "provider-secret"
	event.Ref.ETag = `"provider-etag"`
	event.Raw = `{"syncToken":"secret"}`
	mail := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "flight-itinerary",
		UID:       42,
		Folder:    "Travel",
		Sender:    "airline@example.com",
		Subject:   "Flight itinerary for SFO",
		Date:      start.Add(-24 * time.Hour),
	}
	nearby := event
	nearby.Title = "Team sync"
	nearby.Location = "Downtown office"
	nearby.Ref.EventID = "team-sync"
	nearby.Ref.LocalID = ""
	nearby.Ref = nearby.Ref.WithDefaults()
	nearby.Start = start.Add(-45 * time.Minute)
	nearby.End = start.Add(-15 * time.Minute)
	buffer := &models.CalendarTravelBuffer{
		Event:        event,
		QueryTerms:   []string{"Partner onsite", "SFO Terminal 2", "flight"},
		RelatedMail:  []*models.EmailData{mail},
		NearbyEvents: []models.CalendarEvent{nearby},
		Recommendations: []models.CalendarTravelBufferRecommendation{
			{Label: "Arrive early", Window: "90 min before", Reason: "Flight itinerary for SFO"},
			{Label: "Tight nearby gap", Window: "15 min", Reason: "Team sync ends before this event"},
		},
	}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event, nearby}, travelBuffer: buffer}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &event
	m.calendarDetailOpen = true

	model, cmd := m.handleKeyMsg(keyRunes("b"))
	m = model.(*Model)
	if !m.calendarTravelBufferOpen || !m.calendarTravelBufferLoading {
		t.Fatalf("travel buffer state open=%v loading=%v, want open loading", m.calendarTravelBufferOpen, m.calendarTravelBufferLoading)
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.travelCalls) != 1 || b.travelCalls[0].LocalID != event.Ref.WithDefaults().LocalID {
		t.Fatalf("travel buffer calls = %#v, want selected event ref", b.travelCalls)
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{
		"Travel Buffer",
		"read-only cached travel context",
		"Partner onsite",
		"Buffer Suggestions",
		"Arrive early",
		"Flight itinerary for SFO",
		"Travel Mail",
		"airline@example.com",
		"Nearby Gaps",
		"Team sync",
		"Query Terms",
		"SFO Terminal 2",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("travel buffer missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"provider-secret", "provider-etag", "syncToken", "OAuth", "Save", "Edit"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("travel buffer leaked or advertised %q:\n%s", forbidden, rendered)
		}
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarTravelBufferOpen {
		t.Fatal("expected Esc to close travel buffer")
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Esc from travel buffer to return to Event Detail")
	}
}

func TestCalendarTravelBufferShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("b"))
		m = model.(*Model)
		if got := m.composeBody.Value(); got != "b" {
			t.Fatalf("compose body=%q, want literal b", got)
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

		model, cmd := m.handleKeyMsg(keyRunes("b"))
		m = model.(*Model)
		for _, msg := range calendarImmediateMessagesForTest(cmd) {
			model, _ = m.Update(msg)
			m = model.(*Model)
		}
		if m.calendarSearchQuery != "b" {
			t.Fatalf("calendar search query=%q, want literal b", m.calendarSearchQuery)
		}
		if m.calendarTravelBufferOpen {
			t.Fatal("travel buffer should not open while typing in calendar search")
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

		model, _ := m.Update(keyRunes("b"))
		m = model.(*Model)
		if got := m.promptEditor.name; got != "b" {
			t.Fatalf("prompt editor name=%q, want literal b", got)
		}
	})
}

func TestCalendarAISummaryOpensFromDetailWithCachedContext(t *testing.T) {
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
		Subject:   "Launch planning risks",
		Date:      start.Add(-time.Hour),
	}
	nearby := event
	nearby.Title = "Launch retro"
	nearby.Ref.EventID = "launch-retro"
	nearby.Ref.LocalID = ""
	nearby.Ref = nearby.Ref.WithDefaults()
	nearby.Start = start.Add(2 * time.Hour)
	nearby.End = nearby.Start.Add(time.Hour)
	summary := &models.CalendarAISummary{
		Event:        event,
		QueryTerms:   []string{"Launch planning", "mina@example.com"},
		RelatedMail:  []*models.EmailData{mail},
		NearbyEvents: []models.CalendarEvent{nearby},
		Bullets: []string{
			"Mina flagged launch risk in cached mail.",
			"Launch retro follows this event.",
		},
		ActionItems: []string{"Review Launch planning risks before the event."},
		GeneratedBy: models.CalendarAISummaryGeneratedByAI,
	}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event, nearby}, aiSummary: summary}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events
	m.calendarDetail = &event
	m.calendarDetailOpen = true

	model, cmd := m.handleKeyMsg(keyRunes("s"))
	m = model.(*Model)
	if !m.calendarAISummaryOpen || !m.calendarAISummaryLoading {
		t.Fatalf("AI summary state open=%v loading=%v, want open loading", m.calendarAISummaryOpen, m.calendarAISummaryLoading)
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.aiSummaryCalls) != 1 || b.aiSummaryCalls[0].LocalID != event.Ref.WithDefaults().LocalID {
		t.Fatalf("AI summary calls = %#v, want selected event ref", b.aiSummaryCalls)
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{
		"AI Summary",
		"read-only cached AI summary",
		"Launch planning",
		"Summary Bullets",
		"Mina flagged launch risk",
		"Action Items",
		"Review Launch planning risks",
		"Related Sources",
		"1 cached mail",
		"1 nearby event",
		"Query Terms",
		"mina@example.com",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("AI summary missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"provider-secret", "provider-etag", "syncToken", "OAuth", "Save", "Edit"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("AI summary leaked or advertised %q:\n%s", forbidden, rendered)
		}
	}

	model, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(*Model)
	if m.calendarAISummaryOpen {
		t.Fatal("expected Esc to close AI summary")
	}
	if !m.calendarDetailOpen {
		t.Fatal("expected Esc from AI summary to return to Event Detail")
	}
}

func TestCalendarAISummaryShortcutDoesNotStealTextEntry(t *testing.T) {
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

		model, _ := m.handleKeyMsg(keyRunes("s"))
		m = model.(*Model)
		if got := m.composeBody.Value(); got != "s" {
			t.Fatalf("compose body=%q, want literal s", got)
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

		model, cmd := m.handleKeyMsg(keyRunes("s"))
		m = model.(*Model)
		for _, msg := range calendarImmediateMessagesForTest(cmd) {
			model, _ = m.Update(msg)
			m = model.(*Model)
		}
		if m.calendarSearchQuery != "s" {
			t.Fatalf("calendar search query=%q, want literal s", m.calendarSearchQuery)
		}
		if m.calendarAISummaryOpen {
			t.Fatal("AI summary should not open while typing in calendar search")
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

		model, _ := m.Update(keyRunes("s"))
		m = model.(*Model)
		if got := m.promptEditor.name; got != "s" {
			t.Fatalf("prompt editor name=%q, want literal s", got)
		}
	})
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
	for _, want := range []string{"Calendar", "Agenda", "Roadmap sync", "past events hidden", "[p] Show past", "Event Detail", "read-only"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("calendar view missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "etag") || strings.Contains(rendered, "oauth") || strings.Contains(rendered, "caldav") {
		t.Fatalf("calendar view exposed provider internals:\n%s", rendered)
	}

	if got := m.selectedCalendarEvent(); got == nil || got.Title != "Roadmap sync" {
		t.Fatalf("selected event with past rows hidden = %#v, want Roadmap sync", got)
	}
	model, _ = m.handleKeyMsg(keyRunes("p"))
	m = model.(*Model)
	rendered = stripANSI(m.renderMainView())
	for _, want := range []string{"[p] Hide past", "Design review", "Daily standup"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("calendar show-past view missing %q:\n%s", want, rendered)
		}
	}
	model, _ = m.handleKeyMsg(keyRunes("k"))
	m = model.(*Model)
	if got := m.selectedCalendarEvent(); got == nil || got.Title != "Weekly planning" {
		t.Fatalf("selected event after k with past rows shown = %#v, want Weekly planning", got)
	}
}

func TestCalendarAgendaFiltersZeroStartRowsAndUsesDefaultRange(t *testing.T) {
	today := calendarDayStartFor(time.Now())
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
	nextMonth := monthStart.AddDate(0, 1, 0)
	old := today.AddDate(0, 0, -45).Add(9 * time.Hour)
	malformedSpan := today.AddDate(-2, 0, 0).Add(16 * time.Hour)
	current := today.Add(10 * time.Hour)
	future := nextMonth.AddDate(0, 0, -1).Add(11 * time.Hour)
	events := []models.CalendarEvent{
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "zero-start"}.WithDefaults(),
			Title:  "Zero start should stay hidden",
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "old-event"}.WithDefaults(),
			Title:  "Old event outside window",
			Start:  old,
			End:    old.Add(time.Hour),
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "malformed-span"}.WithDefaults(),
			Title:  "Malformed historic span",
			Start:  malformedSpan,
			End:    today.AddDate(0, 0, 10).Add(17 * time.Hour),
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "current-event"}.WithDefaults(),
			Title:  "Current window event",
			Start:  current,
			End:    current.Add(time.Hour),
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "future-event"}.WithDefaults(),
			Title:  "Future window event",
			Start:  future,
			End:    future.Add(time.Hour),
			Status: "confirmed",
		},
	}
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	for _, msg := range calendarImmediateMessagesForTest(m.loadCalendarAgenda()) {
		model, _ := m.Update(msg)
		m = model.(*Model)
	}

	if len(m.calendarEvents) != 4 {
		t.Fatalf("calendar events = %d, want zero-start row filtered from model", len(m.calendarEvents))
	}
	if !sameCalendarDate(m.calendarAgendaStart, monthStart) || !sameCalendarDate(m.calendarAgendaEnd, nextMonth) {
		t.Fatalf("agenda range = %v..%v, want local calendar month %v..%v", m.calendarAgendaStart, m.calendarAgendaEnd, monthStart, nextMonth)
	}
	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Agenda", "Current window event", "Future window event"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("agenda missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"Zero start should stay hidden", "Old event outside window", "Malformed historic span", "Dec 31", "1950"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("agenda rendered forbidden %q:\n%s", forbidden, rendered)
		}
	}
}

func TestCalendarAgendaHidesPastRowsByDefaultAndCanShowPast(t *testing.T) {
	today := calendarDayStartFor(time.Now())
	pastStart := today.AddDate(0, 0, -1).Add(9 * time.Hour)
	spanningStart := today.Add(-1 * time.Hour)
	events := []models.CalendarEvent{
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "past-finished"}.WithDefaults(),
			Title:  "Past finished",
			Start:  pastStart,
			End:    pastStart.Add(time.Hour),
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "spans-today"}.WithDefaults(),
			Title:  "Spans into today",
			Start:  spanningStart,
			End:    today.Add(time.Hour),
			Status: "confirmed",
		},
	}
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarView = calendarViewAgenda
	m.calendarEvents = normalizeCalendarEventsForDisplay(events)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(pastStart)
	m.ensureCalendarSelectionVisible()

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Spans into today", "1 past event hidden before", "[p] Show past"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("agenda with hidden past rows missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Past finished") {
		t.Fatalf("agenda rendered a past row by default:\n%s", rendered)
	}
	if hints := stripANSI(m.renderKeyHints()); !strings.Contains(hints, "p: show past") {
		t.Fatalf("agenda hints missing show-past action:\n%s", hints)
	}

	model, _ := m.handleKeyMsg(keyRunes("p"))
	m = model.(*Model)
	rendered = stripANSI(m.renderMainView())
	for _, want := range []string{"Past finished", "Spans into today", "[p] Hide past"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("agenda after show-past missing %q:\n%s", want, rendered)
		}
	}
	if hints := stripANSI(m.renderKeyHints()); !strings.Contains(hints, "p: hide past") {
		t.Fatalf("agenda hints missing hide-past action:\n%s", hints)
	}

	model, _ = m.handleKeyMsg(keyRunes("p"))
	m = model.(*Model)
	if rendered = stripANSI(m.renderMainView()); strings.Contains(rendered, "Past finished") {
		t.Fatalf("agenda kept rendering past row after hiding again:\n%s", rendered)
	}
}

func TestCalendarAgendaEmptyUpcomingStateShowsHiddenPastAffordance(t *testing.T) {
	today := calendarDayStartFor(time.Now())
	pastStart := today.AddDate(0, 0, -1).Add(9 * time.Hour)
	event := models.CalendarEvent{
		Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "past-only"}.WithDefaults(),
		Title:  "Past only",
		Start:  pastStart,
		End:    pastStart.Add(time.Hour),
		Status: "confirmed",
	}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event}}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarView = calendarViewAgenda
	m.calendarEvents = normalizeCalendarEventsForDisplay([]models.CalendarEvent{event})
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(pastStart)
	m.ensureCalendarSelectionVisible()

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"1 past event hidden before", "[p] Show past", "No upcoming calendar events"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("empty upcoming agenda missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Past only") {
		t.Fatalf("empty upcoming agenda rendered hidden past row:\n%s", rendered)
	}
}

func TestCalendarDefaultAgendaRangeIgnoresPastOnlyHistory(t *testing.T) {
	today := calendarDayStartFor(time.Now())
	pastOnly := today.AddDate(0, -2, 0).Add(9 * time.Hour)
	events := []models.CalendarEvent{
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "past-only-history"}.WithDefaults(),
			Title:  "Past only history",
			Start:  pastOnly,
			End:    pastOnly.Add(time.Hour),
			Status: "confirmed",
		},
	}

	got := calendarDefaultAgendaStart(events, today)
	want, _ := calendarAgendaWindowFor(today)
	if !sameCalendarDate(got, want) {
		t.Fatalf("default agenda start = %v, want current month %v instead of past-only history", got, want)
	}
}

func TestCalendarAgendaLoadUsesCachedRowsBeforeProviderRefresh(t *testing.T) {
	now := calendarDayStartFor(time.Now()).Add(10 * time.Hour)
	cached := models.CalendarEvent{
		Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "cached"}.WithDefaults(),
		Title:  "Cached planning",
		Start:  now,
		End:    now.Add(time.Hour),
		Status: "confirmed",
	}
	refreshed := cached
	refreshed.Ref.EventID = "fresh"
	refreshed.Ref = refreshed.Ref.WithDefaults()
	refreshed.Title = "Provider planning"
	b := &calendarAgendaStubBackend{
		available:     true,
		cachedEvents:  []models.CalendarEvent{cached},
		refreshEvents: []models.CalendarEvent{refreshed},
	}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarLoading = true

	msgs := calendarImmediateMessagesForTest(m.loadCalendarAgenda())
	if len(msgs) < 2 {
		t.Fatalf("expected cache-first load to return cached and refreshed messages, got %d", len(msgs))
	}
	first, ok := msgs[0].(CalendarAgendaLoadedMsg)
	if !ok {
		t.Fatalf("first calendar load message = %T, want CalendarAgendaLoadedMsg", msgs[0])
	}
	if len(first.Events) != 1 || first.Events[0].Title != "Cached planning" {
		t.Fatalf("first calendar load events = %#v, want cached event before provider refresh", first.Events)
	}
	model, _ := m.Update(first)
	m = model.(*Model)
	if len(m.calendarEvents) != 1 || m.calendarEvents[0].Title != "Cached planning" {
		t.Fatalf("model events after cached load = %#v, want cached rows visible", m.calendarEvents)
	}
	if !m.calendarLoading {
		t.Fatalf("calendar loading should remain active while provider refresh is pending")
	}

	second, ok := msgs[1].(CalendarAgendaLoadedMsg)
	if !ok {
		t.Fatalf("second calendar load message = %T, want CalendarAgendaLoadedMsg", msgs[1])
	}
	model, _ = m.Update(second)
	m = model.(*Model)
	if len(m.calendarEvents) != 1 || m.calendarEvents[0].Title != "Provider planning" {
		t.Fatalf("model events after refresh = %#v, want provider-refreshed rows", m.calendarEvents)
	}
	if m.calendarLoading {
		t.Fatalf("calendar loading should be false after provider refresh completes")
	}
	if b.cachedAgendaCalls != 1 || b.refreshAgendaCalls != 1 {
		t.Fatalf("cache/refresh calls = %d/%d, want one each", b.cachedAgendaCalls, b.refreshAgendaCalls)
	}
}

func TestCalendarDerivedEventSlicesAreReusedUntilInvalidated(t *testing.T) {
	loc := time.Local
	events := largeCalendarEventsForTest(80, loc)
	m := New(&calendarAgendaStubBackend{available: true}, nil, "", nil, false)
	m.setCalendarEventsForDisplay(events)
	m.calendarView = calendarViewAgenda
	m.calendarAgendaShowPast = true
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(time.Date(2026, 5, 12, 0, 0, 0, 0, loc))

	first := m.indexedVisibleCalendarEvents()
	second := m.indexedVisibleCalendarEvents()
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("expected visible test events")
	}
	if &first[0] != &second[0] {
		t.Fatalf("visible calendar event slices were rebuilt between identical reads")
	}

	day := time.Date(2026, 5, 12, 0, 0, 0, 0, loc)
	firstDay := m.calendarEventsForDay(day)
	secondDay := m.calendarEventsForDay(day)
	if len(firstDay) == 0 || len(secondDay) == 0 {
		t.Fatalf("expected day test events")
	}
	if &firstDay[0] != &secondDay[0] {
		t.Fatalf("day calendar event slices were rebuilt between identical reads")
	}

	m.calendarHiddenCollections = map[string]bool{
		calendarCollectionRefKey(models.CollectionRef{SourceID: "demo-calendar", AccountID: "default", Kind: models.SourceKindCalendar, CollectionID: "work"}): true,
	}
	m.invalidateCalendarFilterDerivations()
	hidden := m.indexedVisibleCalendarEvents()
	if len(hidden) != 0 {
		t.Fatalf("hidden collection should invalidate derived cache and remove events, got %d", len(hidden))
	}
}

func BenchmarkCalendarAgendaNavigationRenderLarge(b *testing.B) {
	loc := time.Local
	m := New(&calendarAgendaStubBackend{available: true}, nil, "", nil, false)
	m.windowWidth = 140
	m.windowHeight = 40
	m.activeTab = tabCalendar
	m.calendarAvailable = true
	m.calendarView = calendarViewAgenda
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(time.Date(2026, 5, 12, 0, 0, 0, 0, loc))
	m.setCalendarEventsForDisplay(largeCalendarEventsForTest(1200, loc))
	m.ensureCalendarSelectionVisible()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model, _ := m.handleKeyMsg(keyRunes("j"))
		m = model.(*Model)
		_ = m.renderCalendarView()
	}
}

func BenchmarkSettingsOverlayCalendarBackdropMovement(b *testing.B) {
	loc := time.Local
	m := New(&calendarAgendaStubBackend{available: true}, nil, "", nil, false)
	m.windowWidth = 120
	m.windowHeight = 40
	m.activeTab = tabCalendar
	m.calendarAvailable = true
	m.calendarView = calendarViewAgenda
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(time.Date(2026, 5, 12, 0, 0, 0, 0, loc))
	m.setCalendarEventsForDisplay(largeCalendarEventsForTest(1200, loc))
	model, _ := m.handleKeyMsg(keyRunes("S"))
	m = model.(*Model)
	_ = m.View().Content
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = model.(*Model)
		_ = m.View().Content
	}
}

func largeCalendarEventsForTest(count int, loc *time.Location) []models.CalendarEvent {
	events := make([]models.CalendarEvent, 0, count)
	for i := 0; i < count; i++ {
		day := 1 + (i % 28)
		hour := 8 + (i % 10)
		start := time.Date(2026, 5, day, hour, 0, 0, 0, loc)
		events = append(events, models.CalendarEvent{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: fmt.Sprintf("event-%04d", i)}.WithDefaults(),
			Title:  fmt.Sprintf("Planning %04d", i),
			Start:  start,
			End:    start.Add(time.Hour),
			Status: "confirmed",
		})
	}
	return events
}

func TestCalendarAgendaWindowUsesCalendarMonth(t *testing.T) {
	loc := time.Local
	start, end := calendarAgendaWindowFor(time.Date(2026, 5, 27, 14, 30, 0, 0, loc))

	wantStart := time.Date(2026, 5, 1, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("calendarAgendaWindowFor = %v..%v, want %v..%v", start, end, wantStart, wantEnd)
	}
}

func TestCalendarMiniMonthBoldsEventDaysDifferentlyFromEmptyDays(t *testing.T) {
	loc := time.Local
	workRef := models.CollectionRef{
		SourceID:     "demo-calendar",
		AccountID:    "default",
		Kind:         models.SourceKindCalendar,
		CollectionID: "work",
		DisplayName:  "Work",
	}
	personalRef := models.CollectionRef{
		SourceID:     "demo-calendar",
		AccountID:    "default",
		Kind:         models.SourceKindCalendar,
		CollectionID: "personal",
		DisplayName:  "Personal",
	}
	events := []models.CalendarEvent{
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "planning"}.WithDefaults(),
			Title:  "Planning",
			Start:  time.Date(2026, 5, 4, 10, 0, 0, 0, loc),
			End:    time.Date(2026, 5, 4, 11, 0, 0, 0, loc),
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "personal", EventID: "hidden"}.WithDefaults(),
			Title:  "Hidden personal event",
			Start:  time.Date(2026, 5, 5, 10, 0, 0, 0, loc),
			End:    time.Date(2026, 5, 5, 11, 0, 0, 0, loc),
			Status: "confirmed",
		},
	}
	m := New(&calendarAgendaStubBackend{available: true, events: events}, nil, "", nil, false)
	m.calendarEvents = events
	m.calendarCollections = []models.CalendarCollection{
		{Ref: workRef, Color: "#ff5f87"},
		{Ref: personalRef, Color: "#87d75f"},
	}
	m.calendarHiddenCollections = map[string]bool{calendarCollectionRefKey(personalRef): true}

	month := time.Date(2026, 5, 1, 0, 0, 0, 0, loc)
	rangeStart, rangeEndExclusive := calendarAgendaWindowFor(month)
	m.calendarView = calendarViewAgenda
	m.calendarCursor = -1
	m.calendarAgendaStart = rangeStart
	m.calendarAgendaEnd = rangeEndExclusive
	rangeEnd := rangeEndExclusive.AddDate(0, 0, -1)
	eventCell := m.renderCalendarMiniMonthDayCell(time.Date(2026, 5, 4, 0, 0, 0, 0, loc), month, rangeStart, rangeEnd)
	emptyCell := m.renderCalendarMiniMonthDayCell(time.Date(2026, 5, 6, 0, 0, 0, 0, loc), month, rangeStart, rangeEnd)
	hiddenCell := m.renderCalendarMiniMonthDayCell(time.Date(2026, 5, 5, 0, 0, 0, 0, loc), month, rangeStart, rangeEnd)

	if eventCell == emptyCell {
		t.Fatalf("event day and empty day rendered the same: %q", eventCell)
	}
	if !hasBoldANSI(eventCell) {
		t.Fatalf("event day did not use bold day text: %q", eventCell)
	}
	if hasBoldANSI(emptyCell) {
		t.Fatalf("empty day used bold day text: %q", emptyCell)
	}
	if hasBoldANSI(hiddenCell) {
		t.Fatalf("hidden calendar event still bolded the mini month day: %q", hiddenCell)
	}
	if ansi.StringWidth(eventCell) != 2 || ansi.StringWidth(emptyCell) != 2 || ansi.StringWidth(hiddenCell) != 2 {
		t.Fatalf("mini-month cells should stay two columns wide, got event=%d empty=%d hidden=%d", ansi.StringWidth(eventCell), ansi.StringWidth(emptyCell), ansi.StringWidth(hiddenCell))
	}
}

func hasBoldANSI(value string) bool {
	return strings.Contains(value, "[1m") || strings.Contains(value, "[1;") || strings.Contains(value, ";1m") || strings.Contains(value, ";1;")
}

func TestCalendarAgendaRangeNavigationMovesByCalendarMonth(t *testing.T) {
	loc := time.Local
	m := &Model{
		calendarView:        calendarViewAgenda,
		calendarAgendaStart: time.Date(2026, 5, 1, 0, 0, 0, 0, loc),
		calendarAgendaEnd:   time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
	}

	m.moveCalendarRange(1)
	if want := time.Date(2026, 6, 1, 0, 0, 0, 0, loc); !m.calendarAgendaStart.Equal(want) {
		t.Fatalf("next agenda start = %v, want %v", m.calendarAgendaStart, want)
	}
	if want := time.Date(2026, 7, 1, 0, 0, 0, 0, loc); !m.calendarAgendaEnd.Equal(want) {
		t.Fatalf("next agenda end = %v, want %v", m.calendarAgendaEnd, want)
	}

	m.moveCalendarRange(-1)
	if want := time.Date(2026, 5, 1, 0, 0, 0, 0, loc); !m.calendarAgendaStart.Equal(want) {
		t.Fatalf("previous agenda start = %v, want %v", m.calendarAgendaStart, want)
	}
	if want := time.Date(2026, 6, 1, 0, 0, 0, 0, loc); !m.calendarAgendaEnd.Equal(want) {
		t.Fatalf("previous agenda end = %v, want %v", m.calendarAgendaEnd, want)
	}
}

func TestCalendarWeekStartForUsesMonday(t *testing.T) {
	loc := time.Local
	wednesday := time.Date(2026, 5, 27, 15, 0, 0, 0, loc)
	start := calendarWeekStartFor(wednesday)
	want := time.Date(2026, 5, 25, 0, 0, 0, 0, loc)
	if !start.Equal(want) {
		t.Fatalf("calendarWeekStartFor(%v) = %v, want Monday %v", wednesday, start, want)
	}
	if got := calendarWeekRange(wednesday); got != "Mon May 25 - Sun May 31, 2026" {
		t.Fatalf("calendarWeekRange = %q, want Monday-Sunday range", got)
	}
}

func TestCalendarAgendaFallsBackToNearestValidEventWindow(t *testing.T) {
	today := calendarDayStartFor(time.Now())
	malformedSpan := today.AddDate(-2, 0, 0).Add(16 * time.Hour)
	nearFuture := today.AddDate(0, 0, 45).Add(9 * time.Hour)
	farFuture := today.AddDate(0, 0, 80).Add(9 * time.Hour)
	events := []models.CalendarEvent{
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "malformed-span"}.WithDefaults(),
			Title:  "Malformed span should not anchor today",
			Start:  malformedSpan,
			End:    today.AddDate(0, 0, 10).Add(17 * time.Hour),
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "near-future"}.WithDefaults(),
			Title:  "Nearest future event",
			Start:  nearFuture,
			End:    nearFuture.Add(time.Hour),
			Status: "confirmed",
		},
		{
			Ref:    models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "far-future"}.WithDefaults(),
			Title:  "Far future event",
			Start:  farFuture,
			End:    farFuture.Add(time.Hour),
			Status: "confirmed",
		},
	}
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	for _, msg := range calendarImmediateMessagesForTest(m.loadCalendarAgenda()) {
		model, _ := m.Update(msg)
		m = model.(*Model)
	}

	nearFutureMonth := time.Date(nearFuture.Year(), nearFuture.Month(), 1, 0, 0, 0, 0, nearFuture.Location())
	if !sameCalendarDate(m.calendarAgendaStart, nearFutureMonth) {
		t.Fatalf("agenda start = %v, want nearest future event month %v", m.calendarAgendaStart, nearFutureMonth)
	}
	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "Nearest future event") {
		t.Fatalf("agenda did not render nearest future event:\n%s", rendered)
	}
	if strings.Contains(rendered, "Malformed span should not anchor today") {
		t.Fatalf("agenda rendered malformed historic span:\n%s", rendered)
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
	if m.calendarWeekStart.Local().Day() != 18 {
		t.Fatalf("calendarWeekStart = %s, want week starting May 18", m.calendarWeekStart)
	}

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Week Time-Grid", "Mon May 18", "Sun May 24", "Design review", "Daily standup", "Week Inspector", "Herald planning room", "Local", "Event TZ", "h/l: week", "d: day", "a: agenda"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("week grid missing %q:\n%s", want, rendered)
		}
	}
	if lower := strings.ToLower(rendered); strings.Contains(lower, "etag") || strings.Contains(lower, "oauth") || strings.Contains(lower, "caldav") {
		t.Fatalf("week grid exposed provider internals:\n%s", rendered)
	}
}

func TestCalendarWeekGridShowsHalfHourRowsOnTallScreensWithoutCuttingLongEvents(t *testing.T) {
	start := time.Date(2026, 4, 20, 13, 0, 0, 0, time.Local)
	event := models.CalendarEvent{
		Ref: models.EventRef{
			SourceID:   "demo-calendar",
			AccountID:  models.DefaultAccountID,
			CalendarID: "personal",
			EventID:    "focus-block",
		}.WithDefaults(),
		Title:  "Focus Block",
		Start:  start,
		End:    start.Add(2 * time.Hour),
		Status: "busy",
	}
	m := New(&calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event}}, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarView = calendarViewWeek
	m.calendarEvents = normalizeCalendarEventsForDisplay([]models.CalendarEvent{event})
	m.calendarWeekStart = calendarWeekStartFor(start)
	m.calendarDetail = &m.calendarEvents[0]

	rendered := stripANSI(m.renderCalendarWeekGrid(110, 40))
	if !strings.Contains(rendered, "13:30") {
		t.Fatalf("tall week grid should include 30-minute rows:\n%s", rendered)
	}
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "13:00") || !strings.Contains(line, "Focus Block") {
			continue
		}
		if i+1 >= len(lines) {
			t.Fatalf("13:00 event row was the final rendered line:\n%s", rendered)
		}
		next := lines[i+1]
		if !strings.Contains(next, "13:30") {
			t.Fatalf("long event should continue into the next half-hour row, got next line %q:\n%s", next, rendered)
		}
		cells := strings.Split(next[7:], "│")
		if len(cells) < 2 {
			t.Fatalf("half-hour row should keep day cells, got %q:\n%s", next, rendered)
		}
		if strings.Contains(cells[0], "·") {
			t.Fatalf("long event continuation cell should mask guide dots, got %q in row %q:\n%s", cells[0], next, rendered)
		}
		if !strings.Contains(strings.Join(cells[1:], ""), "····") {
			t.Fatalf("guide dots should remain in empty half-hour cells, got %q:\n%s", next, rendered)
		}
		return
	}
	t.Fatalf("did not find 13:00 Focus Block row:\n%s", rendered)
}

func TestCalendarViewSwitchingUsesSelectedDateInsteadOfAgendaMonthStart(t *testing.T) {
	events := dateAnchorCalendarEventsForTest()
	b := &calendarAgendaStubBackend{available: true, events: events}

	for _, tc := range []struct {
		name      string
		key       string
		wantView  calendarViewMode
		wantStart string
	}{
		{name: "day", key: "d", wantView: calendarViewDay, wantStart: "2026-05-28"},
		{name: "week", key: "w", wantView: calendarViewWeek, wantStart: "2026-05-25"},
		{name: "three-day", key: "t", wantView: calendarViewThreeDay, wantStart: "2026-05-28"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(b, nil, "", nil, false)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
			m = updated.(*Model)
			m.loading = false
			m.activeTab = tabCalendar
			m.calendarView = calendarViewAgenda
			m.calendarEvents = normalizeCalendarEventsForDisplay(events)
			m.calendarAgendaShowPast = true
			m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(events[0].Start)
			m.calendarAgendaShowPast = true
			m.calendarCursor = 1
			m.calendarDetail = &m.calendarEvents[1]

			model, _ := m.handleKeyMsg(keyRunes(tc.key))
			m = model.(*Model)
			if m.calendarView != tc.wantView {
				t.Fatalf("calendarView = %q, want %q", m.calendarView, tc.wantView)
			}
			start, _, _ := m.calendarActiveRange()
			if got := calendarDayStartFor(start).Format("2006-01-02"); got != tc.wantStart {
				t.Fatalf("active range starts %s, want %s", got, tc.wantStart)
			}
		})
	}
}

func TestCalendarAgendaReturnsToSelectedEventMonth(t *testing.T) {
	events := dateAnchorCalendarEventsForTest()
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarView = calendarViewDay
	m.calendarEvents = normalizeCalendarEventsForDisplay(events)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(events[0].Start)
	m.calendarCursor = 2
	m.calendarDetail = &m.calendarEvents[2]
	m.calendarDay = calendarDayStartFor(events[2].Start)

	model, _ := m.handleKeyMsg(keyRunes("a"))
	m = model.(*Model)
	if m.calendarView != calendarViewAgenda {
		t.Fatalf("calendarView = %q, want agenda", m.calendarView)
	}
	start, end := m.calendarAgendaWindow()
	if got := calendarCompactDateRange(start, end.AddDate(0, 0, -1)); got != "Jun 1-30, 2026" {
		t.Fatalf("agenda range = %s, want Jun 1-30, 2026", got)
	}
}

func TestCalendarAllDayExclusiveEndDoesNotDuplicateNextDay(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	event := models.CalendarEvent{
		Title:  "No school",
		Start:  time.Date(2026, 5, 25, 0, 0, 0, 0, loc),
		End:    time.Date(2026, 5, 26, 0, 0, 0, 0, loc),
		AllDay: true,
	}
	if !eventOccursOnCalendarDate(event, event.Start) {
		t.Fatal("all-day event should occur on its start date")
	}
	if eventOccursOnCalendarDate(event, event.End) {
		t.Fatal("exclusive all-day end date should not render as another event day")
	}
}

func TestCalendarAgendaDoesNotShowEventStartingBeforeDisplayedMonth(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	agendaStart, agendaEnd := calendarAgendaWindowFor(time.Date(2026, 5, 20, 12, 0, 0, 0, loc))
	event := models.CalendarEvent{
		Title:  "Intersession",
		Start:  time.Date(2026, 4, 27, 0, 0, 0, 0, loc),
		End:    time.Date(2026, 5, 9, 0, 0, 0, 0, loc),
		AllDay: true,
	}
	if calendarEventOccursInAgendaWindow(event, agendaStart, agendaEnd) {
		t.Fatal("May agenda should not include an all-day event whose displayed start date is in April")
	}
}

func TestNormalizeCalendarEventsForDisplayRepairsLegacyUTCAllDayCache(t *testing.T) {
	oldLocal := time.Local
	loc := time.FixedZone("PDT", -7*60*60)
	time.Local = loc
	t.Cleanup(func() { time.Local = oldLocal })

	events := normalizeCalendarEventsForDisplay([]models.CalendarEvent{{
		Ref:    models.EventRef{SourceID: "icloud-calendar", AccountID: "icloud", CalendarID: "family", EventID: "no-school"}.WithDefaults(),
		Title:  "No school",
		Start:  time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		End:    time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC),
		AllDay: true,
	}})
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if got := events[0].Start; got.Format("2006-01-02 15:04 MST") != "2026-05-25 00:00 PDT" {
		t.Fatalf("normalized all-day start = %s, want provider date at local midnight", got.Format("2006-01-02 15:04 MST"))
	}
	if !eventOccursOnCalendarDate(events[0], time.Date(2026, 5, 25, 12, 0, 0, 0, loc)) {
		t.Fatal("normalized all-day event should occur on provider start date")
	}
	if eventOccursOnCalendarDate(events[0], time.Date(2026, 5, 24, 12, 0, 0, 0, loc)) {
		t.Fatal("normalized all-day event should not leak to the previous local date")
	}
}

func TestNormalizeCalendarEventsForDisplayRepairsCachedCalDAVTimezoneStart(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	raw := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:phantom",
		"SUMMARY:Phantom",
		"DTSTART;TZID=America/Los_Angeles:20260530T193000",
		"DTEND;TZID=America/Los_Angeles:20260530T223000",
		"END:VEVENT",
		"BEGIN:VTIMEZONE",
		"TZID:America/Los_Angeles",
		"BEGIN:STANDARD",
		"DTSTART:20071104T020000",
		"END:STANDARD",
		"END:VTIMEZONE",
		"END:VCALENDAR",
	}, "\r\n") + "\r\n"

	events := normalizeCalendarEventsForDisplay([]models.CalendarEvent{{
		Ref:      models.EventRef{SourceID: "icloud-calendar", AccountID: "icloud", CalendarID: "home", EventID: "phantom.ics", ETag: `"etag"`}.WithDefaults(),
		Title:    "Phantom",
		Start:    time.Date(2007, 11, 4, 2, 0, 0, 0, loc),
		End:      time.Date(2026, 5, 30, 22, 30, 0, 0, loc),
		TimeZone: "America/Los_Angeles",
		Raw:      raw,
	}})
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if got := events[0].Start.In(loc).Format("2006-01-02 15:04 MST"); got != "2026-05-30 19:30 PDT" {
		t.Fatalf("normalized cached CalDAV start = %s, want VEVENT DTSTART", got)
	}
	if got := events[0].End.In(loc).Format("2006-01-02 15:04 MST"); got != "2026-05-30 22:30 PDT" {
		t.Fatalf("normalized cached CalDAV end = %s, want VEVENT DTEND", got)
	}
}

func TestNormalizeCalendarEventsForDisplayKeepsExpandedRecurringOccurrence(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	raw := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:rsm-alg-sofia",
		"SUMMARY:RSM Alg Sofia",
		"DTSTART;TZID=America/Los_Angeles:20250825T153500",
		"DTEND;TZID=America/Los_Angeles:20250825T180500",
		"RRULE:FREQ=WEEKLY;UNTIL=20260602T065959Z",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n") + "\r\n"

	events := normalizeCalendarEventsForDisplay([]models.CalendarEvent{{
		Ref: models.EventRef{
			SourceID:   "icloud-calendar",
			AccountID:  "icloud",
			CalendarID: "home",
			EventID:    "rsm-alg-sofia.ics",
			InstanceID: "20260525T223500Z",
		}.WithDefaults(),
		Title: "RSM Alg Sofia",
		Start: time.Date(2026, 5, 25, 15, 35, 0, 0, loc),
		End:   time.Date(2026, 5, 25, 18, 5, 0, 0, loc),
		Raw:   raw,
	}})
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if got := events[0].Start.In(loc).Format("2006-01-02 15:04"); got != "2026-05-25 15:35" {
		t.Fatalf("expanded occurrence start = %s, want May occurrence preserved", got)
	}
}

func TestCalendarWeekStartCanUseSundayFromConfig(t *testing.T) {
	events := dateAnchorCalendarEventsForTest()
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	cfg := &config.Config{}
	setCalendarWeekStartForTest(t, cfg, "sunday")
	m.SetConfig(cfg)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarView = calendarViewAgenda
	m.calendarEvents = normalizeCalendarEventsForDisplay(events)
	m.calendarAgendaShowPast = true
	m.calendarCursor = 1
	m.calendarDetail = &m.calendarEvents[1]

	model, _ := m.handleKeyMsg(keyRunes("w"))
	m = model.(*Model)
	if got := m.calendarWeekStart.Format("2006-01-02"); got != "2026-05-24" {
		t.Fatalf("Sunday week start = %s, want 2026-05-24", got)
	}
	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "Sun May 24") || !strings.Contains(rendered, "Sat May 30") {
		t.Fatalf("Sunday-start week range missing from render:\n%s", rendered)
	}
}

func TestCalendarTimedMultiDayEventDoesNotFillEveryWeekHour(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	event := models.CalendarEvent{
		Ref:   models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "travel", EventID: "flight-week"}.WithDefaults(),
		Title: "Flight hold",
		Start: time.Date(2026, 5, 25, 2, 0, 0, 0, loc),
		End:   time.Date(2026, 5, 31, 22, 0, 0, 0, loc),
	}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{event}}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarView = calendarViewWeek
	m.calendarEvents = normalizeCalendarEventsForDisplay([]models.CalendarEvent{event})

	got, continuation := m.calendarEventInHour(time.Date(2026, 5, 27, 0, 0, 0, 0, loc), 10)
	if got != nil || continuation {
		t.Fatalf("multi-day timed event occupied a normal hour cell: event=%#v continuation=%v", got, continuation)
	}
}

func TestCalendarWeekVisibleEventsAreScopedToActiveWeek(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	inWeek := models.CalendarEvent{
		Ref:   models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "in-week"}.WithDefaults(),
		Title: "In week",
		Start: time.Date(2026, 5, 25, 9, 0, 0, 0, loc),
		End:   time.Date(2026, 5, 25, 10, 0, 0, 0, loc),
	}
	nextWeek := models.CalendarEvent{
		Ref:   models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "next-week"}.WithDefaults(),
		Title: "Next week",
		Start: time.Date(2026, 6, 1, 9, 0, 0, 0, loc),
		End:   time.Date(2026, 6, 1, 10, 0, 0, 0, loc),
	}
	m := &Model{
		calendarView:      calendarViewWeek,
		calendarWeekStart: time.Date(2026, 5, 24, 0, 0, 0, 0, loc),
		calendarEvents:    []models.CalendarEvent{inWeek, nextWeek},
	}

	events := m.indexedCalendarEventsForActiveAnchorRange()
	if len(events) != 1 || events[0].event.Title != "In week" {
		t.Fatalf("visible week events = %#v, want only active week event", events)
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
	if m.calendarWeekStart.Local().Day() != 25 {
		t.Fatalf("calendarWeekStart = %s, want May 25", m.calendarWeekStart)
	}
	if got := m.selectedCalendarEvent(); got == nil || got.Title != "Weekly planning" {
		t.Fatalf("selected event after next week = %#v, want Weekly planning", got)
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
	if !strings.Contains(detail, "Event Detail") || !strings.Contains(detail, "Weekly planning") {
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
	if m.calendarDay.Local().Day() != 25 {
		t.Fatalf("calendarDay = %s, want selected event day May 25", m.calendarDay)
	}
	model, _ = m.handleKeyMsg(keyRunes("w"))
	m = model.(*Model)
	if m.calendarView != calendarViewWeek || m.calendarWeekStart.Local().Day() != 25 {
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

func TestCalendarEventDetailLinkifiesEventURLs(t *testing.T) {
	rich := richCalendarEventForTest()
	locationURL := "https://us06web.zoom.us/j/86920825197?pwd=dp0q1p26mDDxEHsgtMtULPaHB3hz5s.1"
	briefURL := "https://example.com/interview/brief"
	attachmentURL := "https://docs.example.com/agenda.pdf"
	rich.Location = locationURL
	rich.Description = "This is a Zoom meeting. Click this link to join:\n" + locationURL + "\nBrief: " + briefURL
	rich.Attachments = []models.CalendarAttachment{{
		Title:    "Agenda",
		URI:      attachmentURL,
		MIMEType: "application/pdf",
	}}
	b := &calendarAgendaStubBackend{
		available: true,
		events:    []models.CalendarEvent{rich},
		collections: []models.CalendarCollection{{
			Ref: models.CollectionRef{
				SourceID:     "demo-calendar",
				AccountID:    "default",
				Kind:         models.SourceKindCalendar,
				CollectionID: "work",
				DisplayName:  "Work",
			},
			Color: "#ff5f87",
		}},
	}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarCollections = b.collections
	m.calendarEvents = b.events
	m.calendarDetail = &rich
	m.calendarDetailOpen = true

	rendered := m.renderCalendarEventDetail(120, 60, false)
	for _, target := range []string{locationURL, briefURL, attachmentURL} {
		if !strings.Contains(rendered, "\x1b]8;;"+target) {
			t.Fatalf("calendar detail missing OSC8 target %q:\n%q", target, rendered)
		}
	}
	visible := ansi.Strip(rendered)
	if strings.Contains(visible, locationURL) {
		t.Fatalf("calendar detail should shorten visible event URLs, got:\n%s", visible)
	}
	if !strings.Contains(visible, "us06web.zoom.us/j/86920825197") || !strings.Contains(visible, "example.com/interview/brief") || !strings.Contains(visible, "Agenda (application/pdf)") {
		t.Fatalf("calendar detail missing readable shortened URL labels:\n%s", visible)
	}
}

func TestCalendarEventDetailSeparatesRSVPFromRunnableActions(t *testing.T) {
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

	rendered := stripANSI(m.renderCalendarEventDetail(70, 60, false))
	for _, want := range []string{
		"RSVP",
		"needs response",
		"y: accept  m: maybe  n: decline",
		"Actions",
		"e: edit event",
		"p: meeting prep",
		"b: travel buffer",
		"s: AI summary",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("event detail missing %q:\n%s", want, rendered)
		}
	}
	rsvpAt := strings.Index(rendered, "y: accept  m: maybe  n: decline")
	actionsAt := strings.Index(rendered, "Actions")
	if rsvpAt == -1 || actionsAt == -1 || rsvpAt > actionsAt {
		t.Fatalf("RSVP controls should render before runnable actions:\n%s", rendered)
	}
	for _, forbidden := range []string{"n: new event", "d: delete event", "r: reply", "a: add reminder"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("event detail advertised stale/conflicting action %q:\n%s", forbidden, rendered)
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

func TestCalendarRailRangeHeaderAndRenderedNotes(t *testing.T) {
	rich := richCalendarEventForTest()
	rich.Description = `<p>Before the meeting</p><ul><li><strong>Read</strong> brief</li><li>Bring notes</li></ul>`
	rich.Attendees[0].RSVP = "needs-action"
	b := &calendarAgendaStubBackend{
		available: true,
		events:    []models.CalendarEvent{rich},
		collections: []models.CalendarCollection{
			{
				Ref: models.CollectionRef{
					SourceID:     "demo-calendar",
					AccountID:    "default",
					Kind:         models.SourceKindCalendar,
					CollectionID: "work",
					DisplayName:  "Work",
				},
				Color: "#5fd7ff",
			},
		},
	}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	for _, msg := range calendarImmediateMessagesForTest(m.switchToCalendar()) {
		model, _ := m.Update(msg)
		m = model.(*Model)
	}
	m.calendarAgendaShowPast = true
	m.ensureCalendarSelectionVisible()

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"Calendars", "[x] Work", "Agenda (1) for", "<-/->/h/l to switch", "! Timezone planning"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("calendar render missing %q:\n%s", want, rendered)
		}
	}
	m.calendarDetail = &rich
	detail := stripANSI(m.renderCalendarEventDetail(70, 60, false))
	for _, want := range []string{"Before the meeting", "- Read brief", "- Bring notes", "RSVP", "needs response"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("rendered notes/detail missing %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "<strong>") || strings.Contains(detail, "</p>") {
		t.Fatalf("detail leaked raw HTML:\n%s", detail)
	}
}

func TestCalendarTitleBarUsesSharedTopLevelTabs(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCalendar

	title := stripANSI(m.renderTitleBar(140))
	for _, want := range []string{"Herald", "1  Timeline", "2  Contacts", "3  Calendar"} {
		if !strings.Contains(title, want) {
			t.Fatalf("calendar title bar missing shared tab %q:\n%s", want, title)
		}
	}
	for _, stale := range []string{"Herald Cal", "F1 Month", "F2 Week", "F3 Day", "F4 Agenda", "F5 Search", "t: Today", "z: Timezone"} {
		if strings.Contains(title, stale) {
			t.Fatalf("calendar title bar kept stale calendar-only chrome %q:\n%s", stale, title)
		}
	}
}

func TestCalendarAgendaRangeTitleLivesOnPanelFrame(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = normalizeCalendarEventsForDisplay(b.events)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(b.events[0].Start)
	m.calendarDetail = m.selectedCalendarEvent()

	rendered := stripANSI(m.renderMainView())
	var titleLine string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "Agenda (1) for") {
			titleLine = line
			break
		}
	}
	if titleLine == "" {
		t.Fatalf("calendar agenda render missing range title:\n%s", rendered)
	}
	if !strings.Contains(titleLine, "┌") || !strings.Contains(titleLine, "─") {
		t.Fatalf("agenda range title should be integrated into the panel frame, got:\n%s", titleLine)
	}
	if strings.Contains(titleLine, "---") {
		t.Fatalf("agenda range title should not use the old dashed content row:\n%s", titleLine)
	}
}

func TestCalendarAgendaFrameChromePromotesCountAndRemovesLoadedStatus(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = normalizeCalendarEventsForDisplay(b.events)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(b.events[0].Start)
	m.calendarDetail = m.selectedCalendarEvent()
	m.calendarStatus = "Loaded 4 calendar event(s)"

	rendered := stripANSI(m.renderMainView())
	titleLine := calendarFirstFrameLineForTest(rendered)
	for _, want := range []string{"Agenda (1) for Fri May 1 - Sun May 31, 2026", "(<-/->/h/l to switch)"} {
		if !strings.Contains(titleLine, want) {
			t.Fatalf("calendar agenda frame missing %q in:\n%s\n\nrendered:\n%s", want, titleLine, rendered)
		}
	}
	body := calendarAfterFirstFrameLineForTest(rendered)
	for _, stale := range []string{"Agenda (1)", "Loaded 4 calendar event(s)"} {
		if strings.Contains(body, stale) {
			t.Fatalf("calendar agenda body kept stale %q:\n%s", stale, rendered)
		}
	}
}

func TestCalendarDetailBorderContainsTitleWithoutBodyDuplicate(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = normalizeCalendarEventsForDisplay(b.events)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(b.events[0].Start)
	m.calendarDetail = m.selectedCalendarEvent()

	rendered := stripANSI(m.renderMainView())
	titleLine := calendarFirstFrameLineForTest(rendered)
	if !strings.Contains(titleLine, "Event Detail") {
		t.Fatalf("calendar detail frame missing title:\n%s", rendered)
	}
	body := calendarAfterFirstFrameLineForTest(rendered)
	if strings.Contains(body, "Event Detail") {
		t.Fatalf("calendar detail body duplicated frame title:\n%s", rendered)
	}
}

func TestCalendarRailBorderContainsDateRangeWithoutBodyDuplicate(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = normalizeCalendarEventsForDisplay(b.events)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(b.events[0].Start)
	m.calendarDetail = m.selectedCalendarEvent()

	rendered := stripANSI(m.renderMainView())
	titleLine := calendarFirstFrameLineForTest(rendered)
	if !strings.Contains(titleLine, "May 1-31, 2026") {
		t.Fatalf("calendar rail frame missing date range:\n%s", rendered)
	}
	body := calendarAfterFirstFrameLineForTest(rendered)
	if strings.Contains(body, "May 1-31, 2026") {
		t.Fatalf("calendar rail body duplicated date range:\n%s", rendered)
	}
}

func calendarFirstFrameLineForTest(rendered string) string {
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "┌") {
			return line
		}
	}
	return ""
}

func calendarAfterFirstFrameLineForTest(rendered string) string {
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if strings.Contains(line, "┌") {
			end := len(lines)
			for j := i + 1; j < len(lines); j++ {
				if strings.Contains(lines[j], "└") {
					end = j
					break
				}
			}
			return strings.Join(lines[i+1:end], "\n")
		}
	}
	return rendered
}

func TestCalendarDayNavigationCrossesDayBoundary(t *testing.T) {
	events := testCalendarEvents()
	b := &calendarAgendaStubBackend{available: true, events: events}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = normalizeCalendarEventsForDisplay(events)
	m.calendarView = calendarViewDay
	m.calendarDay = calendarDayStartFor(events[0].Start)
	m.calendarCursor = 1
	m.calendarDetail = &m.calendarEvents[1]

	m.moveCalendarDaySelection(1)
	if m.calendarDetail == nil || m.calendarDetail.Title != "Weekly planning" {
		t.Fatalf("day navigation selected %#v, want next-day Weekly planning", m.calendarDetail)
	}
	if !sameCalendarDate(m.calendarDay, events[2].Start) {
		t.Fatalf("calendarDay=%v, want next event day %v", m.calendarDay, events[2].Start)
	}
}

func TestCalendarExplicitRSVPKeys(t *testing.T) {
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

	model, cmd := m.handleKeyMsg(keyRunes("n"))
	m = model.(*Model)
	if cmd == nil {
		t.Fatal("expected decline key to produce RSVP command")
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.rsvpStatuses) != 1 || b.rsvpStatuses[0] != "declined" {
		t.Fatalf("RSVP statuses=%#v, want declined", b.rsvpStatuses)
	}
	if m.calendarDetail == nil || m.calendarDetail.Attendees[0].RSVP != "declined" {
		t.Fatalf("calendar detail = %#v, want declined attendee", m.calendarDetail)
	}
}

func TestCalendarPlainQQuitsWithoutStealingComposeText(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = b.events

	_, cmd := m.handleKeyMsg(keyRunes("q"))
	if !commandIsQuit(cmd) {
		t.Fatal("expected q to quit from calendar")
	}

	m = New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCompose
	m.focusComposeField(composeFieldBody)
	model, cmd := m.handleKeyMsg(keyRunes("q"))
	m = model.(*Model)
	if commandIsQuit(cmd) {
		t.Fatal("plain q should remain text input in compose")
	}
	if got := m.composeBody.Value(); got != "q" {
		t.Fatalf("compose body = %q, want q", got)
	}
}

func TestTimelineInvitationPromptSavesToSelectedCalendar(t *testing.T) {
	events := testCalendarEvents()
	collections := []models.CalendarCollection{
		{
			Ref: models.CollectionRef{
				SourceID:     "demo-calendar",
				AccountID:    "default",
				Kind:         models.SourceKindCalendar,
				CollectionID: "work",
				DisplayName:  "Work",
			},
		},
		{
			Ref: models.CollectionRef{
				SourceID:     "demo-calendar",
				AccountID:    "default",
				Kind:         models.SourceKindCalendar,
				CollectionID: "family",
				DisplayName:  "Family",
			},
		},
	}
	b := &calendarAgendaStubBackend{available: true, events: events, collections: collections}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.calendarAvailable = true
	m.calendarCollections = collections
	m.timeline.body = &models.EmailBody{
		TextPlain: strings.Join([]string{
			"BEGIN:VCALENDAR",
			"VERSION:2.0",
			"BEGIN:VEVENT",
			"UID:invite-1",
			"SUMMARY:Imported planning",
			"DTSTART:20260530T170000Z",
			"DTEND:20260530T180000Z",
			"END:VEVENT",
			"END:VCALENDAR",
		}, "\r\n"),
	}

	if cmd := m.openCalendarInvitationPrompt(); cmd != nil {
		t.Fatalf("open prompt returned command %T, want prompt-only", cmd)
	}
	if !m.calendarInvitation.Active || len(m.calendarInvitation.Collections) != 2 {
		t.Fatalf("invitation prompt = %#v, want two collection choices", m.calendarInvitation)
	}
	m.calendarInvitation.Cursor = 1
	model, cmd, handled := m.handleCalendarInvitationPromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("expected invitation prompt to handle enter")
	}
	m = model.(*Model)
	if cmd == nil {
		t.Fatal("expected invitation save command")
	}
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.savedEvents) != 1 {
		t.Fatalf("saved events=%d, want 1", len(b.savedEvents))
	}
	if b.savedEvents[0].Ref.CalendarID != "family" || b.savedEvents[0].Title != "Imported planning" {
		t.Fatalf("saved event=%#v, want imported event in family calendar", b.savedEvents[0])
	}
	if m.calendarInvitation.Active {
		t.Fatalf("invitation prompt stayed active after save: %#v", m.calendarInvitation)
	}
}

func TestCalendarPanelContentFitsWarningCollectionLabels(t *testing.T) {
	raw := strings.Join([]string{
		"  \x1b[36m[x]\x1b[0m \x1b[36mFamily ⚠ with a provider label that used to widen the rail\x1b[0m",
		"  \x1b[35m[x]\x1b[0m \x1b[35mReminders ⚠ with a provider label that used to widen the rail\x1b[0m",
	}, "\n")
	rendered := fitCalendarPanelContent(raw, 18, 4)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 4 {
		t.Fatalf("lines=%d, want padded height 4:\n%s", len(lines), rendered)
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > 18 {
			t.Fatalf("line %d width=%d, want <=18: %q", i, got, line)
		}
	}
	visible := ansi.Strip(rendered)
	if !strings.Contains(visible, "Family") || !strings.Contains(visible, "Reminders") {
		t.Fatalf("guard dropped collection labels:\n%s", visible)
	}
}

func TestTimelineInvitationImportUpdatesDuplicateUID(t *testing.T) {
	existing := testCalendarEvents()[0]
	existing.ProviderUID = "invite-1"
	existing.Ref.CalendarID = "work"
	existing.Ref.EventID = "existing-event"
	existing.Ref.LocalID = ""
	existing.Ref = existing.Ref.WithDefaults()
	collections := []models.CalendarCollection{
		{
			Ref: models.CollectionRef{
				SourceID:     "demo-calendar",
				AccountID:    "default",
				Kind:         models.SourceKindCalendar,
				CollectionID: "work",
				DisplayName:  "Work",
			},
		},
	}
	b := &calendarAgendaStubBackend{available: true, events: []models.CalendarEvent{existing}, collections: collections}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.calendarAvailable = true
	m.calendarCollections = collections
	m.calendarEvents = []models.CalendarEvent{existing}
	m.timeline.body = &models.EmailBody{TextPlain: strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:invite-1",
		"SUMMARY:Updated imported planning",
		"DTSTART:20260530T170000Z",
		"DTEND:20260530T180000Z",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")}

	m.openCalendarInvitationPrompt()
	model, cmd, handled := m.handleCalendarInvitationPromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd == nil {
		t.Fatalf("expected handled save command, handled=%v cmd=%T", handled, cmd)
	}
	m = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = m.Update(msg)
		m = model.(*Model)
	}
	if len(b.savedEvents) != 1 {
		t.Fatalf("saved events=%d, want one update", len(b.savedEvents))
	}
	if got, want := b.savedEvents[0].Ref.EventID, "existing-event"; got != want {
		t.Fatalf("saved EventID=%q, want duplicate UID to update %q", got, want)
	}
	if len(b.events) != 1 {
		t.Fatalf("backend events=%d, want duplicate UID update without append", len(b.events))
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

func dateAnchorCalendarEventsForTest() []models.CalendarEvent {
	loc := time.FixedZone("PDT", -7*60*60)
	return []models.CalendarEvent{
		{
			Ref:   models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "month-start"}.WithDefaults(),
			Title: "Month start",
			Start: time.Date(2026, 5, 1, 9, 0, 0, 0, loc),
			End:   time.Date(2026, 5, 1, 10, 0, 0, 0, loc),
		},
		{
			Ref:   models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "selected-current"}.WithDefaults(),
			Title: "Selected current week",
			Start: time.Date(2026, 5, 28, 15, 45, 0, 0, loc),
			End:   time.Date(2026, 5, 28, 17, 45, 0, 0, loc),
		},
		{
			Ref:   models.EventRef{SourceID: "demo-calendar", AccountID: "default", CalendarID: "work", EventID: "next-month"}.WithDefaults(),
			Title: "Next month",
			Start: time.Date(2026, 6, 2, 11, 0, 0, 0, loc),
			End:   time.Date(2026, 6, 2, 12, 0, 0, 0, loc),
		},
	}
}

func setCalendarWeekStartForTest(t *testing.T, cfg *config.Config, value string) {
	t.Helper()
	root := reflect.ValueOf(cfg).Elem()
	calendarField := root.FieldByName("Calendar")
	if !calendarField.IsValid() {
		t.Fatal("config.Config is missing Calendar settings")
	}
	weekStart := calendarField.FieldByName("WeekStart")
	if !weekStart.IsValid() {
		t.Fatal("config.Config.Calendar is missing WeekStart")
	}
	if !weekStart.CanSet() {
		t.Fatal("config.Config.Calendar.WeekStart is not settable")
	}
	weekStart.SetString(value)
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
