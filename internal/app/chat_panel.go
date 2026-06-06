package app

import (
	"context"
	"errors"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/agent"
	"github.com/herald-email/herald-mail-app/internal/ai"
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
	m.chatInput.SetValue("")
	m.chatWaiting = true

	currentFolder := m.currentFolder // snapshot before goroutine
	previousMessages := append([]ai.ChatMessage(nil), m.chatMessages...)

	// Append user message to history
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{
		Role:    "user",
		Content: question,
	})
	m.chatWrappedLines = nil // invalidate wrap cache

	if runner := m.chatAgent; runner != nil {
		input := m.buildChatAgentInput(question, currentFolder, previousMessages)
		return func() tea.Msg {
			result, err := runner.Run(context.Background(), input)
			return ChatAgentResponseMsg{Result: result, Err: err}
		}
	}

	return func() tea.Msg {
		return ChatAgentResponseMsg{Err: errChatAgentUnavailable}
	}
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

// renderChatPanel renders the chat panel content (without border)
func (m *Model) renderChatPanel() string {
	w := m.effectiveChatPanelWidth(m.windowWidth)
	contentHeight := m.buildLayoutPlan(m.windowWidth, m.windowHeight).ContentHeight
	if contentHeight < 5 {
		contentHeight = 5
	}
	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Severity.Info.ForegroundColor()).
		Bold(true).
		Width(w)
	sb.WriteString(titleStyle.Render("Chat") + "\n")
	sb.WriteString(strings.Repeat("─", w) + "\n")

	// Message history — show last messages that fit in height
	msgStyle := lipgloss.NewStyle().Width(w)
	userStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor()).Width(w)
	aiStyle := lipgloss.NewStyle().Foreground(m.theme.Severity.Info.ForegroundColor()).Width(w)

	// Calculate history from the same inner panel height as the main layout:
	// title + top divider + bottom divider + input occupy four rows.
	historyLines := contentHeight - 4
	if historyLines < 1 {
		historyLines = 1
	}

	// Rebuild wrap cache if stale
	if m.chatWrappedLines == nil || m.chatWrappedWidth != w {
		m.chatWrappedLines = make([][]string, len(m.chatMessages))
		for i, msg := range m.chatMessages {
			prefix := "AI: "
			if msg.Role == "user" {
				prefix = "You: "
			}
			m.chatWrappedLines[i] = wrapText(prefix+msg.Content, w)
		}
		m.chatWrappedWidth = w
	}

	// Collect rendered message lines (newest-last)
	var msgLines []string
	for i, msg := range m.chatMessages {
		style := aiStyle
		if msg.Role == "user" {
			style = userStyle
		}
		for _, line := range m.chatWrappedLines[i] {
			msgLines = append(msgLines, style.Render(line))
		}
		msgLines = append(msgLines, "")
	}
	// Show only the last historyLines
	if len(msgLines) > historyLines {
		msgLines = msgLines[len(msgLines)-historyLines:]
	}
	// Pad to fill space
	for len(msgLines) < historyLines {
		msgLines = append([]string{msgStyle.Render("")}, msgLines...)
	}
	for _, line := range msgLines {
		sb.WriteString(line + "\n")
	}

	sb.WriteString(strings.Repeat("─", w) + "\n")

	// Input field
	if m.chatWaiting {
		waitStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor()).Width(w)
		sb.WriteString(waitStyle.Render("Thinking..."))
	} else {
		inputWidth := w - lipgloss.Width(m.chatInput.Prompt)
		if inputWidth < 1 {
			inputWidth = 1
		}
		m.chatInput.SetWidth(inputWidth)
		sb.WriteString(m.chatInput.View())
	}

	return fitPanelContentHeight(sb.String(), contentHeight)
}

// wrapLines splits text on newlines first, then word-wraps each paragraph to
// fit within width runes. Consecutive blank lines are collapsed to one blank
// line, so over-spaced HTML-converted bodies look reasonable.
// expandTilde replaces a leading "~/" with the user's home directory.
