package app

import (
	"fmt"
	"net/mail"
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

const (
	threadCollapsedPrefix = "▸ "
	threadExpandedPrefix  = "▾ "
	threadReplyPrefix     = "↩ "
	threadNestedPrefix    = "  ↳ "
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
		if isVirtualAllMailOnlyFolder(folder) {
			view, err := m.backend.GetAllMailOnlyView()
			if err != nil {
				return TimelineLoadedMsg{
					Emails:   []*models.EmailData{},
					Notice:   "All Mail only inspection failed: " + err.Error(),
					ReadOnly: true,
				}
			}
			if view == nil {
				return TimelineLoadedMsg{
					Emails:   []*models.EmailData{},
					Notice:   "All Mail only inspector returned no data",
					ReadOnly: true,
				}
			}
			emails := view.Emails
			if emails == nil {
				emails = []*models.EmailData{}
			}
			return TimelineLoadedMsg{
				Emails:   emails,
				Notice:   view.Reason,
				ReadOnly: true,
			}
		}
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

func isReplySubject(s string) bool {
	replyPrefixes := []string{"re:", "aw:", "tr:"}
	s = strings.TrimSpace(strings.ToLower(s))
	for _, p := range replyPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func senderAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if addr, err := mail.ParseAddress(raw); err == nil && addr.Address != "" {
		return strings.ToLower(strings.TrimSpace(addr.Address))
	}
	start := strings.LastIndex(raw, "<")
	end := strings.LastIndex(raw, ">")
	if start >= 0 && end > start {
		return strings.ToLower(strings.TrimSpace(raw[start+1 : end]))
	}
	return strings.ToLower(raw)
}

func senderDisplayLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "(unknown)"
	}
	if addr, err := mail.ParseAddress(raw); err == nil {
		if name := sanitizeText(addr.Name); name != "" {
			return name
		}
		if addr.Address != "" {
			return addr.Address
		}
	}
	if lt := strings.Index(raw, " <"); lt > 0 {
		if name := sanitizeText(raw[:lt]); name != "" {
			return name
		}
	}
	if cleaned := sanitizeText(raw); cleaned != "" {
		return cleaned
	}
	return raw
}

func threadParticipantLabels(emails []*models.EmailData, fromAddress string) []string {
	labels := make([]string, 0, len(emails))
	seen := make(map[string]bool)
	from := strings.ToLower(strings.TrimSpace(fromAddress))

	for _, email := range emails {
		if email == nil {
			continue
		}
		addr := senderAddress(email.Sender)
		label := senderDisplayLabel(email.Sender)
		key := addr
		if from != "" && addr == from {
			label = "me"
			key = "me"
		}
		if key == "" {
			key = strings.ToLower(label)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		labels = append(labels, label)
	}

	return labels
}

func styledThreadParticipants(labels []string, maxWidth int) string {
	if len(labels) == 0 {
		labels = []string{"(unknown)"}
	}
	joined := truncate(strings.Join(labels, ", "), maxWidth)
	return lipgloss.NewStyle().Foreground(defaultTheme.TextFg).Render(joined)
}

// buildThreadGroups groups emails by normalised subject.
// emails must already be sorted newest-first; group order is determined by each
// group's most-recent email, so groups are also implicitly newest-first.
func buildThreadGroups(emails []*models.EmailData) []threadGroup {
	var groups []threadGroup
	seen := make(map[string]int) // normalised subject → index in groups

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
		if idx, ok := seen[ns]; ok {
			groups[idx].emails = append(groups[idx].emails, e)
		} else {
			seen[ns] = len(groups)
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
			indicatorWidth := len([]rune(unreadDot)) + len([]rune(starDot)) + len([]rune(threadCollapsedPrefix))
			senderAvail := maxSend - indicatorWidth
			if senderAvail < 1 {
				senderAvail = 1
			}
			threadSender := unreadDot + starDot + threadCollapsedPrefix + styledThreadParticipants(threadParticipantLabels(g.emails, m.fromAddress), senderAvail)
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
			// Expanded: mark replies explicitly and keep nested markers for older
			// non-reply rows so conversation shape is visible at a glance.
			for ei, email := range g.emails {
				prefix := ""
				if ei == 0 {
					prefix = threadExpandedPrefix
					if isReplySubject(email.Subject) {
						prefix += threadReplyPrefix
					}
				} else if isReplySubject(email.Subject) {
					prefix = threadReplyPrefix
				} else if ei > 0 {
					prefix = threadNestedPrefix
				}
				rows = append(rows, emailRow(email, prefix))
				m.timeline.threadRowMap = append(m.timeline.threadRowMap, timelineRowRef{
					kind: rowKindEmail, group: g, emailIdx: ei,
				})
			}
		}
	}

	m.timelineTable.SetRows(rows)
	if len(rows) == 0 {
		m.timelineTable.SetCursor(0)
		return
	}
	cursor := m.timelineTable.Cursor()
	if cursor < 0 {
		m.timelineTable.SetCursor(0)
		return
	}
	if cursor >= len(rows) {
		m.timelineTable.SetCursor(len(rows) - 1)
	}
}

