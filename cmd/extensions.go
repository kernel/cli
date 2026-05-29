package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kernel/cli/pkg/extensions"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	MaxExtensionSizeBytes = 50 * 1024 * 1024 // 50MB
)

// defaultExtensionExclusions contains patterns for files that are not needed
// when zipping Chrome extensions
var defaultExtensionExclusions = util.ZipOptions{
	ExcludeDirectories: []string{
		"node_modules",
		".git",
		"__tests__",
		"coverage",
	},
	ExcludeFilenamePatterns: []string{
		"*.test.js",
		"*.test.ts",
		"*.spec.js",
		"*.spec.ts",
		"*.log",
		"*.swp",
	},
}

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

// ExtensionsCmd handles extension operations independent of cobra.
type ExtensionsCmd struct {
	extensions ExtensionsService
}

func (e ExtensionsCmd) List(ctx context.Context, in ExtensionsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	if in.Output != "json" {
		pterm.Info.Println("Fetching extensions...")
	}
	items, err := e.extensions.List(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSONPointerSlice(items)
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
		return util.RequiredArg("extension ID or name", "kernel extensions delete <id-or-name>")
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
		return util.RequiredArg("extension ID or name", "kernel extensions download <id-or-name> --to <directory>")
	}
	res, err := e.extensions.Download(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()
	if in.Output == "" {
		_, _ = io.Copy(io.Discard, res.Body)
		return util.RequiredFlag("--to", "<directory>")
	}

	outDir, err := filepath.Abs(in.Output)
	if err != nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return fmt.Errorf("resolve --to path %q failed; choose a valid directory path: %w", in.Output, err)
	}
	// Create directory if not exists; if exists, ensure empty
	if st, err := os.Stat(outDir); err == nil {
		if !st.IsDir() {
			_, _ = io.Copy(io.Discard, res.Body)
			return fmt.Errorf("--to %q is a file; choose an empty directory", outDir)
		}
		entries, _ := os.ReadDir(outDir)
		if len(entries) > 0 {
			_, _ = io.Copy(io.Discard, res.Body)
			return fmt.Errorf("--to %q is not empty; choose an empty directory", outDir)
		}
	} else {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			_, _ = io.Copy(io.Discard, res.Body)
			return fmt.Errorf("create --to directory %q failed; check parent permissions: %w", outDir, err)
		}
	}

	// Write response to a temp zip, then extract
	tmpZip, err := os.CreateTemp("", "kernel-ext-*.zip")
	if err != nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return fmt.Errorf("create temporary extension zip failed; check temp directory permissions: %w", err)
	}
	tmpName := tmpZip.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := io.Copy(tmpZip, res.Body); err != nil {
		_ = tmpZip.Close()
		return fmt.Errorf("download extension archive failed while reading response: %w", err)
	}
	_ = tmpZip.Close()
	if err := util.Unzip(tmpName, outDir); err != nil {
		return fmt.Errorf("extract extension archive into %q failed; choose an empty writable directory: %w", outDir, err)
	}
	pterm.Success.Printf("Extracted extension to %s\n", outDir)
	return nil
}

func (e ExtensionsCmd) DownloadWebStore(ctx context.Context, in ExtensionsDownloadWebStoreInput) error {
	if in.URL == "" {
		return util.RequiredArg("Chrome Web Store URL", "kernel extensions download-web-store <url> --to <directory>")
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
		return util.InvalidChoice("--os", in.OS, "linux", "mac", "win")
	}

	res, err := e.extensions.DownloadFromChromeStore(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()

	if in.Output == "" {
		_, _ = io.Copy(io.Discard, res.Body)
		return util.RequiredFlag("--to", "<directory>")
	}

	outDir, err := filepath.Abs(in.Output)
	if err != nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return fmt.Errorf("resolve --to path %q failed; choose a valid directory path: %w", in.Output, err)
	}
	if st, err := os.Stat(outDir); err == nil {
		if !st.IsDir() {
			_, _ = io.Copy(io.Discard, res.Body)
			return fmt.Errorf("--to %q is a file; choose an empty directory", outDir)
		}
		entries, _ := os.ReadDir(outDir)
		if len(entries) > 0 {
			_, _ = io.Copy(io.Discard, res.Body)
			return fmt.Errorf("--to %q is not empty; choose an empty directory", outDir)
		}
	} else {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			_, _ = io.Copy(io.Discard, res.Body)
			return fmt.Errorf("create --to directory %q failed; check parent permissions: %w", outDir, err)
		}
	}

	// Save to temp zip then extract
	var bodyBuf bytes.Buffer
	if _, err := io.Copy(&bodyBuf, res.Body); err != nil {
		return fmt.Errorf("download Web Store archive failed while reading response: %w", err)
	}
	tmpZip, err := os.CreateTemp("", "kernel-webstore-*.zip")
	if err != nil {
		return fmt.Errorf("create temporary Web Store zip failed; check temp directory permissions: %w", err)
	}
	tmpName := tmpZip.Name()
	if _, err := tmpZip.Write(bodyBuf.Bytes()); err != nil {
		_ = tmpZip.Close()
		return fmt.Errorf("write temporary Web Store zip failed; check temp directory permissions: %w", err)
	}
	_ = tmpZip.Close()
	defer os.Remove(tmpName)
	if err := util.Unzip(tmpName, outDir); err != nil {
		return fmt.Errorf("extract Web Store archive into %q failed; choose an empty writable directory: %w", outDir, err)
	}
	pterm.Success.Printf("Extracted extension to %s\n", outDir)
	return nil
}

