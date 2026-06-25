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
		{name: "arrow right primary", scope: "timeline", mode: "normal", key: "right", command: CommandPaneRight},
		{name: "vim right legacy", scope: "timeline", mode: "normal", key: "l", command: CommandPaneRight},
		{name: "new compose primary", scope: "timeline", mode: "normal", key: "ctrl+n", command: CommandComposeNew},
		{name: "new compose legacy", scope: "timeline", mode: "normal", key: "c", command: CommandComposeNew},
		{name: "reply sender primary", scope: "timeline", mode: "normal", key: "ctrl+r", command: CommandMailReplySender},
		{name: "reply sender single-letter", scope: "timeline", mode: "normal", key: "r", command: CommandMailReplySender},
		{name: "reply all primary", scope: "timeline", mode: "normal", key: "ctrl+shift+r", command: CommandMailReplyAll},
		{name: "reply all terminal shifted alias", scope: "timeline", mode: "normal", key: "ctrl+R", command: CommandMailReplyAll},
		{name: "reply all single-letter", scope: "timeline", mode: "normal", key: "R", command: CommandMailReplyAll},
		{name: "forward primary", scope: "timeline", mode: "normal", key: "ctrl+f", command: CommandMailForward},
		{name: "forward legacy", scope: "timeline", mode: "normal", key: "f", command: CommandMailForward},
		{name: "forward legacy", scope: "timeline", mode: "normal", key: "F", command: CommandMailForward},
		{name: "archive primary", scope: "timeline", mode: "normal", key: "a", command: CommandMailArchiveCurrent},
		{name: "archive secondary", scope: "timeline", mode: "normal", key: "e", command: CommandMailArchiveCurrent},
		{name: "delete confirm primary", scope: "timeline", mode: "normal", key: "delete", command: CommandMailDeleteConfirm},
		{name: "delete confirm legacy", scope: "timeline", mode: "normal", key: "d", command: CommandMailDeleteConfirm},
		{name: "delete confirm backspace", scope: "timeline", mode: "normal", key: "backspace", command: CommandMailDeleteConfirm},
		{name: "delete immediate primary", scope: "timeline", mode: "normal", key: "shift+delete", command: CommandMailDeleteImmediate},
		{name: "delete immediate legacy", scope: "timeline", mode: "normal", key: "D", command: CommandMailDeleteImmediate},
		{name: "delete immediate shift backspace", scope: "timeline", mode: "normal", key: "shift+backspace", command: CommandMailDeleteImmediate},
		{name: "classify relocated", scope: "timeline", mode: "normal", key: "T", command: CommandMailReclassify},
		{name: "unsubscribe confirm", scope: "timeline", mode: "normal", key: "u", command: CommandMailUnsubscribeConfirm},
		{name: "unsubscribe immediate", scope: "timeline", mode: "normal", key: "U", command: CommandMailUnsubscribeImmediate},
		{name: "hide future immediate", scope: "timeline", mode: "normal", key: "H", command: CommandMailHideFuture},
		{name: "search ctrl+k alias", scope: "timeline", mode: "normal", key: "ctrl+k", command: CommandHelpSearch},
		{name: "pane next primary", scope: "global", mode: "normal", key: "f6", command: CommandPaneNext},
		{name: "pane prev primary", scope: "global", mode: "normal", key: "shift+f6", command: CommandPanePrev},
		{name: "pane next legacy", scope: "global", mode: "normal", key: "tab", command: CommandPaneNext},
		{name: "account switcher primary", scope: "global", mode: "normal", key: "alt+A", command: CommandAccountSwitcher},
		{name: "alt timeline tab alias", scope: "global", mode: "normal", key: "alt+1", command: CommandTabTimeline},
		{name: "compose send primary", scope: "compose", mode: "normal", key: "ctrl+enter", command: CommandComposeSend},
		{name: "sort cycle primary", scope: "timeline", mode: "normal", key: "O", command: CommandTimelineSortCycle},
		{name: "sidebar primary", scope: "global", mode: "normal", key: "B", command: CommandSidebarToggle},
		{name: "logs primary", scope: "global", mode: "normal", key: "L", command: CommandLogsToggle},
		{name: "refresh primary", scope: "global", mode: "normal", key: "alt+r", command: CommandAppRefresh},
		{name: "refresh legacy ctrl fallback", scope: "global", mode: "normal", key: "ctrl+r", command: CommandAppRefresh},
		{name: "contacts function alias", scope: "global", mode: "normal", key: "f2", command: CommandTabContacts},
		{name: "contacts primary", scope: "global", mode: "normal", key: "2", command: CommandTabContacts},
		{name: "contacts legacy f3", scope: "global", mode: "normal", key: "f3", command: CommandTabContacts},
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