// renderTimelineView renders the timeline tab content.
// When an email is selected, it splits into a list on the left and preview on the right.
func (m *Model) renderTimelineView() string {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	chrome := m.chromeState(plan)

	var tableView string
	if m.timeline.emails != nil && len(m.timeline.emails) == 0 {
		notice := "No emails in this folder  •  press r to refresh"
		if m.timelineIsReadOnlyDiagnostic() {
			if m.timeline.virtualNotice != "" {
				notice = m.timeline.virtualNotice
			} else {
				notice = "No messages matched the All Mail only diagnostic"
			}
		}
		tableView = m.emptyStateView(notice)
	} else {
		style := m.baseStyle.BorderForeground(defaultTheme.BorderInactive)
		tableStyles := m.inactiveTableStyle
		if chrome.FocusedPanel == panelTimeline {
			style = style.BorderForeground(defaultTheme.BorderActive)
			tableStyles = m.activeTableStyle
		}
		tableView = style.Render(renderStyledTableViewWithStyles(&m.timelineTable, tableStyles))
	}

	var mainContent string
	if m.timeline.selectedEmail != nil {
		previewPanel := m.renderEmailPreview()
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, tableView, panelGap, previewPanel)
	} else {
		mainContent = tableView
	}

	if plan.SidebarVisible {
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
	m.timeline.searchToken++
	m.timeline.searchMode = false
	m.timeline.searchFocus = timelineSearchFocusInput
	m.timeline.searchAutoFocusResults = false
	m.timeline.searchInput.Blur()
	m.timeline.searchInput.SetValue("")
	m.timeline.searchResults = nil
	m.timeline.searchResultsQuery = ""
	m.timeline.semanticScores = nil
	m.timeline.searchError = ""
	if m.timeline.emailsCache != nil {
		m.timeline.emails = m.timeline.emailsCache
	}
	if origin := m.timeline.searchOrigin; origin != nil {
		m.timeline.expandedThreads = cloneTimelineExpandedThreads(origin.expandedThreads)
		m.timeline.selectedEmail = origin.selectedEmail
		m.timeline.body = origin.body
		m.timeline.bodyMessageID = origin.bodyMessageID
		m.timeline.bodyLoading = origin.bodyLoading
		m.timeline.inlineImageDescs = cloneInlineImageDescs(origin.inlineImageDescs)
		m.timeline.fullScreen = origin.fullScreen
		m.timeline.bodyScrollOffset = origin.bodyScrollOffset
		m.timeline.bodyWrappedLines = nil
		m.timeline.visualMode = false
		m.timeline.pendingY = false
		m.timeline.quickReplyOpen = false
		m.timeline.quickReplyPending = false
		m.timeline.quickReplyIdx = 0
		m.timeline.attachmentSavePrompt = false
		m.timeline.attachmentSaveInput.Blur()
		m.updateTimelineTable()
		maxCursor := len(m.timeline.threadRowMap) - 1
		cursor := origin.cursor
		if maxCursor >= 0 {
			if cursor < 0 {
				cursor = 0
			}
			if cursor > maxCursor {
				cursor = maxCursor
			}
			m.timelineTable.SetCursor(cursor)
		}
		m.setFocusedPanel(origin.focusedPanel)
	} else {
		m.updateTimelineTable()
		m.setFocusedPanel(panelTimeline)
	}
	m.timeline.searchOrigin = nil
	m.timeline.emailsCache = nil
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
}

