package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
	emailrender "github.com/herald-email/herald-mail-app/internal/render"
)

type previewSelectionSurface string

const (
	previewSelectionNone     previewSelectionSurface = ""
	previewSelectionTimeline previewSelectionSurface = "timeline"
	previewSelectionContacts previewSelectionSurface = "contacts"
)

type previewSelectionPoint struct {
	Row int
	Col int
}

type previewSelectableRow struct {
	Plain    string
	Rendered string
}

type previewSelectionState struct {
	Active   bool
	Dragging bool
	Surface  previewSelectionSurface
	Anchor   previewSelectionPoint
	Cursor   previewSelectionPoint
}

func (s *previewSelectionState) reset() {
	*s = previewSelectionState{}
}

func (s previewSelectionState) activeOn(surface previewSelectionSurface) bool {
	return s.Active && s.Surface == surface
}

func (m *Model) clearPreviewSelection() {
	m.previewSelection.reset()
	m.timeline.pendingY = false
}

func (m *Model) beginPreviewMouseSelection(surface previewSelectionSurface, row, colCells int, rows []previewSelectableRow) bool {
	if surface == previewSelectionNone || len(rows) == 0 || row < 0 || row >= len(rows) {
		return false
	}
	col := previewColumnToRuneIndex(rows[row].Plain, colCells)
	point := previewSelectionPoint{Row: row, Col: col}
	m.previewSelection = previewSelectionState{
		Active:   true,
		Dragging: true,
		Surface:  surface,
		Anchor:   point,
		Cursor:   point,
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

func previewSelectionOrdered(a, b previewSelectionPoint) (previewSelectionPoint, previewSelectionPoint) {
	if a.Row > b.Row || (a.Row == b.Row && a.Col > b.Col) {
		return b, a
	}
	return a, b
}

func previewSelectionRowInterval(sel previewSelectionState, row, rowLen int) (int, int, bool) {
	if !sel.Active {
		return 0, 0, false
	}
	lo, hi := previewSelectionOrdered(sel.Anchor, sel.Cursor)
	if row < lo.Row || row > hi.Row {
		return 0, 0, false
	}
	start, end := 0, rowLen
	switch {
	case lo.Row == hi.Row:
		start, end = lo.Col, hi.Col+1
	case row == lo.Row:
		start = lo.Col
	case row == hi.Row:
		end = hi.Col + 1
	}
	if rowLen == 0 {
		return 0, 1, true
	}
	start = clampInt(start, 0, rowLen)
	end = clampInt(end, 0, rowLen)
	if end <= start {
		end = minInt(rowLen, start+1)
	}
	return start, end, true
}

func previewSelectionPlain(rows []previewSelectableRow, sel previewSelectionState) string {
	if len(rows) == 0 || !sel.Active {
		return ""
	}
	lo, hi := previewSelectionOrdered(sel.Anchor, sel.Cursor)
	lo.Row = clampInt(lo.Row, 0, len(rows)-1)
	hi.Row = clampInt(hi.Row, 0, len(rows)-1)
	out := make([]string, 0, hi.Row-lo.Row+1)
	for row := lo.Row; row <= hi.Row; row++ {
		runes := []rune(rows[row].Plain)
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
		out = append(out, string(runes[start:end]))
	}
	return strings.Join(out, "\n")
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

func selectableRowsFromRendered(lines []string) []previewSelectableRow {
	rows := make([]previewSelectableRow, 0, len(lines))
	for _, line := range lines {
		plain := strings.TrimRight(ansi.Strip(line), "\r\n")
		rows = append(rows, previewSelectableRow{Plain: plain, Rendered: line})
	}
	return rows
}

func selectableRowsFromPreviewLayout(layout previewDocumentLayout, offset, visibleRows int) []previewSelectableRow {
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
		rows = append(rows, previewSelectableRow{
			Plain:    strings.TrimRight(ansi.Strip(content), "\r\n"),
			Rendered: content,
		})
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
		if sel.activeOn(surface) {
			if line, ok := renderPreviewSelectionRow(theme, row, rowIdx, sel); ok {
				rendered = append(rendered, line)
				continue
			}
		}
		rendered = append(rendered, row.Rendered)
	}
	return rendered
}

func renderPreviewSelectableLines(theme Theme, lines []string, surface previewSelectionSurface, sel previewSelectionState, rowOffset int) []string {
	return renderPreviewSelectableRows(theme, selectableRowsFromRendered(lines), surface, sel, rowOffset)
}

func renderPreviewSelectionRow(theme Theme, row previewSelectableRow, rowIdx int, sel previewSelectionState) (string, bool) {
	runes := []rune(row.Plain)
	start, end, selected := previewSelectionRowInterval(sel, rowIdx, len(runes))
	cursorOnRow := sel.Cursor.Row == rowIdx
	if !selected && !cursorOnRow {
		return "", false
	}
	if len(runes) == 0 {
		return theme.Focus.VisualSelection.Style().Render(" "), true
	}
	selectionStyle := theme.Focus.VisualSelection.Style()
	cursorStyle := theme.Focus.SelectionActive.Style()
	var sb strings.Builder
	for i, r := range runes {
		ch := string(r)
		switch {
		case cursorOnRow && i == sel.Cursor.Col:
			sb.WriteString(cursorStyle.Render(ch))
		case selected && i >= start && i < end:
			sb.WriteString(selectionStyle.Render(ch))
		default:
			sb.WriteString(ch)
		}
	}
	if cursorOnRow && sel.Cursor.Col >= len(runes) {
		sb.WriteString(cursorStyle.Render(" "))
	}
	return sb.String(), true
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
		return append(rows, previewSelectableRow{Plain: "Loading...", Rendered: "Loading..."})
	}
	bodyChrome := m.timelinePreviewBodyChromeRows(innerW)
	rows = append(rows, bodyChrome...)
	visibleLines := maxBodyLines - len(bodyChrome)
	if visibleLines < 1 {
		visibleLines = 1
	}
	layout := m.timelinePreviewDocumentLayout(innerW, visibleLines)
	m.timeline.bodyScrollOffset = clampPreviewScrollOffset(m.timeline.bodyScrollOffset, layout.TotalRows, visibleLines)
	return append(rows, selectableRowsFromPreviewLayout(layout, m.timeline.bodyScrollOffset, visibleLines)...)
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
		return append(rows, previewSelectableRow{Plain: "Loading...", Rendered: "Loading..."})
	}
	layout := m.timelinePreviewDocumentLayout(innerW, maxBodyLines)
	m.timeline.bodyScrollOffset = clampPreviewScrollOffset(m.timeline.bodyScrollOffset, layout.TotalRows, maxBodyLines)
	return append(rows, selectableRowsFromPreviewLayout(layout, m.timeline.bodyScrollOffset, maxBodyLines)...)
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
		strings.Repeat("─", innerW),
	}
	if m.contactPreviewLoading {
		lines = append(lines, dimStyle.Render("Loading…"))
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
	firstStr := "—"
	lastStr := "—"
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
		lines = append(lines, dimStyle.Render("  Loading…"))
		return selectableRowsFromRendered(lines)
	}
	maxSubjW := innerW - 14
	if maxSubjW < 10 {
		maxSubjW = 10
	}
	for i, e := range m.contactDetailEmails {
		subj := ansi.Truncate(e.Subject, maxSubjW, "…")
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
