package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// renderStyledTableView replaces the bubbles/table View() call with an
// ANSI-aware renderer. bubbles/table uses runewidth.Truncate on cell values,
// which incorrectly counts ANSI escape body chars as visible width, causing
// mid-sequence truncation that produces garbled output.
//
// This function uses ansi.Truncate (from charmbracelet/x/ansi) which correctly
// measures visual width while skipping escape sequences.
//
// senderCol is the column index whose values may contain ANSI styling (pass -1
// to disable; all cells are rendered plain-aware regardless).
//
// The bubbles table is kept for all state (cursor, key handling, row/column
// data). Only rendering is replaced here.
func renderStyledTableView(t *table.Model, _ int) string {
	cols := t.Columns()
	rows := t.Rows()
	cursor := t.Cursor()
	height := t.Height() // visible rows (excluding header)

	nrows := len(rows)
	styles := table.DefaultStyles()
	if nrows == 0 {
		return renderTableHeader(cols, styles)
	}

	// Compute the visible row window. We keep cursor at the bottom of the
	// window when scrolling down (standard terminal table behaviour).
	start := cursor - height + 1
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > nrows {
		end = nrows
		// Adjust start so we always fill the viewport when possible.
		start = end - height
		if start < 0 {
			start = 0
		}
	}

	var sb strings.Builder
	sb.WriteString(renderTableHeader(cols, styles))

	for i := start; i < end; i++ {
		row := rows[i]
		sb.WriteByte('\n')
		sb.WriteString(renderTableRow(cols, row, i == cursor, styles))
	}

	return sb.String()
}

// renderTableHeader renders the header row using the bubbles table header style.
func renderTableHeader(cols []table.Column, styles table.Styles) string {
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		if col.Width <= 0 {
			continue
		}
		cell := ansi.Truncate(col.Title, col.Width, "…")
		rendered := lipgloss.NewStyle().
			Width(col.Width).MaxWidth(col.Width).Inline(true).
			Render(cell)
		parts = append(parts, styles.Header.Render(rendered))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// renderTableRow renders a single data row. When selected is true it wraps the
// whole row in the Selected style, matching bubbles/table behaviour.
func renderTableRow(cols []table.Column, row table.Row, selected bool, styles table.Styles) string {
	parts := make([]string, 0, len(cols))
	for i, col := range cols {
		if col.Width <= 0 {
			continue
		}
		value := ""
		if i < len(row) {
			value = row[i]
		}
		// ansi.Truncate respects ANSI escape sequences so styled values are
		// clipped at the correct visual position.
		cell := ansi.Truncate(value, col.Width, "…")
		rendered := styles.Cell.Render(
			lipgloss.NewStyle().Width(col.Width).MaxWidth(col.Width).Inline(true).Render(cell),
		)
		parts = append(parts, rendered)
	}
	line := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	if selected {
		return styles.Selected.Render(line)
	}
	return line
}
