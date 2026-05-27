package app

import (
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const mouseWheelDelta = 3

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
	m.timeline.mouseMode = !m.timeline.mouseMode
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
