package auth

import (
	"context"
	"fmt"
	"os"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
)

// GetAuthenticatedClient returns a Kernel client with appropriate authentication
func GetAuthenticatedClient(opts ...option.RequestOption) (*kernel.Client, error) {
	// Try to use API key first if available
	apiKey := os.Getenv("KERNEL_API_KEY")
	if apiKey != "" {
		pterm.Debug.Println("Using API key authentication")

		authOpts := append(opts, option.WithHeader("Authorization", "Bearer "+apiKey))
		client := kernel.NewClient(authOpts...)
		return &client, nil
	}

	// Fallback to OAuth tokens if no API key is available
	tokens, err := LoadTokens()
	if err == nil {
		// Check if access token is expired and refresh if needed
		if tokens.IsExpired() && tokens.RefreshToken != "" {
			refreshed, refreshErr := refreshStoredTokens(context.Background())
			if refreshErr != nil {
				pterm.Warning.Printf("Failed to refresh tokens: %v\n", refreshErr)
				pterm.Info.Println("Please run 'kernel login' to re-authenticate")
				return nil, fmt.Errorf("expired credentials, please re-authenticate: %w", refreshErr)
			}
			tokens = refreshed
		}

		// Use JWT token for authentication via Authorization header
		authOpts := append(opts, option.WithHeader("Authorization", "Bearer "+tokens.AccessToken))
		client := kernel.NewClient(authOpts...)
		return &client, nil
	}

	// No authentication available
	return nil, fmt.Errorf("no authentication available. Please run 'kernel login' or set KERNEL_API_KEY environment variable")
}

// refreshStoredTokens refreshes the stored credentials while holding the
// credentials lock. It re-reads them first, so when another CLI process has
// already refreshed, its tokens are used and the consumed refresh token is
// never presented again.
func refreshStoredTokens(ctx context.Context) (*TokenStorage, error) {
	var tokens *TokenStorage
	err := withCredentialsLock(func() error {
		current, err := loadTokens()
		if err != nil {
			return err
		}
		if !current.IsExpired() || current.RefreshToken == "" {
			tokens = current
			return nil
		}
		pterm.Debug.Println("Access token expired, attempting refresh...")
		refreshed, err := RefreshTokens(ctx, current)
		if err != nil {
			return err
		}
		if err := saveTokens(refreshed); err != nil {
			pterm.Warning.Printf("Failed to save refreshed tokens: %v\n", err)
		}
		pterm.Debug.Println("Successfully refreshed access token")
		tokens = refreshed
		return nil
	})
	return tokens, err
}
