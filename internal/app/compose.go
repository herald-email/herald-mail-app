package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/ai"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	appsmtp "mail-processor/internal/smtp"
)

const (
	composeFieldTo = iota
	composeFieldCC
	composeFieldBCC
	composeFieldSubject
	composeFieldBody
	composeFieldForwardedAttachments
)

type composePreservedContext struct {
	kind                 models.PreservedMessageKind
	mode                 models.PreservationMode
	email                *models.EmailData
	body                 *models.EmailBody
	forwardedAttachments []models.ForwardedAttachment
	selectedAttachment   int
	loadWarning          string
}

type composeSuggestionLayout struct {
	visibleCount int
	compact      bool
}

func (m *Model) composeSuggestionLayout(tableHeight int) composeSuggestionLayout {
	if len(m.suggestions) == 0 {
		return composeSuggestionLayout{}
	}

	maxVisible := len(m.suggestions)
	if maxVisible > 5 {
		maxVisible = 5
	}

	for visible := maxVisible; visible >= 1; visible-- {
		if tableHeight-16-(visible+2) >= 1 {
			return composeSuggestionLayout{visibleCount: visible}
		}
	}

	if tableHeight-16-1 >= 1 {
		return composeSuggestionLayout{visibleCount: 1, compact: true}
	}

	return composeSuggestionLayout{}
}

func (m *Model) composeAdditionalRows(tableHeight int) int {
	rows := 0
	layout := m.composeSuggestionLayout(tableHeight)
	switch {
	case layout.visibleCount == 0:
	case layout.compact:
		rows++
	default:
		rows += layout.visibleCount + 2
	}
	if m.composeAISubjectHint != "" {
		rows++
	}
	if m.attachmentInputActive {
		rows++
	}
	rows += m.attachmentCompletionVisibleRows()
	if m.composePreserved != nil {
		rows++
		rows += len(m.composePreserved.forwardedAttachments)
	}
	rows += len(m.composeAttachments)
	if m.composeStatus != "" {
		rows++
	}
	return rows
}

func (m *Model) handleComposeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Attachment path input intercepts all keys while active
	if m.attachmentInputActive {
		return m.handleAttachmentPathKey(msg)
	}

	// When AI panel prompt is focused, route keystrokes to it
	if m.composeAIPanel && m.composeAIInput.Focused() {
		if msg.String() == "enter" {
			instruction := strings.TrimSpace(m.composeAIInput.Value())
			if instruction == "" {
				return m, nil
			}
			m.composeAILoading = true
			m.composeAIInput.SetValue("")
			return m, m.aiAssistCmd(instruction)
		}
		var cmd tea.Cmd
		m.composeAIInput, cmd = m.composeAIInput.Update(msg)
		return m, cmd
	}

	// When AI response textarea is focused, route to it
	if m.composeAIPanel && m.composeAIResponse.Focused() {
		var cmd tea.Cmd
		m.composeAIResponse, cmd = m.composeAIResponse.Update(msg)
		return m, cmd
	}

	// When AI panel is open, number keys 1-5 trigger quick actions
	if m.composeAIPanel && !m.composeAIInput.Focused() {
		actions := map[string]string{
			"1": "Improve the clarity and professionalism of this email",
			"2": "Shorten this email to be more concise",
			"3": "Lengthen this email with more detail",
			"4": "Rewrite this email in a formal tone",
			"5": "Rewrite this email in a casual, friendly tone",
		}
		if instruction, ok := actions[msg.String()]; ok {
			if m.composeBody.Value() == "" {
				m.composeStatus = "Write something first"
				return m, nil
			}
			m.composeAILoading = true
			return m, m.aiAssistCmd(instruction)
		}
	}

	// Autocomplete dropdown interactions take priority over normal field navigation
	if len(m.suggestions) > 0 {
		switch msg.String() {
		case "up":
			if m.suggestionIdx > 0 {
				m.suggestionIdx--
			}
			return m, nil
		case "down":
			if m.suggestionIdx < len(m.suggestions)-1 {
				m.suggestionIdx++
			}
			return m, nil
		case "enter", "tab":
			// Accept selected suggestion
			if m.suggestionIdx >= 0 && m.suggestionIdx < len(m.suggestions) {
				c := m.suggestions[m.suggestionIdx]
				label := c.DisplayName
				if label == "" {
					label = c.Email
				} else {
					label = fmt.Sprintf("%s <%s>", label, c.Email)
				}
				m.acceptSuggestion(label)
			}
			m.suggestions = nil
			m.suggestionIdx = -1
			if m.windowWidth > 0 {
				m.updateTableDimensions(m.windowWidth, m.windowHeight)
			}
			return m, nil
		case "esc":
			m.suggestions = nil
			m.suggestionIdx = -1
			if m.windowWidth > 0 {
				m.updateTableDimensions(m.windowWidth, m.windowHeight)
			}
			return m, nil
		}
		// Any other key: dismiss dropdown and fall through to normal key handling
		m.suggestions = nil
		m.suggestionIdx = -1
		if m.windowWidth > 0 {
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
		}
	}

	switch msg.String() {
	case "ctrl+s":
		return m, m.sendCompose()
	case "ctrl+p":
		m.composePreview = !m.composePreview
		return m, nil
	case "ctrl+o":
		if m.composePreserved != nil {
			m.cyclePreservationMode()
			return m, nil
		}
	case "ctrl+a":
		m.attachmentInputActive = true
		m.attachmentPathInput.SetValue("")
		m.attachmentPathInput.Focus()
		m.clearAttachmentCompletions()
		return m, nil
	case "ctrl+g":
		if m.classifier == nil {
			m.composeStatus = "No AI backend configured"
			return m, nil
		}
		m.composeAIPanel = !m.composeAIPanel
		if m.composeAIPanel {
			m.composeAIInput.Focus()
		} else {
			m.composeAIInput.Blur()
			m.composeAIResponse.Blur()
		}
		return m, nil
	case "ctrl+j":
		if m.classifier == nil {
			m.composeStatus = "No AI backend configured"
			return m, nil
		}
		if m.composeBody.Value() == "" && m.replyContextEmail == nil {
			m.composeStatus = "Write something first"
			return m, nil
		}
		m.composeAILoading = true
		return m, m.aiSubjectCmd()
	case "ctrl+enter":
		if m.composeAIPanel && m.composeAIResponse.Value() != "" {
			m.composeBody.SetValue(m.composeAIResponse.Value())
			m.composeAIPanel = false
			m.composeAIDiff = ""
			m.composeAIResponse.SetValue("")
			m.composeAIInput.Blur()
			m.composeAIResponse.Blur()
			m.composeBody.Focus()
			m.composeField = 4
		}
		return m, nil
	case "tab":
		// If a subject hint is pending, Tab accepts it
		if m.composeAISubjectHint != "" {
			m.composeSubject.SetValue(m.composeAISubjectHint)
			m.composeAISubjectHint = ""
			return m, nil
		}
		m.cycleComposeField()
		return m, nil
	case "esc":
		return m.handleEscKey()
	}
	if m.composeField == composeFieldForwardedAttachments {
		return m.handleForwardedAttachmentKey(msg)
	}
	// Forward all other keys to the focused field
	var cmd tea.Cmd
	switch m.composeField {
	case composeFieldTo:
		m.composeTo, cmd = m.composeTo.Update(msg)
		return m, tea.Batch(cmd, m.searchContactsCmd(currentComposeToken(m.composeTo.Value())))
	case composeFieldCC:
		m.composeCC, cmd = m.composeCC.Update(msg)
		return m, tea.Batch(cmd, m.searchContactsCmd(currentComposeToken(m.composeCC.Value())))
	case composeFieldBCC:
		m.composeBCC, cmd = m.composeBCC.Update(msg)
		return m, tea.Batch(cmd, m.searchContactsCmd(currentComposeToken(m.composeBCC.Value())))
	case composeFieldSubject:
		m.composeSubject, cmd = m.composeSubject.Update(msg)
	case composeFieldBody:
		m.composeBody, cmd = m.composeBody.Update(msg)
	}
	return m, cmd
}

