package app

import (
	"fmt"
	"html"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
	emailrender "github.com/herald-email/herald-mail-app/internal/render"
)

type previewSelectionSurface string

const (
	previewSelectionNone            previewSelectionSurface = ""
	previewSelectionTimeline        previewSelectionSurface = "timeline"
	previewSelectionContacts        previewSelectionSurface = "contacts"
	previewSelectionComposeOriginal previewSelectionSurface = "compose-original"
)

type previewSelectionPoint struct {
	Row int
	Col int
}

type previewSelectableRow struct {
	Plain    string
	Rendered string
	HTML     string
	Image    *models.InlineImage
	ImageAlt string
}

type previewSelectionState struct {
	Active       bool
	Dragging     bool
	Mouse        bool
	Surface      previewSelectionSurface
	Selecting    bool
	Anchor       previewSelectionPoint
	Cursor       previewSelectionPoint
	PreferredCol int
}

func (s *previewSelectionState) reset() {
	*s = previewSelectionState{}
}

func (m *Model) clearPreviewSelection() {
	m.previewSelection.reset()
	m.timeline.visualMode = false
	m.timeline.pendingY = false
}

func (m *Model) beginPreviewSelection(surface previewSelectionSurface, row, col int, rows []previewSelectableRow) bool {
	m.previewSelection.begin(surface, row, col, rows)
	m.timeline.visualMode = m.previewSelection.selectingOn(previewSelectionTimeline)
	m.syncTimelineVisualCompatibility()
	m.timeline.pendingY = false
	return m.previewSelection.Active
}

func (m *Model) syncTimelineVisualCompatibility() {
	if !m.previewSelection.selectingOn(previewSelectionTimeline) {
		m.timeline.visualMode = false
		return
	}
	m.timeline.visualStart = m.previewSelection.Anchor.Row
	m.timeline.visualEnd = m.previewSelection.Cursor.Row
}

func (m *Model) activePreviewSelectionSurface() previewSelectionSurface {
	switch {
	case m.activeTab == tabCompose && m.composeField == composeFieldOriginalMessage && m.composePreserved != nil:
		return previewSelectionComposeOriginal
	case m.activeTab == tabContacts && m.contactPreviewEmail != nil:
		return previewSelectionContacts
	case m.activeTab == tabTimeline && (m.timeline.fullScreen || m.focusedPanel == panelPreview) && m.timeline.selectedEmail != nil:
		return previewSelectionTimeline
	default:
		return previewSelectionNone
	}
}

func (m *Model) previewRowsForSurface(surface previewSelectionSurface) []previewSelectableRow {
	switch surface {
	case previewSelectionTimeline:
		return m.timelinePreviewSelectableRows()
	case previewSelectionContacts:
		return m.contactsPreviewSelectableRows()
	case previewSelectionComposeOriginal:
		return m.composeOriginalSelectableRows()
	default:
		return nil
	}
}

func (m *Model) previewScrollForSurface(surface previewSelectionSurface) *int {
	switch surface {
	case previewSelectionTimeline:
		return &m.timeline.bodyScrollOffset
	case previewSelectionContacts:
		return &m.contactPreviewScroll
	case previewSelectionComposeOriginal:
		if m.composePreserved == nil {
			return nil
		}
		return &m.composePreserved.originalScrollOffset
	default:
		return nil
	}
}

func (m *Model) previewVisibleRowsForSurface(surface previewSelectionSurface) int {
	switch surface {
	case previewSelectionTimeline:
		if m.timeline.fullScreen {
			_, rows := m.timelineFullScreenDocumentBudget()
			return rows
		}
		if len(m.timeline.bodyWrappedLines) > 0 {
			return len(m.timeline.bodyWrappedLines)
		}
		return maxInt(1, m.timelineTable.Height())
	case previewSelectionContacts:
		plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
		rows := plan.ContentHeight - 6
		if rows < 1 {
			rows = 1
		}
		return rows
	case previewSelectionComposeOriginal:
		rows := m.composeOriginalPreviewRows(m.composeContentHeight()) - 2
		if rows < 1 {
			rows = 1
		}
		return rows
	default:
		return 1
	}
}

func (m *Model) timelinePreviewSelectableRows() []previewSelectableRow {
	if m.timeline.body == nil {
		return nil
	}
	layout := m.timelineSelectionDocumentLayout()
	return selectableRowsFromPreviewLayout(layout)
}

func (m *Model) timelineSelectionDocumentLayout() previewDocumentLayout {
	if m.timeline.fullScreen {
		return m.timelineFullScreenDocumentLayout()
	}
	if m.timeline.previewDocLayout != nil {
		return *m.timeline.previewDocLayout
	}
	innerW := m.timeline.bodyWrappedWidth
	if innerW <= 0 {
		innerW = m.timeline.previewWidth - 4
	}
	if innerW < 10 {
		innerW = 10
	}
	visibleRows := len(m.timeline.bodyWrappedLines)
	if visibleRows < 1 {
		visibleRows = maxInt(1, m.timelineTable.Height())
	}
	return m.timelinePreviewDocumentLayout(innerW, visibleRows)
}

