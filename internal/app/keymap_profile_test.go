package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
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
		{name: "delete confirm primary", scope: "timeline", mode: "normal", key: "d", command: CommandMailDeleteConfirm},
		{name: "delete confirm backspace", scope: "timeline", mode: "normal", key: "backspace", command: CommandMailDeleteConfirm},
		{name: "delete immediate primary", scope: "timeline", mode: "normal", key: "D", command: CommandMailDeleteImmediate},
		{name: "delete immediate shift backspace", scope: "timeline", mode: "normal", key: "shift+backspace", command: CommandMailDeleteImmediate},
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
	m := makeSizedModel(t, 220, 50)
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

func TestCustomKeymapBottomHintsUseResolvedTimelineKeys(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  global:
    normal:
      b: sidebar.toggle
      o: logs.toggle
      7: tab.timeline
      8: tab.cleanup
      9: tab.contacts
  timeline:
    normal:
      n: compose.new
      p: mail.reply_all
      s: mail.reply_sender
      w: mail.forward
      x: mail.archive_current
      z: mail.delete_confirm
      y: mail.delete_immediate
      G: mail.reclassify
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}
	m.keyboard = resolver

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints,
		"7-9: tabs",
		"n: compose",
		"p: all",
		"s: sender",
		"w: forward",
		"z: delete",
		"y: delete now",
		"x: archive",
		"G: re-classify",
		"b: sidebar",
	)
	for _, stale := range []string{"1-3: tabs", "c: compose", "r: all", "R: sender", "f: forward", "a: archive", "T: re-classify", "B: sidebar"} {
		if strings.Contains(hints, stale) {
			t.Fatalf("expected custom keymap hints to omit stale %q, got:\n%s", stale, hints)
		}
	}
}

func TestTimelineDefaultHintAndHelpKeysResolveToHandlers(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	hints := stripANSI(m.renderKeyHints())
	helpEntries := m.shortcutHelpSections()
	for _, tc := range []struct {
		scope   string
		command string
		hint    string
		help    string
	}{
		{scope: "timeline", command: CommandComposeNew, hint: "compose", help: "open a blank Compose"},
		{scope: "timeline", command: CommandMailReplyAll, hint: "all", help: "reply all"},
		{scope: "timeline", command: CommandMailReplySender, hint: "sender", help: "reply sender"},
		{scope: "timeline", command: CommandMailForward, hint: "forward", help: "forward highlighted"},
		{scope: "timeline", command: CommandMailDeleteConfirm, hint: "delete", help: "after confirmation"},
		{scope: "timeline", command: CommandMailDeleteImmediate, hint: "delete now", help: "immediately"},
		{scope: "timeline", command: CommandMailArchiveCurrent, hint: "archive", help: "after confirmation"},
		{scope: keyboardScopeGlobal, command: CommandSidebarToggle, hint: "sidebar", help: "toggle sidebar"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			key := m.commandKey(tc.scope, tc.command)
			if key == "" {
				t.Fatalf("commandKey(%q, %q) was empty", tc.scope, tc.command)
			}
			got, ok := m.keyboard.Resolve(tc.scope, keyboardModeNormal, key)
			if !ok || got != tc.command {
				t.Fatalf("Resolve(%q, %q) = %q, %v; want %q, true", tc.scope, key, got, ok, tc.command)
			}
			if !strings.Contains(hints, displayShortcutKey(key, keyDisplayHint)+": "+tc.hint) {
				t.Fatalf("bottom hints missing %q for %s:\n%s", key+": "+tc.hint, tc.command, hints)
			}
			if !shortcutHelpSectionsContain(helpEntries, displayShortcutKey(key, keyDisplayHelp), tc.help) {
				t.Fatalf("shortcut help missing key %q or help containing %q for %s: %#v", key, tc.help, tc.command, helpEntries)
			}
		})
	}
}

func shortcutHelpSectionsContain(sections []shortcutHelpSection, key, desc string) bool {
	for _, section := range sections {
		for _, entry := range section.Entries {
			if strings.Contains(entry.Key, key) && strings.Contains(entry.Desc, desc) {
				return true
			}
		}
	}
	return false
}

