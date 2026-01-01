package claude

import (
	"context"
	"fmt"
	"time"

	"github.com/onkernel/cli/internal/claude"
	"github.com/onkernel/cli/pkg/util"
	"github.com/onkernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var launchCmd = &cobra.Command{
	Use:   "launch",
	Short: "Create a browser with Claude extension loaded",
	Long: `Create a new Kernel browser session with the Claude for Chrome extension
pre-loaded and authenticated.

This command:
1. Creates a new browser session
2. Uploads the Claude extension and authentication data
3. Loads the extension (browser will restart)
4. Returns the browser ID and live view URL

The browser will have Claude ready to use. You can then interact with it using
'kernel claude send' or 'kernel claude chat'.`,
	Example: `  # Launch with default settings
  kernel claude launch -b claude-bundle.zip

  # Launch with longer timeout
  kernel claude launch -b claude-bundle.zip -t 3600

  # Launch in stealth mode
  kernel claude launch -b claude-bundle.zip --stealth

  # Launch and immediately start chatting
  kernel claude launch -b claude-bundle.zip --chat`,
	RunE: runLaunch,
}

func init() {
	launchCmd.Flags().StringP("bundle", "b", "", "Path to the Claude bundle zip file (required)")
	launchCmd.Flags().IntP("timeout", "t", 600, "Session timeout in seconds")
	launchCmd.Flags().BoolP("stealth", "s", false, "Launch browser in stealth mode")
	launchCmd.Flags().BoolP("headless", "H", false, "Launch browser in headless mode")
	launchCmd.Flags().String("url", "https://claude.ai", "Initial URL to navigate to")
	launchCmd.Flags().Bool("chat", false, "Start interactive chat after launch")
	launchCmd.Flags().String("viewport", "", "Browser viewport size (e.g., 1920x1080@25)")

	_ = launchCmd.MarkFlagRequired("bundle")
}

func runLaunch(cmd *cobra.Command, args []string) error {
	bundlePath, _ := cmd.Flags().GetString("bundle")
	timeout, _ := cmd.Flags().GetInt("timeout")
	stealth, _ := cmd.Flags().GetBool("stealth")
	headless, _ := cmd.Flags().GetBool("headless")
	startURL, _ := cmd.Flags().GetString("url")
	startChat, _ := cmd.Flags().GetBool("chat")
	viewport, _ := cmd.Flags().GetString("viewport")

	ctx := cmd.Context()
	client := util.GetKernelClient(cmd)

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

	// Create the browser session
	pterm.Info.Println("Creating browser session...")
	browserParams := kernel.BrowserNewParams{
		TimeoutSeconds: kernel.Opt(int64(timeout)),
	}

	if stealth {
		browserParams.Stealth = kernel.Opt(true)
	}
	if headless {
		browserParams.Headless = kernel.Opt(true)
	}
	if viewport != "" {
		width, height, refreshRate, err := parseViewport(viewport)
		if err != nil {
			return fmt.Errorf("invalid viewport: %w", err)
		}
		browserParams.Viewport = kernel.BrowserViewportParam{
			Width:  width,
			Height: height,
		}
		if refreshRate > 0 {
			browserParams.Viewport.RefreshRate = kernel.Opt(refreshRate)
		}
	}

	browser, err := client.Browsers.New(ctx, browserParams)
	if err != nil {
		return fmt.Errorf("failed to create browser: %w", err)
	}

	pterm.Info.Printf("Created browser: %s\n", browser.SessionID)

	// Wait for browser to be ready (eventual consistency)
	if err := waitForBrowserReady(ctx, client, browser.SessionID); err != nil {
		_ = client.Browsers.DeleteByID(context.Background(), browser.SessionID)
		return fmt.Errorf("browser not ready: %w", err)
	}

	// Load the Claude extension
	pterm.Info.Println("Loading Claude extension...")
	if err := claude.LoadIntoBrowser(ctx, claude.LoadIntoBrowserOptions{
		BrowserID: browser.SessionID,
		Bundle:    bundle,
		Client:    client,
	}); err != nil {
		// Try to clean up the browser on failure
		_ = client.Browsers.DeleteByID(context.Background(), browser.SessionID)
		return fmt.Errorf("failed to load extension: %w", err)
	}

	pterm.Success.Println("Claude extension loaded successfully!")

	// Navigate to initial URL if specified
	if startURL != "" {
		pterm.Info.Printf("Navigating to: %s\n", startURL)
		_, err := client.Browsers.Playwright.Execute(ctx, browser.SessionID, kernel.BrowserPlaywrightExecuteParams{
			Code: fmt.Sprintf(`await page.goto('%s');`, startURL),
		})
		if err != nil {
			pterm.Warning.Printf("Failed to navigate to URL: %v\n", err)
		}
	}

	// Display results
	pterm.Println()
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Browser ID", browser.SessionID},
		{"Live View URL", browser.BrowserLiveViewURL},
		{"CDP WebSocket URL", truncateURL(browser.CdpWsURL, 60)},
		{"Timeout (seconds)", fmt.Sprintf("%d", timeout)},
	}
	if bundle.HasAuthStorage() {
		tableData = append(tableData, []string{"Auth Status", "Pre-authenticated"})
	} else {
		tableData = append(tableData, []string{"Auth Status", "Login required"})
	}

	_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

	pterm.Println()
	pterm.Info.Println("Next steps:")
	pterm.Printf("  # Send a message\n")
	pterm.Printf("  kernel claude send %s \"Hello Claude!\"\n", browser.SessionID)
	pterm.Println()
	pterm.Printf("  # Start interactive chat\n")
	pterm.Printf("  kernel claude chat %s\n", browser.SessionID)
	pterm.Println()
	pterm.Printf("  # Check extension status\n")
	pterm.Printf("  kernel claude status %s\n", browser.SessionID)

	// Start interactive chat if requested
	if startChat {
		pterm.Println()
		return runChatWithBrowser(ctx, client, browser.SessionID)
	}

	return nil
}

// parseViewport parses a viewport string like "1920x1080@25" into width, height, and refresh rate.
func parseViewport(viewport string) (int64, int64, int64, error) {
	var width, height, refreshRate int64

	// Try parsing with refresh rate
	n, err := fmt.Sscanf(viewport, "%dx%d@%d", &width, &height, &refreshRate)
	if err == nil && n == 3 {
		return width, height, refreshRate, nil
	}

	// Try parsing without refresh rate
	n, err = fmt.Sscanf(viewport, "%dx%d", &width, &height)
	if err == nil && n == 2 {
		return width, height, 0, nil
	}

	return 0, 0, 0, fmt.Errorf("invalid format, expected WIDTHxHEIGHT[@RATE]")
}

// waitForBrowserReady polls until the browser is accessible via GET.
// This handles eventual consistency after browser creation.
func waitForBrowserReady(ctx context.Context, client kernel.Client, browserID string) error {
	const maxAttempts = 10
	const delay = 500 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := client.Browsers.Get(ctx, browserID)
		if err == nil {
			return nil
		}

		if attempt < maxAttempts {
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("browser %s not accessible after %d attempts", browserID, maxAttempts)
}

// truncateURL truncates a URL to a maximum length, adding "..." if truncated.
func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	return url[:maxLen-3] + "..."
}
