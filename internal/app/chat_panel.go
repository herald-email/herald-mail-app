package app

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/agent"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/logger"
)

const (
	chatAgentMaxVisibleIDs         = 50
	chatAgentMaxSelectedIDs        = 50
	chatAgentMaxHistoryTurns       = 20
	chatAgentMaxUserMessageChars   = 4000
	chatAgentMaxHistoryChars       = 2000
	chatAgentMaxComposeHeaderChars = 1000
	chatAgentMaxComposeBodyChars   = 8000
)

var errChatAgentUnavailable = errors.New("Chat agent unavailable: AI provider is disabled or misconfigured")

// submitChat sends the current chat input to the configured Gollem agent runner.
func (m *Model) submitChat() tea.Cmd {
	question := strings.TrimSpace(m.chatInput.Value())
	if question == "" {
		return nil
	}
	if isChatResetCommand(question) {
		m.chatInput.SetValue("")
		m.resetChatConversation()
		return nil
	}
	m.chatInput.SetValue("")
	m.chatWaiting = true
	startedAt := time.Now()
	m.chatStartedAt = startedAt
	m.chatScrollOffset = 0

	currentFolder := m.currentFolder // snapshot before goroutine
	previousMessages := append([]ai.ChatMessage(nil), m.chatMessages...)
	generation := m.chatGeneration

	// Append user message to history
	m.ensureChatMessageTiming()
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{
		Role:    "user",
		Content: question,
	})
	m.chatMessageTimes = append(m.chatMessageTimes, chatMessageTiming{})
	m.chatWrappedLines = nil // invalidate wrap cache

	if runner := m.chatAgent; runner != nil {
		input := m.buildChatAgentInput(question, currentFolder, previousMessages)
		logger.Debug("Chat submit: folder=%s tab=%s question_chars=%d history_turns=%d visible_ids=%d selected_ids=%d compose_snapshot=%t", currentFolder, input.ActiveTab, len([]rune(input.UserMessage)), len(input.History), len(input.VisibleIDs), len(input.SelectedIDs), input.ComposeSnapshot != nil)
		return func() tea.Msg {
			started := time.Now()
			result, err := runner.Run(context.Background(), input)
			logger.Debug("Chat response received: duration=%s error=%t reply_chars=%d timeline_intent=%t summary=%t compose_intent=%t", time.Since(started).Round(time.Millisecond), err != nil, len([]rune(result.Reply)), result.Timeline != nil, result.Summary != nil, result.Compose != nil)
			return ChatAgentResponseMsg{Result: result, Err: err, Generation: generation, StartedAt: startedAt, Elapsed: time.Since(startedAt)}
		}
	}

	return func() tea.Msg {
		return ChatAgentResponseMsg{Err: errChatAgentUnavailable, Generation: generation, StartedAt: startedAt, Elapsed: time.Since(startedAt)}
	}
}

func isChatResetCommand(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "/clear", "/clean":
		return true
	default:
		return false
	}
}

func (m *Model) resetChatConversation() {
	m.chatGeneration++
	m.chatWaiting = false
	m.chatMessages = nil
	m.chatMessageTimes = nil
	m.chatWrappedLines = nil
	m.chatWrappedWidth = 0
	m.chatScrollOffset = 0
	m.chatStartedAt = time.Time{}
}

func (m *Model) buildChatAgentInput(question, currentFolder string, history []ai.ChatMessage) agent.ChatInput {
	return agent.ChatInput{
		UserMessage:     chatAgentBoundedText(question, chatAgentMaxUserMessageChars),
		CurrentFolder:   currentFolder,
		ActiveTab:       chatAgentTabName(m.activeTab),
		VisibleIDs:      m.chatVisibleMessageIDs(chatAgentMaxVisibleIDs),
		SelectedIDs:     m.chatSelectedMessageIDs(chatAgentMaxSelectedIDs),
		ComposeSnapshot: m.chatComposeSnapshot(),
		History:         chatAgentHistory(history, chatAgentMaxHistoryTurns),
	}
}

func chatAgentTabName(tab int) string {
	switch tab {
	case tabTimeline:
		return "timeline"
	case tabCompose:
		return "compose"
	case tabContacts:
		return "contacts"
	case tabCalendar:
		return "calendar"
	case tabMemories:
		return "memories"
	default:
		return ""
	}
}

