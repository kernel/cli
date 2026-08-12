package cmd

import (
	"errors"
	"testing"

	"github.com/kernel/cli/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestConnectorLoginRequiresSavedTokens(t *testing.T) {
	want := errors.New("storage unavailable")

	err := saveLoginTokens(&auth.TokenStorage{}, true, func(*auth.TokenStorage) error { return want })

	require.ErrorIs(t, err, want)
}

func TestNormalLoginKeepsLegacyWarningOnlySaveBehavior(t *testing.T) {
	err := saveLoginTokens(&auth.TokenStorage{}, false, func(*auth.TokenStorage) error {
		return errors.New("storage unavailable")
	})

	require.NoError(t, err)
}
