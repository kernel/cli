package claude

import (
	"fmt"

	"github.com/onkernel/cli/internal/claude"
	"github.com/onkernel/cli/pkg/util"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var loadCmd = &cobra.Command{
	Use:   "load <browser-id>",
	Short: "Load Claude extension into existing browser",
	Long: `Load the Claude for Chrome extension into an existing Kernel browser session.

This command:
1. Uploads the Claude extension and authentication data
2. Loads the extension (browser will restart)

Use this if you already have a browser session running and want to add the
Claude extension to it.

Note: Loading an extension will restart the browser, which may interrupt any
ongoing operations.`,
	Example: `  # Load into existing browser
  kernel claude load abc123xyz -b claude-bundle.zip`,
	Args: cobra.ExactArgs(1),
	RunE: runLoad,
}

func init() {
	loadCmd.Flags().StringP("bundle", "b", "", "Path to the Claude bundle zip file (required)")

	_ = loadCmd.MarkFlagRequired("bundle")
}

func runLoad(cmd *cobra.Command, args []string) error {
	browserID := args[0]
	bundlePath, _ := cmd.Flags().GetString("bundle")

	ctx := cmd.Context()
	client := util.GetKernelClient(cmd)

	// Verify the browser exists
	pterm.Info.Printf("Verifying browser: %s\n", browserID)
	browser, err := client.Browsers.Get(ctx, browserID)
	if err != nil {
		return fmt.Errorf("failed to get browser: %w", err)
	}

	pterm.Info.Printf("Browser found: %s\n", browser.SessionID)

	// Extract the bundle
	pterm.Info.Printf("Extracting bundle: %s\n", bundlePath)
	bundle, err := claude.ExtractBundle(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to extract bundle: %w", err)
	}
	defer bundle.Cleanup()

	if bundle.HasAuthStorage() {
		pterm.Info.Println("Bundle includes authentication data")
	} else {
		pterm.Warning.Println("Bundle does not include authentication - login will be required")
	}

	// Load the Claude extension
	pterm.Info.Println("Loading Claude extension (browser will restart)...")
	if err := claude.LoadIntoBrowser(ctx, claude.LoadIntoBrowserOptions{
		BrowserID: browser.SessionID,
		Bundle:    bundle,
		Client:    client,
	}); err != nil {
		return fmt.Errorf("failed to load extension: %w", err)
	}

	pterm.Success.Println("Claude extension loaded successfully!")

	// Display results
	pterm.Println()
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Browser ID", browser.SessionID},
		{"Live View URL", browser.BrowserLiveViewURL},
	}
	if bundle.HasAuthStorage() {
		tableData = append(tableData, []string{"Auth Status", "Pre-authenticated"})
	} else {
		tableData = append(tableData, []string{"Auth Status", "Login required"})
	}

	_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

	pterm.Println()
	pterm.Info.Println("Next steps:")
	pterm.Printf("  # Check extension status\n")
	pterm.Printf("  kernel claude status %s\n", browser.SessionID)
	pterm.Println()
	pterm.Printf("  # Send a message\n")
	pterm.Printf("  kernel claude send %s \"Hello Claude!\"\n", browser.SessionID)

	return nil
}