func (m *Model) chatVisibleMessageIDs(limit int) []string {
	if limit <= 0 {
		return nil
	}
	displayEmails := m.timelineDisplayEmails()
	ids := make([]string, 0, min(len(displayEmails), limit))
	for _, email := range displayEmails {
		if email == nil || strings.TrimSpace(email.MessageID) == "" {
			continue
		}
		ids = append(ids, email.MessageID)
		if len(ids) >= limit {
			break
		}
	}
	return ids
}

func (m *Model) chatSelectedMessageIDs(limit int) []string {
	if limit <= 0 || len(m.timeline.selectedMessageIDs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(m.timeline.selectedMessageIDs))
	for id, selected := range m.timeline.selectedMessageIDs {
		if selected && strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

func (m *Model) chatComposeSnapshot() *agent.ComposeSnapshot {
	if m.activeTab != tabCompose {
		return nil
	}
	return &agent.ComposeSnapshot{
		To:      chatAgentBoundedText(m.composeTo.Value(), chatAgentMaxComposeHeaderChars),
		CC:      chatAgentBoundedText(m.composeCC.Value(), chatAgentMaxComposeHeaderChars),
		BCC:     chatAgentBoundedText(m.composeBCC.Value(), chatAgentMaxComposeHeaderChars),
		Subject: chatAgentBoundedText(m.composeSubject.Value(), chatAgentMaxComposeHeaderChars),
		Body:    chatAgentBoundedText(m.composeBody.Value(), chatAgentMaxComposeBodyChars),
	}
}

func chatAgentHistory(messages []ai.ChatMessage, limit int) []agent.ChatTurn {
	if limit <= 0 || len(messages) == 0 {
		return nil
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	turns := make([]agent.ChatTurn, 0, len(messages))
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		turns = append(turns, agent.ChatTurn{
			Role:    msg.Role,
			Content: chatAgentBoundedText(content, chatAgentMaxHistoryChars),
		})
	}
	return turns
}

func chatAgentBoundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "...[truncated]"
}

func (m *Model) ensureChatMessageTiming() {
	for len(m.chatMessageTimes) < len(m.chatMessages) {
		m.chatMessageTimes = append(m.chatMessageTimes, chatMessageTiming{})
	}
	if len(m.chatMessageTimes) > len(m.chatMessages) {
		m.chatMessageTimes = m.chatMessageTimes[:len(m.chatMessages)]
	}
}

func chatElapsedTickCmd(startedAt time.Time) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return ChatElapsedTickMsg{StartedAt: startedAt}
	})
}

func chatElapsedLabel(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	return fmtDurationSeconds(elapsed)
}

func fmtDurationSeconds(elapsed time.Duration) string {
	return strconv.FormatInt(int64(elapsed/time.Second), 10) + "s"
}

func (m *Model) chatMessagePrefix(index int, msg ai.ChatMessage) string {
	if msg.Role == "user" {
		return "You: "
	}
	if msg.Role == "assistant" && index >= 0 && index < len(m.chatMessageTimes) && m.chatMessageTimes[index].Set {
		return "AI (" + chatElapsedLabel(m.chatMessageTimes[index].Elapsed) + "): "
	}
	return "AI: "
}

func (m *Model) chatWaitingLabel() string {
	elapsed := time.Duration(0)
	if !m.chatStartedAt.IsZero() {
		elapsed = time.Since(m.chatStartedAt)
	}
	return "Thinking... " + chatElapsedLabel(elapsed)
}

