package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func fitPanelContentHeight(content string, target int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > target {
		lines = lines[:target]
	}
	for len(lines) < target {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func clampInt(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func (m *Model) clearTimelinePreviewDocumentCache() {
	m.timeline.previewDocLayout = nil
	m.timeline.previewDocWidth = 0
	m.timeline.previewDocRows = 0
	m.timeline.previewDocMode = ""
	m.timeline.previewDocMessageID = ""
	m.timeline.previewDocRemoteRevision = 0
}

func (m *Model) timelinePreviewInnerHeight() int {
	if h := m.timelineTable.Height() + 1; h >= 5 {
		return h
	}
	if h := m.windowHeight - 7; h >= 5 {
		return h
	}
	return 5
}

func previewHasUnsubscribe(body *models.EmailBody) bool {
	return body != nil && strings.TrimSpace(body.ListUnsubscribe) != ""
}

func previewActionText(hasUnsubscribe, hasInvitation bool) string {
	actions := []string{}
	if hasUnsubscribe {
		actions = append(actions, "u unsubscribe")
	}
	if hasInvitation {
		actions = append(actions, "i create calendar event")
	}
	actions = append(actions, "H hide future mail")
	return strings.Join(actions, "  ")
}

func previewTagText(category string) string {
	category = strings.TrimSpace(category)
	if category == "" {
		return "none"
	}
	return category
}

func (m *Model) previewCategory(messageID string) string {
	if m.classifications == nil {
		return ""
	}
	return m.classifications[messageID]
}

type previewHeaderStyles struct {
	label  lipgloss.Style
	from   lipgloss.Style
	date   lipgloss.Style
	subj   lipgloss.Style
	tag    lipgloss.Style
	action lipgloss.Style
}

func newPreviewHeaderStyles(active bool) previewHeaderStyles {
	return newPreviewHeaderStylesForTheme(defaultTheme, active)
}

func newPreviewHeaderStylesForTheme(theme Theme, active bool) previewHeaderStyles {
	label := theme.Metadata.Label.Style()
	if active {
		label = label.Bold(true)
	}
	return previewHeaderStyles{
		label:  label,
		from:   theme.Metadata.Sender.Style(),
		date:   theme.Metadata.Date.Style(),
		subj:   theme.Metadata.Subject.Style(),
		tag:    theme.Metadata.Tag.Style(),
		action: theme.Metadata.Action.Style(),
	}
}

func renderPreviewHeaderLine(label, value string, innerW int, styles previewHeaderStyles, valueStyle lipgloss.Style) string {
	line := truncate(label+" "+value, innerW)
	return stylePreviewHeaderLine(line, label, styles, valueStyle)
}

func renderPreviewHeaderWrapped(label, value string, innerW int, styles previewHeaderStyles, valueStyle lipgloss.Style) []string {
	lines := wrapLines(label+" "+value, innerW)
	for i, line := range lines {
		lines[i] = stylePreviewHeaderLine(line, label, styles, valueStyle)
	}
	return lines
}

func stylePreviewHeaderLine(line, label string, styles previewHeaderStyles, valueStyle lipgloss.Style) string {
	prefix := label + " "
	if strings.HasPrefix(line, prefix) {
		return styles.label.Render(label) + " " + valueStyle.Render(strings.TrimPrefix(line, prefix))
	}
	if strings.HasPrefix(line, label) {
		return styles.label.Render(label) + valueStyle.Render(strings.TrimPrefix(line, label))
	}
	return valueStyle.Render(line)
}

func renderPreviewHeaderLines(email *models.EmailData, body *models.EmailBody, category string, hasUnsubscribe bool, innerW int, active bool) []string {
	return renderPreviewHeaderLinesWithLoad(email, body, category, hasUnsubscribe, "", innerW, active)
}

func renderPreviewHeaderLinesWithLoad(email *models.EmailData, body *models.EmailBody, category string, hasUnsubscribe bool, loadTag string, innerW int, active bool) []string {
	return renderPreviewHeaderLinesWithTheme(defaultTheme, email, body, category, hasUnsubscribe, loadTag, innerW, active)
}

func renderPreviewHeaderLinesWithTheme(theme Theme, email *models.EmailData, body *models.EmailBody, category string, hasUnsubscribe bool, loadTag string, innerW int, active bool) []string {
	if email == nil {
		return nil
	}

	styles := newPreviewHeaderStylesForTheme(theme, active)
	lines := []string{
		renderPreviewHeaderLine("From:", email.Sender, innerW, styles, styles.from),
	}
	if body != nil {
		if to := strings.TrimSpace(body.To); to != "" {
			lines = append(lines, renderPreviewHeaderWrapped("To:", to, innerW, styles, styles.from)...)
		}
		if cc := strings.TrimSpace(body.CC); cc != "" {
			lines = append(lines, renderPreviewHeaderWrapped("Cc:", cc, innerW, styles, styles.from)...)
		}
	}
	lines = append(lines,
		renderPreviewHeaderLine("Date:", formatPreviewHeaderDate(email.Date), innerW, styles, styles.date),
		renderPreviewHeaderLine("Subj:", email.Subject, innerW, styles, styles.subj),
	)
	if email.IsDraft {
		lines = append(lines, renderPreviewHeaderLine("State:", draftStateText(email), innerW, styles, styles.action))
	}
	lines = append(lines, renderPreviewHeaderWrapped("Tags:", previewTagText(category), innerW, styles, styles.tag)...)
	lines = append(lines, renderPreviewHeaderWrapped("Actions:", previewActionText(hasUnsubscribe, calendarBodyHasInvitation(body)), innerW, styles, styles.action)...)
	if strings.TrimSpace(loadTag) != "" {
		lines = append(lines, renderPreviewHeaderLine("Load:", loadTag, innerW, styles, styles.date))
	}
	lines = append(lines, strings.Repeat("─", innerW))
	return lines
}

func previewAttachmentDivider(innerW int) string {
	if innerW <= 0 {
		return ""
	}
	labels := []string{
		"Attachments ( [ and ] for selection, s for save )",
		"Attachments ([/]: select, s: save)",
		"Attachments ([/]: sel, s: save)",
	}
	label := labels[len(labels)-1]
	for _, candidate := range labels {
		if ansi.StringWidth(" "+candidate+" ") < innerW {
			label = candidate
			break
		}
	}
	title := " " + label + " "
	titleW := ansi.StringWidth(title)
	if titleW >= innerW {
		return truncateVisual(label, innerW)
	}
	fill := innerW - titleW
	left := fill / 2
	right := fill - left
	return strings.Repeat("─", left) + title + strings.Repeat("─", right)
}

func sameTimelineMessage(a, b *models.EmailData) bool {
	if a == nil || b == nil {
		return false
	}
	if a.MessageID != "" && b.MessageID != "" {
		return a.MessageID == b.MessageID
	}
	if a.UID != 0 && b.UID != 0 {
		return a.UID == b.UID && a.Folder == b.Folder
	}
	return a == b
}

func (m *Model) visibleTimelineThreadEmails(email *models.EmailData) []*models.EmailData {
	if email == nil {
		return nil
	}
	normalized := normalizeSubject(email.Subject)
	if normalized == "" {
		return nil
	}

	var out []*models.EmailData
	seenSelected := false
	for _, candidate := range m.timelineDisplayEmails() {
		if candidate == nil || normalizeSubject(candidate.Subject) != normalized {
			continue
		}
		if sameTimelineMessage(candidate, email) {
			seenSelected = true
		}
		out = append(out, candidate)
	}
	if !seenSelected {
		out = append([]*models.EmailData{email}, out...)
	}
	return out
}

func draftThreadContextRole(email *models.EmailData) string {
	if email == nil {
		return "Message"
	}
	if email.IsDraft {
		return draftKindLabel(email)
	}
	if isReplySubject(email.Subject) {
		return "Reply"
	}
	return "Message"
}

func draftThreadContextLine(email *models.EmailData, innerW int) string {
	if email == nil {
		return ""
	}
	date := "N/A"
	if !email.Date.IsZero() {
		date = email.Date.Format("01-02 15:04")
	}
	subject := sanitizeText(email.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	line := fmt.Sprintf("%s  %s  %s  %s", draftThreadContextRole(email), date, senderDisplayLabel(email.Sender), subject)
	return truncate(line, innerW)
}

func (m *Model) renderDraftThreadContextLines(email *models.EmailData, innerW, maxMessages int) []string {
	if email == nil || !email.IsDraft || !isReplySubject(email.Subject) {
		return nil
	}
	threadEmails := m.visibleTimelineThreadEmails(email)
	if len(threadEmails) <= 1 {
		return nil
	}
	if maxMessages <= 0 {
		maxMessages = len(threadEmails)
	}
	limit := len(threadEmails)
	if limit > maxMessages {
		limit = maxMessages
	}
	lines := []string{truncate(fmt.Sprintf("Thread: %d messages", len(threadEmails)), innerW)}
	for _, threadEmail := range threadEmails[:limit] {
		line := draftThreadContextLine(threadEmail, innerW)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if hidden := len(threadEmails) - limit; hidden > 0 {
		lines = append(lines, truncate(fmt.Sprintf("+%d more in thread", hidden), innerW))
	}
	lines = append(lines, strings.Repeat("-", innerW))
	return lines
}

type emailPreviewRender struct {
	Panel             string
	NativeOverlays    []previewNativeOverlay
	DocumentStartLine int
	PanelInnerHeight  int
}

func (m *Model) renderEmailPreview() string {
	return m.renderEmailPreviewFrame().Panel
}

func (m *Model) renderEmailPreviewFrame() emailPreviewRender {
	w := m.timeline.previewWidth
	if w <= 0 {
		w = 40
	}
	innerW := w - 4 // left border + padding

	var sb strings.Builder
	var nativeOverlays []previewNativeOverlay
	documentStartLine := 0

	chrome := m.chromeState(m.buildLayoutPlan(m.windowWidth, m.windowHeight))
	headerActive := chrome.FocusedPanel == panelPreview
	borderColor := m.theme.PanelBorderColor(headerActive)
	headerStyle := m.theme.Text.Muted.Style()
	if headerActive {
		headerStyle = m.theme.Severity.Info.Style()
	}

	// Header block
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
	headerLines := renderPreviewHeaderLinesWithTheme(m.theme, email, headerBody, category, bodyMatchesSelected && previewHasUnsubscribe(m.timeline.body), loadTag, innerW, headerActive)
	headerRows := selectableRowsFromRendered(headerLines)
	renderedHeaderLines := renderPreviewSelectableRows(m.theme, headerRows, previewSelectionTimeline, m.previewSelection, 0)
	selectionRowOffset := len(headerRows)
	for _, line := range renderedHeaderLines {
		sb.WriteString(line + "\n")
	}

	panelHeight := m.timelinePreviewInnerHeight()
	// Reserve 1 row for scroll indicator beneath the dynamic header block.
	maxBodyLines := panelHeight - len(headerLines) - 1
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}

	// Reserve rows for the quick reply picker overlay when open.
	pickerLines := 0
	if m.timeline.quickReplyOpen {
		pickerLines = m.quickReplyPickerHeight(innerW)
		maxBodyLines -= pickerLines
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}
	}

	dimStyle := headerStyle
	threadContextLines := m.renderDraftThreadContextLines(email, innerW, 4)
	if len(threadContextLines) > 0 {
		maxBodyLines -= len(threadContextLines)
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}
		renderedThreadLines := make([]string, 0, len(threadContextLines))
		for _, line := range threadContextLines {
			renderedThreadLines = append(renderedThreadLines, dimStyle.Render(line))
		}
		for _, line := range renderPreviewSelectableLines(m.theme, renderedThreadLines, previewSelectionTimeline, m.previewSelection, selectionRowOffset) {
			sb.WriteString(line + "\n")
		}
		selectionRowOffset += len(threadContextLines)
	}
	if m.timeline.bodyLoading || !bodyMatchesSelected {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.timeline.body != nil {
		bodyChromeLines := 0
		imageMode := m.currentPreviewImageMode()
		if nImg := len(m.timeline.body.InlineImages); nImg > 0 {
			label := splitInlineImageHint(nImg, imageMode)
			sb.WriteString(dimStyle.Render(truncate(label, innerW)) + "\n")
			bodyChromeLines++
			selectionRowOffset++
		}
		if nRemote := m.timelineRemoteImageCount(); nRemote > 0 {
			label := splitRemoteImageHint(nRemote, imageMode, m.timelineRemoteRevealAvailable())
			sb.WriteString(dimStyle.Render(truncate(label, innerW)) + "\n")
			bodyChromeLines++
			selectionRowOffset++
		}

		// Show downloadable attachments
		attachStyle := m.theme.Text.Primary.Style()
		selectedAttachStyle := m.theme.Focus.SelectionActive.Style()
		for i, att := range m.timeline.body.Attachments {
			sizeStr := fmt.Sprintf("%.1f KB", float64(att.Size)/1024)
			if att.Size >= 1024*1024 {
				sizeStr = fmt.Sprintf("%.1f MB", float64(att.Size)/(1024*1024))
			}
			label := fmt.Sprintf("[attach] %s  %s  %s", att.Filename, att.MIMEType, sizeStr)
			label = truncate(label, innerW)
			if i == m.timeline.selectedAttachment {
				sb.WriteString(selectedAttachStyle.Render(label) + "\n")
			} else {
				sb.WriteString(attachStyle.Render(label) + "\n")
			}
			bodyChromeLines++
			selectionRowOffset++
		}

		for _, line := range m.renderCalendarInvitationPrompt(innerW) {
			sb.WriteString(line + "\n")
			bodyChromeLines++
			selectionRowOffset++
		}

		// Save-path prompt
		if m.timeline.attachmentSavePrompt {
			promptStyle := m.theme.Severity.Info.Style()
			if m.timeline.attachmentSaveWarning != "" {
				sb.WriteString(promptStyle.Render(truncate(m.timeline.attachmentSaveWarning, innerW)) + "\n")
				bodyChromeLines++
				selectionRowOffset++
			}
			sb.WriteString(promptStyle.Render("Save to: ") + m.timeline.attachmentSaveInput.View() + "\n")
			bodyChromeLines++
			selectionRowOffset++
		}
		if len(m.timeline.body.Attachments) > 0 {
			sb.WriteString(previewAttachmentDivider(innerW) + "\n")
			sb.WriteString("\n")
			bodyChromeLines += 2
			selectionRowOffset += 2
		}

		visibleLines := maxBodyLines - bodyChromeLines
		if visibleLines < 1 {
			visibleLines = 1
		}

		documentStartLine = strings.Count(sb.String(), "\n")
		layout := m.timelinePreviewDocumentLayout(innerW, visibleLines)
		m.timeline.bodyScrollOffset = clampPreviewScrollOffset(m.timeline.bodyScrollOffset, layout.TotalRows, visibleLines)
		if m.previewSelection.activeOn(previewSelectionTimeline) && !m.previewSelection.Mouse {
			m.previewSelection.ensureCursorVisible(&m.timeline.bodyScrollOffset, visibleLines, layout.TotalRows)
		}
		viewport := renderPreviewDocumentViewportWithSelection(m.theme, layout, m.timeline.bodyScrollOffset, visibleLines,
			m.previewSelection, previewSelectionTimeline)
		if m.previewSelection.activeOn(previewSelectionTimeline) && m.previewSelection.Mouse {
			viewportLines := strings.Split(viewport.Content, "\n")
			viewport.Content = strings.Join(renderPreviewSelectableLines(m.theme, viewportLines, previewSelectionTimeline, m.previewSelection, selectionRowOffset), "\n")
		}
		sb.WriteString(viewport.Content)
		nativeOverlays = viewport.NativeOverlays
		m.timeline.bodyWrappedLines = previewLayoutPlainRows(layout)
		m.timeline.bodyWrappedWidth = innerW

		// Scroll indicator (pinned to bottom)
		if layout.TotalRows > visibleLines {
			maxOffset := layout.TotalRows - visibleLines
			pct := 0
			if maxOffset > 0 {
				pct = m.timeline.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%", m.timeline.bodyScrollOffset+1, layout.TotalRows, pct)
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
		}
	}

	// Quick reply picker overlay at the bottom of the preview panel.
	if m.timeline.quickReplyOpen {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(m.renderQuickReplyPicker(innerW, pickerLines))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		PaddingLeft(1)

	return emailPreviewRender{
		Panel:             panelStyle.Render(fitPanelContentHeight(sb.String(), panelHeight)),
		NativeOverlays:    nativeOverlays,
		DocumentStartLine: documentStartLine,
		PanelInnerHeight:  panelHeight,
	}
}

// renderBodyLines joins lines[start:end] into a string, applying the active
// highlight to lines within [visualStart, visualEnd] when visualMode is true.
func renderBodyLines(lines []string, start, end int, visualMode bool, visualStart, visualEnd int) string {
	return renderBodyLinesWithTheme(defaultTheme, lines, start, end, visualMode, visualStart, visualEnd)
}

func renderBodyLinesWithTheme(theme Theme, lines []string, start, end int, visualMode bool, visualStart, visualEnd int) string {
	if !visualMode {
		return strings.Join(lines[start:end], "\n")
	}
	highlightStyle := theme.Focus.VisualSelection.Style()
	lo, hi := visualStart, visualEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	var sb strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			sb.WriteByte('\n')
		}
		if i >= lo && i <= hi {
			sb.WriteString(highlightStyle.Render(lines[i]))
		} else {
			sb.WriteString(lines[i])
		}
	}
	return sb.String()
}

func (m *Model) renderFullScreenEmail() string {
	innerW, maxBodyLines := m.timelineFullScreenDocumentBudget()

	var sb strings.Builder
	var nativeImageTail string

	email := m.timeline.selectedEmail
	category := ""
	if email != nil {
		category = m.previewCategory(email.MessageID)
	}
	var headerBody *models.EmailBody
	if email != nil && m.timeline.bodyMessageID == email.MessageID {
		headerBody = m.timeline.body
	}
	loadTag := ""
	if email != nil {
		loadTag = previewLoadTag(m.timeline.previewLoad, email.MessageID)
	}
	headerLines := renderPreviewHeaderLinesWithLoad(email, headerBody, category, previewHasUnsubscribe(headerBody), loadTag, innerW, true)
	headerRows := selectableRowsFromRendered(headerLines)
	selectionRowOffset := len(headerRows)
	for _, line := range renderPreviewSelectableRows(m.theme, headerRows, previewSelectionTimeline, m.previewSelection, 0) {
		sb.WriteString(line + "\n")
	}
	bodyStartRow := len(headerLines) + 1
	promptLines := m.renderCalendarInvitationPrompt(innerW)
	if len(promptLines) > 0 {
		for _, line := range renderPreviewSelectableLines(m.theme, promptLines, previewSelectionTimeline, m.previewSelection, selectionRowOffset) {
			sb.WriteString(line + "\n")
		}
		maxBodyLines -= len(promptLines)
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}
		bodyStartRow += len(promptLines)
		selectionRowOffset += len(promptLines)
	}

	dimStyle := m.theme.Text.Muted.Style()
	threadContextLines := m.renderDraftThreadContextLines(email, innerW, 6)
	if len(threadContextLines) > 0 {
		maxBodyLines -= len(threadContextLines)
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}
		renderedThreadLines := make([]string, 0, len(threadContextLines))
		for _, line := range threadContextLines {
			renderedThreadLines = append(renderedThreadLines, dimStyle.Render(line))
		}
		for _, line := range renderPreviewSelectableLines(m.theme, renderedThreadLines, previewSelectionTimeline, m.previewSelection, selectionRowOffset) {
			sb.WriteString(line + "\n")
		}
		bodyStartRow += len(threadContextLines)
		selectionRowOffset += len(threadContextLines)
	}

	if m.timeline.bodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.timeline.body != nil {
		layout := m.timelinePreviewDocumentLayout(innerW, maxBodyLines)
		m.timeline.bodyScrollOffset = clampPreviewScrollOffset(m.timeline.bodyScrollOffset, layout.TotalRows, maxBodyLines)
		if m.previewSelection.activeOn(previewSelectionTimeline) && !m.previewSelection.Mouse {
			m.previewSelection.ensureCursorVisible(&m.timeline.bodyScrollOffset, maxBodyLines, layout.TotalRows)
		}
		viewport := renderPreviewDocumentViewportWithSelection(m.theme, layout, m.timeline.bodyScrollOffset, maxBodyLines,
			m.previewSelection, previewSelectionTimeline)
		if m.previewSelection.activeOn(previewSelectionTimeline) && m.previewSelection.Mouse {
			viewportLines := strings.Split(viewport.Content, "\n")
			viewport.Content = strings.Join(renderPreviewSelectableLines(m.theme, viewportLines, previewSelectionTimeline, m.previewSelection, selectionRowOffset), "\n")
		}
		sb.WriteString(viewport.Content)
		nativeImageTail = renderNativeImageOverlayTail(viewport.NativeOverlays, bodyStartRow, 1)

		if layout.TotalRows > maxBodyLines {
			maxOffset := layout.TotalRows - maxBodyLines
			pct := 0
			if maxOffset > 0 {
				pct = m.timeline.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%  │  z/esc: exit full-screen", m.timeline.bodyScrollOffset+1, layout.TotalRows, pct)
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
		} else {
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(" z/esc: exit full-screen", innerW)))
		}
	}

	// Quick reply picker overlay at the bottom of full-screen view.
	if m.timeline.quickReplyOpen {
		pickerLines := m.quickReplyPickerHeight(innerW)
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(m.renderQuickReplyPicker(innerW, pickerLines))
	}

	rendered := lipgloss.NewStyle().
		Width(m.windowWidth).
		Height(m.windowHeight).
		Render(sb.String())
	return appendNativeImageOverlayTailWithinRows(rendered, nativeImageTail, m.windowHeight)
}