func (m *Model) contactsPreviewSelectableRows() []previewSelectableRow {
	if m.contactPreviewBody == nil {
		return nil
	}
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	rightInnerW := plan.Contacts.DetailWidth - 4
	if rightInnerW < 10 {
		rightInnerW = 10
	}
	body := stripInvisibleChars(emailrender.EmailBodyMarkdown(m.contactPreviewBody))
	if body == "" {
		body = "(No text content)"
	}
	return plainSelectableRows(renderEmailBodyLines(body, rightInnerW))
}

func (m *Model) composeOriginalSelectableRows() []previewSelectableRow {
	if m.composePreserved == nil {
		return nil
	}
	width := m.composeOriginalSelectionWidth()
	return plainSelectableRows(m.composeOriginalMessageRows(width, 9999))
}

func (m *Model) composeOriginalSelectionWidth() int {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	width := plan.Compose.BodyInnerWidth
	if width < 20 {
		width = m.windowWidth - 4
	}
	if width < 20 {
		width = 20
	}
	return width
}

func (m *Model) moveActivePreviewSelection(dRow, dCol int) bool {
	surface := m.activePreviewSelectionSurface()
	if !m.previewSelection.activeOn(surface) {
		return false
	}
	rows := m.previewRowsForSurface(surface)
	m.previewSelection.move(rows, dRow, dCol)
	if scroll := m.previewScrollForSurface(surface); scroll != nil {
		m.previewSelection.ensureCursorVisible(scroll, m.previewVisibleRowsForSurface(surface), len(rows))
	}
	m.timeline.visualMode = m.previewSelection.selectingOn(previewSelectionTimeline)
	m.syncTimelineVisualCompatibility()
	return true
}

func (m *Model) beginPreviewMouseSelection(surface previewSelectionSurface, row, colCells int, rows []previewSelectableRow) bool {
	if surface == previewSelectionNone || len(rows) == 0 || row < 0 || row >= len(rows) {
		return false
	}
	col := previewColumnToRuneIndex(rows[row].Plain, colCells)
	point := previewSelectionPoint{Row: row, Col: col}
	m.previewSelection = previewSelectionState{
		Active:       true,
		Dragging:     true,
		Mouse:        true,
		Surface:      surface,
		Selecting:    true,
		Anchor:       point,
		Cursor:       point,
		PreferredCol: col,
	}
	m.timeline.visualMode = false
	m.timeline.pendingY = false
	return true
}

func (m *Model) updatePreviewMouseSelection(surface previewSelectionSurface, row, colCells int, rows []previewSelectableRow) bool {
	if !m.previewSelection.activeOn(surface) || len(rows) == 0 {
		return false
	}
	row = clampInt(row, 0, len(rows)-1)
	col := previewColumnToRuneIndex(rows[row].Plain, colCells)
	m.previewSelection.Cursor = previewSelectionPoint{Row: row, Col: col}
	m.previewSelection.PreferredCol = col
	m.previewSelection.Selecting = true
	return true
}

func (m *Model) finishPreviewMouseSelection(surface previewSelectionSurface, row, colCells int, rows []previewSelectableRow) bool {
	if !m.previewSelection.activeOn(surface) {
		return false
	}
	if len(rows) > 0 {
		m.updatePreviewMouseSelection(surface, row, colCells, rows)
	}
	m.previewSelection.Dragging = false
	return true
}

func (m *Model) activePreviewSelectionPlainText() string {
	if !m.previewSelection.Active {
		return ""
	}
	rows := m.previewRowsForSelectionSurface(m.previewSelection.Surface)
	return previewSelectionPlain(rows, m.previewSelection)
}

func (m *Model) activePreviewSelectionAllPlainText(surface previewSelectionSurface) string {
	rows := m.previewRowsForSelectionSurface(surface)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Plain)
	}
	return strings.Join(out, "\n")
}

func (m *Model) previewRowsForSelectionSurface(surface previewSelectionSurface) []previewSelectableRow {
	switch surface {
	case previewSelectionTimeline:
		if m.timeline.fullScreen {
			return m.timelineFullScreenSelectableRows()
		}
		return m.timelineSplitPreviewSelectableRows()
	case previewSelectionContacts:
		plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
		return m.contactsRightPanelSelectableRows(plan.Contacts.DetailWidth, plan.ContentHeight)
	default:
		return nil
	}
}

func (m *Model) togglePreviewSelectionForSurface(surface previewSelectionSurface) bool {
	if surface == previewSelectionNone {
		return false
	}
	if m.previewSelection.activeOn(surface) {
		m.previewSelection.toggleSelecting()
		m.timeline.visualMode = m.previewSelection.selectingOn(previewSelectionTimeline)
		m.syncTimelineVisualCompatibility()
		m.timeline.pendingY = false
		return true
	}
	rows := m.previewRowsForSurface(surface)
	if len(rows) == 0 {
		return false
	}
	row := 0
	if scroll := m.previewScrollForSurface(surface); scroll != nil {
		row = *scroll
	}
	return m.beginPreviewSelection(surface, row, 0, rows)
}

