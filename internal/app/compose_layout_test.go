package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestComposeCCBCCWidth_MatchesToField verifies that CC and BCC textinput
// fields are given the same width as the To field after a window resize.
// Regression test for the "tiny box" rendering bug where CC/BCC showed
// only a single character because their Width was never set.
func TestComposeCCBCCWidth_MatchesToField(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	// Simulate a window resize event — this triggers the width calculation
	// that sets composeTo.Width and composeSubject.Width.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	if m.composeCC.Width == 0 {
		t.Fatal("composeCC.Width is 0 — width was never set")
	}
	if m.composeCC.Width != m.composeTo.Width {
		t.Fatalf("CC width %d != To width %d", m.composeCC.Width, m.composeTo.Width)
	}
	if m.composeBCC.Width != m.composeTo.Width {
		t.Fatalf("BCC width %d != To width %d", m.composeBCC.Width, m.composeTo.Width)
	}
}
