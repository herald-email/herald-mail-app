package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/deeplink"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/notifications"
)

type NotificationActivatedMsg struct {
	DeepLink string
}

func (m *Model) SetNotifier(notifier notifications.Notifier) {
	m.notifier = notifier
}

func (m *Model) SetInitialDeepLink(raw string) error {
	target, err := deeplink.Parse(raw)
	if err != nil {
		return err
	}
	m.initialDeepLink = &target
	return nil
}

func (m *Model) prepareInitialDeepLink() tea.Cmd {
	if m.initialDeepLink == nil {
		return nil
	}
	target := *m.initialDeepLink
	m.initialDeepLink = nil
	if target.Kind != deeplink.KindCompose && target.Folder != "" && target.Folder != m.currentFolder {
		m.resetMailboxStateForFolder(target.Folder)
	}
	if target.Kind == deeplink.KindCompose || !m.loading {
		return m.applyDeepLinkTarget(target)
	}
	m.pendingDeepLink = &target
	return nil
}

func (m *Model) listenForNotificationResponses() tea.Cmd {
	if m.notifier == nil {
		return nil
	}
	ch := m.notifier.Responses()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		link, ok := <-ch
		if !ok {
			return nil
		}
		return NotificationActivatedMsg{DeepLink: link}
	}
}

func (m *Model) applyDeepLinkTarget(target deeplink.Target) tea.Cmd {
	if cmd := m.switchAccountForDeepLink(target); cmd != nil {
		return cmd
	}

	switch target.Kind {
	case deeplink.KindCompose:
		m.activeTab = tabCompose
		m.showSettings = false
		m.showRuleEditor = false
		m.composeTo.SetValue(target.To)
		m.composeSubject.SetValue(target.Subject)
		m.composeField = 0
		m.composeTo.Focus()
		m.composeSubject.Blur()
		m.composeBody.Blur()
		m.statusMessage = "Opened compose link."
		return nil

	case deeplink.KindFolder:
		m.activeTab = tabTimeline
		m.showSettings = false
		m.showRuleEditor = false
		if target.Folder != "" && target.Folder != m.currentFolder {
			m.resetMailboxStateForFolder(target.Folder)
			m.hydrateCachedTimelineForCurrentFolder()
			m.statusMessage = "Opened " + displayFolderName(target.Folder) + "."
			return m.activateCurrentFolder()
		}
		m.clearTimelinePreview()
		m.statusMessage = "Opened " + displayFolderName(m.currentFolder) + "."
		return nil

	case deeplink.KindMessage:
		m.activeTab = tabTimeline
		m.showSettings = false
		m.showRuleEditor = false
		if cmd := m.ensureDeepLinkFolderLoaded(target); cmd != nil {
			return cmd
		}
		if email, row := m.findDeepLinkEmail(target); email != nil {
			if row >= 0 {
				m.timelineTable.SetCursor(row)
			}
			return m.openTimelineEmail(email)
		}
		if m.loading {
			m.pendingDeepLink = &target
		} else {
			m.pendingDeepLink = nil
			m.statusMessage = "Message link target is not in this folder yet."
		}
		return nil

	case deeplink.KindSender:
		m.activeTab = tabTimeline
		m.showSettings = false
		m.showRuleEditor = false
		if cmd := m.ensureDeepLinkFolderLoaded(target); cmd != nil {
			return cmd
		}
		return m.openDeepLinkSearch(target.Sender)

	case deeplink.KindSearch:
		m.activeTab = tabTimeline
		m.showSettings = false
		m.showRuleEditor = false
		if cmd := m.ensureDeepLinkFolderLoaded(target); cmd != nil {
			return cmd
		}
		return m.openDeepLinkSearch(target.Query)
	}
	return nil
}