func (e ExtensionsCmd) Upload(ctx context.Context, in ExtensionsUploadInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	if in.Dir == "" {
		return util.RequiredArg("extension directory", "kernel extensions upload <directory>")
	}
	absDir, err := filepath.Abs(in.Dir)
	if err != nil {
		return fmt.Errorf("resolve extension directory %q failed; pass a valid path: %w", in.Dir, err)
	}
	stat, err := os.Stat(absDir)
	if err != nil || !stat.IsDir() {
		return fmt.Errorf("extension directory %q does not exist; pass an unpacked extension directory", absDir)
	}

	// Pre-flight size check
	if in.Output != "json" {
		pterm.Info.Println("Compressing extension directory...")
	}

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kernel_ext_%d.zip", time.Now().UnixNano()))

	if err := util.ZipDirectory(absDir, tmpFile, &defaultExtensionExclusions); err != nil {
		return fmt.Errorf("zip extension directory %q failed; check the directory contents: %w", absDir, err)
	}
	defer os.Remove(tmpFile)

	fileInfo, err := os.Stat(tmpFile)
	if err != nil {
		return fmt.Errorf("stat extension bundle %q failed: %w", tmpFile, err)
	}

	if in.Output != "json" {
		pterm.Success.Printf("Created bundle: %s\n", util.FormatBytes(fileInfo.Size()))
	}

	if fileInfo.Size() > MaxExtensionSizeBytes {
		pterm.Error.Printf("Extension bundle is too large: %s (max: %s)\n",
			util.FormatBytes(fileInfo.Size()), util.FormatBytes(MaxExtensionSizeBytes))
		pterm.Info.Println("\nSuggestions to reduce size:")
		pterm.Info.Println("  1. Ensure you're building the extension for production")
		pterm.Info.Println("  2. Remove unnecessary assets (large images, videos)")
		pterm.Info.Println("  3. Check manifest.json references only needed files")
		return fmt.Errorf("extension bundle is %s; keep it under %s", util.FormatBytes(fileInfo.Size()), util.FormatBytes(MaxExtensionSizeBytes))
	}

	f, err := os.Open(tmpFile)
	if err != nil {
		return fmt.Errorf("open extension bundle %q failed: %w", tmpFile, err)
	}
	defer f.Close()

	if in.Output != "json" {
		pterm.Info.Println("Uploading extension...")
	}

	params := kernel.ExtensionUploadParams{File: f}
	if in.Name != "" {
		params.Name = kernel.Opt(in.Name)
	}

	item, err := e.extensions.Upload(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
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
	Short: "Build the Cloudflare web-bot-auth extension for Kernel",
	Long: `Download, build, and prepare the Cloudflare web-bot-auth extension with Kernel-specific configurations.
					Defaults to RFC9421 test key (works with Cloudflare's test site).
					Uploads it to Kernel as 'web-bot-auth'. Optionally accepts a custom JWK or PEM key file.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("to")
		url, _ := cmd.Flags().GetString("url")
		keyPath, _ := cmd.Flags().GetString("key")
		uploadName, _ := cmd.Flags().GetString("upload")
		signatureAgentURL, _ := cmd.Flags().GetString("signature-agent")
		// Use upload name for extension name, or default to "web-bot-auth"
		extensionName := "web-bot-auth"
		if uploadName != "" {
			extensionName = uploadName
		}

		// Build the extension
		result, err := extensions.BuildWebBotAuth(cmd.Context(), extensions.ExtensionsBuildWebBotAuthInput{
			Output:            output,
			HostURL:           url,
			KeyPath:           keyPath,
			ExtensionName:     extensionName,
			AutoUpload:        uploadName != "",
			SignatureAgentURL: signatureAgentURL,
		})
		if err != nil {
			return err
		}

		// Upload if requested
		if uploadName != "" {
			client := getKernelClient(cmd)
			svc := client.Extensions
			e := ExtensionsCmd{extensions: &svc}
			pterm.Info.Println("Uploading extension to Kernel...")
			return e.Upload(cmd.Context(), ExtensionsUploadInput{
				Dir:  result.OutputDir,
				Name: extensionName,
			})
		}

		return nil
	},
}

func init() {
	extensionsCmd.AddCommand(extensionsListCmd)
	extensionsCmd.AddCommand(extensionsDeleteCmd)
	extensionsCmd.AddCommand(extensionsDownloadCmd)
	extensionsCmd.AddCommand(extensionsDownloadWebStoreCmd)
	extensionsCmd.AddCommand(extensionsUploadCmd)
	extensionsCmd.AddCommand(extensionsBuildWebBotAuthCmd)

	addJSONOutputFlag(extensionsListCmd)
	util.AddSkipConfirmFlag(extensionsDeleteCmd)
	extensionsDownloadCmd.Flags().String("to", "", "Output zip file path")
	extensionsDownloadWebStoreCmd.Flags().String("to", "", "Output zip file path for the downloaded archive")
	extensionsDownloadWebStoreCmd.Flags().String("os", "", "Target OS: mac, win, or linux (default linux)")
	addJSONOutputFlag(extensionsUploadCmd)
	extensionsUploadCmd.Flags().String("name", "", "Optional unique extension name")
	extensionsBuildWebBotAuthCmd.Flags().String("to", "./web-bot-auth", "Output directory for the prepared extension")
	extensionsBuildWebBotAuthCmd.Flags().String("url", "http://127.0.0.1:10001", "Base URL for update.xml and policy templates")
	extensionsBuildWebBotAuthCmd.Flags().String("key", "", "Path to Ed25519 private key file (JWK or PEM format)")
	extensionsBuildWebBotAuthCmd.Flags().String("upload", "", "Upload extension to Kernel with specified name (e.g., --upload web-bot-auth)")
	extensionsBuildWebBotAuthCmd.Flags().String("signature-agent", "", "Base URL of the signature agent (e.g., https://agent.example.com). Verifiers will look up /.well-known/http-message-signatures-directory at this URL.")
}
