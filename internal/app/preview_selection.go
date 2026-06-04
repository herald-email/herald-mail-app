package app

import (
	"fmt"
	"html"
	"strings"

	tea "charm.land/bubbletea/v2"
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
	HTML     string
	Image    *models.InlineImage
	ImageAlt string
}

type previewSelectionState struct {
	Active       bool
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
	rows := m.previewRowsForSurface(surface)
	if len(rows) == 0 || !m.previewSelection.selectingOn(surface) {
		return previewClipboardPayload{}, false
	}
	return previewClipboardPayloadForRows(rows, m.previewSelection), true
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
		item := previewSelectableRow{Plain: plain, HTML: row.CopyHTML}
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
			Plain: plain,
			HTML:  html.EscapeString(plain),
		})
	}
	return rows
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
