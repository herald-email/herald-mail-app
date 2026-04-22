package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/iterm2"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// --- Thread grouping types ---

// threadGroup holds all emails that share the same normalised subject.
type threadGroup struct {
	normalizedSubject string
	emails            []*models.EmailData // newest first (inherited from sorted input)
}

// timelineRowKind distinguishes collapsed thread headers from individual email rows.
type timelineRowKind int

const (
	rowKindThread timelineRowKind = iota // collapsed thread header (>1 email, not expanded)
	rowKindEmail                         // individual email row
)

// timelineRowRef maps a table-cursor position to a thread group and email.
type timelineRowRef struct {
	kind     timelineRowKind
	group    *threadGroup
	emailIdx int // index into group.emails; meaningful only for rowKindEmail
}

func (m *Model) loadTimelineEmails() tea.Cmd {
	folder := m.currentFolder
	return func() tea.Msg {
		emails, err := m.backend.GetTimelineEmails(folder)
		if err != nil {
			logger.Error("Failed to load timeline emails: %v", err)
			return TimelineLoadedMsg{Emails: nil}
		}
		return TimelineLoadedMsg{Emails: emails}
	}
}

// normalizeSubject strips common reply/forward prefixes (case-insensitive) so that
// "Re: Re: Hello" and "Fwd: Hello" both map to "hello".
func normalizeSubject(s string) string {
	prefixes := []string{"re:", "fwd:", "fw:", "aw:", "tr:"}
	s = strings.TrimSpace(strings.ToLower(s))
	for {
		changed := false
		for _, p := range prefixes {
			if strings.HasPrefix(s, p) {
				s = strings.TrimSpace(s[len(p):])
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return s
}

// buildThreadGroups groups emails by normalised subject.
// emails must already be sorted newest-first; group order is determined by each
// group's most-recent email, so groups are also implicitly newest-first.
func buildThreadGroups(emails []*models.EmailData) []threadGroup {
	var groups []threadGroup
	seen := make(map[string]int) // (normalised subject + "\n" + sender) → index in groups

	for _, e := range emails {
		ns := normalizeSubject(e.Subject)
		if ns == "" {
			// Empty subjects are never grouped; each stands alone.
			groups = append(groups, threadGroup{
				normalizedSubject: ns,
				emails:            []*models.EmailData{e},
			})
			continue
		}
		key := ns + "\n" + strings.ToLower(e.Sender)
		if idx, ok := seen[key]; ok {
			groups[idx].emails = append(groups[idx].emails, e)
		} else {
			seen[key] = len(groups)
			groups = append(groups, threadGroup{
				normalizedSubject: ns,
				emails:            []*models.EmailData{e},
			})
		}
	}
	return groups
}

// updateTimelineTable rebuilds the timeline table rows from m.timeline.emails,
// grouping them into collapsed threads where appropriate.
func (m *Model) updateTimelineTable() {
	maxSubj := m.timeline.subjectWidth
	if maxSubj <= 0 {
		maxSubj = 40
	}
	maxSend := m.timeline.senderWidth
	if maxSend <= 0 {
		maxSend = 20
	}

	trunc := func(s string, n int) string {
		if n <= 0 {
			return ""
		}
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		if n <= 3 {
			return string(r[:n])
		}
		return string(r[:n-3]) + "..."
	}

	emailRow := func(email *models.EmailData, senderPrefix string) table.Row {
		dateStr := "N/A"
		if !email.Date.IsZero() {
			dateStr = email.Date.Format("06-01-02 15:04")
		}
		subject := sanitizeText(email.Subject)
		if subject == "" {
			subject = "(no subject)"
		}
		// Prepend similarity score badge for semantic search results
		if m.timeline.semanticScores != nil {
			if score, ok := m.timeline.semanticScores[email.MessageID]; ok {
				pct := int(score * 100)
				subject = fmt.Sprintf("[%d%%] %s", pct, subject)
			}
		}
		unreadDot := " "
		if !email.IsRead {
			unreadDot = "●"
		}
		starDot := " "
		if email.IsStarred {
			starDot = "★"
		}
		indicatorWidth := len([]rune(unreadDot)) + len([]rune(starDot)) + len([]rune(senderPrefix))
		senderAvail := maxSend - indicatorWidth
		if senderAvail < 1 {
			senderAvail = 1
		}
		sender := unreadDot + starDot + senderPrefix + styledSender(email.Sender, senderAvail)
		att := "N"
		if email.HasAttachments {
			att = "Y"
		}
		tag := ""
		if m.classifications != nil {
			tag = m.classifications[email.MessageID]
		}
		return table.Row{
			sender,
			trunc(subject, maxSubj),
			dateStr,
			fmt.Sprintf("%.1f", float64(email.Size)/1024),
			att,
			tag,
		}
	}

	// Priority: chat filter > search results > full list
	var displayEmails []*models.EmailData
	switch {
	case m.timeline.chatFilterMode:
		displayEmails = m.timeline.chatFilteredEmails
	case m.timeline.searchMode && m.timeline.searchResults != nil:
		displayEmails = m.timeline.searchResults
	default:
		displayEmails = m.timeline.emails
	}

	// Build thread groups from the full email list
	m.timeline.threadGroups = buildThreadGroups(displayEmails)
	m.timeline.threadRowMap = m.timeline.threadRowMap[:0]

	// Sort starred threads to the top, preserving date order within each bucket.
	sort.SliceStable(m.timeline.threadGroups, func(i, j int) bool {
		iStarred := len(m.timeline.threadGroups[i].emails) > 0 && m.timeline.threadGroups[i].emails[0].IsStarred
		jStarred := len(m.timeline.threadGroups[j].emails) > 0 && m.timeline.threadGroups[j].emails[0].IsStarred
		return iStarred && !jStarred
	})

	var rows []table.Row
	for gi := range m.timeline.threadGroups {
		g := &m.timeline.threadGroups[gi]
		expanded := m.timeline.expandedThreads[g.normalizedSubject]

		if len(g.emails) == 1 {
			// Single-email thread: show as a plain row
			rows = append(rows, emailRow(g.emails[0], ""))
			m.timeline.threadRowMap = append(m.timeline.threadRowMap, timelineRowRef{
				kind: rowKindEmail, group: g, emailIdx: 0,
			})
			continue
		}

		if !expanded {
			// Collapsed thread header: newest email's sender, subject with [N] prefix
			newest := g.emails[0]
			dateStr := "N/A"
			if !newest.Date.IsZero() {
				dateStr = newest.Date.Format("06-01-02 15:04")
			}
			subject := sanitizeText(newest.Subject)
			if subject == "" {
				subject = "(no subject)"
			}
			totalSize := 0
			anyAtt := false
			for _, e := range g.emails {
				totalSize += e.Size
				if e.HasAttachments {
					anyAtt = true
				}
			}
			att := "N"
			if anyAtt {
				att = "Y"
			}
			tag := ""
			if m.classifications != nil {
				tag = m.classifications[newest.MessageID]
			}
			threadSubj := fmt.Sprintf("[%d] %s", len(g.emails), subject)
			// Build sender cell with the same indicators as single-email rows
			// so columns stay aligned across all timeline rows.
			unreadDot := " "
			if !newest.IsRead {
				unreadDot = "●"
			}
			starDot := " "
			if newest.IsStarred {
				starDot = "★"
			}
			indicatorWidth := len([]rune(unreadDot)) + len([]rune(starDot))
			senderAvail := maxSend - indicatorWidth
			if senderAvail < 1 {
				senderAvail = 1
			}
			threadSender := unreadDot + starDot + styledSender(newest.Sender, senderAvail)
			rows = append(rows, table.Row{
				threadSender,
				trunc(threadSubj, maxSubj),
				dateStr,
				fmt.Sprintf("%.1f", float64(totalSize)/1024),
				att,
				tag,
			})
			m.timeline.threadRowMap = append(m.timeline.threadRowMap, timelineRowRef{
				kind: rowKindThread, group: g,
			})
		} else {
			// Expanded: show each email with an indent prefix on all but the first
			for ei, email := range g.emails {
				prefix := ""
				if ei > 0 {
					prefix = "  ↳ "
				}
				rows = append(rows, emailRow(email, prefix))
				m.timeline.threadRowMap = append(m.timeline.threadRowMap, timelineRowRef{
					kind: rowKindEmail, group: g, emailIdx: ei,
				})
			}
		}
	}

	m.timelineTable.SetRows(rows)
}

// renderTimelineView renders the timeline tab content.
// When an email is selected, it splits into a list on the left and preview on the right.
func (m *Model) renderTimelineView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	chrome := m.chromeState(plan)

	var tableView string
	if m.timeline.emails != nil && len(m.timeline.emails) == 0 {
		tableView = m.emptyStateView("No emails in this folder  •  press r to refresh")
	} else {
		style := m.baseStyle.BorderForeground(defaultTheme.BorderInactive)
		if chrome.FocusedPanel == panelTimeline {
			style = style.BorderForeground(defaultTheme.BorderActive)
		}
		tableView = style.Render(renderStyledTableView(&m.timelineTable, 0))
	}

	var mainContent string
	if m.timeline.selectedEmail != nil {
		previewPanel := m.renderEmailPreview()
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, tableView, panelGap, previewPanel)
	} else {
		mainContent = tableView
	}

	if m.showSidebar && !m.sidebarTooWide {
		sidebarStyle := m.baseStyle
		if chrome.FocusedPanel == panelSidebar {
			sidebarStyle = sidebarStyle.BorderForeground(defaultTheme.BorderActive)
		} else {
			sidebarStyle = sidebarStyle.BorderForeground(defaultTheme.BorderInactive)
		}
		sidebarView := sidebarStyle.Render(m.renderSidebar())
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, panelGap, mainContent)
	}
	return mainContent
}

