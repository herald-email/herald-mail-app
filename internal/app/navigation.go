package app

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) composeExitCmds() []tea.Cmd {
	if m.activeTab != tabCompose || !composeHasContent(m) || m.draftSaving {
		return nil
	}
	m.draftSaving = true
	var cmds []tea.Cmd
	if m.lastDraftUID != 0 {
		cmds = append(cmds, m.deleteDraftCmd(m.lastDraftUID, m.lastDraftFolder))
		m.lastDraftUID = 0
		m.lastDraftFolder = ""
	}
	cmds = append(cmds, m.saveDraftCmd())
	return cmds
}

func (m *Model) resetComposeMode() {
	m.timelineTable.Blur()
	m.summaryTable.Blur()
	m.detailsTable.Blur()
	m.composeField = 0
	m.composeTo.Focus()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	m.replyContextEmail = nil
	m.composeAIThread = false
	m.composeAIPanel = false
	m.composeAIDiff = ""
	m.composeAISubjectHint = ""
	m.composeAIResponse.SetValue("")
	m.composeAILoading = false
}

func (m *Model) switchToTimeline() tea.Cmd {
	cmds := m.composeExitCmds()
	m.activeTab = tabTimeline
	m.setFocusedPanel(panelTimeline)
	cmds = append(cmds, m.loadTimelineEmails())
	return tea.Batch(cmds...)
}

func (m *Model) switchToCompose() tea.Cmd {
	m.activeTab = tabCompose
	m.resetComposeMode()
	return nil
}

func (m *Model) switchToCleanup() tea.Cmd {
	cmds := m.composeExitCmds()
	m.activeTab = tabCleanup
	m.setFocusedPanel(panelSummary)
	return tea.Batch(cmds...)
}

func (m *Model) switchToContacts() tea.Cmd {
	cmds := m.composeExitCmds()
	m.activeTab = tabContacts
	m.contactFocusPanel = 0
	m.contactDetail = nil
	m.contactDetailEmails = nil
	m.contactPreviewEmail = nil
	m.contactPreviewBody = nil
	m.contactPreviewLoading = false
	cmds = append(cmds, m.loadContacts())
	return tea.Batch(cmds...)
}
