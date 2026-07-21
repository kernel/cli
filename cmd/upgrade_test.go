package cmd

import (
	"testing"

	"github.com/kernel/cli/pkg/update"
	"github.com/stretchr/testify/assert"
)

func TestUpgradeCommandArgs(t *testing.T) {
	tests := []struct {
		method   update.InstallMethod
		expected [][]string
	}{
		{
			// brew upgrade no-ops on a stale tap, so update must run first.
			update.InstallMethodBrew,
			[][]string{
				{"brew", "update"},
				{"brew", "upgrade", "kernel/tap/kernel"},
			},
		},
		{update.InstallMethodNPM, [][]string{{"npm", "i", "-g", "@onkernel/cli@latest"}}},
		{update.InstallMethodPNPM, [][]string{{"pnpm", "add", "-g", "@onkernel/cli@latest"}}},
		{update.InstallMethodBun, [][]string{{"bun", "add", "-g", "@onkernel/cli@latest"}}},
		{update.InstallMethodUnknown, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			assert.Equal(t, tt.expected, upgradeCommandArgs(tt.method))
		})
	}
}

func TestUpgradeCommandArgsBrewUpdatesBeforeUpgrade(t *testing.T) {
	cmds := upgradeCommandArgs(update.InstallMethodBrew)
	updateIdx, upgradeIdx := -1, -1
	for i, args := range cmds {
		if len(args) >= 2 && args[0] == "brew" && args[1] == "update" {
			updateIdx = i
		}
		if len(args) >= 2 && args[0] == "brew" && args[1] == "upgrade" {
			upgradeIdx = i
		}
	}
	assert.NotEqual(t, -1, updateIdx, "expected a brew update step")
	assert.NotEqual(t, -1, upgradeIdx, "expected a brew upgrade step")
	assert.Less(t, updateIdx, upgradeIdx, "brew update must run before brew upgrade")
}

func TestGetUpgradeCommandBrew(t *testing.T) {
	assert.Equal(t, "brew update && brew upgrade kernel/tap/kernel", getUpgradeCommand(update.InstallMethodBrew))
	assert.Equal(t, "", getUpgradeCommand(update.InstallMethodUnknown))
}
