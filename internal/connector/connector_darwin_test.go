//go:build darwin

package connector

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMacOSAppleScriptCompiles(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Kernel Connector.app")
	command := exec.Command("/usr/bin/osacompile", "-o", app, "-e", macOSAppleScript("/opt/homebrew/bin/kernel"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}