func (m *Model) clearTimelineSearch() {
	m.timeline.searchMode = false
	m.timeline.searchInput.Blur()
	m.timeline.searchInput.SetValue("")
	m.timeline.searchResults = nil
	m.timeline.semanticScores = nil
	m.timeline.searchError = ""
	if m.timeline.emailsCache != nil {
		m.timeline.emails = m.timeline.emailsCache
		m.timeline.emailsCache = nil
	}
	m.updateTimelineTable()
}

func (m *Model) clearTimelineQuickReply() {
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyIdx = 0
}

func (m *Model) clearTimelineFullScreen() {
	m.timeline.fullScreen = false
	m.timeline.bodyWrappedLines = nil
}

func (m *Model) clearTimelinePreview() {
	m.timeline.selectedEmail = nil
	m.timeline.body = nil
	m.timeline.bodyLoading = false
	m.timeline.bodyWrappedLines = nil
	m.timeline.bodyScrollOffset = 0
	m.timeline.visualMode = false
	m.timeline.pendingY = false
	m.setFocusedPanel(panelTimeline)
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
}

func (m *Model) clearTimelineChatFilter() {
	m.timeline.chatFilterMode = false
	m.timeline.chatFilteredEmails = nil
	m.timeline.chatFilterLabel = ""
	m.updateTimelineTable()
}

