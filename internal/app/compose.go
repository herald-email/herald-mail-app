package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
)

const (
	composeFieldTo = iota
	composeFieldCC
	composeFieldBCC
	composeFieldSubject
	composeFieldBody
	composeFieldOriginalMessage
	composeFieldForwardedAttachments
	composeFieldFrom
)

type composePreservedContext struct {
	kind                 models.PreservedMessageKind
	mode                 models.PreservationMode
	email                *models.EmailData
	body                 *models.EmailBody
	forwardedAttachments []models.ForwardedAttachment
	selectedAttachment   int
	originalScrollOffset int
	loadWarning          string
}

type composeSuggestionLayout struct {
	visibleCount int
	compact      bool
}

type composeAIAction struct {
	Key         string
	Label       string
	Instruction string
}

type composeAIRewritePayload struct {
	Status    string `json:"status"`
	Text      string `json:"text"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

type composeAIRewriteError struct {
	Code    string
	Message string
}

func (e *composeAIRewriteError) Error() string {
	if e == nil {
		return ""
	}
	code := strings.TrimSpace(e.Code)
	message := strings.TrimSpace(e.Message)
	switch {
	case code != "" && message != "":
		return code + ": " + message
	case message != "":
		return message
	case code != "":
		return code
	default:
		return "AI rewrite failed"
	}
}

const (
	composeAIMenuTranslate = "translate"
	composeAIMenuStyle     = "style"
)

func composeAIQuickActions() []composeAIAction {
	return []composeAIAction{
		{
			Key:         "i",
			Label:       "Improve",
			Instruction: "Improve the clarity, flow, and professionalism of this email while preserving its meaning and key details.",
		},
		{
			Key:         "f",
			Label:       "Fix typos",
			Instruction: "Fix typos, grammar, punctuation, and awkward wording in this email while preserving the original meaning and level of detail.",
		},
		{
			Key:         "n",
			Label:       "Shorten",
			Instruction: "Shorten this email to the essential points while preserving all necessary context and requested actions.",
		},
		{
			Key:         "e",
			Label:       "Expand",
			Instruction: "Expand this email with helpful context, smoother transitions, and complete sentences while preserving its intent.",
		},
	}
}

func composeAIActionByKey(key string) (composeAIAction, bool) {
	for _, action := range composeAIQuickActions() {
		if action.Key == key {
			return action, true
		}
	}
	return composeAIAction{}, false
}

func composeAITranslateOptions() []string {
	return []string{"Spanish", "French", "German", "Japanese", "Portuguese", "Custom..."}
}

func composeAIStyleOptions() []string {
	return []string{"Professional", "Friendly", "Direct", "Formal", "Warmer", "Concise"}
}

func composeAISelectedLanguage(language string) string {
	if strings.TrimSpace(language) == "" {
		return "Spanish"
	}
	return language
}

func composeAISelectedStyle(style string) string {
	if strings.TrimSpace(style) == "" {
		return "Friendly"
	}
	return style
}

func composeAITranslateInstruction(language string) string {
	selected := composeAISelectedLanguage(language)
	instruction := fmt.Sprintf("Translate this email to %s as a natural, idiomatic translation while preserving formatting intent, names, dates, commitments, and the original meaning. Preserve names, signatures, separators, and line breaks. Do not transliterate source-language sentences, approximate their sounds, invent words, or output placeholder language.", selected)
	if strings.EqualFold(selected, "Japanese") {
		instruction += " For Japanese, use standard modern Japanese with normal kanji/kana where appropriate. Do not output random kana or hiragana-only gibberish; keep proper names such as sender names or product names unchanged unless a conventional Japanese rendering is clearly appropriate. Example: English `you are the best, Herald.` should translate naturally as `Herald、あなたは最高です。`, not as a phonetic kana string."
	}
	return instruction
}

func composeAIStyleInstruction(style string) string {
	return fmt.Sprintf("Rewrite this email in a %s style while preserving all facts, commitments, names, dates, and action items.", strings.ToLower(composeAISelectedStyle(style)))
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
	if m.composeAIPanel {
		rows++ // compact command bar with inline custom instruction input
		if m.composeAIMenu != "" {
			rows++
		}
		if m.composeAILoading {
			rows++
		}
		rows += m.composeAIReviewExtraRows()
	}
	if m.composeAISubjectHint != "" {
		rows++
	}
	rows += m.composeMemoryRadarRows(tableHeight)
	if m.attachmentInputActive {
		rows++
	}
	rows += m.attachmentCompletionVisibleRows()
	if m.composePreserved != nil {
		rows += m.composeOriginalPreviewRows(tableHeight)
		rows++
		rows += len(m.composePreserved.forwardedAttachments)
	}
	rows += len(m.composeAttachments)
	if m.composeStatus != "" {
		rows++
	}
	return rows
}

func (m *Model) refreshComposeLayout() {
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
}

func (m *Model) composeCCVisible() bool {
	return m.composeCCExpanded || strings.TrimSpace(m.composeCC.Value()) != "" || m.composeField == composeFieldCC
}

func (m *Model) composeBCCVisible() bool {
	return m.composeBCCExpanded || strings.TrimSpace(m.composeBCC.Value()) != "" || m.composeField == composeFieldBCC
}

func (m *Model) composeOptionalRecipientHintVisible() bool {
	return !m.composeCCVisible() || !m.composeBCCVisible()
}

func (m *Model) composeAIReviewActive() bool {
	return m.composeAIPanel && strings.TrimSpace(m.composeAIResponse.Value()) != ""
}

func (m *Model) composeAIReviewExtraRows() int {
	if !m.composeAIReviewActive() {
		return 0
	}
	if m.windowHeight <= 24 {
		return 7
	}
	// The framed review surface adds an outer frame, header, nested Changes
	// section, and action row around the suggestion editor.
	return 12
}

func (m *Model) composeFixedRows() int {
	fieldRows := 2 * 3 // To and Subject are always visible.
	if m.composeAccountPickerVisible() {
		fieldRows += 3
	}
	if m.composeCCVisible() {
		fieldRows += 3
	}
	if m.composeBCCVisible() {
		fieldRows += 3
	}
	if m.composeOptionalRecipientHintVisible() {
		fieldRows++
	}
	return fieldRows + 1 + 2 // divider + main editor borders.
}

func (m *Model) resetComposeAIBar() {
	m.composeAIPanel = true
	m.composeAIMenu = ""
	m.composeAIStyle = ""
	m.composeAITranslate = ""
	m.composeAIUndoBody = ""
	m.composeAIOriginal = ""
	m.composeAIShowOriginal = false
	m.composeAIDiff = ""
	m.composeAISubjectHint = ""
	m.composeAIInput.SetValue("")
	m.composeAIInput.Blur()
	m.composeAIResponse.SetValue("")
	m.composeAIResponse.Blur()
	m.composeAILoading = false
}

func (m *Model) handleComposeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Attachment path input intercepts all keys while active
	if m.attachmentInputActive {
		return m.handleAttachmentPathKey(msg)
	}

	switch shortcutKey(msg) {
	case "ctrl+alt+c":
		m.composeCCExpanded = true
		m.composeAIInput.Blur()
		m.composeAIResponse.Blur()
		m.focusComposeField(composeFieldCC)
		m.refreshComposeLayout()
		return m, nil
	case "ctrl+alt+b":
		m.composeBCCExpanded = true
		m.composeAIInput.Blur()
		m.composeAIResponse.Blur()
		m.focusComposeField(composeFieldBCC)
		m.refreshComposeLayout()
		return m, nil
	}

	if m.composeAIPanel {
		if model, cmd, handled := m.handleComposeAIKey(msg); handled {
			return model, cmd
		}
	}

	// When AI panel prompt is focused, route keystrokes to it
	if m.composeAIPanel && m.composeAIInput.Focused() {
		if shortcutKey(msg) == "ctrl+enter" {
			return m, m.sendCompose()
		}
		if msg.String() == "enter" {
			instruction := strings.TrimSpace(m.composeAIInput.Value())
			if instruction == "" {
				return m, nil
			}
			m.composeAILoading = true
			m.composeAIInput.SetValue("")
			m.refreshComposeLayout()
			return m, m.aiAssistCmd(instruction)
		}
		var cmd tea.Cmd
		m.composeAIInput, cmd = m.composeAIInput.Update(msg)
		return m, cmd
	}

	if m.composeAIReviewActive() {
		switch shortcutKey(msg) {
		case "tab":
			m.composeAIShowOriginal = !m.composeAIShowOriginal
			if m.composeAIShowOriginal {
				m.composeAIResponse.Blur()
			} else {
				m.composeAIResponse.Focus()
			}
			m.refreshComposeLayout()
			return m, nil
		case "esc":
			m.dismissComposeAIReview()
			return m, nil
		case "ctrl+enter":
			m.acceptComposeAIResponse()
			return m, nil
		}
		if m.composeAIShowOriginal {
			return m, nil
		}
		var cmd tea.Cmd
		m.composeAIResponse, cmd = m.composeAIResponse.Update(msg)
		return m, cmd
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

	switch shortcutKey(msg) {
	case "ctrl+enter":
		return m, m.sendCompose()
	case "ctrl+s":
		return m, m.sendCompose()
	case "ctrl+p":
		m.composePreview = !m.composePreview
		return m, nil
	case composeExternalEditorShortcut:
		m.composeStatus = "Opening editor for Compose body..."
		return m, m.openComposeExternalEditorCmd()
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
	case "ctrl+k":
		m.composeAIPanel = true
		if m.classifier == nil {
			m.composeStatus = "AI writing tools are disabled until an AI provider is configured"
		} else {
			m.composeAIInput.Focus()
			m.composeAIResponse.Blur()
		}
		m.refreshComposeLayout()
		return m, nil
	case "ctrl+g":
		if m.composeAIReviewActive() {
			m.dismissComposeAIReview()
			return m, nil
		}
		if !m.composeAIPanel {
			m.composeAIPanel = true
			if m.classifier != nil {
				m.composeAIInput.Focus()
				m.composeAIResponse.Blur()
			}
			m.refreshComposeLayout()
			return m, nil
		}
		if m.classifier == nil {
			m.composeAIPanel = false
			m.composeAIMenu = ""
			m.refreshComposeLayout()
			return m, nil
		}
		if !m.composeAIInput.Focused() && !m.composeAIResponse.Focused() {
			m.composeAIInput.Focus()
			m.composeAIResponse.Blur()
		} else {
			m.composeAIPanel = false
			m.composeAIMenu = ""
			m.composeAIInput.Blur()
			m.composeAIResponse.Blur()
		}
		m.refreshComposeLayout()
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
		m.refreshComposeLayout()
		return m, m.aiSubjectCmd()
	case "tab":
		// If a subject hint is pending, Tab accepts it
		if m.composeAISubjectHint != "" {
			m.composeSubject.SetValue(m.composeAISubjectHint)
			m.composeAISubjectHint = ""
			m.refreshComposeLayout()
			return m, nil
		}
		m.cycleComposeField()
		return m, nil
	case "esc":
		if model, cmd, handled := m.handleVimFieldKey(msg); handled {
			return model, cmd
		}
		return m.handleEscKey()
	}
	if m.composeField == composeFieldForwardedAttachments {
		return m.handleForwardedAttachmentKey(msg)
	}
	if m.composeField == composeFieldOriginalMessage {
		return m.handleOriginalMessageKey(msg)
	}
	if model, cmd, handled := m.handleComposeFromKey(msg); handled {
		return model, cmd
	}
	if model, cmd, handled := m.handleVimFieldKey(msg); handled {
		return model, cmd
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
		if msg.Key().Mod.Contains(tea.ModAlt) && msg.Key().Text != "" {
			cmd = nil
		}
	}
	return m, cmd
}

func (m *Model) handleComposeAIKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	key := shortcutKey(msg)
	switch key {
	case "ctrl+k":
		if m.classifier == nil {
			m.composeStatus = "AI writing tools are disabled until an AI provider is configured"
			return m, nil, true
		}
		m.composeAIPanel = true
		m.composeAIInput.Focus()
		m.composeAIResponse.Blur()
		m.refreshComposeLayout()
		return m, nil, true
	case "ctrl+t":
		if m.classifier == nil {
			m.composeStatus = "AI writing tools are disabled until an AI provider is configured"
			return m, nil, true
		}
		m.openComposeAIMenu(composeAIMenuTranslate)
		return m, nil, true
	case "ctrl+y":
		if m.classifier == nil {
			m.composeStatus = "AI writing tools are disabled until an AI provider is configured"
			return m, nil, true
		}
		m.openComposeAIMenu(composeAIMenuStyle)
		return m, nil, true
	case "ctrl+z":
		return m.undoComposeAIRewrite(), nil, true
	case "ctrl+f":
		if action, ok := composeAIActionByKey("f"); ok {
			model, cmd := m.startComposeAIAction(action)
			return model, cmd, true
		}
	case "ctrl+n":
		if action, ok := composeAIActionByKey("n"); ok {
			model, cmd := m.startComposeAIAction(action)
			return model, cmd, true
		}
	case "ctrl+e":
		if action, ok := composeAIActionByKey("e"); ok {
			model, cmd := m.startComposeAIAction(action)
			return model, cmd, true
		}
	case "ctrl+enter":
		if m.acceptComposeAIResponse() {
			return m, nil, true
		}
	}

	if m.composeAIMenu != "" {
		switch msg.String() {
		case "esc":
			m.composeAIMenu = ""
			m.refreshComposeLayout()
			return m, nil, true
		case "up", "k":
			return m, nil, true
		case "down", "j":
			return m, nil, true
		}
		if cmd, handled := m.selectComposeAIMenuOption(msg.String()); handled {
			return m, cmd, true
		}
		return m, nil, true
	}

	if !m.composeAIInput.Focused() && !m.composeAIResponse.Focused() {
		switch key {
		case "ctrl+u":
			return m.undoComposeAIRewrite(), nil, true
		}
	}
	return m, nil, false
}

func (m *Model) openComposeAIMenu(menu string) {
	m.composeAIMenu = menu
	m.composeAIInput.Blur()
	m.composeAIResponse.Blur()
	m.refreshComposeLayout()
}

func (m *Model) selectComposeAIMenuOption(key string) (tea.Cmd, bool) {
	index := -1
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		index = int(key[0] - '1')
	}
	if index < 0 {
		return nil, false
	}
	switch m.composeAIMenu {
	case composeAIMenuTranslate:
		options := composeAITranslateOptions()
		if index >= len(options) {
			return nil, true
		}
		selected := options[index]
		m.composeAIMenu = ""
		if selected == "Custom..." {
			m.composeAIInput.SetValue("Translate this email to ")
			m.composeAIInput.Focus()
			m.composeStatus = "Enter a target language, then press Enter"
			m.refreshComposeLayout()
			return nil, true
		}
		m.composeAITranslate = selected
		_, cmd := m.startComposeAIAction(composeAIAction{Instruction: composeAITranslateInstruction(selected)})
		return cmd, true
	case composeAIMenuStyle:
		options := composeAIStyleOptions()
		if index >= len(options) {
			return nil, true
		}
		selected := options[index]
		m.composeAIMenu = ""
		m.composeAIStyle = selected
		_, cmd := m.startComposeAIAction(composeAIAction{Instruction: composeAIStyleInstruction(selected)})
		return cmd, true
	default:
		m.composeAIMenu = ""
		m.refreshComposeLayout()
		return nil, true
	}
}

func (m *Model) startComposeAIAction(action composeAIAction) (tea.Model, tea.Cmd) {
	if m.classifier == nil {
		m.composeStatus = "AI writing tools are disabled until an AI provider is configured"
		return m, nil
	}
	if strings.TrimSpace(m.composeBody.Value()) == "" {
		m.composeStatus = "Write something first"
		return m, nil
	}
	m.composeAILoading = true
	m.composeStatus = ""
	m.refreshComposeLayout()
	return m, m.aiAssistCmd(action.Instruction)
}

func (m *Model) acceptComposeAIResponse() bool {
	if !m.composeAIPanel || m.composeAIResponse.Value() == "" {
		return false
	}
	m.composeAIUndoBody = m.composeBody.Value()
	m.composeBody.SetValue(m.composeAIResponse.Value())
	m.composeAIPanel = true
	m.composeAIMenu = ""
	m.composeAIDiff = ""
	m.composeAIOriginal = ""
	m.composeAIShowOriginal = false
	m.composeAIResponse.SetValue("")
	m.composeAIInput.Blur()
	m.composeAIResponse.Blur()
	m.composeBody.Focus()
	m.composeField = composeFieldBody
	m.refreshComposeLayout()
	return true
}

func (m *Model) dismissComposeAIReview() {
	m.composeAIPanel = true
	m.composeAIMenu = ""
	m.composeAIDiff = ""
	m.composeAIOriginal = ""
	m.composeAIShowOriginal = false
	m.composeAIResponse.SetValue("")
	m.composeAIInput.Blur()
	m.composeAIResponse.Blur()
	m.composeBody.Focus()
	m.composeField = composeFieldBody
	m.refreshComposeLayout()
}

func (m *Model) undoComposeAIRewrite() *Model {
	if m.composeAIUndoBody == "" {
		m.composeStatus = "Nothing to undo"
		return m
	}
	current := m.composeBody.Value()
	m.composeBody.SetValue(m.composeAIUndoBody)
	m.composeAIUndoBody = current
	m.composeStatus = "AI rewrite undone"
	m.composeField = composeFieldBody
	m.composeBody.Focus()
	m.composeAIInput.Blur()
	m.composeAIResponse.Blur()
	return m
}

func (m *Model) handleVimFieldKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.vimFieldProfileActive() {
		return m, nil, false
	}
	if m.fieldKeyMode == "" {
		if mode, ok := m.composeFieldDefaultMode(); ok {
			m.fieldKeyMode = mode
		} else {
			m.fieldKeyMode = keyboardModeNormal
		}
	}
	key := shortcutKey(msg)
	if m.fieldKeyMode == keyboardModeInsert {
		if key == "esc" {
			m.fieldKeyMode = keyboardModeNormal
			return m, nil, true
		}
		return m, nil, false
	}
	switch key {
	case "i":
		m.fieldKeyMode = keyboardModeInsert
		return m, nil, true
	case "a":
		m.fieldKeyMode = keyboardModeInsert
		m.moveFocusedComposeCursorEnd()
		return m, nil, true
	case "A":
		m.fieldKeyMode = keyboardModeInsert
		m.moveFocusedComposeCursorEnd()
		return m, nil, true
	case "v":
		m.fieldKeyMode = keyboardModeVisual
		return m, nil, true
	case "esc":
		if m.fieldKeyMode == keyboardModeVisual {
			m.fieldKeyMode = keyboardModeNormal
			return m, nil, true
		}
	}
	if key == "tab" || key == "shift+tab" || strings.HasPrefix(key, "ctrl+") {
		return m, nil, false
	}
	return m, nil, true
}

func (m *Model) composeFieldDefaultMode() (string, bool) {
	if m == nil || m.keyboard == nil {
		return "", false
	}
	mode := m.keyboard.FieldDefaultMode(keyboardFieldCompose)
	switch mode {
	case keyboardModeNormal, keyboardModeVisual:
		return mode, true
	default:
		return "", false
	}
}

func (m *Model) vimFieldProfileActive() bool {
	_, ok := m.composeFieldDefaultMode()
	return ok
}

func (m *Model) moveFocusedComposeCursorEnd() {
	switch m.composeField {
	case composeFieldTo:
		m.composeTo.CursorEnd()
	case composeFieldCC:
		m.composeCC.CursorEnd()
	case composeFieldBCC:
		m.composeBCC.CursorEnd()
	case composeFieldSubject:
		m.composeSubject.CursorEnd()
	case composeFieldBody:
		m.composeBody.CursorEnd()
	}
}

func (m *Model) handleOriginalMessageKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.composePreserved == nil {
		return m, nil
	}
	key := shortcutKey(msg)
	switch key {
	case "v":
		m.togglePreviewSelectionForSurface(previewSelectionComposeOriginal)
	case "left", "h":
		if m.previewSelection.activeOn(previewSelectionComposeOriginal) {
			m.moveActivePreviewSelection(0, -1)
		}
	case "right", "l":
		if m.previewSelection.activeOn(previewSelectionComposeOriginal) {
			m.moveActivePreviewSelection(0, 1)
		}
	case "up", "k":
		if m.previewSelection.activeOn(previewSelectionComposeOriginal) {
			m.moveActivePreviewSelection(-1, 0)
		} else if m.composePreserved.originalScrollOffset > 0 {
			m.composePreserved.originalScrollOffset--
		}
	case "down", "j":
		if m.previewSelection.activeOn(previewSelectionComposeOriginal) {
			m.moveActivePreviewSelection(1, 0)
		} else {
			m.composePreserved.originalScrollOffset++
		}
	case "y", "Y":
		if cmd, handled := m.handlePreviewCopyKey(previewSelectionComposeOriginal, key); handled {
			return m, cmd
		}
	case "enter":
		m.cycleComposeField()
	}
	return m, nil
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

func (m *Model) handleForwardedAttachmentKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.composePreserved == nil || len(m.composePreserved.forwardedAttachments) == 0 {
		return m, nil
	}
	switch shortcutKey(msg) {
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
			item := &m.composePreserved.forwardedAttachments[idx]
			if len(item.Attachment.Data) > 0 {
				item.Include = !item.Include
			}
		}
	case "ctrl+o":
		m.cyclePreservationMode()
	case "enter", "tab":
		m.cycleComposeField()
	}
	return m, nil
}

func (m *Model) handleAttachmentPathKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Code {
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
		if msg.Mod.Contains(tea.ModShift) {
			m.applyAttachmentCompletion(-1)
			return m, nil
		}
		m.applyAttachmentCompletion(1)
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
	tableHeight := m.composeContentHeight()
	layout := m.composeSuggestionLayout(tableHeight)
	if layout.visibleCount == 0 {
		return ""
	}

	selectedStyle := m.theme.Focus.SelectionActive.Style()
	normalStyle := m.theme.Chrome.TabInactive.Style()
	maxW := m.windowWidth - 6
	if maxW < 20 {
		maxW = 20
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.PanelBorderColor(true)).
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
			Foreground(m.theme.Severity.Info.ForegroundColor()).
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
	selectedStyle := m.theme.Focus.SelectionActive.Style()
	normalStyle := m.theme.Chrome.TabInactive.Style()

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

func (m *Model) focusComposeField(field int) {
	if field == composeFieldCC {
		m.composeCCExpanded = true
	}
	if field == composeFieldBCC {
		m.composeBCCExpanded = true
	}
	m.composeField = field
	if m.composeField > composeFieldBCC {
		m.suggestions = nil
		m.suggestionIdx = -1
	}
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	m.composeAIResponse.Blur()
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

func (m *Model) visibleComposeFields() []int {
	fields := make([]int, 0, 7)
	if m.composeAccountPickerVisible() {
		fields = append(fields, composeFieldFrom)
	}
	fields = append(fields, composeFieldTo)
	if m.composeCCVisible() {
		fields = append(fields, composeFieldCC)
	}
	if m.composeBCCVisible() {
		fields = append(fields, composeFieldBCC)
	}
	fields = append(fields, composeFieldSubject, composeFieldBody)
	if m.composePreserved != nil {
		fields = append(fields, composeFieldOriginalMessage)
	}
	if m.hasForwardedAttachments() {
		fields = append(fields, composeFieldForwardedAttachments)
	}
	return fields
}

// cycleComposeField advances focus to the next visible compose input field.
// Empty CC/BCC fields are skipped until expanded or populated.
func (m *Model) cycleComposeField() {
	fields := m.visibleComposeFields()
	if len(fields) == 0 {
		return
	}
	current := -1
	for i, field := range fields {
		if field == m.composeField {
			current = i
			break
		}
	}
	next := fields[0]
	if current >= 0 {
		next = fields[(current+1)%len(fields)]
	}
	m.focusComposeField(next)
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
	b := m.backend
	accountSender, accountSenderOK := b.(backend.AccountComposeBackend)
	demoMode := m.demoMode
	sourceID := m.composeSelectedSourceID()
	from := m.composeFromAddress()
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
	var preservedPtr *appsmtp.PreservedMessageRequest
	if preserved != nil {
		preservedPtr = &preservedReq
	}
	req := backend.ComposeSendRequest{
		SourceID:     sourceID,
		From:         from,
		To:           to,
		CC:           cc,
		BCC:          bcc,
		Subject:      subject,
		MarkdownBody: markdownBody,
		Attachments:  attachments,
		Preserved:    preservedPtr,
	}
	return func() tea.Msg {
		if to == "" {
			return ComposeStatusMsg{Message: "Error: To field is empty"}
		}
		if subject == "" {
			return ComposeStatusMsg{Message: "Error: Subject is empty"}
		}
		if accountSenderOK {
			if preserved != nil && preservedErr != nil {
				return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", preservedErr), Err: preservedErr}
			}
			if err := accountSender.SendCompose(req); err != nil {
				return ComposeStatusMsg{Message: fmt.Sprintf("Send failed: %v", err), Err: err}
			}
			return ComposeStatusMsg{Message: "Message sent!"}
		}
		if demoMode {
			if b != nil {
				if err := b.SendEmail(to, subject, markdownBody, from); err != nil {
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
		Foreground(m.theme.Chrome.TabInactive.ForegroundColor()).
		Width(plan.Compose.LabelWidth)
	activeFieldStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Chrome.TabActive.BackgroundColor())
	inactiveFieldStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Focus.PanelBorder.ForegroundColor())

	renderField := func(style lipgloss.Style, view string) string {
		return style.Width(plan.Compose.FieldInnerWidth).Render(view)
	}

	if m.composeAccountPickerVisible() {
		fromStyle := inactiveFieldStyle
		if m.composeField == composeFieldFrom {
			fromStyle = activeFieldStyle
		}
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			labelStyle.Render("From:"),
			renderField(fromStyle, m.composeFromPickerValue(plan.Compose.FieldInnerWidth)),
		) + "\n")
	}

	// To field
	toStyle := inactiveFieldStyle
	if m.composeField == composeFieldTo {
		toStyle = activeFieldStyle
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("To:"),
		renderField(toStyle, m.composeTo.View()),
	) + "\n")

	if m.composeOptionalRecipientHintVisible() {
		var hints []string
		if !m.composeCCVisible() {
			hints = append(hints, "Ctrl+Alt+C CC")
		}
		if !m.composeBCCVisible() {
			hints = append(hints, "Ctrl+Alt+B BCC")
		}
		hint := truncateVisual(strings.Join(hints, "  "), plan.Compose.FieldInnerWidth)
		sb.WriteString(labelStyle.Render("") + m.theme.Compose.AIDim.Style().Render(hint) + "\n")
	}

	// CC field
	if m.composeCCVisible() {
		ccStyle := inactiveFieldStyle
		if m.composeField == composeFieldCC {
			ccStyle = activeFieldStyle
		}
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			labelStyle.Render("CC:"),
			renderField(ccStyle, m.composeCC.View()),
		) + "\n")
	}

	// BCC field
	if m.composeBCCVisible() {
		bccStyle := inactiveFieldStyle
		if m.composeField == composeFieldBCC {
			bccStyle = activeFieldStyle
		}
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			labelStyle.Render("BCC:"),
			renderField(bccStyle, m.composeBCC.View()),
		) + "\n")
	}

	// Autocomplete dropdown (shown when address field has suggestions)
	if drop := m.renderSuggestionDropdown(); drop != "" {
		sb.WriteString(drop + "\n")
	}

	// Subject field
	subStyle := inactiveFieldStyle
	if m.composeField == composeFieldSubject {
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
	divider := strings.Repeat("─", divWidth)
	if m.composePreserved != nil {
		divider = composeSectionDivider("Response", divWidth)
	}
	if aiBar := m.renderComposeAIBar(divWidth); aiBar != "" {
		if m.composeAIMenu != "" {
			aiRows := strings.Split(aiBar, "\n")
			sb.WriteString(aiRows[0] + "\n")
			sb.WriteString(divider + "\n")
			if len(aiRows) > 1 {
				sb.WriteString(strings.Join(aiRows[1:], "\n") + "\n")
			}
		} else {
			sb.WriteString(aiBar + "\n")
			sb.WriteString(divider + "\n")
		}
	} else {
		sb.WriteString(divider + "\n")
	}

	// Subject hint (shown below divider when a suggestion is pending)
	if m.composeAISubjectHint != "" {
		hintStyle := m.theme.Compose.Accent.Style()
		dimStyle := lipgloss.NewStyle().Foreground(m.theme.Focus.PanelBorder.ForegroundColor())
		hintText := m.composeAISubjectHint
		if len(hintText) > divWidth-30 && divWidth > 35 {
			hintText = hintText[:divWidth-30] + "…"
		}
		hint := hintStyle.Render("✨ "+hintText) +
			"  " + dimStyle.Render("Tab: accept  Esc: dismiss")
		sb.WriteString(hint + "\n")
	}

	if radar := m.renderComposeMemoryRadar(plan.Compose.BodyInnerWidth); radar != "" {
		sb.WriteString(radar + "\n")
	}

	// Body
	if m.composeAIReviewActive() {
		sb.WriteString(m.renderComposeAIReview(plan.Compose.BodyInnerWidth, activeFieldStyle, inactiveFieldStyle) + "\n")
	} else if m.compactPreservedCompose() {
		sb.WriteString(m.renderCompactPreservedResponse(plan.Compose.BodyInnerWidth) + "\n")
	} else {
		// Normal full-width body / preview
		if m.composePreview {
			previewLabel := m.theme.Compose.Accent.Style().Render("  Preview (Ctrl+P to edit)  ")
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

	if aiResult := m.renderComposeAIResult(plan.Compose.BodyInnerWidth); aiResult != "" {
		sb.WriteString(aiResult + "\n")
	}

	if original := m.renderComposeOriginalMessagePreview(plan.Compose.BodyInnerWidth); original != "" {
		sb.WriteString(original + "\n")
	}

	if summary := m.renderComposePreservedSummary(plan.Compose.BodyInnerWidth); summary != "" {
		sb.WriteString(summary + "\n")
	}

	// Attachment path input prompt
	if m.attachmentInputActive {
		promptStyle := m.theme.Compose.Accent.Style()
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
		attachStyle := m.theme.Compose.Attachment.Style()
		if att.Data == nil {
			attachStyle = m.theme.Severity.Error.Style()
		}
		sb.WriteString(attachStyle.Render(label) + "\n")
	}

	// Status message
	if m.composeStatus != "" {
		statusStyle := m.theme.Compose.StatusInfo.Style()
		if strings.HasPrefix(m.composeStatus, "Error") || strings.HasPrefix(m.composeStatus, "Send failed") || strings.HasPrefix(m.composeStatus, "Attach error") {
			statusStyle = m.theme.Compose.StatusError.Style()
		} else if strings.HasPrefix(m.composeStatus, "Warning") {
			statusStyle = m.theme.Compose.StatusWarning.Style()
		}
		sb.WriteString(statusStyle.Render(m.composeStatus) + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (m *Model) isForwardCompose() bool {
	return m.composePreserved != nil && m.composePreserved.kind == models.PreservedMessageKindForward
}

func (m *Model) compactForwardCompose() bool {
	return m.isForwardCompose() && m.windowHeight > 0 && m.windowHeight <= 24
}

func (m *Model) compactPreservedCompose() bool {
	return m.composePreserved != nil && m.windowHeight > 0 && m.windowHeight <= 24
}

func composeSectionDivider(label string, width int) string {
	if width < 10 {
		width = 10
	}
	title := " " + label + " "
	if ansi.StringWidth(title) >= width {
		return truncateVisual(label, width)
	}
	left := 2
	right := width - left - ansi.StringWidth(title)
	if right < 0 {
		right = 0
	}
	return strings.Repeat("─", left) + title + strings.Repeat("─", right)
}

func (m *Model) renderCompactPreservedResponse(width int) string {
	if width < 20 {
		width = 20
	}
	value := strings.TrimSpace(m.composeBody.Value())
	if value == "" {
		value = "Write your message here (Markdown supported)..."
	}
	value = strings.ReplaceAll(value, "\n", " ")
	line := truncateVisual("Response: "+value, width)
	style := lipgloss.NewStyle().Foreground(m.theme.Chrome.TabInactive.ForegroundColor())
	if m.composeField == composeFieldBody {
		style = style.Foreground(m.theme.Chrome.TabActive.ForegroundColor()).Background(m.theme.Chrome.TabActive.BackgroundColor())
	}
	return style.Render(line)
}

func (m *Model) composeOriginalPreviewRows(tableHeight int) int {
	if m.compactPreservedCompose() || tableHeight <= 22 {
		return 1
	}
	// Match Compose's full-viewport height budget so preserved replies keep the
	// read-only original pane balanced with the editable response pane.
	composeViewportRows := tableHeight + 2
	rows := (composeViewportRows - m.composeFixedRows()) / 2
	if rows < 3 {
		return 3
	}
	return rows
}

func (m *Model) renderComposeOriginalMessagePreview(width int) string {
	ctx := m.composePreserved
	if ctx == nil {
		return ""
	}
	if width < 20 {
		width = 20
	}
	outerRows := m.composeOriginalPreviewRows(m.composeContentHeight())
	if outerRows < 1 {
		outerRows = 1
	}

	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Chrome.TabInactive.ForegroundColor())
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.Severity.Info.ForegroundColor())
	borderColor := m.theme.Focus.PanelBorder.ForegroundColor()
	if m.composeField == composeFieldOriginalMessage {
		dimStyle = dimStyle.Foreground(m.theme.Chrome.TabActive.ForegroundColor())
		labelStyle = labelStyle.Foreground(m.theme.Chrome.TabActive.ForegroundColor()).Background(m.theme.Chrome.TabActive.BackgroundColor())
		borderColor = m.theme.Focus.PanelBorderFocused.ForegroundColor()
	}
	if outerRows == 1 {
		parts := m.composeOriginalMessageCompactParts(width)
		return labelStyle.Render(truncateVisual(strings.Join(parts, "  |  "), width))
	}

	innerRows := outerRows - 2
	if innerRows < 1 {
		innerRows = 1
	}
	rows := m.composeOriginalMessageRows(width, innerRows)
	maxOffset := len(rows) - innerRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	ctx.originalScrollOffset = clampInt(ctx.originalScrollOffset, 0, maxOffset)
	if m.previewSelection.activeOn(previewSelectionComposeOriginal) {
		m.previewSelection.ensureCursorVisible(&ctx.originalScrollOffset, innerRows, len(rows))
		content := renderPlainRowsWithSelection(m.theme, rows, ctx.originalScrollOffset, innerRows, m.previewSelection, previewSelectionComposeOriginal)
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Width(width).
			Height(innerRows).
			Render(content)
	}
	start := ctx.originalScrollOffset
	end := start + innerRows
	if end > len(rows) {
		end = len(rows)
	}
	visible := rows[start:end]
	for i, row := range visible {
		style := dimStyle
		if start+i == 0 {
			style = labelStyle
		}
		visible[i] = style.Render(truncateVisual(row, width))
	}
	content := fitPanelContentHeight(strings.Join(visible, "\n"), innerRows)
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Width(width).
		Height(innerRows).
		Render(content)
}

func (m *Model) composeOriginalMessageCompactParts(width int) []string {
	ctx := m.composePreserved
	parts := []string{"Original message"}
	sender, subject, _ := composeOriginalMessageHeader(ctx)
	bodyLines := composeOriginalMessageBodyLines(ctx, width)
	maxOffset := len(bodyLines) - 1
	if maxOffset < 0 {
		maxOffset = 0
	}
	ctx.originalScrollOffset = clampInt(ctx.originalScrollOffset, 0, maxOffset)
	for _, part := range []string{sender, subject} {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	if len(bodyLines) > 0 {
		parts = append(parts, bodyLines[ctx.originalScrollOffset])
	}
	return parts
}

func (m *Model) composeOriginalMessageRows(width, visibleRows int) []string {
	ctx := m.composePreserved
	sender, subject, date := composeOriginalMessageHeader(ctx)
	if visibleRows <= 4 {
		parts := []string{"Original message"}
		for _, part := range []string{sender, subject, date} {
			if strings.TrimSpace(part) != "" {
				parts = append(parts, strings.TrimSpace(part))
			}
		}
		rows := []string{strings.Join(parts, "  |  ")}
		rows = append(rows, composeOriginalMessageBodyLines(ctx, width)...)
		return rows
	}

	rows := []string{"Original message"}
	if sender != "" {
		rows = append(rows, "From: "+sender)
	}
	if subject != "" {
		rows = append(rows, "Subject: "+subject)
	}
	if date != "" {
		rows = append(rows, "Date: "+date)
	}
	rows = append(rows, composeOriginalMessageBodyLines(ctx, width)...)
	return rows
}

func composeOriginalMessageHeader(ctx *composePreservedContext) (sender, subject, date string) {
	if ctx == nil || ctx.email == nil {
		return "", "", ""
	}
	sender = ctx.email.Sender
	subject = ctx.email.Subject
	if !ctx.email.Date.IsZero() {
		date = ctx.email.Date.Format("Mon, 02 Jan 2006 15:04")
	}
	return sender, subject, date
}

func composeOriginalMessageBodyLines(ctx *composePreservedContext, width int) []string {
	body := ""
	if ctx != nil && ctx.body != nil {
		body = strings.TrimSpace(stripInvisibleChars(ctx.body.TextPlain))
	}
	if body == "" && ctx != nil && ctx.body != nil && strings.TrimSpace(ctx.body.TextHTML) != "" {
		body = "(Original has HTML-only content; Herald will preserve it when sending.)"
	}
	if body == "" {
		body = "(Original message body unavailable.)"
	}

	var lines []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			lines = append(lines, "")
			continue
		}
		wrapped := wrapLines(line, width)
		if len(wrapped) == 0 {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, wrapped...)
	}
	return lines
}

func (m *Model) composeContentHeight() int {
	tableHeight := m.buildLayoutPlan(m.windowWidth, m.windowHeight).ContentHeight
	if tableHeight < 5 {
		tableHeight = 5
	}
	return tableHeight
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
		selectedStyle := m.theme.Focus.SelectionActive.Style()
		normalStyle := m.theme.Severity.Info.Style()
		removedStyle := m.theme.Focus.PanelBorder.Style()
		for i, item := range ctx.forwardedAttachments {
			status := "include"
			action := "x remove"
			style := normalStyle
			if len(item.Attachment.Data) == 0 {
				status = "unavailable"
				action = "not loaded"
				style = removedStyle
			} else if !item.Include {
				status = "removed"
				action = "x include"
				style = removedStyle
			}
			prefix := "  "
			if m.composeField == composeFieldForwardedAttachments && i == ctx.selectedAttachment {
				prefix = "> "
				style = selectedStyle
			}
			label := fmt.Sprintf("%s[%s] %s  (%s)", prefix, status, item.Attachment.Filename, action)
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
	return strings.TrimSpace(m.composeTo.Value()) != "" ||
		strings.TrimSpace(m.composeCC.Value()) != "" ||
		strings.TrimSpace(m.composeBCC.Value()) != "" ||
		strings.TrimSpace(m.composeSubject.Value()) != "" ||
		m.composeBodyHasUserContent() ||
		m.composePreserved != nil
}

func draftFolderIsReplaceable(folder string) bool {
	name := strings.ToLower(strings.TrimSpace(folder))
	if name == "" {
		return false
	}
	return name == "drafts" ||
		name == "[gmail]/drafts" ||
		name == "[google mail]/drafts" ||
		name == "inbox.drafts" ||
		name == "inbox/drafts" ||
		strings.HasSuffix(name, "/drafts") ||
		strings.HasSuffix(name, ".drafts")
}

// saveDraftCmd saves the current compose content as a draft.
// Snapshots the values before the goroutine to prevent data races.
func (m *Model) saveDraftCmd() tea.Cmd {
	b := m.backend
	accountSaver, accountSaverOK := b.(backend.AccountComposeBackend)
	sourceID := m.composeSelectedSourceID()
	from := m.composeFromAddress()
	to := m.composeTo.Value()
	cc := m.composeCC.Value()
	bcc := m.composeBCC.Value()
	subject := m.composeSubject.Value()
	body := m.composeBody.Value()
	attachments := m.composeAttachments
	preserved := m.composePreserved
	replaceUID := uint32(0)
	replaceFolder := ""
	replaceSourceID := models.SourceID("")
	if m.lastDraftReplaceable {
		replaceUID = m.lastDraftUID
		replaceFolder = m.lastDraftFolder
		replaceSourceID = m.lastDraftSourceID
	}
	preservedReq, preservedErr := appsmtp.PreservedMessageRequest{}, error(nil)
	if preserved != nil {
		preservedReq, preservedErr = m.buildPreservedComposeRequest(from, to, subject, attachments)
	}
	return func() tea.Msg {
		if preserved != nil {
			if preservedErr != nil {
				return DraftSavedMsg{SourceID: sourceID, ReplaceUID: replaceUID, ReplaceFolder: replaceFolder, ReplaceSourceID: replaceSourceID, Err: preservedErr}
			}
			raw, err := appsmtp.BuildPreservedMIMEMessage(preservedReq)
			if err != nil {
				return DraftSavedMsg{SourceID: sourceID, ReplaceUID: replaceUID, ReplaceFolder: replaceFolder, ReplaceSourceID: replaceSourceID, Err: err}
			}
			var uid uint32
			var folder string
			if accountSaverOK {
				uid, folder, err = accountSaver.SaveRawDraftForAccount(sourceID, []byte(raw))
			} else {
				uid, folder, err = b.SaveRawDraft([]byte(raw))
			}
			return DraftSavedMsg{UID: uid, Folder: folder, SourceID: sourceID, ReplaceUID: replaceUID, ReplaceFolder: replaceFolder, ReplaceSourceID: replaceSourceID, Err: err}
		}
		var uid uint32
		var folder string
		var err error
		if accountSaverOK {
			uid, folder, err = accountSaver.SaveDraftForAccount(sourceID, to, cc, bcc, subject, body)
		} else {
			uid, folder, err = b.SaveDraft(to, cc, bcc, subject, body)
		}
		return DraftSavedMsg{UID: uid, Folder: folder, SourceID: sourceID, ReplaceUID: replaceUID, ReplaceFolder: replaceFolder, ReplaceSourceID: replaceSourceID, Err: err}
	}
}

// deleteDraftCmd deletes the draft with the given UID from the given folder.
func (m *Model) deleteDraftCmd(uid uint32, folder string) tea.Cmd {
	return m.deleteDraftForSourceCmd(m.lastDraftSourceID, uid, folder)
}

func (m *Model) deleteDraftForSourceCmd(sourceID models.SourceID, uid uint32, folder string) tea.Cmd {
	b := m.backend
	return func() tea.Msg {
		if scoped, ok := b.(backend.AccountComposeBackend); ok {
			return DraftDeletedMsg{Err: scoped.DeleteDraftForAccount(sourceID, uid, folder)}
		}
		err := b.DeleteDraft(uid, folder)
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
	return wordDiffWithTheme(defaultTheme, original, revised)
}

func wordDiffWithTheme(theme Theme, original, revised string) string {
	delStyle := theme.Diff.Delete.Style()
	addStyle := theme.Diff.Add.Style()

	origTokens := tokenizeWords(original)
	revTokens := tokenizeWords(revised)
	common := lcsTokens(origTokens, revTokens)

	var sb strings.Builder
	i, j, k := 0, 0, 0
	for k < len(common) {
		deletedStart := i
		for i < len(origTokens) && origTokens[i] != common[k] {
			i++
		}
		addedStart := j
		for j < len(revTokens) && revTokens[j] != common[k] {
			j++
		}
		writeWordDiffChunk(&sb, delStyle, addStyle, origTokens[deletedStart:i], revTokens[addedStart:j])
		sb.WriteString(common[k])
		i++
		j++
		k++
	}
	writeWordDiffChunk(&sb, delStyle, addStyle, origTokens[i:], revTokens[j:])
	return sb.String()
}

func writeWordDiffChunk(sb *strings.Builder, delStyle, addStyle lipgloss.Style, deleted, added []string) {
	for _, token := range deleted {
		sb.WriteString(delStyle.Render(token))
	}
	if wordDiffNeedsReplacementSeparator(deleted, added) {
		sb.WriteString(" ")
	}
	for _, token := range added {
		sb.WriteString(addStyle.Render(token))
	}
}

func wordDiffNeedsReplacementSeparator(deleted, added []string) bool {
	if len(deleted) == 0 || len(added) == 0 {
		return false
	}
	lastDeleted := deleted[len(deleted)-1]
	firstAdded := added[0]
	return strings.TrimSpace(lastDeleted) != "" && strings.TrimSpace(firstAdded) != ""
}

func parseComposeAIRewriteResponse(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &composeAIRewriteError{Code: "empty_response", Message: "AI returned an empty rewrite"}
	}

	candidate := trimComposeAIRewriteEnvelope(raw)
	if text, err, ok := parseComposeAIRewriteJSONCandidate(candidate); ok {
		return text, err
	}
	if embedded, ok := extractComposeAIRewriteJSONCandidate(candidate); ok {
		if text, err, parsed := parseComposeAIRewriteJSONCandidate(embedded); parsed {
			return text, err
		}
	}

	if looksLikeComposeAIRefusal(raw) {
		return "", &composeAIRewriteError{Code: "safety_refusal", Message: "AI declined this rewrite"}
	}
	return raw, nil
}

func parseComposeAIRewriteJSONCandidate(candidate string) (string, error, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", nil, false
	}
	switch candidate[0] {
	case '{':
		var payload composeAIRewritePayload
		if err := json.Unmarshal([]byte(candidate), &payload); err != nil {
			return "", nil, false
		}
		text, err := composeAIRewriteResultFromPayload(payload)
		return text, err, true
	case '[':
		var payloads []composeAIRewritePayload
		if err := json.Unmarshal([]byte(candidate), &payloads); err != nil || len(payloads) == 0 {
			return "", nil, false
		}
		text, err := composeAIRewriteResultFromPayload(payloads[0])
		return text, err, true
	case '"':
		var inner string
		if err := json.Unmarshal([]byte(candidate), &inner); err != nil {
			return "", nil, false
		}
		text, err := parseComposeAIRewriteResponse(inner)
		return text, err, true
	default:
		return "", nil, false
	}
}

func composeAIRewriteResultFromPayload(payload composeAIRewritePayload) (string, error) {
	status := strings.ToLower(strings.TrimSpace(payload.Status))
	switch status {
	case "", "ok", "success":
		text := strings.TrimSpace(payload.Text)
		if text == "" {
			return "", &composeAIRewriteError{Code: "empty_rewrite", Message: "AI returned an empty rewrite"}
		}
		if looksLikeComposeAIRefusal(text) {
			return "", &composeAIRewriteError{Code: "safety_refusal", Message: "AI declined this rewrite"}
		}
		return text, nil
	case "error", "refusal", "refused":
		code := strings.TrimSpace(payload.ErrorCode)
		if code == "" {
			code = "rewrite_error"
		}
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = "AI declined or could not complete this rewrite"
		}
		return "", &composeAIRewriteError{Code: code, Message: message}
	default:
		return "", &composeAIRewriteError{Code: "rewrite_error", Message: "AI returned an unsupported rewrite status"}
	}
}

func extractComposeAIRewriteJSONCandidate(raw string) (string, bool) {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '{' && raw[i] != '[' {
			continue
		}
		end := matchingJSONEnd(raw, i)
		if end <= i {
			continue
		}
		candidate := raw[i:end]
		if strings.Contains(candidate, `"status"`) && (strings.Contains(candidate, `"text"`) || strings.Contains(candidate, `"error_code"`)) {
			return candidate, true
		}
	}
	return "", false
}

func matchingJSONEnd(raw string, start int) int {
	if start < 0 || start >= len(raw) {
		return -1
	}
	var stack []byte
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != c {
				return -1
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func trimComposeAIRewriteEnvelope(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}
	lines := strings.Split(raw, "\n")
	if len(lines) < 3 {
		return raw
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if strings.HasPrefix(first, "```") && strings.HasPrefix(last, "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return raw
}

