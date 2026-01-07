package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestIsAuthExempt(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *cobra.Command
		expected bool
	}{
		{
			name:     "root command is exempt",
			cmd:      rootCmd,
			expected: true,
		},
		{
			name:     "login command is exempt",
			cmd:      loginCmd,
			expected: true,
		},
		{
			name:     "logout command is exempt",
			cmd:      logoutCmd,
			expected: true,
		},
		{
			name:     "top-level create command is exempt",
			cmd:      createCmd,
			expected: true,
		},
		{
			name:     "browser-pools create subcommand requires auth",
			cmd:      browserPoolsCreateCmd,
			expected: false,
		},
		{
			name:     "browsers create subcommand requires auth",
			cmd:      browsersCreateCmd,
			expected: false,
		},
		{
			name:     "profiles create subcommand requires auth",
			cmd:      profilesCreateCmd,
			expected: false,
		},
		{
			name:     "browser-pools list requires auth",
			cmd:      browserPoolsListCmd,
			expected: false,
		},
		{
			name:     "browsers list requires auth",
			cmd:      browsersListCmd,
			expected: false,
		},
		{
			name:     "deploy command requires auth",
			cmd:      deployCmd,
			expected: false,
		},
		{
			name:     "invoke command requires auth",
			cmd:      invokeCmd,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAuthExempt(tt.cmd)
			assert.Equal(t, tt.expected, result, "isAuthExempt(%s) = %v, want %v", tt.cmd.Name(), result, tt.expected)
		})
	}
}
