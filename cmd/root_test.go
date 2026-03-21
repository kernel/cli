package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRootCommandSubcommands(t *testing.T) {
	expected := []string{
		"app",
		"browser-pools",
		"browsers",
		"create",
		"credentials",
		"deploy",
		"extensions",
		"invoke",
		"login",
		"logout",
		"profiles",
		"proxies",
	}

	registered := make(map[string]bool)
	for _, sub := range rootCmd.Commands() {
		registered[sub.Name()] = true
	}

	for _, name := range expected {
		assert.True(t, registered[name], "expected subcommand %q to be registered on rootCmd", name)
	}
}

func TestRootCommandHelpOutput(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "kernel")
	assert.Contains(t, output, "deploy")
	assert.Contains(t, output, "invoke")
	assert.Contains(t, output, "browsers")
}

func TestLogLevelToPterm(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"trace", "trace"},
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"fatal", "fatal"},
		{"print", "print"},
		{"garbage", "info"},
		{"", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := logLevelToPterm(tt.input)
			expected := logLevelToPterm(tt.expected)
			assert.Equal(t, expected, got)
		})
	}
}

func TestIsUsageError(t *testing.T) {
	tests := []struct {
		name     string
		err      string
		expected bool
	}{
		{"unknown flag", "unknown flag: --foo", true},
		{"unknown command", "unknown command \"bogus\"", true},
		{"invalid argument", "invalid argument \"x\" for \"--count\"", true},
		{"random error", "connection refused", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.err)
			assert.Equal(t, tt.expected, isUsageError(err))
		})
	}
}
