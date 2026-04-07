package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/ai"
	"mail-processor/internal/backend"
	"mail-processor/internal/iterm2"
	"mail-processor/internal/models"
)

func (m *Model) renderEmailPreview() string {
	w := m.emailPreviewWidth
	if w <= 0 {
		w = 40
	}
	innerW := w - 4 // left border + padding

	var sb strings.Builder

	// Focus-aware colors: brighter when preview panel has focus
	borderColor := "238"
	headerColor := "245"
	if m.focusedPanel == panelPreview {
		borderColor = "39"
		headerColor = "255"
	}

	// Header block
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(headerColor))
	email := m.selectedTimelineEmail
	sb.WriteString(headerStyle.Render(truncate("From: "+email.Sender, innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Subj: "+email.Subject, innerW)) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	panelHeight := m.windowHeight - 7
	if panelHeight < 5 {
		panelHeight = 5
	}
	// Header block is 4 rows (From + Date + Subj + separator).
	// Reserve 1 row for scroll indicator → maxBodyLines = panelHeight - 4 - 1.
	maxBodyLines := panelHeight - 5
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}

	// Reserve rows for the quick reply picker overlay when open.
	pickerLines := 0
	if m.quickReplyOpen {
		pickerLines = 2 + len(m.quickReplies) // divider + header + items
		if len(m.quickReplies) == 0 {
			pickerLines = 3
		}
		if pickerLines > 12 {
			pickerLines = 12
		}
		maxBodyLines -= pickerLines
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}
	}

	dimStyle := headerStyle
	if m.emailBodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.emailBody != nil {
		// Show inline image descriptors (raw escape sequences corrupt the TUI renderer)
		imageLines := 0
		for _, img := range m.emailBody.InlineImages {
			var label string
			if iterm2.IsSupported() {
				// Render image inline via iTerm2 protocol
				rendered := iterm2.Render(img.Data, innerW)
				if rendered != "" {
					sb.WriteString(rendered)
					imageLines++
					continue
				}
			}
			if desc, ok := m.inlineImageDescs[img.ContentID]; ok {
				label = fmt.Sprintf("[Image: %s]", desc)
			} else if m.classifier != nil && m.classifier.HasVisionModel() {
				label = fmt.Sprintf("[image  %s  %d KB  — describing…]", img.MIMEType, len(img.Data)/1024)
			} else {
				label = fmt.Sprintf("[image: %s]", img.MIMEType)
			}
			label = truncate(label, innerW)
			sb.WriteString(dimStyle.Render(label) + "\n")
			imageLines++
		}

		// Show downloadable attachments
		attachStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
		selectedAttachStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
		for i, att := range m.emailBody.Attachments {
			sizeStr := fmt.Sprintf("%.1f KB", float64(att.Size)/1024)
			if att.Size >= 1024*1024 {
				sizeStr = fmt.Sprintf("%.1f MB", float64(att.Size)/(1024*1024))
			}
			label := fmt.Sprintf("[attach] %s  %s  %s", att.Filename, att.MIMEType, sizeStr)
			label = truncate(label, innerW)
			if i == m.selectedAttachment {
				sb.WriteString(selectedAttachStyle.Render(label) + "\n")
			} else {
				sb.WriteString(attachStyle.Render(label) + "\n")
			}
			imageLines++
		}

		// Save-path prompt
		if m.attachmentSavePrompt {
			promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
			sb.WriteString(promptStyle.Render("Save to: ") + m.attachmentSaveInput.View() + "\n")
			imageLines++
		}

		// Body — wrap/render once and cache; re-render only if panel width changed
		body := stripInvisibleChars(m.emailBody.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		if m.bodyWrappedLines == nil || m.bodyWrappedWidth != innerW {
			if m.emailBody.IsFromHTML {
				// Render markdown (converted from HTML) via glamour at panel width.
				// Don't linkify before glamour — OSC 8 escape sequences break
				// glamour's width calculation and cause lines to overflow the panel.
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						m.bodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
					}
				} else {
					m.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
				}
			} else {
				m.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
			}
			m.bodyWrappedWidth = innerW
		}

		// Clamp scroll offset
		visibleLines := maxBodyLines - imageLines
		if visibleLines < 1 {
			visibleLines = 1
		}
		totalLines := len(m.bodyWrappedLines)
		maxOffset := totalLines - visibleLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.bodyScrollOffset > maxOffset {
			m.bodyScrollOffset = maxOffset
		}

		end := m.bodyScrollOffset + visibleLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(renderBodyLines(m.bodyWrappedLines, m.bodyScrollOffset, end,
			m.visualMode, m.visualStart, m.visualEnd))

		// Pad short content so the indicator always sits at the bottom of the panel.
		shownLines := end - m.bodyScrollOffset
		for i := shownLines; i < visibleLines; i++ {
			sb.WriteString("\n")
		}

		// Scroll indicator (pinned to bottom)
		if totalLines > visibleLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%", m.bodyScrollOffset+1, totalLines, pct)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		}
	}

	// Quick reply picker overlay at the bottom of the preview panel.
	if m.quickReplyOpen {
		sb.WriteString(m.renderQuickReplyPicker(innerW))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w - 2). // subtract 2 for left+right borders
		Height(panelHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingLeft(1)

	return panelStyle.Render(sb.String())
}

