package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// ProgramOptions returns Bubble Tea options Herald needs for terminal-native
// behavior. Bubble Tea v2 synchronized output wraps frame bytes in mode 2026;
// iTerm2/Kitty/Ghostty graphics are emitted as terminal control overlays and
// need to remain outside that wrapper to render reliably.
func ProgramOptions() []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithFilter(filterNativeGraphicsUnsafeTerminalModes),
	}
}

func filterNativeGraphicsUnsafeTerminalModes(_ tea.Model, msg tea.Msg) tea.Msg {
	report, ok := msg.(tea.ModeReportMsg)
	if !ok {
		return msg
	}
	if report.Mode == ansi.ModeSynchronizedOutput {
		return nil
	}
	return msg
}
