package app

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type keyDisplayStyle int

const (
	keyDisplayHint keyDisplayStyle = iota
	keyDisplayHelp
)

func (m *Model) commandKey(scope, command string) string {
	if m != nil && m.keyboard != nil {
		return m.keyboard.PrimaryKey(scope, keyboardModeNormal, command)
	}
	return canonicalKeyForCommand(scope, command)
}

func (m *Model) commandHint(scope, command, desc string) string {
	key := m.commandKey(scope, command)
	if key == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s", displayShortcutKey(key, keyDisplayHint), desc)
}

func (m *Model) commandHelpEntry(scope, command, desc string) shortcutHelpEntry {
	return shortcutHelpEntry{
		Key:  displayShortcutKey(m.commandKey(scope, command), keyDisplayHelp),
		Desc: desc,
	}
}

func (m *Model) commandHelpKey(scope, command string) string {
	return displayShortcutKey(m.commandKey(scope, command), keyDisplayHelp)
}

func (m *Model) movementHint(scope, desc string) string {
	down, up := m.verticalKeys(scope)
	return fmt.Sprintf("↑/%s ↓/%s: %s", up, down, desc)
}

func (m *Model) verticalPairHint(scope, upDesc, downDesc string) string {
	down, up := m.verticalKeys(scope)
	return fmt.Sprintf("%s: %s  %s: %s", up, upDesc, down, downDesc)
}

func (m *Model) rangeExtendHint(scope string) string {
	down, up := m.verticalKeys(scope)
	return fmt.Sprintf("%s/%s: extend range", down, up)
}

func (m *Model) verticalKeys(scope string) (down, up string) {
	up = displayShortcutKey(m.commandKey(scope, CommandPaneUp), keyDisplayHint)
	down = displayShortcutKey(m.commandKey(scope, CommandPaneDown), keyDisplayHint)
	if up == "" {
		up = "k"
	}
	if down == "" {
		down = "j"
	}
	return down, up
}

func (m *Model) leftFocusHint(scope, desc string) string {
	key := displayShortcutKey(m.commandKey(scope, CommandPaneLeft), keyDisplayHint)
	if key == "" {
		key = "h"
	}
	return fmt.Sprintf("%s/left: %s", key, desc)
}

func (m *Model) previewFocusHint(scope string) string {
	key := displayShortcutKey(m.commandKey(scope, CommandPaneRight), keyDisplayHint)
	if key == "" {
		key = "l"
	}
	return fmt.Sprintf("%s/right/]: focus preview", key)
}

func (m *Model) foldersFocusHint(scope string) string {
	key := displayShortcutKey(m.commandKey(scope, CommandPaneLeft), keyDisplayHint)
	if key == "" {
		key = "h"
	}
	return fmt.Sprintf("%s/left/[: folders", key)
}

func (m *Model) timelineOpenPreviewHint() string {
	key := displayShortcutKey(m.commandKey("timeline", CommandPaneRight), keyDisplayHint)
	if key == "" {
		key = "l"
	}
	return fmt.Sprintf("%s/right/]: preview", key)
}

func (m *Model) primaryTabShortcutHint() string {
	keys := m.primaryTabKeys(keyDisplayHint)
	tabs := m.visibleTopLevelTabNavigation()
	if len(keys) == len(tabs) {
		if compressed, ok := compressSequentialDigitKeys(keys); ok {
			return compressed + ": tabs"
		}
	}
	if len(keys) > 0 {
		return strings.Join(keys, "/") + ": tabs"
	}
	return primaryTabShortcutHint
}

func (m *Model) primaryTabHelpKey() string {
	keys := m.primaryTabKeys(keyDisplayHelp)
	tabs := m.visibleTopLevelTabNavigation()
	if len(keys) == len(tabs) {
		if compressed, ok := compressSequentialDigitKeys(keys); ok {
			return compressed
		}
	}
	if len(keys) > 0 {
		return strings.Join(keys, "/")
	}
	return "1-2"
}

func (m *Model) primaryTabHelpDescription() string {
	if m != nil && m.calendarAvailable {
		return "switch tabs; F2/F3 open Contacts; F4 opens Calendar"
	}
	return "switch tabs; F2 and F3 open Contacts"
}

func (m *Model) primaryTabKeys(style keyDisplayStyle) []string {
	tabs := m.visibleTopLevelTabNavigation()
	keys := make([]string, 0, len(tabs))
	for _, item := range tabs {
		key := item.key
		if item.command != "" {
			if resolved := m.commandKey(keyboardScopeGlobal, item.command); resolved != "" {
				key = resolved
			}
		}
		keys = append(keys, displayShortcutKey(key, style))
	}
	return keys
}

func compressSequentialDigitKeys(keys []string) (string, bool) {
	if len(keys) < 2 {
		return "", false
	}
	start, err := strconv.Atoi(keys[0])
	if err != nil {
		return "", false
	}
	for i := 1; i < len(keys); i++ {
		n, err := strconv.Atoi(keys[i])
		if err != nil || n != start+i {
			return "", false
		}
	}
	return fmt.Sprintf("%s-%s", keys[0], keys[len(keys)-1]), true
}

func displayShortcutKey(key string, style keyDisplayStyle) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if style == keyDisplayHint {
		return key
	}
	parts := strings.Split(key, "+")
	for i, part := range parts {
		parts[i] = displayHelpKeyPart(part)
		if i == len(parts)-1 && len(parts) > 1 && len(parts[i]) == 1 {
			parts[i] = strings.ToUpper(parts[i])
		}
	}
	return strings.Join(parts, "+")
}

func displayHelpKeyPart(part string) string {
	switch strings.ToLower(part) {
	case "ctrl":
		return "Ctrl"
	case "alt":
		return "Alt"
	case "meta":
		return "Meta"
	case "super":
		return "Super"
	case "hyper":
		return "Hyper"
	case "esc":
		return "Esc"
	case "enter":
		return "Enter"
	case "tab":
		return "Tab"
	case "backspace":
		return "Backspace"
	case "shift":
		return "Shift"
	case "pgup":
		return "PgUp"
	case "pgdown":
		return "PgDn"
	case "up":
		return "Up"
	case "down":
		return "Down"
	case "left":
		return "Left"
	case "right":
		return "Right"
	}
	if len(part) == 2 && (part[0] == 'f' || part[0] == 'F') && unicode.IsDigit(rune(part[1])) {
		return strings.ToUpper(part)
	}
	return part
}