func (m *Model) cyclePreservationMode() {
	if m.composePreserved == nil {
		return
	}
	switch m.composePreserved.mode {
	case models.PreservationModeSafe:
		m.composePreserved.mode = models.PreservationModeFidelity
	case models.PreservationModeFidelity:
		m.composePreserved.mode = models.PreservationModePrivacy
	default:
		m.composePreserved.mode = models.PreservationModeSafe
	}
}

func (m *Model) handleForwardedAttachmentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.composePreserved == nil || len(m.composePreserved.forwardedAttachments) == 0 {
		return m, nil
	}
	switch msg.String() {
	case "up", "k":
		if m.composePreserved.selectedAttachment > 0 {
			m.composePreserved.selectedAttachment--
		}
	case "down", "j":
		if m.composePreserved.selectedAttachment < len(m.composePreserved.forwardedAttachments)-1 {
			m.composePreserved.selectedAttachment++
		}
	case "x", "delete", "backspace":
		idx := m.composePreserved.selectedAttachment
		if idx >= 0 && idx < len(m.composePreserved.forwardedAttachments) {
			m.composePreserved.forwardedAttachments[idx].Include = false
		}
	case "ctrl+o":
		m.cyclePreservationMode()
	case "enter", "tab":
		m.cycleComposeField()
	}
	return m, nil
}

func (m *Model) handleAttachmentPathKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		path := m.attachmentPathInput.Value()
		if isDirectoryPath(path) {
			m.attachmentPathInput.SetValue(ensureTrailingPathSeparator(path))
			m.attachmentPathInput.CursorEnd()
			m.clearAttachmentCompletions()
			return m, nil
		}
		m.attachmentInputActive = false
		m.attachmentPathInput.SetValue("")
		m.attachmentPathInput.Blur()
		m.clearAttachmentCompletions()
		return m, addAttachmentCmd(expandTilde(path))
	case tea.KeyEsc:
		m.attachmentInputActive = false
		m.attachmentPathInput.SetValue("")
		m.attachmentPathInput.Blur()
		m.clearAttachmentCompletions()
		return m, nil
	case tea.KeyTab:
		m.applyAttachmentCompletion(1)
		return m, nil
	case tea.KeyShiftTab:
		m.applyAttachmentCompletion(-1)
		return m, nil
	case tea.KeyUp:
		if m.attachmentCompletionVisible {
			m.moveAttachmentCompletion(-1)
			return m, nil
		}
	case tea.KeyDown:
		if m.attachmentCompletionVisible {
			m.moveAttachmentCompletion(1)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.attachmentPathInput, cmd = m.attachmentPathInput.Update(msg)
	m.clearAttachmentCompletions()
	return m, cmd
}