func (m *Model) clearTimelineQuickReply() {
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyPending = false
	m.timeline.quickReplyIdx = 0
}

func (m *Model) clearTimelineFullScreen() {
	m.timeline.fullScreen = false
	m.timeline.bodyWrappedLines = nil
}

func (m *Model) clearTimelinePreview() {
	m.timeline.selectedEmail = nil
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
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
	if m.timeline.searchOrigin == nil {
		m.timeline.searchOrigin = &timelineSearchOrigin{
			cursor:           m.timelineTable.Cursor(),
			expandedThreads:  cloneTimelineExpandedThreads(m.timeline.expandedThreads),
			focusedPanel:     m.focusedPanel,
			selectedEmail:    m.timeline.selectedEmail,
			body:             m.timeline.body,
			bodyMessageID:    m.timeline.bodyMessageID,
			bodyLoading:      m.timeline.bodyLoading,
			inlineImageDescs: cloneInlineImageDescs(m.timeline.inlineImageDescs),
			fullScreen:       m.timeline.fullScreen,
			bodyScrollOffset: m.timeline.bodyScrollOffset,
		}
	}
	m.timeline.searchToken++
	m.timeline.searchMode = true
	m.timeline.searchFocus = timelineSearchFocusInput
	m.timeline.searchAutoFocusResults = false
	if m.timeline.emailsCache == nil {
		m.timeline.emailsCache = m.timeline.emails
	}
	m.timeline.selectedEmail = nil
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = false
	m.timeline.inlineImageDescs = nil
	m.timeline.fullScreen = false
	m.timeline.bodyWrappedLines = nil
	m.timeline.bodyScrollOffset = 0
	m.timeline.visualMode = false
	m.timeline.pendingY = false
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyPending = false
	m.timeline.quickReplyIdx = 0
	m.timeline.attachmentSavePrompt = false
	m.timeline.attachmentSaveInput.Blur()
	m.timeline.searchInput.SetValue("")
	m.timeline.searchResults = nil
	m.timeline.searchResultsQuery = ""
	m.timeline.semanticScores = nil
	m.timeline.searchError = ""
	m.timeline.searchInput.Focus()
	m.setFocusedPanel(panelTimeline)
	m.updateTimelineTable()
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
}

func (m *Model) openTimelineSemanticSearch() {
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("? ")
	m.timeline.searchError = ""
	m.timeline.searchResultsQuery = ""
}

func (m *Model) currentTimelineRowEmail() *models.EmailData {
	cursor := m.timelineTable.Cursor()
	if cursor < 0 || cursor >= len(m.timeline.threadRowMap) {
		return nil
	}
	ref := m.timeline.threadRowMap[cursor]
	if ref.kind == rowKindThread {
		return ref.group.emails[0]
	}
	return ref.group.emails[ref.emailIdx]
}

func buildForwardSubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "fwd:") {
		return subject
	}
	return "Fwd: " + subject
}

func buildForwardBody(email *models.EmailData, bodyText string) string {
	if email == nil {
		return bodyText
	}
	forwarded := fmt.Sprintf("\n\n--- Forwarded message ---\nFrom: %s\nDate: %s\nSubject: %s\n\n",
		email.Sender, email.Date.Format("Mon, 02 Jan 2006 15:04"), email.Subject)
	return forwarded + bodyText
}

func buildReplySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func newComposePreservedContext(kind models.PreservedMessageKind, email *models.EmailData, body *models.EmailBody, warning string) *composePreservedContext {
	if body == nil {
		body = &models.EmailBody{}
	}
	if body.MessageID == "" && email != nil {
		body.MessageID = email.MessageID
	}
	ctx := &composePreservedContext{
		kind:        kind,
		mode:        models.PreservationModeSafe,
		email:       email,
		body:        body,
		loadWarning: warning,
	}
	if kind == models.PreservedMessageKindForward {
		ctx.forwardedAttachments = make([]models.ForwardedAttachment, 0, len(body.Attachments))
		for _, att := range body.Attachments {
			ctx.forwardedAttachments = append(ctx.forwardedAttachments, models.ForwardedAttachment{
				Attachment: att,
				Include:    len(att.Data) > 0,
			})
		}
	}
	return ctx
}

