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
	token, err := BearerToken(context.Background())
	if err != nil {
		return nil, err
	}
	authOpts := append(opts, option.WithHeader("Authorization", "Bearer "+token))
	client := kernel.NewClient(authOpts...)
	return &client, nil
}

// BearerToken returns the same refreshed API key or OAuth token used by the
// generated SDK. Commands that call an API endpoint not yet present in the
// released SDK use this narrow accessor instead of implementing auth twice.
func BearerToken(ctx context.Context) (string, error) {
	if apiKey := os.Getenv("KERNEL_API_KEY"); apiKey != "" {
		pterm.Debug.Println("Using API key authentication")
		return apiKey, nil
	}

	tokens, err := LoadTokens()
	if err != nil {
		return "", fmt.Errorf("no authentication available. Please run 'kernel login' or set KERNEL_API_KEY environment variable")
	}
	if !tokens.IsExpired() || tokens.RefreshToken == "" {
		return tokens.AccessToken, nil
	}

	pterm.Debug.Println("Access token expired, attempting refresh...")
	refreshedTokens, refreshErr := RefreshTokens(ctx, tokens)
	if refreshErr != nil {
		pterm.Warning.Printf("Failed to refresh tokens: %v\n", refreshErr)
		pterm.Info.Println("Please run 'kernel login' to re-authenticate")
		return "", fmt.Errorf("expired credentials, please re-authenticate: %w", refreshErr)
	}
	if saveErr := SaveTokens(refreshedTokens); saveErr != nil {
		pterm.Warning.Printf("Failed to save credentials: %v\n", saveErr)
	}
	pterm.Debug.Println("Successfully refreshed access token")
	return refreshedTokens.AccessToken, nil
}
