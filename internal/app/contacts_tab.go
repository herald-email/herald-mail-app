package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/ai"
	"mail-processor/internal/contacts"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	emailrender "mail-processor/internal/render"
)

// --- Contacts tab ---

// loadContacts returns a Cmd that fetches all contacts from the backend.
func (m *Model) loadContacts() tea.Cmd {
	return func() tea.Msg {
		contacts, err := m.backend.ListContacts(200, "last_seen")
		if err != nil {
			logger.Warn("loadContacts: %v", err)
			return ContactsLoadedMsg{}
		}
		return ContactsLoadedMsg{Contacts: contacts}
	}
}

// loadContactDetail returns a Cmd that fetches recent emails for the given contact.
func (m *Model) loadContactDetail(contact models.ContactData) tea.Cmd {
	return func() tea.Msg {
		emails, err := m.backend.GetContactEmails(contact.Email, 20)
		if err != nil {
			logger.Warn("loadContactDetail: %v", err)
			return ContactDetailLoadedMsg{}
		}
		return ContactDetailLoadedMsg{Emails: emails}
	}
}

// applyContactSearch filters contactsFiltered based on the current search query and mode.
func (m *Model) applyContactSearch() {
	if m.contactSearch == "" {
		m.contactsFiltered = m.contactsList
		m.contactsIdx = 0
		return
	}
	if m.contactSearchMode == "keyword" {
		q := strings.ToLower(m.contactSearch)
		var out []models.ContactData
		for _, c := range m.contactsList {
			if strings.Contains(strings.ToLower(c.DisplayName), q) ||
				strings.Contains(strings.ToLower(c.Email), q) ||
				strings.Contains(strings.ToLower(c.Company), q) {
				out = append(out, c)
			}
		}
		m.contactsFiltered = out
	} else {
		// semantic: search via backend; fall back to keyword if classifier unavailable
		var out []models.ContactData
		if m.classifier != nil {
			vec, embErr := ai.WithTaskKind(m.classifier, ai.TaskKindSemanticSearch).Embed(m.contactSearch)
			if embErr == nil {
				results, err := m.backend.SearchContactsSemantic(vec, 50, 0.3)
				if err != nil {
					logger.Warn("applyContactSearch semantic: %v", err)
				} else {
					for _, r := range results {
						out = append(out, r.Contact)
					}
				}
			}
		}
		if len(out) == 0 {
			q := strings.ToLower(m.contactSearch)
			for _, c := range m.contactsList {
				if strings.Contains(strings.ToLower(c.DisplayName), q) ||
					strings.Contains(strings.ToLower(c.Email), q) ||
					strings.Contains(strings.ToLower(c.Company), q) {
					out = append(out, c)
				}
			}
		}
		m.contactsFiltered = out
	}
	m.contactsIdx = 0
}

// runSingleContactEnrichment enriches one specific contact by email address.
func (m *Model) runSingleContactEnrichment(contact models.ContactData) tea.Cmd {
	return func() tea.Msg {
		if m.classifier == nil {
			return ContactEnrichedMsg{Count: 0}
		}
		subjects, err := m.backend.GetRecentSubjectsByContact(contact.Email, 10)
		if err != nil {
			logger.Warn("runSingleContactEnrichment: GetRecentSubjectsByContact %s: %v", contact.Email, err)
			return ContactEnrichedMsg{Count: 0}
		}
		company, topics, err := ai.WithTaskKind(m.classifier, ai.TaskKindContactEnrich).EnrichContact(contact.Email, subjects)
		if err != nil {
			logger.Warn("runSingleContactEnrichment: EnrichContact %s: %v", contact.Email, err)
			return ContactEnrichedMsg{Count: 0}
		}
		if err := m.backend.UpdateContactEnrichment(contact.Email, company, topics); err != nil {
			logger.Warn("runSingleContactEnrichment: UpdateContactEnrichment %s: %v", contact.Email, err)
			return ContactEnrichedMsg{Count: 0}
		}
		return ContactEnrichedMsg{Count: 1}
	}
}

