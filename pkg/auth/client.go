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
			pterm.Debug.Println("Access token expired, attempting refresh...")

			refreshedTokens, refreshErr := RefreshTokens(context.Background(), tokens)
			if refreshErr != nil {
				pterm.Warning.Printf("Failed to refresh tokens: %v\n", refreshErr)
				pterm.Info.Println("Please run 'kernel login' to re-authenticate")
				return nil, fmt.Errorf("expired credentials, please re-authenticate: %w", refreshErr)
			}

			// Save refreshed tokens
			if saveErr := SaveTokens(refreshedTokens); saveErr != nil {
				pterm.Warning.Printf("Failed to save refreshed tokens: %v\n", saveErr)
			}

			tokens = refreshedTokens
			pterm.Debug.Println("Successfully refreshed access token")
		}

		// Use JWT token for authentication via Authorization header
		authOpts := append(opts, option.WithHeader("Authorization", "Bearer "+tokens.AccessToken))
		client := kernel.NewClient(authOpts...)
		return &client, nil
	}

	// No authentication available
	return nil, fmt.Errorf("no authentication available. Please run 'kernel login' or set KERNEL_API_KEY environment variable")
}