func (m *Model) openTimelineSearch() {
	m.timeline.searchMode = true
	if m.timeline.emailsCache == nil {
		m.timeline.emailsCache = m.timeline.emails
	}
	m.timeline.searchInput.SetValue("")
	m.timeline.searchInput.Focus()
}

func (m *Model) currentTimelineRowEmail() *models.EmailData {
	cursor := m.timelineTable.Cursor()
	if cursor >= len(m.timeline.threadRowMap) {
		return nil
	}
	ref := m.timeline.threadRowMap[cursor]
	if ref.kind == rowKindThread {
		return ref.group.emails[0]
	}
	return ref.group.emails[ref.emailIdx]
}

func (m *Model) currentTimelineRowRef() (timelineRowRef, bool) {
	cursor := m.timelineTable.Cursor()
	if cursor >= len(m.timeline.threadRowMap) {
		return timelineRowRef{}, false
	}
	return m.timeline.threadRowMap[cursor], true
}

func (m *Model) openCurrentTimelineEmail() tea.Cmd {
	ref, ok := m.currentTimelineRowRef()
	if !ok {
		return nil
	}
	if ref.kind == rowKindThread {
		key := ref.group.normalizedSubject
		savedCursor := m.timelineTable.Cursor()
		m.timeline.expandedThreads[key] = !m.timeline.expandedThreads[key]
		m.updateTimelineTable()
		m.timelineTable.SetCursor(savedCursor)
		return nil
	}
	if ref.emailIdx == 0 && len(ref.group.emails) > 1 && m.timeline.expandedThreads[ref.group.normalizedSubject] {
		savedCursor := m.timelineTable.Cursor()
		m.timeline.expandedThreads[ref.group.normalizedSubject] = false
		m.updateTimelineTable()
		m.timelineTable.SetCursor(savedCursor)
		return nil
	}
	email := ref.group.emails[ref.emailIdx]
	m.timeline.selectedEmail = email
	m.timeline.body = nil
	m.timeline.bodyLoading = true
	m.timeline.inlineImageDescs = nil
	m.timeline.bodyScrollOffset = 0
	m.timeline.quickRepliesAIFetched = false
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	return m.loadEmailBodyCmd(email.Folder, email.UID)
}

