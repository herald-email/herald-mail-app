package app

import tea "charm.land/bubbletea/v2"

func (m *Model) rememberComposeReturn() {
	if m.activeTab == tabCompose {
		return
	}
	m.composeReturnSet = true
	m.composeReturnTab = m.activeTab
	m.composeReturnPanel = m.focusedPanel
}

func (m *Model) clearComposeReturn() {
	m.composeReturnSet = false
	m.composeReturnTab = 0
	m.composeReturnPanel = 0
}

func (m *Model) clearComposeFieldsForBlankMessage() {
	m.composeTo.SetValue("")
	m.composeCC.SetValue("")
	m.composeBCC.SetValue("")
	m.composeSubject.SetValue("")
	m.composeBody.SetValue("")
	m.composeStatus = ""
	m.composePreview = false
	m.composeAttachments = nil
	m.suggestions = nil
	m.suggestionIdx = -1
	m.replyContextEmail = nil
	m.composePreserved = nil
	m.composeAIThread = false
	m.composeAIPanel = false
	m.composeAIDiff = ""
	m.composeAISubjectHint = ""
	m.composeAIInput.SetValue("")
	m.composeAIResponse.SetValue("")
	m.composeAILoading = false
	m.attachmentInputActive = false
	m.attachmentPathInput.SetValue("")
	m.clearAttachmentCompletions()
	m.lastDraftUID = 0
	m.lastDraftFolder = ""
}

func (m *Model) openBlankComposeFromCurrent() tea.Cmd {
	m.clearContactsStatus()
	m.finishTimelineRangeSelection()
	if m.activeTab == tabCleanup {
		m.closeCleanupPreviewForTabSwitch()
	}
	m.rememberComposeReturn()
	m.activeTab = tabCompose
	m.clearComposeFieldsForBlankMessage()
	m.resetComposeMode()
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
	return nil
}

func (m *Model) returnFromCompose() tea.Cmd {
	cmds := m.composeExitCmds()
	targetTab := tabTimeline
	targetPanel := panelTimeline
	if m.composeReturnSet {
		targetTab = m.composeReturnTab
		targetPanel = m.composeReturnPanel
	}
	m.activeTab = targetTab
	m.clearComposeReturn()
	switch targetTab {
	case tabTimeline:
		if targetPanel == panelPreview && m.timeline.selectedEmail == nil {
			targetPanel = panelTimeline
		}
		m.setFocusedPanel(targetPanel)
	case tabCleanup:
		m.setFocusedPanel(targetPanel)
	case tabContacts:
		m.focusedPanel = panelTimeline
	default:
		m.activeTab = tabTimeline
		m.setFocusedPanel(panelTimeline)
	}
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
	return tea.Batch(cmds...)
}
