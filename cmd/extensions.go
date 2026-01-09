package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// ExtensionsService defines the subset of the Kernel SDK extension client that we use.
type ExtensionsService interface {
	List(ctx context.Context, opts ...option.RequestOption) (res *[]kernel.ExtensionListResponse, err error)
	Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) (err error)
	Download(ctx context.Context, idOrName string, opts ...option.RequestOption) (res *http.Response, err error)
	DownloadFromChromeStore(ctx context.Context, query kernel.ExtensionDownloadFromChromeStoreParams, opts ...option.RequestOption) (res *http.Response, err error)
	Upload(ctx context.Context, body kernel.ExtensionUploadParams, opts ...option.RequestOption) (res *kernel.ExtensionUploadResponse, err error)
}

type ExtensionsListInput struct {
	Output string
}

type ExtensionsDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

type ExtensionsDownloadInput struct {
	Identifier string
	Output     string
}

type ExtensionsDownloadWebStoreInput struct {
	URL    string
	Output string
	OS     string
}

type ExtensionsUploadInput struct {
	Dir    string
	Name   string
	Output string
}

type ExtensionsBuildWebBotAuthInput struct {
	Output  string
	KeyFile string
	Upload  bool
	Name    string
}

// ExtensionsCmd handles extension operations independent of cobra.
type ExtensionsCmd struct {
	extensions ExtensionsService
}