func (m *Model) toggleTimelineQuickReply() tea.Cmd {
	if !(m.focusedPanel == panelPreview || m.timeline.fullScreen) || m.timeline.body == nil {
		return nil
	}
	m.timeline.quickReplyOpen = !m.timeline.quickReplyOpen
	if !m.timeline.quickReplyOpen {
		return nil
	}
	if m.timeline.quickReplyIdx >= len(m.timeline.quickReplies) {
		m.timeline.quickReplyIdx = 0
	}
	if m.classifier != nil && m.timeline.body.TextPlain != "" && m.timeline.selectedEmail != nil && !m.timeline.quickRepliesAIFetched {
		m.timeline.quickRepliesAIFetched = true
		email := m.timeline.selectedEmail
		bodyPreview := m.timeline.body.TextPlain
		if len([]rune(bodyPreview)) > 500 {
			bodyPreview = string([]rune(bodyPreview)[:500])
		}
		return generateQuickRepliesCmd(m.classifier, email.Sender, email.Subject, bodyPreview)
	}
	return nil
}

func (m *Model) timelineFilterPrefix() string {
	if m.activeTab != tabTimeline || !m.timeline.chatFilterMode {
		return ""
	}
	filterLabel := m.timeline.chatFilterLabel
	if filterLabel == "" {
		filterLabel = "filtered"
	}
	return lipgloss.NewStyle().
		Foreground(defaultTheme.InfoFg).
		Bold(true).
		Render(fmt.Sprintf("⬡ filter: %s (%d emails)  ", filterLabel, len(m.timeline.chatFilteredEmails)))
}

func (m *Model) appendTimelineStatusParts(parts []string) []string {
	if m.activeTab == tabTimeline {
		parts = append(parts, fmt.Sprintf("%d emails", len(m.timeline.emails)))
	}
	if m.timeline.searchMode {
		if m.timeline.searchResults != nil {
			parts = append(parts, fmt.Sprintf("Search: %d results", len(m.timeline.searchResults)))
		} else {
			parts = append(parts, "Search")
		}
	}
	if m.activeTab == tabTimeline && m.timeline.body != nil {
		if !m.timeline.quickRepliesReady && m.classifier != nil {
			parts = append(parts, "⚡ generating replies…")
		} else if m.timeline.quickRepliesReady && !m.timeline.quickReplyOpen {
			parts = append(parts, "ctrl+q: quick reply")
		}
	}
	if m.timeline.mouseMode {
		parts = append([]string{"[mouse] select mode — m: restore TUI"}, parts...)
	}
	return parts
}

func (m *Model) timelineKeyHints(chrome ChromeState) (string, bool) {
	if m.activeTab != tabTimeline {
		return "", false
	}
	if m.timeline.quickReplyOpen {
		return "↑/k ↓/j: navigate replies  │  enter: compose  │  1-8: select  │  esc: close picker  │  q: quit", true
	}
	if m.timeline.searchMode {
		q := m.timeline.searchInput.View()
		if m.timeline.searchError != "" {
			return fmt.Sprintf("/ %s  │  Error: %s  │  esc: clear", q, m.timeline.searchError), true
		}
		query := m.timeline.searchInput.Value()
		if m.timeline.searchResults != nil && len(m.timeline.searchResults) == 0 && query != "" && !strings.HasPrefix(query, "/*") {
			return fmt.Sprintf("/ %s  │  No results in this folder — try: /* %s  │  esc: clear  │  ctrl+i: server search", q, query), true
		}
		return fmt.Sprintf("/ %s  │  esc: clear  │  ctrl+s: save  │  ctrl+i: server search", q), true
	}
	if m.timeline.chatFilterMode {
		return "esc: clear filter  │  1/2/3/4: tabs  │  ↑/k ↓/j: navigate  │  enter: open  │  q: quit", true
	}
	if chrome.FocusedPanel == panelPreview {
		hasAttachments := m.timeline.body != nil && len(m.timeline.body.Attachments) > 0
		hasMultipleAttachments := m.timeline.body != nil && len(m.timeline.body.Attachments) > 1
		hasUnsub := m.timeline.body != nil && m.timeline.body.ListUnsubscribe != ""
		if m.timeline.visualMode {
			return "j/k: extend selection  │  y: copy selection  │  Y: copy all  │  esc: cancel visual", true
		}
		attachmentHints := ""
		if hasAttachments {
			attachmentHints = " │  s: save attachment"
			if hasMultipleAttachments {
				attachmentHints = " │  [ and ]: attachments" + attachmentHints
			}
		}
		if hasAttachments && hasUnsub {
			return "tab/shift+tab: panels  │  ↑/k ↓/j: scroll" + attachmentHints + " │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  u: unsubscribe  │  esc: close  │  q: quit", true
		}
		if hasUnsub {
			return "tab/shift+tab: panels  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  u: unsubscribe  │  esc: close  │  q: quit", true
		}
		if hasAttachments {
			return "tab/shift+tab: panels  │  ↑/k ↓/j: scroll" + attachmentHints + " │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  esc: close  │  q: quit", true
		}
		return "tab/shift+tab: panels  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  esc: close  │  q: quit", true
	}
	if m.timeline.selectedEmail != nil {
		return "tab/shift+tab: panels  │  ↑/k ↓/j: navigate  │  enter: open  │  esc: close  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  A: re-classify  │  q: quit", true
	}
	return "1/2/3/4: tabs  │  ↑/k ↓/j: navigate  │  enter: open  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  /: search  │  a: AI tag  │  A: re-classify  │  f: sidebar  │  q: quit", true
}

