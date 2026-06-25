package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const accountSwitcherMaxWidth = 76

func (m *Model) syncAccountsFromBackend() {
	m.syncAccountsFromBackendWithStatus(true)
}

func (m *Model) selectStartupAccountScope() {
	aware, ok := m.backend.(backend.AccountAwareBackend)
	if !ok || !aware.HasMultipleAccounts() {
		return
	}
	if err := aware.SwitchAccount(backend.AllAccountsSourceID); err != nil {
		m.statusMessage = "Account default failed: " + err.Error()
		return
	}
	if m.accountSelectedFolders == nil {
		m.accountSelectedFolders = make(map[models.SourceID]string)
	}
	m.accountSelectedFolders[backend.AllAccountsSourceID] = "INBOX"
	m.currentFolder = "INBOX"
	m.syncAccountIdentityFromBackend()
}

func (m *Model) syncAccountIdentityFromBackend() {
	m.syncAccountsFromBackendWithStatus(false)
}

func (m *Model) syncAccountsFromBackendWithStatus(refreshStatuses bool) {
	aware, ok := m.backend.(backend.AccountAwareBackend)
	if !ok {
		m.accounts = nil
		m.accountStatuses = nil
		m.activeSourceID = ""
		m.activeAccountID = ""
		return
	}
	m.accounts = aware.Accounts()
	active := aware.ActiveAccount()
	m.activeSourceID = active.SourceID
	m.activeAccountID = active.AccountID
	if m.accountSelectedFolders == nil {
		m.accountSelectedFolders = make(map[models.SourceID]string)
	}
	if m.activeSourceID != "" && strings.TrimSpace(m.accountSelectedFolders[m.activeSourceID]) == "" {
		if refreshStatuses && strings.TrimSpace(m.currentFolder) != "" {
			m.accountSelectedFolders[m.activeSourceID] = m.currentFolder
		} else {
			m.accountSelectedFolders[m.activeSourceID] = "INBOX"
		}
	}
	if refreshStatuses {
		m.accountStatuses = aware.AccountStatuses()
	} else if m.accountStatuses == nil {
		m.accountStatuses = make(map[models.SourceID]backend.AccountStatus)
	}
	if len(m.accounts) == 0 {
		m.showAccountSwitcher = false
		m.accountSwitcherCursor = 0
		return
	}
	if m.accountSwitcherCursor < 0 {
		m.accountSwitcherCursor = 0
	}
	switchable := m.accountSwitcherAccounts()
	if m.accountSwitcherCursor >= len(switchable) {
		m.accountSwitcherCursor = len(switchable) - 1
	}
}

func (m *Model) hasMultipleAccounts() bool {
	return len(m.accounts) > 1
}

func (m *Model) allAccountsInfo() backend.AccountInfo {
	return backend.AccountInfo{
		SourceID:    backend.AllAccountsSourceID,
		AccountID:   models.AccountID("all"),
		DisplayName: "All Accounts",
		Provider:    "virtual",
	}
}

func (m *Model) accountSwitcherAccounts() []backend.AccountInfo {
	if !m.hasMultipleAccounts() {
		return m.accounts
	}
	out := make([]backend.AccountInfo, 0, len(m.accounts)+1)
	out = append(out, m.allAccountsInfo())
	out = append(out, m.accounts...)
	return out
}

func (m *Model) allAccountsScopeActive() bool {
	return m.hasMultipleAccounts() && m.activeSourceID == backend.AllAccountsSourceID
}

func (m *Model) scopedResultMatchesActive(sourceID models.SourceID) bool {
	if sourceID == "" || m.activeSourceID == "" {
		return true
	}
	return sourceID == m.activeSourceID
}

func (m *Model) syncEventMatchesActive(sourceID models.SourceID) bool {
	if sourceID == "" || m.activeSourceID == "" {
		return true
	}
	if m.activeSourceID == backend.AllAccountsSourceID {
		return true
	}
	return sourceID == m.activeSourceID
}

func (m *Model) syncGenerationScope(sourceID models.SourceID) models.SourceID {
	if sourceID != "" {
		return sourceID
	}
	if m.activeSourceID != "" {
		return m.activeSourceID
	}
	return models.DefaultMailSourceID
}

