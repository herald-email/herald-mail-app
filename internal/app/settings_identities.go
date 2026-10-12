package app

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type settingsIdentityState struct {
	sourceID, primary                     string
	legacy                                bool
	compose                               config.ComposeConfig
	choice, name, address, action, status string
	index                                 int // -1 primary, len(Identities) new
	makeDefault, remove                   bool
}

// SettingsIdentitiesSavedMsg only changes sender metadata, never account credentials.
type SettingsIdentitiesSavedMsg struct {
	SourceID string
	Legacy   bool
	Compose  config.ComposeConfig
}

func (s *Settings) isIdentitySection() bool {
	return s.panelSection == settingsPanelSectionIdentities || s.panelSection == settingsPanelSectionIdentityEdit || s.panelSection == settingsPanelSectionIdentityRemove
}

func (s *Settings) openIdentityManager() bool {
	source, ok := s.existingSourceForKind(models.SourceKindMail)
	if !ok {
		return false
	}
	s.identity = settingsIdentityState{sourceID: source.ID, primary: sourceAddressForSettings(source), legacy: len(s.cfg.Sources) == 0, compose: source.Compose, choice: "primary"}
	s.identity.compose.Identities = append([]config.IdentityConfig(nil), source.Compose.Identities...)
	s.panelSection = settingsPanelSectionIdentities
	s.buildForm()
	return true
}

func (s *Settings) buildIdentityForm() {
	id := &s.identity
	var fields []huh.Field
	title := "Sending identities"
	switch s.panelSection {
	case settingsPanelSectionIdentities:
		label := func(name, address string) string {
			if name != "" {
				address = name + " <" + address + ">"
			}
			return address
		}
		primary := id.primary + " (primary)"
		if id.compose.DefaultIdentity == "" || strings.EqualFold(id.compose.DefaultIdentity, id.primary) {
			primary += " · default"
		}
		options := []huh.Option[string]{huh.NewOption(primary, "primary")}
		for i, alias := range id.compose.Identities {
			text := label(alias.Name, alias.Address)
			if strings.EqualFold(alias.Address, id.compose.DefaultIdentity) {
				text += " · default"
			}
			options = append(options, huh.NewOption(text, strconv.Itoa(i)))
		}
		options = append(options, huh.NewOption("[ Add identity ]", "add"), huh.NewOption("[ Save changes ]", "save"))
		description := "Use addresses this provider allows. Save applies changes; Esc discards."
		if id.status != "" {
			description = id.status + "\n" + description
		}
		fields = []huh.Field{huh.NewSelect[string]().Title(id.primary).Description(description).Options(options...).Value(&id.choice)}
	case settingsPanelSectionIdentityEdit:
		title = "Edit identity"
		if id.index == len(id.compose.Identities) {
			title = "Add identity"
		}
		if id.index < 0 {
			fields = append(fields, huh.NewNote().Title(id.primary).Description("Primary sender. Its address is managed by the account."))
		} else {
			fields = append(fields,
				huh.NewInput().Key("identity.name").Title("Name (optional)").Inline(true).Value(&id.name),
				huh.NewInput().Key("identity.address").Title("Email address").Inline(true).Value(&id.address).Validate(func(value string) error { return s.validateIdentityEdit(value) }),
			)
		}
		if id.index >= 0 {
			fields = append(fields, huh.NewConfirm().Title("Default for new messages").Affirmative("Yes").Negative("No").Value(&id.makeDefault))
		}
		applyLabel := "Apply to list"
		if id.index < 0 {
			applyLabel = "Use primary as default"
			id.makeDefault = true
		}
		options := []huh.Option[string]{huh.NewOption(applyLabel, "apply")}
		if id.index >= 0 && id.index < len(id.compose.Identities) {
			options = append(options, huh.NewOption("Remove identity", "remove"))
		}
		fields = append(fields, huh.NewSelect[string]().Key("identity.action").Title("Action").Options(options...).Value(&id.action))
	case settingsPanelSectionIdentityRemove:
		title = "Remove identity"
		alias := id.compose.Identities[id.index]
		fields = []huh.Field{huh.NewConfirm().Title("Remove " + alias.Address + "?").Description("Removes it from Herald only. Save the list to apply.").Affirmative("Remove").Negative("Cancel").Value(&id.remove)}
	}
	s.form = huh.NewForm(huh.NewGroup(fields...).Title(title)).WithTheme(huh.ThemeFunc(heraldHuhTheme)).WithShowHelp(true).WithShowErrors(true).WithKeyMap(settingsPanelMenuKeyMap())
	if s.width > 0 {
		s.form = s.form.WithWidth(s.formWidth())
	}
	if s.height > 0 {
		s.form = s.form.WithHeight(s.formHeight())
	}
	s.prepareFormForView()
}

func (s *Settings) validateIdentityEdit(address string) error {
	id := &s.identity
	candidate := config.ComposeConfig{Identities: append([]config.IdentityConfig(nil), id.compose.Identities...)}
	entry := config.IdentityConfig{Name: strings.TrimSpace(id.name), Address: strings.TrimSpace(address)}
	if id.index < len(candidate.Identities) {
		candidate.Identities[id.index] = entry
	} else {
		candidate.Identities = append(candidate.Identities, entry)
	}
	return candidate.ValidateIdentities(id.primary)
}

