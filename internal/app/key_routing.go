package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.pendingDeleteConfirm {
		switch msg.String() {
		case "y", "Y":
			m.pendingDeleteConfirm = false
			action := m.pendingDeleteAction
			m.pendingDeleteAction = nil
			m.pendingDeleteDesc = ""
			if action != nil {
				return m, action(), true
			}
		case "n", "N", "esc":
			m.pendingDeleteConfirm = false
			m.pendingDeleteAction = nil
			m.pendingDeleteDesc = ""
		}
		return m, nil, true
	}

	if m.pendingUnsubscribe {
		switch msg.String() {
		case "y", "Y":
			m.pendingUnsubscribe = false
			action := m.pendingUnsubscribeAction
			m.pendingUnsubscribeAction = nil
			m.pendingUnsubscribeDesc = ""
			if action != nil {
				return m, action(), true
			}
		case "n", "N", "esc":
			m.pendingUnsubscribe = false
			m.pendingUnsubscribeAction = nil
			m.pendingUnsubscribeDesc = ""
		}
		return m, nil, true
	}

	if m.timeline.attachmentSavePrompt {
		switch msg.String() {
		case "enter":
			if m.timeline.body != nil && m.timeline.selectedAttachment < len(m.timeline.body.Attachments) {
				att := &m.timeline.body.Attachments[m.timeline.selectedAttachment]
				path := expandTilde(m.timeline.attachmentSaveInput.Value())
				if suggested, warning, blocked := attachmentSaveCollision(path); blocked {
					m.timeline.attachmentSaveInput.SetValue(suggested)
					m.timeline.attachmentSaveWarning = warning
					m.timeline.attachmentSaveInput.Focus()
					return m, nil, true
				}
				m.timeline.attachmentSavePrompt = false
				m.timeline.attachmentSaveWarning = ""
				m.timeline.attachmentSaveInput.Blur()
				return m, saveAttachmentCmd(m.backend, att, path), true
			}
			m.timeline.attachmentSavePrompt = false
			m.timeline.attachmentSaveWarning = ""
			m.timeline.attachmentSaveInput.Blur()
		case "esc":
			m.timeline.attachmentSavePrompt = false
			m.timeline.attachmentSaveWarning = ""
			m.timeline.attachmentSaveInput.Blur()
		default:
			var cmd tea.Cmd
			m.timeline.attachmentSaveInput, cmd = m.timeline.attachmentSaveInput.Update(msg)
			return m, cmd, true
		}
		return m, nil, true
	}

	if m.timeline.searchMode && m.activeTab == tabTimeline && m.timeline.searchFocus == timelineSearchFocusInput {
		switch msg.String() {
		case "esc":
			m.clearTimelineSearch()
			return m, nil, true
		case "enter":
			query := strings.TrimSpace(m.timeline.searchInput.Value())
			if query != "" && query == strings.TrimSpace(m.timeline.searchResultsQuery) && len(m.timeline.searchResults) > 0 {
				m.timeline.searchFocus = timelineSearchFocusResults
				m.timeline.searchInput.Blur()
				m.setFocusedPanel(panelTimeline)
				return m, nil, true
			}
			if query == "" {
				return m, nil, true
			}
			m.timeline.searchToken++
			m.timeline.searchAutoFocusResults = true
			return m, m.performSearchWithToken(m.timeline.searchInput.Value(), m.timeline.searchToken), true
		case "ctrl+i", "tab":
			if strings.TrimSpace(m.timeline.searchInput.Value()) == "" {
				return m, nil, true
			}
			m.timeline.searchToken++
			m.timeline.searchAutoFocusResults = false
			return m, m.performIMAPSearchWithToken(m.timeline.searchInput.Value(), m.timeline.searchToken), true
		case "ctrl+s":
			return m, nil, true
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit, true
		default:
			var cmd tea.Cmd
			m.timeline.searchInput, cmd = m.timeline.searchInput.Update(msg)
			m.timeline.searchError = ""
			m.timeline.searchAutoFocusResults = false
			m.timeline.searchResultsQuery = ""
			m.timeline.searchToken++
			return m, tea.Batch(cmd, scheduleTimelineSearchDebounce(m.timeline.searchToken, m.timeline.searchInput.Value())), true
		}
	}

	if m.focusedPanel == panelChat && m.showChat {
		switch msg.String() {
		case "enter":
			if !m.chatWaiting {
				return m, m.submitChat(), true
			}
		case "esc":
			m.showChat = false
			m.chatInput.Blur()
			if m.windowWidth > 0 {
				m.updateTableDimensions(m.windowWidth, m.windowHeight)
			}
			m.setFocusedPanel(m.defaultFocusPanel())
		case "tab":
			m.chatInput.Blur()
			m.setFocusedPanel(m.defaultFocusPanel())
		}
		return m, nil, true
	}

	return m, nil, false
}

