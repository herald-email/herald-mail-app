package app

import tea "charm.land/bubbletea/v2"

func (m *Model) composeExitCmds() []tea.Cmd {
	if m.activeTab != tabCompose || !composeHasContent(m) || m.draftSaving {
		return nil
	}
	m.draftSaving = true
	return []tea.Cmd{m.saveDraftCmd()}
}

func (m *Model) resetComposeMode() {
	m.timelineTable.Blur()
	m.composeField = composeFieldTo
	m.composeCCExpanded = false
	m.composeBCCExpanded = false
	m.composeTo.Focus()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	m.replyContextEmail = nil
	m.composePreserved = nil
	m.composeAIThread = false
	m.resetComposeAIBar()
	m.resetFieldKeyMode()
}

func (m *Model) resetFieldKeyMode() {
	if mode, ok := m.composeFieldDefaultMode(); ok {
		m.fieldKeyMode = mode
		return
	}
	m.fieldKeyMode = ""
}

func (m *Model) clearContactsStatus() {
	m.contactStatusMessage = ""
}

func (m *Model) switchToTimeline() tea.Cmd {
	if m.shouldPromptForComposeExitDraft() {
		return m.openComposeExitPrompt(tabTimeline, panelTimeline, true)
	}
	cmds := m.composeExitCmds()
	m.clearContactsStatus()
	return m.finishComposeExit(tabTimeline, panelTimeline, true, cmds...)
}

func (m *Model) switchToCompose() tea.Cmd {
	return m.openBlankComposeFromCurrent()
}

func (m *Model) switchToContacts() tea.Cmd {
	if m.shouldPromptForComposeExitDraft() {
		return m.openComposeExitPrompt(tabContacts, panelTimeline, true)
	}
	cmds := m.composeExitCmds()
	return m.finishComposeExit(tabContacts, panelTimeline, true, cmds...)
}

func (m *Model) switchToCalendar() tea.Cmd {
	if m.shouldPromptForComposeExitDraft() {
		return m.openComposeExitPrompt(tabCalendar, panelTimeline, true)
	}
	cmds := m.composeExitCmds()
	return m.finishComposeExit(tabCalendar, panelTimeline, true, cmds...)
}

func (m *Model) switchToMemories() tea.Cmd {
	if m.shouldPromptForComposeExitDraft() {
		return m.openComposeExitPrompt(tabMemories, panelTimeline, true)
	}
	cmds := m.composeExitCmds()
	return m.finishComposeExit(tabMemories, panelTimeline, true, cmds...)
}
