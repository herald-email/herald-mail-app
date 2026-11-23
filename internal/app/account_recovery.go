package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// Provider errors arrive as text over the sync event boundary. Match explicit
// credential failures, not generic connection errors or permission denials.
func accountNeedsSignIn(status backend.AccountStatus) bool {
	if status.State == "auth" {
		return true
	}
	message := strings.ToLower(status.Error)
	for _, marker := range []string{"invalid_grant", "invalid_token", "authenticationfailed", "authentication failed", "invalid credentials", "invalidcredentials", "unauthenticated", "token has been expired or revoked", "token expired", "login failed", "authentication required", "please re-authenticate"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (m *Model) recordAccountSyncStatus(event models.FolderSyncEvent) {
	id := event.SourceID
	if _, aware := m.backend.(backend.AccountAwareBackend); !aware && len(m.accounts) == 1 {
		id = m.accounts[0].SourceID
	}
	if id == "" {
		id = m.activeSourceID
	}
	if id == "" || id == backend.AllAccountsSourceID {
		return
	}
	if event.Generation < m.accountStatusGenerations[id] {
		return
	}
	if m.accountStatusGenerations == nil {
		m.accountStatusGenerations = make(map[models.SourceID]int64)
	}
	m.accountStatusGenerations[id] = event.Generation
	if event.Phase != models.SyncPhaseError && event.Phase != models.SyncPhaseComplete {
		return
	}
	if m.accountStatuses == nil {
		m.accountStatuses = make(map[models.SourceID]backend.AccountStatus)
	}
	status := m.accountStatuses[id]
	status.SourceID = id
	if event.Phase == models.SyncPhaseError {
		message := strings.TrimSpace(event.Error)
		if message == "" {
			message = strings.TrimSpace(event.Message)
		}
		// A later transport failure is not evidence that credentials recovered.
		if !accountNeedsSignIn(status) || accountNeedsSignIn(backend.AccountStatus{Error: message}) {
			status.State, status.Error = "error", message
		}
	} else {
		status.State, status.Error = "live", ""
	}
	m.accountStatuses[id] = status
}

func (m *Model) accountSignInWarning() string {
	for _, account := range m.accounts {
		if accountNeedsSignIn(m.accountStatuses[account.SourceID]) {
			return "🚨 " + truncateVisual(accountRecoveryLabel(account), 18) + ": sign in · " + m.commandHelpKey(keyboardScopeGlobal, CommandAccountSwitcher) + " Accounts"
		}
	}
	return ""
}

func (m *Model) recoverAccount(account backend.AccountInfo) tea.Cmd {
	if m.cfg == nil || account.SourceID == backend.AllAccountsSourceID {
		return nil
	}
	// Work on a candidate, so cancelling authorization never edits live tokens.
	candidate := *m.cfg
	candidate.Sources = append([]config.SourceConfig(nil), m.cfg.Sources...)
	var source config.SourceConfig
	found := false
	for _, item := range candidate.NormalizedSources() {
		if settingsSourceIDForSource(item) == account.SourceID {
			source, found = item, true
			break
		}
	}
	if !found {
		m.statusMessage = "Account configuration is unavailable"
		return nil
	}
	m.showAccountSwitcher = false
	if settingsMailSourceUsesGoogleOAuth(source) {
		ids := []models.SourceID{account.SourceID}
		for _, item := range candidate.NormalizedSources() {
			if item.AccountID == source.AccountID && item.AccountID != "" && settingsSourceUsesGoogleOAuth(item) && settingsSourceIDForSource(item) != account.SourceID {
				ids = append(ids, settingsSourceIDForSource(item))
			}
		}
		return func() tea.Msg {
			return OAuthRequiredMsg{Email: account.Address, Config: &candidate, SourceIDs: ids, ServiceLabel: "Sign in again", ReturnToMenu: false, ValidateAccount: true, ValidationSourceID: account.SourceID}
		}
	}
	m.settingsPanel = m.newSettingsPanel(settingsPanelSectionAccounts, "")
	m.settingsPanel.accountsMenuChoice = "account:" + source.AccountID
	m.settingsPanel.openSelectedAccountFromList()
	m.settingsPanel.panelStatus = "Update credentials, then save to reconnect."
	m.settingsPanel.setSize(m.windowWidth, m.windowHeight)
	m.showSettings = true
	m.invalidateSettingsBackdrop()
	return m.settingsPanel.Init()
}

func accountRecoveryLabel(account backend.AccountInfo) string {
	if name := strings.TrimSpace(account.DisplayName); name != "" {
		return name
	}
	if address := strings.TrimSpace(account.Address); address != "" {
		return address
	}
	return string(account.SourceID)
}
