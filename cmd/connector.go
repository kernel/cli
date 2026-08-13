package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kernel/cli/internal/connector"
	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
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
	input := ProfilesImportLocalInput{
		Count:           10,
		Days:            30,
		ProjectID:       link.ProjectID,
		Version:         metadata.Version,
		WaitTimeout:     30 * time.Minute,
		DashboardLaunch: true,
	}
	project, err := validateConnectorProject(cmd, input.ProjectID)
	if err != nil {
		recovered, recoveryErr := recoverConnectorAuthentication(
			cmd,
			err,
			interactive.NewPrompter().ConfirmDefault,
			runLoginWithForce,
		)
		if recoveryErr != nil {
			return recoveryErr
		}
		if !recovered {
			return err
		}
		if err := authenticateConnector(cmd); err != nil {
			return err
		}
		project, err = validateConnectorProject(cmd, input.ProjectID)
		if err != nil {
			return err
		}
	}
	input.Project = &project
	return runProfilesImportLocalWithInput(cmd, input)
}

type connectorConfirm func(action, prompt string, defaultValue bool) (bool, error)
type connectorLogin func(*cobra.Command, bool) error
type connectorClientFactory func(...option.RequestOption) (*kernel.Client, error)

func recoverConnectorAuthentication(cmd *cobra.Command, cause error, confirm connectorConfirm, login connectorLogin) (bool, error) {
	if !dashboardProjectAuthRecovery(cause) {
		return false, nil
	}
	hasAPIKey := os.Getenv("KERNEL_API_KEY") != ""
	prompt := "The current Kernel account cannot access this project. Switch accounts with browser sign-in?"
	if hasAPIKey {
		prompt = "Kernel API key is invalid, disabled, or cannot access this project. Continue with browser sign-in instead?"
	}
	proceed, err := confirm(
		"continue with browser sign-in",
		prompt,
		true,
	)
	if err != nil {
		return false, err
	}
	if !proceed {
		if !hasAPIKey {
			return false, fmt.Errorf("account switch declined; run `kernel login --force` when you are ready to switch accounts, then open the import again")
		}
		return false, fmt.Errorf("browser sign-in declined; unset KERNEL_API_KEY or replace it with a valid key, then open the import again")
	}
	if hasAPIKey {
		if err := os.Unsetenv("KERNEL_API_KEY"); err != nil {
			return false, fmt.Errorf("ignore invalid KERNEL_API_KEY for this import: %w", err)
		}
	}
	pterm.Info.Println("Using browser sign-in for this import; your shell environment was not changed")
	if err := login(cmd, true); err != nil {
		return false, err
	}
	return true, nil
}

func validateConnectorProject(cmd *cobra.Command, projectID string) (kernel.Project, error) {
	client := getKernelClient(cmd)
	return chooseImportProject(cmd.Context(), &client.Projects, interactive.NewPrompter(), projectID, true)
}

func authenticateConnector(cmd *cobra.Command) error {
	return authenticateConnectorWith(
		cmd,
		interactive.NewPrompter().ConfirmDefault,
		runLoginWithForce,
		auth.GetAuthenticatedClient,
	)
}

func authenticateConnectorWith(cmd *cobra.Command, confirm connectorConfirm, login connectorLogin, getClient connectorClientFactory) error {
	client, err := getClient(option.WithHeader("X-Kernel-Cli-Version", metadata.Version))
	if err != nil {
		if !errors.Is(err, auth.ErrAuthenticationRequired) {
			return err
		}
		proceed, promptErr := confirm(
			"sign in to continue this browser import",
			"Sign in to Kernel to continue this browser import?",
			true,
		)
		if promptErr != nil {
			return promptErr
		}
		if !proceed {
			return fmt.Errorf("Kernel sign-in is required to continue this browser import")
		}
		if err := login(cmd, false); err != nil {
			return err
		}
		client, err = getClient(option.WithHeader("X-Kernel-Cli-Version", metadata.Version))
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
