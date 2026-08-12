package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kernel/cli/internal/connector"
	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var connectorCmd = &cobra.Command{
	Use:   "connector",
	Short: "Connect Kernel dashboard actions to the local CLI",
}

var connectorOpenCmd = &cobra.Command{
	Use:    "open <kernel-url>",
	Short:  "Open a Kernel dashboard action",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE:   runConnectorOpen,
}

var connectorInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Register kernel:// links for this user",
	Args:  cobra.NoArgs,
	RunE:  runConnectorInstall,
}

func init() {
	connectorCmd.AddCommand(connectorOpenCmd, connectorInstallCmd)
	rootCmd.AddCommand(connectorCmd)
}

func runConnectorOpen(cmd *cobra.Command, args []string) error {
	link, err := connector.ParseBrowserImportLink(args[0])
	if err != nil {
		return err
	}
	if err := authenticateConnector(cmd); err != nil {
		return err
	}
	pterm.Info.Println("Browser import requested from a kernel:// link")
	return runProfilesImportLocalWithInput(cmd, ProfilesImportLocalInput{
		Count:           5,
		Days:            30,
		ProjectID:       link.ProjectID,
		Version:         metadata.Version,
		WaitTimeout:     30 * time.Minute,
		DashboardLaunch: true,
	})
}

func authenticateConnector(cmd *cobra.Command) error {
	client, err := auth.GetAuthenticatedClient(option.WithHeader("X-Kernel-Cli-Version", metadata.Version))
	if err != nil {
		pterm.Info.Println("Sign in to continue this browser import")
		if err := runLogin(cmd, nil); err != nil {
			return err
		}
		client, err = auth.GetAuthenticatedClient(option.WithHeader("X-Kernel-Cli-Version", metadata.Version))
		if err != nil {
			return fmt.Errorf("authentication required: %w", err)
		}
	}
	cmd.SetContext(context.WithValue(cmd.Context(), util.KernelClientKey, *client))
	return nil
}

func runConnectorInstall(cmd *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	executable, err := stableKernelExecutable()
	if err != nil {
		return err
	}
	installed, err := connector.Install(cmd.Context(), home, executable)
	if err != nil {
		return err
	}
	pterm.Success.Printf("Kernel Connector installed at %s\n", installed)
	return nil
}

func stableKernelExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find Kernel executable: %w", err)
	}
	return stableExecutablePath(executable), nil
}

func stableExecutablePath(executable string) string {
	const cellarSegment = "/Cellar/"
	if index := strings.Index(filepath.ToSlash(executable), cellarSegment); index >= 0 {
		return filepath.Join(executable[:index], "bin", "kernel")
	}
	return executable
}
