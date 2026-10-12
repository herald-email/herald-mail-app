package app

import (
	"net/mail"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type composeIdentityOption struct {
	source        models.SourceID
	name, address string
}

func mailboxAddress(value string) string {
	if parsed, err := mail.ParseAddress(value); err == nil {
		return parsed.Address
	}
	return strings.TrimSpace(value)
}

func (m *Model) composeIdentityConfig(source models.SourceID) config.ComposeConfig {
	if m.cfg == nil {
		return config.ComposeConfig{}
	}
	sources := m.cfg.NormalizedSources()
	for _, candidate := range sources {
		if candidate.Kind == string(models.SourceKindMail) && (string(source) == candidate.ID || source == "" && !m.hasMultipleAccounts()) {
			return candidate.Compose
		}
	}
	return config.ComposeConfig{}
}

func (m *Model) composeIdentityOptions() []composeIdentityOption {
	accounts := m.composeAccountOptions()
	if len(accounts) == 0 {
		accounts = []backend.AccountInfo{{Address: m.fromAddress}}
	}
	var out []composeIdentityOption
	for _, account := range accounts {
		out = append(out, composeIdentityOption{source: account.SourceID, address: strings.TrimSpace(account.Address)})
		for _, identity := range m.composeIdentityConfig(account.SourceID).Identities {
			out = append(out, composeIdentityOption{source: account.SourceID, name: identity.Name, address: strings.TrimSpace(identity.Address)})
		}
	}
	return out
}

func (m *Model) resetComposeIdentity() {
	m.composeIdentityAddress = ""
	m.composeIdentityFilter = ""
}

func (m *Model) composeFromAddress() string {
	source := m.composeSelectedSourceID()
	selected := m.composeIdentityAddress
	if selected == "" {
		selected = m.composeIdentityConfig(source).DefaultIdentity
	}
	for _, option := range m.composeIdentityOptions() {
		if option.source == source && strings.EqualFold(option.address, strings.TrimSpace(selected)) {
			if option.name != "" {
				return (&mail.Address{Name: option.name, Address: option.address}).String()
			}
			return option.address
		}
	}
	return m.composePrimaryAddress()
}

func (m *Model) composeIdentitySearchEnabled() bool {
	for _, option := range m.composeIdentityOptions() {
		if len(m.composeIdentityConfig(option.source).Identities) > 0 {
			return true
		}
	}
	return false
}

func (m *Model) filteredComposeIdentities() []composeIdentityOption {
	var out []composeIdentityOption
	for _, option := range m.composeIdentityOptions() {
		if strings.Contains(strings.ToLower(option.name+" "+option.address+" "+string(option.source)), strings.ToLower(m.composeIdentityFilter)) {
			out = append(out, option)
		}
	}
	return out
}

func (m *Model) selectComposeIdentity(option composeIdentityOption) {
	m.composeSourceID = option.source
	m.composeIdentityAddress = option.address
}

func (m *Model) matchComposeIdentityFilter() {
	if matches := m.filteredComposeIdentities(); len(matches) > 0 {
		m.selectComposeIdentity(matches[0])
	}
}

// Match only senders belonging to this message's account.
func (m *Model) restoreComposeIdentity(addresses string) {
	parsed, err := mail.ParseAddressList(addresses)
	if err != nil {
		return
	}
	for _, address := range parsed {
		for _, option := range m.composeIdentityOptions() {
			if option.source == m.composeSelectedSourceID() && strings.EqualFold(option.address, address.Address) {
				m.selectComposeIdentity(option)
				return
			}
		}
	}
}
