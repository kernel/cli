package claude

import (
	"encoding/json"
	"fmt"

	"github.com/onkernel/cli/internal/claude"
	"github.com/onkernel/cli/pkg/util"
	"github.com/onkernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status <browser-id>",
	Short: "Check Claude extension status",
	Long: `Check the status of the Claude for Chrome extension in a Kernel browser.

This command checks:
- Whether the extension is loaded
- Whether the user is authenticated
- Whether there are any errors
- Whether there's an active conversation`,
	Example: `  kernel claude status abc123xyz`,
	Args:    cobra.ExactArgs(1),
	RunE:    runStatus,
}

func init() {
	statusCmd.Flags().StringP("output", "o", "", "Output format: json for raw response")
}

// StatusResult represents the result of a status check.
type StatusResult struct {
	ExtensionLoaded bool   `json:"extensionLoaded"`
	Authenticated   bool   `json:"authenticated"`
	HasConversation bool   `json:"hasConversation"`
	Error           string `json:"error,omitempty"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	browserID := args[0]
	outputFormat, _ := cmd.Flags().GetString("output")

	ctx := cmd.Context()
	client := util.GetKernelClient(cmd)

	// Verify the browser exists
	if outputFormat != "json" {
		pterm.Info.Printf("Checking browser: %s\n", browserID)
	}

	browser, err := client.Browsers.Get(ctx, browserID)
	if err != nil {
		return fmt.Errorf("failed to get browser: %w", err)
	}

	// Execute the status check script
	if outputFormat != "json" {
		pterm.Info.Println("Checking Claude extension status...")
	}

	result, err := client.Browsers.Playwright.Execute(ctx, browser.SessionID, kernel.BrowserPlaywrightExecuteParams{
		Code:       claude.CheckStatusScript,
		TimeoutSec: kernel.Opt(int64(30)),
	})
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	if !result.Success {
		if result.Error != "" {
			return fmt.Errorf("status check failed: %s", result.Error)
		}
		return fmt.Errorf("status check failed")
	}

	// Parse the result
	var status StatusResult
	if result.Result != nil {
		resultBytes, err := json.Marshal(result.Result)
		if err != nil {
			return fmt.Errorf("failed to parse result: %w", err)
		}
		if err := json.Unmarshal(resultBytes, &status); err != nil {
			return fmt.Errorf("failed to parse status: %w", err)
		}
	}

	// Output results
	if outputFormat == "json" {
		output, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	// Table output
	pterm.Println()
	tableData := pterm.TableData{
		{"Property", "Status"},
	}

	// Extension status
	if status.ExtensionLoaded {
		tableData = append(tableData, []string{"Extension", pterm.Green("Loaded")})
	} else {
		tableData = append(tableData, []string{"Extension", pterm.Red("Not Loaded")})
	}

	// Auth status
	if status.Authenticated {
		tableData = append(tableData, []string{"Authentication", pterm.Green("Authenticated")})
	} else {
		tableData = append(tableData, []string{"Authentication", pterm.Yellow("Not Authenticated")})
	}

	// Conversation status
	if status.HasConversation {
		tableData = append(tableData, []string{"Conversation", "Active"})
	} else {
		tableData = append(tableData, []string{"Conversation", "None"})
	}

	// Error status
	if status.Error != "" {
		tableData = append(tableData, []string{"Error", pterm.Red(status.Error)})
	}

	_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

	// Provide next steps based on status
	pterm.Println()
	if !status.ExtensionLoaded {
		pterm.Warning.Println("Extension is not loaded. Try loading it with:")
		pterm.Printf("  kernel claude load %s -b claude-bundle.zip\n", browserID)
	} else if !status.Authenticated {
		pterm.Warning.Println("Extension is not authenticated. You need to log in manually via the live view:")
		pterm.Printf("  Open: %s\n", browser.BrowserLiveViewURL)
	} else {
		pterm.Success.Println("Claude is ready!")
		pterm.Println()
		pterm.Info.Println("You can:")
		pterm.Printf("  # Send a message\n")
		pterm.Printf("  kernel claude send %s \"Hello Claude!\"\n", browserID)
		pterm.Println()
		pterm.Printf("  # Start interactive chat\n")
		pterm.Printf("  kernel claude chat %s\n", browserID)
	}

	return nil
}
