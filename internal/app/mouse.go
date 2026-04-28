package app

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
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
	if msg.Action != tea.MouseActionPress {
		return m, nil, false
	}
	if m.windowWidth > 0 && (m.windowWidth < minTermWidth || m.windowHeight < minTermHeight) {
		return m, nil, true
	}
	if cmd, handled := m.handleMouseTabClick(msg); handled {
		return m, cmd, true
	}
	if m.timeline.fullScreen {
		return m.handleTimelinePreviewMouse(msg)
	}
	if m.cleanupFullScreen {
		return m.handleCleanupPreviewMouse(msg)
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
		if sidebar.contains(msg.X, msg.Y) {
			return m.handleSidebarMouse(msg, sidebar)
		}
	}

	switch m.activeTab {
	case tabTimeline:
		return m.handleTimelineMouse(msg, plan, top)
	case tabCleanup:
		return m.handleCleanupMouse(msg, plan, top)
	}
	return m, nil, false
}

func (m *Model) handleMouseTabClick(msg tea.MouseMsg) (tea.Cmd, bool) {
	if msg.Button != tea.MouseButtonLeft || msg.Y != 1 {
		return nil, false
	}
	if !m.canInteractWithVisibleData() {
		return nil, true
	}
	tabWidths := []struct {
		tab int
		w   int
	}{
		{tabTimeline, len("1  Timeline") + 4},
		{tabCompose, len("2  Compose") + 4},
		{tabCleanup, len("3  Cleanup") + 4},
		{tabContacts, len("4  Contacts") + 4},
	}
	x := 0
	for _, item := range tabWidths {
		if msg.X >= x && msg.X < x+item.w {
			switch item.tab {
			case tabTimeline:
				if m.activeTab != tabTimeline {
					return m.switchToTimeline(), true
				}
			case tabCompose:
				if m.activeTab != tabCompose {
					return m.switchToCompose(), true
				}
			case tabCleanup:
				if m.activeTab != tabCleanup {
					return m.switchToCleanup(), true
				}
			case tabContacts:
				if m.activeTab != tabContacts {
					return m.switchToContacts(), true
				}
			}
			return nil, true
		}
		x += item.w
	}
	return nil, false
}

func (m *Model) mouseContentTop() int {
	if m.hasTopSyncStrip() {
		return 3
	}
	return 2
}

func (m *Model) mousePanelHeight() int {
	h := m.windowHeight - 7
	if m.hasTopSyncStrip() {
		h--
	}
	if h < 5 {
		return 5
	}
	return h + 2
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

func (m *Model) handleSidebarMouse(msg tea.MouseMsg, rect mouseRect) (tea.Model, tea.Cmd, bool) {
	if msg.Button != tea.MouseButtonLeft {
		return m, nil, true
	}
	items := flattenTree(m.folderTree)
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
	m.selectSidebarFolder()
	m.clearTimelineChatFilter()
	return m, m.activateCurrentFolder(), true
}

func mouseIsWheel(msg tea.MouseMsg) bool {
	return msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp
}

func mouseWheelDirection(msg tea.MouseMsg) int {
	if msg.Button == tea.MouseButtonWheelUp {
		return -1
	}
	if msg.Button == tea.MouseButtonWheelDown {
		return 1
	}
	return 0
}

func (m *Model) toggleMouseCaptureMode() tea.Cmd {
	m.timeline.mouseMode = !m.timeline.mouseMode
	if m.timeline.mouseMode {
		return tea.DisableMouse
	}
	return tea.EnableMouseCellMotion
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

func (m *Model) scrollCleanupPreview(direction int) {
	if direction < 0 {
		m.cleanupBodyScrollOffset -= mouseWheelDelta
		if m.cleanupBodyScrollOffset < 0 {
			m.cleanupBodyScrollOffset = 0
		}
		return
	}
	if direction > 0 {
		m.cleanupBodyScrollOffset += mouseWheelDelta
	}
}

func (m *Model) handleTimelinePreviewMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if !mouseIsWheel(msg) {
		m.setFocusedPanel(panelPreview)
		return m, nil, true
	}
	m.setFocusedPanel(panelPreview)
	m.scrollTimelinePreview(mouseWheelDirection(msg))
	return m, nil, true
}

func (m *Model) handleCleanupPreviewMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if !mouseIsWheel(msg) {
		m.setFocusedPanel(panelDetails)
		return m, nil, true
	}
	m.setFocusedPanel(panelDetails)
	m.scrollCleanupPreview(mouseWheelDirection(msg))
	return m, nil, true
}

