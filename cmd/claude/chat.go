package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/onkernel/cli/internal/claude"
	"github.com/onkernel/cli/pkg/util"
	"github.com/onkernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat <browser-id>",
	Short: "Interactive chat with Claude",
	Long: `Start an interactive chat session with Claude in a Kernel browser.

This provides a simple command-line interface for having a conversation
with Claude. Type your messages and receive responses directly in the terminal.

Special commands:
  /quit, /exit  - Exit the chat session
  /clear        - Clear the terminal
  /status       - Check extension status
  /help         - Show available commands`,
	Example: `  # Start chat with existing browser
  kernel claude chat abc123xyz

  # Launch new browser and start chatting
  kernel claude launch -b claude-bundle.zip --chat`,
	Args: cobra.ExactArgs(1),
	RunE: runChat,
}

func init() {
	chatCmd.Flags().Bool("no-tui", false, "Disable interactive mode (line-by-line I/O)")
}

func runChat(cmd *cobra.Command, args []string) error {
	browserID := args[0]
	// noTUI, _ := cmd.Flags().GetBool("no-tui")
	// For now, both modes use the same implementation

	ctx := cmd.Context()
	client := util.GetKernelClient(cmd)

	return runChatWithBrowser(ctx, client, browserID)
}

func runChatWithBrowser(ctx context.Context, client kernel.Client, browserID string) error {
	// Verify the browser exists
	pterm.Info.Printf("Connecting to browser: %s\n", browserID)

	browser, err := client.Browsers.Get(ctx, browserID)
	if err != nil {
		return fmt.Errorf("failed to get browser: %w", err)
	}

	// Open the side panel by clicking the extension icon
	if err := claude.OpenSidePanel(ctx, client, browser.SessionID); err != nil {
		return fmt.Errorf("failed to open side panel: %w", err)
	}

	// Check Claude status first
	pterm.Info.Println("Checking Claude extension status...")
	statusResult, err := client.Browsers.Playwright.Execute(ctx, browser.SessionID, kernel.BrowserPlaywrightExecuteParams{
		Code:       claude.CheckStatusScript,
		TimeoutSec: kernel.Opt(int64(30)),
	})
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	if statusResult.Result != nil {
		var status struct {
			ExtensionLoaded bool   `json:"extensionLoaded"`
			Authenticated   bool   `json:"authenticated"`
			Error           string `json:"error"`
		}
		resultBytes, _ := json.Marshal(statusResult.Result)
		_ = json.Unmarshal(resultBytes, &status)

		if !status.ExtensionLoaded {
			return fmt.Errorf("Claude extension is not loaded. Load it first with: kernel claude load %s -b claude-bundle.zip", browserID)
		}
		if !status.Authenticated {
			pterm.Warning.Println("Claude extension is not authenticated.")
			pterm.Info.Printf("Please log in via the live view: %s\n", browser.BrowserLiveViewURL)
			return fmt.Errorf("authentication required")
		}
	}

	// Display chat header
	pterm.Println()
	pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).
		WithTextStyle(pterm.NewStyle(pterm.FgWhite)).
		Println("Claude Chat")
	pterm.Println()
	pterm.Info.Printf("Browser: %s\n", browserID)
	pterm.Info.Printf("Live View: %s\n", browser.BrowserLiveViewURL)
	pterm.Println()
	pterm.Info.Println("Type your message and press Enter. Use /help for commands, /quit to exit.")
	pterm.Println()

	// Start the chat loop
	scanner := bufio.NewScanner(os.Stdin)
	messageCount := 0

	for {
		// Show prompt
		pterm.Print(pterm.Cyan("You: "))

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle special commands
		if strings.HasPrefix(input, "/") {
			handled, shouldExit := handleChatCommand(ctx, client, browserID, input)
			if shouldExit {
				pterm.Info.Println("Goodbye!")
				return nil
			}
			if handled {
				continue
			}
		}

		// Send the message
		messageCount++
		spinner, _ := pterm.DefaultSpinner.Start("Claude is thinking...")

		response, err := sendChatMessage(ctx, client, browser.SessionID, input)
		spinner.Stop()

		if err != nil {
			pterm.Error.Printf("Error: %v\n", err)
			pterm.Println()
			continue
		}

		// Display the response
		pterm.Println()
		pterm.Print(pterm.Green("Claude: "))
		fmt.Println(response)
		pterm.Println()
	}

	return nil
}

func handleChatCommand(ctx context.Context, client kernel.Client, browserID, input string) (handled bool, shouldExit bool) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, false
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/quit", "/exit", "/q":
		return true, true

	case "/clear":
		// Clear terminal (works on most terminals)
		fmt.Print("\033[H\033[2J")
		pterm.Info.Println("Terminal cleared.")
		return true, false

	case "/status":
		pterm.Info.Println("Checking status...")
		result, err := client.Browsers.Playwright.Execute(ctx, browserID, kernel.BrowserPlaywrightExecuteParams{
			Code:       claude.CheckStatusScript,
			TimeoutSec: kernel.Opt(int64(30)),
		})
		if err != nil {
			pterm.Error.Printf("Status check failed: %v\n", err)
			return true, false
		}

		var status struct {
			ExtensionLoaded bool   `json:"extensionLoaded"`
			Authenticated   bool   `json:"authenticated"`
			HasConversation bool   `json:"hasConversation"`
			Error           string `json:"error"`
		}
		if result.Result != nil {
			resultBytes, _ := json.Marshal(result.Result)
			_ = json.Unmarshal(resultBytes, &status)
		}

		pterm.Info.Printf("Extension: %v, Auth: %v, Conversation: %v\n",
			status.ExtensionLoaded, status.Authenticated, status.HasConversation)
		if status.Error != "" {
			pterm.Warning.Printf("Error: %s\n", status.Error)
		}
		return true, false

	case "/help", "/?":
		pterm.Println()
		pterm.Info.Println("Available commands:")
		pterm.Println("  /quit, /exit  - Exit the chat session")
		pterm.Println("  /clear        - Clear the terminal")
		pterm.Println("  /status       - Check extension status")
		pterm.Println("  /help         - Show this help message")
		pterm.Println()
		return true, false

	default:
		pterm.Warning.Printf("Unknown command: %s (use /help for available commands)\n", cmd)
		return true, false
	}
}

func sendChatMessage(ctx context.Context, client kernel.Client, browserID, message string) (string, error) {
	// Build the script with the message
	script := fmt.Sprintf(`
process.env.CLAUDE_MESSAGE = %s;
process.env.CLAUDE_TIMEOUT_MS = '300000';

%s
`, jsonMarshalString(message), claude.SendMessageScript)

	result, err := client.Browsers.Playwright.Execute(ctx, browserID, kernel.BrowserPlaywrightExecuteParams{
		Code:       script,
		TimeoutSec: kernel.Opt(int64(330)), // 5.5 minutes
	})
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	if !result.Success {
		if result.Error != "" {
			return "", fmt.Errorf("%s", result.Error)
		}
		return "", fmt.Errorf("send failed")
	}

	// Parse the result
	var response struct {
		Response string `json:"response"`
		Warning  string `json:"warning"`
	}
	if result.Result != nil {
		resultBytes, _ := json.Marshal(result.Result)
		_ = json.Unmarshal(resultBytes, &response)
	}

	if response.Response == "" {
		return "", fmt.Errorf("empty response")
	}

	return response.Response, nil
}