func (m *Model) importAppleContacts() tea.Cmd {
	return func() tea.Msg {
		addrs, err := contacts.ImportFromAppleContacts()
		if err != nil || len(addrs) == 0 {
			return AppleContactsImportedMsg{Count: 0}
		}
		if err := m.backend.UpsertContacts(addrs, "from"); err != nil {
			logger.Warn("Apple Contacts import: %v", err)
			return AppleContactsImportedMsg{Count: 0}
		}
		return AppleContactsImportedMsg{Count: len(addrs)}
	}
}

// handleContactsKey handles key events for the Contacts tab.
func (m *Model) handleContactsKey(msg tea.KeyMsg) (*Model, tea.Cmd) {
	key := msg.String()

	// In search mode route printable chars to the search buffer
	if m.contactSearchMode == "keyword" || m.contactSearchMode == "semantic" {
		switch key {
		case "esc":
			m.contactSearchMode = ""
			m.contactSearch = ""
			m.contactsFiltered = m.contactsList
			m.contactsIdx = 0
		case "backspace", "ctrl+h":
			runes := []rune(m.contactSearch)
			if len(runes) > 0 {
				m.contactSearch = string(runes[:len(runes)-1])
			}
			m.applyContactSearch()
		case "enter":
			m.contactSearchMode = "" // confirm; keep results
		default:
			if len(key) == 1 {
				m.contactSearch += key
				m.applyContactSearch()
			}
		}
		return m, nil
	}

	switch key {
	case "/":
		m.contactSearchMode = "keyword"
		m.contactSearch = ""
	case "?":
		m.contactSearchMode = "semantic"
		m.contactSearch = ""
	case "esc":
		// Close inline email preview first, then detail, then search
		if m.contactPreviewEmail != nil {
			m.contactPreviewEmail = nil
			m.contactPreviewBody = nil
			m.contactPreviewLoading = false
			return m, nil
		}
		m.contactSearchMode = ""
		m.contactSearch = ""
		m.contactsFiltered = m.contactsList
		m.contactsIdx = 0
		m.contactDetail = nil
		m.contactDetailEmails = nil
		m.contactFocusPanel = 0
	case "tab":
		if m.contactDetail != nil {
			m.contactFocusPanel = 1 - m.contactFocusPanel
		}
	case "j", "down":
		if m.contactFocusPanel == 0 {
			if m.contactsIdx < len(m.contactsFiltered)-1 {
				m.contactsIdx++
			}
		} else {
			if m.contactDetailIdx < len(m.contactDetailEmails)-1 {
				m.contactDetailIdx++
			}
		}
	case "k", "up":
		if m.contactFocusPanel == 0 {
			if m.contactsIdx > 0 {
				m.contactsIdx--
			}
		} else {
			if m.contactDetailIdx > 0 {
				m.contactDetailIdx--
			}
		}
	case "enter":
		if m.contactFocusPanel == 0 {
			if len(m.contactsFiltered) > 0 && m.contactsIdx < len(m.contactsFiltered) {
				c := m.contactsFiltered[m.contactsIdx]
				m.contactDetail = &c
				m.contactDetailEmails = nil
				m.contactDetailIdx = 0
				return m, m.loadContactDetail(c)
			}
		} else {
			// Open selected email inline in the contact detail panel
			if len(m.contactDetailEmails) > 0 && m.contactDetailIdx < len(m.contactDetailEmails) {
				email := m.contactDetailEmails[m.contactDetailIdx]
				m.contactPreviewEmail = email
				m.contactPreviewBody = nil
				m.contactPreviewLoading = true
				return m, m.loadEmailBodyCmd(email.MessageID, email.Folder, email.UID)
			}
		}
	case "e":
		var target *models.ContactData
		if m.contactFocusPanel == 1 && m.contactDetail != nil {
			target = m.contactDetail
		} else if len(m.contactsFiltered) > 0 && m.contactsIdx < len(m.contactsFiltered) {
			c := m.contactsFiltered[m.contactsIdx]
			target = &c
		}
		if target != nil {
			return m, m.runSingleContactEnrichment(*target)
		}
		return m, nil
	}
	return m, nil
}

