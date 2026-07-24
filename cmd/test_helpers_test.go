package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/pterm/pterm"
)

func capturePtermOutput(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer
	pterm.SetDefaultOutput(&buf)
	pterm.Info.Writer = &buf
	pterm.Error.Writer = &buf
	pterm.Success.Writer = &buf
	pterm.Warning.Writer = &buf
	pterm.Debug.Writer = &buf
	pterm.Fatal.Writer = &buf
	pterm.DefaultTable = *pterm.DefaultTable.WithWriter(&buf)
	t.Cleanup(func() {
		pterm.SetDefaultOutput(os.Stdout)
		pterm.Info.Writer = os.Stdout
		pterm.Error.Writer = os.Stdout
		pterm.Success.Writer = os.Stdout
		pterm.Warning.Writer = os.Stdout
		pterm.Debug.Writer = os.Stdout
		pterm.Fatal.Writer = os.Stdout
		pterm.DefaultTable = *pterm.DefaultTable.WithWriter(os.Stdout)
	})
	return &buf
}
