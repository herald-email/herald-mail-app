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
	m.composeCCExpanded = false
	m.composeBCCExpanded = false
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
	m.resetComposeAIBar()
	m.attachmentInputActive = false
	m.attachmentPathInput.SetValue("")
	m.clearAttachmentCompletions()
	m.lastDraftUID = 0
	m.lastDraftFolder = ""
	m.lastDraftSourceID = ""
	m.lastDraftReplaceable = false
}

func (m *Model) openBlankComposeFromCurrent() tea.Cmd {
	m.clearContactsStatus()
	m.finishTimelineRangeSelection()
	m.rememberComposeReturn()
	m.activeTab = tabCompose
	m.clearComposeFieldsForBlankMessage()
	m.resetComposeMode()
	m.setComposeSource(m.defaultComposeSourceID())
	m.applyConfiguredSignatureToComposeBody()
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