func (e ExtensionsCmd) List(ctx context.Context, in ExtensionsListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Output != "json" {
		pterm.Info.Println("Fetching extensions...")
	}
	items, err := e.extensions.List(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if items == nil || len(*items) == 0 {
			fmt.Println("[]")
			return nil
		}
		bs, err := json.MarshalIndent(*items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	if items == nil || len(*items) == 0 {
		pterm.Info.Println("No extensions found")
		return nil
	}
	rows := pterm.TableData{{"Extension ID", "Name", "Created At", "Size (bytes)", "Last Used At"}}
	for _, it := range *items {
		name := it.Name
		if name == "" {
			name = "-"
		}
		rows = append(rows, []string{
			it.ID,
			name,
			util.FormatLocal(it.CreatedAt),
			fmt.Sprintf("%d", it.SizeBytes),
			util.FormatLocal(it.LastUsedAt),
		})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func (e ExtensionsCmd) Delete(ctx context.Context, in ExtensionsDeleteInput) error {
	if in.Identifier == "" {
		pterm.Error.Println("Missing identifier")
		return nil
	}

	if !in.SkipConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete extension '%s'?", in.Identifier)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := e.extensions.Delete(ctx, in.Identifier); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Extension '%s' not found\n", in.Identifier)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted extension: %s\n", in.Identifier)
	return nil
}

func (e ExtensionsCmd) Download(ctx context.Context, in ExtensionsDownloadInput) error {
	if in.Identifier == "" {
		pterm.Error.Println("Missing identifier")
		return nil
	}
	res, err := e.extensions.Download(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()
	if in.Output == "" {
		pterm.Error.Println("Missing --to output directory")
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}

	outDir, err := filepath.Abs(in.Output)
	if err != nil {
		pterm.Error.Printf("Failed to resolve output path: %v\n", err)
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	// Create directory if not exists; if exists, ensure empty
	if st, err := os.Stat(outDir); err == nil {
		if !st.IsDir() {
			pterm.Error.Printf("Output path exists and is not a directory: %s\n", outDir)
			_, _ = io.Copy(io.Discard, res.Body)
			return nil
		}
		entries, _ := os.ReadDir(outDir)
		if len(entries) > 0 {
			pterm.Error.Printf("Output directory must be empty: %s\n", outDir)
			_, _ = io.Copy(io.Discard, res.Body)
			return nil
		}
	} else {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			pterm.Error.Printf("Failed to create output directory: %v\n", err)
			_, _ = io.Copy(io.Discard, res.Body)
			return nil
		}
	}

	// Write response to a temp zip, then extract
	tmpZip, err := os.CreateTemp("", "kernel-ext-*.zip")
	if err != nil {
		pterm.Error.Printf("Failed to create temp zip: %v\n", err)
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	tmpName := tmpZip.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := io.Copy(tmpZip, res.Body); err != nil {
		_ = tmpZip.Close()
		pterm.Error.Printf("Failed to read response: %v\n", err)
		return nil
	}
	_ = tmpZip.Close()
	if err := util.Unzip(tmpName, outDir); err != nil {
		pterm.Error.Printf("Failed to extract zip: %v\n", err)
		return nil
	}
	pterm.Success.Printf("Extracted extension to %s\n", outDir)
	return nil
}

func (e ExtensionsCmd) DownloadWebStore(ctx context.Context, in ExtensionsDownloadWebStoreInput) error {
	if in.URL == "" {
		pterm.Error.Println("Missing URL argument")
		return nil
	}
	params := kernel.ExtensionDownloadFromChromeStoreParams{URL: in.URL}
	switch in.OS {
	case "", string(kernel.ExtensionDownloadFromChromeStoreParamsOsLinux):
		// default linux
	case string(kernel.ExtensionDownloadFromChromeStoreParamsOsMac):
		params.Os = kernel.ExtensionDownloadFromChromeStoreParamsOsMac
	case string(kernel.ExtensionDownloadFromChromeStoreParamsOsWin):
		params.Os = kernel.ExtensionDownloadFromChromeStoreParamsOsWin
	default:
		pterm.Error.Println("--os must be one of mac, win, linux")
		return nil
	}

	res, err := e.extensions.DownloadFromChromeStore(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()

	if in.Output == "" {
		pterm.Error.Println("Missing --to output directory")
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}

	outDir, err := filepath.Abs(in.Output)
	if err != nil {
		pterm.Error.Printf("Failed to resolve output path: %v\n", err)
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	if st, err := os.Stat(outDir); err == nil {
		if !st.IsDir() {
			pterm.Error.Printf("Output path exists and is not a directory: %s\n", outDir)
			_, _ = io.Copy(io.Discard, res.Body)
			return nil
		}
		entries, _ := os.ReadDir(outDir)
		if len(entries) > 0 {
			pterm.Error.Printf("Output directory must be empty: %s\n", outDir)
			_, _ = io.Copy(io.Discard, res.Body)
			return nil
		}
	} else {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			pterm.Error.Printf("Failed to create output directory: %v\n", err)
			_, _ = io.Copy(io.Discard, res.Body)
			return nil
		}
	}

	// Save to temp zip then extract
	var bodyBuf bytes.Buffer
	if _, err := io.Copy(&bodyBuf, res.Body); err != nil {
		pterm.Error.Printf("Failed to read response: %v\n", err)
		return nil
	}
	tmpZip, err := os.CreateTemp("", "kernel-webstore-*.zip")
	if err != nil {
		pterm.Error.Printf("Failed to create temp zip: %v\n", err)
		return nil
	}
	tmpName := tmpZip.Name()
	if _, err := tmpZip.Write(bodyBuf.Bytes()); err != nil {
		_ = tmpZip.Close()
		pterm.Error.Printf("Failed to write temp zip: %v\n", err)
		return nil
	}
	_ = tmpZip.Close()
	defer os.Remove(tmpName)
	if err := util.Unzip(tmpName, outDir); err != nil {
		pterm.Error.Printf("Failed to extract zip: %v\n", err)
		return nil
	}
	pterm.Success.Printf("Extracted extension to %s\n", outDir)
	return nil
}

func (e ExtensionsCmd) Upload(ctx context.Context, in ExtensionsUploadInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Dir == "" {
		return fmt.Errorf("missing directory argument")
	}
	absDir, err := filepath.Abs(in.Dir)
	if err != nil {
		return fmt.Errorf("failed to resolve directory: %w", err)
	}
	stat, err := os.Stat(absDir)
	if err != nil || !stat.IsDir() {
		return fmt.Errorf("directory %s does not exist", absDir)
	}

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kernel_ext_%d.zip", time.Now().UnixNano()))
	if in.Output != "json" {
		pterm.Info.Println("Zipping extension directory...")
	}
	if err := util.ZipDirectory(absDir, tmpFile); err != nil {
		pterm.Error.Println("Failed to zip directory")
		return err
	}
	defer os.Remove(tmpFile)

	f, err := os.Open(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to open temp zip: %w", err)
	}
	defer f.Close()

	params := kernel.ExtensionUploadParams{File: f}
	if in.Name != "" {
		params.Name = kernel.Opt(in.Name)
	}
	item, err := e.extensions.Upload(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		bs, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	name := item.Name
	if name == "" {
		name = "-"
	}
	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", item.ID})
	rows = append(rows, []string{"Name", name})
	rows = append(rows, []string{"Created At", util.FormatLocal(item.CreatedAt)})
	rows = append(rows, []string{"Size (bytes)", fmt.Sprintf("%d", item.SizeBytes)})
	PrintTableNoPad(rows, true)
	return nil
}

// RFC9421 test key for Cloudflare's test site
const defaultWebBotAuthKey = `{"kty":"OKP","crv":"Ed25519","d":"n4Ni-HpISpVObnQMW0wOhCKROaIKqKtW_2ZYb2p9KcU","x":"JrQLj5P_89iXES9-vFgrIy29clF9CC_oPPsw3c5D0bs"}`

func (e ExtensionsCmd) BuildWebBotAuth(ctx context.Context, in ExtensionsBuildWebBotAuthInput) error {
	if in.Output == "" {
		return fmt.Errorf("missing --to output directory")
	}

	// Check npm is available
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm is required but not found in PATH. Please install Node.js and npm")
	}

	// Resolve output directory
	outDir, err := filepath.Abs(in.Output)
	if err != nil {
		return fmt.Errorf("failed to resolve output path: %w", err)
	}

	// Ensure output directory exists and is empty
	if st, err := os.Stat(outDir); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("output path exists and is not a directory: %s", outDir)
		}
		entries, _ := os.ReadDir(outDir)
		if len(entries) > 0 {
			return fmt.Errorf("output directory must be empty: %s", outDir)
		}
	} else {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Determine the signing key to use
	var keyJSON string
	if in.KeyFile != "" {
		keyData, err := os.ReadFile(in.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to read key file: %w", err)
		}
		// Validate it's valid JSON
		var keyObj map[string]interface{}
		if err := json.Unmarshal(keyData, &keyObj); err != nil {
			return fmt.Errorf("key file is not valid JSON: %w", err)
		}
		keyJSON = string(keyData)
		pterm.Info.Printf("Using signing key from: %s\n", in.KeyFile)
	} else {
		keyJSON = defaultWebBotAuthKey
		pterm.Info.Println("Using default RFC9421 test key (for Cloudflare's test site)")
	}

	// Create temp directory for building
	tmpDir, err := os.MkdirTemp("", "kernel-web-bot-auth-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download web-bot-auth repo tarball
	pterm.Info.Println("Downloading web-bot-auth from GitHub...")
	tarballURL := "https://github.com/cloudflare/web-bot-auth/archive/refs/heads/main.tar.gz"
	resp, err := http.Get(tarballURL)
	if err != nil {
		return fmt.Errorf("failed to download web-bot-auth: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download web-bot-auth: HTTP %d", resp.StatusCode)
	}

	// Extract tarball
	pterm.Info.Println("Extracting...")
	if err := extractTarGz(resp.Body, tmpDir); err != nil {
		return fmt.Errorf("failed to extract tarball: %w", err)
	}

	// Find the extracted directory (it will be named web-bot-auth-main)
	repoDir := filepath.Join(tmpDir, "web-bot-auth-main")
	if _, err := os.Stat(repoDir); err != nil {
		return fmt.Errorf("extracted directory not found: %w", err)
	}

	// Write the signing key
	keyDir := filepath.Join(repoDir, "examples", "rfc9421-keys")
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}
	keyPath := filepath.Join(keyDir, "ed25519.json")
	if err := os.WriteFile(keyPath, []byte(keyJSON), 0o644); err != nil {
		return fmt.Errorf("failed to write signing key: %w", err)
	}

	// Remove package-lock.json to work around npm optional dependencies bug
	// See: https://github.com/npm/cli/issues/4828
	_ = os.Remove(filepath.Join(repoDir, "package-lock.json"))

	// Run npm install at the repo root (workspace root) to install all dependencies including tsup
	pterm.Info.Println("Installing dependencies (npm install)...")
	npmInstall := exec.CommandContext(ctx, "npm", "install")
	npmInstall.Dir = repoDir
	npmInstall.Stdout = os.Stdout
	npmInstall.Stderr = os.Stderr
	if err := npmInstall.Run(); err != nil {
		return fmt.Errorf("npm install failed: %w", err)
	}

	// Build the web-bot-auth package first (the browser extension depends on it)
	pterm.Info.Println("Building web-bot-auth package...")
	npmBuildPkg := exec.CommandContext(ctx, "npm", "run", "build")
	npmBuildPkg.Dir = repoDir
	npmBuildPkg.Stdout = os.Stdout
	npmBuildPkg.Stderr = os.Stderr
	if err := npmBuildPkg.Run(); err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}

	// Run npm run bundle:chrome in the browser-extension directory (builds and packs as CRX)
	extDir := filepath.Join(repoDir, "examples", "browser-extension")
	pterm.Info.Println("Building and bundling extension (npm run bundle:chrome)...")
	npmBundle := exec.CommandContext(ctx, "npm", "run", "bundle:chrome")
	npmBundle.Dir = extDir
	npmBundle.Stdout = os.Stdout
	npmBundle.Stderr = os.Stderr
	if err := npmBundle.Run(); err != nil {
		return fmt.Errorf("npm run bundle:chrome failed: %w", err)
	}

	// Create unpacked subdirectory for the extension files (used for upload)
	unpackedDir := filepath.Join(outDir, "unpacked")
	if err := os.MkdirAll(unpackedDir, 0o755); err != nil {
		return fmt.Errorf("failed to create unpacked directory: %w", err)
	}

	// Copy built extension files to unpacked/
	builtDir := filepath.Join(extDir, "dist", "mv3", "chromium")

	// Copy background.mjs
	bgSrc := filepath.Join(builtDir, "background.mjs")
	if err := copyFile(bgSrc, filepath.Join(unpackedDir, "background.mjs")); err != nil {
		return fmt.Errorf("failed to copy background.mjs: %w", err)
	}

	// Copy manifest.json (bundle:chrome already adds version)
	manifestSrc := filepath.Join(builtDir, "manifest.json")
	if err := copyFile(manifestSrc, filepath.Join(unpackedDir, "manifest.json")); err != nil {
		return fmt.Errorf("failed to copy manifest.json: %w", err)
	}

	// Copy CRX bundle artifacts
	artifactsDir := filepath.Join(extDir, "dist", "web-ext-artifacts")
	crxSrc := filepath.Join(artifactsDir, "http-message-signatures-extension.crx")
	if err := copyFile(crxSrc, filepath.Join(outDir, "extension.crx")); err != nil {
		return fmt.Errorf("failed to copy extension.crx: %w", err)
	}

	updateXMLSrc := filepath.Join(artifactsDir, "update.xml")
	if err := copyFile(updateXMLSrc, filepath.Join(outDir, "update.xml")); err != nil {
		return fmt.Errorf("failed to copy update.xml: %w", err)
	}

	// Copy policy files
	policyDir := filepath.Join(extDir, "policy")
	policyJSONSrc := filepath.Join(policyDir, "policy.json")
	if err := copyFile(policyJSONSrc, filepath.Join(outDir, "policy.json")); err != nil {
		return fmt.Errorf("failed to copy policy.json: %w", err)
	}

	plistSrc := filepath.Join(policyDir, "com.google.Chrome.managed.plist")
	if err := copyFile(plistSrc, filepath.Join(outDir, "com.google.Chrome.managed.plist")); err != nil {
		return fmt.Errorf("failed to copy plist: %w", err)
	}

	// Copy RSA private key (useful for re-signing later)
	privateKeySrc := filepath.Join(extDir, "private_key.pem")
	if err := copyFile(privateKeySrc, filepath.Join(outDir, "private_key.pem")); err != nil {
		return fmt.Errorf("failed to copy private_key.pem: %w", err)
	}

	// Extract extension ID from update.xml and save it
	updateXMLData, err := os.ReadFile(updateXMLSrc)
	if err == nil {
		// Parse extension ID from update.xml (it's in the appid attribute)
		xmlStr := string(updateXMLData)
		if idx := strings.Index(xmlStr, `appid="`); idx != -1 {
			start := idx + 7
			if end := strings.Index(xmlStr[start:], `"`); end != -1 {
				extID := xmlStr[start : start+end]
				if err := os.WriteFile(filepath.Join(outDir, "extension-id.txt"), []byte(extID), 0o644); err != nil {
					pterm.Warning.Printf("Failed to write extension-id.txt: %v\n", err)
				}
			}
		}
	}

	pterm.Success.Printf("Built extension bundle to: %s\n", outDir)
	pterm.Info.Println("Bundle contents:")
	pterm.Info.Println("  - extension.crx      (packed extension for policy installation)")
	pterm.Info.Println("  - update.xml         (Chrome update manifest)")
	pterm.Info.Println("  - policy.json        (Linux/Chrome OS policy file)")
	pterm.Info.Println("  - com.google.Chrome.managed.plist (macOS policy file)")
	pterm.Info.Println("  - private_key.pem    (RSA key for re-signing)")
	pterm.Info.Println("  - extension-id.txt   (Chrome extension ID)")
	pterm.Info.Println("  - unpacked/          (unpacked extension files)")

	// Optionally upload
	if in.Upload {
		name := in.Name
		if name == "" {
			name = "web-bot-auth"
		}
		pterm.Info.Printf("Uploading extension as '%s'...\n", name)
		if err := e.Upload(ctx, ExtensionsUploadInput{Dir: unpackedDir, Name: name}); err != nil {
			return err
		}
	}

	return nil
}

// extractTarGz extracts a tar.gz stream to the destination directory
func extractTarGz(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		// Protect against directory traversal
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// --- Cobra wiring ---

var extensionsCmd = &cobra.Command{
	Use:     "extensions",
	Aliases: []string{"extension"},
	Short:   "Manage browser extensions",
	Long:    "Commands for managing Kernel browser extensions",
}

var extensionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List extensions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		output, _ := cmd.Flags().GetString("output")
		svc := client.Extensions
		e := ExtensionsCmd{extensions: &svc}
		return e.List(cmd.Context(), ExtensionsListInput{Output: output})
	},
}

var extensionsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete an extension by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		skip, _ := cmd.Flags().GetBool("yes")
		svc := client.Extensions
		e := ExtensionsCmd{extensions: &svc}
		return e.Delete(cmd.Context(), ExtensionsDeleteInput{Identifier: args[0], SkipConfirm: skip})
	},
}

var extensionsDownloadCmd = &cobra.Command{
	Use:   "download <id-or-name>",
	Short: "Download an extension archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		out, _ := cmd.Flags().GetString("to")
		svc := client.Extensions
		e := ExtensionsCmd{extensions: &svc}
		return e.Download(cmd.Context(), ExtensionsDownloadInput{Identifier: args[0], Output: out})
	},
}