func (m *Model) currentPreviewImageMode() previewImageMode {
	return detectPreviewImageMode(m.previewImageMode, m.localImageLinks, !m.localImageLinks)
}

func (m *Model) timelineFullScreenDocumentBudget() (int, int) {
	innerW := m.windowWidth - 2
	if innerW < 10 {
		innerW = 10
	}

	email := m.timeline.selectedEmail
	category := ""
	if email != nil {
		category = m.previewCategory(email.MessageID)
	}
	var headerBody *models.EmailBody
	if email != nil && m.timeline.bodyMessageID == email.MessageID {
		headerBody = m.timeline.body
	}
	loadTag := ""
	if email != nil {
		loadTag = previewLoadTag(m.timeline.previewLoad, email.MessageID)
	}
	headerLines := renderPreviewHeaderLinesWithLoad(email, headerBody, category, previewHasUnsubscribe(headerBody), loadTag, innerW, true)

	maxBodyLines := m.windowHeight - len(headerLines) - 1
	if m.timeline.quickReplyOpen {
		pickerRows := 2 + len(m.timeline.quickReplies)
		if len(m.timeline.quickReplies) == 0 {
			pickerRows = 3
		}
		if pickerRows > 12 {
			pickerRows = 12
		}
		maxBodyLines -= pickerRows
	}
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	return innerW, maxBodyLines
}

