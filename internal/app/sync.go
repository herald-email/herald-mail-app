package app

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	imapClient "github.com/herald-email/herald-mail-app/internal/imap"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// --- Background sync helpers ---

// listenForNewEmails returns a Cmd that blocks on the backend's new-emails channel.
func (m *Model) listenForNewEmails() tea.Cmd {
	ch := m.backend.NewEmailsCh()
	return func() tea.Msg {
		notif, ok := <-ch
		if !ok {
			return nil
		}
		return NewEmailsMsg{
			subscription: ch,
			SourceID:     models.NormalizeSourceID(notif.SourceID, models.DefaultMailSourceID),
			AccountID:    models.NormalizeAccountID(notif.AccountID),
			Emails:       notif.Emails,
			Folder:       notif.Folder,
		}
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
	if isVirtualAllMailOnlyFolder(m.currentFolder) {
		m.syncStatusMode = ""
		m.syncCountdown = 0
		return nil
	}
	if interval <= 0 {
		logger.Warn("startPolling called with non-positive interval %d; using 60s fallback", interval)
		interval = 60
	}
	m.syncStatusMode = "polling"
	m.syncCountdown = interval
	m.backend.StartPolling(m.currentFolder, interval)
	return tea.Batch(m.listenForNewEmails(), m.tickSyncCountdown())
}

// startMultiAccountPolling follows the visible folder without restarting the Inbox workers.
func (m *Model) startMultiAccountPolling(folder string) tea.Cmd {
	if !m.usesMultiAccountPolling() {
		return nil
	}
	if isVirtualAllMailOnlyFolder(folder) {
		folder = ""
	}
	interval := 60
	if m.cfg != nil && m.cfg.Sync.Interval > 0 {
		interval = m.cfg.Sync.Interval
	}
	m.backend.StartPolling(folder, interval)
	m.syncStatusMode = "polling-accounts"
	if m.multiPollingListening {
		return nil
	}
	m.multiPollingListening = true
	return m.listenForNewEmails()
}

func (m *Model) usesMultiAccountPolling() bool {
	multi, ok := m.backend.(*backend.MultiBackend)
	return ok && multi.HasMultipleAccounts() && !m.demoMode
}

// startSync tries IDLE first (if enabled in config), falling back to polling.
func (m *Model) startSync(folder string) tea.Cmd {
	if m.usesMultiAccountPolling() {
		return m.startMultiAccountPolling(folder)
	}
	// Keep the timer and listeners alive when only the folder snapshot changed.
	if m.syncFolder == folder && m.syncSourceID == m.activeSourceID && m.syncStatusMode == "polling" {
		return nil
	}
	m.syncFolder = folder
	m.syncSourceID = m.activeSourceID
	// Stop any running sync before starting a new one.
	m.backend.StopIDLE()
	m.backend.StopPolling()
	if isVirtualAllMailOnlyFolder(folder) {
		m.syncStatusMode = ""
		m.syncCountdown = 0
		return nil
	}
	if m.demoMode {
		m.syncStatusMode = "off"
		m.syncCountdown = 0
		logger.Info("Demo mode sync listeners disabled; demo data is served from in-memory fixtures")
		return nil
	}

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
