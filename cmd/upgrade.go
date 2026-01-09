package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/update"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var dryRun bool

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
	upgradeCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be executed without running")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	currentVersion := metadata.Version

	// Fetch latest version from GitHub
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	pterm.Info.Println("Checking for updates...")

	latestTag, releaseURL, err := update.FetchLatest(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	// Compare versions
	isNewer, err := update.IsNewerVersion(currentVersion, latestTag)
	if err != nil {
		// If version comparison fails (e.g., dev version), still allow upgrade
		pterm.Warning.Printf("Could not compare versions (%s vs %s): %v\n", currentVersion, latestTag, err)
		pterm.Info.Println("Proceeding with upgrade...")
	} else if !isNewer {
		pterm.Success.Printf("You are already on the latest version (%s)\n", strings.TrimPrefix(currentVersion, "v"))
		return nil
	} else {
		pterm.Info.Printf("New version available: %s → %s\n", strings.TrimPrefix(currentVersion, "v"), strings.TrimPrefix(latestTag, "v"))
		if releaseURL != "" {
			pterm.Info.Printf("Release notes: %s\n", releaseURL)
		}
	}

	// Detect installation method
	method, binaryPath := update.DetectInstallMethod()

	if method == update.InstallMethodUnknown {
		printManualUpgradeInstructions(latestTag, binaryPath)
		return fmt.Errorf("could not detect installation method")
	}

	if dryRun {
		pterm.Info.Printf("Would run: %s\n", getUpgradeCommand(method))
		return nil
	}

	pterm.Info.Printf("Upgrading via %s...\n", method)
	return executeUpgrade(method)
}

// getUpgradeCommand returns the command string for a given installation method
func getUpgradeCommand(method update.InstallMethod) string {
	switch method {
	case update.InstallMethodBrew:
		return "brew upgrade kernel/tap/kernel"
	case update.InstallMethodPNPM:
		return "pnpm add -g @onkernel/cli@latest"
	case update.InstallMethodNPM:
		return "npm i -g @onkernel/cli@latest"
	case update.InstallMethodBun:
		return "bun add -g @onkernel/cli@latest"
	default:
		return ""
	}
}

// executeUpgrade runs the appropriate upgrade command based on the installation method
func executeUpgrade(method update.InstallMethod) error {
	var cmd *exec.Cmd

	switch method {
	case update.InstallMethodBrew:
		cmd = exec.Command("brew", "upgrade", "kernel/tap/kernel")
	case update.InstallMethodPNPM:
		cmd = exec.Command("pnpm", "add", "-g", "@onkernel/cli@latest")
	case update.InstallMethodNPM:
		cmd = exec.Command("npm", "i", "-g", "@onkernel/cli@latest")
	case update.InstallMethodBun:
		cmd = exec.Command("bun", "add", "-g", "@onkernel/cli@latest")
	default:
		return fmt.Errorf("unknown installation method")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
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
	fmt.Printf("  sudo cp /tmp/kernel %s\n", binaryPath)
	pterm.Println()
}
