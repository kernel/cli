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
	defaultFileMode       = 0644
	webBotAuthCommit      = "3f437a1fb17dcfd31a33b268f2f9447a83122057"
	webBotAuthDownloadURL = "https://github.com/kernel/web-bot-auth/archive/" + webBotAuthCommit + ".zip"
	downloadTimeout       = 5 * time.Minute
	// defaultWebBotAuthKey is the RFC9421 test key that works with Cloudflare's test site
	// https://developers.cloudflare.com/bots/reference/bot-verification/web-bot-auth/
	defaultWebBotAuthKey = `{"kty":"OKP","crv":"Ed25519","d":"n4Ni-HpISpVObnQMW0wOhCKROaIKqKtW_2ZYb2p9KcU","x":"JrQLj5P_89iXES9-vFgrIy29clF9CC_oPPsw3c5D0bs"}`
)

type ExtensionsBuildWebBotAuthInput struct {
	Output            string
	HostURL           string
	KeyPath           string // Path to user's JWK or PEM file (optional, defaults to RFC9421 test key)
	ExtensionName     string // Name for the extension paths (defaults to "web-bot-auth")
	AutoUpload        bool   // Whether the extension will be automatically uploaded after building
	SignatureAgentURL string // URL of the signature agent
}

// BuildWebBotAuthOutput contains the result of building the extension
type BuildWebBotAuthOutput struct {
	ExtensionID string
	OutputDir   string
}

func BuildWebBotAuth(ctx context.Context, in ExtensionsBuildWebBotAuthInput) (*BuildWebBotAuthOutput, error) {
	pterm.Info.Println("Preparing web-bot-auth extension...")

	// Validate preconditions
	if err := validateToolDependencies(); err != nil {
		return nil, err
	}

	outputDir, err := filepath.Abs(in.Output)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve output path: %w", err)
	}
	if st, err := os.Stat(outputDir); err == nil {
		if !st.IsDir() {
			return nil, fmt.Errorf("output path exists and is not a directory: %s", outputDir)
		}
		entries, err := os.ReadDir(outputDir)
		if err != nil {
			return nil, fmt.Errorf("failed to read output directory: %w", err)
		}
		if len(entries) > 0 {
			return nil, fmt.Errorf("output directory must be empty: %s", outputDir)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, defaultDirMode); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	} else {
		return nil, fmt.Errorf("failed to check output directory: %w", err)
	}

	// Download and extract
	browserExtDir, cleanup, err := downloadAndExtractWebBotAuth(ctx)
	defer cleanup()
	if err != nil {
		return nil, err
	}

	// Load key (custom or default)
	var keyData string
	var usingDefaultKey bool
	if in.KeyPath != "" {
		pterm.Info.Printf("Loading custom key from %s...\n", in.KeyPath)
		keyBytes, err := os.ReadFile(in.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}
		keyData = string(keyBytes)
		usingDefaultKey = false
	} else {
		pterm.Info.Println("Using default RFC9421 test key (works with Cloudflare test site)...")
		keyData = defaultWebBotAuthKey
		usingDefaultKey = true
	}

	// Build extension
	extensionID, err := buildWebBotAuthExtension(ctx, browserExtDir, in.HostURL, keyData, in.ExtensionName, in.SignatureAgentURL)
	if err != nil {
		return nil, err
	}

	// Copy artifacts
	if err := copyExtensionArtifacts(browserExtDir, outputDir); err != nil {
		return nil, err
	}

	// Display success message
	displayWebBotAuthSuccess(outputDir, in.ExtensionName, extensionID, in.HostURL, usingDefaultKey, in.AutoUpload)

	return &BuildWebBotAuthOutput{
		ExtensionID: extensionID,
		OutputDir:   outputDir,
	}, nil
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
	if _, err := os.Stat(browserExtDir); err != nil {
		if os.IsNotExist(err) {
			return "", cleanup, fmt.Errorf("browser-extension directory not found in archive")
		}
		return "", cleanup, fmt.Errorf("failed to access browser-extension directory: %w", err)
	}

	return browserExtDir, cleanup, nil
}

