package app

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	mouseWheelDelta              = 3
	calendarEventDoubleClickTime = 500 * time.Millisecond
)

type mouseRect struct {
	x int
	y int
	w int
	h int
}

func (r mouseRect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func (m *Model) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.(type) {
	case tea.MouseClickMsg, tea.MouseWheelMsg:
	default:
		return m, nil, false
	}
	mouse := msg.Mouse()
	if m.windowWidth > 0 && (m.windowWidth < minTermWidth || m.windowHeight < minTermHeight) {
		return m, nil, true
	}
	if cmd, handled := m.handleMouseTabClick(mouse); handled {
		return m, cmd, true
	}
	if m.timeline.fullScreen {
		return m.handleTimelinePreviewMouse(mouse)
	}
	if m.showLogs {
		return m, nil, false
	}
	if !m.canInteractWithVisibleData() {
		return m, nil, true
	}

	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	top := m.mouseContentTop()

	if plan.SidebarVisible {
		sidebar := mouseRect{x: 0, y: top, w: sidebarContentWidth + 2, h: m.mousePanelHeight()}
		if sidebar.contains(mouse.X, mouse.Y) {
			return m.handleSidebarMouse(mouse, sidebar)
		}
	}

	switch m.activeTab {
	case tabTimeline:
		return m.handleTimelineMouse(mouse, plan, top)
	case tabCalendar:
		return m.handleCalendarMouse(mouse, plan, top)
	}
	return m, nil, false
}

func (m *Model) handleMouseTabClick(msg tea.Mouse) (tea.Cmd, bool) {
	if msg.Button != tea.MouseLeft || msg.Y != 0 {
		return nil, false
	}
	if !m.canInteractWithVisibleData() {
		return nil, true
	}
	x := m.titleTabStartX()
	for _, item := range m.visibleTopLevelTabNavigation() {
		w := m.tabMouseWidth(item)
		if msg.X >= x && msg.X < x+w {
			switch item.tab {
			case tabTimeline:
				if m.activeTab != tabTimeline {
					return m.switchToTimeline(), true
				}
			case tabContacts:
				if m.activeTab != tabContacts {
					return m.switchToContacts(), true
				}
			case tabCalendar:
				if m.activeTab != tabCalendar {
					return m.switchToCalendar(), true
				}
			}
			return nil, true
		}
		x += w
	}
	return nil, false
}

func (m *Model) mouseContentTop() int {
	if m.hasTopSyncStrip() {
		return 2
	}
	return 1
}

func (m *Model) mousePanelHeight() int {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	return plan.ContentHeight + 2
}

func tableVisibleStart(t *table.Model) int {
	rows := len(t.Rows())
	height := t.Height()
	cursor := t.Cursor()
	if rows == 0 || height <= 0 {
		return 0
	}
	start := cursor - height + 1
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > rows {
		end = rows
		start = end - height
		if start < 0 {
			start = 0
		}
	}
	return start
}

func mouseTableRowAt(t *table.Model, rect mouseRect, y int) (int, bool) {
	rowOffset := y - (rect.y + 2)
	if rowOffset < 0 || rowOffset >= t.Height() {
		return 0, false
	}
	row := tableVisibleStart(t) + rowOffset
	if row < 0 || row >= len(t.Rows()) {
		return 0, false
	}
	return row, true
}

func mouseTableHeaderAt(rect mouseRect, y int) bool {
	return y == rect.y+1
}

func mouseTimelineSortCriterionAt(t *table.Model, rect mouseRect, x int) (timelineSortCriterion, bool) {
	contentX := x - (rect.x + 1)
	if contentX < 0 || contentX >= t.Width() {
		return 0, false
	}
	offset := 0
	for _, col := range t.Columns() {
		if col.Width <= 0 {
			continue
		}
		cellWidth := col.Width + 2
		if contentX >= offset && contentX < offset+cellWidth {
			title := ansi.Strip(col.Title)
			switch {
			case strings.HasPrefix(title, "Sender"):
				return timelineSortCriterionSender, true
			case strings.HasPrefix(title, "Subject"):
				return timelineSortCriterionCount, true
			case strings.HasPrefix(title, "When"):
				return timelineSortCriterionWhen, true
			default:
				return 0, false
			}
		}
		offset += cellWidth
	}
	return 0, false
}

