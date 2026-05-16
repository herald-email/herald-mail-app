package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const modifierHintFallbackDuration = 1200 * time.Millisecond

type modifierHintExpiredMsg struct {
	Token int
}

type modifierHintLayer int

const (
	modifierHintNone modifierHintLayer = iota
	modifierHintShift
	modifierHintCtrl
	modifierHintAlt
)

func (m *Model) handleModifierHintPress(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if mod, ok := modifierKeyMod(msg.Code); ok {
		m.activeHintMods |= mod
		m.modifierHintMods = 0
		return nil, true
	}

	mods := hintRelevantMods(msg.Mod)
	if mods == 0 {
		m.modifierHintMods = 0
		return nil, false
	}

	m.modifierHintMods = mods
	m.modifierHintFallbackToken++
	token := m.modifierHintFallbackToken
	return tea.Tick(modifierHintFallbackDuration, func(time.Time) tea.Msg {
		return modifierHintExpiredMsg{Token: token}
	}), false
}

func (m *Model) handleModifierHintRelease(msg tea.KeyReleaseMsg) {
	if mod, ok := modifierKeyMod(msg.Key().Code); ok {
		m.activeHintMods &^= mod
	}
}

func (m *Model) activeModifierHintLayer() modifierHintLayer {
	mods := hintRelevantMods(m.activeHintMods)
	if mods == 0 {
		mods = hintRelevantMods(m.modifierHintMods)
	}
	switch {
	case mods.Contains(tea.ModCtrl):
		return modifierHintCtrl
	case mods.Contains(tea.ModAlt):
		return modifierHintAlt
	case mods.Contains(tea.ModShift):
		return modifierHintShift
	default:
		return modifierHintNone
	}
}

func hintRelevantMods(mod tea.KeyMod) tea.KeyMod {
	var result tea.KeyMod
	if mod.Contains(tea.ModShift) {
		result |= tea.ModShift
	}
	if mod.Contains(tea.ModCtrl) {
		result |= tea.ModCtrl
	}
	if mod.Contains(tea.ModAlt) {
		result |= tea.ModAlt
	}
	return result
}

func modifierKeyMod(code rune) (tea.KeyMod, bool) {
	switch code {
	case tea.KeyLeftShift, tea.KeyRightShift:
		return tea.ModShift, true
	case tea.KeyLeftCtrl, tea.KeyRightCtrl:
		return tea.ModCtrl, true
	case tea.KeyLeftAlt, tea.KeyRightAlt, tea.KeyIsoLevel3Shift:
		return tea.ModAlt, true
	default:
		return 0, false
	}
}

func (m *Model) modifierHintText(layer modifierHintLayer, chrome ChromeState, defaultHints string) string {
	if m.pendingDeleteConfirm || m.pendingUnsubscribe {
		return defaultHints
	}
	var segments []string
	switch layer {
	case modifierHintCtrl:
		segments = m.ctrlModifierHintSegments(chrome)
	case modifierHintAlt:
		segments = m.altModifierHintSegments(chrome, defaultHints)
	case modifierHintShift:
		segments = m.shiftModifierHintSegments(chrome)
	}
	if len(segments) == 0 {
		return defaultHints
	}
	return joinHintSegments(segments...)
}

func (m *Model) shiftModifierHintSegments(chrome ChromeState) []string {
	if m.activeTab == tabCompose {
		return []string{m.primaryTabShortcutHint(), "shift+tab: prev field", "esc: back"}
	}
	if m.showLogs {
		return []string{"L: close logs", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit")}
	}
	if m.activeTab == tabContacts {
		return []string{m.primaryTabShortcutHint(), "shift+tab: prev panel", "?: semantic", "esc: clear"}
	}
	if m.activeTab == tabCleanup {
		if m.showCleanupPreview {
			return []string{"shift+tab: prev panel", "H: hide future mail", "D: delete", "A/T: re-classify", "esc: close preview"}
		}
		return []string{m.primaryTabShortcutHint(), "shift+tab: prev panel", "H: hide future mail", "D: delete", "W/C/P: tools", "S: settings"}
	}
	if m.activeTab == tabTimeline {
		segments := []string{"shift+tab: prev panel", "shift+↑/↓: range", "C: compose", "S: settings"}
		if m.currentTimelineRowEmail() != nil || m.timeline.selectedEmail != nil {
			segments = append(segments, "R: sender", "D: delete", "U: unread", "F: forward", "T/A: re-classify")
		}
		if chrome.FocusedPanel == panelPreview {
			segments = append(segments, "Y: copy all", "H: hide future mail")
		}
		return segments
	}
	return nil
}

func (m *Model) ctrlModifierHintSegments(chrome ChromeState) []string {
	if m.activeTab == tabCompose {
		segments := []string{"ctrl+s: send", "ctrl+p: preview", "ctrl+a: attach", "ctrl+g: AI", "ctrl+j: subject", "ctrl+c: quit"}
		if m.composePreserved != nil {
			segments = append([]string{"ctrl+o: preserve mode"}, segments...)
		}
		return segments
	}
	if m.timeline.searchMode && m.activeTab == tabTimeline && m.timeline.searchFocus == timelineSearchFocusInput {
		return []string{"ctrl+i: server search", "ctrl+c: quit"}
	}
	if m.activeTab == tabTimeline {
		segments := []string{"ctrl+c: quit", "ctrl+r: refresh", "ctrl+d/u: half-page"}
		if m.timeline.quickRepliesReady || m.timeline.quickReplyOpen || chrome.FocusedPanel == panelPreview {
			segments = append(segments, "ctrl+q: quick reply")
		}
		if m.currentTimelineDraftEmail() != nil {
			segments = append(segments, "ctrl+s: send draft")
		}
		return segments
	}
	if m.activeTab == tabCleanup {
		return []string{"ctrl+c: quit", "ctrl+r: refresh"}
	}
	if m.activeTab == tabContacts {
		return []string{"ctrl+c: quit", "ctrl+r: refresh"}
	}
	return []string{"ctrl+c: quit"}
}

func (m *Model) altModifierHintSegments(chrome ChromeState, defaultHints string) []string {
	if m.showLogs {
		return []string{"alt+l: close logs", "esc: close logs"}
	}
	if m.showSettings {
		return []string{"alt+enter: newline in text fields", defaultHints}
	}
	return []string{"alt: no actions here", defaultHints}
}
