package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/util"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var errLoginCanceled = errors.New("Kernel login canceled")

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Kernel using OAuth",
	Long: `Authenticate with Kernel using your browser. This will open your default browser 
to complete the OAuth authentication flow and securely store your credentials.`,
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().Bool("force", false, "Force re-authentication even if already logged in")
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	err := runLoginFlow(cmd, force, false)
	if errors.Is(err, errLoginCanceled) {
		return nil
	}
	return err
}

func runLoginWithForce(cmd *cobra.Command, force bool) error {
	return runLoginFlow(cmd, force, true)
}

func runLoginFlow(cmd *cobra.Command, force, requireSavedTokens bool) error {
	// Check if already logged in (unless force flag is used)
	if !force {
		if tokens, err := auth.LoadTokens(); err == nil && !tokens.IsExpired() {
			pterm.Info.Println("Already authenticated with Kernel")
			pterm.Info.Println("Use --force to re-authenticate")
			return nil
		}
	}

	pterm.Info.Println("Starting Kernel authentication...")
	pterm.Info.Println("This will open your browser to complete the OAuth flow")

	// Create cancellable context for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create OAuth configuration
	oauthConfig, err := auth.NewOAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to create OAuth configuration: %w", err)
	}
	pterm.Info.Printf("API URL: %s\n", util.GetBaseURL())
	pterm.Info.Printf("Auth URL: %s\n", oauthConfig.AuthBaseURL)

	pterm.Debug.Printf("Starting local callback server on %s\n", oauthConfig.Config.RedirectURL)

	// Start OAuth flow
	spinner, _ := pterm.DefaultSpinner.Start("Waiting for authentication...")
	tokens, err := oauthConfig.StartOAuthFlow(ctx)
	if err != nil {
		spinner.Fail("Authentication failed")

		// Handle common error cases with helpful messages
		if ctx.Err() == context.Canceled {
			pterm.Info.Println("Authentication cancelled by user")
			return errLoginCanceled
		}

		return fmt.Errorf("authentication failed: %w", err)
	}

	spinner.Success("Authentication successful!")

	// Save tokens securely
	if err := saveLoginTokens(tokens, requireSavedTokens, auth.SaveTokens); err != nil {
		return err
	}

	pterm.Success.Println("✓ Successfully authenticated with Kernel!")
	pterm.Info.Println("You can now use other Kernel CLI commands without setting KERNEL_API_KEY")

	return nil
}

func saveLoginTokens(tokens *auth.TokenStorage, required bool, save func(*auth.TokenStorage) error) error {
	if err := save(tokens); err != nil {
		pterm.Warning.Printf("Authentication succeeded but failed to save credentials: %v\n", err)
		if required {
			return fmt.Errorf("save authenticated session: %w", err)
		}
		pterm.Warning.Println("You may need to re-authenticate on your next CLI usage")
	}
	return nil
}