func (m *Model) applyAttachmentCompletion(delta int) {
	if m.attachmentCompletionVisible && len(m.attachmentCompletions) > 0 {
		m.moveAttachmentCompletion(delta)
		return
	}

	input := m.attachmentPathInput.Value()
	result := completeAttachmentPath(input, ".")
	if result.Status != "" {
		m.composeStatus = result.Status
		m.clearAttachmentCompletions()
		return
	}

	m.composeStatus = ""
	m.attachmentCompletions = result.Candidates
	if result.Completed != "" && result.Completed != input {
		m.attachmentPathInput.SetValue(result.Completed)
		m.attachmentPathInput.CursorEnd()
		m.attachmentCompletionIdx = 0
		m.attachmentCompletionAnchor = result.Completed
		m.attachmentCompletionVisible = false
		if m.windowWidth > 0 {
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
		}
		return
	}

	if len(result.Candidates) == 1 {
		m.attachmentPathInput.SetValue(result.Candidates[0].Value)
		m.attachmentPathInput.CursorEnd()
		m.attachmentCompletionIdx = 0
		m.attachmentCompletionAnchor = result.Candidates[0].Value
		m.attachmentCompletionVisible = false
		return
	}

	if m.attachmentCompletionAnchor != input {
		m.attachmentCompletionIdx = 0
		m.attachmentCompletionAnchor = input
		m.attachmentCompletionVisible = false
		m.composeStatus = fmt.Sprintf("%d matches (Tab again to show)", len(result.Candidates))
		if m.windowWidth > 0 {
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
		}
		return
	}

	m.attachmentCompletionVisible = true
	m.attachmentCompletionIdx = 0
	m.moveAttachmentCompletion(0)
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
}

func (m *Model) moveAttachmentCompletion(delta int) {
	if len(m.attachmentCompletions) == 0 {
		return
	}
	if m.attachmentCompletionIdx < 0 || m.attachmentCompletionIdx >= len(m.attachmentCompletions) {
		m.attachmentCompletionIdx = 0
	}
	m.attachmentCompletionIdx = (m.attachmentCompletionIdx + delta) % len(m.attachmentCompletions)
	if m.attachmentCompletionIdx < 0 {
		m.attachmentCompletionIdx += len(m.attachmentCompletions)
	}
	selected := m.attachmentCompletions[m.attachmentCompletionIdx]
	m.attachmentPathInput.SetValue(selected.Value)
	m.attachmentPathInput.CursorEnd()
	m.composeStatus = ""
}

func (m *Model) clearAttachmentCompletions() {
	m.attachmentCompletions = nil
	m.attachmentCompletionIdx = -1
	m.attachmentCompletionVisible = false
	m.attachmentCompletionAnchor = ""
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
}

func (m *Model) attachmentCompletionVisibleRows() int {
	if !m.attachmentCompletionVisible || len(m.attachmentCompletions) == 0 {
		return 0
	}
	if len(m.attachmentCompletions) > 5 {
		return 5
	}
	return len(m.attachmentCompletions)
}