// renderChatPanel renders the chat panel content (without border)
func (m *Model) renderChatPanel() string {
	w := m.effectiveChatPanelWidth(m.windowWidth)
	contentHeight := m.chatPanelContentHeight()
	rows := make([]string, 0, contentHeight)

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Severity.Info.ForegroundColor()).
		Bold(true)
	rows = append(rows, chatPanelFitRow(titleStyle.Render("Chat"), w))
	rows = append(rows, strings.Repeat("─", w))

	// Message history — show last messages that fit in height
	userStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
	aiStyle := lipgloss.NewStyle().Foreground(m.theme.Severity.Info.ForegroundColor())

	historyLines := chatHistoryLineCapacity(contentHeight)

	// Rebuild wrap cache if stale
	m.ensureChatWrappedLines(w)

	// Collect rendered message lines (newest-last)
	var msgLines []string
	for i, msg := range m.chatMessages {
		style := aiStyle
		if msg.Role == "user" {
			style = userStyle
		}
		for _, line := range m.chatWrappedLines[i] {
			for _, visualLine := range strings.Split(style.Render(line), "\n") {
				msgLines = append(msgLines, chatPanelFitRow(visualLine, w))
			}
		}
		msgLines = append(msgLines, "")
	}

	maxScroll := chatMaxScrollOffset(len(msgLines), historyLines)
	m.chatScrollOffset = clampInt(m.chatScrollOffset, 0, maxScroll)
	start := len(msgLines) - historyLines - m.chatScrollOffset
	if start < 0 {
		start = 0
	}
	end := start + historyLines
	if end > len(msgLines) {
		end = len(msgLines)
	}
	msgLines = msgLines[start:end]

	// Pad to fill space
	for len(msgLines) < historyLines {
		msgLines = append([]string{chatPanelFitRow("", w)}, msgLines...)
	}
	rows = append(rows, msgLines...)
	rows = append(rows, strings.Repeat("─", w))

	// Input field
	if m.chatWaiting {
		waitStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
		rows = append(rows, chatPanelFitRow(waitStyle.Render(m.chatWaitingLabel()), w))
	} else {
		inputWidth := w - lipgloss.Width(m.chatInput.Prompt)
		if inputWidth < 1 {
			inputWidth = 1
		}
		m.chatInput.SetWidth(inputWidth)
		rows = append(rows, chatPanelFitRow(m.chatInput.View(), w))
	}

	return strings.Join(chatPanelFitRows(rows, contentHeight, w), "\n")
}

func chatPanelFitRows(rows []string, target, width int) []string {
	if len(rows) > target {
		rows = rows[:target]
	}
	for len(rows) < target {
		rows = append(rows, chatPanelFitRow("", width))
	}
	return rows
}

func chatPanelFitRow(row string, width int) string {
	if width <= 0 {
		return ""
	}
	row = strings.ReplaceAll(row, "\n", " ")
	return lipgloss.NewStyle().Width(width).Render(ansi.Truncate(row, width, ""))
}

func (m *Model) chatPanelContentHeight() int {
	contentHeight := m.buildLayoutPlan(m.windowWidth, m.windowHeight).ContentHeight
	if contentHeight < 5 {
		contentHeight = 5
	}
	return contentHeight
}

func chatHistoryLineCapacity(contentHeight int) int {
	// Title, top divider, bottom divider, and input/thinking row occupy four rows.
	if lines := contentHeight - 4; lines >= 1 {
		return lines
	}
	return 1
}

func (m *Model) ensureChatWrappedLines(width int) {
	if m.chatWrappedLines != nil && m.chatWrappedWidth == width && len(m.chatWrappedLines) == len(m.chatMessages) {
		return
	}
	m.ensureChatMessageTiming()
	m.chatWrappedLines = make([][]string, len(m.chatMessages))
	for i, msg := range m.chatMessages {
		prefix := m.chatMessagePrefix(i, msg)
		m.chatWrappedLines[i] = wrapText(prefix+msg.Content, width)
	}
	m.chatWrappedWidth = width
}

func chatMaxScrollOffset(totalLines, visibleLines int) int {
	if totalLines <= visibleLines {
		return 0
	}
	return totalLines - visibleLines
}

func (m *Model) chatHistoryLineCount(width int) int {
	m.ensureChatWrappedLines(width)
	total := 0
	for _, lines := range m.chatWrappedLines {
		total += len(lines) + 1
	}
	return total
}

func (m *Model) chatHistoryPageStep() int {
	if step := chatHistoryLineCapacity(m.chatPanelContentHeight()) - 1; step >= 1 {
		return step
	}
	return 1
}

func (m *Model) scrollChatHistory(delta int) {
	if delta == 0 {
		return
	}
	w := m.effectiveChatPanelWidth(m.windowWidth)
	visibleLines := chatHistoryLineCapacity(m.chatPanelContentHeight())
	maxScroll := chatMaxScrollOffset(m.chatHistoryLineCount(w), visibleLines)
	m.chatScrollOffset = clampInt(m.chatScrollOffset+delta, 0, maxScroll)
}

func (m *Model) jumpChatHistory(top bool) {
	if !top {
		m.chatScrollOffset = 0
		return
	}
	w := m.effectiveChatPanelWidth(m.windowWidth)
	visibleLines := chatHistoryLineCapacity(m.chatPanelContentHeight())
	m.chatScrollOffset = chatMaxScrollOffset(m.chatHistoryLineCount(w), visibleLines)
}

// wrapLines splits text on newlines first, then word-wraps each paragraph to
// fit within width runes. Consecutive blank lines are collapsed to one blank
// line, so over-spaced HTML-converted bodies look reasonable.
// expandTilde replaces a leading "~/" with the user's home directory.
