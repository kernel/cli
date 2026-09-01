package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadTokensFromFileReportsMissingCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := loadTokensFromFile()

	require.ErrorIs(t, err, ErrNoStoredCredentials)
}

func TestLoadTokensPreservesUnexpectedKeyringFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	want := errors.New("keychain access denied")

	_, err := loadTokens(func(string, string) (string, error) { return "", want })

	require.ErrorIs(t, err, want)
	require.NotErrorIs(t, err, ErrNoStoredCredentials)
}
