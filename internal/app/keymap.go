package app

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	keyboardProfileDefault = "default"
	keyboardProfileVim     = "vim"
	keyboardProfileEmacs   = "emacs"
	keyboardProfileCustom  = "custom"

	keyboardModeNormal   = "normal"
	keyboardModeInsert   = "insert"
	keyboardModeVisual   = "visual"
	keyboardScopeGlobal  = "global"
	keyboardFieldCompose = "compose"
)

const (
	CommandAppQuit     = "app.quit"
	CommandAppRefresh  = "app.refresh"
	CommandAppSettings = "app.settings"

	CommandTabTimeline = "tab.timeline"
	CommandTabContacts = "tab.contacts"

	CommandPaneLeft  = "pane.left"
	CommandPaneRight = "pane.right"
	CommandPaneUp    = "pane.up"
	CommandPaneDown  = "pane.down"

	CommandComposeNew = "compose.new"

	CommandMailReplyAll        = "mail.reply_all"
	CommandMailReplySender     = "mail.reply_sender"
	CommandMailForward         = "mail.forward"
	CommandMailArchiveCurrent  = "mail.archive_current"
	CommandMailDeleteConfirm   = "mail.delete_confirm"
	CommandMailDeleteImmediate = "mail.delete_immediate"
	CommandMailReclassify      = "mail.reclassify"
	CommandMailHideFuture      = "mail.hide_future"

	CommandHelpOpen   = "help.open"
	CommandHelpSearch = "help.search"
	CommandLogsToggle = "logs.toggle"
	CommandChatToggle = "chat.toggle"

	CommandSidebarToggle = "sidebar.toggle"

	CommandTimelineGroupCycle = "timeline.group_cycle"

	CommandFieldInsert        = "field.insert"
	CommandFieldAppend        = "field.append"
	CommandFieldAppendLineEnd = "field.append_line_end"
	CommandFieldVisual        = "field.visual"
)

type keyboardBindingMap map[string]map[string]map[string]string
type keyboardCommandKeyMap map[string]map[string]map[string]string

// KeyboardResolver owns the configured key profile and custom overrides.
// Handlers can resolve by scope/mode without knowing whether a binding came
// from the built-in profiles or from a user keymap file.
type KeyboardResolver struct {
	profile       string
	bindings      keyboardBindingMap
	primaryKeys   keyboardCommandKeyMap
	fieldDefaults map[string]string
}

type customKeymapFile struct {
	Extends  string                                  `yaml:"extends"`
	Bindings map[string]map[string]map[string]string `yaml:"bindings"`
	Fields   map[string]struct {
		DefaultMode string `yaml:"default_mode"`
	} `yaml:"fields"`
}

var commandCatalog = map[string]struct{}{
	CommandAppQuit:     {},
	CommandAppRefresh:  {},
	CommandAppSettings: {},
	CommandTabTimeline: {},
	CommandTabContacts: {},

	CommandPaneLeft:  {},
	CommandPaneRight: {},
	CommandPaneUp:    {},
	CommandPaneDown:  {},

	CommandComposeNew: {},

	CommandMailReplyAll:        {},
	CommandMailReplySender:     {},
	CommandMailForward:         {},
	CommandMailArchiveCurrent:  {},
	CommandMailDeleteConfirm:   {},
	CommandMailDeleteImmediate: {},
	CommandMailReclassify:      {},
	CommandMailHideFuture:      {},

	CommandHelpOpen:   {},
	CommandHelpSearch: {},
	CommandLogsToggle: {},
	CommandChatToggle: {},

	CommandSidebarToggle: {},

	CommandTimelineGroupCycle: {},

	CommandFieldInsert:        {},
	CommandFieldAppend:        {},
	CommandFieldAppendLineEnd: {},
	CommandFieldVisual:        {},
}

func NewKeyboardResolver(cfg *config.Config) *KeyboardResolver {
	profile := keyboardProfileDefault
	if cfg != nil && strings.TrimSpace(cfg.Keyboard.Profile) != "" {
		profile = strings.ToLower(strings.TrimSpace(cfg.Keyboard.Profile))
	}
	switch profile {
	case keyboardProfileDefault, keyboardProfileVim, keyboardProfileEmacs, keyboardProfileCustom:
	default:
		profile = keyboardProfileDefault
	}
	bindings, primaryKeys := builtInKeyboardProfile(profile)
	resolver := &KeyboardResolver{
		profile:       profile,
		bindings:      bindings,
		primaryKeys:   primaryKeys,
		fieldDefaults: builtInKeyboardFieldDefaults(profile),
	}
	return resolver
}

