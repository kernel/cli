package extensions

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	"github.com/pterm/pterm"
)

const (
	defaultLocalhostURL   = "http://localhost:8000"
	defaultDirMode        = 0755
	webBotAuthDownloadURL = "https://github.com/cloudflare/web-bot-auth/archive/refs/heads/main.zip"
	downloadTimeout       = 5 * time.Minute
)

type ExtensionsPrepareWebBotAuthInput struct {
	Output  string
	HostURL string
}

func PrepareWebBotAuth(ctx context.Context, in ExtensionsPrepareWebBotAuthInput) error {
	pterm.Info.Println("Preparing web-bot-auth extension...")

	// Validate preconditions
	if err := validateToolDependencies(); err != nil {
		return err
	}

	outputDir, err := filepath.Abs(in.Output)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}
	if st, err := os.Stat(outputDir); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("output path exists and is not a directory: %s", outputDir)
		}
		entries, _ := os.ReadDir(outputDir)
		if len(entries) > 0 {
			return fmt.Errorf("output directory must be empty: %s", outputDir)
		}
	} else {
		if err := os.MkdirAll(outputDir, defaultDirMode); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Download and extract
	browserExtDir, cleanup, err := downloadAndExtractWebBotAuth(ctx)
	defer cleanup()
	if err != nil {
		return err
	}

	// Build extension
	extensionID, err := buildWebBotAuthExtension(ctx, browserExtDir, in.HostURL)
	if err != nil {
		return err
	}

	// Copy artifacts
	if err := copyExtensionArtifacts(browserExtDir, outputDir); err != nil {
		return err
	}

	// Display success message
	displayWebBotAuthSuccess(outputDir, extensionID, in.HostURL)

	return nil
}

