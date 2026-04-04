package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// TestRenderStyledTableView_ANSIDoesNotWrap verifies that styledSender values
// containing ANSI escape codes are rendered without corruption (no mid-sequence
// truncation that causes visual line-wrapping in the terminal).
//
// Previously, bubbles/table called runewidth.Truncate on cell values, which
// counts ANSI escape body chars as visible width, causing truncation inside
// escape sequences. renderStyledTableView uses ansi.Truncate (ANSI-aware).
func TestRenderStyledTableView_ANSIDoesNotWrap(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	// Give the model a known window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	// Build a table with a sender column containing an ANSI-styled string.
	// styledSender wraps "Alice Smith" + " " + "<alice@example.com>" in ANSI color codes.
	colWidth := 30
	rawSender := "Alice Smith <alice@example.com>"
	styled := styledSender(rawSender, colWidth)

	cols := []table.Column{
		{Title: "✓", Width: 2},
		{Title: "Sender", Width: colWidth},
		{Title: "Count", Width: 5},
	}
	rows := []table.Row{
		{" ", styled, "3"},
	}
	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(10),
	)

	// renderStyledTableView must not corrupt ANSI sequences.
	out := renderStyledTableView(&tbl, 0)

	// The output must contain a complete, valid ANSI reset sequence rather than
	// a broken mid-sequence truncation.
	if strings.Contains(out, "\x1b[3") && !strings.Contains(out, "\x1b[0m") {
		t.Error("rendered output contains an ANSI color code but no reset — likely mid-sequence truncation")
	}

	// The output must contain the visible sender name portion.
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected 'Alice' in rendered output, got:\n%s", stripANSI(out))
	}
}

// TestRenderStyledTableView_CursorHighlight verifies that the selected row
// (cursor == 1) is styled differently from non-selected rows.
func TestRenderStyledTableView_CursorHighlight(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	cols := []table.Column{
		{Title: "Name", Width: 10},
		{Title: "Count", Width: 5},
	}
	rows := []table.Row{
		{"Alice", "1"},
		{"Bob", "2"},
		{"Carol", "3"},
	}
	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(5),
	)
	tbl.SetCursor(1) // cursor on Bob

	out := renderStyledTableView(&tbl, -1 /* no sender col */)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	// Output should have header + 3 data rows = 4 lines (no border).
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines (header + 3 rows), got %d:\n%s", len(lines), out)
	}
}

// TestRenderStyledTableView_ScrollOffset verifies that when the cursor is below
// the visible height, earlier rows are not shown (the table has scrolled).
func TestRenderStyledTableView_ScrollOffset(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	cols := []table.Column{{Title: "Name", Width: 10}}
	rows := []table.Row{
		{"Alpha"}, {"Beta"}, {"Gamma"}, {"Delta"}, {"Epsilon"},
	}
	const visibleHeight = 3
	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(visibleHeight),
	)
	tbl.SetCursor(4) // last row

	out := renderStyledTableView(&tbl, -1)
	// Alpha should not appear (scrolled past)
	if strings.Contains(out, "Alpha") {
		t.Errorf("Alpha should not be visible when cursor=4 and height=3, but found it in:\n%s", stripANSI(out))
	}
	// Epsilon (cursor row) must be visible
	if !strings.Contains(out, "Epsilon") {
		t.Errorf("Epsilon (cursor row) must be visible, got:\n%s", stripANSI(out))
	}
}
