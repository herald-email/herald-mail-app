package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type compactOverlayLayout struct {
	panelWidth    int
	panelHeight   int
	contentWidth  int
	contentHeight int
}

func newCompactOverlayLayout(width, height int) compactOverlayLayout {
	return newCompactOverlayLayoutWithMax(width, height, shortcutHelpMaxWidth, shortcutHelpMaxHeight)
}

func newCompactOverlayLayoutWithMax(width, height, maxWidth, maxHeight int) compactOverlayLayout {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	panelW := maxWidth
	if maxW := width - 4; maxW < panelW {
		panelW = maxW
	}
	if panelW < 40 {
		panelW = width
	}
	if panelW < 32 {
		panelW = 32
	}

	panelH := maxHeight
	if maxH := height - 4; maxH < panelH {
		panelH = maxH
	}
	if panelH < 10 {
		panelH = height
	}
	if panelH < 6 {
		panelH = 6
	}

	contentW := panelW - 6
	if contentW < 20 {
		contentW = 20
	}
	contentH := panelH - 4
	if contentH < 1 {
		contentH = 1
	}

	return compactOverlayLayout{
		panelWidth:    panelW,
		panelHeight:   panelH,
		contentWidth:  contentW,
		contentHeight: contentH,
	}
}

func renderCompactOverlayBox(content string, layout compactOverlayLayout) string {
	box := lipgloss.NewStyle().
		Width(layout.panelWidth-2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	rendered := strings.TrimRight(box.Render(strings.TrimRight(content, "\n")), "\n")
	lines := strings.Split(rendered, "\n")
	if len(lines) > layout.panelHeight {
		lines = lines[:layout.panelHeight]
	}
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, layout.panelWidth)
	}
	return strings.Join(lines, "\n")
}
