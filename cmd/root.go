package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/kernel/cli/cmd/mcp"
	"github.com/kernel/cli/cmd/proxies"
	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/update"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type Metadata struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
}

var metadata = Metadata{
	// these are set at build-time via ldflags.
	// https://goreleaser.com/cookbooks/using-main.version/
	Version:   "dev",
	Commit:    "none",
	Date:      "unknown",
	GoVersion: runtime.Version(),
}

// rootCmd is the base command for the CLI.
var rootCmd = &cobra.Command{
	Use:   "kernel",
	Short: "CLI for Kernel deployment and invocation",
	Run: func(cmd *cobra.Command, args []string) {
		// If called without any subcommands, just show help.
		_ = cmd.Help()
	},
}

var logger *pterm.Logger

func logLevelToPterm(level string) pterm.LogLevel {
	switch level {
	case "trace":
		return pterm.LogLevelTrace
	case "debug":
		return pterm.LogLevelDebug
	case "info":
		return pterm.LogLevelInfo
	case "warn":
		return pterm.LogLevelWarn
	case "error":
		return pterm.LogLevelError
	case "fatal":
		return pterm.LogLevelFatal
	case "print":
		return pterm.LogLevelPrint
	default:
		return pterm.LogLevelInfo
	}
}

func getKernelClient(cmd *cobra.Command) kernel.Client {
	return util.GetKernelClient(cmd)
}

// isAuthExempt returns true if the command should skip auth.
func isAuthExempt(cmd *cobra.Command) bool {
	// Root command doesn't need auth
	if cmd == rootCmd {
		return true
	}

	// Walk up to find the top-level command (direct child of rootCmd)
	topLevel := cmd
	for topLevel.Parent() != nil && topLevel.Parent() != rootCmd {
		topLevel = topLevel.Parent()
	}

	// Check if the top-level command is in the exempt list
	switch topLevel.Name() {
	case "login", "logout", "help", "completion", "create", "mcp", "upgrade", "status":
		return true
	case "auth":
		// Only exempt the auth command itself (status display), not its subcommands
		return cmd == topLevel
	}

	return false
}