func (m *Model) timelineBodyLoadedFor(email *models.EmailData) bool {
	return email != nil &&
		m.timeline.body != nil &&
		m.timeline.bodyMessageID == email.MessageID
}

func (m *Model) openTimelineForwardCompose(email *models.EmailData, body *models.EmailBody, composeStatus string) {
	m.activeTab = tabCompose
	m.composeTo.SetValue("")
	m.composeSubject.SetValue(buildForwardSubject(email.Subject))
	m.composeBody.SetValue("")
	m.composeStatus = composeStatus
	m.statusMessage = ""
	m.replyContextEmail = nil
	m.composeAIThread = false
	m.composePreserved = newComposePreservedContext(models.PreservedMessageKindForward, email, body, composeStatus)
	m.composeField = 0
	m.composeTo.Focus()
	m.composeSubject.Blur()
	m.composeBody.Blur()
}

func (m *Model) openTimelineReplyCompose(email *models.EmailData, body *models.EmailBody, composeStatus string) {
	m.activeTab = tabCompose
	m.replyContextEmail = email
	m.composeAIThread = true
	m.composeTo.SetValue(email.Sender)
	m.composeSubject.SetValue(buildReplySubject(email.Subject))
	m.composeBody.SetValue("")
	m.composeStatus = composeStatus
	m.statusMessage = ""
	m.composePreserved = newComposePreservedContext(models.PreservedMessageKindReply, email, body, composeStatus)
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeSubject.Blur()
	m.composeBody.Focus()
}

func (m *Model) startTimelineForward(email *models.EmailData) tea.Cmd {
	if email == nil {
		return nil
	}
	if m.timelineBodyLoadedFor(email) {
		m.openTimelineForwardCompose(email, m.timeline.body, "")
		return nil
	}
	m.timeline.forwardRequestID++
	requestID := m.timeline.forwardRequestID
	m.timeline.forwardPendingMessage = email.MessageID
	m.statusMessage = "Loading forwarded message body..."
	return m.loadTimelineForwardBodyCmd(email, requestID)
}

func (m *Model) loadTimelineForwardBodyCmd(email *models.EmailData, requestID int) tea.Cmd {
	emailCopy := *email
	b := m.backend
	return func() tea.Msg {
		if emailCopy.UID == 0 {
			return TimelineForwardBodyMsg{
				Email: &emailCopy,
				Body: &models.EmailBody{
					TextPlain: "(Body unavailable: this cached email has no server UID yet, so Herald cannot safely load its full contents. Re-sync the folder or use server search to refresh it.)",
				},
				MessageID: emailCopy.MessageID,
				RequestID: requestID,
			}
		}
		body, err := b.FetchEmailBody(emailCopy.Folder, emailCopy.UID)
		return TimelineForwardBodyMsg{
			Email:     &emailCopy,
			Body:      body,
			Err:       err,
			MessageID: emailCopy.MessageID,
			RequestID: requestID,
		}
	}
}

func (m *Model) startTimelineReply(email *models.EmailData) tea.Cmd {
	if email == nil {
		return nil
	}
	if m.timelineBodyLoadedFor(email) {
		m.openTimelineReplyCompose(email, m.timeline.body, "")
		return nil
	}
	m.timeline.replyRequestID++
	requestID := m.timeline.replyRequestID
	m.timeline.replyPendingMessage = email.MessageID
	m.statusMessage = "Loading reply message body..."
	return m.loadTimelineReplyBodyCmd(email, requestID)
}

func (m *Model) loadTimelineReplyBodyCmd(email *models.EmailData, requestID int) tea.Cmd {
	emailCopy := *email
	b := m.backend
	return func() tea.Msg {
		if emailCopy.UID == 0 {
			return TimelineReplyBodyMsg{
				Email: &emailCopy,
				Body: &models.EmailBody{
					TextPlain: "(Body unavailable: this cached email has no server UID yet, so Herald cannot safely load its full contents. Re-sync the folder or use server search to refresh it.)",
				},
				MessageID: emailCopy.MessageID,
				RequestID: requestID,
			}
		}
		body, err := b.FetchEmailBody(emailCopy.Folder, emailCopy.UID)
		return TimelineReplyBodyMsg{
			Email:     &emailCopy,
			Body:      body,
			Err:       err,
			MessageID: emailCopy.MessageID,
			RequestID: requestID,
		}
	}
}

