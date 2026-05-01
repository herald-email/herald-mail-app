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
	m.summaryTable.Blur()
	m.detailsTable.Blur()
	m.composeField = 0
	m.composeTo.Focus()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	m.replyContextEmail = nil
	m.composePreserved = nil
	m.composeAIThread = false
	m.composeAIPanel = false
	m.composeAIDiff = ""
	m.composeAISubjectHint = ""
	m.composeAIResponse.SetValue("")
	m.composeAILoading = false
}

func (m *Model) clearContactsStatus() {
	m.contactStatusMessage = ""
}

func (m *Model) closeCleanupPreviewForTabSwitch() {
	if !m.cleanupFullScreen && !m.showCleanupPreview {
		return
	}
	m.revokeImagePreviews()
	m.showCleanupPreview = false
	m.cleanupPreviewEmail = nil
	m.cleanupEmailBody = nil
	m.cleanupBodyLoading = false
	m.cleanupBodyScrollOffset = 0
	m.cleanupBodyWrappedLines = nil
	m.cleanupFullScreen = false
	m.cleanupPreviewDeleting = false
	m.cleanupPreviewIsArchive = false
	m.showSidebar = m.cleanupPreviewHadSidebar
	m.clearCleanupPreviewDocumentCache()
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
}

func (m *Model) switchToTimeline() tea.Cmd {
	cmds := m.composeExitCmds()
	m.clearContactsStatus()
	if m.activeTab == tabCleanup {
		m.closeCleanupPreviewForTabSwitch()
	}
	m.activeTab = tabTimeline
	m.clearComposeReturn()
	m.setFocusedPanel(panelTimeline)
	cmds = append(cmds, m.loadTimelineEmails())
	return tea.Batch(cmds...)
}

func (m *Model) switchToCompose() tea.Cmd {
	return m.openBlankComposeFromCurrent()
}

func (m *Model) switchToCleanup() tea.Cmd {
	cmds := m.composeExitCmds()
	m.clearContactsStatus()
	m.activeTab = tabCleanup
	m.clearComposeReturn()
	m.setFocusedPanel(panelSummary)
	return tea.Batch(cmds...)
}

func (m *Model) switchToContacts() tea.Cmd {
	cmds := m.composeExitCmds()
	if m.activeTab == tabCleanup {
		m.closeCleanupPreviewForTabSwitch()
	}
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