func (m *Model) switchAccountForDeepLink(target deeplink.Target) tea.Cmd {
	if target.Kind == deeplink.KindCompose || target.SourceID == "" || target.SourceID == m.activeSourceID {
		return nil
	}
	if _, ok := m.backend.(backend.AccountAwareBackend); !ok {
		return nil
	}
	if m.accountSelectedFolders == nil {
		m.accountSelectedFolders = make(map[models.SourceID]string)
	}
	if target.Folder != "" {
		m.accountSelectedFolders[target.SourceID] = target.Folder
	}
	m.pendingDeepLink = &target
	return m.switchActiveAccount(target.SourceID)
}

func (m *Model) ensureDeepLinkFolderLoaded(target deeplink.Target) tea.Cmd {
	if target.Folder == "" || target.Folder == m.currentFolder {
		return nil
	}
	m.pendingDeepLink = &target
	m.resetMailboxStateForFolder(target.Folder)
	m.hydrateCachedTimelineForCurrentFolder()
	return m.activateCurrentFolder()
}

func (m *Model) consumePendingDeepLinkCmd() tea.Cmd {
	if m.pendingDeepLink == nil {
		return nil
	}
	target := *m.pendingDeepLink
	m.pendingDeepLink = nil
	return m.applyDeepLinkTarget(target)
}

func (m *Model) openDeepLinkSearch(query string) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue(query)
	m.timeline.searchToken++
	token := m.timeline.searchToken
	m.timeline.searchAutoFocusResults = true
	m.statusMessage = "Opened search: " + query
	return m.performSearchWithToken(query, token)
}

func (m *Model) findDeepLinkEmail(target deeplink.Target) (*models.EmailData, int) {
	for rowIdx, ref := range m.timeline.threadRowMap {
		for _, email := range m.timelineRowEmails(ref) {
			if deepLinkMatchesEmail(target, email) {
				return email, rowIdx
			}
		}
	}
	for _, email := range m.timeline.emails {
		if deepLinkMatchesEmail(target, email) {
			return email, -1
		}
	}
	return nil, -1
}

func deepLinkMatchesEmail(target deeplink.Target, email *models.EmailData) bool {
	if email == nil {
		return false
	}
	ref := email.MessageRef()
	if target.LocalID != "" && ref.LocalID == target.LocalID {
		return true
	}
	return target.MessageID != "" && email.MessageID == target.MessageID
}

func (m *Model) notifyNewMailCmd(msg NewEmailsMsg) tea.Cmd {
	if !m.notificationEventEnabled(func(cfg notificationsConfigView) bool { return cfg.NewMail }) || len(msg.Emails) == 0 {
		return nil
	}
	folder := strings.TrimSpace(msg.Folder)
	if folder == "" {
		folder = m.currentFolder
	}
	req := notifications.Request{
		ID:    "new-mail-" + folder,
		Title: "New mail",
		Sound: m.cfg.Notifications.Sound,
	}
	if len(msg.Emails) == 1 {
		email := msg.Emails[0]
		if email == nil {
			return nil
		}
		if email.Folder == "" {
			email.Folder = folder
		}
		req.ID = email.MessageID
		req.Body = fmt.Sprintf("%s: %s", strings.TrimSpace(email.Sender), strings.TrimSpace(email.Subject))
		req.DeepLink = deeplink.Build(deeplink.MessageTarget(email))
	} else {
		req.Body = fmt.Sprintf("%d new messages in %s", len(msg.Emails), displayFolderName(folder))
		req.DeepLink = deeplink.Build(deeplink.FolderTarget(folder, msg.SourceID, msg.AccountID))
	}
	return m.deliverNotificationCmd(req)
}

func (m *Model) notifySyncFailureCmd(event models.FolderSyncEvent) tea.Cmd {
	if !m.notificationEventEnabled(func(cfg notificationsConfigView) bool { return cfg.SyncFailures }) {
		return nil
	}
	folder := strings.TrimSpace(event.Folder)
	if folder == "" {
		folder = m.currentFolder
	}
	body := strings.TrimSpace(event.Message)
	if body == "" {
		body = strings.TrimSpace(event.Error)
	}
	if body == "" {
		body = "Sync failed."
	}
	return m.deliverNotificationCmd(notifications.Request{
		ID:       "sync-failure-" + folder,
		Title:    "Herald sync failed",
		Body:     body,
		DeepLink: deeplink.Build(deeplink.FolderTarget(folder, event.SourceID, event.AccountID)),
		Sound:    m.cfg.Notifications.Sound,
	})
}