func looksLikeComposeAIRefusal(raw string) bool {
	text := strings.ToLower(strings.TrimSpace(raw))
	text = strings.ReplaceAll(text, "’", "'")
	patterns := []string{
		"i'm sorry, but i cannot",
		"i'm sorry, but i can't",
		"i am sorry, but i cannot",
		"i am sorry, but i can't",
		"sorry, but i cannot",
		"sorry, but i can't",
		"i cannot fulfill your request",
		"i can't fulfill your request",
		"i cannot comply with",
		"i can't comply with",
		"i cannot assist with",
		"i can't assist with",
		"i'm unable to assist",
		"i am unable to assist",
		"i'm unable to help",
		"i am unable to help",
		"as an ai, i cannot",
		"as an ai language model, i cannot",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func cleanComposeAISuggestion(raw, original string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if raw == "" {
		return ""
	}
	original = strings.TrimSpace(strings.ReplaceAll(original, "\r\n", "\n"))

	var kept []string
	for _, paragraph := range composeAIParagraphs(raw) {
		if composeAIScaffoldParagraph(paragraph, original) {
			continue
		}
		kept = append(kept, paragraph)
	}

	cleaned := strings.TrimSpace(strings.Join(kept, "\n\n"))
	if cleaned != "" {
		return cleaned
	}
	if original != "" {
		return original
	}
	return raw
}

func composeAIParagraphs(text string) []string {
	var paragraphs []string
	var current []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(strings.Join(current, "\n")))
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.TrimSpace(strings.Join(current, "\n")))
	}
	return paragraphs
}

