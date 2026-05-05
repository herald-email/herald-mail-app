package app

import (
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
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
// The bubbles table is kept for all state (cursor, key handling, row/column
// data). Only rendering is replaced here.
func renderStyledTableView(t *table.Model, _ int) string {
	return renderStyledTableViewWithStyles(t, table.DefaultStyles())
}

type tableRenderOptions struct {
	compactLeadingCell bool
}

func renderStyledTableViewWithStyles(t *table.Model, styles table.Styles) string {
	return renderStyledTableViewWithStylesAndOptions(t, styles, tableRenderOptions{})
}

func renderStyledTableViewWithCompactLeadingCell(t *table.Model, styles table.Styles) string {
	return renderStyledTableViewWithStylesAndOptions(t, styles, tableRenderOptions{compactLeadingCell: true})
}

func renderStyledTableViewWithStylesAndOptions(t *table.Model, styles table.Styles, opts tableRenderOptions) string {
	cols := t.Columns()
	rows := t.Rows()
	cursor := t.Cursor()
	height := t.Height() // visible rows (excluding header)
	targetWidth := t.Width()

	nrows := len(rows)
	if nrows == 0 {
		return fitTableHeaderLine(renderTableHeader(cols, styles, opts), targetWidth, styles)
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
	sb.WriteString(fitTableHeaderLine(renderTableHeader(cols, styles, opts), targetWidth, styles))

	for i := start; i < end; i++ {
		row := rows[i]
		sb.WriteByte('\n')
		sb.WriteString(fitTableLine(renderTableRow(cols, row, i == cursor, styles, opts), targetWidth))
	}

	// Pad with blank rows to fill the viewport when data rows < height.
	// This ensures panels stretch to the full terminal height (fixes Cleanup tab
	// with few senders leaving large blank gaps below the panel border).
	rendered := end - start
	if rendered < height {
		emptyRow := make(table.Row, len(cols))
		for i := rendered; i < height; i++ {
			sb.WriteByte('\n')
			sb.WriteString(fitTableLine(renderTableRow(cols, emptyRow, false, styles, opts), targetWidth))
		}
	}

	return sb.String()
}

func fitTableLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	line = ansi.Truncate(line, width, "")
	if missing := width - ansi.StringWidth(line); missing > 0 {
		line += strings.Repeat(" ", missing)
	}
	return line
}

func fitTableHeaderLine(line string, width int, styles table.Styles) string {
	if width <= 0 {
		return line
	}
	line = ansi.Truncate(line, width, "")
	if missing := width - ansi.StringWidth(line); missing > 0 {
		padStyle := lipgloss.NewStyle().
			Foreground(styles.Header.GetForeground()).
			Background(styles.Header.GetBackground()).
			Bold(styles.Header.GetBold())
		line += padStyle.Render(strings.Repeat(" ", missing))
	}
	return line
}

// renderTableHeader renders the header row using the bubbles table header style.
func renderTableHeader(cols []table.Column, styles table.Styles, opts tableRenderOptions) string {
	parts := make([]string, 0, len(cols))
	headerStyle := lipgloss.NewStyle().
		Foreground(styles.Header.GetForeground()).
		Background(styles.Header.GetBackground()).
		Bold(styles.Header.GetBold())
	visibleCol := 0
	for _, col := range cols {
		if col.Width <= 0 {
			continue
		}
		cell := ansi.Truncate(col.Title, col.Width, "…")
		rendered := headerStyle.
			Width(col.Width).MaxWidth(col.Width).Inline(true).
			Render(cell)
		cellWrapper := styles.Cell
		if opts.compactLeadingCell && visibleCol == 0 {
			cellWrapper = cellWrapper.PaddingLeft(0).PaddingRight(0)
		}
		cellWrapper = cellWrapper.Background(styles.Header.GetBackground())
		parts = append(parts, cellWrapper.Render(rendered))
		visibleCol++
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// renderTableRow renders a single data row. When selected is true, the Selected
// style is applied per-cell to ensure the background color covers the full row
// (wrapping the joined line doesn't work because inner ANSI codes reset the bg).
func renderTableRow(cols []table.Column, row table.Row, selected bool, styles table.Styles, opts tableRenderOptions) string {
	parts := make([]string, 0, len(cols))
	visibleCol := 0
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
		cellStyle := lipgloss.NewStyle().Width(col.Width).MaxWidth(col.Width).Inline(true)
		if selected {
			// Apply selection colors directly to each cell so the background
			// isn't overridden by inner ANSI codes from styled sender etc.
			cellStyle = cellStyle.
				Foreground(styles.Selected.GetForeground()).
				Background(styles.Selected.GetBackground()).
				Underline(styles.Selected.GetUnderline()).
				Bold(styles.Selected.GetBold())
			// Strip inner ANSI from cell value to prevent color conflicts.
			cell = ansi.Strip(cell)
		}
		cellWrapper := styles.Cell
		if opts.compactLeadingCell && visibleCol == 0 {
			cellWrapper = cellWrapper.PaddingLeft(0).PaddingRight(0)
		}
		rendered := cellWrapper.Render(cellStyle.Render(cell))
		parts = append(parts, rendered)
		visibleCol++
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
