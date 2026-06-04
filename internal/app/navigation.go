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
	cmds := m.composeExitCmds()
	m.clearContactsStatus()
	m.activeTab = tabTimeline
	m.clearComposeReturn()
	m.setFocusedPanel(panelTimeline)
	cmds = append(cmds, m.loadTimelineEmails())
	return tea.Batch(cmds...)
}

func (m *Model) switchToCompose() tea.Cmd {
	return m.openBlankComposeFromCurrent()
}

func (m *Model) switchToContacts() tea.Cmd {
	cmds := m.composeExitCmds()
	m.finishTimelineRangeSelection()
	m.clearPreviewSelection()
	m.activeTab = tabContacts
	m.clearComposeReturn()
	m.contactFocusPanel = 0
	m.contactDetail = nil
	m.contactDetailEmails = nil
	m.contactPreviewEmail = nil
	m.contactPreviewBody = nil
	m.contactPreviewLoading = false
	cmds = append(cmds, m.loadContacts())
	return tea.Batch(cmds...)
}

func (m *Model) switchToCalendar() tea.Cmd {
	cmds := m.composeExitCmds()
	m.clearContactsStatus()
	m.finishTimelineRangeSelection()
	m.activeTab = tabCalendar
	m.clearComposeReturn()
	m.setFocusedPanel(panelTimeline)
	m.calendarDetailOpen = false
	m.calendarLoading = true
	m.calendarStatus = "Loading calendar agenda..."
	cmds = append(cmds, m.loadCalendarAgenda())
	return tea.Batch(cmds...)
}