func (m *Model) timelineFullScreenDocumentLayout() previewDocumentLayout {
	innerW, maxBodyLines := m.timelineFullScreenDocumentBudget()
	return m.timelinePreviewDocumentLayout(innerW, maxBodyLines)
}

func (m *Model) timelineNativeImageClearCmd() tea.Cmd {
	if !m.timelineNativeImageClearNeeded() {
		return nil
	}
	return tea.ClearScreen
}

func (m *Model) timelineNativeImageClearNeeded() bool {
	switch m.currentPreviewImageMode() {
	case previewImageModeIterm2, previewImageModeKitty:
	default:
		return false
	}
	if m.timeline.body == nil {
		return false
	}
	if len(m.timeline.body.InlineImages) > 0 || m.timelineHasLoadedRemoteImages() {
		return true
	}
	return false
}

func (m *Model) timelineIterm2NativeImageRepaintCmd() tea.Cmd {
	return m.timelineNativeImageClearCmd()
}

func (m *Model) timelineHasLoadedRemoteImages() bool {
	for _, state := range m.timeline.remoteImageLoads {
		if len(state.Image.Data) > 0 {
			return true
		}
	}
	return false
}

func (m *Model) timelinePreviewDocumentLayout(innerW, availableRows int) previewDocumentLayout {
	mode := m.currentPreviewImageMode()
	messageID := m.timeline.bodyMessageID
	scopeKey := "timeline:" + messageID
	if m.timeline.previewDocLayout != nil &&
		m.timeline.previewDocWidth == innerW &&
		m.timeline.previewDocRows == availableRows &&
		m.timeline.previewDocMode == mode &&
		m.timeline.previewDocMessageID == messageID &&
		m.timeline.previewDocRemoteRevision == m.timeline.remoteImageRevision &&
		(mode != previewImageModeLinks || m.imagePreviewLinks == nil || m.imagePreviewLinks.CurrentKey() == scopeKey) {
		return *m.timeline.previewDocLayout
	}

	doc := buildPreviewDocument(m.timeline.body, m.timeline.inlineImageDescs)
	images := previewDocumentRenderableImages(doc, m.timeline.remoteImageLoads)
	imageLinks := m.localImageLinkMap(scopeKey, images, mode)
	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    innerW,
		AvailableRows: availableRows,
		ImageMode:     mode,
		Descriptions:  m.timeline.inlineImageDescs,
		ImageLinks:    imageLinks,
		RemoteImages:  m.timeline.remoteImageLoads,
	})
	m.timeline.previewDocLayout = &layout
	m.timeline.previewDocWidth = innerW
	m.timeline.previewDocRows = availableRows
	m.timeline.previewDocMode = mode
	m.timeline.previewDocMessageID = messageID
	m.timeline.previewDocRemoteRevision = m.timeline.remoteImageRevision
	return layout
}

