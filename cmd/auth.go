package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/util"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	Sub     string `json:"sub"`      // User ID
	Exp     int64  `json:"exp"`      // Expiration time
	Iss     string `json:"iss"`      // Issuer
	OrgID   string `json:"org_id"`   // Organization ID
	OrgName string `json:"org_name"` // Organization name
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Show authentication status and manage auth connections",
	Long: `Display the credential commands authenticate with (API key, workload identity federation, or saved login) and its state, including logged-in user details and token expiry.
Use --log-level debug to show additional details like user ID and storage method.

Use "kernel auth connections" to manage managed auth connections for automated login flows.`,
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
	// The sources are reported in the order commands try them.
	if apiKey := os.Getenv("KERNEL_API_KEY"); apiKey != "" {
		pterm.Info.Println("Authentication method: API Key")
		pterm.Info.Printf("API URL: %s\n", util.GetBaseURL())
		if len(apiKey) >= 12 {
			pterm.Info.Printf("API Key: %s...%s\n", apiKey[:8], apiKey[len(apiKey)-4:])
		} else {
			pterm.Info.Printf("API Key: %s\n", strings.Repeat("*", len(apiKey)))
		}
		return nil
	}

	if federation := auth.DescribeFederation(); federation != nil {
		printFederationStatus(federation)
		return nil
	}

	tokens, err := auth.LoadTokens()
	if err != nil {
		pterm.Info.Println("No active session found - not authenticated")
		pterm.Info.Printf("API URL: %s\n", util.GetBaseURL())
		pterm.Info.Printf("Auth URL: %s\n", auth.CurrentAuthBaseURL())
		pterm.Info.Println("Run 'kernel login' to authenticate with OAuth")
		pterm.Info.Println("Or set KERNEL_API_KEY environment variable")
		return nil
	}

	// Display OAuth authentication status
	pterm.Success.Println("✓ Authenticated with OAuth")
	pterm.Info.Printf("API URL: %s\n", util.GetBaseURL())
	pterm.Info.Printf("Auth URL: %s\n", tokenAuthBaseURLForDisplay(tokens))
	pterm.Info.Printf("OAuth client ID: %s\n", maskClientID(tokenOAuthClientIDForDisplay(tokens)))
	pterm.Info.Printf("Access scope: %s\n", accessScopeLabel(tokens))

	// Extract info from JWT token
	if claims, err := parseJWT(tokens.AccessToken); err == nil && claims != nil {
		if claims.Sub != "" {
			logger.Debug("User details", logger.Args("user_id", claims.Sub, "org_id", tokens.OrgID))
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

	logger.Debug("Storage method", logger.Args("method", tokens.Source))

	return nil
}

func printFederationStatus(federation *auth.FederationStatus) {
	pterm.Info.Println("Authentication method: Workload identity federation")
	pterm.Info.Printf("API URL: %s\n", util.GetBaseURL())
	pterm.Info.Printf("Auth URL: %s\n", auth.CurrentAuthBaseURL())
	pterm.Info.Printf("Federation rule: %s\n", federation.RuleID)
	switch {
	case federation.Err != nil:
		pterm.Error.Printf("%v\n", federation.Err)
	case federation.NotCached != "":
		pterm.Info.Printf("Cached token: not in use (%s); every command exchanges a new one\n", federation.NotCached)
	case federation.CachedTokenExpiresAt.IsZero():
		pterm.Info.Println("Cached token: not in use; the next command exchanges a new one")
	default:
		pterm.Info.Printf("Cached token: in use, expires in %s\n", time.Until(federation.CachedTokenExpiresAt).Round(time.Second))
	}
}

func tokenAuthBaseURLForDisplay(tokens *auth.TokenStorage) string {
	if tokens.AuthBaseURL != "" {
		return tokens.AuthBaseURL
	}
	return auth.DefaultAuthBaseURL
}

func tokenOAuthClientIDForDisplay(tokens *auth.TokenStorage) string {
	if tokens.OAuthClientID != "" {
		return tokens.OAuthClientID
	}
	return auth.DefaultClientID
}

func maskClientID(clientID string) string {
	if len(clientID) <= 8 {
		return clientID
	}
	return clientID[:4] + "..." + clientID[len(clientID)-4:]
}

// accessScopeLabel describes where the stored credentials may act: one
// project, the projects a native grant is limited to, or the organization.
// Credentials saved by older CLI versions did not record the projects, so
// their scope is unknown until they are refreshed or replaced.
func accessScopeLabel(tokens *auth.TokenStorage) string {
	switch {
	case tokens.AccessScope == "project" && tokens.ProjectID != "":
		return "project " + tokens.ProjectID
	case len(tokens.Projects) == 1:
		return "project " + tokens.Projects[0]
	case len(tokens.Projects) > 1:
		return "projects " + strings.Join(tokens.Projects, ", ")
	case !tokens.ScopeRecorded:
		return "unknown (saved by an older CLI version; run 'kernel login --force' to record it)"
	default:
		return "organization-wide"
	}
}
