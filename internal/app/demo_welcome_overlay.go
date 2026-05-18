package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const demoWelcomeMaxWidth = 68

func (m *Model) shouldShowDemoWelcomeOverlay() bool {
	if !m.demoMode || !m.showDemoWelcome {
		return false
	}
	if m.windowWidth > 0 && m.windowWidth < minTermWidth {
		return false
	}
	if m.windowHeight > 0 && m.windowHeight < minTermHeight {
		return false
	}
	return !m.loading || m.hasVisibleStartupData()
}

func (m *Model) handleDemoWelcomeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.shouldShowDemoWelcomeOverlay() {
		return m, nil, false
	}

	switch shortcutKey(msg) {
	case "ctrl+c":
		m.cleanup()
		return m, tea.Quit, true
	case "q":
		m.cleanup()
		return m, tea.Quit, true
	case "esc", "enter", " ", "space":
		m.showDemoWelcome = false
		return m, nil, true
	default:
		return m, nil, true
	}
}

func (m *Model) renderDemoWelcomeOverlayView() string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	h := m.windowHeight
	if h <= 0 {
		h = 24
	}

	backdrop := m.renderShortcutHelpBackdropView()
	panel := renderDemoWelcomePanelWithTheme(m.theme, w)
	return overlayCentered(backdrop, panel, w, h)
}

func renderDemoWelcomePanel(width int) string {
	return renderDemoWelcomePanelWithTheme(defaultTheme, width)
}

func renderDemoWelcomePanelWithTheme(theme Theme, width int) string {
	panelW := demoWelcomeMaxWidth
	if maxW := width - 8; maxW < panelW {
		panelW = maxW
	}
	if panelW < 44 {
		panelW = width - 4
	}
	if panelW < 32 {
		panelW = 32
	}

	styleW := panelW - 2
	if styleW < 30 {
		styleW = 30
	}
	contentW := styleW - 4
	if contentW < 24 {
		contentW = 24
	}

	titleStyle := lipgloss.NewStyle().Foreground(theme.Severity.Info.ForegroundColor()).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(theme.Text.Primary.ForegroundColor())
	footerStyle := lipgloss.NewStyle().Foreground(theme.Severity.Success.ForegroundColor()).Bold(true)

	body := `This is a safe synthetic mailbox. A few Herald demo emails are waiting at the top of Timeline. Open the first email, "Welcome to Herald", to start the onboarding guide, or dismiss this and explore on your own.`

	lines := []string{
		titleStyle.Render(ansi.Truncate("Welcome to Herald Demo", contentW, "")),
		strings.Repeat("─", contentW),
	}
	for _, line := range wrapText(body, contentW) {
		lines = append(lines, bodyStyle.Render(ansi.Truncate(line, contentW, "")))
	}
	lines = append(lines,
		"",
		footerStyle.Render(ansi.Truncate("Esc / Space / Enter: continue", contentW, "")),
	)

	return lipgloss.NewStyle().
		Width(styleW).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Focus.PanelBorderFocused.ForegroundColor()).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}