func composeAIScaffoldParagraph(paragraph, original string) bool {
	trimmed := strings.TrimSpace(paragraph)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "current draft:") || strings.Contains(lower, "\ncurrent draft:") {
		return true
	}
	if composeAIComparableText(trimmed) == composeAIComparableText(original) {
		return true
	}
	if strings.HasPrefix(lower, "demo ai:") {
		for _, marker := range []string{
			"most relevant mailbox context",
			"current draft",
			"while preserving",
			"rewrite",
			"translate this email",
		} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

func composeAIComparableText(text string) string {
	return strings.Join(strings.Fields(ansi.Strip(text)), " ")
}

func composeAIInstructionWithDraftBounds(instruction, draft string) string {
	if !isComposeAITranslationInstruction(instruction) {
		return instruction
	}
	limit := composeAITranslationLengthLimit(draft)
	if limit <= 0 {
		return instruction
	}
	return fmt.Sprintf("%s\n\nOutput bounds: return only the translated body; keep the same number of lines where possible and no longer than %d Unicode characters. Do not add examples, alternatives, explanations, or new content.", instruction, limit)
}

func validateComposeAIRewrite(instruction, draft, rewrite string) error {
	if isComposeAITranslationInstruction(instruction) && composeAIRewriteExceedsTranslationLengthLimit(draft, rewrite) {
		return &composeAIRewriteError{
			Code:    "translation_quality",
			Message: "AI returned a translation that was much longer than the draft",
		}
	}
	if isComposeAIJapaneseTranslationInstruction(instruction) && looksLikeJapaneseKanaNoise(rewrite) {
		return &composeAIRewriteError{
			Code:    "translation_quality",
			Message: "AI returned text that did not look like a natural Japanese translation",
		}
	}
	return nil
}

func isComposeAITranslationInstruction(instruction string) bool {
	return strings.Contains(strings.ToLower(instruction), "translate")
}

func isComposeAIJapaneseTranslationInstruction(instruction string) bool {
	instruction = strings.ToLower(instruction)
	return strings.Contains(instruction, "translate") && strings.Contains(instruction, "japanese")
}

func composeAIRewriteExceedsTranslationLengthLimit(draft, rewrite string) bool {
	limit := composeAITranslationLengthLimit(draft)
	if limit <= 0 {
		return false
	}
	return len([]rune(strings.TrimSpace(rewrite))) > limit
}

func composeAITranslationLengthLimit(draft string) int {
	draftLen := len([]rune(strings.TrimSpace(draft)))
	if draftLen <= 0 {
		return 0
	}
	limit := draftLen*2 + 40
	if limit < 80 {
		return 80
	}
	return limit
}

func looksLikeJapaneseKanaNoise(text string) bool {
	var japanese, hiragana, katakana, kanji int
	longestHiraganaRun := 0
	currentHiraganaRun := 0

	for _, r := range text {
		switch {
		case r >= '\u3040' && r <= '\u309f':
			japanese++
			hiragana++
			currentHiraganaRun++
			if currentHiraganaRun > longestHiraganaRun {
				longestHiraganaRun = currentHiraganaRun
			}
		case r >= '\u30a0' && r <= '\u30ff':
			japanese++
			katakana++
			currentHiraganaRun = 0
		case r >= '\u4e00' && r <= '\u9fff':
			japanese++
			kanji++
			currentHiraganaRun = 0
		default:
			currentHiraganaRun = 0
		}
	}

	if japanese < 40 {
		return false
	}
	kana := hiragana + katakana
	return longestHiraganaRun >= 18 && kanji == 0 && kana*100 >= japanese*90
}

func composeAIStatusForRewriteError(err error) (string, bool) {
	var rewriteErr *composeAIRewriteError
	if !errors.As(err, &rewriteErr) {
		return "", false
	}
	switch rewriteErr.Code {
	case "safety_refusal":
		return "AI warning: rewrite declined by the model; your draft was not changed", true
	case "translation_quality":
		return "AI warning: translation looked invalid; your draft was not changed", true
	default:
		return "AI warning: rewrite failed; your draft was not changed", true
	}
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
		requestInstruction := composeAIInstructionWithDraftBounds(instruction, draft)

		messages := []ai.ChatMessage{
			{
				Role: "system",
				Content: "You are an expert email writing assistant. " +
					"Rewrite the email body according to the user's instruction, including requests to translate, fix typos, adjust tone, change style, shorten, or expand. " +
					"Preserve facts, names, dates, commitments, formatting intent, and any signature unless the user explicitly asks otherwise. " +
					"Treat translation, style, grammar, length, and clarity requests as text transformation tasks. " +
					"For translation requests, produce a natural, idiomatic translation in the target language. " +
					"Do not transliterate source-language sentences, approximate their sounds, invent words, or output random kana or placeholder language. " +
					"For Japanese, use standard modern Japanese with normal kanji/kana where appropriate. " +
					"Preserve names, signatures, separators, and line breaks unless the user explicitly asks otherwise. " +
					"Return JSON only using one of these shapes: {\"status\":\"ok\",\"text\":\"...rewritten body...\"} or {\"status\":\"error\",\"error_code\":\"safety_refusal\",\"message\":\"...short reason...\"}. " +
					"If you decline or cannot complete the request, put the reason only in the error JSON message; never put refusal text in the ok text field. " +
					"Do not return markdown, preamble, or explanations.",
			},
			{
				Role:    "user",
				Content: requestInstruction + "\n\n" + context,
			},
		}
		result, err := classifier.Chat(messages)
		if err != nil {
			return AIAssistMsg{Err: err}
		}
		rewrite, err := parseComposeAIRewriteResponse(result)
		if err != nil {
			return AIAssistMsg{Err: err}
		}
		rewrite = cleanComposeAISuggestion(rewrite, draft)
		if err := validateComposeAIRewrite(instruction, draft, rewrite); err != nil {
			return AIAssistMsg{Err: err}
		}
		return AIAssistMsg{Result: rewrite, Original: draft}
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

func (m *Model) renderComposeAIBar(width int) string {
	if !m.composeAIPanel {
		return ""
	}
	if width < 30 {
		width = 30
	}

	barStyle := m.theme.Compose.AIAction.Style()

	if m.classifier == nil {
		line := "AI disabled  Configure an AI provider in Settings to enable writing tools"
		return barStyle.Width(width).Render(truncateVisual(line, width))
	}

	translateLabel := composeAIToolbarTranslateLabel(m.composeAITranslate)
	styleLabel := composeAIToolbarStyleLabel(m.composeAIStyle)
	undoLabel := "[Undo: ctrl+z]"
	prefixes := []string{
		fmt.Sprintf("%s %s [Fix: ctrl+f] [Shorten: ctrl+n] [Expand: ctrl+e] %s Ask: ctrl+k ", translateLabel, styleLabel, undoLabel),
		fmt.Sprintf("%s %s [Fix: ctrl+f] [Short: ctrl+n] [Exp: ctrl+e] %s Ask: ctrl+k ", translateLabel, styleLabel, undoLabel),
		fmt.Sprintf("%s %s [Fix: ctrl+f] [Short: ctrl+n] [Exp: ctrl+e] %s Ask: ctrl+k ", composeAIShortToolbarTranslateLabel(m.composeAITranslate), composeAIShortToolbarStyleLabel(m.composeAIStyle), undoLabel),
		fmt.Sprintf("[Fix: ctrl+f] [Short: ctrl+n] [Exp: ctrl+e] %s Ask: ctrl+k ", undoLabel),
	}
	prefix := prefixes[len(prefixes)-1]
	for _, candidate := range prefixes {
		if width-ansi.StringWidth(candidate) >= 8 {
			prefix = candidate
			break
		}
	}
	inputWidth := width - ansi.StringWidth(prefix)
	if m.composeAILoading {
		inputWidth -= ansi.StringWidth("  Thinking...")
	}
	if inputWidth < 8 {
		inputWidth = 8
	}
	m.composeAIInput.SetWidth(inputWidth)
	line := prefix + m.composeAIInput.View()
	if m.composeAILoading {
		line += "  Thinking..."
	}

	rows := []string{barStyle.Width(width).Render(truncateVisual(line, width))}
	if dropdown := m.renderComposeAIDropdown(width); dropdown != "" {
		rows = append(rows, dropdown)
	}
	return strings.Join(rows, "\n")
}

func composeAIToolbarTranslateLabel(language string) string {
	if strings.TrimSpace(language) == "" {
		return "[Translate: ctrl+t]"
	}
	return fmt.Sprintf("[Translate: %s v]", composeAISelectedLanguage(language))
}

func composeAIToolbarStyleLabel(style string) string {
	if strings.TrimSpace(style) == "" {
		return "[Style: ctrl+y]"
	}
	return fmt.Sprintf("[Style: %s v]", composeAISelectedStyle(style))
}

func composeAIShortToolbarTranslateLabel(language string) string {
	if strings.TrimSpace(language) == "" {
		return "[T: ctrl+t]"
	}
	return fmt.Sprintf("[T: %s v]", composeAISelectedLanguage(language))
}

func composeAIShortToolbarStyleLabel(style string) string {
	if strings.TrimSpace(style) == "" {
		return "[S: ctrl+y]"
	}
	return fmt.Sprintf("[S: %s v]", composeAISelectedStyle(style))
}

func (m *Model) renderComposeAIDropdown(width int) string {
	if m.composeAIMenu == "" {
		return ""
	}
	var title string
	var options []string
	var selected string
	switch m.composeAIMenu {
	case composeAIMenuTranslate:
		title = "Translate"
		options = composeAITranslateOptions()
		selected = composeAISelectedLanguage(m.composeAITranslate)
	case composeAIMenuStyle:
		title = "Style"
		options = composeAIStyleOptions()
		selected = composeAISelectedStyle(m.composeAIStyle)
	default:
		return ""
	}

	itemStyle := m.theme.Compose.AIToggleInactive.Style()
	selectedStyle := m.theme.Compose.AIToggleActive.Style()
	parts := []string{title + ":"}
	for i, option := range options {
		label := fmt.Sprintf("%d %s", i+1, option)
		if option == selected {
			label = "> " + label
			parts = append(parts, selectedStyle.Render(label))
		} else {
			parts = append(parts, itemStyle.Render(label))
		}
	}
	return truncateVisual(strings.Join(parts, "  "), width)
}

func (m *Model) renderComposeAIReview(width int, activeFieldStyle, inactiveFieldStyle lipgloss.Style) string {
	if width < 30 {
		width = 30
	}
	compact := m.windowHeight <= 24
	contentWidth := width - 2
	if contentWidth < 28 {
		contentWidth = 28
	}
	title := "AI Assist · Suggestion replaces draft"
	trailing := "Original: tab"
	if m.composeAIShowOriginal {
		title = "AI Assist · Original draft"
		trailing = "Suggestion: tab"
	}
	header := composeReviewAlignedLine(
		m.theme.Compose.AILabel.Style().Render(title),
		m.theme.Compose.AILabel.Style().Render(trailing),
		contentWidth,
	)

	sectionTitle := "Suggestion"
	sectionTrailing := "(edit freely)"
	bodyWidth := contentWidth - 4
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	var bodyLines []string
	suggestionHeight := m.composeAIReviewSuggestionHeight()
	if m.composeAIShowOriginal {
		sectionTitle = "Original"
		sectionTrailing = "(read only)"
		bodyLines = strings.Split(m.renderComposeAIOriginalView(bodyWidth, suggestionHeight), "\n")
	} else {
		oldHeight := m.composeAIResponse.Height()
		m.composeAIResponse.SetWidth(bodyWidth)
		m.composeAIResponse.SetHeight(suggestionHeight)
		if compact {
			m.composeAIResponse.MoveToBegin()
		}
		bodyLines = strings.Split(m.composeAIResponse.View(), "\n")
		m.composeAIResponse.SetHeight(oldHeight)
	}
	suggestion := m.renderComposeAISection(sectionTitle, sectionTrailing, bodyLines, contentWidth)
	extraChangeRows := m.composeAIReviewFillerRows(suggestionHeight)
	changes := m.renderComposeAIChanges(contentWidth, compact, extraChangeRows)
	actions := m.renderComposeAIActionRow(contentWidth)

	parts := []string{
		header,
		suggestion,
		changes,
		actions,
	}
	if !compact {
		rule := m.theme.Compose.AIBorder.Style().Render(strings.Repeat("─", contentWidth))
		parts = []string{header, rule, suggestion, "", changes, actions}
	}
	content := strings.Join(parts, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Compose.AIBorder.ForegroundColor()).
		Width(width).
		Render(content)
}

func (m *Model) composeAIReviewFillerRows(suggestionHeight int) int {
	if m.windowHeight <= 24 {
		return 0
	}
	filler := m.composeAIResponse.Height() - suggestionHeight
	if filler < 0 {
		return 0
	}
	return filler
}

func (m *Model) composeAIReviewSuggestionHeight() int {
	height := m.composeAIResponse.Height()
	if height < 1 {
		height = 1
	}
	switch {
	case m.windowHeight <= 24:
		return 1
	case m.windowHeight <= 40 && height > 8:
		return 8
	case height > 12:
		return 12
	default:
		return height
	}
}

func (m *Model) renderComposeAIOriginalView(width, height int) string {
	if height < 1 {
		height = 1
	}
	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}
	text := m.composeAIOriginal
	if text == "" {
		text = m.composeBody.Value()
	}
	lines := wrapLines(text, innerW)
	if len(lines) == 0 {
		lines = []string{""}
	}
	rows := make([]string, 0, height)
	lineNo := 1
	for len(rows) < height {
		line := ""
		if len(rows) < len(lines) {
			line = lines[len(rows)]
		}
		rows = append(rows, fmt.Sprintf("┃ %3d %s", lineNo, truncateVisual(line, innerW)))
		lineNo++
	}
	return strings.Join(rows, "\n")
}

func composeReviewAlignedLine(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return truncateVisual(line, width)
}

func composeReviewPadLine(line string, width int) string {
	line = truncateVisual(line, width)
	if missing := width - ansi.StringWidth(line); missing > 0 {
		line += strings.Repeat(" ", missing)
	}
	return line
}

func (m *Model) renderComposeAISection(title, trailing string, bodyLines []string, width int) string {
	innerWidth := width - 2
	if innerWidth < 20 {
		innerWidth = 20
	}
	titleLabel := " " + title + " "
	rightLabel := ""
	if trailing != "" {
		rightLabel = " " + trailing + " "
	}
	topInnerWidth := innerWidth - ansi.StringWidth(titleLabel) - ansi.StringWidth(rightLabel)
	if topInnerWidth < 0 {
		topInnerWidth = 0
	}
	leftRule := topInnerWidth
	if rightLabel != "" {
		leftRule = topInnerWidth / 2
	}
	rightRule := topInnerWidth - leftRule
	borderStyle := m.theme.Compose.AIBorder.Style()
	labelStyle := m.theme.Compose.AILabel.Style()
	dimStyle := m.theme.Compose.AIDim.Style()
	top := borderStyle.Render("┌") +
		borderStyle.Render(strings.Repeat("─", leftRule)) +
		labelStyle.Render(titleLabel) +
		borderStyle.Render(strings.Repeat("─", rightRule)) +
		dimStyle.Render(rightLabel) +
		borderStyle.Render("┐")

	rows := []string{top}
	for _, line := range bodyLines {
		rows = append(rows,
			borderStyle.Render("│")+
				composeReviewPadLine(line, innerWidth)+
				borderStyle.Render("│"),
		)
	}
	rows = append(rows, borderStyle.Render("└"+strings.Repeat("─", innerWidth)+"┘"))
	return strings.Join(rows, "\n")
}

func (m *Model) composeAIChangeRows(width int) []string {
	original := strings.TrimSpace(m.composeAIOriginal)
	if original == "" {
		original = strings.TrimSpace(m.composeBody.Value())
	}
	suggestion := strings.TrimSpace(m.composeAIResponse.Value())
	if original == "" && suggestion == "" {
		return nil
	}
	if composeAIComparableText(original) == composeAIComparableText(suggestion) {
		return nil
	}

	diff := m.composeAIDiff
	if strings.TrimSpace(ansi.Strip(diff)) == "" {
		diff = wordDiffWithTheme(m.theme, original, suggestion)
	}
	wrapped := wrapLines(diff, width)
	rows := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		if strings.TrimSpace(ansi.Strip(line)) == "" {
			rows = append(rows, "")
			continue
		}
		rows = append(rows, truncateVisual(line, width))
	}
	return rows
}