func (m *Model) previewClipboardPayloadForCurrentLine(surface previewSelectionSurface) (previewClipboardPayload, bool) {
	rows := m.previewRowsForSurface(surface)
	if len(rows) == 0 {
		return previewClipboardPayload{}, false
	}
	row := 0
	if m.previewSelection.activeOn(surface) {
		row = m.previewSelection.Cursor.Row
	} else if scroll := m.previewScrollForSurface(surface); scroll != nil {
		row = *scroll
	}
	row = clampInt(row, 0, len(rows)-1)
	return previewClipboardPayloadForRows([]previewSelectableRow{rows[row]}, previewSelectionState{
		Active:    true,
		Surface:   surface,
		Selecting: true,
		Anchor:    previewSelectionPoint{Row: 0, Col: 0},
		Cursor:    previewSelectionPoint{Row: 0, Col: maxInt(0, previewSelectableRowLen(rows[row])-1)},
	}), true
}

func (m *Model) previewClipboardPayloadForSelection(surface previewSelectionSurface) (previewClipboardPayload, bool) {
	rows := m.previewRowsForClipboardSelection(surface)
	if len(rows) == 0 || !m.previewSelection.selectingOn(surface) {
		return previewClipboardPayload{}, false
	}
	return previewClipboardPayloadForRows(rows, m.previewSelection), true
}

func (m *Model) previewRowsForClipboardSelection(surface previewSelectionSurface) []previewSelectableRow {
	if m.previewSelection.activeOn(surface) && m.previewSelection.Mouse {
		return m.previewRowsForSelectionSurface(surface)
	}
	return m.previewRowsForSurface(surface)
}

func (m *Model) previewClipboardPayloadForCurrentImage(surface previewSelectionSurface) (previewClipboardPayload, bool) {
	rows := m.previewRowsForSurface(surface)
	if len(rows) == 0 || !m.previewSelection.activeOn(surface) {
		return previewClipboardPayload{}, false
	}
	row := clampInt(m.previewSelection.Cursor.Row, 0, len(rows)-1)
	if rows[row].Image == nil {
		return previewClipboardPayload{}, false
	}
	return previewClipboardPayloadForRows([]previewSelectableRow{rows[row]}, previewSelectionState{
		Active:    true,
		Surface:   surface,
		Selecting: true,
		Anchor:    previewSelectionPoint{Row: 0, Col: 0},
		Cursor:    previewSelectionPoint{Row: 0, Col: maxInt(0, previewSelectableRowLen(rows[row])-1)},
	}), true
}

func (m *Model) previewClipboardPayloadForAll(surface previewSelectionSurface) (previewClipboardPayload, bool) {
	rows := m.previewRowsForSurface(surface)
	if len(rows) == 0 {
		return previewClipboardPayload{}, false
	}
	return previewClipboardPayload{Plain: previewAllPlain(rows), HTML: previewAllHTML(rows)}, true
}

func previewClipboardPayloadForRows(rows []previewSelectableRow, sel previewSelectionState) previewClipboardPayload {
	if len(rows) == 1 && rows[0].Image != nil {
		image := *rows[0].Image
		return previewClipboardPayload{
			Plain:     rows[0].Plain,
			HTML:      rows[0].HTML,
			Image:     &image,
			ImageName: firstNonEmptyString(rows[0].ImageAlt, image.ContentID, "herald-preview-image"),
		}
	}
	return previewClipboardPayload{
		Plain: previewSelectionPlain(rows, sel),
		HTML:  previewSelectionHTML(rows, sel),
	}
}

func (m *Model) handlePreviewCopyKey(surface previewSelectionSurface, key string) (tea.Cmd, bool) {
	if surface == previewSelectionNone {
		return nil, false
	}
	switch key {
	case "y":
		if m.timeline.pendingY {
			m.timeline.pendingY = false
			if payload, ok := m.previewClipboardPayloadForCurrentLine(surface); ok {
				return copyPreviewPayloadToClipboard(payload), true
			}
			m.statusMessage = "No preview line to copy"
			return nil, true
		}
		if m.previewSelection.selectingOn(surface) {
			payload, ok := m.previewClipboardPayloadForSelection(surface)
			m.clearPreviewSelection()
			if ok {
				return copyPreviewPayloadToClipboard(payload), true
			}
			m.statusMessage = "No preview selection to copy"
			return nil, true
		}
		if m.previewSelection.activeOn(surface) {
			if payload, ok := m.previewClipboardPayloadForCurrentImage(surface); ok {
				m.clearPreviewSelection()
				return copyPreviewPayloadToClipboard(payload), true
			}
			m.timeline.pendingY = true
			m.statusMessage = "Press y again to copy this line, or v to start selection"
			return nil, true
		}
		m.timeline.pendingY = true
		return nil, true
	case "Y":
		m.timeline.pendingY = false
		if payload, ok := m.previewClipboardPayloadForAll(surface); ok {
			return copyPreviewPayloadToClipboard(payload), true
		}
		m.statusMessage = "No preview text to copy"
		return nil, true
	default:
		return nil, false
	}
}

