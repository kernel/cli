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

func TestResolveProjectSelection(t *testing.T) {
	t.Run("flag value wins over env vars", func(t *testing.T) {
		t.Setenv("KERNEL_PROJECT", "env-project")
		t.Setenv("KERNEL_PROJECT_ID", "env-project-id")
		assert.Equal(t, "flag-project", resolveProjectSelection("flag-project"))
	})

	t.Run("new env var wins over legacy env var", func(t *testing.T) {
		t.Setenv("KERNEL_PROJECT", "env-project")
		t.Setenv("KERNEL_PROJECT_ID", "env-project-id")
		assert.Equal(t, "env-project", resolveProjectSelection(""))
	})

	t.Run("legacy env var used as fallback", func(t *testing.T) {
		t.Setenv("KERNEL_PROJECT", "")
		t.Setenv("KERNEL_PROJECT_ID", "env-project-id")
		assert.Equal(t, "env-project-id", resolveProjectSelection(""))
	})

	t.Run("empty when no flag or env var", func(t *testing.T) {
		t.Setenv("KERNEL_PROJECT", "")
		t.Setenv("KERNEL_PROJECT_ID", "")
		assert.Equal(t, "", resolveProjectSelection(""))
	})
}