func (m *Model) handleLogsOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if !m.showLogs {
		return m, nil, false
	}
	switch msg.String() {
	case "l", "L", "alt+l", "alt+L", "esc":
		m.showLogs = false
		return m, nil, true
	case "q":
		m.cleanup()
		return m, tea.Quit, true
	}
	var cmd tea.Cmd
	_, cmd = m.logViewer.Update(msg)
	return m, cmd, true
}

func (m *Model) handleGlobalCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c":
		m.cleanup()
		return m, tea.Quit, true
	case "f1", "alt+1":
		if m.canInteractWithVisibleData() && m.activeTab != tabTimeline {
			return m, m.switchToTimeline(), true
		}
		return m, nil, true
	case "f2", "alt+2":
		if m.canInteractWithVisibleData() && m.activeTab != tabCompose {
			return m, m.switchToCompose(), true
		}
		return m, nil, true
	case "f3", "alt+3":
		if m.canInteractWithVisibleData() && m.activeTab != tabCleanup {
			return m, m.switchToCleanup(), true
		}
		return m, nil, true
	case "f4", "alt+4":
		if m.canInteractWithVisibleData() && m.activeTab != tabContacts {
			return m, m.switchToContacts(), true
		}
		if m.canInteractWithVisibleData() {
			return m, m.loadContacts(), true
		}
		return m, nil, true
	case "alt+l", "alt+L":
		return m, m.toggleLogs(), true
	case "alt+c", "alt+C":
		return m, m.toggleChat(), true
	case "alt+f", "alt+F":
		return m, m.toggleSidebar(), true
	case "alt+r", "alt+R":
		return m, m.refreshCurrentFolder(), true
	}
	return m, nil, false
}

func (m *Model) toggleSidebar() tea.Cmd {
	if m.canInteractWithVisibleData() {
		m.showSidebar = !m.showSidebar
		if m.windowWidth > 0 {
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
		}
		if !m.showSidebar && m.focusedPanel == panelSidebar {
			m.setFocusedPanel(panelSummary)
		}
	}
	return nil
}

func (m *Model) toggleLogs() tea.Cmd {
	if m.canInteractWithVisibleData() {
		m.showLogs = !m.showLogs
	}
	return nil
}

func (m *Model) refreshCurrentFolder() tea.Cmd {
	if !m.loading {
		m.loading = true
		m.startTime = time.Now()
		m.clearTimelineChatFilter()
		return m.activateCurrentFolder()
	}
	return nil
}

func (m *Model) toggleChat() tea.Cmd {
	if !m.loading {
		if !m.showChat && m.windowWidth > 0 && !m.canRenderChat(m.windowWidth) {
			m.statusMessage = "Chat hidden at this size — widen terminal to open it"
			return nil
		}
		m.showChat = !m.showChat
		if m.windowWidth > 0 {
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
		}
		if m.showChat {
			m.focusedPanel = panelChat
			m.chatInput.Focus()
			m.summaryTable.Blur()
			m.detailsTable.Blur()
		} else {
			m.chatInput.Blur()
			m.setFocusedPanel(m.defaultFocusPanel())
		}
	}
	return nil
}