func (s previewSelectionState) activeOn(surface previewSelectionSurface) bool {
	return s.Active && s.Surface == surface
}

func (s previewSelectionState) selectingOn(surface previewSelectionSurface) bool {
	return s.activeOn(surface) && s.Selecting
}

func (s *previewSelectionState) begin(surface previewSelectionSurface, row, col int, rows []previewSelectableRow) {
	if len(rows) == 0 {
		return
	}
	row = clampInt(row, 0, len(rows)-1)
	col = clampInt(col, 0, previewSelectableRowLen(rows[row]))
	p := previewSelectionPoint{Row: row, Col: col}
	*s = previewSelectionState{
		Active:       true,
		Surface:      surface,
		Selecting:    false,
		Anchor:       p,
		Cursor:       p,
		PreferredCol: col,
	}
}

func (s *previewSelectionState) toggleSelecting() {
	if !s.Active {
		return
	}
	if s.Selecting {
		s.Selecting = false
		s.Anchor = s.Cursor
		return
	}
	s.Selecting = true
	s.Anchor = s.Cursor
}

func previewSelectableRowLen(row previewSelectableRow) int {
	return len([]rune(row.Plain))
}

func previewColumnToRuneIndex(text string, colCells int) int {
	if colCells <= 0 {
		return 0
	}
	runes := []rune(text)
	cells := 0
	for i, r := range runes {
		w := ansi.StringWidth(string(r))
		if w < 1 {
			w = 1
		}
		if colCells < cells+w {
			return i
		}
		cells += w
	}
	return len(runes)
}

func (s *previewSelectionState) move(rows []previewSelectableRow, dRow, dCol int) {
	if !s.Active || len(rows) == 0 {
		return
	}
	row := clampInt(s.Cursor.Row+dRow, 0, len(rows)-1)
	col := s.Cursor.Col
	if dRow != 0 {
		col = s.PreferredCol
	} else {
		col += dCol
	}
	col = clampInt(col, 0, previewSelectableRowLen(rows[row]))
	s.Cursor = previewSelectionPoint{Row: row, Col: col}
	if dRow == 0 {
		s.PreferredCol = col
	}
	if !s.Selecting {
		s.Anchor = s.Cursor
	}
}

func (s *previewSelectionState) ensureCursorVisible(scroll *int, visibleRows int, totalRows int) {
	if !s.Active || scroll == nil || visibleRows < 1 || totalRows < 1 {
		return
	}
	if s.Cursor.Row < *scroll {
		*scroll = s.Cursor.Row
	}
	if s.Cursor.Row >= *scroll+visibleRows {
		*scroll = s.Cursor.Row - visibleRows + 1
	}
	*scroll = clampPreviewScrollOffset(*scroll, totalRows, visibleRows)
}

func previewSelectionOrdered(a, b previewSelectionPoint) (previewSelectionPoint, previewSelectionPoint) {
	if a.Row > b.Row || (a.Row == b.Row && a.Col > b.Col) {
		return b, a
	}
	return a, b
}

func previewSelectionRowInterval(sel previewSelectionState, row int, rowLen int) (start int, end int, selected bool) {
	if !sel.Active || !sel.Selecting {
		return 0, 0, false
	}
	lo, hi := previewSelectionOrdered(sel.Anchor, sel.Cursor)
	if row < lo.Row || row > hi.Row {
		return 0, 0, false
	}
	switch {
	case lo.Row == hi.Row:
		start, end = lo.Col, hi.Col+1
	case row == lo.Row:
		start, end = lo.Col, rowLen
	case row == hi.Row:
		start, end = 0, hi.Col+1
	default:
		start, end = 0, rowLen
	}
	if rowLen == 0 {
		start, end = 0, 1
	} else {
		start = clampInt(start, 0, rowLen)
		end = clampInt(end, 0, rowLen)
		if end <= start {
			end = minInt(rowLen, start+1)
		}
	}
	return start, end, true
}

func renderPreviewSelectionText(theme Theme, text string, row int, sel previewSelectionState) string {
	runes := []rune(text)
	rowLen := len(runes)
	start, end, selected := previewSelectionRowInterval(sel, row, rowLen)
	cursorCol := sel.Cursor.Col
	cursorOnRow := sel.Active && sel.Cursor.Row == row

	if rowLen == 0 {
		if cursorOnRow {
			return theme.Focus.SelectionActive.Style().Render(" ")
		}
		if selected {
			return theme.Focus.VisualSelection.Style().Render(" ")
		}
		return ""
	}

	selectionStyle := theme.Focus.VisualSelection.Style()
	cursorStyle := theme.Focus.SelectionActive.Style()
	var sb strings.Builder
	for i, r := range runes {
		ch := string(r)
		switch {
		case cursorOnRow && i == cursorCol:
			sb.WriteString(cursorStyle.Render(ch))
		case selected && i >= start && i < end:
			sb.WriteString(selectionStyle.Render(ch))
		default:
			sb.WriteString(ch)
		}
	}
	if cursorOnRow && cursorCol >= rowLen {
		sb.WriteString(cursorStyle.Render(" "))
	}
	return sb.String()
}

