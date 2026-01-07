package cmd

import (
	"fmt"

	"github.com/kernel/cli/pkg/auth"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear stored authentication credentials",
	Long: `Log out of Kernel by removing stored authentication tokens. 
After logout, you will need to run 'kernel login' again to authenticate.`,
	RunE: runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	// Check if user is currently logged in
	_, err := auth.LoadTokens()
	if err != nil {
		pterm.Info.Println("No active session found - already logged out")
		return nil
	}

	pterm.Info.Println("Logging out...")

	// Delete stored tokens
	if err := auth.DeleteTokens(); err != nil {
		return fmt.Errorf("failed to clear stored credentials: %w", err)
	}

	pterm.Success.Println("✓ Successfully logged out")
	pterm.Info.Println("Run 'kernel login' to authenticate again")

	return nil
}