func TestCustomKeymapOverriddenDefaultKeyIsNotAdvertisedForOldCommand(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  timeline:
    normal:
      r: mail.archive_current
      x: mail.reply_all
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}
	m.keyboard = resolver

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "x: all", "r: archive")
	if strings.Contains(hints, "r: all") {
		t.Fatalf("expected overwritten default key not to be advertised for reply-all, got:\n%s", hints)
	}

	for _, tc := range []struct {
		label   string
		key     string
		command string
	}{
		{label: "advertised reply-all", key: "x", command: CommandMailReplyAll},
		{label: "advertised archive", key: "r", command: CommandMailArchiveCurrent},
	} {
		t.Run(tc.label, func(t *testing.T) {
			got, ok := resolver.Resolve("timeline", keyboardModeNormal, tc.key)
			if !ok {
				t.Fatalf("expected %q to resolve", tc.key)
			}
			if got != tc.command {
				t.Fatalf("Resolve(%q) = %q, want %q", tc.key, got, tc.command)
			}
		})
	}
}

func TestCustomKeymapTabBarAndShortcutHelpUseResolvedKeys(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  global:
    normal:
      7: tab.timeline
      8: tab.cleanup
      9: tab.contacts
  timeline:
    normal:
      n: compose.new
      p: mail.reply_all
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}
	m.keyboard = resolver

	tabBar := stripANSI(m.renderTabBar())
	for _, want := range []string{"7  Timeline", "8  Cleanup", "9  Contacts"} {
		if !strings.Contains(tabBar, want) {
			t.Fatalf("expected custom tab label %q, got %q", want, tabBar)
		}
	}
	for _, stale := range []string{"1  Timeline", "2  Cleanup", "3  Contacts"} {
		if strings.Contains(tabBar, stale) {
			t.Fatalf("expected tab bar to omit stale %q, got %q", stale, tabBar)
		}
	}

	updated := pressQuestion(m)
	help := stripANSI(updated.View().Content)
	for _, want := range []string{"7-9", "switch tabs", "n", "open a blank Compose", "p", "reply all"} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected shortcut help to include %q, got:\n%s", want, help)
		}
	}
	for _, stale := range []string{"1-3", "c              open a blank Compose", "r / R / f"} {
		if strings.Contains(help, stale) {
			t.Fatalf("expected shortcut help to omit stale %q, got:\n%s", stale, help)
		}
	}
}

func TestCustomKeymapBottomHintsUseResolvedComposeCleanupAndContactsKeys(t *testing.T) {
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  global:
    normal:
      7: tab.timeline
      8: tab.cleanup
      9: tab.contacts
      b: sidebar.toggle
      o: logs.toggle
      y: chat.toggle
  cleanup:
    normal:
      x: mail.archive_current
      z: mail.delete_confirm
      v: mail.delete_immediate
      G: mail.reclassify
      h: mail.hide_future
  contacts:
    normal:
      n: pane.down
      p: pane.up
      ;: help.search
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}

	t.Run("compose tab switcher", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		m.keyboard = resolver
		m.activeTab = tabCompose

		hints := stripANSI(m.renderKeyHints())
		requireHintSegments(t, hints, "7-9: tabs", "ctrl+s: send", "ctrl+p: preview")
		if strings.Contains(hints, "1-3: tabs") || strings.Contains(hints, "?: help") {
			t.Fatalf("expected Compose hints to use custom tab keys and avoid browse help, got:\n%s", hints)
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		m := makeSizedModel(t, 180, 40)
		m.keyboard = resolver
		m.activeTab = tabCleanup
		m.stats = makeCleanupStats()
		m.updateSummaryTable()

		hints := stripANSI(m.renderKeyHints())
		requireHintSegments(t, hints, "7-9: tabs", "h: hide future mail", "z: delete", "v: delete now", "x: archive", "b: sidebar", "y: chat")
		for _, stale := range []string{"1-3: tabs", "H: hide future mail", "a: archive", "B: sidebar", "g: chat"} {
			if strings.Contains(hints, stale) {
				t.Fatalf("expected Cleanup custom hints to omit stale %q, got:\n%s", stale, hints)
			}
		}
	})

	t.Run("contacts", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.keyboard = resolver
		m.activeTab = tabContacts
		m.contactsList = []models.ContactData{{Email: "mara@forgepoint.example", DisplayName: "Mara Vale"}}
		m.contactsFiltered = m.contactsList

		hints := stripANSI(m.renderKeyHints())
		requireHintSegments(t, hints, "7-9: tabs", "↑/p ↓/n: nav", ";: search")
		for _, stale := range []string{"1-3: tabs", "↑/k ↓/j: nav", "/: search"} {
			if strings.Contains(hints, stale) {
				t.Fatalf("expected Contacts custom hints to omit stale %q, got:\n%s", stale, hints)
			}
		}
	})
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