// extractExtensionID extracts the extension ID from npm bundle output
func extractExtensionID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if after, found := strings.CutPrefix(line, "Build Extension with ID:"); found {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// validateToolDependencies checks for required tools (node and npm)
func validateToolDependencies() error {
	if _, err := exec.LookPath("node"); err != nil {
		pterm.Error.Println("Node.js is required but not found in PATH")
		pterm.Info.Println("Please install Node.js from https://nodejs.org/")
		return fmt.Errorf("node not found")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		pterm.Error.Println("npm is required but not found in PATH")
		pterm.Info.Println("Please install npm (usually comes with Node.js)")
		return fmt.Errorf("npm not found")
	}
	return nil
}

// downloadAndExtractWebBotAuth downloads and extracts the web-bot-auth repo, returns the browser-extension directory path
func downloadAndExtractWebBotAuth(ctx context.Context) (browserExtDir string, cleanup func(), err error) {
	cleanup = func() {}

	// Download from GitHub
	pterm.Info.Printf("Downloading web-bot-auth from GitHub...\n")
	client := &http.Client{Timeout: downloadTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, webBotAuthDownloadURL, nil)
	if err != nil {
		return "", cleanup, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", cleanup, fmt.Errorf("failed to download web-bot-auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", cleanup, fmt.Errorf("failed to download web-bot-auth: HTTP %d", resp.StatusCode)
	}

	// Save to temporary file
	tmpZip, err := os.CreateTemp("", "web-bot-auth-*.zip")
	if err != nil {
		return "", cleanup, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpZipPath := tmpZip.Name()
	cleanup = func() { os.Remove(tmpZipPath) }

	if _, err := io.Copy(tmpZip, resp.Body); err != nil {
		tmpZip.Close()
		return "", cleanup, fmt.Errorf("failed to save download: %w", err)
	}
	tmpZip.Close()

	// Extract to temporary directory
	tmpExtractDir, err := os.MkdirTemp("", "web-bot-auth-extract-*")
	if err != nil {
		return "", cleanup, fmt.Errorf("failed to create temp directory: %w", err)
	}
	cleanup = func() {
		os.Remove(tmpZipPath)
		os.RemoveAll(tmpExtractDir)
	}

	pterm.Info.Println("Extracting archive...")
	if err := util.Unzip(tmpZipPath, tmpExtractDir); err != nil {
		return "", cleanup, fmt.Errorf("failed to extract archive: %w", err)
	}

	entries, err := os.ReadDir(tmpExtractDir)
	if err != nil {
		return "", cleanup, fmt.Errorf("failed to read extracted directory: %w", err)
	}
	if len(entries) == 0 {
		return "", cleanup, fmt.Errorf("extracted archive is empty")
	}

	extractedDir := filepath.Join(tmpExtractDir, entries[0].Name())
	browserExtDir = filepath.Join(extractedDir, "examples", "browser-extension")

	// Verify the browser-extension directory exists
	if _, err := os.Stat(browserExtDir); os.IsNotExist(err) {
		return "", cleanup, fmt.Errorf("browser-extension directory not found in archive")
	}

	return browserExtDir, cleanup, nil
}

// buildWebBotAuthExtension modifies templates, builds the extension, and returns the extension ID
func buildWebBotAuthExtension(ctx context.Context, browserExtDir, hostURL string) (string, error) {
	// Modify template files
	pterm.Info.Println("Modifying templates with host URL...")

	policyTemplPath := filepath.Join(browserExtDir, "policy", "policy.json.templ")
	if err := util.ModifyFile(policyTemplPath, defaultLocalhostURL, hostURL); err != nil {
		return "", fmt.Errorf("failed to modify policy.json.templ: %w", err)
	}

	plistTemplPath := filepath.Join(browserExtDir, "policy", "com.google.Chrome.managed.plist.templ")
	if err := util.ModifyFile(plistTemplPath, defaultLocalhostURL, hostURL); err != nil {
		return "", fmt.Errorf("failed to modify plist template: %w", err)
	}

	buildScriptPath := filepath.Join(browserExtDir, "scripts", "build_web_artifacts.mjs")
	if err := util.ModifyFile(buildScriptPath, defaultLocalhostURL+"/", hostURL+"/"); err != nil {
		return "", fmt.Errorf("failed to modify build script: %w", err)
	}

	// Get the root directory (parent of browser-extension)
	extractedDir := filepath.Dir(filepath.Dir(browserExtDir))

	// Install dependencies
	pterm.Info.Println("Installing dependencies (this may take a minute)...")
	npmInstall := exec.CommandContext(ctx, "npm", "install")
	npmInstall.Dir = extractedDir
	npmInstall.Stdout = os.Stdout
	npmInstall.Stderr = os.Stderr
	if err := npmInstall.Run(); err != nil {
		return "", fmt.Errorf("npm install failed: %w", err)
	}

	// Build workspace packages
	pterm.Info.Println("Building workspace packages...")
	npmBuildWorkspaces := exec.CommandContext(ctx, "npm", "run", "build")
	npmBuildWorkspaces.Dir = extractedDir
	npmBuildWorkspaces.Stdout = os.Stdout
	npmBuildWorkspaces.Stderr = os.Stderr
	if err := npmBuildWorkspaces.Run(); err != nil {
		return "", fmt.Errorf("npm run build (workspaces) failed: %w", err)
	}

	// Build the extension
	pterm.Info.Println("Building extension...")
	npmBuild := exec.CommandContext(ctx, "npm", "run", "build:chrome")
	npmBuild.Dir = browserExtDir
	npmBuild.Stdout = os.Stdout
	npmBuild.Stderr = os.Stderr
	if err := npmBuild.Run(); err != nil {
		return "", fmt.Errorf("npm run build:chrome failed: %w", err)
	}

	// Bundle the extension
	pterm.Info.Println("Bundling extension...")
	npmBundle := exec.CommandContext(ctx, "npm", "run", "bundle:chrome")
	npmBundle.Dir = browserExtDir
	var bundleOutput bytes.Buffer
	npmBundle.Stdout = io.MultiWriter(os.Stdout, &bundleOutput)
	npmBundle.Stderr = os.Stderr
	if err := npmBundle.Run(); err != nil {
		return "", fmt.Errorf("npm run bundle:chrome failed: %w", err)
	}

	// Extract extension ID
	extensionID := extractExtensionID(bundleOutput.String())
	if extensionID == "" {
		return "", fmt.Errorf("failed to extract extension ID from bundle output")
	}

	// Update URLs with extension-specific paths
	pterm.Info.Printf("Updating URLs to use extension ID: %s\n", extensionID)

	updateXMLPath := filepath.Join(browserExtDir, "dist", "web-ext-artifacts", "update.xml")
	extensionSpecificCodebase := fmt.Sprintf("%s/extensions/%s/http-message-signatures-extension.crx", hostURL, extensionID)
	if err := util.ModifyFile(updateXMLPath,
		fmt.Sprintf("%s/http-message-signatures-extension.crx", hostURL),
		extensionSpecificCodebase); err != nil {
		pterm.Warning.Printf("Failed to update update.xml codebase: %v\n", err)
	}

	pterm.Info.Println("Updating policy files with extension-specific paths...")

	policyJSONPath := filepath.Join(browserExtDir, "policy", "policy.json")
	if err := util.ModifyFile(policyJSONPath,
		fmt.Sprintf("%s/update.xml", hostURL),
		fmt.Sprintf("%s/extensions/%s/update.xml", hostURL, extensionID)); err != nil {
		pterm.Warning.Printf("Failed to update policy.json: %v\n", err)
	}

	plistPath := filepath.Join(browserExtDir, "policy", "com.google.Chrome.managed.plist")
	if err := util.ModifyFile(plistPath,
		fmt.Sprintf("%s/update.xml", hostURL),
		fmt.Sprintf("%s/extensions/%s/update.xml", hostURL, extensionID)); err != nil {
		pterm.Warning.Printf("Failed to update plist: %v\n", err)
	}

	return extensionID, nil
}

// copyExtensionArtifacts copies built extension files to the output directory
func copyExtensionArtifacts(browserExtDir, outputDir string) error {
	pterm.Info.Println("Copying extension files to output directory...")

	chromiumSrc := filepath.Join(browserExtDir, "dist", "mv3", "chromium")
	entries, err := os.ReadDir(chromiumSrc)
	if err != nil {
		return fmt.Errorf("failed to read chromium directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(chromiumSrc, entry.Name())
		dstPath := filepath.Join(outputDir, entry.Name())

		if entry.IsDir() {
			if err := util.CopyDir(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", entry.Name(), err)
			}
		} else {
			if err := util.CopyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", entry.Name(), err)
			}
		}
	}

	updateXMLSrc := filepath.Join(browserExtDir, "dist", "web-ext-artifacts", "update.xml")
	updateXMLDst := filepath.Join(outputDir, "update.xml")
	if err := util.CopyFile(updateXMLSrc, updateXMLDst); err != nil {
		return fmt.Errorf("failed to copy update.xml: %w", err)
	}

	crxSrc := filepath.Join(browserExtDir, "dist", "web-ext-artifacts", "http-message-signatures-extension.crx")
	crxDst := filepath.Join(outputDir, "http-message-signatures-extension.crx")
	if err := util.CopyFile(crxSrc, crxDst); err != nil {
		return fmt.Errorf("failed to copy .crx file: %w", err)
	}

	return nil
}

// displayWebBotAuthSuccess displays success message and next steps
func displayWebBotAuthSuccess(outputDir, extensionID, hostURL string) {
	pterm.Success.Println("Web-bot-auth extension prepared successfully!")
	pterm.Println()

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"Extension ID", extensionID})
	rows = append(rows, []string{"Output directory", outputDir})
	rows = append(rows, []string{"Host URL", hostURL})
	table.PrintTableNoPad(rows, true)

	pterm.Println()
	pterm.Info.Println("Next steps:")
	pterm.Printf("1. Upload using the extension ID as the name:\n")
	pterm.Printf("   kernel extensions upload %s --name %s\n\n", outputDir, extensionID)
	pterm.Printf("2. Use in your browser:\n")
	pterm.Printf("   kernel browsers create --extension %s\n\n", extensionID)
	pterm.Warning.Println("⚠️  Keep private_key.pem secure - it determines your extension ID!")
}
