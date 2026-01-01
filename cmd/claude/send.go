package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/onkernel/cli/internal/claude"
	"github.com/onkernel/cli/pkg/util"
	"github.com/onkernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <browser-id> [message]",
	Short: "Send a message to Claude",
	Long: `Send a single message to Claude and get the response.

The message can be provided as:
- A command line argument
- From stdin (piped input)
- From a file (using --file)

This command is designed for scripting and automation. For interactive
conversations, use 'kernel claude chat' instead.`,
	Example: `  # Send a message as argument
  kernel claude send abc123 "What is 2+2?"

  # Pipe a message from stdin
  echo "Explain this error" | kernel claude send abc123

  # Read message from a file
  kernel claude send abc123 -f prompt.txt

  # Output as JSON for scripting
  kernel claude send abc123 "Hello" --json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSend,
}

func init() {
	sendCmd.Flags().StringP("file", "f", "", "Read message from file")
	sendCmd.Flags().Int("timeout", 120, "Response timeout in seconds")
	sendCmd.Flags().Bool("json", false, "Output response as JSON")
	sendCmd.Flags().Bool("raw", false, "Output raw response without formatting")
}

// SendResponse represents the JSON output of the send command.
type SendResponse struct {
	Response string `json:"response"`
	Warning  string `json:"warning,omitempty"`
}

func runSend(cmd *cobra.Command, args []string) error {
	browserID := args[0]
	filePath, _ := cmd.Flags().GetString("file")
	timeout, _ := cmd.Flags().GetInt("timeout")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	rawOutput, _ := cmd.Flags().GetBool("raw")

	ctx := cmd.Context()
	client := util.GetKernelClient(cmd)

	// Get the message from various sources
	var message string
	var err error

	if len(args) > 1 {
		// Message from command line arguments
		message = strings.Join(args[1:], " ")
	} else if filePath != "" {
		// Message from file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		message = strings.TrimSpace(string(content))
	} else {
		// Check for stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Stdin has data
			reader := bufio.NewReader(os.Stdin)
			content, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			message = strings.TrimSpace(string(content))
		}
	}

	if message == "" {
		return fmt.Errorf("no message provided. Provide a message as an argument, via stdin, or with --file")
	}

	// Verify the browser exists
	if !jsonOutput && !rawOutput {
		pterm.Info.Printf("Sending message to browser: %s\n", browserID)
	}

	browser, err := client.Browsers.Get(ctx, browserID)
	if err != nil {
		return fmt.Errorf("failed to get browser: %w", err)
	}

	// Build the script with environment variables
	script := fmt.Sprintf(`
process.env.CLAUDE_MESSAGE = %s;
process.env.CLAUDE_TIMEOUT_MS = '%d';

%s
`, jsonMarshalString(message), timeout*1000, claude.SendMessageScript)

	// Execute the send message script
	if !jsonOutput && !rawOutput {
		pterm.Info.Println("Sending message...")
	}

	result, err := client.Browsers.Playwright.Execute(ctx, browser.SessionID, kernel.BrowserPlaywrightExecuteParams{
		Code:       script,
		TimeoutSec: kernel.Opt(int64(timeout + 30)), // Add buffer for script setup
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	if !result.Success {
		if result.Error != "" {
			return fmt.Errorf("send failed: %s", result.Error)
		}
		return fmt.Errorf("send failed")
	}

	// Parse the result
	var response SendResponse
	if result.Result != nil {
		resultBytes, err := json.Marshal(result.Result)
		if err != nil {
			return fmt.Errorf("failed to parse result: %w", err)
		}
		if err := json.Unmarshal(resultBytes, &response); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	// Output the response
	if jsonOutput {
		output, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	if rawOutput {
		fmt.Print(response.Response)
		return nil
	}

	// Formatted output
	pterm.Println()
	if response.Warning != "" {
		pterm.Warning.Println(response.Warning)
	}
	pterm.Success.Println("Response:")
	pterm.Println()
	fmt.Println(response.Response)

	return nil
}

// jsonMarshalString returns a JSON-encoded string suitable for embedding in JavaScript.
func jsonMarshalString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		// Fallback to simple escaping
		return fmt.Sprintf(`"%s"`, strings.ReplaceAll(s, `"`, `\"`))
	}
	return string(b)
}