func (m *Model) currentTimelineRowRef() (timelineRowRef, bool) {
	cursor := m.timelineTable.Cursor()
	if cursor < 0 || cursor >= len(m.timeline.threadRowMap) {
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
	return m.openTimelineEmail(email)
}

func (m *Model) openTimelineEmail(email *models.EmailData) tea.Cmd {
	if email == nil {
		return nil
	}
	m.timeline.selectedEmail = email
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = true
	m.timeline.inlineImageDescs = nil
	m.timeline.bodyScrollOffset = 0
	m.timeline.bodyWrappedLines = nil
	m.timeline.quickReplies = nil
	m.timeline.quickRepliesReady = false
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyIdx = 0
	m.timeline.quickRepliesAIFetched = false
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	return m.loadEmailBodyCmd(email.MessageID, email.Folder, email.UID)
}

func (m *Model) openTimelineQuickReply() tea.Cmd {
	if m.timelineIsReadOnlyDiagnostic() {
		return nil
	}
	if m.timeline.selectedEmail == nil || m.timeline.body == nil {
		return nil
	}
	m.timeline.quickReplyPending = false
	m.timeline.quickReplyOpen = true
	m.setFocusedPanel(panelPreview)
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

func (m *Model) toggleTimelineQuickReply() tea.Cmd {
	if m.timelineIsReadOnlyDiagnostic() {
		return nil
	}
	if m.timeline.selectedEmail == nil || m.timeline.body == nil {
		if email := m.currentTimelineRowEmail(); email != nil {
			m.timeline.quickReplyPending = true
			return m.openTimelineEmail(email)
		}
		return nil
	}
	m.timeline.quickReplyOpen = !m.timeline.quickReplyOpen
	if !m.timeline.quickReplyOpen {
		m.timeline.quickReplyPending = false
		return nil
	}
	return m.openTimelineQuickReply()
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
		if m.timelineIsReadOnlyDiagnostic() {
			parts = append(parts, fmt.Sprintf("%d emails", len(m.timeline.emails)))
			parts = append(parts, "diagnostic read-only")
		} else if _, ok := m.folderStatus[m.currentFolder]; !ok {
			parts = append(parts, fmt.Sprintf("%d emails", len(m.timeline.emails)))
		}
	}
	if m.timeline.searchMode {
		if m.timeline.searchFocus == timelineSearchFocusResults {
			parts = append(parts, fmt.Sprintf("Search results: %d", len(m.timeline.searchResults)))
		} else if m.timeline.searchResults != nil {
			parts = append(parts, fmt.Sprintf("Search: %d results", len(m.timeline.searchResults)))
		} else {
			parts = append(parts, "Search input")
		}
	}
	if m.activeTab == tabTimeline && m.timeline.body != nil && !m.timelineIsReadOnlyDiagnostic() {
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
		if m.timeline.searchFocus == timelineSearchFocusResults {
			if m.timeline.selectedEmail != nil && chrome.FocusedPanel == panelPreview {
				return "tab: back to results  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  esc: back to results  │  q: quit", true
			}
			return fmt.Sprintf("/ %s  │  %d results  │  ↑/k ↓/j: results  │  enter: open  │  esc: back to search", q, len(m.timeline.threadRowMap)), true
		}
		if m.timeline.searchError != "" {
			return fmt.Sprintf("/ %s  │  Error: %s  │  esc: back", q, m.timeline.searchError), true
		}
		query := m.timeline.searchInput.Value()
		if !m.timelineIsReadOnlyDiagnostic() && m.timeline.searchResults != nil && len(m.timeline.searchResults) == 0 && query != "" && !strings.HasPrefix(query, "/*") {
			return fmt.Sprintf("/ %s  │  No results in this folder — try: /* %s  │  ctrl+i: server search  │  esc: back", q, query), true
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return fmt.Sprintf("/ %s  │  read-only local search  │  enter: results  │  esc: back", q), true
		}
		return fmt.Sprintf("/ %s  │  current-folder hybrid search  │  enter: results  │  ctrl+i: server search  │  esc: back", q), true
	}
	if m.timeline.chatFilterMode {
		return "esc: clear filter  │  " + primaryTabShortcutHint + "  │  ↑/k ↓/j: navigate  │  enter: open  │  q: quit", true
	}
	if m.timelineIsReadOnlyDiagnostic() && chrome.FocusedPanel == panelPreview {
		return "tab/shift+tab: panels  │  ↑/k ↓/j: scroll  │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  esc: close  │  q: quit", true
	}
	if m.timelineIsReadOnlyDiagnostic() && m.timeline.selectedEmail != nil {
		return "tab/shift+tab: panels  │  ↑/k ↓/j: navigate  │  enter: open  │  esc: close  │  q: quit  │  read-only", true
	}
	if m.timelineIsReadOnlyDiagnostic() {
		return primaryTabShortcutHint + "  │  ↑/k ↓/j: navigate  │  enter: open  │  /: local search  │  f: sidebar  │  q: quit  │  read-only", true
	}
	if chrome.FocusedPanel == panelPreview {
		hasAttachments := m.timeline.body != nil && len(m.timeline.body.Attachments) > 0
		hasMultipleAttachments := m.timeline.body != nil && len(m.timeline.body.Attachments) > 1
		hasUnsub := m.timeline.selectedEmail != nil && m.timeline.bodyMessageID == m.timeline.selectedEmail.MessageID && previewHasUnsubscribe(m.timeline.body)
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
		actionHints := " │  " + previewActionHintText(hasUnsub)
		return "tab/shift+tab: panels  │  ↑/k ↓/j: scroll" + actionHints + attachmentHints + " │  z: full-screen  │  v: visual  │  yy: copy line  │  Y: copy all  │  m: mouse mode  │  esc: close  │  q: quit", true
	}
	if m.timeline.selectedEmail != nil {
		return "tab/shift+tab: panels  │  ↑/k ↓/j: navigate  │  enter: open  │  esc: close  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  A: re-classify  │  q: quit", true
	}
	return primaryTabShortcutHint + "  │  ↑/k ↓/j: navigate  │  enter: open  │  *: star  │  R: reply  │  F: forward  │  D: delete  │  e: archive  │  /: hybrid search  │  A: re-classify  │  f: sidebar  │  q: quit", true
}

func (m *Model) handleTimelineMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case TimelineLoadedMsg:
		m.timeline.emails = msg.Emails
		m.timeline.virtualNotice = msg.Notice
		if msg.ReadOnly {
			m.loading = false
			if isVirtualAllMailOnlyFolder(m.currentFolder) {
				m.hydrateCleanupFromVirtualFolderEmails(msg.Emails)
			}
			unseen := 0
			for _, email := range msg.Emails {
				if email != nil && !email.IsRead {
					unseen++
				}
			}
			m.folderStatus[m.currentFolder] = models.FolderStatus{Unseen: unseen, Total: len(msg.Emails)}
		}
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
		if m.classifier != nil && !msg.ReadOnly {
			cmds := []tea.Cmd{
				m.startEmbeddingBatchIfNeeded(),
				m.startContactEnrichmentIfNeeded(),
			}
			if !m.demoMode {
				cmds = append(cmds, m.startClassificationIfNeeded())
			}
			return m, tea.Batch(cmds...), true
		}
		return m, nil, true

	case EmailBodyMsg:
		if m.contactPreviewLoading {
			return m, nil, false
		}
		if msg.MessageID != "" {
			if m.timeline.selectedEmail == nil || msg.MessageID != m.timeline.selectedEmail.MessageID {
				return m, nil, true
			}
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
			if m.timeline.selectedEmail != nil {
				m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
			}
		} else {
			m.timeline.body = msg.Body
			m.timeline.bodyMessageID = msg.MessageID
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
				if !email.IsRead && !m.timelineIsReadOnlyDiagnostic() {
					email.IsRead = true
					cmds = append(cmds, markReadCmd(m.backend, email.MessageID, email.Folder))
				}
				if body != nil && (body.ListUnsubscribe != "" || body.ListUnsubscribePost != "") && !m.timelineIsReadOnlyDiagnostic() {
					cmds = append(cmds, cacheUnsubscribeHeadersCmd(m.backend, email.MessageID, body.ListUnsubscribe, body.ListUnsubscribePost))
				}
				if body != nil && len(body.InlineImages) > 0 && m.classifier != nil && m.classifier.HasVisionModel() && !iterm2.IsSupported() {
					cmds = append(cmds, describeImagesCmd(m.classifier, body.InlineImages)...)
				}
				m.timeline.quickRepliesReady = true
				if m.timeline.quickReplyPending {
					cmds = append(cmds, m.openTimelineQuickReply())
				}
				if len(cmds) > 0 {
					m.timeline.bodyWrappedLines = nil
					return m, tea.Batch(cmds...), true
				}
			} else {
				m.timeline.quickRepliesReady = true
				if m.timeline.quickReplyPending {
					cmd := m.openTimelineQuickReply()
					m.timeline.bodyWrappedLines = nil
					return m, cmd, true
				}
			}
		}
		m.timeline.bodyWrappedLines = nil
		return m, nil, true

	case QuickRepliesMsg:
		if msg.Err != nil {
			logger.Warn("Quick reply generation failed: %v", msg.Err)
		} else if len(msg.Replies) > 0 {
			for _, reply := range msg.Replies {
				reply = strings.TrimSpace(reply)
				if reply == "" {
					continue
				}
				if !strings.HasPrefix(reply, "[AI] ") {
					reply = "[AI] " + reply
				}
				m.timeline.quickReplies = append(m.timeline.quickReplies, reply)
			}
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
		if m.timeline.searchMode && msg.Token != 0 && msg.Token != m.timeline.searchToken {
			return m, nil, true
		}
		if msg.Err != nil {
			m.timeline.searchError = msg.Err.Error()
			m.timeline.searchAutoFocusResults = false
			return m, nil, true
		}
		m.timeline.searchError = ""
		if msg.Query == "" {
			m.timeline.searchResults = nil
			m.timeline.searchResultsQuery = ""
			m.timeline.semanticScores = nil
			if m.timeline.emailsCache != nil {
				m.timeline.emails = m.timeline.emailsCache
			}
			m.updateTimelineTable()
		} else {
			m.timeline.searchResults = msg.Emails
			m.timeline.searchResultsQuery = msg.Query
			m.timeline.semanticScores = msg.Scores
			m.updateTimelineTable()
			if len(m.timeline.threadRowMap) > 0 {
				m.timelineTable.SetCursor(0)
			}
		}
		if m.timeline.searchAutoFocusResults {
			if len(m.timeline.threadRowMap) > 0 {
				m.timeline.searchFocus = timelineSearchFocusResults
				m.timeline.searchInput.Blur()
				m.setFocusedPanel(panelTimeline)
			}
			m.timeline.searchAutoFocusResults = false
		}
		return m, nil, true

	case TimelineSearchDebounceMsg:
		if !m.timeline.searchMode || m.timeline.searchFocus != timelineSearchFocusInput {
			return m, nil, true
		}
		if msg.Token != m.timeline.searchToken {
			return m, nil, true
		}
		return m, m.performSearchWithToken(msg.Query, msg.Token), true

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

	case TimelineForwardBodyMsg:
		if msg.RequestID != m.timeline.forwardRequestID || msg.MessageID != m.timeline.forwardPendingMessage {
			return m, nil, true
		}
		m.timeline.forwardPendingMessage = ""
		if m.activeTab != tabTimeline {
			return m, nil, true
		}
		email := msg.Email
		if email == nil {
			return m, nil, true
		}
		body := msg.Body
		composeStatus := ""
		if msg.Err != nil {
			logger.Warn("Failed to fetch forwarded email body: %v", msg.Err)
			composeStatus = "Forward body failed to load: " + msg.Err.Error()
			body = &models.EmailBody{TextPlain: "(" + composeStatus + ")"}
		}
		m.openTimelineForwardCompose(email, body, composeStatus)
		return m, nil, true

	case TimelineReplyBodyMsg:
		if msg.RequestID != m.timeline.replyRequestID || msg.MessageID != m.timeline.replyPendingMessage {
			return m, nil, true
		}
		m.timeline.replyPendingMessage = ""
		if m.activeTab != tabTimeline {
			return m, nil, true
		}
		email := msg.Email
		if email == nil {
			return m, nil, true
		}
		body := msg.Body
		composeStatus := ""
		if msg.Err != nil {
			logger.Warn("Failed to fetch reply email body: %v", msg.Err)
			composeStatus = "Reply body failed to load: " + msg.Err.Error()
			body = &models.EmailBody{TextPlain: "(" + composeStatus + ")"}
		}
		m.openTimelineReplyCompose(email, body, composeStatus)
		return m, nil, true

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
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			if email := m.currentTimelineRowEmail(); email != nil {
				return m, m.toggleStarCmd(email), true
			}
		}
		return m, nil, true
	case "u":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.timeline.body != nil && m.timeline.selectedEmail != nil && m.timeline.body.ListUnsubscribe != "" {
			sender := m.timeline.selectedEmail.Sender
			body := m.timeline.body
			m.pendingUnsubscribe = true
			m.pendingUnsubscribeDesc = fmt.Sprintf("Unsubscribe from %s?", sender)
			m.pendingUnsubscribeAction = func() tea.Cmd { return unsubscribeCmd(body) }
		}
		return m, nil, true
	case "h", "H":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.timeline.selectedEmail != nil {
			return m, createHideFutureMailCmd(m.backend, m.timeline.selectedEmail.Sender), true
		}
		return m, nil, true
	case "F":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			if email := m.currentTimelineRowEmail(); email != nil {
				return m, m.startTimelineForward(email), true
			}
		}
		return m, nil, true
	case "/":
		if !m.loading && !m.timeline.searchMode {
			m.openTimelineSearch()
		}
		return m, nil, true
	case "?":
		if !m.loading && !m.timeline.searchMode {
			m.openTimelineSemanticSearch()
		}
		return m, nil, true
	case "enter":
		if m.canInteractWithVisibleData() {
			if m.timeline.searchMode && m.timeline.searchFocus == timelineSearchFocusResults {
				if m.focusedPanel == panelPreview {
					m.setFocusedPanel(panelTimeline)
					return m, nil, true
				}
				return m, m.openCurrentTimelineEmail(), true
			}
			if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 0 {
				model, cmd := m.openQuickReply(m.timeline.quickReplies[m.timeline.quickReplyIdx])
				return model, cmd, true
			}
			if m.focusedPanel == panelSidebar {
				m.selectSidebarFolder()
				m.clearTimelineChatFilter()
				return m, m.activateCurrentFolder(), true
			}
			if m.focusedPanel != panelSidebar {
				return m, m.openCurrentTimelineEmail(), true
			}
		}
		return m, nil, true
	case "tab", "shift+tab":
		if m.canInteractWithVisibleData() && m.timeline.searchMode && m.timeline.searchFocus == timelineSearchFocusResults &&
			m.timeline.selectedEmail != nil && m.focusedPanel == panelPreview {
			m.setFocusedPanel(panelTimeline)
			return m, nil, true
		}
	case "ctrl+q":
		return m, m.toggleTimelineQuickReply(), true
	case "z":
		if !m.loading && m.timeline.selectedEmail != nil {
			m.timeline.fullScreen = !m.timeline.fullScreen
			m.timeline.bodyWrappedLines = nil
		}
		return m, nil, true
	case "s":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
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
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if !m.loading && (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && m.timeline.selectedAttachment < len(m.timeline.body.Attachments)-1 {
			m.timeline.selectedAttachment++
			return m, nil, true
		}
		if m.timeline.body != nil && len(m.timeline.body.Attachments) > 1 {
			return m, nil, true
		}
	case "[":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if !m.loading && (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && m.timeline.selectedAttachment > 0 {
			m.timeline.selectedAttachment--
			return m, nil, true
		}
		if m.timeline.body != nil && len(m.timeline.body.Attachments) > 1 {
			return m, nil, true
		}
	case "R":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if !m.loading {
			if email := m.currentTimelineRowEmail(); email != nil {
				return m, m.startTimelineReply(email), true
			}
		}
		return m, nil, true
	case "up", "k":
		if m.canInteractWithVisibleData() {
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
		if m.canInteractWithVisibleData() {
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
		return m, m.toggleMouseCaptureMode(), true
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

func cloneTimelineExpandedThreads(src map[string]bool) map[string]bool {
	if len(src) == 0 {
		return make(map[string]bool)
	}
	dst := make(map[string]bool, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneInlineImageDescs(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
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
