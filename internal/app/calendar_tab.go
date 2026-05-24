package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func (m *Model) refreshCalendarAvailability() {
	agenda, ok := m.backend.(backend.CalendarAgendaBackend)
	m.calendarAvailable = ok && agenda.CalendarAgendaAvailable()
	if !m.calendarAvailable {
		m.calendarEvents = nil
		m.calendarDetail = nil
		m.calendarDetailOpen = false
		m.calendarLoading = false
		m.calendarDetailLoading = false
	}
}

func (m *Model) calendarAgendaBackend() (backend.CalendarAgendaBackend, bool) {
	agenda, ok := m.backend.(backend.CalendarAgendaBackend)
	if !ok || !agenda.CalendarAgendaAvailable() {
		return nil, false
	}
	return agenda, true
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

func (m *Model) selectedCalendarEvent() *models.CalendarEvent {
	if m.calendarCursor < 0 || m.calendarCursor >= len(m.calendarEvents) {
		return nil
	}
	event := m.calendarEvents[m.calendarCursor]
	return &event
}

func (m *Model) openCalendarDetail() tea.Cmd {
	event := m.selectedCalendarEvent()
	if event == nil {
		return nil
	}
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

func (m *Model) handleCalendarKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := shortcutKey(msg)
	switch key {
	case "j", "down":
		if !m.calendarDetailOpen && m.calendarCursor < len(m.calendarEvents)-1 {
			m.calendarCursor++
			m.calendarDetail = m.selectedCalendarEvent()
		}
		return m, nil
	case "k", "up":
		if !m.calendarDetailOpen && m.calendarCursor > 0 {
			m.calendarCursor--
			m.calendarDetail = m.selectedCalendarEvent()
		}
		return m, nil
	case "enter":
		if !m.calendarDetailOpen {
			return m, m.openCalendarDetail()
		}
		return m, nil
	case "esc":
		if m.calendarDetailOpen {
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
	if m.calendarDetailOpen {
		return m.renderCalendarDetailFullView()
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

func (m *Model) renderCalendarEventDetail(width, height int, full bool) string {
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
	header := "Event Detail"
	if !full {
		header = "Event Detail"
	}
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render(calendarFit(header, width)))
	if m.calendarDetailLoading {
		lines = append(lines, m.theme.Text.Dim.Style().Render(calendarFit("Loading latest cached detail...", width)))
	}
	lines = append(lines, "")
	lines = append(lines, m.theme.Metadata.Subject.Style().Render(calendarFit(event.Title, width)))
	lines = append(lines, calendarDetailRow(m, "Time", calendarTimeRange(*event), width))
	if strings.TrimSpace(event.Location) != "" {
		lines = append(lines, calendarDetailRow(m, "Location", event.Location, width))
	}
	lines = append(lines, calendarDetailRow(m, "Status", calendarStatusLabel(*event), width))
	lines = append(lines, calendarDetailRow(m, "Calendar", calendarSourceLabel(*event), width))
	lines = append(lines, calendarDetailRow(m, "Mode", "read-only", width))
	if strings.TrimSpace(event.Description) != "" {
		lines = append(lines, "")
		lines = append(lines, m.theme.Metadata.Label.Style().Render(calendarFit("Notes", width)))
		for _, line := range wrapText(event.Description, width) {
			lines = append(lines, m.theme.Text.Primary.Style().Render(calendarFit(line, width)))
		}
	}
	return fitPanelContentHeight(strings.Join(lines, "\n"), height)
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

func calendarDetailRow(m *Model, label, value string, width int) string {
	label = calendarFit(label+":", 10)
	valueW := width - ansi.StringWidth(label) - 1
	if valueW < 4 {
		valueW = 4
	}
	return m.theme.Metadata.Label.Style().Render(label) + " " + m.theme.Text.Primary.Style().Render(calendarFit(value, valueW))
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

func sameCalendarDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
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