// renderSuggestionDropdown renders the autocomplete dropdown list.
// Returns an empty string when there are no suggestions.
func (m *Model) renderSuggestionDropdown() string {
	if len(m.suggestions) == 0 {
		return ""
	}
	tableHeight := 5
	if m.windowHeight > 0 {
		extraChromeRows := 0
		if m.hasTopSyncStrip() {
			extraChromeRows = 1
		}
		tableHeight = m.windowHeight - 7 - extraChromeRows
		if tableHeight < 5 {
			tableHeight = 5
		}
	}
	layout := m.composeSuggestionLayout(tableHeight)
	if layout.visibleCount == 0 {
		return ""
	}

	selectedStyle := lipgloss.NewStyle().
		Background(defaultTheme.TabActiveBg).
		Foreground(defaultTheme.ConfirmFg)
	normalStyle := lipgloss.NewStyle().
		Foreground(defaultTheme.TabInactiveFg)
	maxW := m.windowWidth - 6
	if maxW < 20 {
		maxW = 20
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(defaultTheme.TabActiveBg).
		Padding(0, 1).
		MaxWidth(maxW)

	if layout.compact {
		selectedIdx := m.suggestionIdx
		if selectedIdx < 0 || selectedIdx >= len(m.suggestions) {
			selectedIdx = 0
		}
		selected := m.suggestions[selectedIdx]
		label := selected.DisplayName
		if label == "" {
			label = selected.Email
		} else {
			label = fmt.Sprintf("%s <%s>", label, selected.Email)
		}
		more := len(m.suggestions) - 1
		if more > 0 {
			label = fmt.Sprintf("%s  (+%d more)", label, more)
		}
		return lipgloss.NewStyle().
			Foreground(defaultTheme.InfoFg).
			Render(truncateVisual("↓ "+label, maxW))
	}

	var rows []string
	for i, c := range m.suggestions[:layout.visibleCount] {
		label := c.DisplayName
		if label == "" {
			label = c.Email
		} else {
			label = fmt.Sprintf("%s <%s>", label, c.Email)
		}
		if i == m.suggestionIdx {
			rows = append(rows, selectedStyle.Render(label))
		} else {
			rows = append(rows, normalStyle.Render(label))
		}
	}
	if hidden := len(m.suggestions) - layout.visibleCount; hidden > 0 {
		rows = append(rows, normalStyle.Render(fmt.Sprintf("... %d more", hidden)))
	}
	return boxStyle.Render(strings.Join(rows, "\n"))
}

func (m *Model) renderAttachmentCompletionDropdown() string {
	rows := m.attachmentCompletionVisibleRows()
	if rows == 0 {
		return ""
	}

	start := 0
	if m.attachmentCompletionIdx >= rows {
		start = m.attachmentCompletionIdx - rows + 1
	}
	if start+rows > len(m.attachmentCompletions) {
		start = len(m.attachmentCompletions) - rows
	}
	if start < 0 {
		start = 0
	}

	maxW := m.windowWidth - 6
	if maxW < 20 {
		maxW = 20
	}
	selectedStyle := lipgloss.NewStyle().
		Background(defaultTheme.TabActiveBg).
		Foreground(defaultTheme.ConfirmFg)
	normalStyle := lipgloss.NewStyle().
		Foreground(defaultTheme.TabInactiveFg)

	rendered := make([]string, 0, rows)
	for i := start; i < start+rows && i < len(m.attachmentCompletions); i++ {
		prefix := "  "
		if i == m.attachmentCompletionIdx {
			prefix = "> "
		}
		label := truncateVisual(prefix+m.attachmentCompletions[i].Display, maxW)
		if i == m.attachmentCompletionIdx {
			rendered = append(rendered, selectedStyle.Render(label))
		} else {
			rendered = append(rendered, normalStyle.Render(label))
		}
	}
	return strings.Join(rendered, "\n")
}

// acceptSuggestion replaces the current token in the active address field
// with the accepted label (DisplayName <email>), followed by ", ".
func (m *Model) acceptSuggestion(label string) {
	replaceToken := func(existing, replacement string) string {
		if i := strings.LastIndex(existing, ","); i >= 0 {
			return existing[:i+1] + " " + replacement + ", "
		}
		return replacement + ", "
	}

	switch m.composeField {
	case composeFieldTo:
		m.composeTo.SetValue(replaceToken(m.composeTo.Value(), label))
		m.composeTo.CursorEnd()
	case composeFieldCC:
		m.composeCC.SetValue(replaceToken(m.composeCC.Value(), label))
		m.composeCC.CursorEnd()
	case composeFieldBCC:
		m.composeBCC.SetValue(replaceToken(m.composeBCC.Value(), label))
		m.composeBCC.CursorEnd()
	}
}

// cycleComposeField advances focus to the next compose input field.
// Order: To(0) → CC(1) → BCC(2) → Subject(3) → Body(4) → wrap.
func (m *Model) cycleComposeField() {
	fieldCount := 5
	if m.hasForwardedAttachments() {
		fieldCount = 6
	}
	m.composeField = (m.composeField + 1) % fieldCount
	// Clear autocomplete when moving away from address fields (0–2)
	if m.composeField > composeFieldBCC {
		m.suggestions = nil
		m.suggestionIdx = -1
	}
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	switch m.composeField {
	case composeFieldTo:
		m.composeTo.Focus()
	case composeFieldCC:
		m.composeCC.Focus()
	case composeFieldBCC:
		m.composeBCC.Focus()
	case composeFieldSubject:
		m.composeSubject.Focus()
	case composeFieldBody:
		m.composeBody.Focus()
	}
}

func (m *Model) hasForwardedAttachments() bool {
	return m.composePreserved != nil && len(m.composePreserved.forwardedAttachments) > 0
}

func (m *Model) buildPreservedComposeRequest(from, to, subject string, attachments []models.ComposeAttachment) (appsmtp.PreservedMessageRequest, error) {
	ctx := m.composePreserved
	if ctx == nil || ctx.email == nil || ctx.body == nil {
		return appsmtp.PreservedMessageRequest{}, fmt.Errorf("missing preserved reply/forward context")
	}
	originalAttachments := ctx.body.Attachments
	var omitted []string
	if ctx.kind == models.PreservedMessageKindForward && len(ctx.forwardedAttachments) > 0 {
		originalAttachments = make([]models.Attachment, 0, len(ctx.forwardedAttachments))
		for _, item := range ctx.forwardedAttachments {
			originalAttachments = append(originalAttachments, item.Attachment)
			if !item.Include || len(item.Attachment.Data) == 0 {
				omitted = append(omitted, item.Attachment.Filename)
			}
		}
	}
	messageID := firstNonEmptyString(ctx.body.MessageID, ctx.email.MessageID)
	return appsmtp.PreservedMessageRequest{
		Kind:            ctx.kind,
		Mode:            ctx.mode,
		From:            from,
		To:              to,
		CC:              m.composeCC.Value(),
		BCC:             m.composeBCC.Value(),
		Subject:         subject,
		TopNoteMarkdown: m.composeBody.Value(),
		Original: models.PreservedMessageOriginal{
			MessageID:    messageID,
			InReplyTo:    ctx.body.InReplyTo,
			References:   ctx.body.References,
			Sender:       ctx.email.Sender,
			Subject:      ctx.email.Subject,
			Date:         ctx.email.Date,
			TextPlain:    ctx.body.TextPlain,
			TextHTML:     ctx.body.TextHTML,
			InlineImages: ctx.body.InlineImages,
			Attachments:  originalAttachments,
		},
		ManualAttachments:              attachments,
		OmittedOriginalAttachmentNames: omitted,
	}, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// sendCompose sends the composed message via SMTP as multipart/alternative
// (HTML + plain-text fallback). The body textarea is treated as Markdown.
// Any staged attachments are sent as multipart/mixed parts.
// Local inline images referenced as ![alt](~/path) or ![alt](/path) are
// embedded as multipart/related parts with cid: references.
func (m *Model) sendCompose() tea.Cmd {
	mailer := m.mailer // snapshot before goroutine to avoid data races
	backend := m.backend
	demoMode := m.demoMode
	from := m.fromAddress
	to := m.composeTo.Value()
	cc := m.composeCC.Value()
	bcc := m.composeBCC.Value()
	subject := m.composeSubject.Value()
	markdownBody := m.composeBody.Value()
	attachments := m.composeAttachments // snapshot; cleared on success in Update()
	preserved := m.composePreserved
	preservedReq, preservedErr := appsmtp.PreservedMessageRequest{}, error(nil)
	if preserved != nil {
		preservedReq, preservedErr = m.buildPreservedComposeRequest(from, to, subject, attachments)
	}
	return func() tea.Msg {
		if to == "" {
			return ComposeStatusMsg{Message: "Error: To field is empty"}
		}
		if subject == "" {
			return ComposeStatusMsg{Message: "Error: Subject is empty"}
		}
		if demoMode {
			if backend != nil {
				if err := backend.SendEmail(to, subject, markdownBody, from); err != nil {
					return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", err), Err: err}
				}
			}
			return ComposeStatusMsg{Message: "Message sent!"}
		}
		if mailer == nil {
			return ComposeStatusMsg{Message: "Error: SMTP not configured", Err: fmt.Errorf("smtp not configured")}
		}
		if preserved != nil {
			if preservedErr != nil {
				return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", preservedErr), Err: preservedErr}
			}
			if err := mailer.SendPreserved(preservedReq); err != nil {
				return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", err), Err: err}
			}
			return ComposeStatusMsg{Message: "Message sent!"}
		}
		htmlBody, inlines, inlineErr := appsmtp.BuildInlineImages(markdownBody)
		if inlineErr != nil {
			// Log and fall back to plain markdown conversion without inline images.
			logger.Warn("inline image embedding failed: %v", inlineErr)
			htmlBody, _ = appsmtp.MarkdownToHTMLAndPlain(markdownBody)
			inlines = nil
		}
		_, plainText := appsmtp.MarkdownToHTMLAndPlain(markdownBody)
		err := mailer.SendWithInlineImages(from, to, subject, plainText, htmlBody, cc, bcc, attachments, inlines)
		if err != nil {
			return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", err), Err: err}
		}
		return ComposeStatusMsg{Message: "Message sent!"}
	}
}

// renderComposeView renders the compose tab content
func (m *Model) renderComposeView() string {
	var sb strings.Builder
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)

	labelStyle := lipgloss.NewStyle().
		Foreground(defaultTheme.TabInactiveFg).
		Width(plan.Compose.LabelWidth)
	activeFieldStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(defaultTheme.TabActiveBg)
	inactiveFieldStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(defaultTheme.BorderInactive)

	renderField := func(style lipgloss.Style, view string) string {
		return style.Width(plan.Compose.FieldInnerWidth).Render(view)
	}

	// To field
	toStyle := inactiveFieldStyle
	if m.composeField == 0 {
		toStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("To:"),
		renderField(toStyle, m.composeTo.View()),
	) + "\n")

	// CC field
	ccStyle := inactiveFieldStyle
	if m.composeField == 1 {
		ccStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("CC:"),
		renderField(ccStyle, m.composeCC.View()),
	) + "\n")

	// BCC field
	bccStyle := inactiveFieldStyle
	if m.composeField == 2 {
		bccStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("BCC:"),
		renderField(bccStyle, m.composeBCC.View()),
	) + "\n")

	// Autocomplete dropdown (shown when address field has suggestions)
	if drop := m.renderSuggestionDropdown(); drop != "" {
		sb.WriteString(drop + "\n")
	}

	// Subject field
	subStyle := inactiveFieldStyle
	if m.composeField == 3 {
		subStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("Subject:"),
		renderField(subStyle, m.composeSubject.View()),
	) + "\n")

	// Divider
	divWidth := plan.Compose.LabelWidth + plan.Compose.FieldInnerWidth + 2
	if divWidth < 10 {
		divWidth = 10
	}
	sb.WriteString(strings.Repeat("─", divWidth) + "\n")

	// Subject hint (shown below divider when a suggestion is pending)
	if m.composeAISubjectHint != "" {
		hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
		dimStyle := lipgloss.NewStyle().Foreground(defaultTheme.BorderInactive)
		hintText := m.composeAISubjectHint
		if len(hintText) > divWidth-30 && divWidth > 35 {
			hintText = hintText[:divWidth-30] + "…"
		}
		hint := hintStyle.Render("✨ "+hintText) +
			"  " + dimStyle.Render("Tab: accept  Esc: dismiss")
		sb.WriteString(hint + "\n")
	}

	// Body + optional AI panel
	bodyAreaWidth := plan.Compose.BodyInnerWidth + 2
	if bodyAreaWidth < 10 {
		bodyAreaWidth = 10
	}
	if m.composeAIPanel {
		// Split outer width between bordered body pane and bordered AI pane.
		totalOuterWidth := plan.Compose.BodyInnerWidth + 2
		panelWidth := 36 // inner width
		if totalOuterWidth < 90 {
			panelWidth = totalOuterWidth/2 - 3
		}
		if panelWidth < 24 {
			panelWidth = 24
		}
		panelOuterWidth := panelWidth + 2
		bodyOuterWidth := totalOuterWidth - panelOuterWidth - 2 // 2 for gap
		if bodyOuterWidth < 24 {
			bodyOuterWidth = 24
			panelOuterWidth = totalOuterWidth - bodyOuterWidth - 2
			panelWidth = panelOuterWidth - 2
		}
		bodyWidth := bodyOuterWidth - 2
		if bodyWidth < 20 {
			bodyWidth = 20
		}

		var bodyPane string
		if m.composePreview {
			previewLabel := lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Render("  Preview (Ctrl+P to edit)  ")
			body := m.composeBody.Value()
			if body == "" {
				body = "_empty body_"
			}
			rendered := body
			if r, err := glamour.Render(body, "dark"); err == nil {
				rendered = r
			}
			bodyPane = previewLabel + "\n" + lipgloss.NewStyle().Width(bodyWidth).Render(rendered)
		} else {
			bodyStyle := inactiveFieldStyle.Width(bodyWidth)
			if m.composeField == composeFieldBody {
				bodyStyle = activeFieldStyle.Width(bodyWidth)
			}
			m.composeBody.SetWidth(bodyWidth)
			bodyPane = bodyStyle.Render(m.composeBody.View())
		}
		panelPane := m.renderAIPanel(panelWidth)
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, bodyPane, "  ", panelPane) + "\n")
	} else {
		// Normal full-width body / preview
		if m.composePreview {
			previewLabel := lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Render("  Preview (Ctrl+P to edit)  ")
			sb.WriteString(previewLabel + "\n")
			body := m.composeBody.Value()
			if body == "" {
				body = "_empty body_"
			}
			if rendered, err := glamour.Render(body, "dark"); err == nil {
				sb.WriteString(rendered)
			} else {
				sb.WriteString(body + "\n")
			}
		} else {
			bodyStyle := inactiveFieldStyle.Width(plan.Compose.BodyInnerWidth)
			if m.composeField == composeFieldBody {
				bodyStyle = activeFieldStyle.Width(plan.Compose.BodyInnerWidth)
			}
			sb.WriteString(bodyStyle.Render(m.composeBody.View()) + "\n")
		}
	}

	if summary := m.renderComposePreservedSummary(plan.Compose.BodyInnerWidth); summary != "" {
		sb.WriteString(summary + "\n")
	}

	// Attachment path input prompt
	if m.attachmentInputActive {
		promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
		sb.WriteString(promptStyle.Render("Attach file: ") + m.attachmentPathInput.View() + "\n")
		if drop := m.renderAttachmentCompletionDropdown(); drop != "" {
			sb.WriteString(drop + "\n")
		}
	}

	// Staged attachments list
	for _, att := range m.composeAttachments {
		sizeStr := fmt.Sprintf("%.1f KB", float64(att.Size)/1024)
		if att.Size >= 1024*1024 {
			sizeStr = fmt.Sprintf("%.1f MB", float64(att.Size)/(1024*1024))
		}
		warnIcon := ""
		if att.Size > 10*1024*1024 {
			warnIcon = " ⚠ (>10 MB)"
		}
		label := fmt.Sprintf("  [attach] %s  (%s)%s", att.Filename, sizeStr, warnIcon)
		attachColor := "111"
		if att.Data == nil {
			attachColor = string(defaultTheme.ErrorFg)
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(attachColor)).Render(label) + "\n")
	}

	// Status message
	if m.composeStatus != "" {
		color := "86"
		if strings.HasPrefix(m.composeStatus, "Error") || strings.HasPrefix(m.composeStatus, "Send failed") || strings.HasPrefix(m.composeStatus, "Attach error") {
			color = "196"
		} else if strings.HasPrefix(m.composeStatus, "Warning") {
			color = "214"
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(m.composeStatus) + "\n")
	}

	return sb.String()
}