func previewSelectionPlain(rows []previewSelectableRow, sel previewSelectionState) string {
	if len(rows) == 0 {
		return ""
	}
	if !sel.Selecting {
		return ""
	}
	lo, hi := previewSelectionOrdered(sel.Anchor, sel.Cursor)
	lo.Row = clampInt(lo.Row, 0, len(rows)-1)
	hi.Row = clampInt(hi.Row, 0, len(rows)-1)
	var out []string
	for row := lo.Row; row <= hi.Row; row++ {
		text := rows[row].Plain
		runes := []rune(text)
		start, end := 0, len(runes)
		switch {
		case lo.Row == hi.Row:
			start, end = lo.Col, hi.Col+1
		case row == lo.Row:
			start = lo.Col
		case row == hi.Row:
			end = hi.Col + 1
		}
		start = clampInt(start, 0, len(runes))
		end = clampInt(end, start, len(runes))
		if len(runes) == 0 && rows[row].Image != nil {
			out = append(out, rows[row].Plain)
			continue
		}
		out = append(out, string(runes[start:end]))
	}
	return strings.Join(out, "\n")
}

func previewSelectionHTML(rows []previewSelectableRow, sel previewSelectionState) string {
	if len(rows) == 0 {
		return ""
	}
	if !sel.Selecting {
		return ""
	}
	lo, hi := previewSelectionOrdered(sel.Anchor, sel.Cursor)
	lo.Row = clampInt(lo.Row, 0, len(rows)-1)
	hi.Row = clampInt(hi.Row, 0, len(rows)-1)
	var parts []string
	for row := lo.Row; row <= hi.Row; row++ {
		plain := previewSelectionPlainForRow(rows[row], row, lo, hi)
		if rows[row].Image != nil {
			parts = append(parts, html.EscapeString(firstNonEmptyString(plain, rows[row].Plain)))
			continue
		}
		if previewSelectionCoversWholeRow(rows[row], row, lo, hi) && strings.TrimSpace(rows[row].HTML) != "" {
			parts = append(parts, rows[row].HTML)
			continue
		}
		parts = append(parts, html.EscapeString(plain))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "<br>\n")
}

func previewSelectionPlainForRow(row previewSelectableRow, rowIdx int, lo, hi previewSelectionPoint) string {
	text := row.Plain
	runes := []rune(text)
	start, end := 0, len(runes)
	switch {
	case lo.Row == hi.Row:
		start, end = lo.Col, hi.Col+1
	case rowIdx == lo.Row:
		start = lo.Col
	case rowIdx == hi.Row:
		end = hi.Col + 1
	}
	start = clampInt(start, 0, len(runes))
	end = clampInt(end, start, len(runes))
	return string(runes[start:end])
}

func previewSelectionCoversWholeRow(row previewSelectableRow, rowIdx int, lo, hi previewSelectionPoint) bool {
	rowLen := previewSelectableRowLen(row)
	switch {
	case lo.Row == hi.Row:
		return lo.Col <= 0 && hi.Col >= maxInt(0, rowLen-1)
	case rowIdx == lo.Row:
		return lo.Col <= 0
	case rowIdx == hi.Row:
		return hi.Col >= maxInt(0, rowLen-1)
	default:
		return true
	}
}

func previewAllPlain(rows []previewSelectableRow) string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Plain)
	}
	return strings.Join(out, "\n")
}

func previewAllHTML(rows []previewSelectableRow) string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.HTML) != "" {
			out = append(out, row.HTML)
		} else if row.Image != nil {
			out = append(out, html.EscapeString(row.Plain))
		} else {
			out = append(out, html.EscapeString(row.Plain))
		}
	}
	return strings.Join(out, "<br>\n")
}

func selectableRowsFromPreviewLayout(layout previewDocumentLayout) []previewSelectableRow {
	rows := make([]previewSelectableRow, 0, len(layout.Rows))
	for _, row := range layout.Rows {
		plain := row.CopyPlain
		if plain == "" {
			plain = strings.TrimRight(ansi.Strip(row.Content), "\r\n")
		}
		item := previewSelectableRow{Plain: plain, Rendered: row.Content, HTML: row.CopyHTML}
		if row.HasImage {
			image := row.Image
			item.Image = &image
			item.ImageAlt = row.ImageAlt
			if strings.TrimSpace(item.Plain) == "" {
				item.Plain = previewSelectionImageLabel(item.Image, item.ImageAlt)
			}
			if strings.TrimSpace(item.HTML) == "" {
				item.HTML = html.EscapeString(item.Plain)
			}
		}
		rows = append(rows, item)
	}
	return rows
}