// buildWebBotAuthExtension modifies templates, builds the extension, and returns the extension ID
// extensionName is used for URL paths (e.g., "web-bot-auth") instead of the Chrome extension ID
func buildWebBotAuthExtension(ctx context.Context, browserExtDir, hostURL, keyData, extensionName, signatureAgentURL string) (string, error) {
	// Normalize hostURL by removing trailing slashes to prevent double slashes in URLs
	hostURL = strings.TrimRight(hostURL, "/")

	// Validate key and write to browserExtDir before building
	pterm.Info.Println("Validating key...")
	var pemData []byte
	var err error
	// JWK data is used for keyid signing in background.ts
	var jwkData string

	if util.IsPEMKey(keyData) {
		// Key is already in PEM format, validate it
		if err := util.ValidatePEMKey(keyData); err != nil {
			return "", fmt.Errorf("failed to validate PEM key: %w", err)
		}

		jwkData, err = util.ConvertPEMToJWK(keyData)
		if err != nil {
			return "", fmt.Errorf("failed to convert PEM to JWK: %w", err)
		}
		pemData = []byte(keyData)
	} else {
		// Key is in JWK format, convert to PEM
		pemData, err = util.ConvertJWKToPEM(keyData)
		if err != nil {
			return "", fmt.Errorf("failed to convert JWK to PEM: %w", err)
		}
		jwkData = keyData
	}

	privateKeyPath := filepath.Join(browserExtDir, "private_key.pem")
	if err := os.WriteFile(privateKeyPath, pemData, 0600); err != nil {
		return "", fmt.Errorf("failed to write private key: %w", err)
	}
	pterm.Success.Println("Private key written successfully")

	// Inject the JWK into background.ts (replacing the hardcoded test key)
	pterm.Info.Println("Injecting custom JWK into background.ts...")
	backgroundTsPath := filepath.Join(browserExtDir, "src", "background.ts")
	if err := injectJWKIntoBackgroundTs(backgroundTsPath, jwkData); err != nil {
		return "", fmt.Errorf("failed to inject JWK: %w", err)
	}
	pterm.Success.Println("Custom JWK injected successfully")

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
	npmBuild.Env = append(os.Environ(), "SIGNATURE_AGENT_URL="+signatureAgentURL)
	npmBuild.Stdout = os.Stdout
	npmBuild.Stderr = os.Stderr
	if err := npmBuild.Run(); err != nil {
		return "", fmt.Errorf("npm run build:chrome failed: %w", err)
	}

	// Bundle the extension
	pterm.Info.Println("Bundling extension...")
	npmBundle := exec.CommandContext(ctx, "npm", "run", "bundle:chrome")
	npmBundle.Dir = browserExtDir
	npmBundle.Env = append(os.Environ(), "SIGNATURE_AGENT_URL="+signatureAgentURL)
	var bundleOutput bytes.Buffer
	npmBundle.Stdout = io.MultiWriter(os.Stdout, &bundleOutput)
	npmBundle.Stderr = os.Stderr
	if err := npmBundle.Run(); err != nil {
		return "", fmt.Errorf("npm run bundle:chrome failed: %w", err)
	}

	// Extract extension ID (still needed for logging/reference)
	extensionID := extractExtensionID(bundleOutput.String())
	if extensionID == "" {
		return "", fmt.Errorf("failed to extract extension ID from bundle output")
	}

	// Update URLs with extension name paths (not extension ID)
	// This allows using readable names like "web-bot-auth" instead of the Chrome extension ID
	pterm.Info.Printf("Updating URLs to use extension name: %s (Chrome ID: %s)\n", extensionName, extensionID)

	updateXMLPath := filepath.Join(browserExtDir, "dist", "web-ext-artifacts", "update.xml")
	extensionSpecificCodebase := fmt.Sprintf("%s/extensions/%s/http-message-signatures-extension.crx", hostURL, extensionName)
	if err := util.ModifyFile(updateXMLPath,
		fmt.Sprintf("%s/http-message-signatures-extension.crx", hostURL),
		extensionSpecificCodebase); err != nil {
		return "", fmt.Errorf("failed to update update.xml codebase: %w", err)
	}

	pterm.Info.Println("Updating policy files with extension-specific paths...")

	policyJSONPath := filepath.Join(browserExtDir, "policy", "policy.json")
	if err := util.ModifyFile(policyJSONPath,
		fmt.Sprintf("%s/update.xml", hostURL),
		fmt.Sprintf("%s/extensions/%s/update.xml", hostURL, extensionName)); err != nil {
		return "", fmt.Errorf("failed to update policy.json: %w", err)
	}

	plistPath := filepath.Join(browserExtDir, "policy", "com.google.Chrome.managed.plist")
	if err := util.ModifyFile(plistPath,
		fmt.Sprintf("%s/update.xml", hostURL),
		fmt.Sprintf("%s/extensions/%s/update.xml", hostURL, extensionName)); err != nil {
		return "", fmt.Errorf("failed to update plist: %w", err)
	}

	return extensionID, nil
}

