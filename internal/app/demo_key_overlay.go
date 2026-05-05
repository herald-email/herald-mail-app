package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const demoKeyOverlayMaxKeys = 6

var demoKeyOverlayLabels = map[string]string{
	"?":          "?",
	"S":          "S",
	"1":          "1",
	"2":          "2",
	"3":          "3",
	"C":          "C",
	"V":          "V",
	"z":          "z",
	"enter":      "Enter",
	"tab":        "Tab",
	"esc":        "Esc",
	"up":         "Up",
	"down":       "Down",
	"left":       "Left",
	"right":      "Right",
	"[":          "[",
	"]":          "]",
	"shift+up":   "Shift+Up",
	"shift+down": "Shift+Down",
}

func demoKeyOverlayLabel(msg tea.KeyPressMsg) string {
	if label, ok := demoKeyOverlayLabels[shortcutKey(msg)]; ok {
		return label
	}
	return ""
}

func (m *Model) recordDemoKeyOverlayPress(msg tea.KeyPressMsg) {
	if !m.demoKeyOverlay || m.demoKeyOverlayTextEntryActive() {
		return
	}
	label := demoKeyOverlayLabel(msg)
	if label == "" {
		return
	}
	m.demoKeyOverlayKeys = append(m.demoKeyOverlayKeys, label)
	if len(m.demoKeyOverlayKeys) > demoKeyOverlayMaxKeys {
		m.demoKeyOverlayKeys = m.demoKeyOverlayKeys[len(m.demoKeyOverlayKeys)-demoKeyOverlayMaxKeys:]
	}
}

func (m *Model) demoKeyOverlayTextEntryActive() bool {
	if m.activeTab == tabCompose {
		return true
	}
	if m.timeline.attachmentSavePrompt {
		return true
	}
	if m.timeline.searchMode && m.activeTab == tabTimeline && m.timeline.searchFocus == timelineSearchFocusInput {
		return true
	}
	if m.showRuleEditor || m.showPromptEditor {
		return true
	}
	if m.activeTab == tabContacts && m.contactSearchMode != "" {
		return true
	}
	if m.showChat && m.focusedPanel == panelChat {
		return true
	}
	return false
}

func (m *Model) renderDemoKeyOverlay(content string) string {
	if !m.demoKeyOverlay || len(m.demoKeyOverlayKeys) == 0 {
		return content
	}

	width := m.windowWidth
	if width <= 0 {
		width = 80
	}
	height := m.windowHeight
	if height <= 0 {
		height = 24
	}

	maxLabelWidth := width - 6
	if maxLabelWidth < 8 {
		return content
	}
	label := truncateVisual("Keys: "+strings.Join(m.demoKeyOverlayKeys, "  "), maxLabelWidth)
	badge := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("238")).
		Padding(0, 1).
		Render(label)
	badgeW := ansi.StringWidth(badge)
	if badgeW > width {
		return content
	}

	row := 2
	if height < 6 {
		row = height - 1
	}
	if row < 0 {
		row = 0
	}
	if row >= height {
		row = height - 1
	}
	startX := width - badgeW - 2
	if startX < 0 {
		startX = 0
	}

	lines := splitViewportLines(content, height)
	line := lines[row]
	left := padANSIToWidth(ansi.Cut(line, 0, startX), startX)
	right := ansi.Cut(line, startX+badgeW, width)
	lines[row] = ansi.Cut(left+badge+right, 0, width)
	return strings.Join(lines, "\n")
}