func previewSelectionImageLabel(image *models.InlineImage, alt string) string {
	label := strings.TrimSpace(alt)
	if label == "" && image != nil {
		label = strings.TrimSpace(image.ContentID)
	}
	if label == "" {
		label = "inline image"
	}
	if image != nil && image.MIMEType != "" {
		return fmt.Sprintf("[image: %s  %s]", label, image.MIMEType)
	}
	return "[image: " + label + "]"
}

func plainSelectableRows(lines []string) []previewSelectableRow {
	rows := make([]previewSelectableRow, 0, len(lines))
	for _, line := range lines {
		plain := strings.TrimRight(ansi.Strip(line), "\r\n")
		rows = append(rows, previewSelectableRow{
			Plain:    plain,
			Rendered: line,
			HTML:     html.EscapeString(plain),
		})
	}
	return rows
}

func selectableRowsFromRendered(lines []string) []previewSelectableRow {
	rows := make([]previewSelectableRow, 0, len(lines))
	for _, line := range lines {
		plain := strings.TrimRight(ansi.Strip(line), "\r\n")
		rows = append(rows, previewSelectableRow{
			Plain:    plain,
			Rendered: line,
			HTML:     html.EscapeString(plain),
		})
	}
	return rows
}

func selectableRowsFromPreviewLayoutViewport(layout previewDocumentLayout, offset, visibleRows int) []previewSelectableRow {
	if visibleRows < 1 {
		visibleRows = 1
	}
	offset = clampPreviewScrollOffset(offset, layout.TotalRows, visibleRows)
	end := minInt(layout.TotalRows, offset+visibleRows)
	rows := make([]previewSelectableRow, 0, visibleRows)
	for i := offset; i < end && i < len(layout.Rows); i++ {
		row := layout.Rows[i]
		content := row.Content
		if row.TerminalConsumed || isNativePreviewImageContent(layout.ImageMode, content) {
			content = ""
		}
		plain := row.CopyPlain
		if plain == "" {
			plain = strings.TrimRight(ansi.Strip(content), "\r\n")
		}
		item := previewSelectableRow{
			Plain:    plain,
			Rendered: content,
			HTML:     row.CopyHTML,
		}
		if row.HasImage {
			image := row.Image
			item.Image = &image
			item.ImageAlt = row.ImageAlt
			if strings.TrimSpace(item.Plain) == "" {
				item.Plain = previewSelectionImageLabel(item.Image, item.ImageAlt)
			}
			if strings.TrimSpace(item.HTML) == "" {
				item.HTML = html.EscapeString(item.Plain)
			}
		}
		rows = append(rows, item)
	}
	for len(rows) < visibleRows {
		rows = append(rows, previewSelectableRow{})
	}
	return rows
}

func renderPreviewSelectableRows(theme Theme, rows []previewSelectableRow, surface previewSelectionSurface, sel previewSelectionState, rowOffset int) []string {
	rendered := make([]string, 0, len(rows))
	for i, row := range rows {
		rowIdx := rowOffset + i
		if sel.activeOn(surface) && sel.Mouse {
			rendered = append(rendered, renderPreviewSelectionText(theme, row.Plain, rowIdx, sel))
			continue
		}
		rendered = append(rendered, row.Rendered)
	}
	return rendered
}

func renderPreviewSelectableLines(theme Theme, lines []string, surface previewSelectionSurface, sel previewSelectionState, rowOffset int) []string {
	return renderPreviewSelectableRows(theme, selectableRowsFromRendered(lines), surface, sel, rowOffset)
}

func renderPlainRowsWithSelection(theme Theme, lines []string, offset, visibleRows int, sel previewSelectionState, surface previewSelectionSurface) string {
	rows := plainSelectableRows(lines)
	offset = clampPreviewScrollOffset(offset, len(rows), visibleRows)
	end := minInt(len(rows), offset+visibleRows)
	rendered := make([]string, 0, visibleRows)
	for row := offset; row < end; row++ {
		line := lines[row]
		if sel.activeOn(surface) {
			line = renderPreviewSelectionText(theme, rows[row].Plain, row, sel)
		}
		rendered = append(rendered, line)
	}
	for len(rendered) < visibleRows {
		rendered = append(rendered, "")
	}
	return strings.Join(rendered, "\n")
}

func (m *Model) timelinePreviewHeaderRows(innerW int, active bool) []previewSelectableRow {
	email := m.timeline.selectedEmail
	bodyMatchesSelected := email != nil && m.timeline.bodyMessageID == email.MessageID
	category := ""
	if email != nil {
		category = m.previewCategory(email.MessageID)
	}
	var headerBody *models.EmailBody
	if bodyMatchesSelected {
		headerBody = m.timeline.body
	}
	loadTag := ""
	if email != nil {
		loadTag = previewLoadTag(m.timeline.previewLoad, email.MessageID)
	}
	lines := renderPreviewHeaderLinesWithTheme(m.theme, email, headerBody, category, bodyMatchesSelected && previewHasUnsubscribe(m.timeline.body), loadTag, innerW, active)
	return selectableRowsFromRendered(lines)
}