func (m *Model) handleTimelineMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case TimelineLoadedMsg:
		m.timeline.emails = msg.Emails
		m.updateTimelineTable()
		if m.timeline.selectedEmail != nil {
			targetID := m.timeline.selectedEmail.MessageID
			for rowIdx, ref := range m.timeline.threadRowMap {
				if ref.kind == rowKindEmail &&
					ref.group != nil &&
					ref.emailIdx < len(ref.group.emails) &&
					ref.group.emails[ref.emailIdx].MessageID == targetID {
					m.timelineTable.SetCursor(rowIdx)
					break
				}
			}
		}
		if m.classifier != nil {
			return m, tea.Batch(m.runEmbeddingBatch(), m.runContactEnrichment()), true
		}
		return m, nil, true

	case EmailBodyMsg:
		if m.contactPreviewLoading {
			return m, nil, false
		}
		if msg.MessageID != "" && m.timeline.selectedEmail != nil && msg.MessageID != m.timeline.selectedEmail.MessageID {
			return m, nil, true
		}
		m.timeline.bodyLoading = false
		m.timeline.selectedAttachment = 0
		m.timeline.quickReplies = nil
		m.timeline.quickRepliesReady = false
		m.timeline.quickReplyOpen = false
		m.timeline.quickReplyIdx = 0
		if msg.Err != nil {
			logger.Warn("Failed to fetch email body: %v", msg.Err)
			m.timeline.body = &models.EmailBody{TextPlain: "(Failed to load body)"}
		} else {
			m.timeline.body = msg.Body
			if msg.Body != nil && msg.Body.TextPlain != "" && m.timeline.selectedEmail != nil {
				msgID := m.timeline.selectedEmail.MessageID
				bodyText := msg.Body.TextPlain
				go func() {
					if err := m.backend.CacheBodyText(msgID, bodyText); err != nil {
						logger.Warn("Failed to cache body text: %v", err)
					}
				}()
			}
			if m.timeline.selectedEmail != nil {
				email := m.timeline.selectedEmail
				m.timeline.quickReplies = buildCannedReplies(email.Sender)
				body := msg.Body
				var cmds []tea.Cmd
				if !email.IsRead {
					email.IsRead = true
					cmds = append(cmds, markReadCmd(m.backend, email.MessageID, email.Folder))
				}
				if body != nil && (body.ListUnsubscribe != "" || body.ListUnsubscribePost != "") {
					cmds = append(cmds, cacheUnsubscribeHeadersCmd(m.backend, email.MessageID, body.ListUnsubscribe, body.ListUnsubscribePost))
				}
				if body != nil && len(body.InlineImages) > 0 && m.classifier != nil && m.classifier.HasVisionModel() && !iterm2.IsSupported() {
					cmds = append(cmds, describeImagesCmd(m.classifier, body.InlineImages)...)
				}
				m.timeline.quickRepliesReady = true
				if len(cmds) > 0 {
					m.timeline.bodyWrappedLines = nil
					return m, tea.Batch(cmds...), true
				}
			} else {
				m.timeline.quickRepliesReady = true
			}
		}
		m.timeline.bodyWrappedLines = nil
		return m, nil, true

	case QuickRepliesMsg:
		if msg.Err != nil {
			logger.Warn("Quick reply generation failed: %v", msg.Err)
		} else if len(msg.Replies) > 0 {
			m.timeline.quickReplies = append(m.timeline.quickReplies, msg.Replies...)
		}
		m.timeline.quickRepliesReady = true
		return m, nil, true

	case ImageDescMsg:
		if msg.Err == nil && msg.Description != "" {
			if m.timeline.inlineImageDescs == nil {
				m.timeline.inlineImageDescs = make(map[string]string)
			}
			m.timeline.inlineImageDescs[msg.ContentID] = msg.Description
		}
		return m, nil, true

	case ChatFilterActivatedMsg:
		m.timeline.chatFilterMode = true
		m.timeline.chatFilteredEmails = msg.Emails
		m.timeline.chatFilterLabel = msg.Label
		m.activeTab = tabTimeline
		m.updateTimelineTable()
		return m, nil, true

	case SearchResultMsg:
		if msg.Err != nil {
			m.timeline.searchError = msg.Err.Error()
			return m, nil, true
		}
		m.timeline.searchError = ""
		if msg.Query == "" {
			m.timeline.searchResults = nil
			m.timeline.semanticScores = nil
			if m.timeline.emailsCache != nil {
				m.timeline.emails = m.timeline.emailsCache
				m.timeline.emailsCache = nil
			}
			m.updateTimelineTable()
		} else {
			m.timeline.searchResults = msg.Emails
			m.timeline.semanticScores = msg.Scores
			m.updateTimelineTable()
		}
		return m, nil, true

	case NewEmailsMsg:
		if msg.Folder == m.currentFolder {
			existing := make(map[string]struct{}, len(m.timeline.emails))
			for _, e := range m.timeline.emails {
				existing[e.MessageID] = struct{}{}
			}
			var fresh []*models.EmailData
			for _, e := range msg.Emails {
				if _, dup := existing[e.MessageID]; !dup {
					fresh = append(fresh, e)
				}
			}
			if len(fresh) > 0 {
				m.timeline.emails = append(fresh, m.timeline.emails...)
				if m.timeline.emailsCache != nil {
					m.timeline.emailsCache = append(fresh, m.timeline.emailsCache...)
				}
				m.updateTimelineTable()
			}
		}
		var cmds []tea.Cmd
		cmds = append(cmds, m.listenForNewEmails())
		for _, email := range msg.Emails {
			if m.classifier != nil && m.classifications[email.MessageID] == "" {
				cmds = append(cmds, m.autoClassifyEmailCmd(email))
			} else if cat := m.classifications[email.MessageID]; cat != "" {
				select {
				case m.ruleRequestCh <- models.RuleRequest{Email: email, Category: cat}:
				default:
				}
			}
			if ok, _ := m.backend.IsUnsubscribedSender(email.Sender); ok {
				m.statusMessage = fmt.Sprintf("⚠ Email from unsubscribed sender: %s", email.Sender)
			}
		}
		return m, tea.Batch(cmds...), true

	case EmailExpungedMsg:
		if msg.Folder == m.currentFolder {
			filtered := m.timeline.emails[:0]
			for _, e := range m.timeline.emails {
				if e.MessageID != msg.MessageID {
					filtered = append(filtered, e)
				}
			}
			m.timeline.emails = filtered
			if m.timeline.emailsCache != nil {
				filtered2 := m.timeline.emailsCache[:0]
				for _, e := range m.timeline.emailsCache {
					if e.MessageID != msg.MessageID {
						filtered2 = append(filtered2, e)
					}
				}
				m.timeline.emailsCache = filtered2
			}
			m.updateTimelineTable()
		}
		return m, m.listenForExpunged(), true

	case StarResultMsg:
		if msg.Err != nil {
			m.statusMessage = "Star failed: " + msg.Err.Error()
		} else {
			for _, e := range m.timeline.emails {
				if e.MessageID == msg.MessageID {
					e.IsStarred = msg.Starred
					break
				}
			}
			m.updateTimelineTable()
			if msg.Starred {
				m.statusMessage = "★ Starred"
			} else {
				m.statusMessage = "☆ Unstarred"
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) handleTimelineKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.activeTab != tabTimeline {
		return m, nil, false
	}
	switch msg.String() {
	case "*":
		if !m.loading {
			if email := m.currentTimelineRowEmail(); email != nil {
				return m, m.toggleStarCmd(email), true
			}
		}
		return m, nil, true
	case "u":
		if m.timeline.body != nil && m.timeline.selectedEmail != nil && m.timeline.body.ListUnsubscribe != "" {
			sender := m.timeline.selectedEmail.Sender
			body := m.timeline.body
			m.pendingUnsubscribe = true
			m.pendingUnsubscribeDesc = fmt.Sprintf("Unsubscribe from %s?", sender)
			m.pendingUnsubscribeAction = func() tea.Cmd { return unsubscribeCmd(body) }
		}
		return m, nil, true
	case "F":
		if !m.loading {
			if email := m.currentTimelineRowEmail(); email != nil {
				subject := email.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "fwd:") {
					subject = "Fwd: " + subject
				}
				fwdBody := fmt.Sprintf("\n\n--- Forwarded message ---\nFrom: %s\nDate: %s\nSubject: %s\n\n",
					email.Sender, email.Date.Format("Mon, 02 Jan 2006 15:04"), email.Subject)
				if m.timeline.body != nil && m.timeline.selectedEmail != nil && m.timeline.selectedEmail.MessageID == email.MessageID {
					fwdBody += m.timeline.body.TextPlain
				}
				m.activeTab = tabCompose
				m.composeTo.SetValue("")
				m.composeSubject.SetValue(subject)
				m.composeBody.SetValue(fwdBody)
				m.composeField = 0
				m.composeTo.Focus()
				m.composeSubject.Blur()
				m.composeBody.Blur()
			}
		}
		return m, nil, true
	case "/":
		if !m.loading && !m.timeline.searchMode {
			m.openTimelineSearch()
		}
		return m, nil, true
	case "enter":
		if !m.loading {
			if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 0 {
				model, cmd := m.openQuickReply(m.timeline.quickReplies[m.timeline.quickReplyIdx])
				return model, cmd, true
			}
			if m.focusedPanel == panelSidebar {
				m.selectSidebarFolder()
				m.clearTimelineChatFilter()
				return m, tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForProgress()), true
			}
			if m.focusedPanel != panelSidebar {
				return m, m.openCurrentTimelineEmail(), true
			}
		}
		return m, nil, true
	case "ctrl+q":
		return m, m.toggleTimelineQuickReply(), true
	case "z":
		if !m.loading && m.timeline.selectedEmail != nil {
			m.timeline.fullScreen = !m.timeline.fullScreen
			m.timeline.bodyWrappedLines = nil
		}
		return m, nil, true
	case "s":
		if !m.loading && m.focusedPanel == panelPreview && m.timeline.body != nil &&
			len(m.timeline.body.Attachments) > 0 && !m.timeline.attachmentSavePrompt {
			att := m.timeline.body.Attachments[m.timeline.selectedAttachment]
			defaultPath := expandTilde("~/Downloads/" + att.Filename)
			m.timeline.attachmentSaveInput.SetValue(defaultPath)
			m.timeline.attachmentSaveInput.Focus()
			m.timeline.attachmentSavePrompt = true
		}
		return m, nil, true
	case "]":
		if !m.loading && (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && m.timeline.selectedAttachment < len(m.timeline.body.Attachments)-1 {
			m.timeline.selectedAttachment++
			return m, nil, true
		}
		if m.timeline.body != nil && len(m.timeline.body.Attachments) > 1 {
			return m, nil, true
		}
	case "[":
		if !m.loading && (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && m.timeline.selectedAttachment > 0 {
			m.timeline.selectedAttachment--
			return m, nil, true
		}
		if m.timeline.body != nil && len(m.timeline.body.Attachments) > 1 {
			return m, nil, true
		}
	case "R":
		if !m.loading {
			if email := m.currentTimelineRowEmail(); email != nil {
				m.activeTab = tabCompose
				m.replyContextEmail = email
				m.composeAIThread = true
				m.composeTo.SetValue(email.Sender)
				subject := email.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}
				m.composeSubject.SetValue(subject)
				m.composeField = 4
				m.composeTo.Blur()
				m.composeSubject.Blur()
				m.composeBody.Focus()
			}
		}
		return m, nil, true
	case "up", "k":
		if !m.loading {
			if m.timeline.quickReplyOpen {
				if m.timeline.quickReplyIdx > 0 {
					m.timeline.quickReplyIdx--
				}
				return m, nil, true
			}
			if m.timeline.fullScreen {
				if m.timeline.visualMode {
					if m.timeline.visualEnd > m.timeline.visualStart {
						m.timeline.visualEnd--
					}
				} else if m.timeline.bodyScrollOffset > 0 {
					m.timeline.bodyScrollOffset--
				}
				return m, nil, true
			}
			if m.focusedPanel == panelPreview {
				if m.timeline.visualMode {
					if m.timeline.visualEnd > m.timeline.visualStart {
						m.timeline.visualEnd--
					}
				} else if m.timeline.bodyScrollOffset > 0 {
					m.timeline.bodyScrollOffset--
				}
				return m, nil, true
			}
			if m.focusedPanel == panelSidebar {
				model, cmd := m.handleNavigation(-1)
				return model, cmd, true
			}
			m.timelineTable.MoveUp(1)
			return m, m.maybeUpdatePreview(), true
		}
		return m, nil, true
	case "down", "j":
		if !m.loading {
			if m.timeline.quickReplyOpen {
				if m.timeline.quickReplyIdx < len(m.timeline.quickReplies)-1 {
					m.timeline.quickReplyIdx++
				}
				return m, nil, true
			}
			if m.timeline.fullScreen {
				if m.timeline.visualMode {
					if m.timeline.visualEnd < len(m.timeline.bodyWrappedLines)-1 {
						m.timeline.visualEnd++
					}
				} else {
					m.timeline.bodyScrollOffset++
				}
				return m, nil, true
			}
			if m.focusedPanel == panelPreview {
				if m.timeline.visualMode {
					if m.timeline.visualEnd < len(m.timeline.bodyWrappedLines)-1 {
						m.timeline.visualEnd++
					}
				} else {
					m.timeline.bodyScrollOffset++
				}
				return m, nil, true
			}
			if m.focusedPanel == panelSidebar {
				model, cmd := m.handleNavigation(1)
				return model, cmd, true
			}
			m.timelineTable.MoveDown(1)
			return m, m.maybeUpdatePreview(), true
		}
		return m, nil, true
	case "v":
		if m.timeline.fullScreen || m.focusedPanel == panelPreview {
			if len(m.timeline.bodyWrappedLines) > 0 {
				m.timeline.visualMode = !m.timeline.visualMode
				if m.timeline.visualMode {
					m.timeline.visualStart = m.timeline.bodyScrollOffset
					m.timeline.visualEnd = m.timeline.bodyScrollOffset
				}
			}
		}
		return m, nil, true
	case "m":
		m.timeline.mouseMode = !m.timeline.mouseMode
		if m.timeline.mouseMode {
			return m, tea.DisableMouse, true
		}
		return m, tea.EnableMouseCellMotion, true
	case "y":
		if m.timeline.pendingY {
			m.timeline.pendingY = false
			if m.timeline.bodyScrollOffset < len(m.timeline.bodyWrappedLines) {
				return m, copyToClipboard(m.timeline.bodyWrappedLines[m.timeline.bodyScrollOffset]), true
			}
		} else if m.timeline.visualMode {
			m.timeline.visualMode = false
			m.timeline.pendingY = false
			start, end := m.timeline.visualStart, m.timeline.visualEnd
			if start > end {
				start, end = end, start
			}
			if end >= len(m.timeline.bodyWrappedLines) {
				end = len(m.timeline.bodyWrappedLines) - 1
			}
			if start < len(m.timeline.bodyWrappedLines) {
				selected := strings.Join(m.timeline.bodyWrappedLines[start:end+1], "\n")
				return m, copyToClipboard(selected), true
			}
		} else {
			m.timeline.pendingY = true
		}
		return m, nil, true
	case "Y":
		m.timeline.visualMode = false
		m.timeline.pendingY = false
		if len(m.timeline.bodyWrappedLines) > 0 {
			return m, copyToClipboard(strings.Join(m.timeline.bodyWrappedLines, "\n")), true
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) handleNavigation(direction int) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.focusedPanel == panelSidebar {
		max := len(flattenTree(m.folderTree)) - 1
		if max < 0 {
			max = 0
		}
		if direction > 0 {
			if m.sidebarCursor < max {
				m.sidebarCursor++
			}
		} else {
			if m.sidebarCursor > 0 {
				m.sidebarCursor--
			}
		}
		return m, nil
	}

	if m.summaryTable.Focused() {
		// Let the table handle navigation properly (including scrolling)
		if direction > 0 {
			m.summaryTable, cmd = m.summaryTable.Update(tea.KeyMsg{Type: tea.KeyDown})
		} else {
			m.summaryTable, cmd = m.summaryTable.Update(tea.KeyMsg{Type: tea.KeyUp})
		}
		// Auto-update details table on navigation
		m.updateDetailsTable()
	} else if m.detailsTable.Focused() {
		// Let the table handle navigation properly (including scrolling)
		if direction > 0 {
			m.detailsTable, cmd = m.detailsTable.Update(tea.KeyMsg{Type: tea.KeyDown})
		} else {
			m.detailsTable, cmd = m.detailsTable.Update(tea.KeyMsg{Type: tea.KeyUp})
		}
	}

	return m, cmd
}
