package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
)

func TestKeyboardResolverProfilesAndLegacyAliases(t *testing.T) {
	resolver := NewKeyboardResolver(nil)

	tests := []struct {
		name    string
		scope   string
		mode    string
		key     string
		command string
	}{
		{name: "vim right", scope: "timeline", mode: "normal", key: "l", command: CommandPaneRight},
		{name: "vim left", scope: "timeline", mode: "normal", key: "h", command: CommandPaneLeft},
		{name: "new compose", scope: "timeline", mode: "normal", key: "c", command: CommandComposeNew},
		{name: "legacy compose", scope: "timeline", mode: "normal", key: "C", command: CommandComposeNew},
		{name: "reply all primary", scope: "timeline", mode: "normal", key: "r", command: CommandMailReplyAll},
		{name: "reply sender primary", scope: "timeline", mode: "normal", key: "R", command: CommandMailReplySender},
		{name: "forward primary", scope: "timeline", mode: "normal", key: "f", command: CommandMailForward},
		{name: "forward legacy", scope: "timeline", mode: "normal", key: "F", command: CommandMailForward},
		{name: "archive primary", scope: "timeline", mode: "normal", key: "a", command: CommandMailArchiveCurrent},
		{name: "archive legacy", scope: "timeline", mode: "normal", key: "e", command: CommandMailArchiveCurrent},
		{name: "classify relocated", scope: "timeline", mode: "normal", key: "T", command: CommandMailReclassify},
		{name: "sidebar primary", scope: "global", mode: "normal", key: "B", command: CommandSidebarToggle},
		{name: "logs primary", scope: "global", mode: "normal", key: "L", command: CommandLogsToggle},
		{name: "refresh primary", scope: "global", mode: "normal", key: "ctrl+r", command: CommandAppRefresh},
		{name: "tab legacy", scope: "global", mode: "normal", key: "f2", command: CommandTabCleanup},
		{name: "tab primary", scope: "global", mode: "normal", key: "2", command: CommandTabCleanup},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolver.Resolve(tc.scope, tc.mode, tc.key)
			if !ok {
				t.Fatalf("Resolve(%q,%q,%q) was not handled", tc.scope, tc.mode, tc.key)
			}
			if got != tc.command {
				t.Fatalf("Resolve(%q,%q,%q) = %q, want %q", tc.scope, tc.mode, tc.key, got, tc.command)
			}
		})
	}
}

func TestKeyboardResolverCustomKeymapValidationRejectsUnknownCommands(t *testing.T) {
	err := ValidateCustomKeymap([]byte(`
extends: vim
bindings:
  timeline:
    normal:
      x: mail.blast_everything
`))
	if err == nil {
		t.Fatal("expected validation error for unknown command")
	}
	if !strings.Contains(err.Error(), "mail.blast_everything") {
		t.Fatalf("validation error = %v, want unknown command id", err)
	}
}

func TestKeyboardResolverCustomKeymapCanOverrideKnownCommands(t *testing.T) {
	cfg := &config.Config{}
	cfg.Keyboard.Profile = "custom"
	cfg.Keyboard.CustomKeymap = ""
	resolver := NewKeyboardResolver(cfg)
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: vim
bindings:
  timeline:
    normal:
      x: mail.archive_current
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}

	got, ok := resolver.Resolve("timeline", "normal", "x")
	if !ok {
		t.Fatal("expected custom x binding to resolve")
	}
	if got != CommandMailArchiveCurrent {
		t.Fatalf("custom x resolved to %q, want %q", got, CommandMailArchiveCurrent)
	}
}

func TestCustomKeymapRoutesTimelineCommands(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: vim
bindings:
  timeline:
    normal:
      x: compose.new
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}
	m.keyboard = resolver

	model, cmd := m.handleKeyMsg(keyRunes("x"))
	updated := model.(*Model)
	if cmd != nil {
		t.Fatalf("expected custom compose binding to be synchronous, got %T", cmd)
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab = %d, want Compose", updated.activeTab)
	}
}