func (m *Model) timelineSplitPreviewSelectableRows() []previewSelectableRow {
	w := m.timeline.previewWidth
	if w <= 0 {
		w = 40
	}
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	chrome := m.chromeState(m.buildLayoutPlan(m.windowWidth, m.windowHeight))
	rows := m.timelinePreviewHeaderRows(innerW, chrome.FocusedPanel == panelPreview)
	panelHeight := m.timelinePreviewInnerHeight()
	maxBodyLines := panelHeight - len(rows) - 1
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	threadContextLines := m.renderDraftThreadContextLines(m.timeline.selectedEmail, innerW, 4)
	rows = append(rows, selectableRowsFromRendered(threadContextLines)...)
	maxBodyLines -= len(threadContextLines)
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	if m.timeline.bodyLoading || m.timeline.body == nil || (m.timeline.selectedEmail != nil && m.timeline.bodyMessageID != m.timeline.selectedEmail.MessageID) {
		return append(rows, previewSelectableRow{Plain: "Loading...", Rendered: "Loading...", HTML: "Loading..."})
	}
	bodyChrome := m.timelinePreviewBodyChromeRows(innerW)
	rows = append(rows, bodyChrome...)
	visibleLines := maxBodyLines - len(bodyChrome)
	if visibleLines < 1 {
		visibleLines = 1
	}
	layout := m.timelinePreviewDocumentLayout(innerW, visibleLines)
	m.timeline.bodyScrollOffset = clampPreviewScrollOffset(m.timeline.bodyScrollOffset, layout.TotalRows, visibleLines)
	return append(rows, selectableRowsFromPreviewLayoutViewport(layout, m.timeline.bodyScrollOffset, visibleLines)...)
}

func (m *Model) timelinePreviewBodyChromeRows(innerW int) []previewSelectableRow {
	if m.timeline.body == nil {
		return nil
	}
	lines := make([]string, 0)
	imageMode := m.currentPreviewImageMode()
	if nImg := len(m.timeline.body.InlineImages); nImg > 0 {
		lines = append(lines, truncate(splitInlineImageHint(nImg, imageMode), innerW))
	}
	if nRemote := m.timelineRemoteImageCount(); nRemote > 0 {
		lines = append(lines, truncate(splitRemoteImageHint(nRemote, imageMode, m.timelineRemoteRevealAvailable()), innerW))
	}
	for _, att := range m.timeline.body.Attachments {
		sizeStr := fmt.Sprintf("%.1f KB", float64(att.Size)/1024)
		if att.Size >= 1024*1024 {
			sizeStr = fmt.Sprintf("%.1f MB", float64(att.Size)/(1024*1024))
		}
		lines = append(lines, truncate(fmt.Sprintf("[attach] %s  %s  %s", att.Filename, att.MIMEType, sizeStr), innerW))
	}
	lines = append(lines, m.renderCalendarInvitationPrompt(innerW)...)
	if m.timeline.attachmentSavePrompt {
		if m.timeline.attachmentSaveWarning != "" {
			lines = append(lines, truncate(m.timeline.attachmentSaveWarning, innerW))
		}
		lines = append(lines, "Save to: "+m.timeline.attachmentSaveInput.Value())
	}
	if len(m.timeline.body.Attachments) > 0 {
		lines = append(lines, previewAttachmentDivider(innerW), "")
	}
	return selectableRowsFromRendered(lines)
}

func (m *Model) timelineFullScreenSelectableRows() []previewSelectableRow {
	innerW, maxBodyLines := m.timelineFullScreenDocumentBudget()
	rows := m.timelinePreviewHeaderRows(innerW, true)
	promptLines := m.renderCalendarInvitationPrompt(innerW)
	rows = append(rows, selectableRowsFromRendered(promptLines)...)
	maxBodyLines -= len(promptLines)
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	threadContextLines := m.renderDraftThreadContextLines(m.timeline.selectedEmail, innerW, 6)
	rows = append(rows, selectableRowsFromRendered(threadContextLines)...)
	maxBodyLines -= len(threadContextLines)
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	if m.timeline.bodyLoading || m.timeline.body == nil {
		return append(rows, previewSelectableRow{Plain: "Loading...", Rendered: "Loading...", HTML: "Loading..."})
	}
	layout := m.timelinePreviewDocumentLayout(innerW, maxBodyLines)
	m.timeline.bodyScrollOffset = clampPreviewScrollOffset(m.timeline.bodyScrollOffset, layout.TotalRows, maxBodyLines)
	return append(rows, selectableRowsFromPreviewLayoutViewport(layout, m.timeline.bodyScrollOffset, maxBodyLines)...)
}

func (m *Model) contactsRightPanelSelectableRows(rightW, contentH int) []previewSelectableRow {
	rightInnerW := rightW - 4
	if rightInnerW < 10 {
		rightInnerW = 10
	}
	if m.contactPreviewEmail != nil {
		return m.contactsInlinePreviewSelectableRows(rightInnerW, contentH)
	}
	if m.contactDetail == nil {
		return selectableRowsFromRendered([]string{lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor()).Render("  Select a contact and press Enter")})
	}
	return m.contactsDetailSelectableRows(rightInnerW)
}

