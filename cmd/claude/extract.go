package claude

import (
	"fmt"
	"path/filepath"

	"github.com/onkernel/cli/internal/claude"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract Claude extension from local Chrome",
	Long: `Extract the Claude for Chrome extension and its authentication data from your
local Chrome installation.

This creates a bundle zip file that can be used with 'kernel claude launch' or
'kernel claude load' to load the extension into a Kernel browser.

The bundle includes:
- The extension files (manifest.json, scripts, etc.)
- Authentication storage (optional, enabled by default)

By default, the extension is extracted from Chrome's Default profile. Use
--chrome-profile to specify a different profile if you have multiple Chrome
profiles.`,
	Example: `  # Extract with default settings
  kernel claude extract

  # Extract to a specific file
  kernel claude extract -o my-claude-bundle.zip

  # Extract from a specific Chrome profile
  kernel claude extract --chrome-profile "Profile 1"

  # Extract without authentication (will require login)
  kernel claude extract --no-auth`,
	RunE: runExtract,
}

func init() {
	extractCmd.Flags().StringP("output", "o", claude.DefaultBundleName, "Output path for the bundle zip file")
	extractCmd.Flags().String("chrome-profile", "Default", "Chrome profile name to extract from")
	extractCmd.Flags().Bool("no-auth", false, "Skip authentication storage (extension will require login)")
	extractCmd.Flags().Bool("list-profiles", false, "List available Chrome profiles and exit")
}

func runExtract(cmd *cobra.Command, args []string) error {
	listProfiles, _ := cmd.Flags().GetBool("list-profiles")

	if listProfiles {
		return listChromeProfiles()
	}

	outputPath, _ := cmd.Flags().GetString("output")
	chromeProfile, _ := cmd.Flags().GetString("chrome-profile")
	noAuth, _ := cmd.Flags().GetBool("no-auth")

	// Make output path absolute
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}

	pterm.Info.Printf("Extracting Claude extension from Chrome profile: %s\n", chromeProfile)

	// Check if extension exists
	extPath, err := claude.GetChromeExtensionPath(chromeProfile)
	if err != nil {
		pterm.Error.Printf("Could not find Claude extension: %v\n", err)
		pterm.Info.Println("Make sure:")
		pterm.Info.Println("  1. Chrome is installed")
		pterm.Info.Println("  2. The Claude for Chrome extension is installed")
		pterm.Info.Println("  3. You're using the correct Chrome profile (use --list-profiles to see available profiles)")
		return nil
	}

	pterm.Info.Printf("Found extension at: %s\n", extPath)

	// Check auth storage
	includeAuth := !noAuth
	if includeAuth {
		authPath, err := claude.GetChromeAuthStoragePath(chromeProfile)
		if err != nil {
			pterm.Warning.Printf("Could not find auth storage: %v\n", err)
			pterm.Info.Println("The bundle will be created without authentication.")
			pterm.Info.Println("You will need to log in after loading the extension.")
			includeAuth = false
		} else {
			pterm.Info.Printf("Found auth storage at: %s\n", authPath)
		}
	} else {
		pterm.Info.Println("Skipping auth storage (--no-auth specified)")
	}

	// Create the bundle
	pterm.Info.Printf("Creating bundle: %s\n", absOutput)

	if err := claude.CreateBundle(absOutput, chromeProfile, includeAuth); err != nil {
		return fmt.Errorf("failed to create bundle: %w", err)
	}

	pterm.Success.Printf("Bundle created: %s\n", absOutput)

	if includeAuth {
		pterm.Info.Println("The bundle includes authentication - Claude should be pre-logged-in.")
	} else {
		pterm.Warning.Println("The bundle does not include authentication - you will need to log in.")
	}

	pterm.Println()
	pterm.Info.Println("Next steps:")
	pterm.Println("  # Launch a browser with the extension")
	pterm.Printf("  kernel claude launch -b %s\n", outputPath)
	pterm.Println()
	pterm.Println("  # Or load into an existing browser")
	pterm.Printf("  kernel claude load <browser-id> -b %s\n", outputPath)

	return nil
}

func listChromeProfiles() error {
	pterm.Info.Println("Searching for Chrome profiles...")

	profiles, err := claude.ListChromeProfiles()
	if err != nil {
		return fmt.Errorf("failed to list Chrome profiles: %w", err)
	}

	if len(profiles) == 0 {
		pterm.Warning.Println("No Chrome profiles found")
		return nil
	}

	pterm.Success.Printf("Found %d Chrome profile(s):\n", len(profiles))
	for _, profile := range profiles {
		// Check if Claude extension is installed in this profile
		_, err := claude.GetChromeExtensionPath(profile)
		hasClaude := err == nil

		status := ""
		if hasClaude {
			status = " (Claude extension installed)"
		}

		pterm.Printf("  - %s%s\n", profile, status)
	}

	return nil
}
