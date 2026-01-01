package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onkernel/cli/pkg/util"
	"github.com/onkernel/kernel-go-sdk"
)

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

		// Set proper ownership on the auth storage directory
		if err := fs.SetFilePermissions(ctx, opts.BrowserID, kernel.BrowserFSetFilePermissionsParams{
			Path:  authDestPath,
			Mode:  "0755",
			Owner: kernel.Opt(KernelUser),
			Group: kernel.Opt(KernelUser),
		}); err != nil {
			return fmt.Errorf("failed to set auth storage permissions: %w", err)
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

	return nil
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