// renderContactsTab renders the two-panel Contacts tab (left: list, right: detail).
func (m *Model) renderContactsTab(width, height int) string {
	if width < 20 {
		return "Terminal too narrow"
	}

	plan := m.buildLayoutPlan(width, height)
	leftW := plan.Contacts.ListWidth
	rightW := plan.Contacts.DetailWidth

	// height is the full terminal height. renderMainView adds chrome around us:
	// header(1) + tab bar(1) + blank(1) + "\n" after content(1) + status bar(1) + key hints(1) = 6.
	// Each panel also adds 2 border lines (top + bottom), so total deduction = 8.
	contentH := plan.ContentHeight

	activeColor := defaultTheme.TabActiveBg
	inactiveColor := defaultTheme.BorderInactive

	leftBorderColor := inactiveColor
	if m.contactFocusPanel == 0 {
		leftBorderColor = activeColor
	}
	rightBorderColor := inactiveColor
	if m.contactFocusPanel == 1 {
		rightBorderColor = activeColor
	}

	makePanel := func(borderColor lipgloss.Color, w int) lipgloss.Style {
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Width(w).
			Height(contentH).
			PaddingLeft(1)
	}

	// --- Left panel: contact list ---
	var leftSb strings.Builder

	if m.contactSearchMode == "keyword" {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render(fmt.Sprintf("/ %s_", m.contactSearch)) + "\n")
	} else if m.contactSearchMode == "semantic" {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(defaultTheme.HeaderFg).Render(fmt.Sprintf("? %s_", m.contactSearch)) + "\n")
	} else {
		leftSb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(defaultTheme.HeaderFg).Render(
			fmt.Sprintf("Contacts (%d)", len(m.contactsFiltered))) + "\n")
	}

	if len(m.contactsFiltered) == 0 {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(defaultTheme.DimFg).Render("  No contacts"))
	} else {
		maxRows := contentH - 3
		if maxRows < 1 {
			maxRows = 1
		}
		start := 0
		if m.contactsIdx >= maxRows {
			start = m.contactsIdx - maxRows + 1
		}
		end := start + maxRows
		if end > len(m.contactsFiltered) {
			end = len(m.contactsFiltered)
		}
		for i := start; i < end; i++ {
			c := m.contactsFiltered[i]

			// Inner content width = Width(leftW) - PaddingLeft(1) = leftW-1.
			innerW := leftW - 1

			// Progressive column layout based on available width:
			// Wide (>=60): Name | Email | Company | Count
			// Medium (>=35): Name | Email | Count
			// Narrow (<35): Name | Count
			countW := 4
			showEmail := innerW >= 35
			showCompany := innerW >= 60

			displayName := c.DisplayName
			if displayName == "" {
				displayName = c.Email
			}
			countStr := fmt.Sprintf("%d", c.EmailCount)

			var line string
			if showCompany {
				companyW := 14
				separators := 6 // 3 x "  "
				nameW := (innerW - separators - countW - companyW) * 55 / 100
				if nameW < 8 {
					nameW = 8
				}
				emailW := innerW - separators - countW - companyW - nameW
				if emailW < 6 {
					emailW = 6
				}
				dn := ansi.Truncate(displayName, nameW, "…")
				em := ansi.Truncate(c.Email, emailW, "…")
				co := ""
				if c.Company != "" {
					co = ansi.Truncate(c.Company, companyW, "…")
				}
				dnPad := strings.Repeat(" ", nameW-ansi.StringWidth(dn))
				emPad := strings.Repeat(" ", emailW-ansi.StringWidth(em))
				coPad := strings.Repeat(" ", companyW-ansi.StringWidth(co))
				if i == m.contactsIdx {
					bg := activeColor
					ns := lipgloss.NewStyle().Foreground(defaultTheme.TabActiveFg).Background(bg).Bold(true)
					es := lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Background(bg)
					cs := lipgloss.NewStyle().Foreground(lipgloss.Color("223")).Background(bg)
					ks := lipgloss.NewStyle().Foreground(defaultTheme.TabActiveFg).Background(bg).Bold(true)
					bs := lipgloss.NewStyle().Background(bg)
					line = ns.Render(dn) + bs.Render(dnPad+"  ") + es.Render(em) + bs.Render(emPad+"  ") + cs.Render(co) + bs.Render(coPad+"  ") + ks.Render(countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(defaultTheme.TextFg)
					es := lipgloss.NewStyle().Foreground(defaultTheme.MutedFg)
					cs := lipgloss.NewStyle().Foreground(lipgloss.Color("249"))
					ks := lipgloss.NewStyle().Foreground(defaultTheme.BorderInactive)
					line = ns.Render(dn) + dnPad + "  " + es.Render(em) + emPad + "  " + cs.Render(co) + coPad + "  " + ks.Render(countStr)
				}
			} else if showEmail {
				separators := 4 // 2 x "  "
				nameW := (innerW - separators - countW) * 45 / 100
				if nameW < 8 {
					nameW = 8
				}
				emailW := innerW - separators - countW - nameW
				if emailW < 6 {
					emailW = 6
				}
				dn := ansi.Truncate(displayName, nameW, "…")
				em := ansi.Truncate(c.Email, emailW, "…")
				dnPad := strings.Repeat(" ", nameW-ansi.StringWidth(dn))
				emPad := strings.Repeat(" ", emailW-ansi.StringWidth(em))
				if i == m.contactsIdx {
					bg := activeColor
					ns := lipgloss.NewStyle().Foreground(defaultTheme.TabActiveFg).Background(bg).Bold(true)
					es := lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Background(bg)
					ks := lipgloss.NewStyle().Foreground(defaultTheme.TabActiveFg).Background(bg).Bold(true)
					bs := lipgloss.NewStyle().Background(bg)
					line = ns.Render(dn) + bs.Render(dnPad+"  ") + es.Render(em) + bs.Render(emPad+"  ") + ks.Render(countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(defaultTheme.TextFg)
					es := lipgloss.NewStyle().Foreground(defaultTheme.MutedFg)
					ks := lipgloss.NewStyle().Foreground(defaultTheme.BorderInactive)
					line = ns.Render(dn) + dnPad + "  " + es.Render(em) + emPad + "  " + ks.Render(countStr)
				}
			} else {
				// Narrow: just name + count
				nameW := innerW - 2 - countW
				if nameW < 4 {
					nameW = 4
				}
				dn := ansi.Truncate(displayName, nameW, "…")
				dnPad := strings.Repeat(" ", nameW-ansi.StringWidth(dn))
				if i == m.contactsIdx {
					bg := activeColor
					s := lipgloss.NewStyle().Foreground(defaultTheme.TabActiveFg).Background(bg).Bold(true)
					line = s.Render(dn + dnPad + "  " + countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(defaultTheme.TextFg)
					ks := lipgloss.NewStyle().Foreground(defaultTheme.BorderInactive)
					line = ns.Render(dn) + dnPad + "  " + ks.Render(countStr)
				}
			}
			leftSb.WriteString(line + "\n")
		}
	}

	leftPanel := makePanel(leftBorderColor, leftW).Render(leftSb.String())

	// --- Right panel: contact detail ---
	var rightSb strings.Builder

	if m.contactPreviewEmail != nil {
		// Inline email preview within the Contacts tab
		email := m.contactPreviewEmail
		dimStyle := lipgloss.NewStyle().Foreground(defaultTheme.DimFg)
		boldStyle := lipgloss.NewStyle().Bold(true).Foreground(defaultTheme.HeaderFg)
		rightSb.WriteString(boldStyle.Render(truncate("From: "+sanitizeText(email.Sender), rightW-1)) + "\n")
		rightSb.WriteString(dimStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), rightW-1)) + "\n")
		rightSb.WriteString(boldStyle.Render(truncate("Subj: "+sanitizeText(email.Subject), rightW-1)) + "\n")
		rightSb.WriteString(strings.Repeat("─", rightW-1) + "\n")
		if m.contactPreviewLoading {
			rightSb.WriteString(dimStyle.Render("Loading…"))
		} else if m.contactPreviewBody != nil {
			body := stripInvisibleChars(emailrender.EmailBodyMarkdown(m.contactPreviewBody))
			if body == "" {
				body = "(No text content)"
			}
			innerW := rightW - 1
			if innerW < 10 {
				innerW = 10
			}
			lines := renderEmailBodyLines(body, innerW)
			maxLines := contentH - 6 // header(4) + hint(1) + margin
			if maxLines < 1 {
				maxLines = 1
			}
			if len(lines) > maxLines {
				lines = lines[:maxLines]
			}
			rightSb.WriteString(strings.Join(lines, "\n"))
			// Pad to push hint to bottom
			for i := len(lines); i < maxLines; i++ {
				rightSb.WriteString("\n")
			}
		}
		rightSb.WriteString("\n" + dimStyle.Render(" Esc: back to contact"))
	} else if m.contactDetail == nil {
		rightSb.WriteString(lipgloss.NewStyle().Foreground(defaultTheme.DimFg).
			Render("  Select a contact and press Enter"))
	} else {
		c := m.contactDetail
		boldStyle := lipgloss.NewStyle().Bold(true).Foreground(defaultTheme.HeaderFg)
		dimStyle := lipgloss.NewStyle().Foreground(defaultTheme.DimFg)
		normalStyle := lipgloss.NewStyle().Foreground(defaultTheme.TextFg)

		displayName := c.DisplayName
		if displayName == "" {
			displayName = c.Email
		}
		rightSb.WriteString(boldStyle.Render(displayName) + "\n")
		rightSb.WriteString(dimStyle.Render(c.Email) + "\n")
		if c.Company != "" {
			rightSb.WriteString(normalStyle.Render("Company: "+c.Company) + "\n")
		}
		if len(c.Topics) > 0 {
			rightSb.WriteString(normalStyle.Render("Topics: "+strings.Join(c.Topics, ", ")) + "\n")
		}

		firstStr := "—"
		lastStr := "—"
		if !c.FirstSeen.IsZero() {
			firstStr = c.FirstSeen.Format("2006-01-02")
		}
		if !c.LastSeen.IsZero() {
			lastStr = c.LastSeen.Format("2006-01-02")
		}
		stats := fmt.Sprintf("First seen: %s  Last seen: %s  Received: %d  Sent: %d",
			firstStr, lastStr, c.EmailCount, c.SentCount)
		rightSb.WriteString(dimStyle.Render(stats) + "\n")

		if c.EnrichedAt != nil {
			rightSb.WriteString(dimStyle.Render("Enriched: "+c.EnrichedAt.Format("2006-01-02")) + "\n")
		}

		rightSb.WriteString("\n")
		rightSb.WriteString(boldStyle.Render("Recent Emails") + "\n")

		if len(m.contactDetailEmails) == 0 {
			rightSb.WriteString(dimStyle.Render("  Loading…") + "\n")
		} else {
			// Inner content = Width(rightW) - PaddingLeft(1) = rightW-1.
			// Line = "  "(2) + subj(maxSubjW) + "  "(2) + date(10) + " "(1 right padding) = maxSubjW+15.
			// To fit: maxSubjW+15 <= rightW-1 → maxSubjW = rightW-16.
			maxSubjW := rightW - 16
			if maxSubjW < 10 {
				maxSubjW = 10
			}
			for i, e := range m.contactDetailEmails {
				subj := ansi.Truncate(e.Subject, maxSubjW, "…")
				subjPad := strings.Repeat(" ", maxSubjW-ansi.StringWidth(subj))
				line := "  " + subj + subjPad + "  " + e.Date.Format("2006-01-02")
				rowStyle := normalStyle
				if m.contactFocusPanel == 1 && i == m.contactDetailIdx {
					rowStyle = lipgloss.NewStyle().
						Foreground(defaultTheme.TabActiveFg).
						Background(activeColor).
						Bold(true)
				}
				rightSb.WriteString(rowStyle.Render(line) + "\n")
			}
		}
	}

	rightPanel := makePanel(rightBorderColor, rightW).Render(rightSb.String())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, panelGap, rightPanel)
}
