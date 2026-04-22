package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/ai"
	"mail-processor/internal/backend"
	"mail-processor/internal/models"
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

func (m *Model) timelinePreviewInnerHeight() int {
	if h := m.timelineTable.Height() + 1; h >= 5 {
		return h
	}
	if h := m.windowHeight - 7; h >= 5 {
		return h
	}
	return 5
}

func (m *Model) cleanupPreviewInnerHeight() int {
	if h := m.detailsTable.Height() + 1; h >= 5 {
		return h
	}
	if h := m.windowHeight - 7; h >= 5 {
		return h
	}
	return 5
}

func (m *Model) renderEmailPreview() string {
	w := m.timeline.previewWidth
	if w <= 0 {
		w = 40
	}
	innerW := w - 4 // left border + padding

	var sb strings.Builder

	// Focus-aware colors: brighter when preview panel has focus
	borderColor := "238"
	headerColor := "245"
	chrome := m.chromeState(m.buildLayoutPlan(m.windowWidth, m.windowHeight))
	if chrome.FocusedPanel == panelPreview {
		borderColor = string(defaultTheme.BorderActive)
		headerColor = string(defaultTheme.ConfirmFg)
	} else {
		borderColor = string(defaultTheme.BorderInactive)
		headerColor = string(defaultTheme.TabInactiveFg)
	}

	// Header block
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(headerColor))
	email := m.timeline.selectedEmail
	sb.WriteString(headerStyle.Render(truncate("From: "+email.Sender, innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Subj: "+email.Subject, innerW)) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	panelHeight := m.timelinePreviewInnerHeight()
	// Header block is 4 rows (From + Date + Subj + separator).
	// Reserve 1 row for scroll indicator → maxBodyLines = panelHeight - 4 - 1.
	maxBodyLines := panelHeight - 5
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}

	// Reserve rows for the quick reply picker overlay when open.
	pickerLines := 0
	if m.timeline.quickReplyOpen {
		pickerLines = 2 + len(m.timeline.quickReplies) // divider + header + items
		if len(m.timeline.quickReplies) == 0 {
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
	if m.timeline.bodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.timeline.body != nil {
		// Split view: show compact image placeholders (no iTerm2 rendering).
		// Full-screen (z) renders actual images.
		imageLines := 0
		if nImg := len(m.timeline.body.InlineImages); nImg > 0 {
			label := fmt.Sprintf("[%d image(s) — press z for full-screen to view]", nImg)
			sb.WriteString(dimStyle.Render(truncate(label, innerW)) + "\n")
			imageLines++
		}

		// Show downloadable attachments
		attachStyle := lipgloss.NewStyle().
			Foreground(defaultTheme.TextFg).
			Background(defaultTheme.StatusBg)
		selectedAttachStyle := lipgloss.NewStyle().
			Foreground(defaultTheme.TabActiveFg).
			Background(defaultTheme.TabActiveBg)
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
			imageLines++
		}

		// Save-path prompt
		if m.timeline.attachmentSavePrompt {
			promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
			sb.WriteString(promptStyle.Render("Save to: ") + m.timeline.attachmentSaveInput.View() + "\n")
			imageLines++
		}

		// Body — wrap/render once and cache; re-render only if panel width changed
		body := stripInvisibleChars(m.timeline.body.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		if m.timeline.bodyWrappedLines == nil || m.timeline.bodyWrappedWidth != innerW {
			if m.timeline.body.IsFromHTML {
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
						m.timeline.bodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.timeline.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
					}
				} else {
					m.timeline.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
				}
				// Safety: hard-truncate every line to innerW visual chars.
				// Glamour and linkification can produce lines with ANSI/OSC sequences
				// that break lipgloss width math, causing panel overflow.
				for i, line := range m.timeline.bodyWrappedLines {
					m.timeline.bodyWrappedLines[i] = ansi.Truncate(line, innerW, "")
				}
			} else {
				m.timeline.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
			}
			m.timeline.bodyWrappedWidth = innerW
		}

		// Clamp scroll offset
		visibleLines := maxBodyLines - imageLines
		if visibleLines < 1 {
			visibleLines = 1
		}
		totalLines := len(m.timeline.bodyWrappedLines)
		maxOffset := totalLines - visibleLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.timeline.bodyScrollOffset > maxOffset {
			m.timeline.bodyScrollOffset = maxOffset
		}

		end := m.timeline.bodyScrollOffset + visibleLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(renderBodyLines(m.timeline.bodyWrappedLines, m.timeline.bodyScrollOffset, end,
			m.timeline.visualMode, m.timeline.visualStart, m.timeline.visualEnd))

		// Pad short content so the indicator always sits at the bottom of the panel.
		shownLines := end - m.timeline.bodyScrollOffset
		for i := shownLines; i < visibleLines; i++ {
			sb.WriteString("\n")
		}

		// Scroll indicator (pinned to bottom)
		if totalLines > visibleLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.timeline.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%", m.timeline.bodyScrollOffset+1, totalLines, pct)
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
		}
	}

	// Quick reply picker overlay at the bottom of the preview panel.
	if m.timeline.quickReplyOpen {
		sb.WriteString(m.renderQuickReplyPicker(innerW))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w - 2). // subtract 2 for left+right borders
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingLeft(1)

	return panelStyle.Render(fitPanelContentHeight(sb.String(), panelHeight))
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

	email := m.timeline.selectedEmail
	sb.WriteString(headerStyle.Render(truncate("From: "+email.Sender, innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), innerW)) + "\n")
	sb.WriteString(headerStyle.Render(truncate("Subj: "+email.Subject, innerW)) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	// Reserve 1 row at the bottom for the scroll indicator.
	// Also reserve rows for the quick reply picker overlay when open.
	maxBodyLines := m.windowHeight - 5 // 4 header rows + 1 scroll indicator
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

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	if m.timeline.bodyLoading {
		sb.WriteString(dimStyle.Render("Loading…"))
	} else if m.timeline.body != nil {
		// Show inline images as text placeholders with AI description when available.
		imageLines := 0
		for _, img := range m.timeline.body.InlineImages {
			var label string
			if desc, ok := m.timeline.inlineImageDescs[img.ContentID]; ok {
				label = fmt.Sprintf("[Image: %s]", desc)
			} else {
				label = fmt.Sprintf("[image: %s  %d KB]", img.MIMEType, len(img.Data)/1024)
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

		body := stripInvisibleChars(m.timeline.body.TextPlain)
		if body == "" {
			body = "(No plain text — HTML only)"
		}
		// Re-wrap if width changed (full-screen uses different innerW than split view)
		if m.timeline.bodyWrappedLines == nil || m.timeline.bodyWrappedWidth != innerW {
			if m.timeline.body.IsFromHTML {
				renderer, rerr := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(innerW),
				)
				if rerr == nil {
					if rendered, err := renderer.Render(body); err == nil {
						rendered = strings.TrimRight(rendered, "\n")
						rendered = lipgloss.NewStyle().MaxWidth(innerW).Render(rendered)
						rendered = strings.TrimRight(rendered, "\n")
						m.timeline.bodyWrappedLines = strings.Split(rendered, "\n")
					} else {
						m.timeline.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
					}
				} else {
					m.timeline.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
				}
			} else {
				m.timeline.bodyWrappedLines = linkifyWrappedLines(wrapLines(body, innerW))
			}
			m.timeline.bodyWrappedWidth = innerW
		}

		totalLines := len(m.timeline.bodyWrappedLines)
		maxOffset := totalLines - maxBodyLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.timeline.bodyScrollOffset > maxOffset {
			m.timeline.bodyScrollOffset = maxOffset
		}

		end := m.timeline.bodyScrollOffset + maxBodyLines
		if end > totalLines {
			end = totalLines
		}
		sb.WriteString(renderBodyLines(m.timeline.bodyWrappedLines, m.timeline.bodyScrollOffset, end,
			m.timeline.visualMode, m.timeline.visualStart, m.timeline.visualEnd))

		// Pad short content so the indicator always sits at the bottom of the panel.
		visibleLines := end - m.timeline.bodyScrollOffset
		for i := visibleLines; i < maxBodyLines; i++ {
			sb.WriteString("\n")
		}

		// Scroll indicator (pinned to bottom)
		if totalLines > maxBodyLines {
			pct := 0
			if maxOffset > 0 {
				pct = m.timeline.bodyScrollOffset * 100 / maxOffset
			}
			indicator := fmt.Sprintf(" ↑↓ j/k  line %d/%d  %d%%  │  z/esc: exit full-screen", m.timeline.bodyScrollOffset+1, totalLines, pct)
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
		} else {
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(" z/esc: exit full-screen", innerW)))
		}
	}

	// Quick reply picker overlay at the bottom of full-screen view.
	if m.timeline.quickReplyOpen {
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
	m.timeline.selectedEmail = email
	m.timeline.body = nil
	m.timeline.bodyLoading = true
	m.timeline.inlineImageDescs = nil
	m.timeline.bodyScrollOffset = 0
	m.timeline.bodyWrappedLines = nil
	m.timeline.quickRepliesReady = false
	m.timeline.quickReplies = nil
	m.timeline.quickRepliesAIFetched = false
	return m.loadEmailBodyCmd(email.Folder, email.UID)
}

