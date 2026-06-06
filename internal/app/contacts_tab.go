package app

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/contacts"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/memory"
	"github.com/herald-email/herald-mail-app/internal/models"
	emailrender "github.com/herald-email/herald-mail-app/internal/render"
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

type contactMemorySource interface {
	SearchMemories(context.Context, memory.Query) ([]memory.Memory, error)
}

func (m *Model) resetContactMemoryDossier() {
	m.contactMemoryToken++
	m.contactMemoryDossier = memory.Dossier{}
	m.contactCompanyMemoryDossier = memory.Dossier{}
	m.contactMemoryLoading = false
	m.contactMemoryError = ""
}

func (m *Model) loadContactMemoryDossier(contact models.ContactData) tea.Cmd {
	source, ok := m.backend.(contactMemorySource)
	if !ok || source == nil {
		m.resetContactMemoryDossier()
		return nil
	}
	settings := memory.DefaultSettings()
	if m.cfg != nil {
		settings = m.cfg.Memories
	}
	settings.ApplyDefaults()
	if !settings.Enabled {
		m.resetContactMemoryDossier()
		return nil
	}
	m.contactMemoryToken++
	token := m.contactMemoryToken
	m.contactMemoryDossier = memory.Dossier{}
	m.contactCompanyMemoryDossier = memory.Dossier{}
	m.contactMemoryLoading = true
	m.contactMemoryError = ""
	return func() tea.Msg {
		ctx := context.Background()
		memories, err := searchContactDossierMemories(ctx, source, contact, settings)
		companyMemories, companyErr := searchContactCompanyDossierMemories(ctx, source, contact, settings)
		if err == nil {
			err = companyErr
		}
		now := time.Now()
		dossier := memory.BuildPersonDossier(contactDossierSubject(contact), memories, settings, now)
		companyDossier := memory.BuildCompanyDossier(contactCompanyDossierSubject(contact), companyMemories, settings, now)
		return ContactMemoryDossierMsg{
			Token:          token,
			Email:          contact.Email,
			Dossier:        dossier,
			CompanyDossier: companyDossier,
			Err:            err,
		}
	}
}