func TestRefreshPrimaryKeyDoesNotConflictWithTimelineReply(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.showSidebar = true
	m.setFocusedPanel(panelSidebar)

	if key := m.commandKey(keyboardScopeGlobal, CommandAppRefresh); key != "alt+r" {
		t.Fatalf("refresh primary key = %q, want alt+r so Timeline Ctrl+R can stay reply", key)
	}
	if got, ok := m.keyboard.Resolve("timeline", keyboardModeNormal, "ctrl+r"); !ok || got != CommandMailReplySender {
		t.Fatalf("Timeline Ctrl+R resolves to %q ok=%v, want reply sender", got, ok)
	}

	hints := stripANSI(m.renderKeyHints())
	if strings.Contains(hints, "ctrl+r: refresh") || strings.Contains(hints, "Ctrl+R: refresh") {
		t.Fatalf("Timeline hints must not advertise Ctrl+R as refresh while Ctrl+R replies, got:\n%s", hints)
	}

	m.timeline.emails = []*models.EmailData{}
	m.updateTimelineTable()
	rendered := stripANSI(m.renderTimelineView())
	if !strings.Contains(rendered, "press Alt+R to refresh") {
		t.Fatalf("empty Timeline refresh copy should point at Alt+R, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "press r to refresh") {
		t.Fatalf("empty Timeline refresh copy must not point at reply key r, got:\n%s", rendered)
	}
}

func TestVimKeyboardProfilePreservesTerminalPrimaries(t *testing.T) {
	cfg := &config.Config{}
	cfg.Keyboard.Profile = keyboardProfileVim
	resolver := NewKeyboardResolver(cfg)

	for _, tc := range []struct {
		key     string
		command string
	}{
		{"h", CommandPaneLeft},
		{"j", CommandPaneDown},
		{"k", CommandPaneUp},
		{"l", CommandPaneRight},
		{"c", CommandComposeNew},
		{"r", CommandMailReplySender},
		{"R", CommandMailReplyAll},
		{"f", CommandMailForward},
		{"a", CommandMailArchiveCurrent},
		{"d", CommandMailDeleteConfirm},
		{"D", CommandMailDeleteImmediate},
		{"A", CommandMailReclassify},
	} {
		got, ok := resolver.Resolve("timeline", keyboardModeNormal, tc.key)
		if !ok || got != tc.command {
			t.Fatalf("vim Resolve(%q) = %q, %v; want %q, true", tc.key, got, ok, tc.command)
		}
		if primary := resolver.PrimaryKey("timeline", keyboardModeNormal, tc.command); primary != tc.key {
			t.Fatalf("vim PrimaryKey(%s) = %q, want %q", tc.command, primary, tc.key)
		}
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

func TestKeyboardResolverCustomKeymapRejectsDeprecatedCleanupTabCommand(t *testing.T) {
	err := ValidateCustomKeymap([]byte(`
extends: default
bindings:
  global:
    normal:
      3: tab.cleanup
`))
	if err == nil {
		t.Fatal("expected validation error for deprecated cleanup tab command")
	}
	if !strings.Contains(err.Error(), "tab.cleanup") {
		t.Fatalf("validation error = %v, want deprecated cleanup command id", err)
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

func TestCustomKeymapRoutesTimelineMailActionsByCommand(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.deletionRequestCh = make(chan models.DeletionRequest, 4)
	m.deletionResultCh = make(chan models.DeletionResult, 4)
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  timeline:
    normal:
      x: mail.archive_current
      z: mail.delete_confirm
      G: mail.reclassify
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}
	m.keyboard = resolver

	model, _ := m.handleKeyMsg(keyRunes("x"))
	updated := model.(*Model)
	if updated.pendingDeleteConfirm || updated.pendingArchive {
		t.Fatalf("custom archive command opened confirmation: confirm=%v archive=%v desc=%q", updated.pendingDeleteConfirm, updated.pendingArchive, updated.pendingDeleteDesc)
	}
	reqs := readDeletionRequests(t, updated.deletionRequestCh, 1)
	if !reqs[0].IsArchive {
		t.Fatalf("custom archive command queued IsArchive=%v, want true", reqs[0].IsArchive)
	}

	updated.deleting = false
	updated.pendingDeleteConfirm = false
	updated.pendingArchive = false
	updated.pendingDeleteDesc = ""
	model, _ = updated.handleKeyMsg(keyRunes("z"))
	updated = model.(*Model)
	if !updated.pendingDeleteConfirm || updated.pendingArchive {
		t.Fatalf("custom delete command did not open delete confirmation: confirm=%v archive=%v desc=%q", updated.pendingDeleteConfirm, updated.pendingArchive, updated.pendingDeleteDesc)
	}

	updated.pendingDeleteConfirm = false
	updated.pendingDeleteDesc = ""
	model, _ = updated.handleKeyMsg(keyRunes("G"))
	updated = model.(*Model)
	if updated.statusMessage != "No AI configured" {
		t.Fatalf("custom reclassify command status = %q, want no-AI status", updated.statusMessage)
	}
}

func TestCustomKeymapRoutesGlobalCommandsFromContacts(t *testing.T) {
	m := makeSizedModel(t, 140, 40)
	m.activeTab = tabContacts
	m.contactsList = []models.ContactData{{Email: "mara@forgepoint.example", DisplayName: "Mara Vale"}}
	m.contactsFiltered = m.contactsList
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  global:
    normal:
      7: tab.timeline
      o: logs.toggle
      b: sidebar.toggle
      s: app.settings
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}
	m.keyboard = resolver

	model, _ := m.handleKeyMsg(keyRunes("7"))
	updated := model.(*Model)
	if updated.activeTab != tabTimeline {
		t.Fatalf("custom global tab command from Contacts activeTab=%d, want Timeline", updated.activeTab)
	}

	updated.activeTab = tabContacts
	model, _ = updated.handleKeyMsg(keyRunes("o"))
	updated = model.(*Model)
	if !updated.showLogs {
		t.Fatal("custom global logs command from Contacts did not open logs")
	}

	model, _ = updated.handleKeyMsg(keyRunes("o"))
	updated = model.(*Model)
	if updated.showLogs {
		t.Fatal("custom global logs command from Contacts did not close logs")
	}

	shown := updated.showSidebar
	model, _ = updated.handleKeyMsg(keyRunes("b"))
	updated = model.(*Model)
	if updated.showSidebar == shown {
		t.Fatal("custom global sidebar command from Contacts did not toggle sidebar")
	}

	model, _ = updated.handleKeyMsg(keyRunes("s"))
	updated = model.(*Model)
	if !updated.showSettings || updated.settingsPanel == nil {
		t.Fatal("custom global settings command from Contacts did not open Settings")
	}
}

func TestCustomKeymapRoutesContactsAndCalendarCommands(t *testing.T) {
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  contacts:
    normal:
      n: pane.down
      p: pane.up
      ;: help.search
  calendar:
    normal:
      n: pane.down
      p: pane.up
      ';': help.search
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}

	t.Run("contacts", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.keyboard = resolver
		m.activeTab = tabContacts
		m.contactsList = []models.ContactData{
			{Email: "mara@forgepoint.example", DisplayName: "Mara Vale"},
			{Email: "niko@forgepoint.example", DisplayName: "Niko Park"},
		}
		m.contactsFiltered = m.contactsList

		model, _ := m.handleKeyMsg(keyRunes("n"))
		updated := model.(*Model)
		if updated.contactsIdx != 1 {
			t.Fatalf("custom Contacts down contactsIdx=%d, want 1", updated.contactsIdx)
		}
		model, _ = updated.handleKeyMsg(keyRunes("p"))
		updated = model.(*Model)
		if updated.contactsIdx != 0 {
			t.Fatalf("custom Contacts up contactsIdx=%d, want 0", updated.contactsIdx)
		}
		model, _ = updated.handleKeyMsg(keyRunes(";"))
		updated = model.(*Model)
		if updated.contactSearchMode != "keyword" {
			t.Fatalf("custom Contacts search mode=%q, want keyword", updated.contactSearchMode)
		}
	})

	t.Run("calendar", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.keyboard = resolver
		m.activeTab = tabCalendar
		m.calendarAvailable = true
		m.calendarEvents = testCalendarEvents()
		m.calendarView = calendarViewAgenda
		m.calendarDetail = m.selectedCalendarEvent()

		model, _ := m.handleKeyMsg(keyRunes("n"))
		updated := model.(*Model)
		if updated.calendarCursor != 1 {
			t.Fatalf("custom Calendar down cursor=%d, want 1", updated.calendarCursor)
		}
		model, _ = updated.handleKeyMsg(keyRunes("p"))
		updated = model.(*Model)
		if updated.calendarCursor != 0 {
			t.Fatalf("custom Calendar up cursor=%d, want 0", updated.calendarCursor)
		}
		model, _ = updated.handleKeyMsg(keyRunes(";"))
		updated = model.(*Model)
		if updated.calendarView != calendarViewSearch {
			t.Fatalf("custom Calendar search view=%q, want search", updated.calendarView)
		}
	})
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
      8: tab.contacts
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
		"7-8: tabs",
		"n: compose",
		"p: all",
		"s: sender",
		"w: forward",
		"z: delete",
		"y: delete now",
		"x: archive",
		"O: sort",
		"G: re-classify",
		"b: sidebar",
	)
	for _, stale := range []string{"1-2: tabs", "1-3: tabs", "7-9: tabs", "c: compose", "r: sender", "R: all", "f: forward", "a: archive", "T: re-classify", "B: sidebar"} {
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
	requireHintSegments(t, hints, "Enter: open", "Ctrl+N: new", "Ctrl+R: reply", "Del: delete", "/: search")
	for _, legacy := range []string{"c: compose", "r: sender", "R: all", "f: forward", "d: delete", "D: delete now"} {
		if strings.Contains(hints, legacy) {
			t.Fatalf("Default bottom hints should omit legacy alias %q, got:\n%s", legacy, hints)
		}
	}
	for _, tc := range []struct {
		scope   string
		command string
		help    string
	}{
		{scope: "timeline", command: CommandComposeNew, help: "open a blank Compose"},
		{scope: "timeline", command: CommandMailReplyAll, help: "reply all"},
		{scope: "timeline", command: CommandMailReplySender, help: "reply sender"},
		{scope: "timeline", command: CommandMailForward, help: "forward highlighted"},
		{scope: "timeline", command: CommandMailDeleteConfirm, help: "after confirmation"},
		{scope: "timeline", command: CommandMailDeleteImmediate, help: "immediately"},
		{scope: "timeline", command: CommandMailArchiveCurrent, help: "immediately"},
		{scope: "timeline", command: CommandTimelineSortCycle, help: "cycle Timeline sorting"},
		{scope: keyboardScopeGlobal, command: CommandSidebarToggle, help: "toggle sidebar"},
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
	if hintContainsExactSegment(hints, "r: sender") {
		t.Fatalf("expected overwritten default key not to be advertised for reply-sender, got:\n%s", hints)
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

func hintContainsExactSegment(hints, segment string) bool {
	for _, part := range strings.Split(hints, "│") {
		if strings.TrimSpace(part) == segment {
			return true
		}
	}
	return false
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
      8: tab.contacts
  timeline:
    normal:
      n: compose.new
      p: mail.reply_all
`)); err != nil {
		t.Fatalf("ApplyCustomKeymap failed: %v", err)
	}
	m.keyboard = resolver

	tabBar := stripANSI(m.renderTabBar())
	for _, want := range []string{"7  Timeline", "8  Contacts"} {
		if !strings.Contains(tabBar, want) {
			t.Fatalf("expected custom tab label %q, got %q", want, tabBar)
		}
	}
	for _, stale := range []string{"1  Timeline", "2  Contacts", "Cleanup", "9  Contacts"} {
		if strings.Contains(tabBar, stale) {
			t.Fatalf("expected tab bar to omit stale %q, got %q", stale, tabBar)
		}
	}

	updated := pressQuestion(m)
	help := stripANSI(updated.View().Content)
	for _, want := range []string{"7-8", "switch tabs", "n", "open a blank Compose", "p", "reply all"} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected shortcut help to include %q, got:\n%s", want, help)
		}
	}
	for _, stale := range []string{"1-2", "1-3", "7-9", "Cleanup", "c              open a blank Compose", "r / R / f"} {
		if strings.Contains(help, stale) {
			t.Fatalf("expected shortcut help to omit stale %q, got:\n%s", stale, help)
		}
	}
}

func TestCustomKeymapBottomHintsUseResolvedComposeTimelineAndContactsKeys(t *testing.T) {
	resolver := NewKeyboardResolver(&config.Config{})
	if err := resolver.ApplyCustomKeymap([]byte(`
extends: default
bindings:
  global:
    normal:
      7: tab.timeline
      8: tab.contacts
      b: sidebar.toggle
      o: logs.toggle
      y: chat.toggle
  timeline:
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
		requireHintSegments(t, hints, "7-8: tabs", "ctrl+s: send", "ctrl+p: preview")
		if strings.Contains(hints, "1-2: tabs") || strings.Contains(hints, "1-3: tabs") || strings.Contains(hints, "?: help") {
			t.Fatalf("expected Compose hints to use custom tab keys and avoid browse help, got:\n%s", hints)
		}
	})

	t.Run("timeline", func(t *testing.T) {
		m := makeSizedModel(t, 260, 40)
		m.keyboard = resolver
		m.activeTab = tabTimeline
		m.timeline.emails = mockEmails()
		m.updateTimelineTable()

		hints := stripANSI(m.renderKeyHints())
		requireHintSegments(t, hints, "7-8: tabs", "z: delete", "v: delete now", "x: archive", "b: sidebar")
		for _, stale := range []string{"1-2: tabs", "1-3: tabs", "a: archive", "B: sidebar", "g: chat"} {
			if strings.Contains(hints, stale) {
				t.Fatalf("expected Timeline custom hints to omit stale %q, got:\n%s", stale, hints)
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
		requireHintSegments(t, hints, "7-8: tabs", "↑/p ↓/n: nav", ";: search")
		for _, stale := range []string{"1-2: tabs", "1-3: tabs", "↑/k ↓/j: nav", "/: search"} {
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

func TestMailActionShortcutAliasesDoNotStealTextEntry(t *testing.T) {
	keys := []string{"r", "R", "a", "e", "u", "U", "H"}
	want := "rRaeuUH"

	t.Run("compose", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.activeTab = tabCompose
		m.composeField = composeFieldBody
		m.composeBody.Focus()

		for _, key := range keys {
			model, _ := m.handleKeyMsg(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.composeBody.Value(); got != want {
			t.Fatalf("compose body = %q, want literal mail-action text %q", got, want)
		}
		if m.pendingDeleteConfirm || m.pendingArchive || m.activeTab != tabCompose {
			t.Fatalf("compose text entry fired mail action: confirm=%v archive=%v activeTab=%d", m.pendingDeleteConfirm, m.pendingArchive, m.activeTab)
		}
	})

	t.Run("search prompt", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.activeTab = tabTimeline
		m.timeline.emails = mockEmails()
		m.updateTimelineTable()
		m.openTimelineSearch()

		for _, key := range keys {
			model, _ := m.handleKeyMsg(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.timeline.searchInput.Value(); got != want {
			t.Fatalf("search prompt = %q, want literal mail-action text %q", got, want)
		}
		if m.pendingDeleteConfirm || m.pendingArchive || m.activeTab != tabTimeline || !m.timeline.searchMode {
			t.Fatalf("search prompt text entry fired mail action: confirm=%v archive=%v activeTab=%d search=%v", m.pendingDeleteConfirm, m.pendingArchive, m.activeTab, m.timeline.searchMode)
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

		for _, key := range keys {
			model, _ := m.Update(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.promptEditor.name; got != want {
			t.Fatalf("prompt editor name = %q, want literal mail-action text %q", got, want)
		}
		if m.pendingDeleteConfirm || m.pendingArchive {
			t.Fatalf("prompt editor text entry fired mail action: confirm=%v archive=%v", m.pendingDeleteConfirm, m.pendingArchive)
		}
	})

	t.Run("rule editor", func(t *testing.T) {
		m := makeSizedModel(t, 140, 40)
		m.showRuleEditor = true
		m.ruleEditor = NewRuleEditor("", "", m.windowWidth, m.windowHeight)
		_ = m.ruleEditor.Init()
		_ = m.ruleEditor.form.NextField()

		for _, key := range keys {
			model, _ := m.Update(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.ruleEditor.triggerValue; got != want {
			t.Fatalf("rule editor trigger value = %q, want literal mail-action text %q", got, want)
		}
		if m.pendingDeleteConfirm || m.pendingArchive {
			t.Fatalf("rule editor text entry fired mail action: confirm=%v archive=%v", m.pendingDeleteConfirm, m.pendingArchive)
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
