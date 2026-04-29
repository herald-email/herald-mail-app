package app

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/herald-email/herald-mail-app/internal/ai"
)

const maxToolRounds = 5

// submitChat sends the current chat input to the AI backend with email context,
// using a multi-turn tool-calling loop when supported.
func (m *Model) submitChat() tea.Cmd {
	question := strings.TrimSpace(m.chatInput.Value())
	if question == "" {
		return nil
	}
	m.chatInput.SetValue("")
	m.chatWaiting = true

	currentFolder := m.currentFolder // snapshot before goroutine

	// Append user message to history
	m.chatMessages = append(m.chatMessages, ai.ChatMessage{
		Role:    "user",
		Content: question,
	})
	m.chatWrappedLines = nil // invalidate wrap cache

	// Build system prompt with email context
	var ctx strings.Builder
	ctx.WriteString(fmt.Sprintf("You are an email assistant. The user is viewing folder: %s.\n", currentFolder))
	if st, ok := m.folderStatus[currentFolder]; ok {
		ctx.WriteString(fmt.Sprintf("Folder has %d total emails, %d unread.\n", st.Total, st.Unseen))
	}
	if len(m.timeline.emails) > 0 {
		ctx.WriteString("Recent emails (newest first):\n")
		limit := 20
		if len(m.timeline.emails) < limit {
			limit = len(m.timeline.emails)
		}
		for _, e := range m.timeline.emails[:limit] {
			ctx.WriteString(fmt.Sprintf("  - [%s] From: %s | Subject: %s | Date: %s\n",
				e.MessageID, e.Sender, e.Subject, e.Date.Format("2006-01-02")))
		}
	}
	ctx.WriteString("\nIf the user asks to show, filter, or find specific emails, include a <filter> block at the end of your response:\n")
	ctx.WriteString("<filter>{\"ids\": [\"<message-id-1>\", \"<message-id-2>\"], \"label\": \"short description\"}</filter>\n")
	ctx.WriteString("Only include a <filter> block when the user is explicitly asking to filter or navigate to specific emails.\n")

	systemMsg := ai.ChatMessage{Role: "system", Content: ctx.String()}
	messages := append([]ai.ChatMessage{systemMsg}, m.chatMessages...)

	classifier := m.classifier
	tools, dispatch := m.chatToolRegistryWithFolder(currentFolder)

	return func() tea.Msg {
		if classifier == nil {
			return ChatResponseMsg{Err: fmt.Errorf("AI not configured")}
		}

		for round := 0; round < maxToolRounds; round++ {
			response, calls, err := classifier.ChatWithTools(messages, tools)
			if err != nil {
				if errors.Is(err, ai.ErrToolsNotSupported) {
					// Fall back to plain Chat()
					reply, err2 := classifier.Chat(messages)
					if err2 != nil {
						return ChatResponseMsg{Err: err2}
					}
					return ChatResponseMsg{Content: reply}
				}
				return ChatResponseMsg{Err: err}
			}

			if len(calls) == 0 {
				// Final text response
				return ChatResponseMsg{Content: response}
			}

			// Append assistant turn with all tool calls (once per round)
			messages = append(messages, ai.ChatMessage{
				Role:      "assistant",
				ToolCalls: calls,
			})

			// Execute tool calls and append one result message per call
			for _, call := range calls {
				result, dispErr := dispatch(call.Name, call.Arguments)
				if dispErr != nil {
					result = "Error: " + dispErr.Error()
				}
				messages = append(messages, ai.ChatMessage{
					Role:       "tool",
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Content:    result,
				})
			}
		}

		return ChatResponseMsg{Content: "Max tool rounds reached without a final response."}
	}
}

// renderChatPanel renders the chat panel content (without border)
func (m *Model) renderChatPanel() string {
	w := chatPanelWidth
	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(defaultTheme.InfoFg).
		Bold(true).
		Width(w)
	sb.WriteString(titleStyle.Render("Chat") + "\n")
	sb.WriteString(strings.Repeat("─", w) + "\n")

	// Message history — show last messages that fit in height
	msgStyle := lipgloss.NewStyle().Width(w)
	userStyle := lipgloss.NewStyle().Foreground(defaultTheme.TextFg).Width(w)
	aiStyle := lipgloss.NewStyle().Foreground(defaultTheme.InfoFg).Width(w)

	// Calculate how many lines we have for history
	// Total height = tableHeight; minus title(1) + divider(1) + divider2(1) + input(1) = 4
	historyLines := m.windowHeight - 7 - 4 // same tableHeight formula minus chat chrome
	if historyLines < 3 {
		historyLines = 3
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
		waitStyle := lipgloss.NewStyle().Foreground(defaultTheme.DimFg)
		sb.WriteString(waitStyle.Render("Thinking..."))
	} else {
		sb.WriteString(m.chatInput.View())
	}

	return sb.String()
}

// wrapLines splits text on newlines first, then word-wraps each paragraph to
// fit within width runes. Consecutive blank lines are collapsed to one blank
// line, so over-spaced HTML-converted bodies look reasonable.
// expandTilde replaces a leading "~/" with the user's home directory.
