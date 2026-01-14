package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kernel/cli/pkg/auth"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

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

	pterm.Debug.Printf("Starting local callback server on %s\n", oauthConfig.Config.RedirectURL)

	// Start OAuth flow
	spinner, _ := pterm.DefaultSpinner.Start("Waiting for authentication...")
	tokens, err := oauthConfig.StartOAuthFlow(ctx)
	if err != nil {
		spinner.Fail("Authentication failed")

		// Handle common error cases with helpful messages
		if ctx.Err() == context.Canceled {
			pterm.Info.Println("Authentication cancelled by user")
			return nil
		}

		return fmt.Errorf("authentication failed: %w", err)
	}

	spinner.Success("Authentication successful!")

	// Save tokens securely
	if err := auth.SaveTokens(tokens); err != nil {
		pterm.Warning.Printf("Authentication succeeded but failed to save credentials: %v\n", err)
		pterm.Warning.Println("You may need to re-authenticate on your next CLI usage")
		return nil
	}

	pterm.Success.Println("✓ Successfully authenticated with Kernel!")
	pterm.Info.Println("You can now use other Kernel CLI commands without setting KERNEL_API_KEY")

	return nil
}
