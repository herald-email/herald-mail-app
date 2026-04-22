package app

import "testing"

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