func (m *Model) handleTimelineMouse(msg tea.MouseMsg, plan LayoutPlan, top int) (tea.Model, tea.Cmd, bool) {
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
		if msg.Button == tea.MouseButtonLeft {
			if row, ok := mouseTableRowAt(&m.timelineTable, tableRect, msg.Y); ok {
				m.timelineTable.SetCursor(row)
				return m, m.openCurrentTimelineEmail(), true
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

func (m *Model) handleCleanupMouse(msg tea.MouseMsg, plan LayoutPlan, top int) (tea.Model, tea.Cmd, bool) {
	x := 0
	if plan.SidebarVisible {
		x += sidebarContentWidth + 2 + panelGapWidth
	}

	if !(m.showCleanupPreview && plan.Cleanup.SummaryWidth == 0) {
		summaryRect := mouseRect{x: x, y: top, w: m.summaryTable.Width() + 2, h: m.mousePanelHeight()}
		if summaryRect.contains(msg.X, msg.Y) {
			m.setFocusedPanel(panelSummary)
			if mouseIsWheel(msg) {
				if mouseWheelDirection(msg) > 0 {
					m.summaryTable.MoveDown(mouseWheelDelta)
				} else {
					m.summaryTable.MoveUp(mouseWheelDelta)
				}
				m.updateDetailsTable()
				return m, nil, true
			}
			if msg.Button == tea.MouseButtonLeft {
				if row, ok := mouseTableRowAt(&m.summaryTable, summaryRect, msg.Y); ok {
					m.summaryTable.SetCursor(row)
					m.updateDetailsTable()
				}
				return m, nil, true
			}
		}
		x += summaryRect.w + panelGapWidth
	}

	detailsRect := mouseRect{x: x, y: top, w: m.detailsTable.Width() + 2, h: m.mousePanelHeight()}
	if detailsRect.contains(msg.X, msg.Y) {
		m.setFocusedPanel(panelDetails)
		if mouseIsWheel(msg) {
			if mouseWheelDirection(msg) > 0 {
				m.detailsTable.MoveDown(mouseWheelDelta)
			} else {
				m.detailsTable.MoveUp(mouseWheelDelta)
			}
			return m, nil, true
		}
		if msg.Button == tea.MouseButtonLeft {
			if row, ok := mouseTableRowAt(&m.detailsTable, detailsRect, msg.Y); ok {
				m.detailsTable.SetCursor(row)
				if row < len(m.detailsEmails) {
					return m, m.openCleanupPreviewEmail(m.detailsEmails[row]), true
				}
			}
			return m, nil, true
		}
	}

	if m.showCleanupPreview {
		previewRect := mouseRect{x: detailsRect.x + detailsRect.w + panelGapWidth, y: top, w: m.cleanupPreviewWidth, h: m.mousePanelHeight()}
		if previewRect.contains(msg.X, msg.Y) {
			return m.handleCleanupPreviewMouse(msg)
		}
	}
	return m, nil, false
}

func (m *Model) openCleanupPreviewEmail(email *models.EmailData) tea.Cmd {
	if email == nil {
		return nil
	}
	m.cleanupPreviewEmail = email
	m.showCleanupPreview = true
	m.cleanupBodyLoading = true
	m.cleanupEmailBody = nil
	m.cleanupBodyScrollOffset = 0
	m.cleanupBodyWrappedLines = nil
	m.cleanupPreviewHadSidebar = m.showSidebar
	m.showSidebar = false
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	return fetchCleanupBodyCmd(m.backend, email)
}