func (m *Model) notifyDeletionCompletionCmd(result models.DeletionResult, total int) tea.Cmd {
	if !m.notificationEventEnabled(func(cfg notificationsConfigView) bool { return cfg.DeletionCompletion }) {
		return nil
	}
	folder := strings.TrimSpace(result.Folder)
	if folder == "" {
		folder = m.currentFolder
	}
	action := "Deletion"
	if result.IsArchive {
		action = "Archive"
	}
	body := fmt.Sprintf("%s completed for %d messages.", action, total)
	if total == 1 {
		body = action + " completed for 1 message."
	}
	return m.deliverNotificationCmd(notifications.Request{
		ID:       "mail-action-" + folder,
		Title:    "Herald " + strings.ToLower(action) + " complete",
		Body:     body,
		DeepLink: deeplink.Build(deeplink.FolderTarget(folder, result.SourceID, result.AccountID)),
		Sound:    m.cfg.Notifications.Sound,
	})
}

func (m *Model) notifyClassificationCompletionCmd() tea.Cmd {
	if !m.notificationEventEnabled(func(cfg notificationsConfigView) bool { return cfg.ClassificationCompletion }) {
		return nil
	}
	return m.deliverNotificationCmd(notifications.Request{
		ID:       "classification-" + m.currentFolder,
		Title:    "Classification complete",
		Body:     fmt.Sprintf("%d messages tagged in %s.", m.classifyDone, displayFolderName(m.currentFolder)),
		DeepLink: deeplink.Build(deeplink.FolderTarget(m.currentFolder, m.activeSourceID, m.activeAccountID)),
		Sound:    m.cfg.Notifications.Sound,
	})
}

func (m *Model) notifyChatResultCmd(msg ChatResponseMsg) tea.Cmd {
	if !m.notificationEventEnabled(func(cfg notificationsConfigView) bool { return cfg.ChatResults }) {
		return nil
	}
	title := "Herald chat result"
	body := strings.TrimSpace(msg.Content)
	if msg.Err != nil {
		title = "Herald chat failed"
		body = msg.Err.Error()
	}
	if body == "" {
		body = "Chat finished."
	}
	if len([]rune(body)) > 180 {
		body = string([]rune(body)[:177]) + "..."
	}
	return m.deliverNotificationCmd(notifications.Request{
		ID:       "chat-" + m.currentFolder,
		Title:    title,
		Body:     body,
		DeepLink: deeplink.Build(deeplink.FolderTarget(m.currentFolder, m.activeSourceID, m.activeAccountID)),
		Sound:    m.cfg.Notifications.Sound,
	})
}

type notificationsConfigView struct {
	NewMail                  bool
	SyncFailures             bool
	DeletionCompletion       bool
	ClassificationCompletion bool
	ChatResults              bool
}

func (m *Model) notificationEventEnabled(event func(notificationsConfigView) bool) bool {
	if m == nil || m.notifier == nil || m.cfg == nil || !m.cfg.Notifications.Enabled {
		return false
	}
	view := notificationsConfigView{
		NewMail:                  m.cfg.Notifications.NewMail,
		SyncFailures:             m.cfg.Notifications.SyncFailures,
		DeletionCompletion:       m.cfg.Notifications.DeletionCompletion,
		ClassificationCompletion: m.cfg.Notifications.ClassificationCompletion,
		ChatResults:              m.cfg.Notifications.ChatResults,
	}
	return event(view)
}

func (m *Model) deliverNotificationCmd(req notifications.Request) tea.Cmd {
	if m.notifier == nil {
		return nil
	}
	return func() tea.Msg {
		if err := m.notifier.Notify(context.Background(), req); err != nil {
			logger.Warn("notification delivery failed: %v", err)
		}
		return nil
	}
}
