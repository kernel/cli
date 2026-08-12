package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStableExecutablePathUsesHomebrewSymlink(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/opt/homebrew/bin/kernel", stableExecutablePath("/opt/homebrew/Cellar/kernel/1.2.3/bin/kernel"))
	assert.Equal(t, "/usr/local/bin/kernel", stableExecutablePath("/usr/local/Cellar/kernel/1.2.3/bin/kernel"))
	assert.Equal(t, "/Users/me/bin/kernel", stableExecutablePath("/Users/me/bin/kernel"))
}
