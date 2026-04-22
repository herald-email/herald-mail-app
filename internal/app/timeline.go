package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// updateTimelineTable rebuilds the timeline table rows from m.timelineEmails,
// grouping them into collapsed threads where appropriate.
func (m *Model) updateTimelineTable() {
	maxSubj := m.timelineSubjectWidth
	if maxSubj <= 0 {
		maxSubj = 40
	}
	maxSend := m.timelineSenderWidth
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
		if m.semanticScores != nil {
			if score, ok := m.semanticScores[email.MessageID]; ok {
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
	case m.chatFilterMode:
		displayEmails = m.chatFilteredEmails
	case m.searchMode && m.searchResults != nil:
		displayEmails = m.searchResults
	default:
		displayEmails = m.timelineEmails
	}

	// Build thread groups from the full email list
	m.threadGroups = buildThreadGroups(displayEmails)
	m.threadRowMap = m.threadRowMap[:0]

	// Sort starred threads to the top, preserving date order within each bucket.
	sort.SliceStable(m.threadGroups, func(i, j int) bool {
		iStarred := len(m.threadGroups[i].emails) > 0 && m.threadGroups[i].emails[0].IsStarred
		jStarred := len(m.threadGroups[j].emails) > 0 && m.threadGroups[j].emails[0].IsStarred
		return iStarred && !jStarred
	})

	var rows []table.Row
	for gi := range m.threadGroups {
		g := &m.threadGroups[gi]
		expanded := m.expandedThreads[g.normalizedSubject]

		if len(g.emails) == 1 {
			// Single-email thread: show as a plain row
			rows = append(rows, emailRow(g.emails[0], ""))
			m.threadRowMap = append(m.threadRowMap, timelineRowRef{
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
			m.threadRowMap = append(m.threadRowMap, timelineRowRef{
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
				m.threadRowMap = append(m.threadRowMap, timelineRowRef{
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
	var tableView string
	if m.timelineEmails != nil && len(m.timelineEmails) == 0 {
		tableView = m.emptyStateView("No emails in this folder  •  press r to refresh")
	} else {
		// Timeline table always gets a bright border on the Timeline tab —
		// it's the primary panel and should always stand out.
		style := m.baseStyle.BorderForeground(defaultTheme.BorderActive)
		tableView = style.Render(renderStyledTableView(&m.timelineTable, 0))
	}

	var mainContent string
	if m.selectedTimelineEmail != nil {
		previewPanel := m.renderEmailPreview()
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, tableView, previewPanel)
	} else {
		mainContent = tableView
	}

	if m.showSidebar && !m.sidebarTooWide {
		sidebarStyle := m.baseStyle
		if m.focusedPanel == panelSidebar {
			sidebarStyle = sidebarStyle.BorderForeground(defaultTheme.BorderActive)
		}
		sidebarView := sidebarStyle.Render(m.renderSidebar())
		return lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, "  ", mainContent)
	}
	return mainContent
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