func (s *Settings) completeIdentityForm() tea.Cmd {
	id := &s.identity
	switch s.panelSection {
	case settingsPanelSectionIdentities:
		if id.choice == "save" {
			if err := id.compose.ValidateIdentities(id.primary); err != nil {
				id.status = err.Error()
				break
			}
			s.done = true
			msg := SettingsIdentitiesSavedMsg{SourceID: id.sourceID, Legacy: id.legacy, Compose: id.compose}
			return func() tea.Msg { return msg }
		}
		id.index = -1
		if id.choice == "add" {
			id.index = len(id.compose.Identities)
		} else if id.choice != "primary" {
			index, err := strconv.Atoi(id.choice)
			if err != nil || index < 0 || index >= len(id.compose.Identities) {
				id.choice = "primary"
				break
			}
			id.index = index
		}
		id.name = ""
		id.address = id.primary
		id.action = "apply"
		id.status = ""
		if id.index >= 0 {
			id.address = ""
			if id.index < len(id.compose.Identities) {
				id.name = id.compose.Identities[id.index].Name
				id.address = id.compose.Identities[id.index].Address
			}
		}
		id.makeDefault = id.address != "" && strings.EqualFold(id.address, id.compose.DefaultIdentity) || id.index < 0 && id.compose.DefaultIdentity == ""
		s.panelSection = settingsPanelSectionIdentityEdit
	case settingsPanelSectionIdentityEdit:
		if id.action == "remove" && id.index >= 0 && id.index < len(id.compose.Identities) {
			id.remove = false
			s.panelSection = settingsPanelSectionIdentityRemove
			break
		}
		if id.index >= 0 {
			if err := s.validateIdentityEdit(id.address); err != nil {
				id.status = err.Error()
				break
			}
			entry := config.IdentityConfig{Name: strings.TrimSpace(id.name), Address: strings.TrimSpace(id.address)}
			oldDefault := false
			if id.index < len(id.compose.Identities) {
				oldDefault = strings.EqualFold(id.compose.DefaultIdentity, id.compose.Identities[id.index].Address)
				id.compose.Identities[id.index] = entry
			} else {
				id.compose.Identities = append(id.compose.Identities, entry)
			}
			if oldDefault {
				id.compose.DefaultIdentity = ""
			}
			id.address = entry.Address
		}
		if id.makeDefault {
			id.compose.DefaultIdentity = id.address
		}
		if id.index < 0 {
			id.choice = "primary"
		} else {
			id.choice = strconv.Itoa(id.index)
		}
		id.status = "Changes staged. Choose Save changes to apply."
		s.panelSection = settingsPanelSectionIdentities
	case settingsPanelSectionIdentityRemove:
		if !id.remove {
			s.panelSection = settingsPanelSectionIdentityEdit
			id.action = "apply"
			break
		}
		if strings.EqualFold(id.compose.DefaultIdentity, id.compose.Identities[id.index].Address) {
			id.compose.DefaultIdentity = ""
		}
		id.compose.Identities = append(id.compose.Identities[:id.index], id.compose.Identities[id.index+1:]...)
		id.choice = "primary"
		id.status = "Identity removed from list. Save changes to apply."
		s.panelSection = settingsPanelSectionIdentities
	}
	s.buildForm()
	return s.form.Init()
}

func (s *Settings) leaveIdentitySection() tea.Cmd {
	if s.panelSection == settingsPanelSectionIdentities {
		s.panelSection = settingsPanelSectionAccount
		s.identity = settingsIdentityState{}
	} else if s.panelSection == settingsPanelSectionIdentityRemove {
		s.panelSection = settingsPanelSectionIdentityEdit
		s.identity.action = "apply"
	} else {
		s.panelSection = settingsPanelSectionIdentities
		s.identity.choice = "primary"
	}
	s.buildForm()
	return s.form.Init()
}

// Merge only identity fields into the current configuration; preserve refreshed
// OAuth tokens and all unrelated settings, even if the manager has been open a while.
func configWithIdentities(base *config.Config, msg SettingsIdentitiesSavedMsg) (*config.Config, error) {
	if base == nil {
		return nil, fmt.Errorf("no active configuration")
	}
	next := *base
	apply := func(c *config.ComposeConfig, primary string) error {
		if err := msg.Compose.ValidateIdentities(primary); err != nil {
			return err
		}
		c.Identities = append([]config.IdentityConfig(nil), msg.Compose.Identities...)
		c.DefaultIdentity = msg.Compose.DefaultIdentity
		return nil
	}
	if msg.Legacy && len(base.Sources) == 0 {
		sources := base.NormalizedSources()
		for _, source := range sources {
			if source.Kind == "mail" {
				if err := apply(&next.Compose, sourceAddressForSettings(source)); err != nil {
					return nil, err
				}
				return &next, nil
			}
		}
	} else {
		next.Sources = append([]config.SourceConfig(nil), base.Sources...)
		for i, source := range next.Sources {
			if source.ID == msg.SourceID && source.Kind == "mail" {
				if err := apply(&next.Sources[i].Compose, sourceAddressForSettings(source)); err != nil {
					return nil, err
				}
				return &next, nil
			}
		}
	}
	return nil, fmt.Errorf("mail account changed; reopen Sending identities")
}

func (m *Model) saveSettingsIdentities(msg SettingsIdentitiesSavedMsg) tea.Cmd {
	next, err := configWithIdentities(m.cfg, msg)
	if err == nil {
		if m.configPath == "" {
			err = fmt.Errorf("configuration path unavailable")
		} else {
			err = next.Save(m.configPath)
		}
	}
	status := "Sending identities saved."
	if err != nil {
		status = "Could not save identities: " + err.Error()
	} else {
		m.SetConfig(next)
	}
	m.statusMessage = status
	if s := m.settingsPanel; s != nil {
		s.done = false
		if err == nil {
			s.cfg = next
		}
		s.identity.status = status
		s.panelSection = settingsPanelSectionIdentities
		s.buildForm()
		return s.form.Init()
	}
	return nil
}