func (m *Model) handleTabKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "1":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 0 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[0])
			return model, cmd, true
		}
		if m.canInteractWithVisibleData() && m.activeTab != tabTimeline {
			return m, m.switchToTimeline(), true
		}
		return m, nil, true
	case "2":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 1 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[1])
			return model, cmd, true
		}
		if m.canInteractWithVisibleData() && m.activeTab != tabCompose {
			return m, m.switchToCompose(), true
		}
		return m, nil, true
	case "3":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 2 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[2])
			return model, cmd, true
		}
		if m.canInteractWithVisibleData() && m.activeTab != tabCleanup {
			return m, m.switchToCleanup(), true
		}
		return m, nil, true
	case "4":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 3 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[3])
			return model, cmd, true
		}
		if m.canInteractWithVisibleData() && m.activeTab != tabContacts {
			return m, m.switchToContacts(), true
		}
		if m.canInteractWithVisibleData() {
			return m, m.loadContacts(), true
		}
		return m, nil, true
	case "5":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 4 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[4])
			return model, cmd, true
		}
		return m, nil, true
	case "6":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 5 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[5])
			return model, cmd, true
		}
		return m, nil, true
	case "7":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 6 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[6])
			return model, cmd, true
		}
		return m, nil, true
	case "8":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 7 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[7])
			return model, cmd, true
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) handleEscKey() (tea.Model, tea.Cmd) {
	if m.timeline.quickReplyOpen {
		m.clearTimelineQuickReply()
		return m, nil
	}
	if m.timeline.visualMode {
		m.timeline.visualMode = false
		m.timeline.pendingY = false
		return m, nil
	}
	if m.timeline.fullScreen {
		m.clearTimelineFullScreen()
		return m, nil
	}
	if m.activeTab == tabCleanup && m.showCleanupPreview && m.cleanupFullScreen {
		m.cleanupFullScreen = false
		m.cleanupBodyWrappedLines = nil
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
		return m, nil
	}
	if m.activeTab == tabCleanup && m.showCleanupPreview {
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
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
		return m, nil
	}
	if m.activeTab == tabTimeline && m.timeline.chatFilterMode {
		m.clearTimelineChatFilter()
		return m, nil
	}
	if m.activeTab == tabTimeline && m.timeline.selectedEmail != nil {
		if m.timeline.searchMode && m.timeline.searchFocus == timelineSearchFocusResults {
			m.clearTimelinePreview()
			return m, nil
		}
		m.clearTimelinePreview()
		return m, nil
	}
	if m.activeTab == tabTimeline && m.timeline.searchMode {
		if m.timeline.searchFocus == timelineSearchFocusResults {
			m.timeline.searchFocus = timelineSearchFocusInput
			m.timeline.searchInput.Focus()
			m.setFocusedPanel(panelTimeline)
			return m, nil
		}
		m.clearTimelineSearch()
		return m, nil
	}
	if m.activeTab == tabCompose {
		if m.composeAISubjectHint != "" {
			m.composeAISubjectHint = ""
		} else if m.composeAIPanel {
			m.composeAIPanel = false
			m.composeAIDiff = ""
			m.composeAIInput.Blur()
			m.composeAIResponse.Blur()
		} else {
			m.composeStatus = ""
		}
		return m, nil
	}
	return m, nil
}

func isGlobalContactsKey(key string) bool {
	switch key {
	case "1", "2", "3", "4", "q", "ctrl+c", "r", "f", "c", "l", "L":
		return true
	}
	return false
}

func (m *Model) isComposeReplyForwardSubject(subject string, prefix string) string {
	if !strings.HasPrefix(strings.ToLower(subject), prefix) {
		return strings.Title(prefix) + " " + subject
	}
	return subject
}