func (m *Model) renderComposePreservedSummary(width int) string {
	ctx := m.composePreserved
	if ctx == nil {
		return ""
	}
	if width < 20 {
		width = 20
	}
	htmlStatus := "plain fallback"
	if ctx.body != nil && strings.TrimSpace(ctx.body.TextHTML) != "" {
		htmlStatus = "HTML"
	}
	inlineCount := 0
	if ctx.body != nil {
		inlineCount = len(ctx.body.InlineImages)
	}
	included, total := 0, len(ctx.forwardedAttachments)
	for _, item := range ctx.forwardedAttachments {
		if item.Include && len(item.Attachment.Data) > 0 {
			included++
		}
	}
	attachmentText := ""
	if ctx.kind == models.PreservedMessageKindForward {
		attachmentText = fmt.Sprintf("  |  %d attachments", included)
		if total != included {
			attachmentText = fmt.Sprintf("  |  %d/%d attachments", included, total)
		}
	}
	summary := fmt.Sprintf("Preserved %s  |  %s  |  %s  |  %d inline%s%s  |  Ctrl+O: mode",
		preservedKindLabel(ctx.kind),
		preservationModeLabel(ctx.mode),
		htmlStatus,
		inlineCount,
		pluralSuffix(inlineCount),
		attachmentText,
	)
	rows := []string{truncateVisual(summary, width)}
	if len(ctx.forwardedAttachments) > 0 {
		selectedStyle := lipgloss.NewStyle().Foreground(defaultTheme.ConfirmFg)
		normalStyle := lipgloss.NewStyle().Foreground(defaultTheme.InfoFg)
		removedStyle := lipgloss.NewStyle().Foreground(defaultTheme.BorderInactive)
		for i, item := range ctx.forwardedAttachments {
			status := "include"
			style := normalStyle
			if !item.Include || len(item.Attachment.Data) == 0 {
				status = "removed"
				style = removedStyle
			}
			prefix := "  "
			if m.composeField == composeFieldForwardedAttachments && i == ctx.selectedAttachment {
				prefix = "> "
				style = selectedStyle
			}
			label := fmt.Sprintf("%s[%s] %s  (x remove)", prefix, status, item.Attachment.Filename)
			rows = append(rows, style.Render(truncateVisual(label, width)))
		}
	}
	return strings.Join(rows, "\n")
}

