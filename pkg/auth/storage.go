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
	AuthBaseURL   string    `json:"auth_base_url,omitempty"`
	OAuthClientID string    `json:"oauth_client_id,omitempty"`
}

// IsExpired checks if the access token is expired (with 5 minute buffer)
func (t *TokenStorage) IsExpired() bool {
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// SaveTokens stores authentication tokens securely in the OS keychain
func SaveTokens(tokens *TokenStorage) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	// Try to store in OS keychain
	err = keyring.Set(KeyringService, KeyringUser, string(data))
	if err != nil {
		// Fallback to file storage with restrictive permissions
		return saveTokensToFile(data)
	}

	return nil
}

// LoadTokens retrieves authentication tokens from secure storage
func LoadTokens() (*TokenStorage, error) {
	// Try to load from OS keychain first
	data, err := keyring.Get(KeyringService, KeyringUser)
	if err != nil {
		// Fallback to file storage
		return loadTokensFromFile()
	}

	var tokens TokenStorage
	if err := json.Unmarshal([]byte(data), &tokens); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tokens from keychain: %w", err)
	}

	return &tokens, nil
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