func (m *Model) timelineFullScreenDocumentPlainRows() []string {
	layout := m.timelineFullScreenDocumentLayout()
	rows := make([]string, 0, len(layout.Rows))
	for _, row := range layout.Rows {
		if row.CopyPlain != "" {
			rows = append(rows, row.CopyPlain)
		} else {
			rows = append(rows, ansi.Strip(row.Content))
		}
	}
	return rows
}

func (m *Model) timelineFullScreenSelectedPlainText() string {
	rows := m.timelineFullScreenDocumentPlainRows()
	if len(rows) == 0 {
		return ""
	}
	start, end := m.timeline.visualStart, m.timeline.visualEnd
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end >= len(rows) {
		end = len(rows) - 1
	}
	if start > end || start >= len(rows) {
		return ""
	}
	return strings.Join(rows[start:end+1], "\n")
}

func (m *Model) timelineFullScreenCurrentPlainLine() string {
	rows := m.timelineFullScreenDocumentPlainRows()
	if m.timeline.bodyScrollOffset < 0 || m.timeline.bodyScrollOffset >= len(rows) {
		return ""
	}
	return rows[m.timeline.bodyScrollOffset]
}

func (m *Model) timelineFullScreenAllPlainText() string {
	return strings.Join(m.timelineFullScreenDocumentPlainRows(), "\n")
}