func TestDeleteShortcutAliasesDoNotStealTextEntry(t *testing.T) {
	t.Run("compose", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.activeTab = tabCompose
		m.composeField = composeFieldBody
		m.composeBody.Focus()

		model, _ := m.handleKeyMsg(keyRunes("d"))
		updated := model.(*Model)
		model, _ = updated.handleKeyMsg(keyRunes("D"))
		updated = model.(*Model)
		model, _ = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyBackspace})
		updated = model.(*Model)

		if got := updated.composeBody.Value(); got != "d" {
			t.Fatalf("expected compose field to keep literal delete-alias text editing, got %q", got)
		}
		if updated.pendingDeleteConfirm {
			t.Fatal("compose text entry must not open delete confirmation")
		}
	})

	t.Run("search prompt", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.activeTab = tabTimeline
		m.timeline.emails = mockEmails()
		m.updateTimelineTable()
		m.openTimelineSearch()

		model, _ := m.handleKeyMsg(keyRunes("d"))
		updated := model.(*Model)
		model, _ = updated.handleKeyMsg(keyRunes("D"))
		updated = model.(*Model)
		model, _ = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyBackspace})
		updated = model.(*Model)

		if got := updated.timeline.searchInput.Value(); got != "d" {
			t.Fatalf("expected search prompt to keep literal delete-alias text editing, got %q", got)
		}
		if updated.pendingDeleteConfirm {
			t.Fatal("search prompt text entry must not open delete confirmation")
		}
	})

	t.Run("prompt editor", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.showPromptEditor = true
		m.promptEditor = NewPromptEditor(nil, 140, 40)
		if cmd := m.promptEditor.Init(); cmd != nil {
			model, _ := m.Update(cmd())
			m = model.(*Model)
		}

		model, _ := m.Update(keyRunes("d"))
		updated := model.(*Model)
		model, _ = updated.Update(keyRunes("D"))
		updated = model.(*Model)
		model, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		updated = model.(*Model)

		if got := updated.promptEditor.name; got != "d" {
			t.Fatalf("expected prompt editor to keep literal delete-alias text editing, got %q", got)
		}
		if updated.pendingDeleteConfirm {
			t.Fatal("prompt editor text entry must not open delete confirmation")
		}
	})
}

func TestVimComposeEscapeLeavesInsertAndVisualModes(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	cfg := &config.Config{}
	cfg.Keyboard.Profile = "vim"
	m.SetConfig(cfg)
	m.activeTab = tabCompose
	m.composeField = composeFieldTo
	m.composeTo.Focus()

	model, _ := m.handleKeyMsg(keyRunes("i"))
	updated := model.(*Model)
	if updated.fieldKeyMode != keyboardModeInsert {
		t.Fatalf("fieldKeyMode after i = %q, want insert", updated.fieldKeyMode)
	}
	model, _ = updated.handleKeyMsg(keyRunes("x"))
	updated = model.(*Model)
	if got := updated.composeTo.Value(); got != "x" {
		t.Fatalf("insert mode should type printable text, got To=%q", got)
	}

	model, _ = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated = model.(*Model)
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab after insert esc = %d, want Compose", updated.activeTab)
	}
	if updated.fieldKeyMode != keyboardModeNormal {
		t.Fatalf("fieldKeyMode after insert esc = %q, want normal", updated.fieldKeyMode)
	}

	model, _ = updated.handleKeyMsg(keyRunes("v"))
	updated = model.(*Model)
	if updated.fieldKeyMode != keyboardModeVisual {
		t.Fatalf("fieldKeyMode after v = %q, want visual", updated.fieldKeyMode)
	}
	model, _ = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated = model.(*Model)
	if updated.activeTab != tabCompose {
		t.Fatalf("activeTab after visual esc = %d, want Compose", updated.activeTab)
	}
	if updated.fieldKeyMode != keyboardModeNormal {
		t.Fatalf("fieldKeyMode after visual esc = %q, want normal", updated.fieldKeyMode)
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