func preservedKindLabel(kind models.PreservedMessageKind) string {
	switch kind {
	case models.PreservedMessageKindReply:
		return "reply"
	default:
		return "forward"
	}
}

func preservationModeLabel(mode models.PreservationMode) string {
	switch models.NormalizePreservationMode(mode) {
	case models.PreservationModeFidelity:
		return "Fidelity"
	case models.PreservationModePrivacy:
		return "Privacy"
	default:
		return "Safe"
	}
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// --- Draft auto-save helpers ---

// draftSaveTick returns a Cmd that fires DraftSaveTickMsg after 30 seconds.
func draftSaveTick() tea.Cmd {
	return tea.Tick(30*time.Second, func(_ time.Time) tea.Msg {
		return DraftSaveTickMsg{}
	})
}

// currentComposeToken returns the text after the last comma in s, trimmed.
// This is the fragment being typed for autocomplete in a comma-separated
// address field.
func currentComposeToken(s string) string {
	if i := strings.LastIndex(s, ","); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s)
}

// searchContactsCmd queries SearchContacts with token and returns a
// ContactSuggestionsMsg. Clears suggestions when token is shorter than 2 chars.
func (m *Model) searchContactsCmd(token string) tea.Cmd {
	if len(token) < 2 {
		return func() tea.Msg { return ContactSuggestionsMsg{} }
	}
	backend := m.backend
	return func() tea.Msg {
		contacts, err := backend.SearchContacts(token)
		if err != nil || len(contacts) == 0 {
			return ContactSuggestionsMsg{}
		}
		if len(contacts) > 5 {
			contacts = contacts[:5]
		}
		return ContactSuggestionsMsg{Contacts: contacts}
	}
}