func (m *Model) handleSidebarMouse(msg tea.Mouse, rect mouseRect) (tea.Model, tea.Cmd, bool) {
	if msg.Button != tea.MouseLeft {
		return m, nil, true
	}
	items := m.visibleSidebarItems()
	rowOffset := msg.Y - (rect.y + 1)
	if rowOffset < 0 {
		return m, nil, true
	}
	startIdx := 0
	maxRows := m.windowHeight - 7
	if maxRows < 5 {
		maxRows = 5
	}
	if len(items) > maxRows {
		startIdx = m.sidebarCursor - maxRows + 1
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx+maxRows > len(items) {
			startIdx = len(items) - maxRows
		}
	}
	idx := startIdx + rowOffset
	if idx < 0 || idx >= len(items) {
		return m, nil, true
	}
	m.sidebarCursor = idx
	m.setFocusedPanel(panelSidebar)
	if cmd, handledAccount := m.selectSidebarFolder(); handledAccount {
		m.clearTimelineChatFilter()
		return m, cmd, true
	}
	m.clearTimelineChatFilter()
	return m, m.activateCurrentFolder(), true
}

func mouseIsWheel(msg tea.Mouse) bool {
	return msg.Button == tea.MouseWheelDown || msg.Button == tea.MouseWheelUp
}

func mouseWheelDirection(msg tea.Mouse) int {
	if msg.Button == tea.MouseWheelUp {
		return -1
	}
	if msg.Button == tea.MouseWheelDown {
		return 1
	}
	return 0
}

func (m *Model) toggleMouseCaptureMode() tea.Cmd {
	m.mouseSelectionMode = !m.mouseSelectionMode
	m.timeline.mouseMode = m.mouseSelectionMode
	return nil
}

func (m *Model) scrollTimelinePreview(direction int) {
	if direction < 0 {
		m.timeline.bodyScrollOffset -= mouseWheelDelta
		if m.timeline.bodyScrollOffset < 0 {
			m.timeline.bodyScrollOffset = 0
		}
		return
	}
	if direction > 0 {
		m.timeline.bodyScrollOffset += mouseWheelDelta
	}
}

func (m *Model) handleTimelinePreviewMouse(msg tea.Mouse) (tea.Model, tea.Cmd, bool) {
	if !mouseIsWheel(msg) {
		m.setFocusedPanel(panelPreview)
		return m, nil, true
	}
	m.setFocusedPanel(panelPreview)
	m.scrollTimelinePreview(mouseWheelDirection(msg))
	return m, nil, true
}

func (m *Model) handleTimelineMouse(msg tea.Mouse, plan LayoutPlan, top int) (tea.Model, tea.Cmd, bool) {
	x := 0
	if plan.SidebarVisible {
		x += sidebarContentWidth + 2 + panelGapWidth
	}
	tableRect := mouseRect{x: x, y: top, w: m.timelineTable.Width() + 2, h: m.mousePanelHeight()}
	if tableRect.contains(msg.X, msg.Y) {
		m.setFocusedPanel(panelTimeline)
		if mouseIsWheel(msg) {
			if mouseWheelDirection(msg) > 0 {
				m.timelineTable.MoveDown(mouseWheelDelta)
			} else {
				m.timelineTable.MoveUp(mouseWheelDelta)
			}
			return m, m.maybeUpdatePreview(), true
		}
		if msg.Button == tea.MouseLeft {
			if mouseTableHeaderAt(tableRect, msg.Y) {
				if criterion, ok := mouseTimelineSortCriterionAt(&m.timelineTable, tableRect, msg.X); ok {
					m.setTimelineSortCriterion(criterion)
				}
				return m, nil, true
			}
			if row, ok := mouseTableRowAt(&m.timelineTable, tableRect, msg.Y); ok {
				m.timelineTable.SetCursor(row)
				return m, m.activateCurrentTimelineRowFromMouse(), true
			}
			return m, nil, true
		}
	}
	if m.timeline.selectedEmail != nil {
		previewRect := mouseRect{
			x: tableRect.x + tableRect.w + panelGapWidth,
			y: top,
			w: m.timeline.previewWidth,
			h: m.mousePanelHeight(),
		}
		if previewRect.contains(msg.X, msg.Y) {
			return m.handleTimelinePreviewMouse(msg)
		}
	}
	return m, nil, false
}

