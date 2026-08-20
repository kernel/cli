package cmd

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStableExecutablePathUsesHomebrewSymlink(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/opt/homebrew/bin/kernel", stableExecutablePath("/opt/homebrew/Cellar/kernel/1.2.3/bin/kernel"))
	assert.Equal(t, "/usr/local/bin/kernel", stableExecutablePath("/usr/local/Cellar/kernel/1.2.3/bin/kernel"))
	assert.Equal(t, "/Users/me/bin/kernel", stableExecutablePath("/Users/me/bin/kernel"))
}

func TestRecoverConnectorInvalidAPIKeyWithOAuth(t *testing.T) {
	t.Setenv("KERNEL_API_KEY", "disabled")
	confirmed := false
	loggedIn := false

	recovered, err := recoverConnectorAuthentication(
		&cobra.Command{},
		&kernel.Error{StatusCode: http.StatusUnauthorized},
		func(action, prompt string, defaultValue bool) (bool, error) {
			confirmed = true
			assert.Contains(t, action, "browser sign-in")
			assert.Contains(t, prompt, "API key is invalid")
			assert.True(t, defaultValue)
			return true, nil
		},
		func(_ *cobra.Command, force bool) error {
			loggedIn = true
			assert.True(t, force)
			assert.Empty(t, os.Getenv("KERNEL_API_KEY"))
			return nil
		},
	)

	require.NoError(t, err)
	assert.True(t, recovered)
	assert.True(t, confirmed)
	assert.True(t, loggedIn)
	assert.Empty(t, os.Getenv("KERNEL_API_KEY"))
}

func TestRecoverConnectorWrongOAuthAccount(t *testing.T) {
	t.Setenv("KERNEL_API_KEY", "")
	forced := false

	recovered, err := recoverConnectorAuthentication(
		&cobra.Command{},
		errRequestedProjectUnavailable,
		func(_ string, prompt string, defaultValue bool) (bool, error) {
			assert.Contains(t, prompt, "current Kernel account")
			assert.True(t, defaultValue)
			return true, nil
		},
		func(_ *cobra.Command, force bool) error {
			forced = force
			return nil
		},
	)

	require.NoError(t, err)
	assert.True(t, recovered)
	assert.True(t, forced)
}

func TestRecoverConnectorWrongOAuthAccountDeclineGivesAccountGuidance(t *testing.T) {
	t.Setenv("KERNEL_API_KEY", "")

	recovered, err := recoverConnectorAuthentication(
		&cobra.Command{},
		errRequestedProjectUnavailable,
		func(string, string, bool) (bool, error) { return false, nil },
		func(*cobra.Command, bool) error { return nil },
	)

	assert.False(t, recovered)
	require.ErrorContains(t, err, "kernel login --force")
	assert.NotContains(t, err.Error(), "unset KERNEL_API_KEY")
}

func TestAuthenticateConnectorAsksToLoginWhenCredentialsAreMissing(t *testing.T) {
	clientCalls := 0
	confirmed := false
	loggedIn := false

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := authenticateConnectorWith(
		cmd,
		func(action, prompt string, defaultValue bool) (bool, error) {
			confirmed = true
			assert.Contains(t, action, "sign in")
			assert.Contains(t, prompt, "Sign in to Kernel")
			assert.True(t, defaultValue)
			return true, nil
		},
		func(_ *cobra.Command, force bool) error {
			loggedIn = true
			assert.False(t, force)
			return nil
		},
		func(...option.RequestOption) (*kernel.Client, error) {
			clientCalls++
			if clientCalls == 1 {
				return nil, auth.ErrAuthenticationRequired
			}
			client := kernel.NewClient()
			return &client, nil
		},
	)

	require.NoError(t, err)
	assert.True(t, confirmed)
	assert.True(t, loggedIn)
	assert.Equal(t, 2, clientCalls)
}

func TestAuthenticateConnectorPreservesNonAuthFailure(t *testing.T) {
	prompted := false
	want := assert.AnError
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := authenticateConnectorWith(
		cmd,
		func(string, string, bool) (bool, error) {
			prompted = true
			return true, nil
		},
		func(*cobra.Command, bool) error {
			t.Fatal("login must not run for a credential storage failure")
			return nil
		},
		func(...option.RequestOption) (*kernel.Client, error) { return nil, want },
	)

	require.ErrorIs(t, err, want)
	assert.False(t, prompted)
}

func TestRecoverConnectorAPIKeyDoesNotHideNonAuthFailure(t *testing.T) {
	t.Setenv("KERNEL_API_KEY", "present")
	prompted := false

	recovered, err := recoverConnectorAuthentication(
		&cobra.Command{},
		&kernel.Error{StatusCode: http.StatusInternalServerError},
		func(string, string, bool) (bool, error) {
			prompted = true
			return true, nil
		},
		func(*cobra.Command, bool) error {
			t.Fatal("login must not run for a non-authentication failure")
			return nil
		},
	)

	require.NoError(t, err)
	assert.False(t, recovered)
	assert.False(t, prompted)
	assert.Equal(t, "present", os.Getenv("KERNEL_API_KEY"))
}

func TestRecoverConnectorAPIKeyDeclinePreservesEnvironment(t *testing.T) {
	t.Setenv("KERNEL_API_KEY", "disabled")

	recovered, err := recoverConnectorAuthentication(
		&cobra.Command{},
		&kernel.Error{StatusCode: http.StatusUnauthorized},
		func(string, string, bool) (bool, error) { return false, nil },
		func(*cobra.Command, bool) error {
			t.Fatal("login must not run after the user declines")
			return nil
		},
	)

	assert.False(t, recovered)
	require.ErrorContains(t, err, "browser sign-in declined")
	assert.Equal(t, "disabled", os.Getenv("KERNEL_API_KEY"))
}

func TestRecoverConnectorAPIKeyStopsAfterCanceledLogin(t *testing.T) {
	t.Setenv("KERNEL_API_KEY", "disabled")

	recovered, err := recoverConnectorAuthentication(
		&cobra.Command{},
		&kernel.Error{StatusCode: http.StatusUnauthorized},
		func(string, string, bool) (bool, error) { return true, nil },
		func(*cobra.Command, bool) error { return errLoginCanceled },
	)

	assert.False(t, recovered)
	require.ErrorIs(t, err, errLoginCanceled)
}
