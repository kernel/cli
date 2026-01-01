// Package claude provides commands for interacting with the Claude for Chrome extension
// in Kernel browsers.
package claude

import (
	"github.com/spf13/cobra"
)

// ClaudeCmd is the parent command for Claude extension operations
var ClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Interact with Claude for Chrome extension in Kernel browsers",
	Long: `Commands for using the Claude for Chrome extension in Kernel browsers.

This command group provides a complete workflow for:
- Extracting the Claude extension from your local Chrome installation
- Launching Kernel browsers with the extension pre-loaded
- Sending messages and interacting with Claude programmatically

Example workflow:
  # Extract extension from local Chrome (run on your machine)
  kernel claude extract -o claude-bundle.zip

  # Transfer to server if needed
  scp claude-bundle.zip server:~/

  # Launch a browser with Claude
  kernel claude launch -b claude-bundle.zip

  # Send a message
  kernel claude send <browser-id> "Hello Claude!"

  # Start interactive chat
  kernel claude chat <browser-id>

For more info: https://docs.onkernel.com/claude`,
	Run: func(cmd *cobra.Command, args []string) {
		// Show help if called without subcommands
		_ = cmd.Help()
	},
}

func init() {
	// Register subcommands
	ClaudeCmd.AddCommand(extractCmd)
	ClaudeCmd.AddCommand(launchCmd)
	ClaudeCmd.AddCommand(loadCmd)
	ClaudeCmd.AddCommand(statusCmd)
	ClaudeCmd.AddCommand(sendCmd)
	ClaudeCmd.AddCommand(chatCmd)
}