func (m *Model) calendarMousePanelWidths(plan LayoutPlan) (int, int, int) {
	contentW := m.windowWidth
	if contentW <= 0 {
		contentW = 80
	}
	if plan.ChatVisible {
		contentW -= chatPanelWidth + 2 + panelGapWidth
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
	mainMin := 26
	switch m.calendarView {
	case calendarViewWeek:
		mainMin = 28
	case calendarViewSearch, calendarViewCrossSearch:
		mainMin = 28
	}
	if remaining < mainMin+24 {
		remaining = available
		railW = 0
	}
	mainW, detailW := splitWidth(remaining, 0, mainMin, 24, remaining*56/100)
	return railW, mainW, detailW
}

func (m *Model) handleCalendarMouse(msg tea.Mouse, plan LayoutPlan, top int) (tea.Model, tea.Cmd, bool) {
	if !m.calendarAvailable || m.calendarDetailOpen || m.calendarEdit.Active ||
		m.calendarMeetingPrepOpen || m.calendarTravelBufferOpen || m.calendarAISummaryOpen {
		return m, nil, false
	}
	railW, mainW, detailW := m.calendarMousePanelWidths(plan)
	contentH := calendarPanelOuterHeight(plan)
	x := 0
	if railW > 0 {
		railRect := mouseRect{x: x, y: top, w: railW, h: contentH}
		if railRect.contains(msg.X, msg.Y) {
			return m.handleCalendarRailMouse(msg, railRect)
		}
		x += railW + panelGapWidth
	}
	mainRect := mouseRect{x: x, y: top, w: mainW, h: contentH}
	if mainRect.contains(msg.X, msg.Y) {
		return m.handleCalendarMainMouse(msg, mainRect)
	}
	x += mainW + panelGapWidth
	detailRect := mouseRect{x: x, y: top, w: detailW, h: contentH}
	if detailRect.contains(msg.X, msg.Y) {
		m.calendarFocus = calendarFocusDetail
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) handleCalendarRailMouse(msg tea.Mouse, rect mouseRect) (tea.Model, tea.Cmd, bool) {
	if msg.Button != tea.MouseLeft {
		return m, nil, true
	}
	m.calendarFocus = calendarFocusRail
	contentX := msg.X - (rect.x + 2)
	contentY := msg.Y - (rect.y + 1)
	if day, ok := m.calendarMiniMonthDayAt(contentX, contentY); ok {
		m.selectCalendarDayFromMouse(day)
		return m, nil, true
	}
	if idx, ok := m.calendarCollectionAtRailContentY(contentY); ok {
		m.calendarRailCursor = idx
		m.toggleFocusedCalendarCollection()
		return m, nil, true
	}
	return m, nil, true
}

func (m *Model) calendarMiniMonthDayAt(contentX, contentY int) (time.Time, bool) {
	if contentY < 4 || contentY > 9 || contentX < 0 {
		return time.Time{}, false
	}
	dayIdx := contentX / 3
	if dayIdx < 0 || dayIdx > 6 || contentX%3 == 2 {
		return time.Time{}, false
	}
	start, _, _ := m.calendarActiveRange()
	if start.IsZero() {
		start = m.calendarAnchorDate()
	}
	month := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	gridStart := m.calendarWeekStartFor(month)
	return gridStart.AddDate(0, 0, (contentY-4)*7+dayIdx), true
}

func (m *Model) selectCalendarDayFromMouse(day time.Time) {
	day = calendarDayStartFor(day)
	m.calendarDay = day
	switch m.calendarView {
	case calendarViewWeek:
		m.calendarWeekStart = m.calendarWeekStartFor(day)
		m.selectFirstCalendarEventForWeek(m.calendarWeekStart)
	case calendarViewThreeDay:
		m.calendarThreeDayStart = day
		m.selectFirstCalendarEventForThreeDay(m.calendarThreeDayStart)
	case calendarViewAgenda, "":
		m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(day)
		m.selectFirstCalendarEventForDay(day)
	default:
		m.calendarDay = day
		m.selectFirstCalendarEventForDay(day)
	}
	m.invalidateSettingsBackdrop()
}

func (m *Model) calendarCollectionAtRailContentY(contentY int) (int, bool) {
	if contentY < 0 {
		return 0, false
	}
	line := 0
	line += 1 // Today control.
	line += 1 // Rule.
	line += 8 // Mini month header, weekday header, six weeks.
	line += 1 // Rule.
	line += 1 // Calendars label.
	lastGroup := ""
	for i, collection := range m.calendarCollections {
		group := calendarCollectionGroupLabel(collection)
		if group != lastGroup {
			if line > 1 {
				line++
			}
			if contentY == line {
				return 0, false
			}
			line++
			lastGroup = group
		}
		if contentY == line {
			return i, true
		}
		line++
	}
	return 0, false
}

func (m *Model) handleCalendarMainMouse(msg tea.Mouse, rect mouseRect) (tea.Model, tea.Cmd, bool) {
	m.calendarFocus = calendarFocusMain
	if mouseIsWheel(msg) {
		m.moveCalendarSelectionPage(mouseWheelDirection(msg))
		return m, nil, true
	}
	if msg.Button != tea.MouseLeft {
		return m, nil, true
	}
	contentX := msg.X - (rect.x + 2)
	contentY := msg.Y - (rect.y + 1)
	if item, ok := m.calendarEventAtMainContentPoint(contentX, contentY, clamp(rect.w-4, 1)); ok {
		now := time.Now()
		eventKey := calendarMouseEventKey(item.event)
		doubleClick := eventKey != "" &&
			eventKey == m.calendarLastClickEventKey &&
			now.Sub(m.calendarLastClickAt) <= calendarEventDoubleClickTime
		m.calendarCursor = item.index
		m.calendarDetail = &item.event
		m.calendarLastClickEventKey = eventKey
		m.calendarLastClickAt = now
		if doubleClick {
			return m, m.openCalendarDetail(), true
		}
		return m, nil, true
	}
	m.calendarLastClickEventKey = ""
	m.calendarLastClickAt = time.Time{}
	return m, nil, true
}

func calendarMouseEventKey(event models.CalendarEvent) string {
	ref := event.Ref.WithDefaults()
	if ref.LocalID != "" {
		return ref.LocalID
	}
	if event.ProviderUID != "" {
		return event.ProviderUID
	}
	return event.Title + "|" + event.Start.UTC().Format(time.RFC3339Nano)
}

func (m *Model) calendarEventAtMainContentPoint(contentX, contentY, width int) (indexedCalendarEvent, bool) {
	switch m.calendarView {
	case calendarViewDay:
		return m.calendarTimeGridEventAtContentPoint(contentX, contentY, width, m.selectedCalendarDay(), 1, m.calendarEventsForDay(m.selectedCalendarDay()))
	case calendarViewWeek:
		start := m.selectedCalendarWeekStart()
		return m.calendarTimeGridEventAtContentPoint(contentX, contentY, width, start, 7, m.calendarEventsForWeek(start))
	case calendarViewThreeDay:
		start := m.selectedCalendarThreeDayStart()
		return m.calendarTimeGridEventAtContentPoint(contentX, contentY, width, start, 3, m.calendarEventsForThreeDay(start))
	default:
		return m.calendarAgendaEventAtContentY(contentY)
	}
}

func (m *Model) calendarAgendaEventAtContentY(contentY int) (indexedCalendarEvent, bool) {
	if contentY < 0 {
		return indexedCalendarEvent{}, false
	}
	line := 0
	if status := m.visibleCalendarStatus(); status != "" {
		_ = status
		line++
	}
	if hiddenPast := m.calendarAgendaHiddenPastCount(); hiddenPast > 0 {
		line += len(m.calendarAgendaPastNoticeLines(hiddenPast))
	}
	visibleEvents := m.indexedVisibleCalendarEvents()
	selectedOffset := m.calendarVisibleOffset()
	maxRows := m.calendarPanelInnerHeight() - line
	if maxRows < 1 {
		maxRows = 1
	}
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
			if contentY == line {
				return indexedCalendarEvent{}, false
			}
			line++
			lastDay = day
		}
		if contentY == line {
			return item, true
		}
		line++
	}
	return indexedCalendarEvent{}, false
}

