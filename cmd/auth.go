package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kernel/cli/pkg/auth"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	Sub     string `json:"sub"`      // User ID
	Email   string `json:"email"`    // User email
	Exp     int64  `json:"exp"`      // Expiration time
	Iss     string `json:"iss"`      // Issuer
	OrgID   string `json:"org_id"`   // Organization ID
	OrgName string `json:"org_name"` // Organization name
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Show current authentication status",
	Long: `Display information about the current authentication state, including logged-in user details and token expiry.
Use --log-level debug to show additional details like user ID and storage method.`,
	RunE: runAuth,
}

func init() {
	rootCmd.AddCommand(authCmd)
}

// parseJWT parses a JWT token and returns the claims
func parseJWT(tokenString string) (*JWTClaims, error) {
	// Parse the token without verification since we don't have the signing key
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}

	// Extract claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		jwtClaims := &JWTClaims{}

		if sub, ok := claims["sub"].(string); ok {
			jwtClaims.Sub = sub
		}

		if email, ok := claims["email"].(string); ok {
			jwtClaims.Email = email
		}

		if exp, ok := claims["exp"].(float64); ok {
			jwtClaims.Exp = int64(exp)
		}

		if iss, ok := claims["iss"].(string); ok {
			jwtClaims.Iss = iss
		}

		if orgID, ok := claims["org_id"].(string); ok {
			jwtClaims.OrgID = orgID
		}

		if orgName, ok := claims["org_name"].(string); ok {
			jwtClaims.OrgName = orgName
		}

		return jwtClaims, nil
	}

	return nil, nil
}

func runAuth(cmd *cobra.Command, args []string) error {
	// Check for stored OAuth tokens
	tokens, err := auth.LoadTokens()
	if err != nil {
		// Check if API key is being used as fallback
		if apiKey := os.Getenv("KERNEL_API_KEY"); apiKey != "" {
			pterm.Info.Println("Authentication method: API Key")
			if len(apiKey) >= 12 {
				pterm.Info.Printf("API Key: %s...%s\n", apiKey[:8], apiKey[len(apiKey)-4:])
			} else {
				pterm.Info.Printf("API Key: %s\n", strings.Repeat("*", len(apiKey)))
			}
			return nil
		}

		pterm.Info.Println("No active session found - not authenticated")
		pterm.Info.Println("Run 'kernel login' to authenticate with OAuth")
		pterm.Info.Println("Or set KERNEL_API_KEY environment variable")
		return nil
	}

	// Display OAuth authentication status
	pterm.Success.Println("✓ Authenticated with OAuth")

	// Extract info from JWT token
	if claims, err := parseJWT(tokens.AccessToken); err == nil && claims != nil {
		if claims.Sub != "" {
			logger.Debug("User details", logger.Args("email", claims.Email, "user_id", claims.Sub, "org_id", tokens.OrgID))
		}
	}

	// Token expiry status
	if tokens.IsExpired() {
		if tokens.RefreshToken != "" {
			pterm.Warning.Println("⚠️ Access token expired (will be refreshed automatically)")
		} else {
			pterm.Error.Println("❌ Access token expired and no refresh token available")
			pterm.Info.Println("Run 'kernel login --force' to re-authenticate")
		}
	} else {
		timeUntilExpiry := time.Until(tokens.ExpiresAt)
		logger.Debug("Time until expiry", logger.Args("time_until_expiry", timeUntilExpiry))
		logger.Debug("Expires at", logger.Args("expires_at", tokens.ExpiresAt))
		if timeUntilExpiry < 24*time.Hour {
			pterm.Warning.Printf("⚠️ Access token expires in %s\n", timeUntilExpiry.Round(time.Second))
		} else {
			pterm.Success.Printf("✓ Access token valid for %s\n", timeUntilExpiry.Round(time.Second))
		}
	}

	// Storage method
	if _, err := keyring.Get(auth.KeyringService, auth.KeyringUser); err == nil {
		logger.Debug("Storage method", logger.Args("method", "OS Keychain"))
	} else {
		logger.Debug("Storage method", logger.Args("method", "Local file (~/.config/kernel/credentials)"))
	}

	return nil
}