func (m *Model) localImageLinkMap(scopeKey string, images []models.InlineImage, mode previewImageMode) map[string]imagePreviewLink {
	if mode != previewImageModeLinks || len(images) == 0 {
		return nil
	}
	if m.imagePreviewLinks == nil {
		m.imagePreviewLinks = newImagePreviewServer()
	}
	links, err := m.imagePreviewLinks.RegisterSet(scopeKey, images)
	if err != nil {
		logger.Warn("local image preview links unavailable: %v", err)
		return nil
	}
	out := make(map[string]imagePreviewLink, len(links))
	for _, link := range links {
		out[normalizeContentID(link.ContentID)] = link
	}
	return out
}

// emptyStateView returns a placeholder string the same height as the content
// area, with msg centred vertically. Used when a table has no rows to display.
func (m *Model) emptyStateView(msg string) string {
	h := m.windowHeight - 7
	if h < 5 {
		h = 5
	}
	dim := m.theme.Text.Dim.Style()
	mid := h / 2
	var sb strings.Builder
	for i := 0; i < mid; i++ {
		sb.WriteString("\n")
	}
	sb.WriteString(dim.Render(msg))
	return sb.String()
}

// maybeUpdatePreview updates the preview panel when the cursor moves to a
// different email while the preview is already open. Returns nil if the preview
// is closed or the cursor already points to the displayed email.
func (m *Model) maybeUpdatePreview() tea.Cmd {
	if m.timeline.selectedEmail == nil {
		return nil // preview not open
	}
	cursor := m.timelineTable.Cursor()
	if cursor >= len(m.timeline.threadRowMap) {
		return nil
	}
	ref := m.timeline.threadRowMap[cursor]
	var email *models.EmailData
	if ref.kind == rowKindThread {
		// Collapsed thread header — preview the newest email in the group.
		if len(ref.group.emails) == 0 {
			return nil
		}
		email = ref.group.emails[0]
	} else {
		email = ref.group.emails[ref.emailIdx]
	}
	if email.MessageID == m.timeline.selectedEmail.MessageID {
		return nil // same email already shown
	}
	clearCmd := m.timelineNativeImageClearCmd()
	m.revokeImagePreviews()
	m.timeline.selectedEmail = email
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = true
	m.timeline.previewLoad = previewLoadTelemetry{}
	m.timeline.inlineImageDescs = nil
	m.timeline.bodyScrollOffset = 0
	m.timeline.bodyWrappedLines = nil
	m.clearTimelinePreviewDocumentCache()
	m.timeline.quickRepliesReady = false
	m.timeline.quickReplies = nil
	m.timeline.quickRepliesAIFetched = false
	loadCmd := m.loadEmailBodyForRefCmd(email.MessageRef())
	if clearCmd != nil {
		return tea.Sequence(clearCmd, loadCmd)
	}
	return loadCmd
}

