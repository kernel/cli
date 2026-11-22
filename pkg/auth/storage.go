package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	KeyringService = "kernel-cli"
	KeyringUser    = "oauth-tokens"
)

// TokenStorage represents stored authentication tokens
type TokenStorage struct {
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token"`
	ExpiresAt     time.Time `json:"expires_at"`
	OrgID         string    `json:"org_id"`
	AccessScope   string    `json:"access_scope,omitempty"`
	ProjectID     string    `json:"project_id,omitempty"`
	AuthBaseURL   string    `json:"auth_base_url,omitempty"`
	OAuthClientID string    `json:"oauth_client_id,omitempty"`
	Projects      []string  `json:"projects,omitempty"` // projects a native grant is limited to; empty means all
	// ScopeRecorded is set by CLI versions that record Projects. Credentials
	// saved without it may be limited to projects the CLI does not know about.
	ScopeRecorded bool `json:"scope_recorded,omitempty"`
	// SavedAt tells which of the keyring and the credentials file is current
	// when both hold credentials.
	SavedAt time.Time `json:"saved_at,omitzero"`
}

// IsExpired checks if the access token is expired (with 5 minute buffer)
func (t *TokenStorage) IsExpired() bool {
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// setKeyring is keyring.Set, replaceable in tests that need it to fail.
var setKeyring = keyring.Set

// SaveTokens stores authentication tokens in the OS keychain, or in the
// credentials file when the keychain refuses them (for example when they are
// too large). Whichever store is written becomes the one LoadTokens reads.
func SaveTokens(tokens *TokenStorage) error {
	stamped := *tokens
	stamped.SavedAt = time.Now().UTC()
	data, err := json.Marshal(stamped)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	if err := setKeyring(KeyringService, KeyringUser, string(data)); err == nil {
		_ = deleteTokensFile()
		return nil
	}

	// Fallback to file storage with restrictive permissions
	if err := saveTokensToFile(data); err != nil {
		return err
	}
	// The keychain may still hold older credentials. LoadTokens prefers the
	// newer copy even if this delete fails.
	_ = keyring.Delete(KeyringService, KeyringUser)
	return nil
}

// LoadTokens retrieves authentication tokens from the keychain or the
// credentials file. When both hold credentials, the newer copy wins.
func LoadTokens() (*TokenStorage, error) {
	fromKeyring, keyringErr := loadTokensFromKeyring()
	fromFile, fileErr := loadTokensFromFile()
	switch {
	case keyringErr == nil && fileErr == nil:
		if newer(fromFile, fromKeyring) {
			return fromFile, nil
		}
		return fromKeyring, nil
	case keyringErr == nil:
		return fromKeyring, nil
	case fileErr == nil:
		return fromFile, nil
	default:
		return nil, fileErr
	}
}

func loadTokensFromKeyring() (*TokenStorage, error) {
	data, err := keyring.Get(KeyringService, KeyringUser)
	if err != nil {
		return nil, err
	}
	var tokens TokenStorage
	if err := json.Unmarshal([]byte(data), &tokens); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tokens from keychain: %w", err)
	}
	return &tokens, nil
}

// newer reports whether a was saved after b. Credentials saved before SavedAt
// existed compare by expiry.
func newer(a, b *TokenStorage) bool {
	if !a.SavedAt.Equal(b.SavedAt) {
		return a.SavedAt.After(b.SavedAt)
	}
	return a.ExpiresAt.After(b.ExpiresAt)
}

// DeleteTokens removes stored authentication tokens
func DeleteTokens() error {
	// Try to delete from keychain
	err := keyring.Delete(KeyringService, KeyringUser)

	// Also try to delete file storage (ignore errors)
	_ = deleteTokensFile()

	// Return keychain error if it exists and it's not "not found"
	if err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("failed to delete tokens from keychain: %w", err)
	}

	return nil
}

// getConfigDir returns the CLI configuration directory
func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".config", "kernel")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}

	return configDir, nil
}

// saveTokensToFile saves tokens to a file with restrictive permissions as fallback
func saveTokensToFile(data []byte) error {
	configDir, err := getConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	tokenFile := filepath.Join(configDir, "credentials")

	// Write with restrictive permissions (only owner can read/write)
	if err := os.WriteFile(tokenFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write tokens to file: %w", err)
	}

	return nil
}

// loadTokensFromFile loads tokens from file as fallback
func loadTokensFromFile() (*TokenStorage, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	tokenFile := filepath.Join(configDir, "credentials")

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no stored credentials found")
		}
		return nil, fmt.Errorf("failed to read tokens from file: %w", err)
	}

	var tokens TokenStorage
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tokens from file: %w", err)
	}

	return &tokens, nil
}

// deleteTokensFile removes the token file
func deleteTokensFile() error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}

	tokenFile := filepath.Join(configDir, "credentials")
	return os.Remove(tokenFile)
}
