package app

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestStartSync_WithoutConfigFallsBackToPolling(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.loading = false
	m.cfg = nil

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("startSync panicked without config: %v", r)
		}
	}()

	cmd := m.startSync("INBOX")
	if cmd == nil {
		t.Fatal("expected fallback polling command when config is nil")
	}
}

func TestStartSync_DemoModeDoesNotStartPollingOrListeners(t *testing.T) {
	backend := &stubBackend{}
	m := New(backend, nil, "", nil, false)
	m.demoMode = true
	m.cfg = &config.Config{}
	m.cfg.Sync.Interval = 60

	cmd := m.startSync("INBOX")

	if cmd != nil {
		t.Fatal("expected demo sync to avoid listener/ticker commands")
	}
	if backend.startPollingCalls != 0 {
		t.Fatalf("StartPolling calls = %d, want 0", backend.startPollingCalls)
	}
	if backend.startIDLECalls != 0 {
		t.Fatalf("StartIDLE calls = %d, want 0", backend.startIDLECalls)
	}
	if m.syncStatusMode != "off" {
		t.Fatalf("syncStatusMode = %q, want off", m.syncStatusMode)
	}
	if m.syncCountdown != 0 {
		t.Fatalf("syncCountdown = %d, want 0", m.syncCountdown)
	}
}

func TestStartSyncSameFolderKeepsPollingSchedule(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.cfg = &config.Config{}
	m.cfg.Sync.Interval = 60
	m.currentFolder = "INBOX"
	m.startSync("INBOX")
	m.syncCountdown = 12
	if cmd := m.startSync("INBOX"); cmd != nil {
		t.Fatal("same-folder sync created duplicate listeners")
	}
	if b.startPollingCalls != 1 || m.syncCountdown != 12 {
		t.Fatalf("poll restarted: calls=%d countdown=%d", b.startPollingCalls, m.syncCountdown)
	}
	m.currentFolder = "Archive"
	m.startSync("Archive")
	if b.startPollingCalls != 2 {
		t.Fatal("folder switch did not restart polling")
	}
}

func TestSyncCompletionKeepsPollingSchedule(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.startSync("INBOX")
	m.syncCountdown = 12
	m.Update(SyncEventMsg{Event: models.FolderSyncEvent{Folder: "INBOX", Generation: 1, Phase: models.SyncPhaseComplete}})
	if b.startPollingCalls != 1 || m.syncCountdown != 12 {
		t.Fatalf("completion reset polling: calls=%d countdown=%d", b.startPollingCalls, m.syncCountdown)
	}
	m.activeSourceID = "other-mail"
	m.startSync("INBOX")
	if b.startPollingCalls != 2 {
		t.Fatal("source switch did not restart polling")
	}
}

func TestStartSyncRestartsIDLEAfterForegroundSync(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.cfg = &config.Config{}
	m.cfg.Sync.Idle = true
	m.currentFolder = "INBOX"
	m.startSync("INBOX")
	m.startSync("INBOX")
	if b.startIDLECalls != 2 {
		t.Fatal("foreground sync must retain IDLE recovery behavior")
	}
}

func TestMultiAccountPollingStaysActiveWhenBrowsing(t *testing.T) {
	first, second := &stubBackend{}, &stubBackend{}
	multi, err := backend.NewMultiBackend([]backend.AccountBackend{
		{Info: backend.AccountInfo{SourceID: "gmail"}, Backend: first},
		{Info: backend.AccountInfo{SourceID: "imap"}, Backend: second},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = multi.Close() })
	m := New(multi, nil, "", nil, false)
	m.cfg = &config.Config{}
	m.cfg.Sync.Idle = true
	m.Init()
	if m.syncStatusMode != "polling-accounts" || first.startPollingCalls == 0 || second.startPollingCalls == 0 {
		t.Fatal("startup did not poll both accounts")
	}
	m.currentFolder = virtualFolderAllMailOnly
	m.activateCurrentFolder()
	if m.syncStatusMode != "polling-accounts" || first.stopPollingCalls != 0 || second.stopPollingCalls != 0 {
		t.Fatal("diagnostic folder stopped account-wide polling")
	}
	if m.startSync("INBOX") != nil {
		t.Fatal("view change installed a second mail listener")
	}
	if first.startPollingCalls != 1 || second.startPollingCalls != 1 {
		t.Fatal("view changes restarted an Inbox worker")
	}
	if first.startIDLECalls != 0 || second.startIDLECalls != 0 {
		t.Fatal("multi-account polling opened an IMAP IDLE session")
	}
}

func TestAllInboxesKeepsSameMessageIDFromBothAccounts(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.activeSourceID = backend.AllAccountsSourceID
	m.currentFolder = "INBOX"
	for _, source := range []models.SourceID{"first", "second"} {
		m.Update(NewEmailsMsg{
			SourceID: source,
			Folder:   "INBOX",
			Emails:   []*models.EmailData{{SourceID: source, Folder: "INBOX", MessageID: "<shared@example.test>"}},
		})
	}
	if len(m.timeline.emails) != 2 {
		t.Fatalf("All Inboxes has %d account copies, want 2", len(m.timeline.emails))
	}
}

func TestOldPollingListenerDoesNotRearmAfterBackendReplacement(t *testing.T) {
	old := &stubBackend{newEmailsCh: make(chan models.NewEmailsNotification, 1)}
	previous := &Model{backend: old}
	listen := previous.listenForNewEmails()
	old.newEmailsCh <- models.NewEmailsNotification{
		Folder: "INBOX",
		Emails: []*models.EmailData{{MessageID: "old-backend", Folder: "INBOX"}},
	}
	m := New(&stubBackend{newEmailsCh: make(chan models.NewEmailsNotification, 1)}, nil, "", nil, false)
	m.currentFolder = "INBOX"
	if _, cmd := m.Update(listen()); cmd != nil || len(m.timeline.emails) != 0 {
		t.Fatal("retired subscription updated the Timeline or started another listener")
	}
}
