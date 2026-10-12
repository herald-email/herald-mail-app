package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func (m *Model) composeAccountOptions() []backend.AccountInfo {
	if !m.hasMultipleAccounts() {
		return nil
	}
	out := make([]backend.AccountInfo, 0, len(m.accounts))
	for _, account := range m.accounts {
		if account.SourceID == "" || account.SourceID == backend.AllAccountsSourceID {
			continue
		}
		out = append(out, account)
	}
	return out
}

func (m *Model) composeAccountPickerVisible() bool {
	return len(m.composeIdentityOptions()) > 1
}

func (m *Model) defaultComposeSourceID() models.SourceID {
	options := m.composeAccountOptions()
	if len(options) == 0 {
		return ""
	}
	if m.activeSourceID != "" && m.activeSourceID != backend.AllAccountsSourceID {
		for _, account := range options {
			if account.SourceID == m.activeSourceID {
				return account.SourceID
			}
		}
	}
	if m.replyContextEmail != nil && m.replyContextEmail.SourceID != "" {
		for _, account := range options {
			if account.SourceID == m.replyContextEmail.SourceID {
				return account.SourceID
			}
		}
	}
	return options[0].SourceID
}

func (m *Model) setComposeSource(sourceID models.SourceID) {
	defer m.resetComposeIdentity()
	options := m.composeAccountOptions()
	if len(options) == 0 {
		m.composeSourceID = ""
		return
	}
	if sourceID != "" && sourceID != backend.AllAccountsSourceID {
		for _, account := range options {
			if account.SourceID == sourceID {
				m.composeSourceID = account.SourceID
				return
			}
		}
	}
	m.composeSourceID = m.defaultComposeSourceID()
}

func (m *Model) setComposeSourceForEmail(email *models.EmailData) {
	if email != nil && email.SourceID != "" {
		m.setComposeSource(email.SourceID)
		return
	}
	m.setComposeSource(m.defaultComposeSourceID())
}

func (m *Model) composeSelectedSourceID() models.SourceID {
	if m.composeSourceID == "" {
		m.composeSourceID = m.defaultComposeSourceID()
	}
	return m.composeSourceID
}

func (m *Model) composeSelectedAccount() backend.AccountInfo {
	sourceID := m.composeSelectedSourceID()
	for _, account := range m.composeAccountOptions() {
		if account.SourceID == sourceID {
			return account
		}
	}
	return backend.AccountInfo{}
}

func (m *Model) composePrimaryAddress() string {
	account := m.composeSelectedAccount()
	if strings.TrimSpace(account.Address) != "" {
		return strings.TrimSpace(account.Address)
	}
	return strings.TrimSpace(m.fromAddress)
}

func (m *Model) composeFromPickerValue(width int) string {
	label := m.composeFromAddress()
	if account := m.composeSelectedAccount(); account.DisplayName != "" {
		label = account.DisplayName + " · " + label
	}
	if m.composeIdentitySearchEnabled() {
		if m.composeIdentityFilter != "" {
			label = fmt.Sprintf("[%s] %s", m.composeIdentityFilter, label)
			if len(m.filteredComposeIdentities()) == 0 {
				label = "No matches · " + label
			}
		}
		label += "  (↑/↓ · type to filter)"
	} else if len(m.composeAccountOptions()) > 1 {
		label += "  (↑/↓)"
	}
	if width > 0 {
		return truncateVisual(label, width)
	}
	return label
}

func (m *Model) moveComposeSource(delta int) {
	options := m.filteredComposeIdentities()
	if len(options) == 0 {
		return
	}
	current := 0
	for i, option := range options {
		if option.source == m.composeSelectedSourceID() && strings.EqualFold(option.address, mailboxAddress(m.composeFromAddress())) {
			current = i
			break
		}
	}
	next := (current + delta) % len(options)
	if next < 0 {
		next += len(options)
	}
	m.selectComposeIdentity(options[next])
}

func (m *Model) handleComposeFromKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.composeField != composeFieldFrom {
		return m, nil, false
	}
	if m.composeIdentitySearchEnabled() {
		switch msg.String() {
		case "up", "left":
			m.moveComposeSource(-1)
		case "down", "right":
			m.moveComposeSource(1)
		case "enter":
			m.composeIdentityFilter = ""
		case "backspace":
			query := []rune(m.composeIdentityFilter)
			if len(query) > 0 {
				m.composeIdentityFilter = string(query[:len(query)-1])
			}
			m.matchComposeIdentityFilter()
		default:
			if msg.Text != "" {
				if len([]rune(m.composeIdentityFilter)) < 100 {
					m.composeIdentityFilter += msg.Text
				}
				m.matchComposeIdentityFilter()
			}
		}
		return m, nil, true
	}
	switch shortcutKey(msg) {
	case "up", "left", "k", "h":
		m.moveComposeSource(-1)
	case "down", "right", "j", "l", "enter", "space":
		m.moveComposeSource(1)
	}
	return m, nil, true
}
