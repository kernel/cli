package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/onkernel/cli/pkg/util"
	"github.com/onkernel/kernel-go-sdk"
)

// KernelPreferencesPath is the path to Chrome's Preferences file in Kernel browsers
const KernelPreferencesPath = "/home/kernel/user-data/Default/Preferences"

// LoadIntoBrowserOptions configures how the extension is loaded into a browser.
type LoadIntoBrowserOptions struct {
	// BrowserID is the Kernel browser session ID
	BrowserID string

	// Bundle is the extracted Claude extension bundle
	Bundle *Bundle

	// Client is the Kernel API client
	Client kernel.Client
}

// LoadIntoBrowser uploads the Claude extension and auth storage to a Kernel browser.
// This will:
// 1. Upload auth storage (if present) to the browser's user data directory
// 2. Set proper permissions on the auth storage
// 3. Load the extension (which triggers a browser restart)
func LoadIntoBrowser(ctx context.Context, opts LoadIntoBrowserOptions) error {
	fs := opts.Client.Browsers.Fs

	// Step 1: Upload auth storage if present
	if opts.Bundle.HasAuthStorage() {
		authDestPath := filepath.Join(KernelExtensionSettingsPath, ExtensionID)

		// Create a temp zip of just the auth storage contents
		authZipPath, err := createTempZip(opts.Bundle.AuthStoragePath)
		if err != nil {
			return fmt.Errorf("failed to create auth storage zip: %w", err)
		}
		defer os.Remove(authZipPath)

		// Upload the auth storage zip
		authZipFile, err := os.Open(authZipPath)
		if err != nil {
			return fmt.Errorf("failed to open auth storage zip: %w", err)
		}
		defer authZipFile.Close()

		if err := fs.UploadZip(ctx, opts.BrowserID, kernel.BrowserFUploadZipParams{
			DestPath: authDestPath,
			ZipFile:  authZipFile,
		}); err != nil {
			return fmt.Errorf("failed to upload auth storage: %w", err)
		}

		// Set proper ownership on the auth storage directory and all files inside
		// using chown -R via process exec (SetFilePermissions is not recursive)
		proc := opts.Client.Browsers.Process
		_, err = proc.Exec(ctx, opts.BrowserID, kernel.BrowserProcessExecParams{
			Command:    "chown",
			Args:       []string{"-R", KernelUser + ":" + KernelUser, authDestPath},
			AsRoot:     kernel.Opt(true),
			TimeoutSec: kernel.Opt(int64(30)),
		})
		if err != nil {
			return fmt.Errorf("failed to set auth storage ownership: %w", err)
		}
	}

	// Step 2: Upload and load the extension
	// Create a temp zip of the extension
	extZipPath, err := createTempZip(opts.Bundle.ExtensionPath)
	if err != nil {
		return fmt.Errorf("failed to create extension zip: %w", err)
	}
	defer os.Remove(extZipPath)

	extZipFile, err := os.Open(extZipPath)
	if err != nil {
		return fmt.Errorf("failed to open extension zip: %w", err)
	}
	defer extZipFile.Close()

	// Use the LoadExtensions API which handles the extension loading and browser restart
	if err := opts.Client.Browsers.LoadExtensions(ctx, opts.BrowserID, kernel.BrowserLoadExtensionsParams{
		Extensions: []kernel.BrowserLoadExtensionsParamsExtension{
			{
				Name:    "claude-for-chrome",
				ZipFile: extZipFile,
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to load extension: %w", err)
	}

	// Step 3: Pin the extension to the toolbar
	// We need to:
	// 1. Stop Chromium so it doesn't overwrite our Preferences changes
	// 2. Update the Preferences file
	// 3. Restart Chromium to pick up the changes
	proc := opts.Client.Browsers.Process

	// Stop Chromium first (use Exec to wait for it to complete)
	_, _ = proc.Exec(ctx, opts.BrowserID, kernel.BrowserProcessExecParams{
		Command:    "supervisorctl",
		Args:       []string{"stop", "chromium"},
		AsRoot:     kernel.Opt(true),
		TimeoutSec: kernel.Opt(int64(30)),
	})

	// Now update the Preferences file while Chrome is stopped
	if err := pinExtension(ctx, opts.Client, opts.BrowserID, ExtensionID); err != nil {
		// Don't fail the whole operation if pinning fails - it's a nice-to-have
		// The extension is still loaded and functional
		// But still restart Chromium
		_, _ = proc.Spawn(ctx, opts.BrowserID, kernel.BrowserProcessSpawnParams{
			Command: "supervisorctl",
			Args:    []string{"start", "chromium"},
			AsRoot:  kernel.Opt(true),
		})
		return nil
	}

	// Restart Chromium to pick up the new pinned extension preference
	// Use Spawn (fire and forget) - the Playwright call below will retry until Chrome is ready
	_, _ = proc.Spawn(ctx, opts.BrowserID, kernel.BrowserProcessSpawnParams{
		Command: "supervisorctl",
		Args:    []string{"start", "chromium"},
		AsRoot:  kernel.Opt(true),
	})

	// Step 4: Close extra tabs and navigate to chrome://newtab
	// The Claude extension opens a tab to claude.ai by default
	navigateScript := `
		const pages = context.pages();
		// Close all but the first page
		for (let i = 1; i < pages.length; i++) {
			await pages[i].close();
		}
		// Navigate the remaining page to newtab
		if (pages.length > 0) {
			await pages[0].goto('chrome://newtab');
		}
	`
	_, _ = opts.Client.Browsers.Playwright.Execute(ctx, opts.BrowserID, kernel.BrowserPlaywrightExecuteParams{
		Code:       navigateScript,
		TimeoutSec: kernel.Opt(int64(30)),
	})

	return nil
}

// pinExtension adds an extension ID to Chrome's pinned_extensions list in the Preferences file.
// This makes the extension icon visible in the toolbar by default.
func pinExtension(ctx context.Context, client kernel.Client, browserID, extensionID string) error {
	fs := client.Browsers.Fs

	// Read the current Preferences file
	resp, err := fs.ReadFile(ctx, browserID, kernel.BrowserFReadFileParams{
		Path: KernelPreferencesPath,
	})
	if err != nil {
		return fmt.Errorf("failed to read preferences: %w", err)
	}
	defer resp.Body.Close()

	prefsData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read preferences body: %w", err)
	}

	// Parse the JSON
	var prefs map[string]any
	if err := json.Unmarshal(prefsData, &prefs); err != nil {
		return fmt.Errorf("failed to parse preferences: %w", err)
	}

	// Get or create the extensions object
	extensions, ok := prefs["extensions"].(map[string]any)
	if !ok {
		extensions = make(map[string]any)
		prefs["extensions"] = extensions
	}

	// Get or create the pinned_extensions array
	var pinnedExtensions []string
	if pinned, ok := extensions["pinned_extensions"].([]any); ok {
		for _, id := range pinned {
			if s, ok := id.(string); ok {
				pinnedExtensions = append(pinnedExtensions, s)
			}
		}
	}

	// Check if extension is already pinned
	for _, id := range pinnedExtensions {
		if id == extensionID {
			// Already pinned, nothing to do
			return nil
		}
	}

	// Add the extension to pinned list
	pinnedExtensions = append(pinnedExtensions, extensionID)
	extensions["pinned_extensions"] = pinnedExtensions

	// Serialize back to JSON
	newPrefsData, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("failed to serialize preferences: %w", err)
	}

	// Write the updated Preferences file
	if err := fs.WriteFile(ctx, browserID, bytes.NewReader(newPrefsData), kernel.BrowserFWriteFileParams{
		Path: KernelPreferencesPath,
	}); err != nil {
		return fmt.Errorf("failed to write preferences: %w", err)
	}

	return nil
}

// OpenSidePanel clicks on the pinned Claude extension icon to open the side panel.
// This uses the computer API to click at the known coordinates of the extension icon.
// If the side panel is already open, it does nothing.
func OpenSidePanel(ctx context.Context, client kernel.Client, browserID string) error {
	// First check if the side panel is already open
	checkScript := `
		const sidepanel = context.pages().find(p => p.url().includes('sidepanel.html'));
		return { isOpen: !!sidepanel };
	`
	result, err := client.Browsers.Playwright.Execute(ctx, browserID, kernel.BrowserPlaywrightExecuteParams{
		Code:       checkScript,
		TimeoutSec: kernel.Opt(int64(10)),
	})
	if err == nil && result.Success {
		if resultMap, ok := result.Result.(map[string]any); ok {
			if isOpen, ok := resultMap["isOpen"].(bool); ok && isOpen {
				// Side panel is already open, no need to click
				return nil
			}
		}
	}

	// Side panel is not open, click to open it
	return client.Browsers.Computer.ClickMouse(ctx, browserID, kernel.BrowserComputerClickMouseParams{
		X: ExtensionIconX,
		Y: ExtensionIconY,
	})
}

// createTempZip creates a temporary zip file from a directory.
func createTempZip(srcDir string) (string, error) {
	tmpFile, err := os.CreateTemp("", "claude-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	if err := util.ZipDirectory(srcDir, tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	return tmpPath, nil
}