// injectJWKIntoBackgroundTs replaces the hardcoded test key import with the custom JWK
func injectJWKIntoBackgroundTs(backgroundTsPath, jwkData string) error {
	content, err := os.ReadFile(backgroundTsPath)
	if err != nil {
		return fmt.Errorf("failed to read background.ts: %w", err)
	}

	contentStr := string(content)

	// Replace the import line with an inline constant
	// Find: import jwk from "../../rfc9421-keys/ed25519.json" assert { type: "json" };
	// Replace with: const jwk = {your-jwk-here};
	searchPattern := `import jwk from "../../rfc9421-keys/ed25519.json" assert { type: "json" };`
	replacement := fmt.Sprintf("const jwk = %s;", jwkData)

	if !strings.Contains(contentStr, searchPattern) {
		return fmt.Errorf("could not find JWK import statement in background.ts")
	}

	contentStr = strings.Replace(contentStr, searchPattern, replacement, 1)

	if err := os.WriteFile(backgroundTsPath, []byte(contentStr), 0644); err != nil {
		return fmt.Errorf("failed to write modified background.ts: %w", err)
	}

	return nil
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

	// Copy private key
	privateKeySrc := filepath.Join(browserExtDir, "private_key.pem")
	privateKeyDst := filepath.Join(outputDir, "private_key.pem")
	if _, err := os.Stat(privateKeySrc); err == nil {
		if err := util.CopyFile(privateKeySrc, privateKeyDst); err != nil {
			return fmt.Errorf("failed to copy private_key.pem: %w", err)
		}

		// Create .gitignore to prevent private key from being uploaded
		gitignorePath := filepath.Join(outputDir, ".gitignore")
		gitignoreContent := "# Exclude private key from uploads\nprivate_key.pem\n"
		if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), defaultFileMode); err != nil {
			return fmt.Errorf("failed to create .gitignore: %w", err)
		}
		pterm.Info.Println("Private key preserved (private_key.pem)")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat private_key.pem: %w", err)
	} else {
		pterm.Warning.Println("No private_key.pem found - extension ID may change on rebuild")
	}

	// Copy policy directory (contains Chrome enterprise policy configuration)
	// Note: This directory must exist since we modified files in it earlier during the build
	policySrc := filepath.Join(browserExtDir, "policy")
	policyDst := filepath.Join(outputDir, "policy")
	if err := util.CopyDir(policySrc, policyDst); err != nil {
		return fmt.Errorf("failed to copy policy directory: %w", err)
	}
	pterm.Info.Println("Policy files copied (required for Chrome configuration)")

	return nil
}

// displayWebBotAuthSuccess displays success message and next steps
func displayWebBotAuthSuccess(outputDir, extensionName, extensionID, hostURL string, usingDefaultKey, autoUpload bool) {
	pterm.Success.Println("Web-bot-auth extension prepared successfully!")
	pterm.Println()

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"Extension Name", extensionName})
	rows = append(rows, []string{"Chrome Extension ID", extensionID})
	rows = append(rows, []string{"Output directory", outputDir})
	rows = append(rows, []string{"Host URL", hostURL})
	if usingDefaultKey {
		rows = append(rows, []string{"Signing Key", "RFC9421 test key (Cloudflare test site)"})
	} else {
		rows = append(rows, []string{"Signing Key", "Custom key"})
	}
	table.PrintTableNoPad(rows, true)

	pterm.Println()
	pterm.Info.Println("Next steps:")

	stepNum := 1
	if !autoUpload {
		pterm.Printf("%d. Upload the extension:\n", stepNum)
		pterm.Printf("   kernel extensions upload %s --name %s\n\n", outputDir, extensionName)
		stepNum++
	}

	pterm.Printf("%d. Use in your browser:\n", stepNum)
	pterm.Printf("   kernel browsers create --extension %s\n\n", extensionName)

	pterm.Println()
	pterm.Info.Println("   For testing with Cloudflare's test site:")
	pterm.Printf("   • Test URL: https://http-message-signatures-example.research.cloudflare.com\n")
	pterm.Printf("   • Or: https://webbotauth.io/test\n")
	pterm.Println()

	if usingDefaultKey {
		pterm.Info.Println("Using default RFC9421 test key - compatible with Cloudflare test sites")
	} else {
		pterm.Warning.Println("⚠️  Private key saved to private_key.pem - keep it secure!")
		pterm.Info.Println("   It's automatically excluded when uploading via .gitignore")
	}

}
