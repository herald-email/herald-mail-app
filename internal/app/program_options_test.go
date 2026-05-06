package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestProgramFilterSuppressesSynchronizedOutputModeReports(t *testing.T) {
	msg := tea.ModeReportMsg{
		Mode:  ansi.ModeSynchronizedOutput,
		Value: ansi.ModeReset,
	}

	if got := filterNativeGraphicsUnsafeTerminalModes(nil, msg); got != nil {
		t.Fatalf("sync-output mode report should be suppressed so native graphics are not wrapped, got %#v", got)
	}
}

func TestProgramFilterPreservesOtherModeReports(t *testing.T) {
	msg := tea.ModeReportMsg{
		Mode:  ansi.ModeUnicodeCore,
		Value: ansi.ModeReset,
	}

	if got := filterNativeGraphicsUnsafeTerminalModes(nil, msg); got == nil {
		t.Fatal("non-sync mode report should pass through")
	}
}