// renderBodyLines joins lines[start:end] into a string, applying a purple
// highlight to lines within [visualStart, visualEnd] when visualMode is true.
func renderBodyLines(lines []string, start, end int, visualMode bool, visualStart, visualEnd int) string {
	if !visualMode {
		return strings.Join(lines[start:end], "\n")
	}
	highlightStyle := lipgloss.NewStyle().Background(lipgloss.Color("57")).Foreground(lipgloss.Color("229"))
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
	innerW := m.windowWidth - 2
	if innerW < 10 {
		innerW = 10
	}

	var sb strings.Builder

	headerColor := "255"
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(headerColor))

	email := m.selectedTimelineEmail
	sb.WriteString(headerStyle.Render(truncate("From: "+email.Sender, innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Subj: "+email.Subject, innerW)) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	// Reserve 1 row at the bottom for the scroll indicator.
	// Also reserve rows for the quick reply picker overlay when open.
	maxBodyLines := m.windowHeight - 5 // 4 header rows + 1 scroll indicator
	if m.quickReplyOpen {
		pickerRows := 2 + len(m.quickReplies)
		if len(m.quickReplies) == 0 {
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

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	if m.emailBodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.emailBody != nil {
		// Show inline images — same 3-path logic as split view
		imageLines := 0
		for _, img := range m.emailBody.InlineImages {
			if iterm2.IsSupported() {
				rendered := iterm2.Render(img.Data, innerW)
				if rendered != "" {
					sb.WriteString(rendered)
					imageLines++
					continue
				}
			}
			var label string
			if desc, ok := m.inlineImageDescs[img.ContentID]; ok {
				label = fmt.Sprintf("[Image: %s]", desc)
			} else if m.classifier != nil && m.classifier.HasVisionModel() {
				label = fmt.Sprintf("[image  %s  %d KB  — describing…]", img.MIMEType, len(img.Data)/1024)
			} else {
				label = fmt.Sprintf("[image: %s]", img.MIMEType)
			}
			label = truncate(label, innerW)
			sb.WriteString(dimStyle.Render(label) + "\n")
			imageLines++
		}
		// Reserve lines used by image labels so body scroll accounting is correct
		maxBodyLines -= imageLines
		if maxBodyLines < 1 {
			maxBodyLines = 1
		}

		body := stripInvisibleChars(m.emailBody.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		// Re-wrap if width changed (full-screen uses different innerW than split view)
		if m.bodyWrappedLines == nil || m.bodyWrappedWidth != innerW {
			if m.emailBody.IsFromHTML {
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						m.bodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
					}
				} else {
					m.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
				}
			} else {
				m.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
			}
			m.bodyWrappedWidth = innerW
		}

		totalLines := len(m.bodyWrappedLines)
		maxOffset := totalLines - maxBodyLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.bodyScrollOffset > maxOffset {
			m.bodyScrollOffset = maxOffset
		}

		end := m.bodyScrollOffset + maxBodyLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(renderBodyLines(m.bodyWrappedLines, m.bodyScrollOffset, end,
			m.visualMode, m.visualStart, m.visualEnd))

		// Pad short content so the indicator always sits at the bottom of the panel.
		visibleLines := end - m.bodyScrollOffset
		for i := visibleLines; i < maxBodyLines; i++ {
			sb.WriteString("\n")
		}

		// Scroll indicator (pinned to bottom)
		if totalLines > maxBodyLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%  │  z/esc: exit full-screen", m.bodyScrollOffset+1, totalLines, pct)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		} else {
			sb.WriteString("\n" + dimStyle.Render(" z/esc: exit full-screen"))
		}
	}

	// Quick reply picker overlay at the bottom of full-screen view.
	if m.quickReplyOpen {
		sb.WriteString(m.renderQuickReplyPicker(innerW))
	}

	return lipgloss.NewStyle().
		Width(m.windowWidth).
		Height(m.windowHeight).
		Render(sb.String())
}

// emptyStateView returns a placeholder string the same height as the content
// area, with msg centred vertically. Used when a table has no rows to display.
func (m *Model) emptyStateView(msg string) string {
	h := m.windowHeight - 7
	if h < 5 {
		h = 5
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
	if m.selectedTimelineEmail == nil {
		return nil // preview not open
	}
	cursor := m.timelineTable.Cursor()
	if cursor >= len(m.threadRowMap) {
		return nil
	}
	ref := m.threadRowMap[cursor]
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
	if email.MessageID == m.selectedTimelineEmail.MessageID {
		return nil // same email already shown
	}
	m.selectedTimelineEmail = email
	m.emailBody = nil
	m.emailBodyLoading = true
	m.inlineImageDescs = nil
	m.bodyScrollOffset = 0
	m.bodyWrappedLines = nil
	m.quickRepliesReady = false
	m.quickReplies = nil
	m.quickRepliesAIFetched = false
	return m.loadEmailBodyCmd(email.Folder, email.UID)
}

func (m *Model) loadEmailBodyCmd(folder string, uid uint32) tea.Cmd {
	// Cancel any in-flight body fetch so rapid j/k doesn't pile up
	// concurrent IMAP requests that overwhelm the connection.
	if m.bodyFetchCancel != nil {
		m.bodyFetchCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	m.bodyFetchCancel = cancel

	messageID := ""
	if m.selectedTimelineEmail != nil {
		messageID = m.selectedTimelineEmail.MessageID
	}
	b := m.backend // capture for goroutine
	return func() tea.Msg {
		defer cancel()
		if ctx.Err() != nil {
			return EmailBodyMsg{Err: ctx.Err(), MessageID: messageID}
		}
		// Single attempt — IMAP layer handles reconnection internally.
		// Context cancellation handles rapid cursor movement.
		body, err := b.FetchEmailBody(folder, uid)
		if ctx.Err() != nil {
			return EmailBodyMsg{Err: ctx.Err(), MessageID: messageID}
		}
		return EmailBodyMsg{Body: body, Err: err, MessageID: messageID}
	}
}

// fetchCleanupBodyCmd returns a tea.Cmd that fetches an email body for the Cleanup tab preview.
func fetchCleanupBodyCmd(b backend.Backend, email *models.EmailData) tea.Cmd {
	return func() tea.Msg {
		body, err := b.FetchEmailBody(email.Folder, email.UID)
		return CleanupEmailBodyMsg{Body: body, Err: err}
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

// openQuickReply pre-fills the Compose tab with the selected reply template and switches to it.
func (m *Model) openQuickReply(template string) (tea.Model, tea.Cmd) {
	m.quickReplyOpen = false
	if m.selectedTimelineEmail == nil {
		return m, nil
	}
	email := m.selectedTimelineEmail
	m.activeTab = tabCompose
	m.composeTo.SetValue(email.Sender)
	subject := email.Subject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	m.composeSubject.SetValue(subject)
	m.composeBody.SetValue(template)
	m.composeField = 4
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Focus()
	return m, nil
}

// renderQuickReplyPicker renders the quick reply picker overlay appended to the preview panel.
func (m *Model) renderQuickReplyPicker(width int) string {
	if width < 20 {
		width = 20
	}
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Width(width)
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(width)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	aiLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	var sb strings.Builder
	sb.WriteString(strings.Repeat("─", width) + "\n")

	header := "Quick Reply — ↑↓ navigate  Enter: compose  Esc: close"
	if !m.quickRepliesReady {
		header = "Quick Reply — generating suggestions…"
	}
	sb.WriteString(headerStyle.Render(truncate(header, width)) + "\n")

	cannedCount := 5 // first 5 are always canned
	for i, reply := range m.quickReplies {
		num := fmt.Sprintf("%d. ", i+1)
		var label string
		if i >= cannedCount {
			label = num + aiLabelStyle.Render("[AI] ") + truncate(reply, width-len(num)-5)
		} else {
			label = num + truncate(reply, width-len(num))
		}
		if i == m.quickReplyIdx {
			sb.WriteString(selectedStyle.Render(label) + "\n")
		} else {
			sb.WriteString(normalStyle.Render(label) + "\n")
		}
	}

	if len(m.quickReplies) == 0 && m.quickRepliesReady {
		sb.WriteString(dimStyle.Render("  No suggestions available") + "\n")
	}

	return sb.String()
}

func (m *Model) renderCleanupPreview() string {
	w := m.cleanupPreviewWidth
	if w <= 0 {
		w = 40
	}
	innerW := w - 4 // left border + padding

	var sb strings.Builder

	borderColor := "238"
	headerColor := "245"

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(headerColor))
	dimStyle := headerStyle

	if m.cleanupPreviewEmail != nil {
		email := m.cleanupPreviewEmail
		sb.WriteString(headerStyle.Render(truncate("From: "+email.Sender, innerW)) + "\n")
		sb.WriteString(headerStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), innerW)) + "\n")
		sb.WriteString(headerStyle.Render(truncate("Subj: "+email.Subject, innerW)) + "\n")
		sb.WriteString(strings.Repeat("─", innerW) + "\n")
	}

	panelHeight := m.windowHeight - 7
	if panelHeight < 5 {
		panelHeight = 5
	}
	// Header block is 4 rows; reserve 1 for scroll indicator → maxBodyLines = panelHeight - 4 - 1
	maxBodyLines := panelHeight - 5
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}

	if m.cleanupBodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.cleanupEmailBody != nil {
		body := stripInvisibleChars(m.cleanupEmailBody.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		if m.cleanupBodyWrappedLines == nil || m.cleanupBodyWrappedWidth != innerW {
			if m.cleanupEmailBody.IsFromHTML {
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						m.cleanupBodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.cleanupBodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
					}
				} else {
					m.cleanupBodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
				}
			} else {
				m.cleanupBodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
			}
			m.cleanupBodyWrappedWidth = innerW
		}

		totalLines := len(m.cleanupBodyWrappedLines)
		maxOffset := totalLines - maxBodyLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.cleanupBodyScrollOffset > maxOffset {
			m.cleanupBodyScrollOffset = maxOffset
		}

		end := m.cleanupBodyScrollOffset + maxBodyLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(strings.Join(m.cleanupBodyWrappedLines[m.cleanupBodyScrollOffset:end], "\n"))

		// Pad short content so the indicator always sits at the bottom of the panel.
		visibleLines := end - m.cleanupBodyScrollOffset
		for i := visibleLines; i < maxBodyLines; i++ {
			sb.WriteString("\n")
		}

		escHint := "Esc: close"
		zHint := "z: full-screen"
		if m.cleanupFullScreen {
			escHint = "Esc: close preview"
			zHint = "z: exit full-screen"
		}
		if totalLines > maxBodyLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.cleanupBodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" D: delete  e: archive  %s  ↑↓ j/k  line %d/%d  %d%%  │  %s", zHint, m.cleanupBodyScrollOffset+1, totalLines, pct, escHint)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		} else {
			indicator := fmt.Sprintf(" D: delete  e: archive  %s  ↑↓ j/k  │  %s", zHint, escHint)
			sb.WriteString("\n" + dimStyle.Render(indicator))
		}
	} else {
		sb.WriteString(dimStyle.Render("(No content)"))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w - 2). // subtract 2 for left+right borders
		Height(panelHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingLeft(1)

	return panelStyle.Render(sb.String())
}
