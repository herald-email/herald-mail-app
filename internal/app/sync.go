package app

import (
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	imapClient "mail-processor/internal/imap"
	"mail-processor/internal/logger"
)

// --- Background sync helpers ---

// listenForNewEmails returns a Cmd that blocks on the backend's new-emails channel.
func (m *Model) listenForNewEmails() tea.Cmd {
	ch := m.backend.NewEmailsCh()
	return func() tea.Msg {
		notif := <-ch
		return NewEmailsMsg{Emails: notif.Emails, Folder: notif.Folder}
	}
}

// listenForExpunged is a no-op stub (IMAP expunge notifications not yet implemented).
func (m *Model) listenForExpunged() tea.Cmd {
	return nil
}

// tickSyncCountdown drives the sync countdown ticker.
func (m *Model) tickSyncCountdown() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		return SyncTickMsg{}
	})
}

// startPolling starts background polling and the sync countdown timer.
func (m *Model) startPolling(interval int) tea.Cmd {
	m.syncStatusMode = "polling"
	m.syncCountdown = interval
	m.backend.StartPolling(m.currentFolder, interval)
	return tea.Batch(m.listenForNewEmails(), m.tickSyncCountdown())
}

// startSync tries IDLE first (if enabled in config), falling back to polling.
func (m *Model) startSync(folder string) tea.Cmd {
	// Stop any running sync before starting a new one.
	m.backend.StopIDLE()
	m.backend.StopPolling()

	if m.cfg == nil {
		logger.Warn("startSync called before config was attached; falling back to 60s polling")
		return m.startPolling(60)
	}

	if m.cfg.Sync.Idle {
		if err := m.backend.StartIDLE(folder); err == nil {
			m.syncStatusMode = "idle"
			return m.listenForNewEmails()
		} else if !errors.Is(err, imapClient.ErrIDLENotSupported) {
			logger.Warn("IDLE failed, falling back to polling: %v", err)
		}
	}
	return m.startPolling(m.cfg.Sync.Interval)
}
