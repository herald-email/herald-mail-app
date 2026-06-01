package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newComposeExternalEditorTestModel(t *testing.T) *Model {
	t.Helper()
	m := New(&stubBackend{}, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCompose
	return m
}

func TestComposeCtrlXStartsExternalEditorAndKeepsDraftUntilReturn(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")

	m := newComposeExternalEditorTestModel(t)
	m.composeField = composeFieldBody
	m.composeBody.Focus()
	m.composeBody.SetValue("draft before editor")

	model, cmd := m.handleComposeKey(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	updated := model.(*Model)

	if cmd == nil {
		t.Fatal("expected ctrl+x to launch the external editor command")
	}
	if got := updated.composeBody.Value(); got != "draft before editor" {
		t.Fatalf("compose body changed before editor returned: %q", got)
	}
	if !strings.Contains(updated.composeStatus, "Opening editor") {
		t.Fatalf("expected compose status to mention editor launch, got %q", updated.composeStatus)
	}
}

func TestRenderKeyHints_ComposeAdvertisesExternalEditor(t *testing.T) {
	m := newComposeExternalEditorTestModel(t)

	hints := m.renderKeyHints()
	if !strings.Contains(hints, "ctrl+x: editor") {
		t.Fatalf("expected hints to include external editor shortcut, got %q", hints)
	}
}

func TestComposeExternalEditorCommandPrefersVisualOverEditor(t *testing.T) {
	t.Setenv("VISUAL", "nvim -f")
	t.Setenv("EDITOR", "nano")

	cmd, args := composeExternalEditorCommand()

	if cmd != "nvim" {
		t.Fatalf("editor command = %q, want nvim", cmd)
	}
	if len(args) != 1 || args[0] != "-f" {
		t.Fatalf("editor args = %#v, want [-f]", args)
	}
}

func TestComposeExternalEditorResultUpdatesOnlyBody(t *testing.T) {
	m := newComposeExternalEditorTestModel(t)
	m.composeField = composeFieldSubject
	m.composeTo.SetValue("you@example.com")
	m.composeSubject.SetValue("Original subject")
	m.composeBody.SetValue("old body")

	m.applyComposeExternalEditorResult(composeExternalEditorFinishedMsg{Body: "new body\n"})

	if got := m.composeBody.Value(); got != "new body\n" {
		t.Fatalf("compose body = %q, want edited text", got)
	}
	if got := m.composeTo.Value(); got != "you@example.com" {
		t.Fatalf("compose to changed to %q", got)
	}
	if got := m.composeSubject.Value(); got != "Original subject" {
		t.Fatalf("compose subject changed to %q", got)
	}
	if m.composeField != composeFieldBody || !m.composeBody.Focused() {
		t.Fatalf("expected body focus after editor, field=%d focused=%v", m.composeField, m.composeBody.Focused())
	}
}

func TestComposeExternalEditorResultKeepsBodyOnError(t *testing.T) {
	m := newComposeExternalEditorTestModel(t)
	m.composeBody.SetValue("old body")

	m.applyComposeExternalEditorResult(composeExternalEditorFinishedMsg{Err: errors.New("exit status 1")})

	if got := m.composeBody.Value(); got != "old body" {
		t.Fatalf("compose body changed after editor error: %q", got)
	}
	if !strings.Contains(m.composeStatus, "Editor failed") {
		t.Fatalf("expected editor failure status, got %q", m.composeStatus)
	}
}
