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
	script := macOSAppleScript("/opt/homebrew/bin/kernel")
	command := exec.Command("/usr/bin/osacompile", "-o", app, "-e", script)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}