func (m *Model) syncGenerationForSource(sourceID models.SourceID) int64 {
	key := m.syncGenerationScope(sourceID)
	if m.syncSourceGenerations != nil {
		if generation, ok := m.syncSourceGenerations[key]; ok {
			return generation
		}
	}
	if m.allAccountsScopeActive() && sourceID != "" {
		return 0
	}
	return m.syncGeneration
}

func (m *Model) setSyncGenerationForSource(sourceID models.SourceID, generation int64) {
	if generation <= 0 {
		return
	}
	if m.syncSourceGenerations == nil {
		m.syncSourceGenerations = make(map[models.SourceID]int64)
	}
	key := m.syncGenerationScope(sourceID)
	if generation > m.syncSourceGenerations[key] {
		m.syncSourceGenerations[key] = generation
	}
	if generation > m.syncGeneration {
		m.syncGeneration = generation
	}
}

func (m *Model) syncHydratedGenerationMatches(sourceID models.SourceID, generation int64) bool {
	if generation == 0 {
		return true
	}
	if m.allAccountsScopeActive() && sourceID != "" {
		return generation == m.syncGenerationForSource(sourceID)
	}
	return generation == m.syncGeneration
}

func (m *Model) activeAccountInfo() backend.AccountInfo {
	if !m.hasMultipleAccounts() {
		return backend.AccountInfo{}
	}
	if m.activeSourceID == backend.AllAccountsSourceID {
		return m.allAccountsInfo()
	}
	for _, account := range m.accounts {
		if account.SourceID == m.activeSourceID {
			return account
		}
	}
	if len(m.accounts) > 0 {
		return m.accounts[0]
	}
	return backend.AccountInfo{}
}

func (m *Model) activeAccountLabel() string {
	account := m.activeAccountInfo()
	return strings.TrimSpace(account.DisplayName)
}

func (m *Model) accountSwitcherShortcutAvailable() bool {
	if !m.hasMultipleAccounts() || m.showSettings || m.showRuleEditor || m.showPromptEditor || m.showCleanupMgr || m.showHelp {
		return false
	}
	if m.activeTab == tabCompose {
		return false
	}
	if m.activeTab == tabTimeline && m.timeline.searchMode && m.timeline.searchFocus == timelineSearchFocusInput {
		return false
	}
	if m.activeTab == tabContacts && m.contactSearchMode != "" {
		return false
	}
	return true
}

func (m *Model) handleAccountSwitcherKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	key := shortcutKey(msg)
	if m.showAccountSwitcher {
		switch key {
		case "esc", "q":
			m.showAccountSwitcher = false
			return m, nil, true
		case "up", "k":
			if m.accountSwitcherCursor > 0 {
				m.accountSwitcherCursor--
			}
			return m, nil, true
		case "down", "j":
			if m.accountSwitcherCursor < len(m.accountSwitcherAccounts())-1 {
				m.accountSwitcherCursor++
			}
			return m, nil, true
		case "enter":
			accounts := m.accountSwitcherAccounts()
			if m.accountSwitcherCursor >= 0 && m.accountSwitcherCursor < len(accounts) {
				return m, m.switchActiveAccount(accounts[m.accountSwitcherCursor].SourceID), true
			}
			m.showAccountSwitcher = false
			return m, nil, true
		}
		return m, nil, true
	}
	if m.keyboard != nil {
		if command, ok := m.keyboard.Resolve(keyboardScopeGlobal, keyboardModeNormal, key); !ok || command != CommandAccountSwitcher {
			return m, nil, false
		}
	} else if key != "alt+A" {
		return m, nil, false
	}
	if m.accountSwitcherShortcutAvailable() {
		m.syncAccountsFromBackend()
		for i, account := range m.accountSwitcherAccounts() {
			if account.SourceID == m.activeSourceID {
				m.accountSwitcherCursor = i
				break
			}
		}
		m.showAccountSwitcher = true
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) switchActiveAccount(sourceID models.SourceID) tea.Cmd {
	aware, ok := m.backend.(backend.AccountAwareBackend)
	if !ok {
		m.showAccountSwitcher = false
		return nil
	}
	if m.activeSourceID != "" {
		m.accountSelectedFolders[m.activeSourceID] = m.currentFolder
	}
	targetFolder := strings.TrimSpace(m.accountSelectedFolders[sourceID])
	if targetFolder == "" {
		targetFolder = "INBOX"
	}
	if err := aware.SwitchAccount(sourceID); err != nil {
		m.statusMessage = "Account switch failed: " + err.Error()
		m.showAccountSwitcher = false
		return nil
	}
	m.syncAccountIdentityFromBackend()
	m.refreshCalendarAvailability()
	if m.activeTab == tabCalendar && !m.calendarAvailable {
		m.activeTab = tabTimeline
	}
	m.accountSelectedFolders[sourceID] = targetFolder
	m.resetMailboxStateForFolder(targetFolder)
	m.hydrateCachedTimelineForCurrentFolder()
	m.showAccountSwitcher = false
	m.statusMessage = fmt.Sprintf("Switched to %s", m.activeAccountLabel())
	return tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForSyncEvents())
}

