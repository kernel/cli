package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kernel/cli/pkg/create"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// CreateCmd is a cobra-independent command handler for create operations
type CreateCmd struct{}

// Create executes the creating a new Kernel app logic
func (c CreateCmd) Create(ctx context.Context, ci create.CreateInput) error {
	appPath, err := filepath.Abs(ci.Name)
	if err != nil {
		return fmt.Errorf("failed to resolve app path: %w", err)
	}

	// Check if directory already exists and prompt for overwrite
	if _, err := os.Stat(appPath); err == nil {
		overwrite, err := create.PromptForOverwrite(ci.Name)
		if err != nil {
			return fmt.Errorf("failed to prompt for overwrite: %w", err)
		}

		if !overwrite {
			pterm.Warning.Println("Operation cancelled.")
			return nil
		}

		// Remove existing directory
		if err := os.RemoveAll(appPath); err != nil {
			return fmt.Errorf("failed to remove existing directory: %w", err)
		}
	}

	if err := os.MkdirAll(appPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	pterm.Printfln("\nCreating a new %s %s", ci.Language, ci.Template)

	spinner, _ := pterm.DefaultSpinner.Start("Copying template files...")

	if err := create.CopyTemplateFiles(appPath, ci.Language, ci.Template); err != nil {
		spinner.Fail("Failed to copy template files")
		return fmt.Errorf("failed to copy template files: %w", err)
	}
	spinner.Success()

	nextSteps, err := create.InstallDependencies(appPath, ci)
	if err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}
	pterm.Success.Println("🎉 Kernel app created successfully!")
	pterm.Println()
	pterm.FgYellow.Println(nextSteps)

	return nil
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new application",
	Long:  "Commands for creating new Kernel applications",
	RunE:  runCreateApp,
}

func init() {
	createCmd.Flags().StringP("name", "n", "", "Name of the application")
	createCmd.Flags().StringP("language", "l", "", "Language of the application")
	createCmd.Flags().StringP("template", "t", "", "Template to use for the application")
}

func runCreateApp(cmd *cobra.Command, args []string) error {
	appName, _ := cmd.Flags().GetString("name")
	language, _ := cmd.Flags().GetString("language")
	template, _ := cmd.Flags().GetString("template")

	appName, err := create.PromptForAppName(appName)
	if err != nil {
		return fmt.Errorf("failed to get app name: %w", err)
	}

	language, err = create.PromptForLanguage(language)
	if err != nil {
		return fmt.Errorf("failed to get language: %w", err)
	}

	template, err = create.PromptForTemplate(template, language)
	if err != nil {
		return fmt.Errorf("failed to get template: %w", err)
	}

	c := CreateCmd{}
	return c.Create(cmd.Context(), create.CreateInput{
		Name:     appName,
		Language: language,
		Template: template,
	})
}