// composeHasContent returns true if any compose field is non-empty.
func composeHasContent(m *Model) bool {
	return m.composeTo.Value() != "" || m.composeSubject.Value() != "" || m.composeBody.Value() != "" || m.composePreserved != nil
}

// saveDraftCmd saves the current compose content as a draft.
// Snapshots the values before the goroutine to prevent data races.
func (m *Model) saveDraftCmd() tea.Cmd {
	backend := m.backend
	from := m.fromAddress
	to := m.composeTo.Value()
	cc := m.composeCC.Value()
	bcc := m.composeBCC.Value()
	subject := m.composeSubject.Value()
	body := m.composeBody.Value()
	attachments := m.composeAttachments
	preserved := m.composePreserved
	preservedReq, preservedErr := appsmtp.PreservedMessageRequest{}, error(nil)
	if preserved != nil {
		preservedReq, preservedErr = m.buildPreservedComposeRequest(from, to, subject, attachments)
	}
	return func() tea.Msg {
		if preserved != nil {
			if preservedErr != nil {
				return DraftSavedMsg{Err: preservedErr}
			}
			raw, err := appsmtp.BuildPreservedMIMEMessage(preservedReq)
			if err != nil {
				return DraftSavedMsg{Err: err}
			}
			uid, folder, err := backend.SaveRawDraft([]byte(raw))
			return DraftSavedMsg{UID: uid, Folder: folder, Err: err}
		}
		uid, folder, err := backend.SaveDraft(to, cc, bcc, subject, body)
		return DraftSavedMsg{UID: uid, Folder: folder, Err: err}
	}
}

// deleteDraftCmd deletes the draft with the given UID from the given folder.
func (m *Model) deleteDraftCmd(uid uint32, folder string) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		err := backend.DeleteDraft(uid, folder)
		return DraftDeletedMsg{Err: err}
	}
}

// tokenizeWords splits s into a slice of word and non-word tokens,
// preserving whitespace and punctuation as separate tokens.
// "Hello, world" → ["Hello", ",", " ", "world"]
func tokenizeWords(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(r))
		} else if r == ',' || r == '.' || r == '!' || r == '?' || r == ';' || r == ':' || r == '"' || r == '\'' || r == '(' || r == ')' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// lcsTokens returns the longest common subsequence of token slices a and b.
func lcsTokens(a, b []string) []string {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	result := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, a[i-1])
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
		result[l], result[r] = result[r], result[l]
	}
	return result
}

// wordDiff computes a word-level diff between original and revised and returns
// a lipgloss-styled string. Deleted tokens appear red with strikethrough,
// added tokens appear green, unchanged tokens are unstyled.
func wordDiff(original, revised string) string {
	delStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Strikethrough(true).
		Background(lipgloss.Color("52"))
	addStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("46")).
		Background(lipgloss.Color("22"))

	origTokens := tokenizeWords(original)
	revTokens := tokenizeWords(revised)
	common := lcsTokens(origTokens, revTokens)

	var sb strings.Builder
	i, j, k := 0, 0, 0
	for k < len(common) {
		for i < len(origTokens) && origTokens[i] != common[k] {
			sb.WriteString(delStyle.Render(origTokens[i]))
			i++
		}
		for j < len(revTokens) && revTokens[j] != common[k] {
			sb.WriteString(addStyle.Render(revTokens[j]))
			j++
		}
		sb.WriteString(common[k])
		i++
		j++
		k++
	}
	for i < len(origTokens) {
		sb.WriteString(delStyle.Render(origTokens[i]))
		i++
	}
	for j < len(revTokens) {
		sb.WriteString(addStyle.Render(revTokens[j]))
		j++
	}
	return sb.String()
}

