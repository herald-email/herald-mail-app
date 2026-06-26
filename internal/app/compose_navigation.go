package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

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

func (m *Model) clearComposeExitPrompt() {
	m.pendingComposeExitPrompt = false
	m.pendingComposeExitDesc = ""
	m.pendingComposeExitTargetTab = 0
	m.pendingComposeExitTargetPanel = 0
	m.pendingComposeExitLoadTarget = false
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
	m.resetComposeMemoryRadar()
	m.attachmentInputActive = false
	m.attachmentPathInput.SetValue("")
	m.clearAttachmentCompletions()
	m.lastDraftUID = 0
	m.lastDraftFolder = ""
	m.lastDraftSourceID = ""
	m.lastDraftReplaceable = false
	m.clearComposeExitPrompt()
}

func prependComposeEntryClearCmd(clearCmd, next tea.Cmd) tea.Cmd {
	if clearCmd == nil {
		return next
	}
	if next == nil {
		return clearCmd
	}
	return tea.Sequence(clearCmd, next)
}

func (m *Model) openBlankComposeFromCurrent() tea.Cmd {
	clearCmd := m.timelineNativeImageClearCmd()
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
	return clearCmd
}

func (m *Model) returnFromCompose() tea.Cmd {
	targetTab := tabTimeline
	targetPanel := panelTimeline
	if m.composeReturnSet {
		targetTab = m.composeReturnTab
		targetPanel = m.composeReturnPanel
	}
	if m.shouldPromptForComposeExitDraft() {
		return m.openComposeExitPrompt(targetTab, targetPanel, false)
	}
	cmds := m.composeExitCmds()
	return m.finishComposeExit(targetTab, targetPanel, false, cmds...)
}

func (m *Model) shouldPromptForComposeExitDraft() bool {
	if m.activeTab != tabCompose || m.pendingComposeExitPrompt || m.draftSaving || !composeHasContent(m) || m.composePreserved == nil {
		return false
	}
	switch m.composePreserved.kind {
	case models.PreservedMessageKindReply, models.PreservedMessageKindForward:
		return true
	default:
		return false
	}
}

func (m *Model) openComposeExitPrompt(targetTab, targetPanel int, loadTarget bool) tea.Cmd {
	if targetTab == 0 {
		targetTab = tabTimeline
	}
	if targetPanel == 0 {
		targetPanel = panelTimeline
	}
	m.pendingComposeExitPrompt = true
	m.pendingComposeExitTargetTab = targetTab
	m.pendingComposeExitTargetPanel = targetPanel
	m.pendingComposeExitLoadTarget = loadTarget
	kind := m.composeExitDraftKind()
	m.pendingComposeExitDesc = "Keep " + kind + " draft?"
	m.statusMessage = ""
	return nil
}

func (m *Model) composeExitDraftKind() string {
	if m.composePreserved == nil {
		return "message"
	}
	switch m.composePreserved.kind {
	case models.PreservedMessageKindReply:
		return "reply"
	case models.PreservedMessageKindForward:
		return "forward"
	default:
		return strings.TrimSpace(string(m.composePreserved.kind))
	}
}

func (m *Model) keepComposeExitDraft() tea.Cmd {
	if !m.pendingComposeExitPrompt {
		return nil
	}
	targetTab := m.pendingComposeExitTargetTab
	targetPanel := m.pendingComposeExitTargetPanel
	loadTarget := m.pendingComposeExitLoadTarget
	m.clearComposeExitPrompt()
	cmds := m.composeExitCmds()
	return m.finishComposeExit(targetTab, targetPanel, loadTarget, cmds...)
}

func (m *Model) discardComposeExitDraft() tea.Cmd {
	if !m.pendingComposeExitPrompt {
		return nil
	}
	targetTab := m.pendingComposeExitTargetTab
	targetPanel := m.pendingComposeExitTargetPanel
	loadTarget := m.pendingComposeExitLoadTarget
	var cmds []tea.Cmd
	if m.lastDraftUID != 0 && m.lastDraftReplaceable {
		cmds = append(cmds, m.deleteDraftForSourceCmd(m.lastDraftSourceID, m.lastDraftUID, m.lastDraftFolder))
	}
	m.clearComposeFieldsForBlankMessage()
	m.draftSaving = false
	m.statusMessage = "Draft discarded"
	return m.finishComposeExit(targetTab, targetPanel, loadTarget, cmds...)
}

func (m *Model) finishComposeExit(targetTab, targetPanel int, loadTarget bool, cmds ...tea.Cmd) tea.Cmd {
	nativeImageClearCmd := m.timelineNativeImageClearCmd()
	m.activeTab = targetTab
	m.clearComposeReturn()
	switch targetTab {
	case tabTimeline:
		if targetPanel == panelPreview && m.timeline.selectedEmail == nil {
			targetPanel = panelTimeline
		}
		m.setFocusedPanel(targetPanel)
		if loadTarget {
			cmds = append(cmds, m.loadTimelineEmails())
		}
	case tabContacts:
		m.finishTimelineRangeSelection()
		m.clearPreviewSelection()
		m.focusedPanel = panelTimeline
		m.contactFocusPanel = 0
		m.contactDetail = nil
		m.contactDetailEmails = nil
		m.resetContactMemoryDossier()
		m.contactPreviewEmail = nil
		m.contactPreviewBody = nil
		m.contactPreviewLoading = false
		if loadTarget {
			cmds = append(cmds, m.loadContacts())
		}
	case tabCalendar:
		m.clearContactsStatus()
		m.finishTimelineRangeSelection()
		m.setFocusedPanel(panelTimeline)
		m.calendarDetailOpen = false
		m.calendarLoading = true
		m.calendarStatus = "Loading calendar agenda..."
		if loadTarget {
			cmds = append(cmds, m.loadCalendarAgenda())
		}
	case tabMemories:
		m.clearContactsStatus()
		m.finishTimelineRangeSelection()
		m.clearPreviewSelection()
		m.setFocusedPanel(panelTimeline)
		m.ensureMemoriesDefaults()
		if loadTarget {
			cmds = append(cmds, m.loadMemoriesExplore())
		}
	default:
		m.activeTab = tabTimeline
		m.setFocusedPanel(panelTimeline)
	}
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
	if nativeImageClearCmd != nil {
		cmds = append([]tea.Cmd{nativeImageClearCmd}, cmds...)
	}
	return tea.Batch(cmds...)
}
