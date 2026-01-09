package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/update"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// InstallMethod represents how kernel was installed
type InstallMethod string

const (
	InstallMethodBrew    InstallMethod = "brew"
	InstallMethodPNPM    InstallMethod = "pnpm"
	InstallMethodNPM     InstallMethod = "npm"
	InstallMethodBun     InstallMethod = "bun"
	InstallMethodUnknown InstallMethod = "unknown"
)

var dryRun bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the Kernel CLI to the latest version",
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

	latestTag, _, err := update.FetchLatest(ctx)
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
		pterm.Success.Printf("You are already on the latest version (%s)\n", currentVersion)
		return nil
	} else {
		pterm.Info.Printf("New version available: %s → %s\n", currentVersion, strings.TrimPrefix(latestTag, "v"))
	}

	// Detect installation method
	method, binaryPath := DetectInstallMethod()

	if method == InstallMethodUnknown {
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

// DetectInstallMethod detects how kernel was installed and returns the method
// along with the path to the kernel binary.
func DetectInstallMethod() (InstallMethod, string) {
	// Collect candidate paths: current executable and shell-resolved binary
	candidates := []string{}
	binaryPath := ""

	if exe, err := os.Executable(); err == nil && exe != "" {
		if real, err2 := filepath.EvalSymlinks(exe); err2 == nil && real != "" {
			exe = real
		}
		candidates = append(candidates, exe)
		binaryPath = exe
	}
	if which, err := exec.LookPath("kernel"); err == nil && which != "" {
		candidates = append(candidates, which)
		if binaryPath == "" {
			binaryPath = which
		}
	}

	// Helpers
	norm := func(p string) string { return strings.ToLower(filepath.ToSlash(p)) }
	hasHomebrew := func(p string) bool {
		p = norm(p)
		return strings.Contains(p, "homebrew") || strings.Contains(p, "/cellar/")
	}
	hasBun := func(p string) bool { p = norm(p); return strings.Contains(p, "/.bun/") }
	hasPNPM := func(p string) bool {
		p = norm(p)
		return strings.Contains(p, "/pnpm/") || strings.Contains(p, "/.pnpm/")
	}
	hasNPM := func(p string) bool {
		p = norm(p)
		return strings.Contains(p, "/npm/") || strings.Contains(p, "/node_modules/.bin/")
	}

	type rule struct {
		check   func(string) bool
		envKeys []string
		method  InstallMethod
	}

	rules := []rule{
		{hasHomebrew, nil, InstallMethodBrew},
		{hasBun, []string{"BUN_INSTALL"}, InstallMethodBun},
		{hasPNPM, []string{"PNPM_HOME"}, InstallMethodPNPM},
		{hasNPM, []string{"NPM_CONFIG_PREFIX", "npm_config_prefix", "VOLTA_HOME"}, InstallMethodNPM},
	}

	// Path-based detection first
	for _, c := range candidates {
		for _, r := range rules {
			if r.check != nil && r.check(c) {
				return r.method, binaryPath
			}
		}
	}

	// Env-only fallbacks
	envSet := func(keys []string) bool {
		for _, k := range keys {
			if k == "" {
				continue
			}
			if os.Getenv(k) != "" {
				return true
			}
		}
		return false
	}
	for _, r := range rules {
		if len(r.envKeys) > 0 && envSet(r.envKeys) {
			return r.method, binaryPath
		}
	}

	return InstallMethodUnknown, binaryPath
}

// getUpgradeCommand returns the command string for a given installation method
func getUpgradeCommand(method InstallMethod) string {
	switch method {
	case InstallMethodBrew:
		return "brew upgrade kernel/tap/kernel"
	case InstallMethodPNPM:
		return "pnpm add -g @onkernel/cli@latest"
	case InstallMethodNPM:
		return "npm i -g @onkernel/cli@latest"
	case InstallMethodBun:
		return "bun add -g @onkernel/cli@latest"
	default:
		return ""
	}
}

// executeUpgrade runs the appropriate upgrade command based on the installation method
func executeUpgrade(method InstallMethod) error {
	var cmd *exec.Cmd

	switch method {
	case InstallMethodBrew:
		cmd = exec.Command("brew", "upgrade", "kernel/tap/kernel")
	case InstallMethodPNPM:
		cmd = exec.Command("pnpm", "add", "-g", "@onkernel/cli@latest")
	case InstallMethodNPM:
		cmd = exec.Command("npm", "i", "-g", "@onkernel/cli@latest")
	case InstallMethodBun:
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
