package cmd

import (
	"github.com/kernel/cli/pkg/table"
	"github.com/pterm/pterm"
)

// PrintTableNoPad is a wrapper around pkg/table.PrintTableNoPad for backwards compatibility
func PrintTableNoPad(data pterm.TableData, hasHeader bool) {
	table.PrintTableNoPad(data, hasHeader)
}
