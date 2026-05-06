package app

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
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