func (m *Model) loadEmailBodyCmd(messageID, folder string, uid uint32) tea.Cmd {
	return m.loadEmailBodyForRefCmd(models.MessageRef{
		MessageID: messageID,
		Folder:    folder,
		UID:       uid,
	}.WithDefaults())
}

func (m *Model) loadEmailFullBodyForRefCmd(ref models.MessageRef) tea.Cmd {
	ref = ref.WithDefaults()
	messageID := ref.MessageID
	folder := ref.Folder
	uid := ref.UID
	if m.timeline.bodyFetchCancel != nil {
		m.timeline.bodyFetchCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	m.timeline.bodyFetchCancel = cancel
	m.timeline.bodyLoading = true
	b := m.backend
	return func() tea.Msg {
		defer cancel()
		started := time.Now()
		finish := func(body *models.EmailBody, err error, source string) EmailBodyMsg {
			finished := time.Now()
			return EmailBodyMsg{
				Body:           body,
				Err:            err,
				MessageRef:     ref,
				MessageID:      messageID,
				Folder:         folder,
				UID:            uid,
				LoadSource:     source,
				LoadStartedAt:  started,
				LoadFinishedAt: finished,
				LoadDuration:   finished.Sub(started),
			}
		}
		if ctx.Err() != nil {
			return finish(nil, ctx.Err(), previewLoadSourceIMAP)
		}
		if serviceBackend, ok := b.(messageBodyServiceBackend); ok {
			result, err := serviceBackend.GetMessage(ctx, ref)
			source := result.Source
			if strings.TrimSpace(source) == "" {
				source = previewLoadSourceIMAP
			}
			return finish(result.Body, err, source)
		}
		if uid == 0 {
			return finish(
				&models.EmailBody{TextPlain: "(Body unavailable: this cached email has no server UID yet, so Herald cannot safely load its full contents. Re-sync the folder or use server search to refresh it.)"},
				nil,
				previewLoadSourceUnavailable,
			)
		}
		body, err := b.FetchEmailBody(folder, uid)
		return finish(body, err, previewLoadSourceIMAP)
	}
}

func (m *Model) loadEmailBodyForRefCmd(ref models.MessageRef) tea.Cmd {
	ref = ref.WithDefaults()
	messageID := ref.MessageID
	folder := ref.Folder
	uid := ref.UID

	// Cancel any in-flight body fetch so rapid j/k doesn't pile up
	// concurrent IMAP requests that overwhelm the connection.
	if m.timeline.bodyFetchCancel != nil {
		m.timeline.bodyFetchCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	m.timeline.bodyFetchCancel = cancel

	b := m.backend // capture for goroutine
	return func() tea.Msg {
		defer cancel()
		started := time.Now()
		finish := func(body *models.EmailBody, err error, source string) EmailBodyMsg {
			finished := time.Now()
			return EmailBodyMsg{
				Body:           body,
				Err:            err,
				MessageRef:     ref,
				MessageID:      messageID,
				Folder:         folder,
				UID:            uid,
				LoadSource:     source,
				LoadStartedAt:  started,
				LoadFinishedAt: finished,
				LoadDuration:   finished.Sub(started),
			}
		}
		if ctx.Err() != nil {
			return finish(nil, ctx.Err(), previewLoadSourceIMAP)
		}
		if serviceBackend, ok := b.(messagePreviewServiceBackend); ok {
			result, err := serviceBackend.GetMessagePreview(ctx, ref, backend.MessageReadIntent{ViewID: "timeline-preview"})
			if ctx.Err() != nil {
				return finish(nil, ctx.Err(), previewLoadSourceIMAP)
			}
			source := result.Source
			if strings.TrimSpace(source) == "" {
				source = previewLoadSourceIMAP
			}
			return finish(result.Body, err, source)
		}
		if previewBackend, ok := b.(previewCacheBackend); ok {
			cached, err := previewBackend.GetCachedPreviewBody(messageID)
			if err != nil {
				logger.Debug("Preview cache lookup failed for %s: %v", messageID, err)
			} else if cached != nil {
				return finish(cached, nil, previewLoadSourceCache)
			}
		}
		if uid == 0 {
			return finish(
				&models.EmailBody{
					TextPlain: "(Body unavailable: this cached email has no server UID yet, so Herald cannot safely load its full contents. Re-sync the folder or use server search to refresh it.)",
				},
				nil,
				previewLoadSourceUnavailable,
			)
		}
		// Single attempt — IMAP layer handles reconnection internally.
		// Context cancellation handles rapid cursor movement.
		var body *models.EmailBody
		var err error
		if previewFetcher, ok := b.(previewFetchBackend); ok {
			body, err = previewFetcher.FetchPreviewBody(messageID, folder, uid)
		}
		if body == nil && err == nil {
			body, err = b.FetchEmailBody(folder, uid)
		}
		if ctx.Err() != nil {
			return finish(nil, ctx.Err(), previewLoadSourceIMAP)
		}
		if err == nil && body != nil {
			if previewBackend, ok := b.(previewCacheBackend); ok {
				if cacheErr := previewBackend.CachePreviewBody(messageID, body); cacheErr != nil {
					logger.Warn("Preview cache write failed for %s: %v", messageID, cacheErr)
				}
			}
		}
		return finish(body, err, previewLoadSourceIMAP)
	}
}

// buildCannedReplies returns 5 pre-written reply templates for the given sender.
func buildCannedReplies(senderName string) []string {
	firstName := senderName
	// Extract display name from "Name <email>" format
	if idx := strings.Index(senderName, "<"); idx > 0 {
		firstName = strings.TrimSpace(senderName[:idx])
	}
	// Use first word only
	if parts := strings.Fields(firstName); len(parts) > 0 {
		firstName = parts[0]
	}
	// Strip surrounding quotes if any
	firstName = strings.Trim(firstName, `"'`)
	if firstName == "" {
		firstName = "there"
	}
	return []string{
		"No thanks.",
		"Thank you for reaching out.",
		fmt.Sprintf("Thank you, %s.", firstName),
		"Copy that.",
		"I'll get back to you.",
	}
}

// generateQuickRepliesCmd returns a tea.Cmd that asks Ollama for AI reply suggestions.
func generateQuickRepliesCmd(classifier ai.AIClient, sender, subject, bodyPreview string) tea.Cmd {
	return func() tea.Msg {
		replies, err := classifier.GenerateQuickReplies(sender, subject, bodyPreview)
		return QuickRepliesMsg{Replies: replies, Err: err}
	}
}

// openQuickReply pre-fills Compose with the selected reply template and switches to it.
func (m *Model) openQuickReply(template string) (tea.Model, tea.Cmd) {
	m.timeline.quickReplyOpen = false
	if m.timeline.selectedEmail == nil {
		return m, nil
	}
	email := m.timeline.selectedEmail
	m.rememberComposeReturn()
	m.activeTab = tabCompose
	m.composePreserved = nil
	m.replyContextEmail = email
	m.composeAIThread = true
	m.resetComposeAIBar()
	m.composeTo.SetValue(email.Sender)
	subject := email.Subject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	m.composeSubject.SetValue(subject)
	m.composeBody.SetValue(template)
	m.setComposeSourceForEmail(email)
	m.applyConfiguredSignatureToComposeBody()
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Focus()
	m.resetFieldKeyMode()
	return m, nil
}

// renderQuickReplyPicker renders the quick reply picker overlay appended to the preview panel.
func (m *Model) quickReplyPickerHeight(width int) int {
	lines := m.quickReplyPickerLines(width, 12)
	if len(lines) == 0 {
		return 0
	}
	return len(lines)
}

func (m *Model) quickReplyPickerLines(width, maxLines int) []string {
	if maxLines < 3 {
		maxLines = 3
	}
	if width < 20 {
		width = 20
	}
	lines := []string{strings.Repeat("─", width)}

	header := "Quick Reply — ↑↓ navigate  Enter: compose  Esc: close"
	if !m.timeline.quickRepliesReady {
		header = "Quick Reply — generating suggestions…"
	}
	lines = append(lines, truncateVisual(header, width))

	if len(m.timeline.quickReplies) == 0 {
		if m.timeline.quickRepliesReady {
			lines = append(lines, "  No suggestions available")
		}
		return lines[:clampInt(len(lines), 0, maxLines)]
	}

	selected := clampInt(m.timeline.quickReplyIdx, 0, len(m.timeline.quickReplies)-1)
	start := 0
	available := maxLines - len(lines)
	if available < 1 {
		return lines[:maxLines]
	}
	rowsPerItem := 2
	windowItems := available / rowsPerItem
	if windowItems < 1 {
		windowItems = 1
	}
	if windowItems > len(m.timeline.quickReplies) {
		windowItems = len(m.timeline.quickReplies)
	}
	if selected >= windowItems {
		start = selected - windowItems + 1
	}
	end := start + windowItems
	if end > len(m.timeline.quickReplies) {
		end = len(m.timeline.quickReplies)
		start = max(0, end-windowItems)
	}

	for i := start; i < end; i++ {
		prefix := fmt.Sprintf("%d. ", i+1)
		replyWidth := width - ansi.StringWidth(prefix)
		if replyWidth < 8 {
			replyWidth = 8
		}
		replyLines := wrapLines(m.timeline.quickReplies[i], replyWidth)
		if len(replyLines) == 0 {
			replyLines = []string{""}
		}
		if len(replyLines) > 2 {
			replyLines = replyLines[:2]
			replyLines[1] = truncateVisual(replyLines[1], replyWidth)
		}
		lines = append(lines, prefix+truncateVisual(replyLines[0], replyWidth))
		if len(replyLines) > 1 {
			lines = append(lines, strings.Repeat(" ", ansi.StringWidth(prefix))+truncateVisual(replyLines[1], replyWidth))
		} else {
			lines = append(lines, "")
		}
		if len(lines) >= maxLines {
			break
		}
	}

	return lines[:clampInt(len(lines), 0, maxLines)]
}

func (m *Model) renderQuickReplyPicker(width, maxLines int) string {
	if width < 20 {
		width = 20
	}
	headerStyle := m.theme.Text.Muted.Style()
	selectedStyle := m.theme.Focus.SelectionActive.Style().Width(width)
	normalStyle := m.theme.Text.Primary.Style().Width(width)
	dimStyle := m.theme.Text.Dim.Style()

	lines := m.quickReplyPickerLines(width, maxLines)
	selected := clampInt(m.timeline.quickReplyIdx, 0, max(0, len(m.timeline.quickReplies)-1))
	startReplyLine := 2

	var rendered []string
	for idx, line := range lines {
		switch {
		case idx == 0:
			rendered = append(rendered, dimStyle.Render(line))
		case idx == 1:
			rendered = append(rendered, headerStyle.Render(line))
		default:
			replyOffset := idx - startReplyLine
			replyIdx := replyOffset / 2
			actualIdx := replyIdx
			if len(m.timeline.quickReplies) > 0 {
				windowItems := max(1, (maxLines-startReplyLine)/2)
				start := 0
				if selected >= windowItems {
					start = selected - windowItems + 1
				}
				end := start + windowItems
				if end > len(m.timeline.quickReplies) {
					end = len(m.timeline.quickReplies)
					start = max(0, end-windowItems)
				}
				actualIdx = start + replyIdx
			}
			if actualIdx == selected {
				rendered = append(rendered, selectedStyle.Render(truncateVisual(line, width)))
			} else {
				rendered = append(rendered, normalStyle.Render(truncateVisual(line, width)))
			}
		}
	}

	return strings.Join(rendered, "\n")
}