func (r *KeyboardResolver) Profile() string {
	if r == nil || r.profile == "" {
		return keyboardProfileDefault
	}
	return r.profile
}

func (r *KeyboardResolver) Resolve(scope, mode, key string) (string, bool) {
	if r == nil {
		r = NewKeyboardResolver(nil)
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	mode = strings.ToLower(strings.TrimSpace(mode))
	key = strings.TrimSpace(key)
	if scope == "" || key == "" {
		return "", false
	}
	if mode == "" {
		mode = keyboardModeNormal
	}
	if cmd, ok := r.lookup(scope, mode, key); ok {
		return cmd, true
	}
	if scope != keyboardScopeGlobal {
		return r.lookup(keyboardScopeGlobal, mode, key)
	}
	return "", false
}

func (r *KeyboardResolver) lookup(scope, mode, key string) (string, bool) {
	if r == nil || r.bindings == nil {
		return "", false
	}
	byMode, ok := r.bindings[scope]
	if !ok {
		return "", false
	}
	byKey, ok := byMode[mode]
	if !ok {
		return "", false
	}
	cmd, ok := byKey[key]
	return cmd, ok
}

func (r *KeyboardResolver) ApplyCustomKeymap(data []byte) error {
	custom, err := parseCustomKeymap(data)
	if err != nil {
		return err
	}
	base := strings.ToLower(strings.TrimSpace(custom.Extends))
	if base == "" || base == keyboardProfileCustom {
		base = keyboardProfileVim
	}
	bindings, primaryKeys := builtInKeyboardProfile(base)
	r.profile = keyboardProfileCustom
	r.bindings = bindings
	r.primaryKeys = primaryKeys
	r.fieldDefaults = builtInKeyboardFieldDefaults(base)
	mergeKeyboardBindings(r.bindings, r.primaryKeys, custom.Bindings)
	for field, cfg := range custom.Fields {
		field = strings.ToLower(strings.TrimSpace(field))
		mode := strings.ToLower(strings.TrimSpace(cfg.DefaultMode))
		if field != "" && mode != "" {
			r.fieldDefaults[field] = mode
		}
	}
	return nil
}

func ValidateCustomKeymap(data []byte) error {
	_, err := parseCustomKeymap(data)
	return err
}

func parseCustomKeymap(data []byte) (*customKeymapFile, error) {
	var custom customKeymapFile
	if err := yaml.Unmarshal(data, &custom); err != nil {
		return nil, fmt.Errorf("parse custom keymap: %w", err)
	}
	base := strings.ToLower(strings.TrimSpace(custom.Extends))
	if base != "" && base != keyboardProfileDefault && base != keyboardProfileVim && base != keyboardProfileEmacs {
		return nil, fmt.Errorf("unknown custom keymap base profile %q", custom.Extends)
	}
	var unknown []string
	for scope, byMode := range custom.Bindings {
		for mode, byKey := range byMode {
			for key, command := range byKey {
				command = strings.TrimSpace(command)
				if _, ok := commandCatalog[command]; !ok {
					unknown = append(unknown, fmt.Sprintf("%s.%s[%s]=%s", scope, mode, key, command))
				}
			}
		}
	}
	for field, cfg := range custom.Fields {
		mode := strings.ToLower(strings.TrimSpace(cfg.DefaultMode))
		if mode == "" {
			continue
		}
		switch mode {
		case keyboardModeNormal, keyboardModeInsert, keyboardModeVisual:
		default:
			unknown = append(unknown, fmt.Sprintf("%s.default_mode=%s", field, cfg.DefaultMode))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("invalid custom keymap entries: %s", strings.Join(unknown, ", "))
	}
	return &custom, nil
}

func builtInKeyboardBindings(profile string) keyboardBindingMap {
	bindings, _ := builtInKeyboardProfile(profile)
	return bindings
}

func builtInKeyboardProfile(profile string) (keyboardBindingMap, keyboardCommandKeyMap) {
	bindings := keyboardBindingMap{}
	primaryKeys := keyboardCommandKeyMap{}
	add := func(scope, mode, keyName, command string) {
		if bindings[scope] == nil {
			bindings[scope] = map[string]map[string]string{}
		}
		if bindings[scope][mode] == nil {
			bindings[scope][mode] = map[string]string{}
		}
		bindings[scope][mode][keyName] = command
		if primaryKeys[scope] == nil {
			primaryKeys[scope] = map[string]map[string]string{}
		}
		if primaryKeys[scope][mode] == nil {
			primaryKeys[scope][mode] = map[string]string{}
		}
		if primaryKeys[scope][mode][command] == "" {
			primaryKeys[scope][mode][command] = keyName
		}
	}
	prefer := func(scope, mode, keyName, command string) {
		add(scope, mode, keyName, command)
		primaryKeys[scope][mode][command] = keyName
	}

	add(keyboardScopeGlobal, keyboardModeNormal, "ctrl+c", CommandAppQuit)
	prefer(keyboardScopeGlobal, keyboardModeNormal, "q", CommandAppQuit)
	add(keyboardScopeGlobal, keyboardModeNormal, "?", CommandHelpOpen)
	add(keyboardScopeGlobal, keyboardModeNormal, "S", CommandAppSettings)
	add(keyboardScopeGlobal, keyboardModeNormal, "ctrl+r", CommandAppRefresh)
	add(keyboardScopeGlobal, keyboardModeNormal, "B", CommandSidebarToggle)
	add(keyboardScopeGlobal, keyboardModeNormal, "L", CommandLogsToggle)
	add(keyboardScopeGlobal, keyboardModeNormal, "g", CommandChatToggle)
	add(keyboardScopeGlobal, keyboardModeNormal, "1", CommandTabTimeline)
	add(keyboardScopeGlobal, keyboardModeNormal, "2", CommandTabContacts)
	add(keyboardScopeGlobal, keyboardModeNormal, "f1", CommandTabTimeline)
	add(keyboardScopeGlobal, keyboardModeNormal, "f2", CommandTabContacts)
	add(keyboardScopeGlobal, keyboardModeNormal, "f3", CommandTabContacts)

	add("timeline", keyboardModeNormal, "h", CommandPaneLeft)
	add("timeline", keyboardModeNormal, "j", CommandPaneDown)
	add("timeline", keyboardModeNormal, "k", CommandPaneUp)
	add("timeline", keyboardModeNormal, "l", CommandPaneRight)
	add("timeline", keyboardModeNormal, "left", CommandPaneLeft)
	add("timeline", keyboardModeNormal, "down", CommandPaneDown)
	add("timeline", keyboardModeNormal, "up", CommandPaneUp)
	add("timeline", keyboardModeNormal, "right", CommandPaneRight)
	add("timeline", keyboardModeNormal, "c", CommandComposeNew)
	add("timeline", keyboardModeNormal, "r", CommandMailReplyAll)
	add("timeline", keyboardModeNormal, "R", CommandMailReplySender)
	add("timeline", keyboardModeNormal, "f", CommandMailForward)
	add("timeline", keyboardModeNormal, "F", CommandMailForward)
	add("timeline", keyboardModeNormal, "a", CommandMailArchiveCurrent)
	add("timeline", keyboardModeNormal, "e", CommandMailArchiveCurrent)
	add("timeline", keyboardModeNormal, "d", CommandMailDeleteConfirm)
	add("timeline", keyboardModeNormal, "backspace", CommandMailDeleteConfirm)
	add("timeline", keyboardModeNormal, "D", CommandMailDeleteImmediate)
	add("timeline", keyboardModeNormal, "shift+backspace", CommandMailDeleteImmediate)
	add("timeline", keyboardModeNormal, "T", CommandMailReclassify)
	add("timeline", keyboardModeNormal, "A", CommandMailReclassify)
	add("timeline", keyboardModeNormal, "H", CommandMailHideFuture)
	add("timeline", keyboardModeNormal, "G", CommandTimelineGroupCycle)
	add("timeline", keyboardModeNormal, "/", CommandHelpSearch)

	for _, scope := range []string{"cleanup", "contacts"} {
		add(scope, keyboardModeNormal, "h", CommandPaneLeft)
		add(scope, keyboardModeNormal, "j", CommandPaneDown)
		add(scope, keyboardModeNormal, "k", CommandPaneUp)
		add(scope, keyboardModeNormal, "l", CommandPaneRight)
		add(scope, keyboardModeNormal, "left", CommandPaneLeft)
		add(scope, keyboardModeNormal, "down", CommandPaneDown)
		add(scope, keyboardModeNormal, "up", CommandPaneUp)
		add(scope, keyboardModeNormal, "right", CommandPaneRight)
		add(scope, keyboardModeNormal, "/", CommandHelpSearch)
	}
	add("cleanup", keyboardModeNormal, "a", CommandMailArchiveCurrent)
	add("cleanup", keyboardModeNormal, "e", CommandMailArchiveCurrent)
	add("cleanup", keyboardModeNormal, "d", CommandMailDeleteConfirm)
	add("cleanup", keyboardModeNormal, "backspace", CommandMailDeleteConfirm)
	add("cleanup", keyboardModeNormal, "D", CommandMailDeleteImmediate)
	add("cleanup", keyboardModeNormal, "shift+backspace", CommandMailDeleteImmediate)
	add("cleanup", keyboardModeNormal, "T", CommandMailReclassify)
	add("cleanup", keyboardModeNormal, "A", CommandMailReclassify)
	add("cleanup", keyboardModeNormal, "H", CommandMailHideFuture)

	add("field", keyboardModeNormal, "i", CommandFieldInsert)
	add("field", keyboardModeNormal, "a", CommandFieldAppend)
	add("field", keyboardModeNormal, "A", CommandFieldAppendLineEnd)
	add("field", keyboardModeNormal, "v", CommandFieldVisual)

	switch profile {
	case keyboardProfileEmacs:
		for _, scope := range []string{"timeline", "cleanup", "contacts"} {
			prefer(scope, keyboardModeNormal, "ctrl+f", CommandPaneRight)
			prefer(scope, keyboardModeNormal, "ctrl+b", CommandPaneLeft)
			prefer(scope, keyboardModeNormal, "ctrl+n", CommandPaneDown)
			prefer(scope, keyboardModeNormal, "ctrl+p", CommandPaneUp)
		}
	case keyboardProfileVim, keyboardProfileCustom, keyboardProfileDefault:
		// The default profile intentionally uses Vim-like movement because it
		// is the coherent remap requested for browsing mail.
	}

	return bindings, primaryKeys
}

func builtInKeyboardFieldDefaults(profile string) map[string]string {
	mode := keyboardModeInsert
	switch profile {
	case keyboardProfileVim, keyboardProfileCustom:
		mode = keyboardModeNormal
	}
	return map[string]string{
		keyboardFieldCompose: mode,
	}
}

func (r *KeyboardResolver) FieldDefaultMode(field string) string {
	if r == nil {
		return keyboardModeInsert
	}
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		return keyboardModeInsert
	}
	if mode := strings.ToLower(strings.TrimSpace(r.fieldDefaults[field])); mode != "" {
		return mode
	}
	return keyboardModeInsert
}

func (r *KeyboardResolver) PrimaryKey(scope, mode, command string) string {
	if r == nil {
		r = NewKeyboardResolver(nil)
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	mode = strings.ToLower(strings.TrimSpace(mode))
	command = strings.TrimSpace(command)
	if scope == "" || command == "" {
		return ""
	}
	if mode == "" {
		mode = keyboardModeNormal
	}
	if key := r.lookupPrimary(scope, mode, command); key != "" {
		return key
	}
	if scope != keyboardScopeGlobal {
		return r.lookupPrimary(keyboardScopeGlobal, mode, command)
	}
	return ""
}

func (r *KeyboardResolver) lookupPrimary(scope, mode, command string) string {
	if r == nil || r.primaryKeys == nil {
		return ""
	}
	byMode, ok := r.primaryKeys[scope]
	if !ok {
		return ""
	}
	byCommand, ok := byMode[mode]
	if !ok {
		return ""
	}
	return byCommand[command]
}

func canonicalKeyForCommand(scope, command string) string {
	switch command {
	case CommandAppQuit:
		return "q"
	case CommandAppRefresh:
		return "ctrl+r"
	case CommandAppSettings:
		return "S"
	case CommandTabTimeline:
		return "1"
	case CommandTabContacts:
		return "2"
	case CommandSidebarToggle:
		return "B"
	case CommandTimelineGroupCycle:
		return "G"
	case CommandLogsToggle:
		return "L"
	case CommandChatToggle:
		return "g"
	case CommandPaneLeft:
		return "h"
	case CommandPaneRight:
		return "l"
	case CommandPaneUp:
		return "k"
	case CommandPaneDown:
		return "j"
	case CommandComposeNew:
		return "c"
	case CommandMailReplyAll:
		return "r"
	case CommandMailReplySender:
		return "R"
	case CommandMailForward:
		return "f"
	case CommandMailArchiveCurrent:
		return "a"
	case CommandMailDeleteConfirm:
		return "d"
	case CommandMailDeleteImmediate:
		return "D"
	case CommandMailReclassify:
		return "T"
	case CommandMailHideFuture:
		return "H"
	case CommandHelpSearch:
		return "/"
	case CommandHelpOpen:
		return "?"
	}
	return ""
}

func shortcutKeyPressMsg(key string) tea.KeyPressMsg {
	switch key {
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "ctrl+d":
		return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	case "f1":
		return tea.KeyPressMsg{Code: tea.KeyF1}
	case "f2":
		return tea.KeyPressMsg{Code: tea.KeyF2}
	case "f3":
		return tea.KeyPressMsg{Code: tea.KeyF3}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "shift+backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModShift}
	}
	runes := []rune(key)
	msg := tea.KeyPressMsg{Text: key}
	if len(runes) == 1 {
		msg.Code = runes[0]
	}
	return msg
}

func mergeKeyboardBindings(dst keyboardBindingMap, primary keyboardCommandKeyMap, src map[string]map[string]map[string]string) {
	scopes := make([]string, 0, len(src))
	for scope := range src {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		byMode := src[scope]
		scope = strings.ToLower(strings.TrimSpace(scope))
		if dst[scope] == nil {
			dst[scope] = map[string]map[string]string{}
		}
		if primary[scope] == nil {
			primary[scope] = map[string]map[string]string{}
		}
		modes := make([]string, 0, len(byMode))
		for mode := range byMode {
			modes = append(modes, mode)
		}
		sort.Strings(modes)
		for _, mode := range modes {
			byKey := byMode[mode]
			mode = strings.ToLower(strings.TrimSpace(mode))
			if mode == "" {
				mode = keyboardModeNormal
			}
			if dst[scope][mode] == nil {
				dst[scope][mode] = map[string]string{}
			}
			if primary[scope][mode] == nil {
				primary[scope][mode] = map[string]string{}
			}
			keys := make([]string, 0, len(byKey))
			for keyName := range byKey {
				keys = append(keys, keyName)
			}
			sort.Strings(keys)
			for _, keyName := range keys {
				command := strings.TrimSpace(byKey[keyName])
				keyName = strings.TrimSpace(keyName)
				previous := dst[scope][mode][keyName]
				dst[scope][mode][keyName] = command
				if previous != "" && previous != command && primary[scope][mode][previous] == keyName {
					setPrimaryKeyForCommand(scope, primary[scope][mode], dst[scope][mode], previous)
				}
				if command != "" {
					primary[scope][mode][command] = keyName
				}
			}
		}
	}
}

func setPrimaryKeyForCommand(scope string, primary map[string]string, bindings map[string]string, command string) {
	delete(primary, command)
	if key := preferredBoundKey(scope, bindings, command); key != "" {
		primary[command] = key
	}
}

func preferredBoundKey(scope string, bindings map[string]string, command string) string {
	var keys []string
	for keyName, boundCommand := range bindings {
		if boundCommand == command {
			keys = append(keys, keyName)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	if canonical := canonicalKeyForCommand(scope, command); canonical != "" {
		for _, keyName := range keys {
			if keyName == canonical {
				return keyName
			}
		}
	}
	sort.Strings(keys)
	return keys[0]
}
