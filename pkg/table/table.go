package table

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// PrintTableNoPad renders a table similar to pterm.DefaultTable, but it avoids
// adding trailing padding spaces after the last column and does not add blank
// padded lines to match multi-line cells in other columns. The last column may
// contain multi-line content which will be printed as-is on following lines.
// It also intelligently truncates columns to prevent line wrapping.
func PrintTableNoPad(data pterm.TableData, hasHeader bool) {
	if len(data) == 0 {
		return
	}

	// Only truncate columns when outputting to a terminal.
	// When piped (non-TTY), output full values so grep/awk/etc. work correctly.
	if term.IsTerminal(int(os.Stdout.Fd())) {
		termWidth := pterm.GetTerminalWidth()
		if termWidth <= 0 {
			termWidth = 80 // fallback
		}
		data = truncateTableData(data, termWidth)
	}

	// Determine number of columns from the first row
	numCols := len(data[0])
	if numCols == 0 {
		return
	}

	// Pre-compute max width per column (including last column for proper alignment)
	maxColWidths := make([]int, numCols)
	for _, row := range data {
		for colIdx := 0; colIdx < numCols && colIdx < len(row); colIdx++ {
			for _, line := range strings.Split(row[colIdx], "\n") {
				// Strip color codes for accurate width measurement
				visibleLine := pterm.RemoveColorFromString(line)
				if w := utf8.RuneCountInString(visibleLine); w > maxColWidths[colIdx] {
					maxColWidths[colIdx] = w
				}
			}
		}
	}

	var b strings.Builder
	sep := pterm.DefaultTable.Separator
	sepStyled := pterm.ThemeDefault.TableSeparatorStyle.Sprint(sep)

	renderRow := func(row []string, styleHeader bool) {
		// Build and pad all columns for proper alignment
		parts := make([]string, 0, numCols)
		for colIdx := 0; colIdx < numCols; colIdx++ {
			var cell string
			if colIdx < len(row) {
				cell = row[colIdx]
			}

			// Get first line only
			lines := strings.Split(cell, "\n")
			first := ""
			if len(lines) > 0 {
				first = lines[0]
			}

			// Pad to column width (measure visible chars, accounting for color codes)
			visibleFirst := pterm.RemoveColorFromString(first)
			padCount := maxColWidths[colIdx] - utf8.RuneCountInString(visibleFirst)
			if padCount < 0 {
				padCount = 0
			}
			parts = append(parts, first+strings.Repeat(" ", padCount))
		}

		line := strings.Join(parts, sepStyled)

		if styleHeader {
			b.WriteString(pterm.ThemeDefault.TableHeaderStyle.Sprint(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	for idx, row := range data {
		renderRow(row, hasHeader && idx == 0)
	}

	pterm.Print(b.String())
}

// IsStdoutTTY reports whether stdout is connected to a terminal.
func IsStdoutTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// truncateTableData intelligently truncates table cells to fit within terminal width
func truncateTableData(data pterm.TableData, termWidth int) pterm.TableData {
	if len(data) == 0 {
		return data
	}

	numCols := len(data[0])
	if numCols == 0 {
		return data
	}

	// Calculate separator space: " | " between each column (3 chars per separator)
	separatorSpace := (numCols - 1) * 3

	// Define minimum column widths (these are the bare minimum before aggressive truncation)
	minWidths := make([]int, numCols)
	for i := 0; i < numCols; i++ {
		minWidths[i] = 8 // minimum 8 chars per column
	}

	// Calculate natural widths (what each column would want)
	// Strip color codes to measure visible characters only
	naturalWidths := make([]int, numCols)
	for colIdx := 0; colIdx < numCols; colIdx++ {
		maxWidth := 0
		for _, row := range data {
			if colIdx < len(row) {
				// Strip ANSI color codes to get visible character count
				visibleText := pterm.RemoveColorFromString(row[colIdx])
				cellWidth := utf8.RuneCountInString(visibleText)
				if cellWidth > maxWidth {
					maxWidth = cellWidth
				}
			}
		}
		naturalWidths[colIdx] = maxWidth
	}

	// Calculate available space for content
	availableWidth := termWidth - separatorSpace - 2 // -2 for margins

	// Distribute width among columns
	columnWidths := distributeColumnWidths(naturalWidths, minWidths, availableWidth)

	// Truncate cells based on calculated widths
	result := make(pterm.TableData, len(data))
	for rowIdx, row := range data {
		result[rowIdx] = make([]string, len(row))
		for colIdx, cell := range row {
			if colIdx < len(columnWidths) {
				result[rowIdx][colIdx] = truncateCell(cell, columnWidths[colIdx])
			} else {
				result[rowIdx][colIdx] = cell
			}
		}
	}

	return result
}

// distributeColumnWidths calculates optimal width for each column using a two-pass strategy:
// Pass 1: ID and short columns get their full natural width
// Pass 2: Long columns share the remaining space
func distributeColumnWidths(naturalWidths, minWidths []int, availableWidth int) []int {
	numCols := len(naturalWidths)
	result := make([]int, numCols)

	// Start with natural widths
	copy(result, naturalWidths)

	// Calculate total natural width needed
	totalNatural := 0
	for _, w := range naturalWidths {
		totalNatural += w
	}

	// If natural widths fit, use them
	if totalNatural <= availableWidth {
		return result
	}

	// Define threshold for "short" columns (these get priority)
	const shortColumnThreshold = 15

	// Pass 1: Give ID (index 0) and short columns their full natural width
	remainingWidth := availableWidth
	longColumnIndices := []int{}

	for i := 0; i < numCols; i++ {
		if i == 0 || naturalWidths[i] <= shortColumnThreshold {
			// Short column or ID - give full natural width
			result[i] = naturalWidths[i]
			remainingWidth -= naturalWidths[i]
		} else {
			// Long column - defer to pass 2
			longColumnIndices = append(longColumnIndices, i)
		}
	}

	// Pass 2: Distribute remaining space among long columns
	if len(longColumnIndices) == 0 {
		return result
	}

	// Calculate how much long columns want
	totalLongNatural := 0
	totalLongMin := 0
	for _, idx := range longColumnIndices {
		totalLongNatural += naturalWidths[idx]
		totalLongMin += minWidths[idx]
	}

	if totalLongNatural <= remainingWidth {
		// Long columns fit naturally
		for _, idx := range longColumnIndices {
			result[idx] = naturalWidths[idx]
		}
		return result
	}

	if totalLongMin > remainingWidth {
		// Even minimums don't fit, distribute equally
		for _, idx := range longColumnIndices {
			result[idx] = remainingWidth / len(longColumnIndices)
			if result[idx] < 5 {
				result[idx] = 5 // absolute minimum
			}
		}
		return result
	}

	// Give long columns minimum, then distribute remainder proportionally
	extraSpace := remainingWidth - totalLongMin
	extraNeed := totalLongNatural - totalLongMin

	for _, idx := range longColumnIndices {
		result[idx] = minWidths[idx]
		if extraNeed > 0 {
			additionalNeed := naturalWidths[idx] - minWidths[idx]
			additionalGrant := (additionalNeed * extraSpace) / extraNeed
			result[idx] += additionalGrant
		}
	}

	return result
}

// truncateCell truncates a cell to maxWidth, adding "..." if truncated
// Handles ANSI color codes properly by measuring visible characters only
func truncateCell(cell string, maxWidth int) string {
	// Strip ANSI codes to measure visible width
	visibleText := pterm.RemoveColorFromString(cell)
	cellWidth := utf8.RuneCountInString(visibleText)

	if cellWidth <= maxWidth {
		return cell
	}

	// Cell needs truncation
	// If the cell has color codes, we need to be careful about truncation
	// For simplicity, strip colors, truncate, then return without color
	if maxWidth <= 3 {
		// Too narrow for "...", just truncate
		return truncateString(visibleText, maxWidth)
	}

	// Truncate and add "..."
	return truncateString(visibleText, maxWidth-3) + "..."
}

// truncateString truncates a string to the specified number of runes
func truncateString(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	return string(runes[:maxRunes])
}
