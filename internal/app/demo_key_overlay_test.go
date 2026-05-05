package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDemoKeyOverlayDisabledByDefault(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, _ := m.Update(keyRunes("?"))
	updated := model.(*Model)

	rendered := stripANSI(updated.View().Content)
	if strings.Contains(rendered, "Keys:") {
		t.Fatalf("demo key overlay should be disabled by default, got:\n%s", rendered)
	}
}

func TestDemoKeyOverlayRecordsShortcutLabels(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.SetDemoKeyOverlay(true)

	model, _ := m.Update(keyRunes("?"))
	updated := model.(*Model)

	rendered := stripANSI(updated.View().Content)
	if !strings.Contains(rendered, "Keys: ?") {
		t.Fatalf("expected key overlay to show ? shortcut, got:\n%s", rendered)
	}

	model, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	updated = model.(*Model)
	rendered = stripANSI(updated.View().Content)
	if !strings.Contains(rendered, "Shift+Down") {
		t.Fatalf("expected key overlay to normalize shifted arrow, got:\n%s", rendered)
	}
}

func TestDemoKeyOverlayFormatsKnownMediaKeys(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{name: "settings", msg: keyRunes("S"), want: "S"},
		{name: "help", msg: keyRunes("?"), want: "?"},
		{name: "cleanup tab", msg: keyRunes("2"), want: "2"},
		{name: "cleanup manager", msg: keyRunes("C"), want: "C"},
		{name: "full screen", msg: keyRunes("z"), want: "z"},
		{name: "right arrow", msg: tea.KeyPressMsg{Code: tea.KeyRight}, want: "Right"},
		{name: "left arrow", msg: tea.KeyPressMsg{Code: tea.KeyLeft}, want: "Left"},
		{name: "shift down", msg: tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}, want: "Shift+Down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := demoKeyOverlayLabel(tt.msg); got != tt.want {
				t.Fatalf("demoKeyOverlayLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDemoKeyOverlayDoesNotRecordComposeText(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.SetDemoKeyOverlay(true)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeBody.Focus()

	model, _ := m.Update(keyRunes("plain text"))
	updated := model.(*Model)

	if got := updated.composeBody.Value(); !strings.Contains(got, "plain text") {
		t.Fatalf("expected literal compose text to be preserved, got %q", got)
	}
	if strings.Contains(stripANSI(updated.View().Content), "Keys:") {
		t.Fatalf("compose text should not be recorded in demo key overlay, got:\n%s", stripANSI(updated.View().Content))
	}
}

func TestDemoKeyOverlayDoesNotRecordSearchPromptText(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.SetDemoKeyOverlay(true)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	model, _ := m.Update(keyRunes("/"))
	updated := model.(*Model)
	model, _ = updated.Update(keyRunes("?"))
	updated = model.(*Model)

	if got := updated.timeline.searchInput.Value(); got != "?" {
		t.Fatalf("expected literal ? in search prompt, got %q", got)
	}
	if strings.Contains(stripANSI(updated.View().Content), "Keys: ?") {
		t.Fatalf("search prompt text should not be recorded as help shortcut, got:\n%s", stripANSI(updated.View().Content))
	}
}

func TestDemoKeyOverlayDoesNotRecordEditorText(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*Model)
	}{
		{
			name: "rule editor",
			setup: func(m *Model) {
				m.activeTab = tabCleanup
				m.showRuleEditor = true
			},
		},
		{
			name: "prompt editor",
			setup: func(m *Model) {
				m.showPromptEditor = true
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := makeSizedModel(t, 120, 40)
			m.SetDemoKeyOverlay(true)
			tt.setup(m)

			model, _ := m.Update(keyRunes("?"))
			updated := model.(*Model)

			if strings.Contains(stripANSI(updated.View().Content), "Keys: ?") {
				t.Fatalf("editor text should not be recorded as help shortcut, got:\n%s", stripANSI(updated.View().Content))
			}
		})
	}
}