func (m *Model) renderComposeAIActionRow(width int) string {
	segments := []string{
		m.theme.Compose.AILabel.Style().Render("Accept ctrl+enter"),
		m.theme.Compose.AIDim.Style().Render("Discard esc"),
		m.theme.Compose.AIDim.Style().Render("Edit in place"),
		m.theme.Compose.AIDim.Style().Render("Undo ctrl+z"),
		m.theme.Compose.AIDim.Style().Render("Tab original/suggestion"),
	}
	return truncateVisual(strings.Join(segments, "  │  "), width)
}

func (m *Model) renderComposeAIChanges(width int, compact bool, extraRows int) string {
	if width < 30 {
		width = 30
	}
	bodyWidth := width - 4
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	if compact {
		return m.renderComposeAISection("Changes", "word diff", []string{m.composeAICompactChangeLine(bodyWidth)}, width)
	}
	rows := m.composeAIChangeRows(bodyWidth)
	if len(rows) == 0 {
		rows = []string{m.theme.Compose.AIDim.Style().Render("(no word-level changes)")}
	}
	rowBudget := 4 + extraRows
	if rowBudget < 1 {
		rowBudget = 1
	}
	for len(rows) < rowBudget {
		rows = append(rows, "")
	}
	if len(rows) > rowBudget {
		rows = rows[:rowBudget]
	}
	return m.renderComposeAISection("Changes", "word diff", rows, width)
}

func (m *Model) composeAICompactChangeLine(width int) string {
	rows := m.composeAIChangeRows(width)
	if len(rows) == 0 {
		return m.theme.Compose.AIDim.Style().Render("(no word-level changes)")
	}
	for _, row := range rows {
		if strings.TrimSpace(ansi.Strip(row)) != "" {
			return truncateVisual(row, width)
		}
	}
	return m.theme.Compose.AIDim.Style().Render("(no word-level changes)")
}

func (m *Model) renderComposeAIResult(width int) string {
	if !m.composeAIPanel {
		return ""
	}
	if m.composeAIReviewActive() {
		return ""
	}
	if !m.composeAILoading {
		return ""
	}
	if width < 30 {
		width = 30
	}
	spinnerStyle := m.theme.Compose.Accent.Style()
	return spinnerStyle.Render(truncateVisual("Thinking...", width))
}