var extensionsDownloadWebStoreCmd = &cobra.Command{
	Use:   "download-web-store <url>",
	Short: "Download an extension from the Chrome Web Store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		out, _ := cmd.Flags().GetString("to")
		osFlag, _ := cmd.Flags().GetString("os")
		svc := client.Extensions
		e := ExtensionsCmd{extensions: &svc}
		return e.DownloadWebStore(cmd.Context(), ExtensionsDownloadWebStoreInput{URL: args[0], Output: out, OS: osFlag})
	},
}

var extensionsUploadCmd = &cobra.Command{
	Use:   "upload <directory>",
	Short: "Upload an unpacked browser extension directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")
		svc := client.Extensions
		e := ExtensionsCmd{extensions: &svc}
		return e.Upload(cmd.Context(), ExtensionsUploadInput{Dir: args[0], Name: name, Output: output})
	},
}

var extensionsBuildWebBotAuthCmd = &cobra.Command{
	Use:   "build-web-bot-auth",
	Short: "Build Cloudflare's Web Bot Auth browser extension",
	Long: `Build the Web Bot Auth browser extension for signing HTTP requests.

This command downloads and builds Cloudflare's web-bot-auth browser extension,
which adds RFC 9421 HTTP Message Signatures to all outgoing requests.

The output includes:
  - extension.crx      (packed extension for policy installation)
  - update.xml         (Chrome update manifest)
  - policy.json        (Linux/Chrome OS policy file)
  - com.google.Chrome.managed.plist (macOS policy file)
  - private_key.pem    (RSA key for re-signing)
  - extension-id.txt   (Chrome extension ID)
  - unpacked/          (unpacked extension files for Kernel upload)

By default, it uses the RFC9421 test key that works with Cloudflare's test site
at https://http-message-signatures-example.research.cloudflare.com/

To use your own signing key, provide a JWK file with --key.

Examples:
  # Build with default test key
  kernel extensions build-web-bot-auth --to ./web-bot-auth-ext

  # Build with custom key
  kernel extensions build-web-bot-auth --to ./web-bot-auth-ext --key ./my-key.jwk

  # Build and upload to Kernel
  kernel extensions build-web-bot-auth --to ./web-bot-auth-ext --upload --name my-web-bot-auth`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		output, _ := cmd.Flags().GetString("to")
		keyFile, _ := cmd.Flags().GetString("key")
		upload, _ := cmd.Flags().GetBool("upload")
		name, _ := cmd.Flags().GetString("name")
		svc := client.Extensions
		e := ExtensionsCmd{extensions: &svc}
		return e.BuildWebBotAuth(cmd.Context(), ExtensionsBuildWebBotAuthInput{
			Output:  output,
			KeyFile: keyFile,
			Upload:  upload,
			Name:    name,
		})
	},
}