// aiAssistCmd fires an AI body-rewrite request with the given instruction.
// If m.composeAIThread is true and m.replyContextEmail is non-nil, the
// original email's sender and subject are included as context.
func (m *Model) aiAssistCmd(instruction string) tea.Cmd {
	classifier := m.classifier
	draft := m.composeBody.Value()
	threadCtx := m.composeAIThread
	replyEmail := m.replyContextEmail

	return func() tea.Msg {
		if classifier == nil {
			return AIAssistMsg{Err: fmt.Errorf("no AI backend configured")}
		}
		if strings.TrimSpace(draft) == "" {
			return AIAssistMsg{Err: fmt.Errorf("draft is empty")}
		}

		var contextParts []string
		if threadCtx && replyEmail != nil {
			contextParts = append(contextParts,
				fmt.Sprintf("This email is a reply to:\nFrom: %s\nSubject: %s",
					replyEmail.Sender, replyEmail.Subject))
		}
		contextParts = append(contextParts, "Current draft:\n"+draft)
		context := strings.Join(contextParts, "\n\n")

		messages := []ai.ChatMessage{
			{
				Role: "system",
				Content: "You are an expert email writing assistant. " +
					"Rewrite the email body according to the user's instruction. " +
					"Return only the rewritten body text, no explanations or preamble.",
			},
			{
				Role:    "user",
				Content: instruction + "\n\n" + context,
			},
		}
		result, err := classifier.Chat(messages)
		if err != nil {
			return AIAssistMsg{Err: err}
		}
		return AIAssistMsg{Result: strings.TrimSpace(result)}
	}
}

// aiSubjectCmd fires an AI subject-suggestion request using the current
// draft body and, if available, the thread context.
func (m *Model) aiSubjectCmd() tea.Cmd {
	classifier := m.classifier
	draft := m.composeBody.Value()
	threadCtx := m.composeAIThread
	replyEmail := m.replyContextEmail

	return func() tea.Msg {
		if classifier == nil {
			return AISubjectMsg{Err: fmt.Errorf("no AI backend configured")}
		}

		var contextParts []string
		if threadCtx && replyEmail != nil {
			contextParts = append(contextParts,
				fmt.Sprintf("Original email subject: %s\nFrom: %s",
					replyEmail.Subject, replyEmail.Sender))
		}
		if strings.TrimSpace(draft) != "" {
			contextParts = append(contextParts, "Email body:\n"+draft)
		}
		if len(contextParts) == 0 {
			return AISubjectMsg{Err: fmt.Errorf("nothing to base a subject on")}
		}

		messages := []ai.ChatMessage{
			{
				Role: "system",
				Content: "You are an email writing assistant. " +
					"Suggest a concise, specific email subject line (maximum 10 words). " +
					"Return only the subject line text, no quotes, no explanation.",
			},
			{
				Role:    "user",
				Content: strings.Join(contextParts, "\n\n"),
			},
		}
		result, err := classifier.Chat(messages)
		if err != nil {
			return AISubjectMsg{Err: err}
		}
		return AISubjectMsg{Subject: strings.TrimSpace(result)}
	}
}

// renderAIPanel renders the compose AI assistant panel.
// Returns an empty string when composeAIPanel is false.
// width is the panel's character width.
func (m *Model) renderAIPanel(width int) string {
	if !m.composeAIPanel {
		return ""
	}
	if width < 20 {
		width = 20
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		Width(width)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(width)
	activeToggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("25")).
		Padding(0, 1)
	inactiveToggleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)
	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Margin(0, 1, 0, 0)
	acceptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("28")).
		Padding(0, 1)
	discardStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	// Title
	sb.WriteString(titleStyle.Render("🤖 AI Assistant") + "\n")
	sb.WriteString(strings.Repeat("─", width) + "\n")

	// Context toggle — only shown when replying (replyContextEmail != nil)
	if m.replyContextEmail != nil {
		sb.WriteString(labelStyle.Render("Context:") + "\n")
		threadLabel := inactiveToggleStyle.Render("Thread")
		draftLabel := inactiveToggleStyle.Render("Draft only")
		if m.composeAIThread {
			threadLabel = activeToggleStyle.Render("● Thread")
		} else {
			draftLabel = activeToggleStyle.Render("● Draft only")
		}
		sb.WriteString(threadLabel + "  " + draftLabel + "\n\n")
	}

	// Quick action buttons
	actions := []string{"Improve", "Shorten", "Lengthen", "Formal", "Casual"}
	var actionRow strings.Builder
	for _, a := range actions {
		actionRow.WriteString(actionStyle.Render(a))
	}
	sb.WriteString(actionRow.String() + "\n\n")

	// Free-form prompt input
	sb.WriteString(labelStyle.Render("Custom prompt:") + "\n")
	m.composeAIInput.Width = width - 2
	sb.WriteString(m.composeAIInput.View() + "\n\n")

	// Loading spinner
	if m.composeAILoading {
		sb.WriteString(spinnerStyle.Render("⠋ Thinking…") + "\n")
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Render(sb.String())
	}

	// Diff view (if result available)
	if m.composeAIDiff != "" {
		sb.WriteString(labelStyle.Render("Changes:") + "\n")
		diffStyle := lipgloss.NewStyle().
			Width(width - 2).
			MaxWidth(width - 2)
		sb.WriteString(diffStyle.Render(m.composeAIDiff) + "\n\n")
	}

	// Editable response textarea
	if m.composeAIResponse.Value() != "" || m.composeAIDiff != "" {
		sb.WriteString(labelStyle.Render("Suggestion (edit freely):") + "\n")
		m.composeAIResponse.SetWidth(width - 2)
		m.composeAIResponse.SetHeight(8)
		sb.WriteString(m.composeAIResponse.View() + "\n\n")

		// Accept / Discard
		sb.WriteString(acceptStyle.Render("✓ Accept") + "  " + discardStyle.Render("Discard") + "\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
			"Ctrl+Enter: accept  Esc: discard") + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("86")).
		Width(width).
		Render(sb.String())
}