func (m *Model) contactsInlinePreviewSelectableRows(innerW, contentH int) []previewSelectableRow {
	email := m.contactPreviewEmail
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor())
	lines := []string{
		boldStyle.Render(truncate("From: "+sanitizeText(email.Sender), innerW)),
		dimStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), innerW)),
		boldStyle.Render(truncate("Subj: "+sanitizeText(email.Subject), innerW)),
		strings.Repeat("-", innerW),
	}
	if m.contactPreviewLoading {
		lines = append(lines, dimStyle.Render("Loading..."))
	} else if m.contactPreviewBody != nil {
		body := stripInvisibleChars(emailrender.EmailBodyMarkdown(m.contactPreviewBody))
		if body == "" {
			body = "(No text content)"
		}
		bodyLines := renderEmailBodyLines(body, innerW)
		maxLines := contentH - 6
		if maxLines < 1 {
			maxLines = 1
		}
		if len(bodyLines) > maxLines {
			bodyLines = bodyLines[:maxLines]
		}
		lines = append(lines, bodyLines...)
		for i := len(bodyLines); i < maxLines; i++ {
			lines = append(lines, "")
		}
	}
	lines = append(lines, dimStyle.Render(" Esc: back to contact"))
	return selectableRowsFromRendered(lines)
}

func (m *Model) contactsDetailSelectableRows(innerW int) []previewSelectableRow {
	c := m.contactDetail
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor())
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
	normalStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
	displayName := c.DisplayName
	if displayName == "" {
		displayName = c.Email
	}
	lines := []string{
		boldStyle.Render(displayName),
		dimStyle.Render(c.Email),
	}
	if c.Company != "" {
		lines = append(lines, normalStyle.Render("Company: "+c.Company))
	}
	if len(c.Topics) > 0 {
		lines = append(lines, normalStyle.Render("Topics: "+strings.Join(c.Topics, ", ")))
	}
	firstStr := "-"
	lastStr := "-"
	if !c.FirstSeen.IsZero() {
		firstStr = c.FirstSeen.Format("2006-01-02")
	}
	if !c.LastSeen.IsZero() {
		lastStr = c.LastSeen.Format("2006-01-02")
	}
	lines = append(lines, dimStyle.Render(fmt.Sprintf("First seen: %s  Last seen: %s  Received: %d  Sent: %d", firstStr, lastStr, c.EmailCount, c.SentCount)))
	if c.EnrichedAt != nil {
		lines = append(lines, dimStyle.Render("Enriched: "+c.EnrichedAt.Format("2006-01-02")))
	}
	lines = append(lines, "", boldStyle.Render("Recent Emails"))
	if len(m.contactDetailEmails) == 0 {
		lines = append(lines, dimStyle.Render("  Loading..."))
		return selectableRowsFromRendered(lines)
	}
	maxSubjW := innerW - 14
	if maxSubjW < 10 {
		maxSubjW = 10
	}
	for i, e := range m.contactDetailEmails {
		subj := ansi.Truncate(e.Subject, maxSubjW, "...")
		subjPad := strings.Repeat(" ", maxSubjW-ansi.StringWidth(subj))
		line := "  " + subj + subjPad + "  " + e.Date.Format("2006-01-02")
		rowStyle := normalStyle
		if m.contactFocusPanel == 1 && i == m.contactDetailIdx {
			rowStyle = lipgloss.NewStyle().
				Foreground(m.theme.Chrome.TabActive.ForegroundColor()).
				Background(m.theme.Chrome.TabActive.BackgroundColor()).
				Bold(true)
		}
		lines = append(lines, rowStyle.Render(line))
	}
	return selectableRowsFromRendered(lines)
}

func previewSelectionStatusLabel(sel previewSelectionState) string {
	if !sel.Active {
		return ""
	}
	if !sel.Selecting {
		return fmt.Sprintf("CURSOR %d:%d", sel.Cursor.Row+1, sel.Cursor.Col+1)
	}
	return fmt.Sprintf("SELECT %d:%d", sel.Cursor.Row+1, sel.Cursor.Col+1)
}

func previewSelectionHintSegments(sel previewSelectionState, surface previewSelectionSurface) []string {
	if !sel.activeOn(surface) {
		return nil
	}
	if sel.Mouse {
		return []string{
			"drag: extend selection",
			"y: copy selection",
			"esc: clear selection",
			"m: mouse mode",
		}
	}
	if sel.Selecting {
		return []string{
			"h/j/k/l: extend selection",
			"arrows: extend selection",
			"v: stop selecting",
			"y: copy selection",
			"yy: copy line",
			"Y: copy all",
			"esc: hide cursor",
		}
	}
	return []string{
		"h/j/k/l: move cursor",
		"arrows: move cursor",
		"v: start selection",
		"yy: copy line",
		"Y: copy all",
		"esc: hide cursor",
	}
}