func init() {
	rootCmd.PersistentFlags().BoolP("version", "v", false, "Print the CLI version")
	rootCmd.PersistentFlags().BoolP("no-color", "", false, "Disable color output")
	rootCmd.PersistentFlags().String("log-level", "warn", "Set the log level (trace, debug, info, warn, error, fatal, print)")
	rootCmd.PersistentFlags().String("project", "", "Project ID or name to scope all requests to (or set KERNEL_PROJECT_ID env var)")
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	cobra.OnInitialize(initConfig)

	// Version flag handling: we use our own persistent pre-run to handle it globally.
	// We also inject a Kernel client object into the command context for commands to use
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		logLevel, _ := cmd.Flags().GetString("log-level")
		logger = pterm.DefaultLogger.WithLevel(logLevelToPterm(logLevel))
		if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
			pterm.DisableStyling()
		}

		// Skip auth check for commands that don't need it (including children, e.g., "completion zsh")
		if isAuthExempt(cmd) {
			return nil
		}

		clientOpts := []option.RequestOption{
			option.WithHeader("X-Kernel-Cli-Version", metadata.Version),
		}

		projectVal, _ := cmd.Flags().GetString("project")
		if projectVal == "" {
			projectVal = os.Getenv("KERNEL_PROJECT_ID")
		}

		// If the value looks like a name (not a cuid2 ID), we need to
		// resolve it after authenticating. Build the client first without
		// the project header, resolve, then re-create with the header.
		needsResolve := projectVal != "" && !looksLikeCUID(projectVal)

		if projectVal != "" && !needsResolve {
			clientOpts = append(clientOpts, option.WithHeader("X-Kernel-Project-Id", projectVal))
		}

		client, err := auth.GetAuthenticatedClient(clientOpts...)
		if err != nil {
			return fmt.Errorf("authentication required: %w", err)
		}

		if needsResolve {
			resolved, resolveErr := resolveProjectByName(cmd.Context(), *client, projectVal)
			if resolveErr != nil {
				return resolveErr
			}
			clientOpts = append(clientOpts, option.WithHeader("X-Kernel-Project-Id", resolved))
			client, err = auth.GetAuthenticatedClient(clientOpts...)
			if err != nil {
				return fmt.Errorf("authentication required: %w", err)
			}
		}

		ctx := context.WithValue(cmd.Context(), util.KernelClientKey, *client)
		cmd.SetContext(ctx)
		return nil
	}

	// Register subcommands
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(invokeCmd)
	rootCmd.AddCommand(browsersCmd)
	rootCmd.AddCommand(browserPoolsCmd)
	rootCmd.AddCommand(appCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(proxies.ProxiesCmd)
	rootCmd.AddCommand(extensionsCmd)
	rootCmd.AddCommand(credentialsCmd)
	rootCmd.AddCommand(credentialProvidersCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(mcp.MCPCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(statusCmd)

	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		// running synchronously so we never slow the command
		update.MaybeShowMessage(cmd.Context(), metadata.Version, 24*time.Hour)
		return nil
	}
}

func initConfig() {
	// Placeholder for future configuration (env vars, config files, etc.)
	pterm.EnableStyling() // ensure pterm is initialised in case env disables it
}

// Execute executes the root command.
func Execute(m Metadata) {
	metadata = m
	vt := "kernel"
	if metadata.Version != "" {
		vt += " " + metadata.Version
	}
	if metadata.Commit != "" {
		vt += " (" + metadata.Commit + ")"
	}
	if metadata.GoVersion != "" {
		vt += " " + metadata.GoVersion
	}
	if metadata.Date != "" {
		vt += " " + metadata.Date
	}
	vt += "\n"
	rootCmd.SetVersionTemplate(vt)
	if err := fang.Execute(context.Background(), rootCmd,
		fang.WithVersion(metadata.Version),
		fang.WithCommit(metadata.Commit),
		fang.WithErrorHandler(func(w io.Writer, styles fang.Styles, err error) {
			err = util.CleanedUpSdkError{Err: err}
			// remove margins so that it matches other pterm.error "style"
			// we should add them back later as it looks cleaner
			errorTextStyle := styles.ErrorText.UnsetMargins()
			pterm.Error.Println(errorTextStyle.Render(strings.TrimSpace(err.Error())))
			if isUsageError(err) {
				pterm.Println()
				pterm.Println(lipgloss.JoinHorizontal(
					lipgloss.Left,
					errorTextStyle.UnsetWidth().Render("Try"),
					styles.Program.Flag.Render("--help"),
					errorTextStyle.UnsetWidth().UnsetTransform().PaddingLeft(1).Render("for usage."),
				))
			}
		}),
	); err != nil {
		// fang takes care of printing the error
		os.Exit(1)
	}
}

// isUsageError is a hack to detect usage errors.
// See: https://github.com/spf13/cobra/pull/2266
// from github.com/charmbracelet/fang/help.go
func isUsageError(err error) bool {
	s := err.Error()
	for _, prefix := range []string{
		"flag needs an argument:",
		"unknown flag:",
		"unknown shorthand flag:",
		"unknown command",
		"invalid argument",
	} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// looksLikeCUID returns true if s matches the cuid2 format used for resource IDs
// (24 lowercase alphanumeric characters).
func looksLikeCUID(s string) bool {
	if len(s) != 24 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !(c >= 'a' && c <= 'z') {
				return false
			}
		} else if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// resolveProjectByName lists the caller's projects and returns the ID of the
// one whose name matches (case-insensitive). Returns an error if no match or
// multiple matches are found.
func resolveProjectByName(ctx context.Context, client kernel.Client, name string) (string, error) {
	projects, err := client.Projects.List(ctx, kernel.ProjectListParams{})
	if err != nil {
		return "", fmt.Errorf("failed to resolve project name %q: %w", name, err)
	}
	var matched []struct{ id, name string }
	lower := strings.ToLower(name)
	for _, p := range projects.Items {
		if strings.ToLower(p.Name) == lower {
			matched = append(matched, struct{ id, name string }{p.ID, p.Name})
		}
	}
	switch len(matched) {
	case 0:
		return "", fmt.Errorf("no project found with name %q", name)
	case 1:
		pterm.Debug.Printf("Resolved project %q → %s\n", matched[0].name, matched[0].id)
		return matched[0].id, nil
	default:
		return "", fmt.Errorf("multiple projects match name %q; use a project ID instead", name)
	}
}

// onCancel runs a function when the provided context is cancelled
func onCancel(ctx context.Context, fn func()) {
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.Canceled {
			fn()
		}
	}()
}
