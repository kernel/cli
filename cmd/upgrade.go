package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/update"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// UpgradeInput holds the input parameters for the upgrade command.
type UpgradeInput struct {
	DryRun bool
}

// UpgradeCmd handles the upgrade command logic, separated from cobra.
type UpgradeCmd struct {
	currentVersion string
}

// Run executes the upgrade command logic.
func (u UpgradeCmd) Run(ctx context.Context, in UpgradeInput) error {
	// Fetch latest version from GitHub
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pterm.Info.Println("Checking for updates...")

	latestTag, releaseURL, err := update.FetchLatest(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	// Compare versions
	isNewer, err := update.IsNewerVersion(u.currentVersion, latestTag)
	if err != nil {
		// If version comparison fails (e.g., dev version), still allow upgrade
		pterm.Warning.Printf("Could not compare versions (%s vs %s): %v\n", u.currentVersion, latestTag, err)
		pterm.Info.Println("Proceeding with upgrade...")
	} else if !isNewer {
		pterm.Success.Printf("You are already on the latest version (%s)\n", strings.TrimPrefix(u.currentVersion, "v"))
		return nil
	} else {
		pterm.Info.Printf("New version available: %s → %s\n", strings.TrimPrefix(u.currentVersion, "v"), strings.TrimPrefix(latestTag, "v"))
		if releaseURL != "" {
			pterm.Info.Printf("Release notes: %s\n", releaseURL)
		}
	}

	// Detect installation method
	method, binaryPath := update.DetectInstallMethod()

	if method == update.InstallMethodUnknown {
		printManualUpgradeInstructions(latestTag, binaryPath)
		// Return nil since we've provided manual instructions - don't fail scripts
		return nil
	}

	if in.DryRun {
		pterm.Info.Printf("Detected installation method: %s\n", method)
		pterm.Info.Printf("Binary path: %s\n", binaryPath)
		pterm.Info.Printf("Would run: %s\n", getUpgradeCommand(method))
		return nil
	}

	pterm.Info.Printf("Upgrading via %s...\n", method)
	stderr, err := executeUpgrade(method)
	if err != nil {
		// If Homebrew upgrade fails, check if it's due to old tap installation
		if method == update.InstallMethodBrew && isOldTapError(stderr) {
			pterm.Println()
			pterm.Error.Println("Homebrew upgrade failed due to old tap installation.")
			pterm.Info.Println("Run these commands to fix:")
			pterm.Println()
			fmt.Println("  brew uninstall kernel")
			fmt.Println("  brew untap onkernel/tap")
			fmt.Println("  brew install kernel/tap/kernel")
			pterm.Println()
		}
		return err
	}
	return nil
}

// isOldTapError checks if the brew error output indicates the user has the old onkernel/tap
// installed and needs to migrate to kernel/tap.
func isOldTapError(stderr string) bool {
	// When a user has onkernel/tap/kernel installed and runs `brew upgrade kernel/tap/kernel`,
	// Homebrew will suggest: "Please tap it and then try again: brew tap kernel/tap"
	return strings.Contains(stderr, "brew tap kernel/tap")
}

// upgradeCommandArgs returns the command and arguments for a given installation method.
// Returns nil if the method is unknown.
func upgradeCommandArgs(method update.InstallMethod) []string {
	switch method {
	case update.InstallMethodBrew:
		return []string{"brew", "upgrade", "kernel/tap/kernel"}
	case update.InstallMethodPNPM:
		return []string{"pnpm", "add", "-g", "@onkernel/cli@latest"}
	case update.InstallMethodNPM:
		return []string{"npm", "i", "-g", "@onkernel/cli@latest"}
	case update.InstallMethodBun:
		return []string{"bun", "add", "-g", "@onkernel/cli@latest"}
	default:
		return nil
	}
}

// getUpgradeCommand returns the command string for display (e.g., dry-run output).
func getUpgradeCommand(method update.InstallMethod) string {
	args := upgradeCommandArgs(method)
	if args == nil {
		return ""
	}
	return strings.Join(args, " ")
}

// executeUpgrade runs the appropriate upgrade command based on the installation method.
// Returns the captured stderr (for error diagnosis) and any error.
func executeUpgrade(method update.InstallMethod) (stderr string, err error) {
	args := upgradeCommandArgs(method)
	if args == nil {
		return "", fmt.Errorf("unknown installation method")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	// Capture stderr while also displaying it to the user
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	err = cmd.Run()
	return stderrBuf.String(), err
}

// printManualUpgradeInstructions prints instructions for manually upgrading kernel
func printManualUpgradeInstructions(version, binaryPath string) {
	// Normalize version (remove 'v' prefix if present)
	version = strings.TrimPrefix(version, "v")

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	downloadURL := fmt.Sprintf(
		"https://github.com/kernel/cli/releases/download/v%s/kernel_%s_%s_%s.tar.gz",
		version, version, goos, goarch,
	)

	if binaryPath == "" {
		binaryPath = "/usr/local/bin/kernel"
	}

	pterm.Warning.Println("Could not detect installation method.")
	pterm.Info.Println("To upgrade manually, run:")
	pterm.Println()
	fmt.Printf("  wget %s -O /tmp/kernel.tar.gz\n", downloadURL)
	fmt.Printf("  tar -xzf /tmp/kernel.tar.gz -C /tmp\n")
	fmt.Printf("  sudo cp /tmp/kernel %q\n", binaryPath)
	pterm.Println()
}

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Aliases: []string{"update"},
	Short:   "Upgrade the Kernel CLI to the latest version",
	Long: `Upgrade the Kernel CLI to the latest version.

Supported installation methods:
  - Homebrew (brew)
  - pnpm
  - npm
  - bun

If your installation method cannot be detected, manual upgrade instructions will be provided.`,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().Bool("dry-run", false, "Show what would be executed without running")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	u := UpgradeCmd{
		currentVersion: metadata.Version,
	}
	return u.Run(cmd.Context(), UpgradeInput{
		DryRun: dryRun,
	})
}
