package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
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

	if m.unsubConfirmMode {
		switch msg.String() {
		case "h", "H":
			sender := m.unsubConfirmSender
			m.unsubConfirmMode = false
			m.unsubConfirmSender = ""
			folder := m.currentFolder
			ch := m.deletionRequestCh
			go func() {
				ch <- models.DeletionRequest{Sender: sender, IsDomain: false, Folder: folder}
			}()
			m.deleting = true
			m.deletionsPending = 1
			m.deletionsTotal = 1
			return m, m.listenForDeletionResults(), true
		case "s", "S":
			sender := m.unsubConfirmSender
			m.unsubConfirmMode = false
			m.unsubConfirmSender = ""
			return m, createSoftUnsubscribeRuleCmd(m.backend, sender), true
		case "esc":
			m.unsubConfirmMode = false
			m.unsubConfirmSender = ""
		}
		return m, nil, true
	}

	if m.timeline.attachmentSavePrompt {
		switch msg.String() {
		case "enter":
			if m.timeline.body != nil && m.timeline.selectedAttachment < len(m.timeline.body.Attachments) {
				att := &m.timeline.body.Attachments[m.timeline.selectedAttachment]
				path := expandTilde(m.timeline.attachmentSaveInput.Value())
				m.timeline.attachmentSavePrompt = false
				m.timeline.attachmentSaveInput.Blur()
				return m, saveAttachmentCmd(m.backend, att, path), true
			}
			m.timeline.attachmentSavePrompt = false
			m.timeline.attachmentSaveInput.Blur()
		case "esc":
			m.timeline.attachmentSavePrompt = false
			m.timeline.attachmentSaveInput.Blur()
		default:
			var cmd tea.Cmd
			m.timeline.attachmentSaveInput, cmd = m.timeline.attachmentSaveInput.Update(msg)
			return m, cmd, true
		}
		return m, nil, true
	}

	if m.timeline.searchMode && m.activeTab == tabTimeline {
		switch msg.String() {
		case "esc":
			m.clearTimelineSearch()
			return m, nil, true
		case "ctrl+s":
			if q := m.timeline.searchInput.Value(); q != "" {
				return m, m.saveCurrentSearch(q), true
			}
		case "ctrl+i":
			return m, m.performIMAPSearch(m.timeline.searchInput.Value()), true
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit, true
		default:
			var cmd tea.Cmd
			m.timeline.searchInput, cmd = m.timeline.searchInput.Update(msg)
			return m, tea.Batch(cmd, m.performSearch(m.timeline.searchInput.Value())), true
		}
		return m, nil, true
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

func (m *Model) handleTabKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "1":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 0 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[0])
			return model, cmd, true
		}
		if !m.loading && m.activeTab != tabTimeline {
			return m, m.switchToTimeline(), true
		}
		return m, nil, true
	case "2":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 1 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[1])
			return model, cmd, true
		}
		if !m.loading && m.activeTab != tabCompose {
			return m, m.switchToCompose(), true
		}
		return m, nil, true
	case "3":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 2 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[2])
			return model, cmd, true
		}
		if !m.loading && m.activeTab != tabCleanup {
			return m, m.switchToCleanup(), true
		}
		return m, nil, true
	case "4":
		if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 3 {
			model, cmd := m.openQuickReply(m.timeline.quickReplies[3])
			return model, cmd, true
		}
		if !m.loading && m.activeTab != tabContacts {
			return m, m.switchToContacts(), true
		}
		return m, m.loadContacts(), true
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
		m.clearTimelinePreview()
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

func (m *Model) isGlobalQuit(msg tea.KeyMsg) bool {
	return msg.String() == "q" || msg.String() == "ctrl+c"
}

func (m *Model) isComposeGlobalPassThrough(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "1", "2", "3", "4":
		return true
	}
	return false
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