func init() {
	extensionsCmd.AddCommand(extensionsListCmd)
	extensionsCmd.AddCommand(extensionsDeleteCmd)
	extensionsCmd.AddCommand(extensionsDownloadCmd)
	extensionsCmd.AddCommand(extensionsDownloadWebStoreCmd)
	extensionsCmd.AddCommand(extensionsUploadCmd)
	extensionsCmd.AddCommand(extensionsBuildWebBotAuthCmd)

	extensionsListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	extensionsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	extensionsDownloadCmd.Flags().String("to", "", "Output zip file path")
	extensionsDownloadWebStoreCmd.Flags().String("to", "", "Output zip file path for the downloaded archive")
	extensionsDownloadWebStoreCmd.Flags().String("os", "", "Target OS: mac, win, or linux (default linux)")
	extensionsUploadCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	extensionsUploadCmd.Flags().String("name", "", "Optional unique extension name")

	extensionsBuildWebBotAuthCmd.Flags().String("to", "", "Output directory for the built extension (required)")
	extensionsBuildWebBotAuthCmd.Flags().String("key", "", "Path to JWK file with Ed25519 signing key (defaults to RFC9421 test key)")
	extensionsBuildWebBotAuthCmd.Flags().Bool("upload", false, "Upload the extension to Kernel after building")
	extensionsBuildWebBotAuthCmd.Flags().String("name", "web-bot-auth", "Extension name when uploading")
	_ = extensionsBuildWebBotAuthCmd.MarkFlagRequired("to")
}
