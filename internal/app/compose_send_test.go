package app

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
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

func TestComposeSendSuccessReturnsToTimeline(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeReturnSet = true
	m.composeReturnTab = tabTimeline
	m.composeReturnPanel = panelTimeline
	m.composeTo.SetValue("recipient@example.com")
	m.composeSubject.SetValue("Demo send")
	m.composeBody.SetValue("Hello from demo mode.")

	model, cmd := m.Update(ComposeStatusMsg{Message: "Message sent!"})
	updated := model.(*Model)

	if cmd != nil {
		t.Fatalf("send success without a tracked draft returned command %T", cmd)
	}
	if updated.activeTab != tabTimeline {
		t.Fatalf("activeTab=%d, want Timeline", updated.activeTab)
	}
	if updated.focusedPanel != panelTimeline {
		t.Fatalf("focusedPanel=%d, want Timeline panel", updated.focusedPanel)
	}
	if updated.statusMessage != "Message sent!" {
		t.Fatalf("statusMessage=%q, want send status on Timeline", updated.statusMessage)
	}
	if updated.composeTo.Value() != "" || updated.composeSubject.Value() != "" || updated.composeBody.Value() != "" {
		t.Fatalf("expected send success to clear compose fields, got to=%q subject=%q body=%q", updated.composeTo.Value(), updated.composeSubject.Value(), updated.composeBody.Value())
	}
}
