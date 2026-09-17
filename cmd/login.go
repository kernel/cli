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
	pterm.Info.Printf("API URL: %s\n", util.GetBaseURL())
	pterm.Info.Printf("Auth URL: %s\n", oauthConfig.AuthBaseURL)

	pterm.Debug.Printf("Starting local callback server on %s\n", oauthConfig.Config.RedirectURL)

	spinner, _ := pterm.DefaultSpinner.WithRemoveWhenDone().Start("Waiting for authentication...")
	return completeLogin(ctx, spinner, oauthConfig.StartOAuthFlow, auth.SaveTokens)
}

type spinnerStopper interface {
	Stop() error
}

func completeLogin(
	ctx context.Context,
	spinner spinnerStopper,
	authenticate func(context.Context) (*auth.TokenStorage, error),
	saveTokens func(*auth.TokenStorage) error,
) error {
	tokens, err := authenticate(ctx)
	_ = spinner.Stop()
	if err != nil {
		if errors.Is(err, auth.ErrAuthorizationDenied) {
			return err
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return errors.New("authentication cancelled by user")
		}
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := saveTokens(tokens); err != nil {
		return fmt.Errorf("OAuth authorization completed, but credentials could not be saved: %w; fix credential storage and run 'kernel login' again", err)
	}

	pterm.Success.Println("Successfully authenticated with Kernel!")
	pterm.Info.Println("You can now use other Kernel CLI commands without setting KERNEL_API_KEY")
	return nil
}