func (m *Model) calendarPanelInnerHeight() int {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	return clamp(calendarPanelOuterHeight(plan)-2, 1)
}

func (m *Model) calendarTimeGridEventAtContentPoint(contentX, contentY, width int, start time.Time, dayCount int, events []indexedCalendarEvent) (indexedCalendarEvent, bool) {
	if contentY < 2 || contentX < 7 || dayCount < 1 || len(events) == 0 {
		return indexedCalendarEvent{}, false
	}
	timeW := 7
	dayW := (width - timeW - (dayCount - 1)) / dayCount
	if dayW < 4 {
		dayW = 4
	}
	dayIdx := (contentX - timeW) / (dayW + 1)
	if dayIdx < 0 || dayIdx >= dayCount {
		return indexedCalendarEvent{}, false
	}
	row := contentY - 2
	startHour, endHour := calendarVisibleHourRange(events)
	showHalfHours := calendarWeekGridShouldShowHalfHours(m.calendarPanelInnerHeight(), startHour, endHour)
	rowsPerHour := 1
	if showHalfHours {
		rowsPerHour = 2
	}
	hour := startHour + row/rowsPerHour
	minute := 0
	if showHalfHours && row%2 == 1 {
		minute = 30
	}
	day := start.AddDate(0, 0, dayIdx)
	event, _ := m.calendarEventInSlot(day, hour, minute)
	if event == nil {
		return indexedCalendarEvent{}, false
	}
	ref := event.Ref.WithDefaults()
	for _, item := range events {
		if item.event.Ref.WithDefaults().LocalID == ref.LocalID {
			return item, true
		}
	}
	return indexedCalendarEvent{}, false
}