func (m *Model) renderAccountSwitcherOverlayView() string {
	w := m.windowWidth
	if w <= 0 {
		w = 120
	}
	h := m.windowHeight
	if h <= 0 {
		h = 40
	}
	backdrop := m.renderShortcutHelpBackdropView()
	panel := m.renderAccountSwitcherPanel()
	return overlayCentered(backdrop, panel, w, h)
}

func (m *Model) renderAccountSwitcherPanel() string {
	width := accountSwitcherMaxWidth
	if m.windowWidth > 0 && m.windowWidth-6 < width {
		width = m.windowWidth - 6
	}
	if width < 42 {
		width = 42
	}
	contentW := width - 4
	var lines []string
	lines = append(lines, m.theme.Text.Primary.Style().Bold(true).Render("Accounts"))
	lines = append(lines, m.theme.Text.Dim.Style().Render("Switch the visible mail source. Reads can span accounts; writes stay scoped to selected messages."))
	lines = append(lines, "")
	accounts := m.accountSwitcherAccounts()
	for i, account := range accounts {
		status := m.accountStatuses[account.SourceID]
		state := strings.TrimSpace(status.State)
		if state == "" {
			state = "live"
		}
		if account.SourceID == backend.AllAccountsSourceID {
			state = "browse"
		}
		if status.Error != "" {
			state = "auth"
		}
		cursor := "  "
		if i == m.accountSwitcherCursor {
			cursor = "> "
		}
		active := " "
		if account.SourceID == m.activeSourceID {
			active = "*"
		}
		counts := ""
		if status.Total > 0 {
			counts = fmt.Sprintf(" %d/%d", status.Unread, status.Total)
		}
		line := fmt.Sprintf("%s%s %-18s %-8s%s", cursor, active, truncateSidebarLabel(formatAccountLabel(account), 18), state, counts)
		line = safeChromeLine(line, contentW)
		if i == m.accountSwitcherCursor {
			line = m.theme.Focus.SelectionActive.Style().Render(line)
		}
		lines = append(lines, line)
		if status.Error != "" {
			lines = append(lines, m.theme.Severity.Caution.Style().Render("    "+truncateVisual(status.Error, contentW-4)))
		}
	}
	lines = append(lines, "")
	selected := backend.AccountInfo{}
	if m.accountSwitcherCursor >= 0 && m.accountSwitcherCursor < len(accounts) {
		selected = accounts[m.accountSwitcherCursor]
	}
	if selected.SourceID != "" {
		status := m.accountStatuses[selected.SourceID]
		details := []string{
			"Source: " + string(selected.SourceID),
			"Account: " + string(selected.AccountID),
			"Provider: " + selected.Provider,
		}
		if selected.Address != "" {
			details = append(details, "Address: "+selected.Address)
		}
		if status.Error != "" {
			details = append(details, "Status: "+status.Error)
		} else if status.State != "" {
			details = append(details, "Status: "+status.State)
		}
		lines = append(lines, details...)
	}
	lines = append(lines, "")
	lines = append(lines, m.theme.Text.Dim.Style().Render("up/down move  enter switch  esc close"))
	body := strings.Join(lines, "\n")
	return m.baseStyle.
		Width(width).
		BorderForeground(m.theme.PanelBorderColor(true)).
		Render(lipgloss.NewStyle().Width(contentW).Render(body))
}
