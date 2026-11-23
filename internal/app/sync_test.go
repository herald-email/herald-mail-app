package app

import (
	"testing"

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