func (m *Model) loadEmailBodyCmd(folder string, uid uint32) tea.Cmd {
	// Cancel any in-flight body fetch so rapid j/k doesn't pile up
	// concurrent IMAP requests that overwhelm the connection.
	if m.timeline.bodyFetchCancel != nil {
		m.timeline.bodyFetchCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	m.timeline.bodyFetchCancel = cancel

	messageID := ""
	if m.timeline.selectedEmail != nil {
		messageID = m.timeline.selectedEmail.MessageID
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
	m.timeline.quickReplyOpen = false
	if m.timeline.selectedEmail == nil {
		return m, nil
	}
	email := m.timeline.selectedEmail
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
	if !m.timeline.quickRepliesReady {
		header = "Quick Reply — generating suggestions…"
	}
	sb.WriteString(headerStyle.Render(truncate(header, width)) + "\n")

	cannedCount := 5 // first 5 are always canned
	for i, reply := range m.timeline.quickReplies {
		num := fmt.Sprintf("%d. ", i+1)
		var label string
		if i >= cannedCount {
			label = num + aiLabelStyle.Render("[AI] ") + truncate(reply, width-len(num)-5)
		} else {
			label = num + truncate(reply, width-len(num))
		}
		if i == m.timeline.quickReplyIdx {
			sb.WriteString(selectedStyle.Render(label) + "\n")
		} else {
			sb.WriteString(normalStyle.Render(label) + "\n")
		}
	}

	if len(m.timeline.quickReplies) == 0 && m.timeline.quickRepliesReady {
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

	panelHeight := m.cleanupPreviewInnerHeight()
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
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
		} else {
			indicator := fmt.Sprintf(" D: delete  e: archive  %s  ↑↓ j/k  │  %s", zHint, escHint)
			sb.WriteString("\n" + dimStyle.Render(truncateVisual(indicator, innerW)))
		}
	} else {
		sb.WriteString(dimStyle.Render("(No content)"))
	}

	panelStyle := lipgloss.NewStyle().
		Width(w - 2). // subtract 2 for left+right borders
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		PaddingLeft(1)

	return panelStyle.Render(fitPanelContentHeight(sb.String(), panelHeight))
}