func searchContactDossierMemories(ctx context.Context, source contactMemorySource, contact models.ContactData, settings memory.Settings) ([]memory.Memory, error) {
	minConfidence := settings.Thresholds.Dossier
	limit := 12
	queries := []memory.Query{}
	people := memory.CompactStrings([]string{contact.Email, contact.DisplayName})
	if len(people) > 0 {
		queries = append(queries, memory.Query{
			People:        people,
			MinConfidence: minConfidence,
			Limit:         limit,
		})
	}
	if strings.TrimSpace(contact.Company) != "" {
		queries = append(queries, memory.Query{
			Company:       contact.Company,
			MinConfidence: minConfidence,
			Limit:         limit,
		})
	}
	if domain := contactEmailDomain(contact.Email); domain != "" {
		queries = append(queries, memory.Query{
			Domain:        domain,
			MinConfidence: minConfidence,
			Limit:         limit,
		})
	}
	if len(queries) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool)
	out := make([]memory.Memory, 0)
	for _, query := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results, err := source.SearchMemories(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, item := range results {
			key := strings.TrimSpace(item.ID)
			if key == "" {
				key = memory.DeterministicID(item)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	memory.SortMemoriesNewestFirst(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func searchContactCompanyDossierMemories(ctx context.Context, source contactMemorySource, contact models.ContactData, settings memory.Settings) ([]memory.Memory, error) {
	minConfidence := settings.Thresholds.Dossier
	limit := 12
	queries := []memory.Query{}
	if strings.TrimSpace(contact.Company) != "" {
		queries = append(queries, memory.Query{
			Company:       contact.Company,
			MinConfidence: minConfidence,
			Limit:         limit,
		})
	}
	if domain := contactEmailDomain(contact.Email); domain != "" {
		queries = append(queries, memory.Query{
			Domain:        domain,
			MinConfidence: minConfidence,
			Limit:         limit,
		})
	}
	if len(queries) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool)
	out := make([]memory.Memory, 0)
	for _, query := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results, err := source.SearchMemories(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, item := range results {
			key := strings.TrimSpace(item.ID)
			if key == "" {
				key = memory.DeterministicID(item)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	memory.SortMemoriesNewestFirst(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func contactDossierSubject(contact models.ContactData) string {
	return firstNonEmptyString(contact.DisplayName, contact.Email, contact.Company)
}

func contactCompanyDossierSubject(contact models.ContactData) string {
	return firstNonEmptyString(contact.Company, contactEmailDomain(contact.Email))
}

func contactEmailDomain(email string) string {
	if at := strings.LastIndex(email, "@"); at >= 0 && at < len(email)-1 {
		return strings.Trim(strings.ToLower(email[at+1:]), " >")
	}
	return ""
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

var importAppleContactsFromSystem = contacts.ImportFromAppleContacts

func (m *Model) importAppleContacts() tea.Cmd {
	return func() tea.Msg {
		if m.demoMode {
			return AppleContactsImportedMsg{Count: 0}
		}
		addrs, err := importAppleContactsFromSystem()
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
func (m *Model) handleContactsKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	key := msg.String()

	// In search mode route printable chars to the search buffer
	if m.contactSearchMode == "keyword" || m.contactSearchMode == "semantic" {
		switch key {
		case "esc":
			m.contactSearchMode = ""
			m.contactSearch = ""
			m.contactsFiltered = m.contactsList
			m.contactsIdx = 0
		case "?":
			if m.contactSearchMode == "keyword" && m.contactSearch == "" {
				m.contactSearchMode = "semantic"
				m.contactSearch = ""
				m.applyContactSearch()
			} else {
				m.contactSearch += key
				m.applyContactSearch()
			}
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
				if m.contactSearchMode == "semantic" && strings.TrimSpace(m.contactSearch) == "" && strings.TrimSpace(key) == "" {
					return m, nil
				}
				m.contactSearch += key
				m.applyContactSearch()
			}
		}
		return m, nil
	}

	key = shortcutKey(msg)
	command, hasCommand := m.scopedCommand("contacts", key)
	if m.contactPreviewEmail != nil {
		switch {
		case key == "esc":
			if m.previewSelection.activeOn(previewSelectionContacts) {
				m.clearPreviewSelection()
				return m, nil
			}
			m.contactPreviewEmail = nil
			m.contactPreviewBody = nil
			m.contactPreviewLoading = false
			m.contactPreviewScroll = 0
			m.clearPreviewSelection()
			return m, nil
		case key == "v":
			m.togglePreviewSelectionForSurface(previewSelectionContacts)
			return m, nil
		case hasCommand && command == CommandPaneLeft:
			if m.previewSelection.activeOn(previewSelectionContacts) {
				m.moveActivePreviewSelection(0, -1)
			}
			return m, nil
		case hasCommand && command == CommandPaneRight:
			if m.previewSelection.activeOn(previewSelectionContacts) {
				m.moveActivePreviewSelection(0, 1)
			}
			return m, nil
		case hasCommand && command == CommandPaneUp:
			if m.previewSelection.activeOn(previewSelectionContacts) {
				m.moveActivePreviewSelection(-1, 0)
			} else if m.contactPreviewScroll > 0 {
				m.contactPreviewScroll--
			}
			return m, nil
		case hasCommand && command == CommandPaneDown:
			if m.previewSelection.activeOn(previewSelectionContacts) {
				m.moveActivePreviewSelection(1, 0)
			} else {
				m.contactPreviewScroll++
			}
			return m, nil
		case key == "y" || key == "Y":
			if cmd, handled := m.handlePreviewCopyKey(previewSelectionContacts, key); handled {
				return m, cmd
			}
			return m, nil
		case hasCommand && command == CommandPreviewPrint:
			model, cmd, _ := m.openPreviewPrintChooser(previewPrintSurfaceContacts)
			return model.(*Model), cmd
		}
	}
	switch {
	case hasCommand && command == CommandHelpSearch:
		m.contactSearchMode = "keyword"
		m.contactSearch = ""
	case key == "?":
		// Plain ? is reserved for global shortcut help; the app-level key
		// router handles it before Contacts sees this branch.
		return m, nil
	case key == "esc":
		if m.previewSelection.activeOn(previewSelectionContacts) {
			m.clearPreviewSelection()
			return m, nil
		}
		// Close inline email preview first, then detail, then search
		if m.contactPreviewEmail != nil {
			m.clearPreviewSelection()
			m.contactPreviewEmail = nil
			m.contactPreviewBody = nil
			m.contactPreviewLoading = false
			m.contactPreviewScroll = 0
			m.clearPreviewSelection()
			return m, nil
		}
		m.contactSearchMode = ""
		m.contactSearch = ""
		m.contactsFiltered = m.contactsList
		m.contactsIdx = 0
		m.contactDetail = nil
		m.contactDetailEmails = nil
		m.resetContactMemoryDossier()
		m.contactFocusPanel = 0
	case key == "tab":
		if m.contactDetail != nil {
			m.contactFocusPanel = 1 - m.contactFocusPanel
		}
	case key == "m":
		return m, m.toggleMouseCaptureMode()
	case key == "y" || key == "Y":
		if cmd, handled := m.handlePreviewCopyKey(previewSelectionContacts, key); handled {
			return m, cmd
		}
		return m, nil
	case hasCommand && command == CommandPaneDown:
		if m.contactFocusPanel == 0 {
			if m.contactsIdx < len(m.contactsFiltered)-1 {
				m.contactsIdx++
			}
		} else {
			if m.contactDetailIdx < len(m.contactDetailEmails)-1 {
				m.contactDetailIdx++
			}
		}
	case hasCommand && command == CommandPaneUp:
		if m.contactFocusPanel == 0 {
			if m.contactsIdx > 0 {
				m.contactsIdx--
			}
		} else {
			if m.contactDetailIdx > 0 {
				m.contactDetailIdx--
			}
		}
	case key == "enter":
		if m.contactFocusPanel == 0 {
			if len(m.contactsFiltered) > 0 && m.contactsIdx < len(m.contactsFiltered) {
				c := m.contactsFiltered[m.contactsIdx]
				m.contactDetail = &c
				m.contactDetailEmails = nil
				m.contactDetailIdx = 0
				cmds := []tea.Cmd{m.loadContactDetail(c)}
				if cmd := m.loadContactMemoryDossier(c); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
		} else {
			// Open selected email inline in the contact detail panel
			if len(m.contactDetailEmails) > 0 && m.contactDetailIdx < len(m.contactDetailEmails) {
				email := m.contactDetailEmails[m.contactDetailIdx]
				m.clearPreviewSelection()
				m.contactPreviewEmail = email
				m.contactPreviewBody = nil
				m.contactPreviewLoading = true
				m.contactPreviewScroll = 0
				m.clearPreviewSelection()
				return m, m.loadEmailBodyForRefCmd(email.MessageRef())
			}
		}
	case key == "e":
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

	// plan.ContentHeight already accounts for title-row tab chrome, the content
	// separator, and status/divider/key-hint rows around the Contacts view.
	// Each panel also adds 2 border lines (top + bottom).
	contentH := plan.ContentHeight

	activeColor := m.theme.Chrome.TabActive.BackgroundColor()
	inactiveColor := m.theme.Focus.PanelBorder.ForegroundColor()

	leftBorderColor := inactiveColor
	if m.contactFocusPanel == 0 {
		leftBorderColor = activeColor
	}
	rightBorderColor := inactiveColor
	if m.contactFocusPanel == 1 {
		rightBorderColor = activeColor
	}

	makePanel := func(borderColor color.Color, w int) lipgloss.Style {
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
		leftSb.WriteString(m.theme.Contacts.KeywordSearch.Style().Render(fmt.Sprintf("/ %s_", m.contactSearch)) + "\n")
	} else if m.contactSearchMode == "semantic" {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(m.theme.Chrome.TitleBar.ForegroundColor()).Render(fmt.Sprintf("? %s_", m.contactSearch)) + "\n")
	} else {
		leftSb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor()).Render(
			fmt.Sprintf("Contacts (%d)", len(m.contactsFiltered))) + "\n")
	}

	if len(m.contactsFiltered) == 0 {
		leftSb.WriteString(lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor()).Render("  No contacts"))
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

			// Bubble Tea v2/Lip Gloss v2 treat Width as the total panel width.
			// Border(2) + PaddingLeft(1) + right padding(1) leaves w-4 content cells.
			innerW := leftW - 4
			if innerW < 1 {
				innerW = 1
			}

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
					ns := lipgloss.NewStyle().Foreground(m.theme.Chrome.TabActive.ForegroundColor()).Background(bg).Bold(true)
					es := m.theme.Contacts.SelectedEmail.Style().Background(bg)
					cs := m.theme.Contacts.SelectedCompany.Style().Background(bg)
					ks := lipgloss.NewStyle().Foreground(m.theme.Chrome.TabActive.ForegroundColor()).Background(bg).Bold(true)
					bs := lipgloss.NewStyle().Background(bg)
					line = ns.Render(dn) + bs.Render(dnPad+"  ") + es.Render(em) + bs.Render(emPad+"  ") + cs.Render(co) + bs.Render(coPad+"  ") + ks.Render(countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
					es := lipgloss.NewStyle().Foreground(m.theme.Text.Muted.ForegroundColor())
					cs := m.theme.Contacts.Company.Style()
					ks := lipgloss.NewStyle().Foreground(m.theme.Focus.PanelBorder.ForegroundColor())
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
					ns := lipgloss.NewStyle().Foreground(m.theme.Chrome.TabActive.ForegroundColor()).Background(bg).Bold(true)
					es := m.theme.Contacts.SelectedEmail.Style().Background(bg)
					ks := lipgloss.NewStyle().Foreground(m.theme.Chrome.TabActive.ForegroundColor()).Background(bg).Bold(true)
					bs := lipgloss.NewStyle().Background(bg)
					line = ns.Render(dn) + bs.Render(dnPad+"  ") + es.Render(em) + bs.Render(emPad+"  ") + ks.Render(countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
					es := lipgloss.NewStyle().Foreground(m.theme.Text.Muted.ForegroundColor())
					ks := lipgloss.NewStyle().Foreground(m.theme.Focus.PanelBorder.ForegroundColor())
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
					s := lipgloss.NewStyle().Foreground(m.theme.Chrome.TabActive.ForegroundColor()).Background(bg).Bold(true)
					line = s.Render(dn + dnPad + "  " + countStr)
				} else {
					ns := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
					ks := lipgloss.NewStyle().Foreground(m.theme.Focus.PanelBorder.ForegroundColor())
					line = ns.Render(dn) + dnPad + "  " + ks.Render(countStr)
				}
			}
			leftSb.WriteString(line + "\n")
		}
	}

	leftPanel := makePanel(leftBorderColor, leftW).Render(leftSb.String())

	// --- Right panel: contact detail ---
	rightRows := m.contactsRightPanelSelectableRows(rightW, contentH)
	rightLines := renderPreviewSelectableRows(m.theme, rightRows, previewSelectionContacts, m.previewSelection, 0)
	var rightSb strings.Builder

	if m.previewSelection.activeOn(previewSelectionContacts) && m.previewSelection.Mouse {
		rightSb.WriteString(strings.Join(rightLines, "\n"))
	} else if m.contactPreviewEmail != nil {
		// Inline email preview within the Contacts tab
		email := m.contactPreviewEmail
		rightInnerW := rightW - 4
		if rightInnerW < 10 {
			rightInnerW = 10
		}
		dimStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
		boldStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor())
		rightSb.WriteString(boldStyle.Render(truncate("From: "+sanitizeText(email.Sender), rightInnerW)) + "\n")
		rightSb.WriteString(dimStyle.Render(truncate("Date: "+email.Date.Format("Mon, 02 Jan 2006 15:04"), rightInnerW)) + "\n")
		rightSb.WriteString(boldStyle.Render(truncate("Subj: "+sanitizeText(email.Subject), rightInnerW)) + "\n")
		rightSb.WriteString(strings.Repeat("─", rightInnerW) + "\n")
		if m.contactPreviewLoading {
			rightSb.WriteString(dimStyle.Render("Loading…"))
		} else if m.contactPreviewBody != nil {
			body := stripInvisibleChars(emailrender.EmailBodyMarkdown(m.contactPreviewBody))
			if body == "" {
				body = "(No text content)"
			}
			lines := renderEmailBodyLines(body, rightInnerW)
			maxLines := contentH - 6 // header(4) + hint(1) + margin
			if maxLines < 1 {
				maxLines = 1
			}
			m.contactPreviewScroll = clampPreviewScrollOffset(m.contactPreviewScroll, len(lines), maxLines)
			if m.previewSelection.activeOn(previewSelectionContacts) {
				m.previewSelection.ensureCursorVisible(&m.contactPreviewScroll, maxLines, len(lines))
			}
			rightSb.WriteString(renderPlainRowsWithSelection(m.theme, lines, m.contactPreviewScroll, maxLines, m.previewSelection, previewSelectionContacts))
		}
		rightSb.WriteString("\n" + dimStyle.Render(" Esc: back to contact"))
	} else if m.contactDetail == nil {
		rightSb.WriteString(lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor()).
			Render("  Select a contact and press Enter"))
	} else {
		c := m.contactDetail
		boldStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor())
		dimStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
		normalStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
		rightInnerW := rightW - 4
		if rightInnerW < 1 {
			rightInnerW = 1
		}

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
		if dossierLines := m.contactMemoryDossierLines(rightInnerW, contactMemoryDossierMaxLines(contentH)); len(dossierLines) > 0 {
			rightSb.WriteString(strings.Join(dossierLines, "\n") + "\n\n")
		}
		rightSb.WriteString(boldStyle.Render("Recent Emails") + "\n")

		if len(m.contactDetailEmails) == 0 {
			rightSb.WriteString(dimStyle.Render("  Loading…") + "\n")
		} else {
			// Line = "  "(2) + subj(maxSubjW) + "  "(2) + date(10) = maxSubjW+14.
			maxSubjW := rightInnerW - 14
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
						Foreground(m.theme.Chrome.TabActive.ForegroundColor()).
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

func contactMemoryDossierMaxLines(contentH int) int {
	switch {
	case contentH >= 24:
		return 10
	case contentH >= 17:
		return 5
	case contentH >= 13:
		return 3
	default:
		return 0
	}
}

func (m *Model) contactMemoryDossierLines(width, maxLines int) []string {
	if maxLines <= 0 || width <= 0 {
		return nil
	}
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor())
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
	normalStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
	if m.contactMemoryLoading {
		return []string{
			boldStyle.Render(truncateVisual("Herald Memories", width)),
			dimStyle.Render(truncateVisual("  Loading memories...", width)),
		}
	}
	if m.contactMemoryError != "" {
		return []string{
			boldStyle.Render(truncateVisual("Herald Memories", width)),
			dimStyle.Render(truncateVisual("  Memories unavailable: "+m.contactMemoryError, width)),
		}
	}
	dossier := m.contactMemoryDossier
	companyDossier := m.contactCompanyMemoryDossier
	if !contactMemoryDossierHasContent(dossier) && !contactMemoryDossierHasContent(companyDossier) {
		return nil
	}
	lines := []string{boldStyle.Render(truncateVisual("Herald Memories", width))}
	if dossier.RelationshipSummary != "" {
		lines = append(lines, normalStyle.Render(truncateVisual("  "+dossier.RelationshipSummary, width)))
	}
	if len(dossier.ActiveTracks) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Track: "+contactTrackLine(dossier.ActiveTracks[0]), width)))
	}
	if len(dossier.OpenLoops) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Open loop: "+contactMemorySummary(dossier.OpenLoops[0]), width)))
	}
	if len(dossier.VaultLinks) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Vault: "+dossier.VaultLinks[0], width)))
	}
	if len(dossier.Evidence) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Evidence: "+nudgeEvidenceLabel(dossier.Evidence[0]), width)))
	}
	if contactMemoryDossierHasContent(companyDossier) {
		if companyDossier.Subject != "" {
			lines = append(lines, normalStyle.Render(truncateVisual("  Company: "+companyDossier.Subject, width)))
		}
		if len(companyDossier.ActiveTracks) > 0 {
			lines = append(lines, dimStyle.Render(truncateVisual("  Company track: "+contactTrackLine(companyDossier.ActiveTracks[0]), width)))
		}
		if len(companyDossier.VaultLinks) > 0 {
			lines = append(lines, dimStyle.Render(truncateVisual("  Company vault: "+companyDossier.VaultLinks[0], width)))
		}
		if len(companyDossier.Evidence) > 0 {
			lines = append(lines, dimStyle.Render(truncateVisual("  Company evidence: "+nudgeEvidenceLabel(companyDossier.Evidence[0]), width)))
		}
	}
	if len(lines) > maxLines {
		return lines[:maxLines]
	}
	return lines
}

func contactMemoryDossierHasContent(dossier memory.Dossier) bool {
	return strings.TrimSpace(dossier.RelationshipSummary) != "" ||
		len(dossier.RecentInteractions) > 0 ||
		len(dossier.ActiveTracks) > 0 ||
		len(dossier.OpenLoops) > 0 ||
		len(dossier.VaultLinks) > 0 ||
		len(dossier.ResearchNotes) > 0 ||
		len(dossier.Evidence) > 0
}

func contactTrackLine(track memory.Track) string {
	parts := memory.CompactStrings([]string{
		track.Topic,
		track.Status,
		firstNonEmptyString(track.Company, track.Domain),
	})
	return strings.Join(parts, " - ")
}

func contactMemorySummary(mem memory.Memory) string {
	return firstNonEmptyString(mem.Summary, mem.Claim, mem.Topic, mem.Kind)
}