func TestCustomKeymapExtendsDefaultKeepsComposeInsertFirst(t *testing.T) {
	keymapPath := filepath.Join(t.TempDir(), "keys.yaml")
	if err := os.WriteFile(keymapPath, []byte(`
extends: default
bindings:
  timeline:
    normal:
      x: compose.new
`), 0o600); err != nil {
		t.Fatalf("write keymap: %v", err)
	}

	m := makeSizedModel(t, 140, 40)
	cfg := &config.Config{}
	cfg.Keyboard.Profile = "custom"
	cfg.Keyboard.CustomKeymap = keymapPath
	m.SetConfig(cfg)
	m.activeTab = tabCompose
	m.composeField = composeFieldTo
	m.composeTo.Focus()

	model, _ := m.handleKeyMsg(keyRunes("a"))
	updated := model.(*Model)
	if got := updated.composeTo.Value(); got != "a" {
		t.Fatalf("custom keymap extending default should keep Compose insert-first, got To=%q", got)
	}
	if updated.fieldKeyMode != "" {
		t.Fatalf("fieldKeyMode = %q, want empty insert-first mode", updated.fieldKeyMode)
	}
}

func TestCustomKeymapCanOptIntoModalComposeFields(t *testing.T) {
	keymapPath := filepath.Join(t.TempDir(), "keys.yaml")
	if err := os.WriteFile(keymapPath, []byte(`
extends: default
bindings:
  timeline:
    normal:
      x: compose.new
fields:
  compose:
    default_mode: normal
`), 0o600); err != nil {
		t.Fatalf("write keymap: %v", err)
	}

	m := makeSizedModel(t, 140, 40)
	cfg := &config.Config{}
	cfg.Keyboard.Profile = "custom"
	cfg.Keyboard.CustomKeymap = keymapPath
	m.SetConfig(cfg)
	m.activeTab = tabCompose
	m.composeField = composeFieldTo
	m.composeTo.Focus()

	if m.fieldKeyMode != keyboardModeNormal {
		t.Fatalf("fieldKeyMode = %q, want normal", m.fieldKeyMode)
	}

	model, _ := m.handleKeyMsg(keyRunes("x"))
	updated := model.(*Model)
	if got := updated.composeTo.Value(); got != "" {
		t.Fatalf("normal mode should swallow printable text, got To=%q", got)
	}

	model, _ = updated.handleKeyMsg(keyRunes("i"))
	updated = model.(*Model)
	if updated.fieldKeyMode != keyboardModeInsert {
		t.Fatalf("fieldKeyMode after i = %q, want insert", updated.fieldKeyMode)
	}
	model, _ = updated.handleKeyMsg(keyRunes("x"))
	updated = model.(*Model)
	if got := updated.composeTo.Value(); got != "x" {
		t.Fatalf("insert mode should type printable text, got To=%q", got)
	}
}

func TestComposeAltGlobalCommandsStayTextSafe(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeBody.Focus()
	m.composeBody.SetValue("draft")

	for _, key := range []tea.KeyPressMsg{altKey('l'), altKey('c'), altKey('f'), altKey('r')} {
		model, cmd := m.handleKeyMsg(key)
		if commandIsQuit(cmd) {
			t.Fatal("Alt-modified printable key should not quit from Compose")
		}
		m = model.(*Model)
	}

	if m.showLogs {
		t.Fatal("alt+l should not toggle logs while composing text")
	}
	if m.showChat {
		t.Fatal("alt+c should not toggle chat while composing text")
	}
	if m.loading {
		t.Fatal("alt+r should not refresh while composing text")
	}
	if m.activeTab != tabCompose {
		t.Fatalf("activeTab = %d, want Compose", m.activeTab)
	}
	if got := m.composeBody.Value(); !strings.HasPrefix(got, "draft") {
		t.Fatalf("compose body changed unexpectedly: %q", got)
	}
}
