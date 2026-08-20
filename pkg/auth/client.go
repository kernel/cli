package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
)

// ErrAuthenticationRequired reports credentials that can be repaired by an
// interactive OAuth login.
var ErrAuthenticationRequired = errors.New("authentication required")

// GetAuthenticatedClient returns a Kernel client with appropriate authentication
func GetAuthenticatedClient(opts ...option.RequestOption) (*kernel.Client, error) {
	token, err := BearerToken(context.Background())
	if err != nil {
		return nil, err
	}
	if baseURL := strings.TrimSpace(os.Getenv("KERNEL_BASE_URL")); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
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
		if !errors.Is(err, ErrNoStoredCredentials) {
			return "", err
		}
		return "", fmt.Errorf("%w: run 'kernel login' or set KERNEL_API_KEY", ErrAuthenticationRequired)
	}
	if !tokens.IsExpired() || tokens.RefreshToken == "" {
		return tokens.AccessToken, nil
	}

	pterm.Debug.Println("Access token expired, attempting refresh...")
	refreshedTokens, refreshErr := RefreshTokens(ctx, tokens)
	if refreshErr != nil {
		pterm.Warning.Printf("Failed to refresh tokens: %v\n", refreshErr)
		if refreshRequiresLogin(refreshErr) {
			pterm.Info.Println("Please run 'kernel login' to re-authenticate")
			return "", fmt.Errorf("%w: expired credentials: %v", ErrAuthenticationRequired, refreshErr)
		}
		return "", fmt.Errorf("refresh credentials: %w", refreshErr)
	}
	if saveErr := SaveTokens(refreshedTokens); saveErr != nil {
		pterm.Warning.Printf("Failed to save credentials: %v\n", saveErr)
	}
	pterm.Debug.Println("Successfully refreshed access token")
	return refreshedTokens.AccessToken, nil
}

func refreshRequiresLogin(err error) bool {
	var rejected *TokenRefreshError
	return errors.As(err, &rejected) && (rejected.StatusCode == 400 || rejected.StatusCode == 401)
}
