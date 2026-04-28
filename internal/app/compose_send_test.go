package app

import (
	"testing"

	"mail-processor/internal/backend"
	"mail-processor/internal/config"
	appsmtp "mail-processor/internal/smtp"
)

func TestDemoComposeSendSimulatesSuccessWithoutSMTPConfig(t *testing.T) {
	m := New(backend.NewDemoBackend(), appsmtp.New(&config.Config{}), "demo@demo.local", nil, false)
	m.composeTo.SetValue("recipient@example.com")
	m.composeSubject.SetValue("Demo send")
	m.composeBody.SetValue("Hello from demo mode.")

	cmd := m.sendCompose()
	raw := cmd()
	msg, ok := raw.(ComposeStatusMsg)
	if !ok {
		t.Fatalf("sendCompose returned %T, want ComposeStatusMsg", raw)
	}
	if msg.Err != nil {
		t.Fatalf("demo send returned error: %v", msg.Err)
	}
	if msg.Message != "Message sent!" {
		t.Fatalf("demo send message = %q, want %q", msg.Message, "Message sent!")
	}
}
